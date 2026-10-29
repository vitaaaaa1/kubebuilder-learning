# `client.RawPatch` — 精确操作 (JSONPatch)

本章节记录 `client.RawPatch` 的原理及在需要"精确控制"时的使用场景。

---

## 1. 核心概念

`client.RawPatch` 是最底层的 Patch 封装，它允许你直接发送预构造的 **JSON Patch (RFC 6902)** 数据。

**为什么叫 "Raw"？**
因为它不关心你的 Go Struct 长什么样，也不自动计算 Diff。它只负责把传进来的 `[]byte` 原封不动地发给 API Server。

---

## 2. JSON Patch (RFC 6902) 的操作

与 MergePatch（描述"最终状态"）不同，JSONPatch 描述的是 **"动作"**。

| 操作 (op) | 含义 | 示例 | 场景 |
| :--- | :--- | :--- | :--- |
| **add** | 添加字段/追加数组 | `{"op": "add", "path": "/data/key", "value": "v1"}` | Upsert（没有则加，有则覆盖） |
| **remove** | 删除字段 | `{"op": "remove", "path": "/metadata/labels/old-key"}` | **精确删除某个 Key** |
| **replace** | 替换字段值 | `{"op": "replace", "path": "/spec/replicas", "value": 3}` | 修改值 |
| **test** | 检查条件 | `{"op": "test", "path": "/metadata/resourceVersion", "value": "100"}` | **悲观锁/前置检查** |
| **move** | 移动位置 | `{"op": "move", "from": "/a", "path": "/b"}` | 字段迁移 |

---

## 3. 代码示例

在 controller-runtime 中，你需要手动构造 JSON 数组（因为一个 Patch 可以包含多个操作）。

```go
// 构造 JSONPatch 数据
patchData := []byte(`[
    {"op": "replace", "path": "/spec/replicas", "value": 5},
    {"op": "add", "path": "/metadata/labels/new-label", "value": "added"}
]`)

// 发送 RawPatch
err := c.Patch(ctx, &deploy, client.RawPatch(types.JSONPatchType, patchData))
```

---

## 4. 什么时候必须用 RawPatch？ (SSA 做不到的场景)

虽然 SSA 是主流，但它有无法触及的角落，这时 RawPatch 是唯一的解法。

### 4.1 精确删除字段

*   **SSA**：如果你想删除一个字段，你必须在 Apply 配置中把它设为 `null` 或者不包含它（取决于具体实现），有时 SSA 会忽略 `null` 值或者行为不符合预期（比如删除数组中的某个元素）。
*   **MergePatch**：你需要发送 `{"metadata": {"labels": {"key": null}}}`。但这需要你构建一个很深的 map 结构。
*   **RawPatch**：最简单！
    ```json
    {"op": "remove", "path": "/metadata/labels/unwanted-key"}
    ```

### 4.2 条件更新 (The "Test" Operation)

这是 RawPatch 的杀手级场景。你想说："**只有当 RV 是 100 的时候，你才帮我改，否则直接报错，不要重试！**"

*   **SSA**：做不到。SSA 总是尝试应用，如果冲突就抛 Conflict。
*   **RawPatch**：
    ```json
    [
        {"op": "test", "path": "/metadata/resourceVersion", "value": "100"},
        {"op": "replace", "path": "/spec/replicas", "value": 5}
    ]
    ```
    如果当前 RV 不是 100，第一步 `test` 失败，整个 Patch 失败，Replicas **绝对不会**被改。这在某些极端的并发控制中非常有用。

### 4.3 按索引操作数组

*   **SSA**：只能识别有 `name` 或 `key` 标记的数组元素（如 Containers）。
*   **RawPatch**：可以直接操作索引！
    ```json
    {"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:latest"}
    ```

---

## 5. 总结：为什么它是 P2？

| 特性 | 使用频率 | 替代方案 |
| :--- | :--- | :--- |
| 精确删除 | 中 | MergePatch (设 null), SSA (移除字段) |
| 条件 Test | 低 | MergeFrom + OptimisticLock |
| 数组索引操作 | 极低 | 通常不建议直接按索引改 K8s 资源 |

**结论：** 95% 的情况下用 SSA 就够了。剩下 5% 需要"原子性精确操作"时，RawPatch 是你的底牌。

---

*上一步：[`Status().Patch()`](./status-patch.md)*
*实验代码：`cmd/controller-runtime/raw-patch-demo/`*