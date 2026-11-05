# 06-manager-anatomy · 拆开 ctrl.NewManager

## 目标

只看 `ctrl.NewManager`，不看 Controller。两个版本做完全相同的四件事：

```text
① 连集群
② 直连读（apiReader）：列出全集群 ConfigMap
③ 缓存读（cachedClient）：列出 test 命名空间 ConfigMap
④ 退出
```

| 目录 | 做法 |
|---|---|
| `with-manager/` | 一句 `ctrl.NewManager(cfg, opts)` |
| `manual/` | 自己建 mapper、cache、两个 client，自己启动缓存 |

## 实测输出

### 版本 A：`ctrl.NewManager`

```text
========== 版本 A：ctrl.NewManager ==========
NewManager 耗时: 450.375µs
GetClient    : *client.client
GetAPIReader : *client.client
GetCache     : *cache.multiNamespaceCache
GetScheme    : *runtime.Scheme
GetRESTMapper: *apiutil.mapper
直连读取 apiReader     : 119 个 ConfigMap（全集群）
缓存读取 cachedClient  : 2 个 ConfigMap（仅 test）

INFO Stopping and waiting for warmup runnables
INFO Stopping and waiting for non leader election runnables
INFO Stopping and waiting for leader election runnables
INFO Stopping and waiting for caches
INFO Stopping and waiting for webhooks
INFO Stopping and waiting for HTTP servers
INFO Wait completed, proceeding to shutdown the manager
```

### 版本 B：手动组装

```text
========== 版本 B：手动组装 ==========
组装耗时: 426.792µs
cachedClient : *client.client
apiReader    : *client.client
cache        : *cache.multiNamespaceCache
scheme       : *runtime.Scheme
mapper       : *apiutil.mapper
直连读取 apiReader     : 119 个 ConfigMap（全集群）
缓存读取 cachedClient  : 2 个 ConfigMap（仅 test）
```

## 第一个发现：类型完全一样

| 能力 | 版本 A | 版本 B |
|---|---|---|
| 缓存读 client | `*client.client` | `*client.client` |
| 直连 reader | `*client.client` | `*client.client` |
| cache | `*cache.multiNamespaceCache` | `*cache.multiNamespaceCache` |
| scheme | `*runtime.Scheme` | `*runtime.Scheme` |
| mapper | `*apiutil.mapper` | `*apiutil.mapper` |

**`NewManager` 没有发明新类型**，它只是把同样的东西打包在一起，再额外挂上生命周期。

## 第二个发现：`NewManager` 不访问 apiserver

```text
NewManager 耗时: 450µs
```

如果它真的去连集群做 discovery，不可能 0.45 毫秒完成。

真实情况：

```text
NewManager:
  · 建 mapper（懒加载，第一次用才请求）
  · 建 cache（只是结构体，informer 还没启动）
  · 建两个 client（只是结构体）
  · 绑定 metrics/healthz/pprof 端口
  → 没有任何网络 IO
```

真正的网络请求发生在**第一次 List / Watch** 时。

> 所以：kubeconfig 指向一个不存在的集群，`NewManager` 也可能成功；
> 到第一次 `List` 才报错。

## 第三个发现：差别在“退出时”最明显

版本 A 打印了 6 行关闭日志：

```text
Stopping and waiting for warmup runnables
Stopping and waiting for non leader election runnables
Stopping and waiting for leader election runnables
Stopping and waiting for caches
Stopping and waiting for webhooks
Stopping and waiting for HTTP servers
```

这 6 组就是 `NewManager` 内部创建的 **runnables 分组**：

| 组 | 装什么 |
|---|---|
| `Warmup` | 预热任务 |
| `Others` | 不需要 leader election 的组件 |
| `LeaderElection` | Controller 等（默认在其中） |
| `Caches` | informer |
| `Webhooks` | webhook server |
| `HTTPServers` | metrics / healthz / pprof |

版本 B 没有这些，因为它根本不认识它们。

## `NewManager` 到底做了哪些事

源码 `pkg/manager/manager.go:341-478`，按顺序：

| # | 动作 | 版本 A | 版本 B |
|---|---|---|---|
| 1 | `setOptionsDefaults` 填默认值 | ✅ 自动 | ❌ |
| 2 | 建 RESTMapper | ✅ | ✅ |
| 3 | 建 Cache | ✅ | ✅ |
| 4 | 建 cachedClient（`client.New` 带 cache） | ✅ | ✅ |
| 5 | 建 apiReader（`client.New` 不带 cache） | ✅ | ✅ |
| 6 | EventRecorder provider | ✅ | ❌ |
| 7 | Leader Election 锁对象 | ✅ | ❌ |
| 8 | Metrics server（绑端口） | ✅ | ❌ |
| 9 | Health probe listener（绑端口） | ✅ | ❌ |
| 10 | Pprof listener（绑端口） | ✅ | ❌ |
| 11 | Webhook server | ✅ | ❌ |
| 12 | 6 组 runnables + errChan | ✅ | ❌ |
| 13 | 优雅关闭字段 | ✅ | ❌ |

版本 B 只覆盖 **2–5**，也就是“数据面”；
版本 A 额外覆盖 **1、6–13**，也就是“生命周期面”。

## 一句话

> **`NewManager` = 数据面（mapper/cache/client×2）+ 生命周期面（runnables 分组、端口、优雅关闭）。**
> 数据面你手写 40 行能复刻；生命周期面才是它真正省事的地方。

## 运行

```bash
cd 02Manager/06-manager-anatomy
go run ./with-manager
go run ./manual
```

对照看两段输出：前 7 行几乎一样，最后 6 行只有版本 A 有。

## 下一步

`NewManager` 之后才是 `NewControllerManagedBy`：
把 Controller 塞进 `LeaderElection` 组，等缓存同步后再启动 worker。