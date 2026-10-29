# `client.Status().Patch()` — 安全更新状态

本章节记录 controller-runtime 中如何通过 `/status` 子资源安全、高效地更新资源状态。

---

## 1. 核心问题：为什么不能用 `r.Update()` 更新状态？

很多初学者在 Reconcile 循环中习惯这样写：
```go
obj.Status.Phase = "Running"
r.Update(ctx, obj) // ❌ 危险！
```

**存在的三大风险：**

1.  **误伤 Spec**：`Update` 发送的是**整个对象**。如果你在 Get 之后，ArgoCD 修改了 `metadata.annotations`，你的 `Update` 会把 ArgoCD 的修改**覆盖掉**。
2.  **409 Conflict (资源版本冲突)**：状态字段（如 Deployment 的 `Replicas` 或 HPA 的 `CurrentMetrics`）变化非常快。
    *   你 Get 时 RV=100。
    *   HPA Controller 更新了状态，RV=101。
    *   你的 `Update` 带着 RV=100 提交 → **报错 409 Conflict**。
3.  **触发 Reconcile 循环**：`Update` 会触发 Watch 事件。如果你更新了状态，你的 Controller 可能会收到这个事件，导致**无限自我循环**（Reconcile → Update Status → Trigger Watch → Reconcile...）。

---

## 2. `Status().Patch()` 的解决方案

使用 `/status` 子资源进行 Patch 更新，完美解决了上述所有问题。

### 2.1 API 分离：Spec 与 Status 解耦

Kubernetes API Server 有两个独立的端点：
*   `/apis/.../namespaces/{ns}/{resource}/{name}` (主资源：管理 Spec)
*   `/apis/.../namespaces/{ns}/{resource}/{name}/status` (子资源：管理 Status)

当你调用 `r.Status().Patch(...)` 时，请求发往 `/status` 端点。
**API Server 保证：** 即使你的 Payload 里包含了 Spec 的修改，服务器也会**忽略它**，只应用 Status 部分。

### 2.2 避免 409 Conflict

与 `MergeFrom` 类似，`Status().Patch()` **不强制检查 `resourceVersion`**（除非显式开启 OptimisticLock）。

它采用的是 **Merge Patch** 语义：
*   你提交：`{"status": {"phase": "Running"}}`
*   服务器：把 `phase` 字段合并进去。
*   即使 HPA 刚刚更新了 `currentMetrics`，你的 Patch 依然会成功，因为你没有动那个字段。

---

## 3. 代码示例：如何安全地更新状态

### 标准范式

```go
func (r *MyReconciler) updateStatus(ctx context.Context, obj *appsv1.Deployment) error {
    // 1. 保存修改前的状态副本
    // 注意：这里必须 DeepCopy，否则 Patch 计算出的 diff 为空
    original := obj.DeepCopy()

    // 2. 修改状态字段
    obj.Status.ReadyReplicas = 5
    obj.Status.Phase = "Running"

    // 3. 使用 MergeFrom 计算 diff，并通过 Status() 发送
    // 这将发送类似 {"status": {"readyReplicas": 5, "phase": "Running"}} 的 Patch
    return r.Status().Patch(ctx, obj, client.MergeFrom(original))
}
```

### 为什么需要 `DeepCopy`？

我们在之前 `MergeFrom` 章节学过，`client.MergeFrom(original)` 需要对比 `original` 和 `obj` 的差异来计算 Patch 数据。
*   如果 `original` 只是引用（浅拷贝），那么 `original` 和 `obj` 指向同一个内存。
*   你修改了 `obj.Status`，`original.Status` 也会变。
*   结果：Diff 为空，服务器什么都没更新。

---

## 4. `Status().Patch()` vs `Status().Update()`

| 特性 | `Status().Update()` | `Status().Patch()` (推荐) |
| :--- | :--- | :--- |
| **发送数据** | 整个 Status 对象 (PUT) | 只有变化的字段 (PATCH) |
| **ResourceVersion** | **必须匹配** (否则 409) | **不强制检查** (更安全) |
| **并发安全** | 差 (容易被 HPA 等组件冲突) | **好** (只合并变动) |
| **性能** | 较低 (数据量大) | **高** (只传 Diff) |

**结论：在 Reconcile 循环中，永远优先使用 `Status().Patch()`，除非你需要原子性地替换整个状态对象。**

---

## 5. 总结

使用 `Status().Patch()` 是 Operator 开发的**最佳实践**：
1.  **不干扰 Spec**：由 `/status` 端点保证。
2.  **不触发 Watch**：状态更新通常不会触发 Spec 变化的 Watch。
3.  **高可用性**：在高频变化的资源（如 HPA, Deployment）上，极大降低 409 报错率。

---

*上一步：[`client.Apply (SSA)`](./ssa.md)*
