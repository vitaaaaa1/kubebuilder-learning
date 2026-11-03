# 子资源的 Webhook：防止非法 Status 写入

本文档解析如何通过 Validating/Mutating Webhook 拦截对 CRD 子资源（Status/Scale）的修改，以及为什么这是生产环境的必学技能。

---

## 1. 为什么需要 Webhook 保护子资源？

### 1.1 问题：任何人都能修改 Status

即使用户没有 `update scalableapps` 权限（只有 `update scalableapps/status`），他们仍然可以：

```bash
# 恶意用户
kubectl patch scalableapp my-app --subresource=status --type merge \
  -p '{"status":{"phase":"Hacked","message":"I own your cluster"}}'
```

**问题：**
- Status 可以写入任意值（"Hacked"、"Running" 等）
- Controller 下一次 Reconcile 可能会覆盖，但中间有一段时间 Status 是假的
- 如果其他系统依赖 Status 做决策（比如 HPA 根据 `status.readyReplicas` 计算），可能被误导

### 1.2 Webhook 的作用

```
用户写入 Status
      ↓
API Server 拦截 → 调用你的 Webhook
      ↓
Webhook 验证：Phase 是否合法？Message 是否合规？
      ↓
合法 → 允许写入
非法 → 拒绝（403 Forbidden）
```

---

## 2. ValidatingWebhook vs MutatingWebhook

### 2.1 ValidatingWebhook（校验）

**作用：** 拦截非法写入，返回允许/拒绝。

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: scalableapp-validator
webhooks:
  - name: validate-scalableapp.demo.vitaaaaa.icu
    rules:
      - apiGroups: ["demo.vitaaaaa.icu"]
        apiVersions: ["v1alpha1"]
        resources: ["scalableapps/status"]  # ← 只拦截 /status 写入
        operations: ["UPDATE"]
    clientConfig:
      service:
        name: scalableapp-webhook
        namespace: test
        path: /validate-status
```

### 2.2 MutatingWebhook（修改）

**作用：** 在写入前自动修改/填充字段。

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: scalableapp-mutator
webhooks:
  - name: mutate-scalableapp.demo.vitaaaaa.icu
    rules:
      - apiGroups: ["demo.vitaaaaa.icu"]
        apiVersions: ["v1alpha1"]
        resources: ["scalableapps/status"]
        operations: ["CREATE", "UPDATE"]
    clientConfig:
      service:
        name: scalableapp-webhook
        namespace: test
        path: /mutate-status
```

---

## 3. 实际场景

### 3.1 场景 1：防止非法 Phase 写入

```go
// Webhook 逻辑
func validateStatus(review admissionv1.AdmissionReview) *admissionv1.AdmissionReview {
    var newStatus ScalableAppStatus
    json.Unmarshal(review.Request.Object.Raw, &newStatus)

    // Phase 只能是预定义的值
    allowedPhases := map[string]bool{
        "Pending": true,
        "Running": true,
        "Failed":  true,
        "Scaling": true,
    }

    if !allowedPhases[newStatus.Phase] {
        return deny(fmt.Sprintf("Phase %q is not allowed", newStatus.Phase))
    }

    return allow()
}
```

**效果：**
```bash
# ❌ 被拒绝
kubectl patch scalableapp my-app --subresource=status \
  -p '{"status":{"phase":"Hacked"}}'
# Error: admission webhook denied the request: Phase "Hacked" is not allowed

# ✅ 允许
kubectl patch scalableapp my-app --subresource=status \
  -p '{"status":{"phase":"Running"}}'
```

### 3.2 场景 2：自动填充 LastTransitionTime

```go
// MutatingWebhook 逻辑
func mutateStatus(review admissionv1.AdmissionReview) *admissionv1.AdmissionReview {
    var app ScalableApp
    json.Unmarshal(review.Request.Object.Raw, &app)

    // 如果用户没填 LastTransitionTime，自动填充
    if app.Status.LastTransitionTime.IsZero() {
        app.Status.LastTransitionTime = metav1.Now()
    }

    // 返回修改后的对象
    patchBytes, _ := json.Marshal(app)
    return patch(patchBytes)
}
```

---

## 4. 如何区分 Status vs Spec 的 Webhook？

### 4.1 关键：resources 字段

```yaml
# 拦截 Spec 修改
resources: ["scalableapps"]          # ← 主资源

# 拦截 Status 修改
resources: ["scalableapps/status"]   # ← 子资源
```

### 4.2 实际用途

| Webhook | 拦截什么？ | 典型用途 |
|---------|----------|---------|
| `scalableapps` | Spec 写入 | 防止用户设置非法 replicas（如负数） |
| `scalableapps/status` | Status 写入 | 防止写入非法 Phase、校验 readyReplicas |
| `scalableapps/scale` | Scale 写入 | 校验 replicas 范围（如 1-100） |

