# `client.MergeFrom` — 三路合并 diff 计算

本章节记录 controller-runtime 中 `client.MergeFrom` 的原理、使用场景及最佳实践。

---

## 1. 基本信息

| 项目 | 值 |
| :--- | :--- |
| **底层 client-go 类型** | `types.MergePatchType`（RFC 7396） |
| **Content-Type** | `application/merge-patch+json` |
| **合并策略** | 三路合并（Original + Server + Modified） |
| **数组处理** | 全量替换（不智能合并） |
| **适用场景** | 修改简单字段（annotations, labels, scalar 字段） |
| **不适用** | 修改数组字段（containers, volumes） |

---

## 2. 为什么需要 MergeFrom？

### client-go 的痛点

在 client-go 中，你必须手动构造 Patch 数据：

```go
// ❌ 容易出错：手动构造 JSON
patchData, _ := json.Marshal(map[string]interface{}{
    "metadata": map[string]interface{}{
        "annotations": map[string]string{
            "my-operator/last-updated": "2026-10-27",
        },
    },
})
clientset.CoreV1().ConfigMaps("default").Patch(
    ctx, "my-config", types.MergePatchType, patchData, metav1.PatchOptions{},
)
```

**问题：**
- 必须知道字段的 JSON 路径（如 `metadata.annotations`）
- 编译器不检查路径正确性
- 容易写错字段名或路径

### controller-runtime 的解决方案

```go
// ✅ 类型安全：修改 Go struct，自动计算 diff
original := cm.DeepCopy()          // 1. 保存原始版本
cm.Annotations["my-operator/last-updated"] = "2026-10-27"  // 2. 修改字段
err := r.Patch(ctx, &cm, client.MergeFrom(original))       // 3. 自动计算 diff
```

**优势：**
- 编译器检查字段名（不会写错路径）
- IDE 提示（自动补全字段）
- 自动计算 diff（只发送变更部分）

---

## 3. 核心机制：diff 是如何计算的？

### 3.1 源代码流程

```go
// 1. MergeFrom(original) 返回一个 Patch 对象
patch := client.MergeFrom(original)
//    内部：mergeFromPatch{from: original, patchType: types.MergePatchType}

// 2. client.Patch(ctx, modifiedObj, patch) 调用 patch.Data(modifiedObj)
data, _ := patch.Data(modifiedObj)
//    内部：
//      originalJSON  = json.Marshal(original)     // 序列化原始版本
//      modifiedJSON  = json.Marshal(modifiedObj)   // 序列化修改后版本
//      data = jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)  // 计算 diff
//    返回：{"metadata":{"annotations":{"key":"value"}}}

// 3. 发送 HTTP PATCH 请求
//    PATCH /api/v1/namespaces/default/configmaps/my-config
//    Content-Type: application/merge-patch+json
//    Body: {"metadata":{"annotations":{"key":"value"}}}
```

### 3.2 三路合并原理

```text
       Original (你 Get 时的版本)
          │
    ┌─────┼─────────┐
    │     │         │
    ▼     │         ▼
Server   │      Modified (你修改后的版本)
(当前)   │
    │     │         │
    └─────┼─────────┘
          │
          ▼
    最终结果 (API Server 计算)
```

**API Server 的合并规则：**

| Original | Server | Modified | 结果 | 解释 |
| :--- | :--- | :--- | :--- | :--- |
| `"key": "v1"` | `"key": "v1"` | `"key": "v2"` | `"key": "v2"` | 你改了，用你的 |
| `"key": "v1"` | `"key": "v3"` | `"key": "v1"` | `"key": "v3"` | 你没改，用服务器的 |
| `"key": "v1"` | `"key": "v3"` | `"key": "v2"` | `"key": "v2"` | 你们都改了，用你的 |
| `"key": "v1"` | — | — | — | 你删了，删掉 |

**关键点：** API Server 比较 Original 和 Server，如果 Server 的某个字段没变（没人改过），就采用 Modified 的值。如果 Server 的字段被别人改了，但你确实改了该字段，你的修改优先。

---

## 4. 如何使用

