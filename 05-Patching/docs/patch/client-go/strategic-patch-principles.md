

本章节记录 StrategicMergePatchType 的底层实现原理，以及 K8s 内置资源中支持智能合并的常用字段。

---

## 1. 实现原理

### 1.1 核心依赖：Go 结构体标签

StrategicMerge 的合并逻辑完全依赖 **K8s Go 源码中的结构体标签**：

```go
// k8s.io/api/core/v1/types.go

type PodSpec struct {
    // +patchMergeKey=name        ← 按 name 字段识别元素
    // +patchStrategy=merge       ← 这个数组应该智能合并
    Containers []Container `json:"containers" patchStrategy:"merge" patchMergeKey:"name"`
}
```

**关键**：
- 这些标签**不记录在用户编写的 YAML 中**。
- 它们硬编码在 K8s Go 源码里。
- API Server 启动时加载类型定义，运行时读取标签决定合并策略。

---

### 1.2 合并算法流程

当 API Server 收到 `Content-Type: application/strategic-merge-patch+json` 请求时：

```text
1. 解析请求路径，确定资源类型（如 apps/v1/Deployment）
   ↓
2. 查找内存中已注册的 Go 类型定义（appsv1.Deployment）
   ↓
3. 递归遍历 JSON Patch 数据的每个字段
   ↓
4. 如果遇到数组字段：
   a. 检查该字段是否有 +patchStrategy 标签
   b. 如果是 "merge"：
      i.   读取 +patchMergeKey 标签（如 "name"）
      ii.  对 Patch 数据中的每个数组元素，提取 key 值
      iii. 在服务器当前资源中查找匹配 key 的元素
      iv.  如果找到 → 更新该元素（递归合并字段）
      v.   如果没找到 → 追加到数组末尾
   c. 如果是 "replace" 或无标签：
      → 全量替换整个数组
   ↓
5. 返回合并后的完整资源
```

---

### 1.3 合并策略类型

| `+patchStrategy` 值 | 含义 | 示例 |
| :--- | :--- | :--- |
| `merge` | 按 `+patchMergeKey` 智能合并 | `containers`, `volumes`, `env` |
| `replace` | 全量替换数组 | 某些列表字段 |
| （无标签） | 全量替换（默认） | 大多数自定义数组 |

---

## 2. 常用支持 Merge 的字段

### 2.1 Pod/Deployment 核心字段

| 资源路径 | 字段 | Merge Key | 说明 |
| :--- | :--- | :--- | :--- |
| `PodSpec` | `containers` | `name` | 容器列表 |
| | `initContainers` | `name` | 初始化容器 |
| | `volumes` | `name` | 卷列表 |
| | `imagePullSecrets` | `name` | 拉取镜像密钥 |
| | `hostAliases` | `ip` | 主机别名 |
| | `topologySpreadConstraints` | `topologyKey` + `whenUnsatisfiable` | 拓扑分布 |

### 2.2 Container 子字段

| 字段 | Merge Key | 说明 |
| :--- | :--- | :--- |
| `env` | `name` | 环境变量 |
| `volumeMounts` | `mountPath` | 卷挂载点 |
| `ports` | `containerPort` + `protocol` | 容器端口 |
| `volumeDevices` | `devicePath` | 块设备挂载 |
| `envFrom` | `prefixRef` 或 `configMapRef.name` | 环境变量来源 |

### 2.3 Service 字段

| 字段 | Merge Key | 说明 |
| :--- | :--- | :--- |
| `spec.ports` | `port` + `protocol` | 服务端口 |
| `spec.selector` | (map，非数组) | 选择器（map 直接合并） |

### 2.4 StatefulSet/DaemonSet

与 Deployment 相同，因为共享 `PodTemplateSpec` 定义。

---

## 3. 实际合并示例

### 3.1 给 Deployment 加 sidecar

**初始状态**：
```yaml
containers:
  - name: main-app
    image: nginx:1.25
```

**Patch 请求**（只写新容器）：
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

**API Server 处理**：
1. 检查 `containers` 字段标签 → `+patchStrategy=merge`, `+patchMergeKey=name`
2. 按 `name` 查找：`sidecar-logging` 不在现有数组中
3. 追加到数组

**最终结果**：
```yaml
containers:
  - name: main-app
    image: nginx:1.25
  - name: sidecar-logging       # ← 智能追加
    image: fluentd:latest
```

---

### 3.2 修改已有容器的字段

**Patch 请求**：
```json
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "main-app",
            "image": "nginx:1.26"  // 只改 image，不影响其他字段
          }
        ]
      }
    }
  }
}
```

**API Server 处理**：
1. 按 `name=main-app` 找到现有容器
2. 递归合并：更新 `image`，保留其他字段（如 `resources`, `env`）不变

---

## 4. 如何查找更多支持 Merge 的字段？

### 方法 1：搜索 K8s 源码

