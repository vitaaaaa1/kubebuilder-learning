# 08-manager-runnables · Manager 的 6 组 runnables

## 目标

`06-manager-anatomy` 讲的是 **Manager 怎么被造出来**；
这一节讲 **`mgr.Start()` 之后，Manager 怎么安排这些组件**：

```text
mgr.Add(x)  →  x 进哪一组？
mgr.Start() →  组按什么顺序启动？
ctx 取消    →  组按什么顺序停止？
优雅关闭超时 → 会发生什么？
```

## 一、`mgr.Add` 把组件分到 6 组

`pkg/manager/runnable_group.go:84`：

```go
func (r *runnables) Add(fn Runnable) error {
	switch runnable := fn.(type) {
	case *Server:                      // metrics / healthz / pprof
		if runnable.NeedLeaderElection() { return r.LeaderElection.Add(fn, nil) }
		return r.HTTPServers.Add(fn, nil)
	case hasCache:                     // 实现了 GetCache() cache.Cache
		return r.Caches.Add(fn, func(ctx) bool { return runnable.GetCache().WaitForCacheSync(ctx) })
	case webhook.Server:
		return r.Webhooks.Add(fn, nil)
	case warmupRunnable, LeaderElectionRunnable:
		if w, ok := fn.(warmupRunnable); ok {
			r.Warmup.Add(RunnableFunc(w.Warmup), nil)   // 先进 Warmup
		}
		if !ok { return r.LeaderElection.Add(fn, nil) }
		if !runnable.NeedLeaderElection() { return r.Others.Add(fn, nil) }
		return r.LeaderElection.Add(fn, nil)
	default:
		return r.LeaderElection.Add(fn, nil)          // 普通 Runnable 默认
	}
}
```

| 你的组件 | 进入的组 |
|---|---|
| `*metrics.Server` / healthz / pprof | `HTTPServers` |
| 实现了 `GetCache() cache.Cache` | `Caches` |
| `webhook.Server` | `Webhooks` |
| 实现了 `Warmup(ctx) error` | `Warmup` |
| `NeedLeaderElection() == false` | `Others` |
| **Controller**（普通 `Runnable`） | `LeaderElection`（默认分支） |

> 注意：Controller 没有 `GetCache()`，所以不会进 `Caches`，
> 而是按默认分支进 `LeaderElection`——这就是“reconciler 属于 leader election 组”的原因。

## 二、启动顺序（源码 `internal.go:437-500`）

```text
① HTTPServers.Start     metrics / healthz / pprof
② Webhooks.Start        转换/校验/默认值 webhook
③ Caches.Start          informer 启动，并阻塞等待同步完成
④ Others.Start          不依赖 leader 的组件（缓存已同步）
⑤ Warmup.Start          预热，执行完才继续
⑥ LeaderElection        抢到锁（或未启用）后才启动 Controller
```

源码注释给出的两个硬性理由：

- HTTPServers 必须在缓存之前：healthz 起来之前，conversion webhook 可能连不上
- Webhooks 必须在缓存之前：否则 webhook 与缓存同步互相等待

## 三、实测启动顺序

`go run .`（运行 6 秒后退出）：

```text
========== mgr.Start ==========
15:54:47  START   {"group": "caches组"}
15:54:47  START   {"group": "others组"}
15:54:47  WARMUP  {"group": "warmup组/leader组"}
15:54:47  START   {"group": "leader组"}
15:54:47  START   {"group": "warmup组/leader组"}
```

对应关系：

| 日志 | 组 |
|---|---|
| `caches组` | Caches（实现了 `GetCache()`） |
| `others组` | Others（`NeedLeaderElection()==false`） |
| `WARMUP` | Warmup |
| `leader组` | LeaderElection（普通 Runnable） |
| `warmup组/leader组` | LeaderElection（同一对象也实现了 Warmup） |

**顺序完全符合：Caches → Others → Warmup → LeaderElection。**

`leader组` 和 `warmup组/leader组` 同组，先后由 goroutine 调度决定，不保证固定顺序。

## 四、停止顺序（源码 `engageStopProcedure`）

```text
① Warmup            （并行 goroutine，避免死锁）
② Others            "non leader election runnables"
③ LeaderElection    "leader election runnables"
④ Caches            ← 在 reconciler 之后，避免收到新事件
⑤ Webhooks
⑥ HTTPServers
```

实测：

```text
15:54:53  Stopping and waiting for non leader election runnables
15:54:53  Stopping and waiting for warmup runnables
15:54:53  STOP  {"group": "others组"}
15:54:53  Stopping and waiting for leader election runnables
15:54:53  STOP  {"group": "warmup组/leader组"}
15:54:53  STOP  {"group": "leader组"}
15:54:53  Stopping and waiting for caches
15:54:53  STOP  {"group": "caches组"}
15:54:53  Stopping and waiting for webhooks
15:54:53  Stopping and waiting for HTTP servers
15:54:53  Wait completed, proceeding to shutdown the manager
========== mgr.Start 正常返回 ==========
```

**停止顺序 = 启动顺序的逆序。**

关键设计：

> Caches 在 LeaderElection 之后停止，
> 是为了防止 reconciler 停止后缓存还在推送事件、继续入队。

## 五、优雅关闭超时实验

```bash
go run . -hold 3s -slow 5s -grace 2s
```

某个组件收到停止信号后要 5 秒才退出，但 `GracefulShutdownTimeout` 只有 2 秒：

```text
15:55:08  Stopping and waiting for leader election runnables
15:55:08  停止中，故意变慢  {"group": "warmup组/leader组", "耗时": "5s"}
15:55:10  Stopping and waiting for caches
========== mgr.Start 返回错误:
failed waiting for all runnables to end within grace period of 2s: context deadline exceeded ==========
```

**2 秒一到，Manager 不再等，直接返回错误。**

| `GracefulShutdownTimeout` | 行为 |
|---|---|
| `> 0`（默认 30s） | 最多等这么久 |
| `0` | 立刻超时（不等待） |
| `< 0` | 永远等下去 |

## 六、各组典型成员

| 组 | 典型成员 | 何时启动 |
|---|---|---|
| `HTTPServers` | metrics、healthz、pprof | 最先 |
| `Webhooks` | admission / conversion webhook | 第二 |
| `Caches` | informer | 第三（阻塞到同步） |
| `Others` | 你的自定义非 leader 组件 | 第四 |
| `Warmup` | 预热任务 | 第五（阻塞到完成） |
| `LeaderElection` | **Controller**、需要选主的组件 | 最后 |

## 七、和前一节的关系

```text
06-manager-anatomy   造 Manager（mapper / cache / client / 端口 / runnables 容器）
08-manager-runnables 用 Manager（Add 分组 → Start 顺序 → Stop 顺序）
```

`06` 里看到的 6 行关闭日志，就是这一节讲的 6 个组。

## 八、运行

```bash
cd 02Manager/08-manager-runnables

go run .                              # 正常启停
go run . -hold 10s                    # 跑 10 秒
go run . -slow 5s -grace 2s           # 触发优雅关闭超时

# 或
make run
make slow
```

## 九、一句话总结

> **`mgr.Add` 按接口把组件分进 6 组；`Start` 按 HTTPServers → Webhooks → Caches → Others → Warmup → LeaderElection 启动，停止时严格逆序，超时则由 `GracefulShutdownTimeout` 强制中断。**