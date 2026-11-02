# 子资源的 RBAC 隔离：为什么 Operator 只需要 update status 权限

本文档解析 Kubernetes 如何通过 RBAC（Role-Based Access Control）隔离子资源的访问权限，以及为什么这是生产环境安全的最佳实践。

---

## 1. 核心概念：主资源 vs 子资源的 RBAC

### 1.1 传统 RBAC（不隔离）

如果你只有以下权限：
```yaml
- apiGroups: ["demo.vitaaaaa.icu"]
  resources: ["scalableapps"]
  verbs: ["get", "list", "watch", "update"]
```

你可以：
- ✅ 修改 `spec`（包括 replicas）
- ✅ 修改 `status`
- ✅ 修改 `metadata`

**问题：** Operator 有权限修改用户的 `spec`！如果 Operator 代码有 bug，可能意外覆盖用户的配置。

### 1.2 RBAC 隔离（推荐）

当 CRD 启用 `/status` 子资源后，RBAC 可以分开授权：

```yaml
# Operator 权限（最小权限原则）
- apiGroups: ["demo.vitaaaaa.icu"]
  resources: ["scalableapps"]          # 只能读主资源
  verbs: ["get", "list", "watch"]

- apiGroups: ["demo.vitaaaaa.icu"]
  resources: ["scalableapps/status"]   # 只能写 status
  verbs: ["update", "patch"]
```

Operator 现在：
- ✅ 能读取 `spec`（知道用户想要什么）
- ✅ 能写入 `status`（汇报实际状态）
- ❌ **不能修改 `spec`**（防止覆盖用户配置）

---

## 2. 为什么需要隔离？

### 2.1 安全原则：最小权限

Operator 的职责是**同步期望到现实**，不是**修改用户的期望**。

| 角色 | 应该能做什么？ | 不应该做什么？ |
|------|--------------|--------------|
| **用户/管理员** | 修改 `spec`（定义期望状态） | - |
| **Operator** | 同步 `status`（汇报现实状态） | 修改 `spec` |
| **HPA** | 通过 `/scale` 修改 `spec.replicas` | 修改其他 spec 字段 |

### 2.2 防止意外覆盖

**场景：用户修改 spec，同时 Operator 也在更新**

```
时间线：
T1: 用户 edit ScalableApp → 修改 spec.image = "v2"
T2: Operator 看到 spec.replicas 变了 → 更新 status + 不小心把 spec.image 写回 "v1"
T3: 用户的修改被覆盖！❌
```

**如果有 RBAC 隔离：**
- Operator 根本没有 `update scalableapps` 权限
- 即使 Operator 代码写错，API Server 也会返回 403 Forbidden ✅

---

## 3. 子资源的 RBAC 权限矩阵

### 3.1 完整的权限分解

| 子资源 | RBAC 资源名 | 谁能访问？ | 典型权限 |
|--------|-----------|----------|---------|
| **主资源** | `scalableapps` | 用户/GitOps | `get, list, watch, update, patch, create, delete` |
| **/status** | `scalableapps/status` | Operator | `get, update, patch`（或 `watch`） |
| **/scale** | `scalableapps/scale` | HPA/kubectl | `get, update, patch` |

### 3.2 如何验证权限隔离？

```bash
# 测试：Operator ServiceAccount 能否修改 spec？
kubectl auth can-i update scalableapps --as=system:serviceaccount:test:my-operator
# 应该返回：no

# 测试：Operator ServiceAccount 能否更新 status？
kubectl auth can-i update scalableapps/status --as=system:serviceaccount:test:my-operator
# 应该返回：yes
```

---

## 4. 代码层面的影响

### 4.1 没有 RBAC 隔离的代码

```go
// ❌ 危险：Operator 有权限修改整个对象
r.Update(ctx, &app)  // 包含 spec + status
```

### 4.2 有 RBAC 隔离的代码

```go
// ✅ 安全：Operator 只能更新 status
r.Status().Update(ctx, &app)      // 或
r.Status().Patch(ctx, patchBase, client.MergeFrom(&app))
```

**如果你用了 `r.Update()`，API Server 会返回 403：**
```
forbidden: User "system:serviceaccount:test:my-operator" cannot update resource "scalableapps" in the namespace "test"
```

---

## 5. Kubebuilder 如何生成 RBAC 配置？

### 5.1 Marker 写法

在 Controller 文件顶部添加：