```bash
# 搜索所有标记了 +patchStrategy=merge 的字段
grep -r "+patchStrategy=merge" $GOPATH/pkg/mod/k8s.io/api@v0.31.3/

# 查找具体字段的 Merge Key
grep -B 2 "+patchMergeKey=" $GOPATH/pkg/mod/k8s.io/api@v0.31.3/core/v1/types.go
```

### 方法 2：查看结构体定义

打开 `k8s.io/api/core/v1/types.go` 或 `k8s.io/api/apps/v1/types.go`，搜索字段名，看上面的注释标签。

---

## 5. 缺陷与限制

### 缺陷 1：仅支持 K8s 内置资源

**问题**：StrategicMerge 的合并逻辑依赖 K8s 类型定义中的 `+patchStrategy` 标签。

**影响**：
- ✅ 对 Deployment、Pod、Service 等内置资源有效
- ❌ 对你自己定义的 **CRD 无效**（CRD 没有 `+patchStrategy` 标签，会退化为全量替换）

**为什么**：API Server 查找的是 Go 源码中的结构体标签，CRD 是动态注册的，没有对应的 Go 类型定义。

---

### 缺陷 2：删除元素需要特殊语法

**问题**：StrategicMerge 不能简单地"不写某个元素"就表示删除它。你"不写"的元素会被认为"不关心"，而不是"要删除"。

**示例**：

```json
{
  "containers": [
    {"name": "main-app", "image": "nginx:1.25"}
  ]
}
```

如果你只想保留 `main-app`，想删除 `sidecar-logging`，**上面这个 Patch 做不到** —— 它只会"更新 main-app"，不会删除其他容器。

**正确做法**：需要 `$patch: delete` 特殊标记：

```json
{
  "containers": [
    {
      "name": "sidecar-logging",
      "$patch: delete"    // ← 告诉 K8s 我要删除这个
    }
  ]
}
```

**问题**：这个语法比较丑，而且**只支持 K8s 内置资源**，CRD 上无效。

---

### 缺陷 3：无法表达"添加但不覆盖"

**问题**：StrategicMerge 没有"如果字段存在则跳过"的语义。如果你 Patch 了一个已存在的元素的字段，它会**覆盖**。

**示例**：

```json
{
  "containers": [
    {"name": "main-app", "image": "nginx:1.26"}
  ]
}
```

如果 `main-app` 的 `image` 已经是 `nginx:1.27`（比你更新的版本还新），StrategicMerge 会**直接覆盖**成 `nginx:1.26`，不会跳过。

**对比 MergePatch**：MergePatch 也是同样的行为，都没有"add-only-if-not-exists"语义。

**替代方案**：使用 `JSONPatchType` 的 `{"op": "add", "path": "/..."}`（已存在时报错，而不是静默覆盖）。

---

### 缺陷 4：Merge Key 硬编码，无法自定义

**问题**：`+patchMergeKey` 是硬编码在 K8s Go 源码里的，用户无法自定义。

**影响**：你无法告诉 K8s"我的这个自定义数组应该按 `id` 字段合并"，除非你去改 K8s 源码。

---

### 缺陷 5：不支持复杂合并逻辑

**问题**：StrategicMerge 只能按**单一 key** 合并，不支持条件合并、多 key 组合等复杂逻辑。

**示例**：如果你想"只在 `env=prod` 时才合并这个容器"，StrategicMerge 做不到。

---

## 6. 为什么要引入其他 PatchType？

| 场景 | StrategicMerge 能做到吗？ | 替代方案 |
| :--- | :--- | :--- |
| **智能合并内置资源数组** | ✅ 按 `+patchStrategy` 合并 | — |
| **给 CRD 加字段** | ❌ 退化为全量替换 | `ApplyPatchType` (SSA) |
| **删除元素** | ⚠️ 需要 `$patch: delete` | `JSONPatchType`（更直观） |
| **"不存在才加，存在跳过"** | ❌ 会静默覆盖 | `JSONPatchType`（`add` 操作已存在时报错） |
| **多 Controller 安全协作** | ⚠️ 仅内置资源有效 | `ApplyPatchType`（字段所有权模型） |

### 下一步学习的两个 PatchType：

**1. JSONPatchType**（`application/json-patch+json`）
- 基于 RFC 6902 标准
- 精确的操作语义：`add`、`remove`、`replace`、`test`
- `add` 操作在路径已存在时**报错**（而不是静默覆盖）
- `remove` 操作更直观（不需要 `$patch: delete`）
- 适合：需要精确控制"添加/删除/替换"语义的场景

**2. ApplyPatchType / Server-Side Apply (SSA)**（`application/apply-patch+yaml`）
- K8s 1.18+ 推荐的生产级方案
- **不依赖 Go 结构体标签**，对 CRD 有效
- 通过 `fieldManager` 记录字段所有权
- 多 Controller 协作时不会互相覆盖
- 支持"声明式"语义：我只声明"我想要什么状态"，API Server 自动处理冲突
- 适合：Operator/Controller 开发，多组件协作管理同一资源

---

*下一步：学习 [JSONPatchType](./json-patch.md)（精确操作语义）或 [ApplyPatchType/SSA](./ssa.md)（CRD 友好的字段所有权）*
