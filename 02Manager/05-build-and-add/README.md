# 05-build-and-add · Complete() 到底做了什么

## 学习目标

把 `Complete()` 拆成两个可以单独调用的动作：

```text
NewControllerManagedBy(mgr).For(&ConfigMap{}).Complete(r)
        │                    │                │
        │                    │                └── Build(r) + 忽略返回值
        │                    └── 记录 forInput
        └── 造 Builder

Build(r)
  ├── doWatch()       → controller.Watch(source.Kind(cache, obj, handler))
  └── doController(r) → controller.New(name, mgr, options) → mgr.Add(controller)
```

## 六份代码

| 目录 | 写法 | 目的 |
|---|---|---|
| `./` | `controller.New` + `c.Watch(...)` | 手动做 Build 的两件事 |
| `./builder` | `For(...).Complete(...)` | 自动做同样两件事 |
| `./no-watch` | 只 `controller.New`，不 Watch | 反例：控制器空转 |
| `./steps` | 用 `-mode` 切换三个步骤 | 同一条链路，逐级观察每一步的作用 |
| `./compare/manual` `./compare/builder` | 双份对照 | 除装配段外完全相同，严格对照 |
| **`./compare/split-manager`** | 手动组装 `NewManager` 内部 | **拆开看 Manager 到底建了什么** |

运行：

```bash
go run .            # 手动装配
go run ./builder    # Builder 装配
go run ./no-watch   # 反例

# 分解演示
go run ./steps -mode=new-only    # 只 controller.New
go run ./steps -mode=new-watch   # controller.New + Watch
go run ./steps -mode=builder     # For + Complete

# 严格对照（两份代码只差装配段）
go run ./compare/manual
go run ./compare/builder
```

## 严格对照：`compare/manual` vs `compare/builder`

两份代码共享同一个 `compare/shared.Reconciler`，Manager 配置、Scheme、日志全部相同，
**只有中间标记「唯一不同」的装配段不同**：

```go
// manual：两步
c, _ := controller.New("demo-configmap", mgr, controller.Options{Reconciler: r})
c.Watch(source.Kind(mgr.GetCache(), client.Object(&corev1.ConfigMap{}),
	&handler.EnqueueRequestForObject{}))
```

```go
// builder：一句，且用 Named 取同样的名字
ctrl.NewControllerManagedBy(mgr).
	Named("demo-configmap").
	For(&corev1.ConfigMap{}).
	Complete(r)
```

### 实测日志对比

手动版：

```text
INFO Starting EventSource {"controller":"demo-configmap","source":"kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller":"demo-configmap"}
INFO Starting workers     {"controller":"demo-configmap","worker count":1}
INFO Reconcile 被调用     {"controller":"demo-configmap","object":{"name":"demo-compare",...},"assembly":"manual"}
```

Builder 版：

```text
INFO Starting EventSource {"controller":"demo-configmap","controllerGroup":"","controllerKind":"ConfigMap","source":"kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller":"demo-configmap","controllerGroup":"","controllerKind":"ConfigMap"}
INFO Starting workers     {"controller":"demo-configmap","controllerGroup":"","controllerKind":"ConfigMap","worker count":1}
INFO Reconcile 被调用     {"controller":"demo-configmap","controllerGroup":"","controllerKind":"ConfigMap","ConfigMap":{"name":"demo-compare",...},"assembly":"builder"}
```

### 行为完全相同，只多出三样东西

| 差异 | 手动 | Builder |
|---|---|---|
| 控制器名 | 手写 | 自动（`Named` 可覆盖） |
| 对象字段名 | `object` | `ConfigMap`（按 kind 命名） |
| 额外日志字段 | 无 | `controllerGroup` / `controllerKind` |
| 触发次数 | 2 初始 + create + delete | **完全一致** |

### `Complete()` 额外做的事（相对于手动两步）

