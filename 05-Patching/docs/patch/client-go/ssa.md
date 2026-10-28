# `types.ApplyPatchType` (Server-Side Apply, SSA)

本章节记录 client-go 中 `types.ApplyPatchType` 的原理、字段所有权模型及使用场景。

---

## 1. 基本信息

| 项目 | 值 |
| :--- | :--- |
| **HTTP 方法** | `PATCH` |
| **Content-Type** | `application/apply-patch+yaml` |
| **标准** | Kubernetes 自定义（K8s 1.18+ 推荐） |
| **client-go 定义** | `k8s.io/apimachinery/pkg/types/types.go` 中 `ApplyPatchType PatchType = "application/apply-patch+yaml"` |
| **Controller Runtime 对应** | `client.Apply` + `client.ForceOwnership` |

---

## 2. 为什么需要 SSA？

### 前面三种 PatchType 的核心缺陷

| PatchType | 核心缺陷 |
| :--- | :--- |
| **MergePatchType** | 不知道"谁"修改了"哪个"字段，多 Controller 并发会互相覆盖 |
| **StrategicMergePatchType** | 只支持 K8s 内置资源，CRD 无效，也没有字段所有权 |
| **JSONPatchType** | 不知道"谁"修改了"哪个"字段，数组操作需要计算索引 |

### SSA 的解决方案：字段所有权模型

SSA 引入了 **`fieldManager`** 概念 —— 每次 Patch 请求都携带一个 `fieldManager` 标识，API Server 会**记录每个 fieldManager 拥有哪些字段**：

```yaml
# API Server 内部的 managedFields 记录
managedFields:
  - manager: "my-controller"
    fieldsType: FieldsV1
    fields:
      - f:spec:
          f:replicas: {}
          f:template:
            f:spec:
              f:containers:
                k:{"name":"main-app"}: {}
  - manager: "kubectl-edit"
    fields:
      - f:metadata:
          f:annotations:
            f:my-annotation: {}
```

**关键差异**：
- 前面三种 PatchType 是"盲操作" —— API Server 不知道"谁"修改了"哪个"字段
- SSA 记录字段所有权，多 Controller 并发时**只修改自己拥有的字段**，不会互相覆盖
- K8s 1.18+ 推荐的生产级方案

---

## 3. 字段所有权模型

### 3.1 基本概念

| 概念 | 说明 |
| :--- | :--- |
| **fieldManager** | 标识谁在修改资源（如 "my-controller", "kubectl"） |
| **managedFields** | 资源上记录的字段所有权信息 |
| **Field Ownership** | 每个字段只属于一个 fieldManager |
| **Conflict** | 当两个 fieldManager 想修改同一个字段时 |

### 3.2 工作流程

```text
1. Controller A 发送 SSA Patch，fieldManager = "controller-a"
   → API Server 应用变更，记录 "controller-a" 拥有这些字段
   ↓
2. Controller B 发送 SSA Patch，fieldManager = "controller-b"
   → API Server 应用变更，记录 "controller-b" 拥有这些字段
   → 如果 Controller B 想修改 Controller A 拥有的字段？
      → 默认：允许（会转移所有权）
      → ForceOwnership: 强制转移
      ↓
3. 两个 Controller 互不干扰，各自管理自己的字段
```

### 3.3 字段所有权的粒度

字段所有权是**递归**的：

```yaml
# Controller A 拥有整个 containers 数组
f:spec:
  f:template:
    f:spec:
      f:containers:
        k:{"name":"main-app"}: {}

# Controller B 只拥有某个 annotation
f:metadata:
  f:annotations:
    f:my-annotation: {}
```

---

## 4. 如何使用

### 4.1 client-go 原生调用

```go
package main

import (
    "context"

    appsv1 "k8s.io/api/apps/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes"
    "sigs.k8s.io/yaml"
)

func main() {
    // ... 初始化 clientset ...

    // 构造 SSA 的 Patch 数据（YAML 格式）
    patchYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deploy
  annotations:
    owned-by: my-controller
spec:
  replicas: 3
`
    patchData := []byte(patchYAML)

    // 发送 SSA Patch 请求
    result, err := clientset.AppsV1().Deployments("default").Patch(
        context.Background(),
        "my-deploy",
        types.ApplyPatchType,        // ← 关键：指定 ApplyPatch
        patchData,
        metav1.PatchOptions{
            FieldManager: "my-controller",  // ← 关键：指定 fieldManager
        },
    )
}
```

**发送的 HTTP 请求**：

```http
PATCH /apis/apps/v1/namespaces/default/deployments/my-deploy?fieldManager=my-controller HTTP/1.1
Content-Type: application/apply-patch+yaml

apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deploy
  annotations:
    owned-by: my-controller
spec:
  replicas: 3
```

**关键差异**：
- SSA 使用 **YAML 格式**，不是 JSON（虽然 JSON 也兼容）
- 必须指定 `fieldManager` 参数
- Patch 数据是一个**完整的资源声明**（类似 `kubectl apply`），不是操作列表或 diff

### 4.2 Controller Runtime 等价写法

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var deploy appsv1.Deployment
    r.Get(ctx, req.NamespacedName, &deploy)

    // 修改资源
    deploy.Spec.Replicas = ptrInt32(3)
    deploy.Annotations["owned-by"] = "my-controller"

    // 发送 SSA Patch
    err := r.Patch(ctx, &deploy, client.Apply, client.FieldOwner("my-controller"))
    // ↑ 底层等价于: clientset.Patch(ctx, name, types.ApplyPatchType, patchData, opts)
}
```

**Controller Runtime 的 SSA 选项**：

| 选项 | 说明 |
| :--- | :--- |
| `client.Apply` | 启用 SSA |
| `client.FieldOwner("my-controller")` | 指定 fieldManager |
| `client.ForceOwnership` | 强制转移字段所有权（当有冲突时） |

---

## 5. SSA vs kubectl apply

SSA 就是 `kubectl apply` 的底层实现。

```bash
# kubectl apply 底层就是 SSA
kubectl apply -f deploy.yaml

# 等价于
kubectl patch deploy my-deploy --type apply --subresource=spec \
  --field-manager=kubectl \
  -p "$(cat deploy.yaml)"
```

`kubectl apply` 默认使用 `fieldManager: kubectl`。

---

## 6. 字段所有权冲突

### 6.1 冲突场景

当两个 fieldManager 想修改**同一个字段**时：

```yaml
# Controller A 拥有的字段
spec.replicas: 3

# Controller B 也想修改 spec.replicas
```

### 6.2 默认行为

**SSA 默认允许字段所有权转移** —— 后提交者获得该字段的所有权。

这和其他 PatchType 行为一致，但 SSA 记录了**谁曾经拥有过**该字段（通过 `managedFields`）。

### 6.3 ForceOwnership

如果你想**强制转移**字段所有权（比如你的 Controller 应该拥有该字段），使用 `ForceOwnership`：

```go
err := r.Patch(ctx, &deploy, client.Apply,
    client.FieldOwner("my-controller"),
    client.ForceOwnership,  // ← 强制转移所有权
)
```

---

## 7. 适用场景

| 场景 | 示例 | 为什么适合 |
| :--- | :--- | :--- |
| **Operator/Controller 开发** | 多 Controller 协作管理同一资源 | 字段所有权模型，不会互相覆盖 |
| **CRD 的智能合并** | 给 CRD 加 sidecar 容器 | 不依赖 Go 结构体标签，对 CRD 有效 |
| **声明式资源管理** | GitOps（ArgoCD, Flux） | 声明"我想要什么状态"，API Server 自动处理 |
| **多团队共享资源** | 团队 A 管理 spec，团队 B 管理 annotations | 字段隔离，互不干扰 |

---

## 8. ❌ 缺陷与限制

### 缺陷 1：复杂度最高

**问题**：SSA 引入了 fieldManager、managedFields、字段所有权等新概念。

**影响**：
- 需要理解字段所有权模型才能正确使用
- 调试时需要查看 `managedFields` 字段
- 学习曲线比 MergePatch 和 JSONPatch 更陡

### 缺陷 2：ForceOwnership 可能破坏别人的修改

**问题**：使用 `ForceOwnership` 会强制转移字段所有权，可能覆盖别人的修改。

**影响**：需要谨慎使用 `ForceOwnership`，只在你的 Controller 确实应该拥有该字段时使用。

### 缺陷 3：Patch 数据是完整声明

**问题**：SSA 的 Patch 数据是一个完整的资源声明（类似 `kubectl apply`），不是 diff。

**影响**：
- 数据量比 JSONPatch 更大
- 需要构造完整的对象声明，不能只声明"我要改哪个字段"

---

## 9. 四种 PatchType 对比

