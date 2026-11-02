# 自定义子资源：为什么 CRD 只能有 /status 和 /scale？

本文档解析 Kubernetes 中自定义子资源的限制与替代方案，以及为什么某些操作（如审批、暂停）被设计为子资源模式。

---

## 1. 核心事实：CRD 目前只支持两个子资源

### 1.1 内置资源 vs CRD

| 资源类型 | 支持的子资源 | 能自定义？ |
|---------|-------------|----------|
| **内置资源**（Pod, Deployment 等） | `/status`, `/scale`, `/log`, `/exec`, `/binding`, `/eviction`, `/ephemeralcontainers` | ❌ 不能（K8s 源码硬编码） |
| **CRD（自定义资源）** | **只能** `/status` 和 `/scale` | ❌ **不能自定义**（API Server 限制） |

### 1.2 这意味着什么？

你**不能**给 CRD 添加类似 `/approve`、`/suspend`、`/restart` 这样的自定义子资源端点。

```yaml
# ❌ 这是无效的！CRD 不支持
subresources:
  approve: {}
  suspend: {}
```

API Server 会忽略或报错。CRD 的 `subresources` 字段**只能接受 `status` 和 `scale`**。

---

## 2. 那"自定义子资源"到底在说什么？

当我们说"自定义子资源"时，实际是在讨论**如何用现有的 K8s 原语模拟子资源的行为**。

### 2.1 子资源的本质

子资源 = **一个独立的 API 端点**，对主资源的某一部分进行隔离操作。

```
主资源：/apis/mygroup/v1/namespaces/ns/myresources/name
子资源：/apis/mygroup/v1/namespaces/ns/myresources/name/approve  ← CRD 不能这样做
```

### 2.2 模拟子资源的 4 种方案

| 方案 | 原理 | 适用场景 | 复杂度 |
|------|------|---------|-------|
| **Annotation/Label 信号** | 用 `my-operator.io/approved: "true"` 触发 Controller 逻辑 | 简单审批、状态切换 | ⭐ 低 |
| **独立的审批 CRD** | 创建 `Approval` 资源，关联到主资源 | 复杂审批流、多级审批 | ⭐⭐ 中 |
| **自定义 API Server**（Aggregation Layer） | 写一个独立的 API Server，注册 `/approve` 端点 | 企业级 Operator、多租户 | ⭐⭐⭐ 高 |
| **Webhook 拦截** | 用 ValidatingWebhook 拦截特定修改 | 校验+默认值填充 | ⭐⭐ 中 |

---

## 3. 方案 1：Annotation/Label 信号（最常用）

### 3.1 原理

用户给资源添加一个 Annotation：
```yaml
metadata:
  annotations:
    my-operator.io/approved: "true"
```

Controller 看到这个 Annotation 后，执行审批逻辑。

### 3.2 为什么这等效于子资源？

| 维度 | 真正的 /approve 子资源 | Annotation 方案 |
|------|----------------------|----------------|
| **隔离性** | 只能触发审批，不能改 spec | ✅ Annotation 不影响 spec |
| **RBAC** | 可以单独授权 `update .../approve` | ❌ 不能单独授权 Annotation |
| **审计** | API Server 自动记录 | ✅ etcd 记录变更 |
| **实现成本** | 需要自定义 API Server | ✅ 零成本 |

### 3.3 代码示例

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app MyCRD
    r.Get(ctx, req.NamespacedName, &app)

    // 检查审批信号
    if app.Annotations["my-operator.io/approved"] == "true" && app.Status.Phase != "Approved" {
        // 执行审批逻辑
        app.Status.Phase = "Approved"
        r.Status().Update(ctx, &app)
    }

    return ctrl.Result{}, nil
}
```

---

## 4. 方案 2：独立的审批 CRD（复杂场景）

### 4.1 原理

定义两个 CRD：
1. `MyApp`（主资源）
2. `Approval`（审批资源，通过 OwnerReference 关联到 MyApp）

### 4.2 数据流

```
用户创建 Approval 资源
      ↓
Approval Controller 看到 Approval
      ↓
验证审批条件（比如检查标签、权限）
      ↓
