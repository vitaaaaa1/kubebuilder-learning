# `types.MergePatchType` (RFC 7396 JSON Merge Patch)

本章节记录 client-go 中 `types.MergePatchType` 的完整使用指南、原理及缺陷验证。

---

## 1. 基本信息

| 项目 | 值 |
| :--- | :--- |
| **HTTP 方法** | `PATCH` |
| **Content-Type** | `application/merge-patch+json` |
| **标准** | [RFC 7396 - JSON Merge Patch](https://datatracker.ietf.org/doc/html/rfc7396) |
| **client-go 定义** | `k8s.io/apimachinery/pkg/types/types.go` 中 `MergePatchType PatchType = "application/merge-patch+json"` |
| **Controller Runtime 对应** | `client.MergeFrom(original)` |

---

## 2. 如何使用

### 2.1 client-go 原生调用

```go
package main

import (
    "context"
    "encoding/json"
    
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes"
)

func main() {
    // ... 初始化 clientset ...

    // 1. 构造 Patch 数据（只包含你想修改的字段）
    patchData, _ := json.Marshal(map[string]interface{}{
        "metadata": map[string]interface{}{
            "annotations": map[string]string{
                "my-operator/last-updated": "2026-10-27",
            },
        },
        "data": map[string]string{
            "new-key": "new-value",
        },
    })

    // 2. 发送 Patch 请求
    result, err := clientset.CoreV1().ConfigMaps("default").Patch(
        context.Background(), 
        "my-config",                      // 资源名
        types.MergePatchType,             // ← 指定 Patch 类型
        patchData,                        // JSON 字节流
        metav1.PatchOptions{},
    )
}
```

**发送的 HTTP 请求**：

```http
PATCH /api/v1/namespaces/default/configmaps/my-config HTTP/1.1
Content-Type: application/merge-patch+json

{
  "metadata": {
    "annotations": {
      "my-operator/last-updated": "2026-10-27"
    }
  },
  "data": {
    "new-key": "new-value"
  }
}
```

---

### 2.2 Controller Runtime 等价写法（推荐）

在 Kubebuilder Controller 中，你通常不直接调 client-go，而是用封装后的 API：

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var cm corev1.ConfigMap
    r.Get(ctx, req.NamespacedName, &cm)

    // 1. 保存原始副本（用于三路合并）
    original := cm.DeepCopy()

    // 2. 修改字段
    cm.Annotations["my-operator/last-updated"] = "2026-10-27"
    cm.Data["new-key"] = "new-value"

    // 3. 发送 Patch（只发差异部分）
    err := r.Patch(ctx, &cm, client.MergeFrom(original))
    // ↑ 底层等价于: clientset.Patch(ctx, name, types.MergePatchType, patchData, opts)
}
```

---

## 3. 合并原理：三路合并（Three-Way Merge）

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

**API Server 的合并规则**：

| Original | Server | Modified | 结果 | 解释 |
| :--- | :--- | :--- | :--- | :--- |
| `"key": "v1"` | `"key": "v1"` | `"key": "v2"` | `"key": "v2"` | 你改了，用你的 |
| `"key": "v1"` | `"key": "v3"` | `"key": "v1"` | `"key": "v3"` | 你没改，用服务器的 |
| `"key": "v1"` | `"key": "v3"` | `"key": "v2"` | `"key": "v2"` | 你们都改了，用你的 |
| `"key": "v1"` | — | `"key": null` | — | 你设为 null = 删除 |

---

## 4. ✅ 适用场景

| 场景 | 示例 | 为什么适合 |
| :--- | :--- | :--- |
| **修改 annotations** | `{"metadata": {"annotations": {"key": "val"}}}` | 简单 key-value，不会冲突 |
| **修改 labels** | `{"metadata": {"labels": {"env": "prod"}}}` | 同上 |
| **修改 ConfigMap data** | `{"data": {"config.yaml": "..."}}` | 小范围变更 |
| **修改单个字段** | `{"spec": {"replicas": 3}}` | 不涉及数组 |

---

## 5. ❌ 缺陷与限制

### 缺陷 1：无法智能合并数组（核心缺陷）

**问题**：MergePatch 把数组当成**原子整体**，不知道元素有"唯一标识"。

**实验验证**：`cmd/patching-demo-merge-limitations/main.go`

**场景**：两个 Controller 同时给 Deployment 的 `containers` 数组加元素

```text
初始: containers = [main-app]

