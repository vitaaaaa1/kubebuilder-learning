# WorkQueue 机制专题 (2): 自动重试与退避策略 (Retry & Backoff)

本章节记录 Controller 在 `Reconcile` 报错后，WorkQueue 如何进行**自动重试**以及**自定义重试策略**。

## 1. 核心概念

当 `Reconcile` 返回 `error` 时，WorkQueue 不会丢弃这个请求，而是将其标记为失败，并根据 **RateLimiter（限速器）** 的计算结果，延迟一段时间后再次将请求放回队列。

### 关键机制：
*   **无限重试**：只要返回 `error`，WorkQueue 就会一直重试，直到成功（返回 `nil` error）。
*   **指数退避 (Exponential Backoff)**：随着失败次数增加，重试间隔会越来越长，防止压垮系统。
*   **无上限但有最大延迟**：重试次数无上限，但等待时间有上限（默认 1000 秒）。

---

## 2. 默认重试策略 (`DefaultControllerRateLimiter`)

如果你不手动配置，Kubebuilder 默认使用一个非常聪明的**组合策略**。

### 默认时间轴
假设 Reconcile 一直报错，你会看到这样的重试节奏：

1.  **前 5 次失败**：每次等待 **10 秒**。
2.  **第 5 次之后**：每次等待 **10 分钟** 🔥（直到你修复它）。

### 为什么这样设计？
*   **前 10 秒 (自愈窗口)**：给系统一个机会。也许只是网络抖动，10 秒后网络恢复了就能成功。
*   **后 10 分钟 (冷宫模式)**：如果重试 5 次还不行，说明问题比较严重（比如依赖的 Service 没创建）。此时降低频率，防止 Controller 疯狂打印错误日志 (Log Spam) 浪费 CPU。

---

## 3. 自定义重试策略

虽然默认策略很棒，但在开发调试时，你可能希望它**重试得更快**以便观察日志；在生产中，你可能希望**限制最大并发**。

你可以通过 `WithOptions` 覆盖默认的限速器。

### 代码示例：使用自定义指数退避

我们需要使用泛型版本 `NewTypedItemExponentialFailureRateLimiter`：

```go
import (
    "time"
    "k8s.io/client-go/util/workqueue"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/controller"
)

// ...

ctrl.NewControllerManagedBy(mgr).
    For(&corev1.ConfigMap{}).
    // 🌟 在这里配置 WorkQueue 的重试策略
    WithOptions(controller.Options{
        RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](
            1 * time.Second,  // BaseDelay: 基础延迟，第一次失败后等 1 秒
            60 * time.Second, // MaxDelay: 最大延迟，最长等待不超过 1 分钟
        ),
    }).
    Complete(&MyReconciler{Client: mgr.GetClient()})
```

**效果**：
1.  **第 1 次报错**
2.  *(等待 **1 秒**)* -> **第 2 次报错**
3.  *(等待 **2 秒**)* -> **第 3 次报错**
4.  *(等待 **4 秒**)* -> **第 4 次报错**
5.  ...直到达到最大限制 **60 秒**。

---

## 4. 如何设置“错误重试上限”？

**注意：WorkQueue 本身没有“重试 N 次后放弃”的配置。** 它的逻辑是“死磕到底”。

如果你想实现“失败 3 次后就停止重试”，必须在 **Go 代码逻辑中** 自己实现判断，然后**吞掉 Error**。

```go
func (r *MyReconciler) Reconcile(...) (ctrl.Result, error) {
    err := doSomething()

    if err != nil {
        // 判断重试次数 (需自行维护计数器，例如在内存 Map 或 CR 的 Annotation 中)
        if retryCount > 3 {
            logger.Error(err, "Failed 3 times, giving up!")
            // ✅ 关键：返回 nil。告诉 WorkQueue“我处理完了（虽然失败了），别再重试了”。
            return ctrl.Result{}, nil
        }
        
        // ❌ 返回 error，WorkQueue 会继续重试
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```