审批通过 → 修改 MyApp 的 Status 或 Annotation
      ↓
MyApp Controller 看到变化 → 执行后续逻辑
```

### 4.3 适用场景

- 多级审批（需要多人签字）
- 审批需要携带额外信息（理由、时间、审批人）
- 审批流需要被审计（独立的 CRD 在 etcd 中有完整记录）

---

## 5. 方案 3：自定义 API Server（企业级）

### 5.1 原理

使用 Kubernetes 的 **Aggregation Layer**，注册你自己的 API Server，提供自定义端点：

```
/apis/mycompany.io/v1/namespaces/ns/myresources/name/approve  ← 你的 API Server 处理
```

### 5.2 架构

```
kube-apiserver
    ↓ (转发)
[Aggregation Layer]
    ↓ (转发)
[Your Custom API Server]  ← 你写的 Go 服务，处理 /approve
```

### 5.3 为什么很少人用？

- 需要维护一个独立的 HTTPS 服务
- 需要配置 `APIService` 注册到 K8s
- 复杂度极高，通常只有大型云厂商或企业级 Operator 使用

---

## 6. 方案 4：Webhook 拦截

### 6.1 原理

用 `ValidatingWebhookConfiguration` 拦截对 CRD 的修改：

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: my-cr-validator
webhooks:
  - name: validate-my-cr.mycompany.io
    rules:
      - operations: ["UPDATE"]
        apiGroups: ["mycompany.io"]
        apiVersions: ["v1"]
        resources: ["myresources"]
    # 当有人修改 myresources 时，调用你的 Webhook 服务
```

### 6.2 适用场景

- 防止用户修改某些字段（如冻结 spec）
- 自动填充默认值（用 `MutatingWebhook`）
- 校验 Status 写入的合法性

---

## 7. 对比：为什么内置资源有那么多的资源有那么多子资源？

### 7.1 Pod 的子资源

| 子资源 | 为什么是子资源？ |
|--------|----------------|
| `/log` | 流式数据，不是对象状态 |
| `/exec` | 远程命令执行，不是对象状态 |
| `/binding` | 调度绑定操作，不是对象状态 |
| `/eviction` | 驱逐操作，不是对象状态 |
| `/status` | 状态汇报，与 spec 隔离 |
| `/ephemeralcontainers` | 动态注入调试容器 |

**共同点：这些操作都不是"修改对象的期望状态（Spec）"，而是执行某个动作或读取动态数据。**

### 7.2 为什么 CRD 不能有这些？

因为 CRD 是**声明式 API**（Declarative API），K8s 的设计哲学是：
- Spec = 用户声明的期望
- Status = Controller 汇报的现实
- 子资源 = 对某一部分的独立操作

**CRD 只暴露了 Spec/Status 的隔离（/status）和扩缩容（/scale），因为这是最通用的两个子资源模式。**

如果你想让 CRD 支持"动作"（比如 approve/restart/suspend），你需要自己实现这个逻辑——通常通过 Annotation 或独立 CRD。

---

## 8. 实际案例：Gateway API 的 /approve 模式

Gateway API 是一个标准的 Kubernetes 项目，它用 **Annotation 方案** 实现"审批"：

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  annotations:
    gateway.networking.k8s.io/gateway-class-allowed: "true"  # ← "审批"信号
```

Gateway Controller 看到这个 Annotation 后，才允许配置 Gateway。

---

## 9. 核心结论

| 问题 | 答案 |
|------|------|
| CRD 能自定义子资源吗？ | ❌ 不能（只能 /status 和 /scale） |
| 为什么？ | API Server 限制（CRD 是声明式 API） |
| 怎么模拟？ | Annotation/Label、独立 CRD、自定义 API Server |
| 推荐方案 | **Annotation 信号**（简单、零成本、99% 场景够用） |
| 什么时候需要自定义 API Server？ | 需要独立 RBAC、审计、复杂业务逻辑时 |

**"自定义子资源"本质是一个模式问题，不是一个技术问题。你用 Annotation 实现的"审批信号"，在逻辑上等同于 `/approve` 子资源。**
