# `client.StrategicMergeFrom` — 数组智能合并

本章节记录 K8s 原生的 Strategic Merge Patch (SMP) 机制，以及它如何处理复杂的数组字段。

---

## 1. 核心痛点：为什么 `MergeFrom` 搞不定数组？

在标准的 RFC 7396 (MergePatch) 中，**数组被视为“原子整体”**。

**场景复现：**
```yaml
# 初始状态
containers:
  - name: app
    image: nginx
```

如果你用 `MergeFrom` 尝试注入一个 Sidecar：
```json
{"spec": {"containers": [{"name": "sidecar", "image": "fluentd"}]}}
```

**MergeFrom 的结果：** 整个数组被**替换**了！
```yaml
# 结果：app 容器丢了 ❌
containers:
  - name: sidecar
    image: fluentd
```

**原因：** MergeFrom 无法识别数组元素的身份，它只看到“你给了一个新数组”。

---

## 2. StrategicMerge 的秘密：K8s 的“上帝视角”

StrategicMerge 之所以“聪明”，是因为它**认识**数组里的元素。

K8s 源码中的字段带有特殊的注释标记（Markers）：

```go
// K8s 源码 (apps/v1/types.go)
type PodSpec struct {
    // +patchMergeKey=name
    // +patchStrategy=merge
    Containers []Container `json:"containers" ...`
}
```

**这两个标记告诉 API Server：**
1.  **`+patchStrategy=merge`**: 这个数组不要傻替换，要按元素合并。
2.  **`+patchMergeKey=name`**: 每个容器的 `name` 字段就是它的“身份证号”。

---

## 3. 智能合并图解

**场景：** 使用 StrategicMerge 注入 Sidecar。

**你的 Patch：**
```json
{
  "spec": {
    "containers": [
      { "name": "sidecar", "image": "fluentd" }
    ]
  }
}
```

**API Server 处理流程：**
1.  看到 Patch 里有个 `name: sidecar`。
2.  去旧列表里找：有没有叫 `sidecar` 的？
    *   **没有** → 这是一个新元素，**追加 (Append)** 到列表。
    *   **有** → 这是一个旧元素，**更新 (Update)** 它内部的字段。
3.  保留列表中其他未被触碰的元素（如 `app`）。

**最终结果：**
```yaml
containers:
  - name: app
    image: nginx        # ✅ 毫发无损
  - name: sidecar       # ✅ 成功注入
    image: fluentd
```

---

## 4. 与 SSA 的深层联系

你现在使用的 **SSA (Server-Side Apply)**，底层依然沿用了这套逻辑，但做得更好。

*   **StrategicMerge**: 能够智能合并数组，但**不记录字段所有权**（并发修改依然可能冲突）。
*   **SSA**: 同样使用 Key（在 CRD 中是 `+listType=map`）来识别数组元素，并且**记录哪个 Manager 添加了哪个元素**。

**结论：** StrategicMerge 是 SSA 能够处理复杂数组（如 Sidecar 注入）的**底层基础**。

---

## 5. 总结：三种 Patch 对待数组的态度

| 策略 | 对待数组的行为 | 适用场景 |
| :--- | :--- | :--- |
| **MergeFrom** | **替换 (Replace)**: 把旧数组扔掉，换上新的 | 更新 ConfigMap 的 Data |
| **StrategicMerge** | **合并 (Merge)**: 按 Key 匹配，追加或更新 | 注入 Sidecar，添加 Tolerations |
| **SSA** | **隔离 (Isolate)**: 按 Key 匹配，并记录谁改了它 | Operator 管理 Pod 模板 |

---

*实验代码：无（SSA 已包含此逻辑）*
*上一步：[`client.Apply (SSA)`](./ssa.md)*