Controller A 的 Patch: {"containers": [main-app, sidecar-logging]}
Controller B 的 Patch: {"containers": [main-app, sidecar-monitoring]}

结果（B 后提交）:
  containers = [main-app, sidecar-monitoring]  ← sidecar-logging 丢了！
```

**原因**：MergePatch 看不到 `name` 字段的语义，它只看到"整个数组被替换了"。

**影响**：如果你的资源有数组字段（如 Deployment 的 `.spec.template.spec.containers`、`.spec.volumes`），**绝对不要用 MergePatch 修改它们**。

**替代方案**：使用 `StrategicMergePatchType`，它理解 `+patchStrategy=merge` 标记，能按元素的 `name` 字段智能合并。

---

### 缺陷 2：无法表达"添加但不覆盖"

**问题**：如果字段已存在，MergePatch 会直接覆盖，没有"如果存在则跳过"的语义。

```json
// MergePatch 只能这样：
{"data": {"key": "new-value"}}
// → 如果 key 已存在，直接覆盖，无法阻止
```

**替代方案**：使用 `JSONPatchType` 的 `{"op": "add", "path": "/data/key", "value": "xxx"}`（如果 key 已存在会报错，不会静默覆盖）。

---

### 缺陷 3：删除字段需要用 `null`

**问题**：想删除字段，必须显式设为 `null`，语义不够直观。

```json
// 正确写法（删除 data.old-key）：
{"data": {"old-key": null}}

// 错误写法（表示"我不改 data"）：
{"data": {}}
```

如果你不知道 RFC 7396 规范，很容易写成 `{"data": {}}` 以为会清空 data，但实际上什么都不发生。

---

### 缺陷 4：无法记录字段所有权

**问题**：MergePatch 不知道"谁"修改了"哪个"字段。

**场景**：Controller A 和 Controller B 都管理同一个 ConfigMap 的不同字段
- A 管理 `data.app-config`
- B 管理 `data.monitoring-config`

如果两者同时 Patch，后提交的会覆盖先提交的，因为 K8s 不知道这些字段分别属于谁。

**替代方案**：使用 `ApplyPatchType` (Server-Side Apply)，它通过 `fieldManager` 记录每个字段的 Owner。

---

## 6. 决策树：何时该用 MergePatch？

```text
你要修改的字段是数组吗？
    │
    ├── 是 → ❌ 用 StrategicMergePatchType（智能合并数组）
    │
    └── 否 → 你需要"添加但不覆盖"语义吗？
                │
                ├── 是 → ❌ 用 JSONPatchType（精确操作）
                │
                └── 否 → 有多个 Controller 协作修改同一资源吗？
                            │
                            ├── 是 → ❌ 用 ApplyPatchType (SSA，字段所有权)
                            │
                            └── 否 → ✅ 用 MergePatchType（最简单）
```

---

## 7. 总结

| 维度 | MergePatchType |
| :--- | :--- |
| **HTTP Content-Type** | `application/merge-patch+json` |
| **合并策略** | 三路合并（Original + Server + Modified） |
| **适合** | 修改简单字段（annotations, labels, 单个 scalar 字段） |
| **不适合** | 修改数组（containers, volumes）、需要字段所有权、"add-if-not-exists" |
| **Controller Runtime** | `client.MergeFrom(original)` |
| **默认推荐** | ✅ **最常用，默认选这个**（只要不涉及数组） |
| **实验代码** | `cmd/patching-demo-merge/`（基本用法）、`cmd/patching-demo-merge-limitations/`（数组覆盖缺陷） |

---

*下一步：如果需要智能合并数组 → [`StrategicMergePatchType`](../strategic-patch.md)*