| 动作 | 手动 | Builder |
|---|---|---|
| 创建 controller + `mgr.Add` | ✅ 自己写 | ✅ 内部做 |
| 注册 watch | ✅ 自己写 | ✅ `For` 做 |
| 用 Scheme 解析 GVK | ❌ 不需要 | ✅ 用于命名和日志 |
| 生成 log constructor | ❌ | ✅ |
| 校验（nil reconciler / 重复 For） | ❌ | ✅ `Build` 时检查 |
| 支持 predicate / Owns / Watches | 手写 | 现成 |

## 拆分 `NewManager`：手动组装四个部件

`compare/split-manager/main.go` 不用 `ctrl.NewManager`，而是自己做它内部做的事：

```go
httpClient, _ := rest.HTTPClientFor(cfg)
mapper, _     := apiutil.NewDynamicRESTMapper(cfg, httpClient)

cmCache, _ := cache.New(cfg, cache.Options{
	Scheme: scheme, Mapper: mapper, HTTPClient: httpClient,
	DefaultNamespaces: map[string]cache.Config{"test": {}},
})

cachedClient, _ := client.New(cfg, client.Options{          // = mgr.GetClient()
	Scheme: scheme, Mapper: mapper, HTTPClient: httpClient,
	Cache: &client.CacheOptions{Reader: cmCache},
})

apiReader, _ := client.New(cfg, client.Options{             // = mgr.GetAPIReader()
	Scheme: scheme, Mapper: mapper, HTTPClient: httpClient,
})
```

然后是 Manager 不会替你做的最后一步——**启动缓存**：

```go
informer, _ := cmCache.GetInformer(ctx, &corev1.ConfigMap{})
informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{...})

go cmCache.Start(ctx)
cmCache.WaitForCacheSync(ctx)
```

### 实测输出

```text
13:53:41  INFO setup  【1/4】RESTMapper 就绪
13:53:41  INFO setup  【2/4】Cache 就绪（尚未 Start）
13:53:41  INFO setup  【3/4】CachedClient + APIReader 就绪
13:53:41  INFO setup  【4/4】Informer 与 Handler 就绪
13:53:41  INFO setup  informer: Add  {"name": "demo-config"}
13:53:41  INFO setup  informer: Add  {"name": "kube-root-ca.crt"}
13:53:41  INFO setup  缓存已同步（这一步在 Manager 里由 Start 自动完成）
13:53:41  INFO setup  cachedClient 读到  {"count": 2}
13:53:41  INFO setup  apiReader 读到     {"count": 119}
13:53:59  INFO setup  informer: Add     {"name": "demo-split"}
13:54:03  INFO setup  informer: Update  {"name": "demo-split"}
13:54:06  INFO setup  informer: Delete  {"name": "demo-split"}
```

### 一个重要发现：`apiReader` 不受 `DefaultNamespaces` 限制

```text
cachedClient 读到 2     ← 缓存只包含 test 命名空间
apiReader    读到 119   ← 直连整个集群，所有命名空间
```

`Cache.DefaultNamespaces` 只约束**缓存**。`GetAPIReader()` 和所有写操作都是直连，
不受缓存范围影响。

### Manager 替你做了什么

| 动作 | 手动（split-manager） | `ctrl.NewManager` |
|---|---|---|
| 建 mapper / cache / client ×2 | ✅ 自己写 | ✅ |
| 启动缓存 | `go cmCache.Start(ctx)` | `mgr.Start(ctx)` |
| 等缓存同步 | `WaitForCacheSync` | 自动（控制器启动前） |
| 事件 → 队列 → worker | ❌ 只有 informer 回调 | ✅ Controller |
| Leader Election | ❌ | ✅ |
| Metrics / Healthz / Pprof | ❌ | ✅ |
| 优雅关闭 | ❌ 自己处理 ctx | ✅ |
| 组件统一生命周期 | ❌ | ✅ |

一句话：

> **`NewManager` = 这四件套 + 生命周期 + 6 组 runnables。**
> 拆开之后你能看到“缓存必须显式启动”，也就能理解为什么 `mgr.Start` 之后才有事件。

## 分解演示：每一步各管什么

### 步骤 1：只 `controller.New`

