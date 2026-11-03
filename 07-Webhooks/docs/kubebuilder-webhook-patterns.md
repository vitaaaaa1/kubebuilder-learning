# Kubebuilder Webhook 模式：从原理到实现

本文档解析 Kubebuilder 如何生成和管理 Webhooks，以及 Operator 开发者应该如何编写 Webhook 逻辑。

---

## 1. 为什么 Operator 需要 Webhook？

在 Operator 模式中，Controller 负责**调和状态（Reconcile）**，而 Webhook 负责**准入控制（Admission Control）**。

| 角色 | 触发时机 | 作用 | 类比 |
|------|---------|------|------|
| **Controller** | 资源变化时（异步） | 确保现实符合期望 | **执行者**（发现灯泡坏了就修） |
| **Webhook** | 写入 etcd 之前（同步） | 确保数据合法/完整 | **安检员**（不让危险品进场） |

**没有 Webhook 的 Operator 就像"没有安检的机场"——用户可以把脏数据写进 CRD，导致 Controller 崩溃或进入死循环。**

---

## 2. Kubebuilder 的 Webhook 生成流

### 2.1 核心工作流

```text
1. 编写代码 (webhook.go)
   包含 Marker:
   // +kubebuilder:webhook:path=/mutate-..., ...
   // +kubebuilder:webhook:path=/validate-..., ...

         ↓

2. make manifests (controller-gen)
   解析 Marker → 生成 config/webhook/manifests.yaml
   (包含 Validating/MutatingWebhookConfiguration)

         ↓

3. SetupWithManager
   在 main.go 中注册 Webhook Server
   mgr.GetWebhookServer().Register(...)

         ↓

4. 部署时 (kustomize/certmanager)
   自动生成 TLS 证书
   将 CA Bundle 注入 manifests.yaml
```

---

## 3. 两种 Webhook 类型

### 3.1 Mutating Webhook (准入修改)

**目的：** 在资源存入 etcd 之前，修改或填充字段。

**典型场景：**
- 自动填充默认值（如 `image: nginx:latest` 如果用户没写）。
- 注入 Sidecar 容器（Istio 的核心原理）。
- 添加标签（如 `app.kubernetes.io/managed-by: my-operator`）。

**Kubebuilder 实现：**
实现 `Defaulter` 接口（`Default()` 方法）。

### 3.2 Validating Webhook (准入校验)

**目的：** 拒绝非法的资源写入。

**典型场景：**
- `spec.replicas` 不能为负数。
- `spec.image` 必须符合特定格式。
- `status.phase` 只能是白名单内的值。

**Kubebuilder 实现：**
实现 `Validator` 接口（`ValidateCreate()`, `ValidateUpdate()`, `ValidateDelete()`）。

---

## 4. Kubebuilder 代码结构解析

### 4.1 Types 定义 (api/v1alpha1/xxx_types.go)

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:webhook:path=/vmutate-demo,mutating=true,failurePolicy=fail,groups="demo.vitaaaaa.icu",resources=demoapps,verbs=create;update,versions=v1,name=mdemoapp.demo.vitaaaaa.icu
type DemoApp struct { ... }
```

**注意：** `// +kubebuilder:webhook` 标记告诉 controller-gen 生成对应的 Webhook 配置。

### 4.2 Webhook 逻辑 (webhooks/xxx_webhook.go)

Kubebuilder 会为每个类型生成两个 Webhook 对象（如果需要）：

```go
// 修改逻辑
func (r *DemoApp) Default() {
    demoapplog.Info("Default", "name", r.Name)
    if r.Spec.Image == "" {
        r.Spec.Image = "nginx:latest" // 自动填充
    }
}

// 校验逻辑
func (r *DemoApp) ValidateCreate() (admission.Warnings, error) {
    demoapplog.Info("ValidateCreate", "name", r.Name)
    if r.Spec.Replicas < 0 {
        return nil, fmt.Errorf("replicas cannot be negative")
    }
    return nil, nil
}
```

### 4.3 注册入口 (main.go)

```go
if os.Getenv("ENABLE_WEBHOOKS") != "false" {
    if err = (&webhooks.DemoApp{}).SetupWebhookWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create webhook", "webhook", "DemoApp")
        os.Exit(1)
    }
}
```

---

## 5. 证书管理（HTTPS）

Kubernetes 要求 Webhook 必须使用 **HTTPS**。Kubebuilder 通过以下方式解决：

1.  **本地开发 (`make run`)**：
    *   使用 `apiserver.local.config.certs` 脚本生成临时证书。
    *   或者使用 `localhost` 和不验证证书的配置。
2.  **集群部署 (`make deploy`)**：
    *   通常配合 **Cert-Manager** 自动签发证书。
    *   或者使用 `kustomize` 的 patch 功能将 CA Bundle 写入配置。

---

## 6. 最佳实践

1.  **Fail-Closed 原则**：`failurePolicy: Fail`。如果 Webhook 挂了，拒绝写入。这比让非法数据进入系统更安全。
2.  **超时设置**：Webhook 响应必须快（建议 < 2s）。慢速 Webhook 会阻塞整个集群的创建/更新操作。
3.  **避免循环**：Mutating Webhook 修改对象后，可能会再次触发 Webhook，需小心处理。
4.  **轻量化**：不要在 Webhook 中执行重型操作（如调用外部 API），它会阻塞用户的 `kubectl apply`。

---

## 7. 总结

| 特性 | Mutating Webhook | Validating Webhook |
|------|-----------------|-------------------|
| **时机** | 写入前 | 写入前 |
| **能力** | 修改对象 | 仅允许/拒绝 |
| **接口** | `Defaulter` | `Validator` |
| **失败后果** | 对象保持原样或使用默认值 | 写入被拒绝 |

**Kubebuilder 让编写 Webhook 变得像编写 Controller 一样简单：你只需关注业务逻辑（Default/Validate），它帮你搞定 HTTPS Server 和 K8s 配置生成。**