---

## 5. 代码架构

### 5.1 Webhook Server 的组成

```
cmd/webhook/
├── main.go              # 启动 HTTPS Server
├── handlers/
│   ├── validate_status.go   # 校验 Status
│   └── mutate_status.go     # 修改 Status
└── config/
    ├── webhook.yaml         # Validating/MutatingWebhookConfiguration
    └── cert/                # TLS 证书（K8s 要求 HTTPS）
```

### 5.2 为什么需要 HTTPS？

API Server **只通过 HTTPS** 调用 Webhook。你需要：
1. 生成自签名证书
2. 将 CA 证书注册到 `WebhookConfiguration.caBundle`
3. Webhook Server 使用该证书启动 HTTPS 服务

---

## 6. 生产环境的最佳实践

### 6.1 必须校验的 Status 字段

| 字段 | 校验规则 | 为什么？ |
|------|---------|---------|
| `phase` | 白名单（Pending/Running/Failed） | 防止任意字符串 |
| `readyReplicas` | >= 0 && <= spec.replicas | 不能超过期望副本数 |
| `selector` | 必须是有效的 Label Selector | 否则 HPA 会崩溃 |
| `message` | 长度限制（如 < 1024 字符） | 防止 etcd 存储过大 |

### 6.2 应该 Mutating 的字段

| 字段 | 默认值 | 为什么？ |
|------|--------|---------|
| `lastTransitionTime` | 当前时间 | 用户不需要手动填 |
| `observedGeneration` | 当前 generation | 方便追踪 |

---

## 8. 集成验证工具（使用 Kubeconfig）

为了在真实集群中验证 Webhook 是否生效，我们编写了一个使用 `kubeconfig` 的验证工具。

### 8.1 验证工具原理

验证工具位于 `cmd/subresource-webhook-verify/`。它使用 `client-go` 模拟恶意请求：

```go
// 1. 加载 kubeconfig
config := clientcmd.BuildConfigFromFlags("", "~/.kube/config")

// 2. 构造恶意 Patch 数据
patchData := []byte(`{"status":{"phase":"HACKED"}}`)

// 3. 发送请求到 /status 子资源
dynamicClient.Resource(gvr).Namespace(ns).Patch(ctx, name, types.MergePatchType, patchData, opts, "status")
```

**预期结果：**
- ✅ **Webhook 正常**：API 返回错误，包含 `"admission webhook denied"`。
- ❌ **Webhook 未运行**：请求成功，非法值 `"HACKED"` 被写入。

### 8.2 如何使用

```bash
# 1. 确保集群中已部署 Webhook（见下方部署指南）
# 2. 确保测试资源存在
kubectl apply -f cmd/scale-with-deployment/crd/sample.yaml

# 3. 运行验证工具
cd cmd/subresource-webhook-verify
go mod tidy
go run main.go
```

**输出示例：**
```text
🛡️  Subresource Webhook 集成验证工具
============================================================
✅ 成功加载 kubeconfig
🔍 检查资源 test/my-scalable-app 是否存在...
✅ 资源存在

⚔️  尝试发送非法 Patch: status.phase = "HACKED"
   (如果 Webhook 正常，这应该被拒绝)

============================================================
🎉 验证成功！Webhook 拦截了非法请求
============================================================
   错误信息: admission webhook "validate-status.scalableapp.demo.vitaaaaa.icu" denied the request: Phase "HACKED" is not allowed.
   结论: 你的 Webhook Server 正在正常工作，非法写入被拒绝。
```

### 8.3 为什么需要这个工具？

虽然你可以手动运行 `kubectl patch ...` 来测试，但这个工具：
1. **自动化**：可以集成到 CI/CD 中。
2. **精确性**：自动解析错误信息，判断是 Webhook 拦截还是其他错误。
3. **教学**：展示了如何在 Go 代码中使用 `kubeconfig` 和 `dynamic client` 操作子资源。

---

## 9. 核心结论

| 问题 | 答案 |
|------|------|
| 为什么需要 Webhook 保护子资源？ | 防止非法 Status 写入，保证数据一致性 |
| 用什么 Webhook？ | ValidatingWebhook（校验）+ MutatingWebhook（填充） |
| 怎么区分 Status vs Spec？ | `resources: ["xxx/status"]` vs `resources: ["xxx"]` |
| 生产必须用吗？ | 建议用（至少对 Phase 做白名单校验） |

**Webhook 是保护子资源的最后一道防线。即使有人绕过了 Controller 和 RBAC，Webhook 仍然能阻止非法数据进入 etcd。**
