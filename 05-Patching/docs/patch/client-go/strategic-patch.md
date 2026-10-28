# `types.StrategicMergePatchType` (K8s Native Strategic Merge)

本章节记录 client-go 中 `types.StrategicMergePatchType` 的原理、智能合并机制及使用场景。

---

## 1. 基本信息

| 项目 | 值 |
| :--- | :--- |
| **HTTP 方法** | `PATCH` |
| **Content-Type** | `application/strategic-merge-patch+json` |
| **标准** | Kubernetes 自定义（非 RFC 标准） |
| **client-go 定义** | `k8s.io/apimachinery/pkg/types/types.go` 中 `StrategicMergePatchType PatchType = "application/strategic-merge-patch+json"` |
| **Controller Runtime 对应** | `client.StrategicMergeFrom(original)` |

---

## 2. 为什么需要 StrategicMerge？

### MergePatchType 的核心缺陷

MergePatch 把**数组当成原子整体**，不知道元素有"唯一标识"：

```text
初始: containers = [main-app]

Controller A: {"containers": [main-app, sidecar-logging]}
Controller B: {"containers": [main-app, sidecar-monitoring]}

结果（B 后提交）: containers = [main-app, sidecar-monitoring]  ← sidecar-logging 丢了！
```

### StrategicMerge 的解决方案

StrategicMerge 理解 K8s 资源定义中的 **`+patchStrategy` 标记**，知道哪些数组应该"按 key 合并"而不是"全量替换"。

```text
初始: containers = [main-app]

Controller A: {"containers": [{"name": "sidecar-logging", ...}]}
Controller B: {"containers": [{"name": "sidecar-monitoring", ...}]}

结果: containers = [main-app, sidecar-logging, sidecar-monitoring]  ✅ 都在！
```

**原理**：StrategicMerge 知道 `containers` 数组的 `+patchStrategy=merge` 和 `+patchMergeKey=name` 标记，所以它按 `name` 字段识别元素，智能合并。

---

## 3. K8s 内置的 `+patchStrategy` 标记

K8s 的核心资源类型（Deployment、StatefulSet、DaemonSet 等）在 Go 定义中已经标注了合并策略：

```go
// k8s.io/api/core/v1/types.go

type PodSpec struct {
    // +patchMergeKey=name
    // +patchStrategy=merge
    Containers []Container `json:"containers" ...`

    // +patchMergeKey=name
    // +patchStrategy=merge
    Volumes []Volume `json:"volumes" ...`

    // +patchMergeKey=name
    // +patchStrategy=merge
    InitContainers []Container `json:"initContainers" ...`
}
```

| 标记 | 含义 |
| :--- | :--- |
| `+patchStrategy=merge` | 这个数组应该按元素合并，不是全量替换 |
| `+patchMergeKey=name` | 用元素的 `name` 字段作为唯一标识 |

---

## 4. 如何使用

### 4.1 client-go 原生调用

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

    // 构造 Patch 数据（只需要声明"我要加什么元素"，不需要列出整个数组）
    patchData, _ := json.Marshal(map[string]interface{}{
        "spec": map[string]interface{}{
            "template": map[string]interface{}{
                "spec": map[string]interface{}{
                    "containers": []map[string]interface{}{
                        // 只需要声明新容器，不需要重复写 main-app
                        {"name": "sidecar-logging", "image": "fluentd:latest"},
                    },
                },
            },
        },
    })

    // 发送 StrategicMergePatch 请求
    result, err := clientset.AppsV1().Deployments("default").Patch(
        context.Background(), 
        "my-deploy",
        types.StrategicMergePatchType,   // ← 关键：指定 StrategicMerge
        patchData,
        metav1.PatchOptions{},
    )
}
```

**发送的 HTTP 请求**：

```http
PATCH /apis/apps/v1/namespaces/default/deployments/my-deploy HTTP/1.1
Content-Type: application/strategic-merge-patch+json

{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {"name": "sidecar-logging", "image": "fluentd:latest"}
        ]
      }
    }
  }
}
```

**关键差异**：与 MergePatch 不同，StrategicMerge 的数组里**只需要写新元素**，不需要列出已有的 `main-app`。API Server 会自动按 `name` 合并。

---

### 4.2 Controller Runtime 等价写法

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var deploy appsv1.Deployment
    r.Get(ctx, req.NamespacedName, &deploy)

    // 1. 保存原始副本
    original := deploy.DeepCopy()

    // 2. 修改：追加 sidecar 容器
    sidecar := corev1.Container{
        Name:  "sidecar-logging",
        Image: "fluentd:latest",
    }
    deploy.Spec.Template.Spec.Containers = append(
        deploy.Spec.Template.Spec.Containers, sidecar,
    )

    // 3. 发送 StrategicMergePatch
    err := r.Patch(ctx, &deploy, client.StrategicMergeFrom(original))
    // ↑ 底层等价于: clientset.Patch(ctx, name, types.StrategicMergePatchType, patchData, opts)
}
```

---

## 5. 智能合并原理

### 5.1 合并规则

当 API Server 收到 StrategicMergePatch 时：