```go
// +kubebuilder:rbac:groups=demo.vitaaaaa.icu,resources=scalableapps,verbs=get;list;watch
// +kubebuilder:rbac:groups=demo.vitaaaaa.icu,resources=scalableapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.vitaaaaa.icu,resources=scalableapps/scale,verbs=get;update;patch
```

### 5.2 生成的 YAML

运行 `make manifests` 后，`config/rbac/role.yaml` 会生成：

```yaml
- apiGroups: ["demo.vitaaaaa.icu"]
  resources: ["scalableapps"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["demo.vitaaaaa.icu"]
  resources: ["scalableapps/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["demo.vitaaaaa.icu"]
  resources: ["scalableapps/scale"]
  verbs: ["get", "update", "patch"]
```

---

## 6. 实验：验证 RBAC 隔离

### 6.1 创建有限权限的 ServiceAccount

我们创建一个只能更新 `status`，不能修改 `spec` 的 Role：

```yaml
# rbac/limited-operator-role.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: limited-operator
rules:
  - apiGroups: ["demo.vitaaaaa.icu"]
    resources: ["scalableapps"]
    verbs: ["get", "list", "watch"]      # ← 只能读
  - apiGroups: ["demo.vitaaaaa.icu"]
    resources: ["scalableapps/status"]   # ← 能写 status
    verbs: ["update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: limited-operator-binding
subjects:
  - kind: ServiceAccount
    name: my-operator
    namespace: test
roleRef:
  kind: Role
  name: limited-operator
  apiGroup: rbac.authorization.k8s.io
```

### 6.2 验证步骤

```bash
# 1. 验证权限
kubectl auth can-i update scalableapps --as=system:serviceaccount:test:my-operator
# 输出：no

kubectl auth can-i update scalableapps/status --as=system:serviceaccount:test:my-operator
# 输出：yes

# 2. 尝试用有限权限修改 spec（应该 403）
kubectl patch scalableapp my-app --type merge -p '{"spec":{"replicas":5}}' \
  --as=system:serviceaccount:test:my-operator
# 输出：Error from server (Forbidden): ...

# 3. 尝试用有限权限更新 status（应该成功）
kubectl patch scalableapp my-app --subresource=status --type merge -p '{"status":{"phase":"Running"}}' \
  --as=system:serviceaccount:test:my-operator
# 输出：scalableapp.demo.vitaaaaa.icu/my-app patched
```

---

## 7. 生产环境的最佳实践

### 7.1 Operator 最小权限清单

```yaml
# 生产环境 Operator 权限（推荐）
- scalableapps:          ["get", "list", "watch"]      # 读用户配置
- scalableapps/status:   ["get", "update", "patch"]    # 写状态
- scalableapps/finalizers: ["update"]                  # 处理删除清理
- deployments:           ["create", "get", "update"]   # 管理工作负载
- pods:                  ["get", "list"]               # 观察 Pod 状态
```

### 7.2 不需要给 Operator 的权限

```yaml
# ❌ 不要给 Operator 这些权限
- scalableapps: ["create", "update", "delete", "patch"]  # 用户的东西
- scalableapps/scale: ["update"]                         # HPA 的
- events: ["create"]                                     # 除非你要发事件
```

---

## 8. 常见问题

### Q1: 如果 Operator 意外获得了 update spec 权限，会怎样？

**后果：** 如果 Operator 代码有 bug（比如 DeepCopy 后忘记清除 spec 修改），它会覆盖用户的配置，且没有告警。

**如何预防：** 
1. 生产环境必须启用 RBAC 隔离
2. 使用 `r.Status().Update()` 而不是 `r.Update()`
3. Webhook 验证 spec 不被 Operator 修改

### Q2: 我的 Operator 能直接 `r.Update()` 吗？

**技术上：** 可以（如果它有权限）。
**最佳实践：** 不应该。用 `r.Status().Update()` 或 `r.Status().Patch()`。

### Q3: `/scale` 子资源需要单独的 RBAC 吗？

**是的。** `scalableapps/scale` 是独立的资源，需要单独的 RBAC 授权。

---

## 9. 核心结论

| 维度 | 不隔离 | 隔离 |
|------|--------|------|
| **Operator 权限** | `update scalableapps` | `update scalableapps/status` |
| **安全性** | 低（可覆盖用户 spec） | 高（只能写 status） |
| **生产推荐** | ❌ | ✅ |
| **代码写法** | `r.Update(ctx, &app)` | `r.Status().Update(ctx, &app)` |

**RBAC 隔离是 Kubernetes "关注点分离" 的终极体现：用户定义期望，Operator 汇报现实，两者通过权限隔离，互不干扰。**