### 4.1 基本用法

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var cm corev1.ConfigMap
    if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 1. 保存原始副本（用于三路合并）
    original := cm.DeepCopy()

    // 2. 修改字段
    cm.Annotations["my-operator/last-updated"] = "2026-10-27"
    cm.Data["config.yaml"] = "new-config-content"

    // 3. 发送 Patch（只发差异部分）
    err := r.Patch(ctx, &cm, client.MergeFrom(original))
    if err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}
```

### 4.2 发送的 HTTP 请求

```http
PATCH /api/v1/namespaces/default/configmaps/my-config
Content-Type: application/merge-patch+json

{
  "metadata": {
    "annotations": {
      "my-operator/last-updated": "2026-10-27"
    }
  },
  "data": {
    "config.yaml": "new-config-content"
  }
}
```

**关键：** 只发送你修改的字段，不是整个对象。

---

## 5. 与 Update 的对比

### 5.1 Update 的问题

```go
// ❌ Update 发送整个对象
err := r.Update(ctx, &cm)

// HTTP 请求：
// PUT /api/v1/namespaces/default/configmaps/my-config
// Body: {完整对象，包括所有字段}
```

**问题场景：**
```text
你 Get 时：
  annotations: {app.kubernetes.io/version: "1.0", owned-by: "argocd"}
  data: {config.yaml: "..."}

你修改了：
  annotations: {app.kubernetes.io/version: "1.0", owned-by: "argocd", my-key: "my-value"}
  data: {config.yaml: "new-content"}

你 Update 时发送：整个对象（包括 owned-by: argocd）

如果在你 Get 和 Update 之间，ArgoCD 修改了 owned-by → 你的 Update 会覆盖 ArgoCD 的修改
```

### 5.2 MergeFrom 的优势

```go
// ✅ MergeFrom 只发送 diff
original := cm.DeepCopy()
cm.Annotations["my-key"] = "my-value"
cm.Data["config.yaml"] = "new-content"
r.Patch(ctx, &cm, client.MergeFrom(original))

// HTTP 请求：
// PATCH /api/v1/namespaces/default/configmaps/my-config
// Content-Type: application/merge-patch+json
// Body: {"metadata":{"annotations":{"my-key":"my-value"}},"data":{"config.yaml":"new-content"}}
```

**只发送你修改的字段，不影响其他 Controller 管理的字段。**

---

## 6. ✅ 适用场景

| 场景 | 示例 | 为什么适合 |
| :--- | :--- | :--- |
| **修改 annotations** | `cm.Annotations["key"] = "value"` | 简单 key-value，三路合并安全 |
| **修改 labels** | `deploy.Labels["env"] = "prod"` | 同上 |
| **修改 ConfigMap data** | `cm.Data["config.yaml"] = "..."` | 小范围变更 |
| **修改单个 scalar 字段** | `deploy.Spec.Replicas = 3` | 不涉及数组 |
| **多 Controller 协作（简单字段）** | 你改 annotations，别人改 spec | 只发 diff，不覆盖别人 |

---

## 7. ❌ 缺陷与限制

### 缺陷 1：无法智能合并数组（核心缺陷）

**问题**：MergeFrom 把数组当成**原子整体**，不知道元素有"唯一标识"。

```text
初始: containers = [main-app]

你的 Patch: {"containers": [main-app, sidecar-logging]}

结果: containers = [main-app, sidecar-logging]  ← 整个数组被替换

