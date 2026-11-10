# 14-reconcile-dispatch · Reconcile 的调度参数怎么用

> **队列、worker、重试、退避，controller-runtime 都已经实现好了。**
> **你只需要决定两件事：Reconcile 返回什么、控制器配什么参数。**

每个情况一个独立目录，`main.go` 只差“返回什么”这一处。

## 目录

| 目录 | 演示什么 | Reconcile 的关键代码 |
|---|---|---|
| `01-result-empty/` | 处理完就结束 | `return ctrl.Result{}, nil` |
| `02-requeue-after/` | 过 2 秒再来 | `return ctrl.Result{RequeueAfter: 2*time.Second}, nil` |
| `03-error-backoff/` | 自动退避重试 | `return ctrl.Result{}, err` |
| `04-panic-recover/` | panic 被恢复 | `panic("...")` |
| `05-concurrent-workers/` | 3 个 worker 并行 | `MaxConcurrentReconciles: 3` |
| `06-sync-period/` | 周期重放 | `SyncPeriod: 2*time.Second` |

`shared/common.go` 只做三件事：建 Manager、确保演示对象存在、装配控制器。**六个 main 的差异全部集中在 Reconcile 里。**

## 一、四种返回

```go
return ctrl.Result{}, nil                             // ① 完成
return ctrl.Result{RequeueAfter: 2 * time.Second}, nil // ② 过 2 秒再来
return ctrl.Result{}, fmt.Errorf("临时失败")           // ③ 失败，自动退避
panic("...")                                          // ④ 被恢复，按失败处理
```

| 返回 | 队列行为 | 场景 |
|---|---|---|
| `{}` | 结束，不排队 | 正常处理完 |
| `RequeueAfter: d` | `d` 后重新入队 | 等外部资源就绪、轮询 |
| `error` | 指数退避重试 | 临时失败 |
| panic | 恢复后按 error 重试 | 程序 bug |

**`err != nil` 时 `RequeueAfter` 会被忽略。**

## 二、实测

### 01 · 空 Result

```text
demo-dispatch at=14:32:28.304     ← 启动时一次，之后不再触发
```

### 02 · RequeueAfter

```text
demo-dispatch at=14:32:35.028
demo-dispatch at=14:32:37.029     ← +2.001s
demo-dispatch at=14:32:39.030     ← +2.001s
```

### 03 · 返回 error

```text
demo-dispatch at=14:32:43.683
demo-dispatch at=14:32:43.689     ← +6ms
demo-dispatch at=14:32:43.700     ← +11ms
demo-dispatch at=14:32:43.722     ← +22ms
demo-dispatch at=14:32:43.763     ← +41ms
```

**间隔逐次翻倍**（默认 5ms 起，上限 1000s），你一行重试代码都不用写。

### 04 · panic

```text
即将 panic
Observed a panic: 模拟 panic
Reconciler error: panic: 模拟 panic [recovered]
即将 panic                    ← 进程没死
Observed a panic: 模拟 panic
Reconciler error: panic: 模拟 panic [recovered]
即将 panic
```

前提是 `RecoverPanic: true`（本实验显式设置）。

### 05 · 3 个 worker

```text
开始 demo-a at=14:32:51.310
开始 demo-b at=14:32:51.310
开始 demo-c at=14:32:51.310     ← 三个同时
完成 demo-a at=14:32:53.313
完成 demo-b at=14:32:53.313
完成 demo-c at=14:32:53.313
```

**同一把 key 永远不会被两个 worker 同时处理**（workqueue 的保证）。

### 06 · SyncPeriod

```text
demo-dispatch at=14:32:57.741     ← 启动
demo-dispatch at=14:33:01.907     ← +4.17s（首次约 2 倍周期）
demo-dispatch at=14:33:04.064     ← +2.16s
```

对象没有任何变化，Reconcile 也被周期性调用。

| 项 | 说明 |
|---|---|
| 默认值 | 10 小时 |
| 作用 | 兜底纠偏：watch 漏事件时也能恢复一致 |
| 成本 | 只把**缓存里的对象重新喂一遍**，不发起 List |
| 抖动 | ±10%，避免所有控制器同时触发 |

## 三、常用参数

### 控制器级（`controller.Options`）

| 参数 | 默认 | 用途 |
|---|---|---|
| `MaxConcurrentReconciles` | 1 | worker 数量 |
| `RateLimiter` | 5ms→1000s | 自定义重试节奏 |
| `RecoverPanic` | true | panic 是否恢复 |
| `CacheSyncTimeout` | 2 分钟 | 等缓存同步上限 |

### 缓存级（`cache.Options`）

| 参数 | 默认 | 用途 |
|---|---|---|
| `SyncPeriod` | 10h | 周期重放缓存对象 |
| `DefaultNamespaces` | 全部 | 只缓存指定命名空间 |
| `ByObject[].Label` | 无 | 只缓存带某标签的对象 |

## 四、怎么选

```text
处理成功、无需再管       → Result{}, nil
过一会儿再检查           → RequeueAfter
临时错误（网络/冲突）    → return err
需要并行处理多个对象     → MaxConcurrentReconciles: N
怕漏事件、需要定期纠偏   → SyncPeriod（默认 10h 通常够）
```

## 五、运行

```bash
cd 02Manager/14-reconcile-dispatch

make 01    # 空 Result
make 02    # RequeueAfter
make 03    # error 退避
make 04    # panic 恢复
make 05    # 3 个 worker
make 06    # SyncPeriod

make clean # 删除 demo 对象
```

每个程序都会自动创建 `test/demo-*` 对象并打上 `demo=dispatch` 标签；控制器只处理带该标签的对象，`demo-config`、`kube-root-ca.crt` 不会被触发。

## 六、一句话总结

> **你只负责“返回什么”；排队、重试、退避、并发、周期重放，controller-runtime 全包了。**