```text
13:39:24  INFO setup  【步骤 1】controller.New：控制器已注册，但没有 Watch
13:39:24  INFO Starting Controller {"controller":"step-new-only"}
13:39:24  INFO Starting workers    {"controller":"step-new-only","worker count":1}
```

创建、删除 `demo-step1` 后：**一行 Reconcile 都没有**。

| 做到了 | 没做到 |
|---|---|
| 控制器注册进 Manager | 没有事件源 |
| workqueue + worker 创建并启动 | 永远不会收到事件 |

**`controller.New` 只负责“装配执行者”，不负责“接事件”。**

### 步骤 2：`controller.New` + `Watch`

```text
13:39:47  INFO setup  【步骤 1+2】controller.New + Watch：事件源已注册
13:39:47  INFO Starting EventSource {"controller":"step-new-watch","source":"kind source: *v1.ConfigMap"}
13:39:48  INFO Starting Controller  {"controller":"step-new-watch"}
13:39:48  INFO Starting workers     {"controller":"step-new-watch","worker count":1}
13:39:48  INFO Reconcile 被调用     {"name":"demo-config"}
13:39:48  INFO Reconcile 被调用     {"name":"kube-root-ca.crt"}
13:39:59  INFO Reconcile 被调用     {"name":"demo-step2"}   ← create
13:40:03  INFO Reconcile 被调用     {"name":"demo-step2"}   ← delete
```

| 步骤 | 作用 |
|---|---|
| `controller.New` | 执行者（worker） |
| `Watch` | 事件源（哪些对象的变化要入队） |

注意多出的 `Starting EventSource`，以及 `EventSource → Controller` 之间隔了 1 秒（等缓存同步）。

### 步骤 3：`For + Complete`

```text
13:40:18  INFO setup  【步骤 1+2 封装】For + Complete
13:40:18  INFO Starting EventSource {"controller":"configmap","controllerKind":"ConfigMap","source":"kind source: *v1.ConfigMap"}
13:40:19  INFO Starting Controller  {"controller":"configmap","controllerKind":"ConfigMap"}
13:40:19  INFO Starting workers     {"controller":"configmap","controllerKind":"ConfigMap","worker count":1}
```

行为与步骤 2 完全一致，差别只在表面：

| 项 | 步骤 2（手动） | 步骤 3（Builder） |
|---|---|---|
| 控制器名 | 自己写 `step-new-watch` | 自动 `configmap` |
| 日志字段 | 只有 `object` | `controllerKind` + `ConfigMap` |
| 后续扩展 | 每种源都要手写 `Watch` | `Owns` / `Watches` / predicate 现成 |

### 三步对照总表

| | 步骤 1 | 步骤 2 | 步骤 3 |
|---|---|---|---|
| 代码 | `controller.New` | `+ c.Watch(...)` | `For().Complete()` |
| 有 worker | ✅ | ✅ | ✅ |
| 有 EventSource | ❌ | ✅ | ✅ |
| 能收到事件 | ❌ | ✅ | ✅ |
| 名字 | 手动 | 手动 | 自动 |
| 日志带 kind | ❌ | ❌ | ✅ |

## 实测对比（namespace=test）

### 手动装配

```text
INFO setup               手动装配完成：controller.New + Watch
INFO Starting EventSource {"controller":"manual-configmap","source":"kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller":"manual-configmap"}
INFO Starting workers     {"controller":"manual-configmap","worker count":1}
INFO Reconcile 被调用     {"controller":"manual-configmap","object":{"name":"demo-manual",...},"mode":"手动 controller.New"}
```

### Builder 装配

```text
INFO setup                Builder 装配完成：For + Complete
INFO Starting EventSource {"controller":"configmap","controllerGroup":"","controllerKind":"ConfigMap","source":"kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller":"configmap","controllerGroup":"","controllerKind":"ConfigMap"}
INFO Starting workers     {"controller":"configmap","controllerGroup":"","controllerKind":"ConfigMap","worker count":1}
INFO Reconcile 被调用     {"controller":"configmap","ConfigMap":{"name":"demo-builder",...},"mode":"Builder Complete"}
```

**行为完全一致**，只有三个表面差异：