如果另一个 Controller 同时添加了 sidecar-monitoring → 它的提交会覆盖你的 sidecar-logging
```

**原因**：RFC 7396 没有 `name` 字段的语义，它只看到"整个数组被替换了"。

**替代方案**：使用 `client.StrategicMergeFrom(original)`，它理解 `+patchStrategy=merge` 标记，能按元素的 `name` 字段智能合并。

### 缺陷 2：无法表达"添加但不覆盖"

**问题**：如果字段已存在，MergeFrom 会直接覆盖，没有"如果存在则跳过"的语义。

**替代方案**：使用 `client.RawPatch(types.JSONPatchType, data)` 构造 `{"op": "add", ...}` 操作。

### 缺陷 3：无法记录字段所有权

**问题**：MergeFrom 不知道"谁"修改了"哪个"字段。多 Controller 并发修改同一字段时，后提交者覆盖先提交者。

**替代方案**：使用 `client.Apply`（SSA），它通过 `fieldManager` 记录每个字段的 Owner。

---

## 8. ⚠️ 并发缺陷："最后写入者胜出" (Last-Writer-Wins)

### 8.1 核心问题：无 ResourceVersion 检查

标准的 `client.MergeFrom(original)` **不会在 Patch 中携带 `resourceVersion`**。

这意味着 API Server **不检查**提交 Patch 时资源的版本是否陈旧。如果你的 Patch 里包含了某个字段，Server 就会**无条件地覆盖**该字段的当前值。

**后果：** 如果多个控制器并发修改同一个字段，**最后一个到达 Server 的 Patch 会静默覆盖前面的所有更新**。没有 409 Conflict，没有报错，只有数据丢失。

### 8.2 真实案例：Karmada PR #7302

在 Karmada 项目中（[PR #7302](https://github.com/karmada-io/karmada/pull/7302)），曾出现严重的静默更新丢失问题：

- **场景**：三个控制器（Taint Manager、Application Failover Controller、Graceful Eviction Controller）并发写同一个资源的 `Spec.GracefulEvictionTasks` 字段。
- **问题**：使用 `client.MergeFrom(binding)` 进行 Patch。
- **结果**：后提交的 Patch 直接丢弃了先提交的修改。任务被静默丢弃，导致故障转移逻辑失效。

**修复方案**：
Karmada 最终将代码修改为使用 `MergeFromWithOptions` 开启乐观锁：
```go
// ✅ 修复后：强制检查 RV
client.MergeFromWithOptions(binding, client.MergeFromWithOptimisticLock{})
```
这会将 `resourceVersion` 包含在 Patch 中。如果版本不匹配，API Server 会拒绝 Patch 并返回 **409 Conflict**，触发 Controller 重试，从而保证更新不丢失。

### 8.3 对比：SSA 如何解决此问题？

SSA 引入了 **FieldManager（字段管理器）** 概念，彻底改变了并发控制模型：
- 每个 Controller 声明自己"拥有"哪些字段。
- 如果其他 Controller 试图修改不属于它的字段，SSA 会报 **Conflict**（除非使用 `ForceOwnership`）。
- **结论**：SSA 从机制上杜绝了"静默覆盖"，强制多控制器进行协调。

---

## 9. 决策树：何时该用 MergeFrom？

```text
你要修改的字段是数组吗？
    │
    ├── 是 → ❌ 用 StrategicMergeFrom（智能合并数组）
    │
    └── 否 → 你需要"添加但不覆盖"语义吗？
                │
                ├── 是 → ❌ 用 RawPatch + JSONPatch（精确操作）
                │
                └── 否 → 有多个 Controller 协作修改同一资源吗？
                            │
                            ├── 是 → ❌ 用 Apply（SSA，字段所有权）
                            │
                            └── 否 → ✅ 用 MergeFrom（最简单）
```

---

## 9. 与 client-go 的映射关系

| controller-runtime | client-go |
|--------------------|-----------|
| `client.MergeFrom(original)` | `types.MergePatchType` |
| `r.Patch(ctx, obj, patch)` | `clientset.Patch(ctx, name, type, data, opts)` |
| `original.DeepCopy()` | 手动保存原始 JSON |
| 自动 diff 计算 | 手动构造 `json.Marshal(patchData)` |

---

## 10. 总结

| 维度 | `client.MergeFrom` |
| :--- | :--- |
| **底层类型** | `types.MergePatchType`（RFC 7396） |
| **合并策略** | 三路合并（Original + Server + Modified） |
| **适合** | 修改简单字段（annotations, labels, scalar 字段） |
| **不适合** | 修改数组、需要字段所有权、"add-if-not-exists" |
| **最佳实践** | `original := obj.DeepCopy()` → 修改 → `r.Patch(ctx, &obj, client.MergeFrom(original))` |
| **实验代码** | `cmd/controller-runtime/merge-from-demo/` |

---

*下一步：如果需要智能合并数组 → [`StrategicMergeFrom`](./strategic-merge-from.md)*
