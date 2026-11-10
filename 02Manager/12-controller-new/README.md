# 12-controller-new · 拆开 controller.New

## 这个章节在学什么

一句话：

> **`controller.New` 建的不是“一个函数”，而是“队列 + worker 循环 + 重试策略”；Reconcile 只是被这个循环调用的回调。**

两个版本，除“谁来跑循环”外完全一致：

| 目录 | 循环由谁实现 |
|---|---|
| `with-controller/` | `controller.New` 内部 |
| `manual-worker/` | 手写 `for { Get → Reconcile → Done }` |

## 零、先看最小模型（20 行，不用集群）

`tiny/` 把整章的核心抽了出来：

```bash
go run ./tiny
```

```text
事件源：塞入 test/demo
10:27:18  第 1 次处理 test/demo
          失败 → 重新入队（退避后重试）
10:27:19  第 2 次处理 test/demo      ← 1 秒后
          失败 → 重新入队（退避后重试）
10:27:21  第 3 次处理 test/demo      ← 2 秒后
          成功 → Forget（清掉重试计数）
```

它只有三部分：

```text
① 队列（带退避）
② q.Add(req)                      事件源的动作
③ for { Get → 处理 → Done }       worker 的动作
```

**看懂这 20 行，再看 `with-controller` 和 `manual-worker` 就只是“同一个循环换了个外壳”。**

## 一、`controller.New` 内部做了什么

```text
controller.New(name, mgr, opts)
  └── NewTyped
        ├── DefaultFromConfig(mgr.GetControllerOptions())   继承 Manager 默认值
        ├── NewTypedUnmanaged
        │     ├── 校验：Reconciler 非空 / name 非空 / name 唯一
        │     └── 组装：
        │           Name、Reconciler、RateLimiter、NewQueue、
        │           MaxConcurrentReconciles、CacheSyncTimeout、LogConstructor
        └── mgr.Add(c)        → LeaderElection 组
```

**此时没有队列**：队列是在 `Start` 时才创建的。

```go
queue := c.NewQueue(c.Name, c.RateLimiter)   // Start 里
```

## 二、默认值

| 字段 | 默认 |
|---|---|
| `MaxConcurrentReconciles` | 1 |
| `CacheSyncTimeout` | 2 分钟 |
| `RateLimiter` | 指数退避 5ms → 1000s |
| `SkipNameValidation` | false |

**本实验把退避放大到 1s → 10s**，否则两次重试间隔只有 5ms、10ms，日志上看不出来：

```go
RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
	1*time.Second, 10*time.Second),
```

## 三、worker 循环（源码简化）

```go
for {
	obj, shutdown := c.Queue.Get()      // 阻塞等事件
	if shutdown { return }

	func() {
		defer c.Queue.Done(obj)         // 标记处理完
		c.reconcileHandler(ctx, obj)    // 调 Reconcile，并按结果处理
	}()
}
```

`reconcileHandler` 的结果分支：

| Reconcile 返回 | 队列动作 |
|---|---|
| `err != nil` | `AddRateLimited(req)` → 退避后重试 |
| `RequeueAfter > 0` | `Forget` + `AddAfter(d)` |
| 成功 | `Forget(req)` |

**`Done` 必须调用，`Forget` 决定是否清掉重试计数。**

## 四、实测：两版日志对比

### controller.New

```text
10:17:25 INFO Reconcile 失败（模拟）{"controller":"retry-demo","attempt":1}
10:17:25 ERROR Reconciler error   {"error":"模拟第 1 次失败"}
10:17:26 INFO Reconcile 失败（模拟）{"attempt":2}          ← 1 秒后
10:17:26 ERROR Reconciler error   {"error":"模拟第 2 次失败"}
10:17:28 INFO Reconcile 成功       {"attempt":3}          ← 2 秒后
```

### 手写 worker

```text
10:17:36 INFO Reconcile 失败（模拟）{"assembly":"manual-worker","attempt":1}
10:17:36 ERROR Reconcile 失败，重新入队 {"error":"模拟第 1 次失败"}
10:17:37 INFO Reconcile 失败（模拟）{"attempt":2}          ← 1 秒后
10:17:37 ERROR Reconcile 失败，重新入队 {"error":"模拟第 2 次失败"}
10:17:39 INFO Reconcile 成功       {"attempt":3}          ← 2 秒后
```

**间隔完全一致：1s → 2s（指数退避）。**

## 五、controller 多做了什么

| 能力 | controller.New | 手写 |
|---|---|---|
| 队列命名 / metrics | ✅ | 需自己配 |
| `reconcileID` | ✅ 每次唯一 | ❌ |
| 日志字段 | `controller` / `controllerKind` / 对象引用 | 只有自己加的 |
| panic 恢复 | 可配 `RecoverPanic` | 需自己写 recover |
| 优雅关闭 | `Shutdown signal received` + 等 worker | 需自己处理 |
| 并发 worker | `MaxConcurrentReconciles` | 需自己起 goroutine |

**手写版只保留了最核心的“取-调-放回”，其余都是 controller 的附加值。**

## 六、事件源

本实验不依赖集群对象，用 `source.Func` 直接往队列塞一个 Request：

```go
c.Watch(source.Func(func(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	q.Add(req)
	return nil
}))
```

这再次印证第 11 章的结论：Source 只是“往队列塞 Request 的东西”。

## 七、关键结论

1. `controller.New` 只装配，不启动；队列在 `Start` 时创建。
2. 真正执行 Reconcile 的是 worker 循环，不是 `controller.New`。
3. 重试 = `AddRateLimited`，退避由 RateLimiter 决定（默认 5ms→1000s）。
4. 成功必须 `Forget`，否则重试计数不会清零。
5. `Done` 与 `Forget` 是两件事：前者表示“处理完了”，后者表示“不用再排队”。
6. 手写循环 20 行就能复刻核心行为，但会丢掉指标、panic 恢复、优雅关闭。

## 八、运行

```bash
cd 02Manager/12-controller-new

go run ./with-controller     # 6 秒后自动退出
go run ./manual-worker       # 6 秒后自动退出
```

预期：每个程序 3 次 Reconcile，间隔 1s、2s。

## 九、一句话总结

> **`controller.New` 给 Reconciler 配了一套“队列 + worker + 退避重试”的运行环境；Reconcile 只是被循环调用的函数。**