| 差异 | 手动 | Builder |
|---|---|---|
| 控制器名 | 自己指定 `manual-configmap` | 默认 `configmap`（kind 小写） |
| 日志字段 | 只有 `object` | `controllerGroup` / `controllerKind` / `ConfigMap` |
| 对象 key 字段名 | `object` | `ConfigMap`（按 kind 命名） |

后两条来自 Builder 生成的 **log constructor**，不影响功能。

### 反例：没有 Watch

```text
INFO setup               已注册控制器，但没有调用 Watch —— 创建 ConfigMap 不会有任何反应
INFO Starting Controller {"controller":"no-watch"}
INFO Starting workers    {"controller":"no-watch","worker count":1}
```

**少了 `Starting EventSource`**。之后创建 `demo-nowatch`，一行 Reconcile 都没有。

```text
有 workers  ≠  能收到事件
```

## 源码对照

### Complete 只是 Build 的包装

```go
// pkg/builder/controller.go:260
func (blder *TypedBuilder[request]) Complete(r reconcile.TypedReconciler[request]) error {
	_, err := blder.Build(r)
	return err
}
```

### controller.New 会自己注册进 Manager

```go
// pkg/controller/controller.go:210
func NewTyped[request comparable](name string, mgr manager.Manager, options TypedOptions[request]) (TypedController[request], error) {
	c, err := NewTypedUnmanaged(name, mgr, options)
	...
	return c, mgr.Add(c)     // ← 关键
}
```

所以：

```text
controller.New(...)  ==  创建 controller + mgr.Add(controller)
```

**手动写 `controller.New` 时不需要再 `mgr.Add`**，否则会重复注册。

### Watch 和 For 的关系

```go
// 手动
c.Watch(source.Kind(mgr.GetCache(), obj, &handler.EnqueueRequestForObject{}))

// Builder 内部（doWatch）
src := source.TypedKind(blder.mgr.GetCache(), obj, hdler, allPredicates...)
blder.ctrl.Watch(src)
```

`For()` 做的就是第二行的 `source.Kind`，只是还附加了 predicate 和 log constructor。

## 一个泛型坑

v0.25 的 `source.Kind` 是泛型函数：

```go
func Kind[object client.Object](cache, obj object, handler handler.TypedEventHandler[object, reconcile.Request], ...)
```

而 `handler.EnqueueRequestForObject` 的类型是：

```go
type EnqueueRequestForObject = TypedEnqueueRequestForObject[client.Object]
```

所以直接写 `source.Kind(mgr.GetCache(), &corev1.ConfigMap{}, &handler.EnqueueRequestForObject{})`
会编译失败：

```text
type *handler.EnqueueRequestForObject does not match inferred type
handler.TypedEventHandler[*v1.ConfigMap, reconcile.Request]
```

修法：把对象显式声明成 `client.Object`。

```go
obj := client.Object(&corev1.ConfigMap{})
c.Watch(source.Kind(mgr.GetCache(), obj, &handler.EnqueueRequestForObject{}))
```

## 观察清单

| 现象 | 说明 |
|---|---|
| 两种写法行为一致 | Builder 只是语法糖 |
| 手动版没有 `Starting EventSource` 之外的区别 | Watch 就是事件源 |
| Builder 日志多了 kind 字段 | log constructor 由 Build 注入 |
| 不 Watch → 有 workers 无事件 | 控制器空转 |
| `controller.New` 已包含 `mgr.Add` | 不需要手动再 Add |

## 常见问题

| 现象 | 原因 |
|---|---|
| `no matches for kind` | CRD 没装进集群（运行时） |
| `no kind is registered for the type` | Scheme 没注册（客户端） |
| 有 workers 但无 Reconcile | 忘了 `Watch`，或 predicate 全拒 |
| 重复的 worker 日志 | 对同一个 controller 调了两次 `mgr.Add` |

## 下一步

M7：`mgr.Add` 之后，控制器被放进 **哪一组** runnables？

- `LeaderElection` 组（默认）：等缓存同步、等抢锁
- `Others` 组：立即启动

这决定了“为什么控制器总是在缓存之后才启动”。