# 01-controller-basics · 最小可运行的 Controller

## 学习目标

从 `NewControllerManagedBy` 出发，看清一个 Controller 是怎么被装配和启动的。

```text
NewManager              → 造运行时（缓存、client、生命周期）
NewControllerManagedBy  → 造 Builder
  .For(ConfigMap)       → 决定监听什么
  .Complete(reconciler) → 注册进 Manager
mgr.Start(ctx)          → 启动缓存 + 控制器
```

## 文件

| 文件 | 作用 |
|---|---|
| `main.go` | 装配：日志 → Scheme → Manager → Builder → Start |
| `reconciler.go` | 唯一的业务逻辑：打一行日志 |

## 运行

```bash
cd 02Manager/01-controller-basics

# 直接跑（使用 ~/.kube/config）
go run .

# 或跑二进制
make build && ./bin/controller

# 调日志级别
go run . --zap-log-level=debug
```

启动成功的标志：

```text
INFO  setup  Manager 启动：监听 test 命名空间的 ConfigMap
INFO  Starting workers  {"controller": "configmap", "worker count": 1}
```

> 缓存范围由 `Cache.DefaultNamespaces` 限定为 `test`，其它命名空间的事件不会进来。

## 触发一次 Reconcile

另开一个终端：

```bash
# 创建
kubectl create configmap demo-config -n test --from-literal=k=v

# 修改（触发 Update 事件）
kubectl label configmap demo-config -n test demo=1 --overwrite

# 删除（触发 Delete 事件）
kubectl delete configmap demo-config -n test
```

或用 Makefile：

```bash
make demo         # 创建 + 打标签
make clean-demo   # 删除
```

控制台会打印：

```text
INFO  Reconcile 被调用  {"controller": "configmap", ..., "namespace": "test", "name": "demo-config"}
```

## 实测记录（dev-cluster）

一次真实运行（2026-11-04，namespace=`test`）：

```text
16:48:19  INFO  setup                Manager 启动：监听 test 命名空间的 ConfigMap
16:48:19  INFO  Starting EventSource {"controller":"configmap","source":"kind source: *v1.ConfigMap"}
16:48:19  INFO  Starting Controller  {"controller":"configmap"}
16:48:19  INFO  Starting workers     {"controller":"configmap","worker count":1}

16:48:19  INFO  Reconcile 被调用  kube-root-ca.crt   ← 启动瞬间（当时 test 里只有这 1 个 ConfigMap）
16:48:30  INFO  Reconcile 被调用  demo-config        ← kubectl create
16:48:34  INFO  Reconcile 被调用  demo-config        ← kubectl label（Update）
16:48:37  INFO  Reconcile 被调用  demo-config        ← kubectl delete
```

### 三个发现

**① 启动顺序固定，但间隔不固定**

```text
Starting EventSource → Starting Controller → Starting workers
```

这次三行都在同一秒；之前在 `default` 命名空间跑时，`Controller` 比 `EventSource` 晚 **1 秒**。

差在**缓存同步耗时**：对象越多、越大，首次 List 越慢。`test` 里只有 1 个 ConfigMap，几乎瞬间完成。

**② 启动瞬间会对“已有对象”各触发一次 Reconcile**

这次只有 **1 次**，因为当时 `test` 里只有 `kube-root-ca.crt`。

对照：之前在 `default` 命名空间跑，那里有 11 个 ConfigMap，启动瞬间就是 **11 次**。

```text
informer 首次 List → 每个对象都作为 Add 事件投递给 handler → 全部入队
```

**调用次数 = 启动那一刻命名空间里已有的对象数**。

**这不是 bug**：控制器需要“对账”，启动时必须知道所有现存对象。

**③ 每种事件各触发一次**

```text
create → 1 次    update → 1 次    delete → 1 次
```

删掉的对象在 Reconcile 里 `Get` 会得到 `NotFound`，这正是业务代码要处理的情况。

### 一个附带的坑：字段重复

最初的实现里我写的是：

```go
log.Info("Reconcile 被调用", "namespace", req.Namespace, "name", req.Name)
```

实测输出里字段出现了两组：

```json
{"controller":"configmap", "namespace":"test", "name":"demo-config",
 "reconcileID":"...", "namespace":"test", "name":"demo-config"}
```

因为注入的 logger **已经带了** `namespace` / `name` / `ConfigMap` / `reconcileID`。

改成只打消息即可：

```go
log.Info("Reconcile 被调用")
```

## 观察清单

| 现象 | 说明 |
|---|---|
| 启动时先有 `Starting workers` 才有 Reconcile | 缓存同步完成后 worker 才启动 |
| 启动瞬间对每个已有对象调用一次 | informer 首次 List 产生 Add 事件 |
| 已有对象越多，启动那一下的调用越多 | 次数 = 命名空间内对象数 |
| 创建 1 次、修改 1 次、删除 1 次 → 3 次调用 | 每种事件都会入队 |
| 同一对象连续改两次，可能只 reconcile 一次 | workqueue 会去重（同 key 合并） |
| `req` 里只有 namespace + name | 事件被“压缩”成了对象标识 |
| 每次调用 `reconcileID` 不同 | 可用于把一次 Reconcile 的多行日志串起来 |

## 常见问题

| 报错 | 原因 |
|---|---|
| `no configuration has been provided` | 找不到 kubeconfig，设置 `KUBECONFIG` |
| `forbidden: cannot list resource "configmaps"` | 当前 kubeconfig 没有权限 |
| 端口占用 | 已把 metrics 设为 `0`；若改回 `:8080` 注意冲突 |
| 看不到别的命名空间的事件 | 已在 `Cache.DefaultNamespaces` 限制为 `test` |
| `test` 命名空间不存在 | `kubectl create namespace test` |

## 下一步

- `Owns()`：子资源变化也能触发父对象
- `WithEventFilter`：过滤掉不需要的事件
- `MaxConcurrentReconciles`：并发处理