```text
1. 读取 K8s 内置的类型定义（如 PodSpec）
2. 检查数组字段的 +patchStrategy 标记
3. 如果 +patchStrategy=merge：
   a. 按 +patchMergeKey（如 name）识别数组元素
   b. 如果元素的 key 已存在 → 更新该元素的字段
   c. 如果元素的 key 不存在 → 追加到数组
4. 如果 +patchStrategy=replace（或无标记）：
   → 全量替换整个数组（同 MergePatch）
```

### 5.2 实际合并示例

**初始状态**：
```yaml
containers:
  - name: main-app
    image: nginx:1.25
```

**Patch 请求**：
```json
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {"name": "sidecar-logging", "image": "fluentd:latest"}
        ]
      }
    }
  }
}
```

**API Server 的处理**：
1. 检查 `containers` 的标记 → `+patchStrategy=merge`, `+patchMergeKey=name`
2. 按 `name` 查找：`sidecar-logging` 不在现有数组中
3. 追加到数组

**最终结果**：
```yaml
containers:
  - name: main-app
    image: nginx:1.25
  - name: sidecar-logging       # ← 智能追加，不影响 main-app
    image: fluentd:latest
```

---

## 6. ✅ 适用场景

| 场景 | 示例 | 为什么适合 |
| :--- | :--- | :--- |
| **给 Deployment 加 sidecar** | `containers: [{name: sidecar, image: ...}]` | 按 name 智能合并 |
| **给 Pod 加 volume** | `volumes: [{name: config, configMap: ...}]` | 按 name 智能合并 |
| **给容器加 env** | `containers: [{name: main-app, env: [...]}]` | env 数组也有 `+patchStrategy=merge` |
| **修改已有容器的字段** | `containers: [{name: main-app, image: nginx:1.26}]` | 按 name 找到元素并更新 |

---

## 7. ❌ 缺陷与限制

### 缺陷 1：仅支持 K8s 内置资源

**问题**：StrategicMerge 的合并逻辑依赖 K8s 类型定义中的 `+patchStrategy` 标记。

**影响**：
- ✅ 对 Deployment、Pod、Service 等内置资源有效
- ❌ 对你自己定义的 **CRD 无效**（CRD 没有 `+patchStrategy` 标记，会退化为全量替换）

**CRD 的替代方案**：使用 `ApplyPatchType` (Server-Side Apply)，它通过 Field Ownership 模型解决并发问题。

---

### 缺陷 2：无法表达"添加但不覆盖"

与 MergePatch 一样，StrategicMerge 没有 "如果存在则跳过" 的语义。如果你 Patch 了一个已存在的字段，它会覆盖。

**替代方案**：使用 `JSONPatchType` 的 `{"op": "add", "path": "/..."}`（已存在时报错）。

---

### 缺陷 3：无法记录字段所有权

与 MergePatch 一样，StrategicMerge 不知道"谁"修改了"哪个"字段。

**替代方案**：使用 `ApplyPatchType` (SSA)。

---

## 8. MergePatch vs StrategicMerge 对比

| 维度 | MergePatchType | StrategicMergePatchType |
| :--- | :--- | :--- |
| **数组处理** | 全量替换（原子操作） | 智能合并（按 `+patchMergeKey`） |
| **Patch 数据量** | 需要列出完整数组 | 只需写新元素 |
| **适用资源** | 所有资源（通用） | 仅 K8s 内置资源（CRD 无效） |
| **标准** | RFC 7396（通用） | K8s 自定义 |
| **Controller Runtime** | `client.MergeFrom(original)` | `client.StrategicMergeFrom(original)` |
| **典型用例** | 修改 annotations、labels | 修改 Deployment containers、volumes |

---

## 9. 决策树：何时该用 StrategicMerge？

```text
你要修改的资源是 K8s 内置类型吗？（Deployment、Pod、Service...）
    │
    ├── 否（CRD） → ❌ 用 ApplyPatchType (SSA)
    │
    └── 是 → 你要修改的是数组字段吗？（containers、volumes、env...）
                │
                ├── 否 → ✅ 用 MergePatchType（更简单）
                │
                └── 是 → ✅ 用 StrategicMergePatchType
```

---

## 10. 总结

| 维度 | StrategicMergePatchType |
| :--- | :--- |
| **HTTP Content-Type** | `application/strategic-merge-patch+json` |
| **合并策略** | 按 `+patchStrategy` 和 `+patchMergeKey` 智能合并 |
| **适合** | 修改 K8s 内置资源的数组字段（containers、volumes、env） |
| **不适合** | CRD（无 `+patchStrategy` 标记）、需要字段所有权 |
| **Controller Runtime** | `client.StrategicMergeFrom(original)` |
| **实验代码** | `cmd/patching-demo-strategic/`（智能合并验证） |
| **与 MergePatch 的关系** | MergePatch 的超集：在简单字段上行为一致，在数组上更智能 |

---

*下一步：如果需要精确操作语义（add-if-not-exists） → [`JSONPatchType`](../json-patch.md)*
*如果需要多 Controller 协作（CRD 也支持） → [`ApplyPatchType/SSA`](../ssa.md)*
