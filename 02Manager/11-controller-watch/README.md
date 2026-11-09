# 11-controller-watch · 拆开 Watch

## 这个章节在学什么

一句话：

> **`c.Watch(src)` 把“一个能把 Request 塞进队列的东西”注册给控制器；`source.Kind` 只是其中最常用的一种。**

两个目录，除事件源外代码完全一致：

| 目录 | 事件源 | 触发方式 |
|---|---|---|
| `with-source/` | `source.Kind`（informer） | 集群里有对象变化 |
| `custom-source/` | 自定义 `injectedSource` | 定时凭空塞 Request |

## 一、`Watch` 做了什么

`pkg/internal/controller/controller.go:238`：

```go
func (c *Controller[request]) Watch(src source.TypedSource[request]) error {
	if !c.startedEventSourcesAndQueue {
		c.startWatches = append(c.startWatches, src)   // 还没启动：先存着
		return nil
	}
	c.LogConstructor(nil).Info("Starting EventSource", "source", src)
	return src.Start(c.ctx, c.Queue)                  // 已启动：立即启动
}
```

**两种时机：**

```text
Start 之前 Watch  →  只登记，等 Start 时统一启动
Start 之后 Watch  →  立刻 Start（日志会多一条 Starting EventSource）
```

## 二、`Source` 接口只有一个方法

```go
type TypedSource[request comparable] interface {
	Start(context.Context, workqueue.TypedRateLimitingInterface[request]) error
}
```

**拿到队列，往里塞 Request——这就是契约。**

另有一个增强版：

```go
type TypedSyncingSource[request comparable] interface {
	TypedSource[request]
	WaitForSync(ctx context.Context) error   // worker 启动前必须同步完
}
```

`source.Kind` 实现的是后者，所以控制器会等它同步完成再启动 worker。

## 三、KindSource 的 `Start` 内部

`pkg/internal/source/kind.go:63`：

```text
① 轮询 cache.GetInformer(ctx, type)       每 10s 重试
      ↑ CRD 还没装、缓存还没起，都会在这里等
② informer.AddEventHandlerWithOptions(
        NewEventHandler(ctx, queue, handler, predicates))
③ cache.WaitForCacheSync(ctx)
④ toolscache.WaitForCacheSync(handlerRegistration.HasSynced)
⑤ close(startedErr)                        ← WaitForSync 等这个
```

## 四、事件 → Request 的完整路径

```text
apiserver
   │ List/Watch
   ▼
informer（缓存）
   │ OnAdd / OnUpdate / OnDelete
   ▼
internal.EventHandler
   │ ① 跑 predicates（不通过直接丢）
   │ ② 调 handler.Create/Update/Delete
   ▼
handler.EnqueueRequestForObject
   │ 只取 namespace + name
   ▼
workqueue.Add(Request)
   │ 去重、限速、重试
   ▼
worker → Reconciler.Reconcile(ctx, req)
```

## 五、实测 A：KindSource

```text
INFO setup  事件源：source.Kind
INFO Starting EventSource {"controller": "configmap-controller", "source": "kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller": "configmap-controller"}
INFO Starting workers     {"controller": "configmap-controller", "worker count": 1}

# 启动时的全量对象
INFO Reconcile 被调用 {"request": "test/kube-root-ca.crt", "assembly": "kind-source"}
INFO Reconcile 被调用 {"request": "test/demo-config",    "assembly": "kind-source"}

# kubectl create configmap demo-watch
INFO Reconcile 被调用 {"request": "test/demo-watch", "assembly": "kind-source"}

# kubectl delete configmap demo-watch
INFO Reconcile 被调用 {"request": "test/demo-watch", "assembly": "kind-source"}
```

**create 和 delete 各触发一次**，且请求内容相同（只有 namespace/name）。

## 六、实测 B：自定义 Source

```go
func (s *injectedSource) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	go func() {
		for i := 1; ; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				queue.Add(reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: "test", Name: fmt.Sprintf("injected-%d", i)},
				})
			}
		}
	}()
	return nil
}
```

日志：

```text
INFO setup       事件源：自定义 injectedSource
INFO Starting EventSource {"controller": "configmap-controller", "source": "injected source"}
INFO injectedSource  Start 被调用，拿到队列 {"every": "3s"}
INFO Starting Controller
INFO Starting workers

INFO injectedSource  往队列塞入 Request {"request": "test/injected-1"}
INFO Reconcile 被调用 {"request": "test/injected-1", "assembly": "custom-source"}
INFO injectedSource  往队列塞入 Request {"request": "test/injected-2"}
INFO Reconcile 被调用 {"request": "test/injected-2"}
INFO injectedSource  往队列塞入 Request {"request": "test/injected-3"}
INFO Reconcile 被调用 {"request": "test/injected-3"}
```

**注意：`test/injected-1` 这个对象根本不存在，Reconcile 照样被调用。**

这证明了三件事：

| 结论 | 说明 |
|---|---|
| Request 只是 namespace/name | 不含对象本身 |
| Source 不必是 informer | 任何能往队列塞东西的都行 |
| 对象是否存在由 Reconciler 自己判断 | `r.Get` 返回 NotFound 时要自己处理 |

## 七、启动时序

```text
controller.Start
  ├── 创建队列：NewQueue(name, rateLimiter)
  ├── 并行启动所有 source（每个都有 CacheSyncTimeout）
  │     └── 日志：Starting EventSource
  ├── 等 SyncingSource.WaitForSync
  ├── 日志：Starting Controller
  └── 启动 workers
        └── 日志：Starting workers
```

**这就是为什么 `Starting EventSource` 总在 `Starting workers` 之前。**

## 八、关键结论

1. `Watch` = 登记一个 Source；启动后立刻调它的 `Start`。
2. Source 的契约只有一个：`Start(ctx, queue)`，把 Request 塞进队列。
3. `source.Kind` 内部做四件事：找 informer、挂 EventHandler、等缓存同步、等 handler 同步。
4. Request 只含 namespace/name，不含对象。
5. 对象是否存在、是否被删除，由 Reconciler 自己判断。
6. SyncingSource 会阻塞 worker 启动，普通 Source 不会。

## 九、运行

```bash
cd 02Manager/11-controller-watch

go run ./with-source
kubectl create configmap demo-watch -n test --from-literal=a=1
kubectl delete configmap demo-watch -n test

go run ./custom-source       # 不需要任何 kubectl 操作
```

## 十、一句话总结

> **`Watch` 不关心事件从哪来，它只要求 Source 把 Request 放进队列；informer 只是“从 apiserver 产生 Request”的一种实现。**