| 维度 | MergePatch | StrategicMerge | JSONPatch | SSA (Apply) |
| :--- | :--- | :--- | :--- | :--- |
| **标准** | RFC 7396 | K8s 自定义 | RFC 6902 | K8s 自定义 |
| **语义** | 声明式三路合并 | 声明式智能合并 | 命令式操作列表 | 声明式字段所有权 |
| **数组处理** | 全量替换 | 按 key 智能合并 | 按索引精确操作 | 按 key 智能合并 + 字段所有权 |
| **字段所有权** | ❌ | ❌ | ❌ | ✅（`fieldManager` + `managedFields`） |
| **删除** | 需要 `null` | 需要 `$patch: delete` | ✅ `remove` 操作 | 从声明中移除字段 |
| **CRD 支持** | ✅ | ❌ | ✅ | ✅ |
| **多 Controller 安全** | ❌ | ⚠️ 仅内置资源 | ⚠️（操作原子性） | ✅ 最佳 |
| **复杂度** | 低 | 中 | 中 | 高 |
| **K8s 推荐度** | 通用 | 内置资源 | 精确操作 | **生产级首选** |

---

## 10. 决策树：何时该用 SSA？

```text
你是 Operator/Controller 开发吗？
    │
    ├── 是 → ✅ 用 ApplyPatchType (SSA)（生产级首选）
    │
    └── 否 → 你要修改的是 K8s 内置资源的数组字段吗？
                │
                ├── 是 → ✅ 用 StrategicMergePatchType（智能合并）
                │
                └── 否 → 你需要精确的操作语义吗？（add/replace/remove）
                            │
                            ├── 是 → ✅ 用 JSONPatchType
                            │
                            └── 否 → ✅ 用 MergePatchType（最简单）
```

---

## 11. 实验代码结构

每个实验是一个独立的代码目录，一个目录只验证一个概念：

```
05-Patching/cmd/
├── ssa-basic/                 # SSA: 基础用法，字段所有权记录
├── ssa-field-ownership/       # SSA: 多 fieldManager 协作，字段隔离
└── ssa-crd-support/           # SSA: 对 CRD 有效（StrategicMerge 做不到）
```

### 11.1 `ssa-basic/` — SSA 基础用法，字段所有权记录

**验证什么**：SSA Patch 后，资源上的 `managedFields` 字段记录了 fieldManager 和拥有的字段。

**场景**：
1. 创建 Deployment，使用 SSA 添加 annotation
2. 查看 `managedFields`，验证 fieldManager 和字段所有权已记录

**核心结论**：
- SSA 使用 YAML 格式（不是 JSON）
- 必须指定 `fieldManager` 参数
- API Server 自动记录 `managedFields`，显示每个 fieldManager 拥有的字段

---

### 11.2 `ssa-field-ownership/` — 多 fieldManager 协作，字段隔离

**验证什么**：两个不同的 fieldManager 可以同时修改同一资源的不同字段，互不干扰。

**场景**：
1. Controller A（fieldManager = "controller-a"）修改 `spec.replicas`
2. Controller B（fieldManager = "controller-b"）修改 `metadata.annotations`
3. 验证两者修改都在，`managedFields` 显示各自拥有的字段

**核心结论**：
- SSA 的字段所有权模型实现了字段隔离
- 两个 Controller 可以安全协作管理同一资源
- 这是 MergePatch/StrategicMerge/JSONPatch 做不到的

---

### 11.3 `ssa-crd-support/` — SSA 对 CRD 有效

**验证什么**：SSA 对 CRD 有效，而 StrategicMergePatch 对 CRD 无效（退化为全量替换）。

**场景**：
1. 创建 CRD 资源（如自定义 Kind）
2. 使用 SSA Patch 修改 CRD 的字段
3. 验证修改成功，`managedFields` 记录了字段所有权

**核心结论**：
- SSA 不依赖 Go 结构体标签，对 CRD 有效
- StrategicMergePatch 对 CRD 无效（因为没有 `+patchStrategy` 标签）
- SSA 是 CRD 多 Controller 协作的唯一安全方案

---

## 12. 总结

| 维度 | ApplyPatchType (SSA) |
| :--- | :--- |
| **HTTP Content-Type** | `application/apply-patch+yaml` |
| **合并策略** | 声明式字段所有权（`fieldManager` + `managedFields`） |
| **核心优势** | 字段所有权模型，多 Controller 安全协作，CRD 支持 |
| **适合** | Operator/Controller 开发、GitOps、多团队共享资源 |
| **不适合** | 简单场景（复杂度高于其他 PatchType） |
| **Controller Runtime** | `client.Apply` + `client.FieldOwner("my-controller")` |
| **实验代码** | `cmd/ssa-basic/`, `cmd/ssa-field-ownership/`, `cmd/ssa-crd-support/` |

---

*前一篇：[JSONPatchType](./json-patch.md)（精确操作语义）*
