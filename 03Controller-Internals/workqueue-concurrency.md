# WorkQueue 机制专题 (4): 并发处理 (Concurrency)

本章节记录 Controller-WorkQueue 如何**同时处理多个资源请求**，以及如何通过配置控制并发度。

## 1. 核心概念

在 Controller 启动时，Manager 会启动一定数量的 **Worker（工作线程）**。
当 Informer 监听到资源变化并推送到 WorkQueue 后，这些 Worker 会从队列中取出请求并调用 `Reconcile` 函数。

*   **默认行为**：通常，Controller 默认是**单线程**或低并发处理请求的（处理完一个再处理下一个）。
*   **并发处理**：如果队列中有多个待处理请求（例如：同时创建了 10 个 ConfigMap），且开启了并发，多个 Worker 会**并行**运行，大幅提升处理速度。

---

## 2. 如何开启并发

你可以通过在 Controller 的构建链中加入 `.WithOptions()` 来设置 `MaxConcurrentReconciles`。

### 代码示例

```go
ctrl.NewControllerManagedBy(mgr).
    For(&corev1.ConfigMap{}).
    WithOptions(controller.Options{
        // 🌟 允许最多 5 个 Worker 同时运行 Reconcile 函数
        MaxConcurrentReconciles: 5,
    }).
    Complete(&MyReconciler{...})
```

### 效果对比

假设每个 Reconcile 任务需要耗时 **5 秒**，现在我们要处理 **3 个**资源变化：

| 模式 | 配置 | 处理流程 | 总耗时 |
| :--- | :--- | :--- | :--- |
| **串行 (默认)** | `MaxConcurrentReconciles: 1` | Worker 1: 任务 A (5s) -> 任务 B (5s) -> 任务 C (5s) | **约 15 秒** |
| **并行 (并发)** | `MaxConcurrentReconciles: 5` | Worker 1: A (5s)<br>Worker 2: B (5s)<br>Worker 3: C (5s) | **约 5 秒** |

---

## 3. 适用场景

1.  **大规模集群**：当你的 Operator 需要管理成千上万个 CR 实例时，提高并发数可以缩短状态同步的延迟。
2.  **耗时任务**：如果每个 Reconcile 任务都比较慢（例如需要调用外部 API 等待响应），增加并发可以防止 WorkQueue 积压。

---

## 4. ⚠️ 注意事项 (并发安全)

开启并发后，你的 `Reconcile` 函数会被多个 Goroutine 同时调用。请务必注意：

*   **结构体线程安全**：不要在 Reconciler 结构体中直接使用非线程安全的变量（如普通的 `map`），除非你加了锁。
    *   *推荐做法*：使用 K8s 的 Cache (`client.Get`) 获取数据，它是线程安全的。
*   **资源竞争**：如果多个任务都在修改**同一个资源**（例如都在更新同一个 ConfigMap 的 Status），可能会导致 `ResourceVersion` 冲突（Conflict Error）。
    *   *应对*：Controller 的重试机制会自动处理这种冲突并重新尝试，但过多的冲突会降低性能。

---

## 5. 实验验证参考

通过观察日志中的**时间戳**，可以直观地验证并发效果：

*   **串行日志**：`Start A` ... (5s) ... `End A`, `Start B` ...
*   **并行日志**：`Start A`, `Start B`, `Start C` (几乎同一时刻) ... (5s) ... `End A`, `End B`, `End C`
