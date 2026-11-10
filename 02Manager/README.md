# 02 Manager: 运行机制与配置实验

本专题记录了从零开始搭建 Controller 时，`Manager` 组件的核心作用及常用配置实验。

## 1. Manager 的核心职责

Manager 是 `controller-runtime` 的心脏，它不仅仅是启动 Controller 的工具，它实际上维护了以下关键基础设施：

1.  **集群连接**：通过 `ctrl.GetConfigOrDie()` 智能获取 kubeconfig（本地环境变量 -> 默认文件 -> 集群 ServiceAccount）。
2.  **Shared Informer (本地缓存)**：**核心性能优化点**。Manager 会把集群数据同步到本地内存，Controller 的读操作（`r.Get`）直接读缓存，极大减轻 API Server 压力。
3.  **领导选举 (Leader Election)**：防止多副本同时运行导致逻辑冲突。
4.  **生命周期管理**：统一管理 Webhook、Healthz、Metrics 以及所有 Controller 的启动和优雅关闭。

## 2. 核心配置实验 (`ctrl.Options`)

我们在 `cmd/bare-metal-controller/main.go` 中进行了以下关键配置的实验：

### 2.1 缓存范围限制 (Cache Scoping)
**配置项**：`Cache.DefaultNamespaces`

*   **现象**：设置为 `map[string]cache.Config{"test": {}}` 后，Controller 只能感知 `test` 命名空间下的资源变化。
*   **重要结论**：**缓存层过滤 > 事件过滤器 (Predicate)**。
    *   如果在 Manager 层面限制了 Namespace，资源根本不会进入本地内存，节省大量 RAM 和网络带宽。
    *   相比之下，使用 `WithEventFilter` 是先把所有资源都缓存下来，再在代码里丢弃，浪费了资源。

### 2.2 领导选举 (Leader Election)
**配置项**：`LeaderElection`, `LeaderElectionID`, `LeaderElectionNamespace`

*   **现象**：开启后，日志会出现 `Attempting to acquire leader lease`。只有抢到锁的 Pod 才会执行 `mgr.Start` 中的 Controller 逻辑。
*   **本地调试注意**：在本地运行（非集群内）必须显式指定 `LeaderElectionNamespace`（如 `default`），否则程序不知道把锁资源（Lease）建在哪里。

### 2.3 健康与就绪探针 (Health & Readiness)
**配置项**：`HealthProbeBindAddress`, `AddHealthzCheck`, `AddReadyzCheck`

*   **配置步骤**：
    1.  在 Options 中绑定端口（如 `:8081`）。
    2.  **必须**显式调用 `mgr.AddHealthzCheck` 和 `mgr.AddReadyzCheck` 注册检查逻辑。
*   **区别**：
    *   **`/healthz` (Liveness)**：检查进程是否活着。挂了由 K8s 重启。
    *   **`/readyz` (Readiness)**：检查缓存是否同步完成（Informer Synced）。没同步完不对外服务，防止 K8s 把流量转过来。

### 2.4 优雅关闭 (Graceful Shutdown)
**配置项**：`GracefulShutdownTimeout`

*   **作用**：按下 `Ctrl+C` 后，Manager 不会立即退出，而是等待正在运行的 `Reconcile` 完成（最长等待设定时间，如 10s），保证数据一致性。

## 3. Manager 启动流程 (`mgr.Start`)

当我们调用 `mgr.Start(ctx)` 时，内部按顺序发生了这些动作：

1.  **抢锁**：如果是多副本，尝试获取 Leader Election Lease。
2.  **启动缓存**：启动 Informers，开始 List/Watch API Server，将数据拉取到本地内存。
3.  **就绪检查**：缓存同步完成后，`/readyz` 接口返回 200 OK。
4.  **启动 Controller**：通知所有 Controller 开始处理 WorkQueue 中的事件。
5.  **阻塞**：进程进入阻塞状态，直到收到 `ctx.Done()` 信号。

## 4. 深入阅读：拆解实验

上面是结论，下面这些目录是“自己动手复刻一遍”的实验：

*   Manager 到 Client 的拆解：[`docs/newmanager-to-client.md`](docs/newmanager-to-client.md)
*   Controller 装配的拆解：[`docs/newcontroller-managed-by.md`](docs/newcontroller-managed-by.md)

| 目录 | 内容 | 对照 |
|---|---|---|
| `06-manager-anatomy/` | 拆开 `ctrl.NewManager` | `with-manager/`（一句 NewManager） vs `manual/`（手写 mapper/cache/client） |
| `07-client-anatomy/` | 拆开 `client.New` | `with-client/`（client.New） vs `manual/`（直接 `rest.RESTClient`） |
| `08-manager-runnables/` | Manager 的 6 组 runnables | 启动/停止顺序、优雅关闭超时 |
| `09-controller-builder/` | 拆开 `NewControllerManagedBy` | `with-builder/`（一行装配） vs `manual/`（五步手写） |
| `10-multi-controller/` | 一个 Manager 挂多个 Controller | ConfigMap 与 Secret 各自独立 |
| `11-controller-watch/` | 拆开 `c.Watch(src)` | `with-source/`（KindSource） vs `custom-source/`（自定义 Source） |
| `12-controller-new/` | 拆开 `controller.New`：队列 + worker + 重试 | `with-controller/`（内置循环） vs `manual-worker/`（手写循环） |
| `13-conflict-retry/` | 并发写冲突与 `RetryOnConflict` | `naive/`（冲突即失败） vs `with-retry/`（重读重写） |
| `14-reconcile-dispatch/` | Reconcile 调度参数 | `-mode quiet/after/error/panic/parallel` |
| `05-build-and-add/compare/split-manager/` | 不含 Manager 的最小可读链路 | informer 回调 + 缓存启动 |

实测结论速览：

*   `NewManager` 完成耗时 ~450µs，**不发任何网络请求**，唯一副作用是绑定 metrics/healthz/pprof 端口。
*   `apiReader` 读到 119 个 ConfigMap（全集群），`cachedClient` 读到 2 个（仅 `test`）。
*   `cachedClient` 在缓存未启动时读会报 `the cache is not started`，但写操作（直连）成功。
*   `WaitForCacheSync` 只等“已注册”的 informer；未注册的类型在首次访问时才创建并阻塞同步。

```bash
cd 06-manager-anatomy && make both     # NewManager vs 手动组装
cd 07-client-anatomy  && make both     # client.New vs rest.RESTClient
```
