# /status 子资源：状态与规格分离更新

## 1. 什么是子资源（Subresource）？

Kubernetes 中，每个资源（Resource）都有一个 **主资源路径**：
```
/apis/{group}/{version}/namespaces/{namespace}/{resource}/{name}
```

例如：
```
GET /apis/apps/v1/namespaces/default/deployments/my-deploy
```

**子资源** 是主资源下的"子路径"，通过 `/子资源名` 访问：
```
/apis/apps/v1/namespaces/default/deployments/my-deploy/status
/apis/apps/v1/namespaces/default/deployments/my-deploy/scale
```

---

## 2. 为什么需要 /status？

### 2.1 Spec vs Status 的分离

Kubernetes 的 API 设计哲学：

| 字段 | 含义 | 谁修改 |
|------|------|--------|
| `spec` | **期望状态**（用户/Controller 声明想要什么） | 用户、Controller |
| `status` | **实际状态**（Controller 回报现实情况） | 仅 Controller |

**问题场景：** 如果 Controller 用普通 `Update` 更新整个资源：
```go
deploy.Spec.Replicas = 5          // spec 修改
deploy.Status.ReadyReplicas = 3   // status 修改
r.Update(ctx, deploy)             // ❌ 同时改了 spec + status
```

这会带来 3 个问题：

1. **触发不必要的 Watch 事件**：`spec` 修改会触发 Watch，导致其他 Controller 收到事件。
2. **409 Conflict 风险**：如果 `spec` 有其他 Controller 管理的字段（如 ArgoCD），并发修改会冲突。
3. **Reconcile 循环**：自己修改自己的 status 可能触发自己的 Reconcile，形成死循环。

### 2.2 /status 子资源的解决方案

当 CRD 启用了 `/status` 子资源后：

```go
// ✅ 只更新 status，不影响 spec
r.Status().Update(ctx, deploy)

// 或
r.Status().Patch(ctx, deploy, client.MergeFrom(original))
```

**好处：**
- **不触发 Watch**：`/status` 更新不会触发基于该资源的 Watch 事件
- **不冲突**：其他 Controller 修改 `spec`，你修改 `status`，互不干扰
- **RBAC 隔离**：可以单独控制谁能更新 `status`（通常只有 Controller）

---

## 3. 如何启用 /status？

### 3.1 CRD Marker

在 Go 结构体上添加 kubebuilder marker：

```go
// +kubebuilder:subresource:status
type MyResourceStatus struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   MySpec   `json:"spec,omitempty"`
    Status MyStatus `json:"status,omitempty"`
}
```

**⚠️ 关键注意事项（踩坑记录）：**

1. **Marker 必须紧贴 struct**：marker 和 `type` 关键字之间 **不能有空行**
2. **不能有块注释**：`/* ... */` 会干扰 `controller-gen`
3. **运行 make manifests**：只生成本地 YAML，不会更新集群
4. **kubectl apply CRD**：需要手动 apply 到集群

```bash
make manifests
kubectl apply -f config/crd/bases/<your-crd>.yaml
```

### 3.2 验证 CRD YAML

```bash
kubectl get crd myresources.demo.vitaaaaa.icu -o yaml | grep -A 2 subresources
```

应该看到：
```yaml
subresources:
  status: {}
```

---

## 4. /status 的 API 路径

启用后，Kubernetes API Server 会暴露两个端点：

| 端点 | 方法 | 用途 |
|------|------|------|
| `/apis/demo.vitaaaaa.icu/v1alpha1/namespaces/{ns}/myresources/{name}` | `GET/PUT/PATCH` | 操作主资源（spec + metadata） |
| `/apis/demo.vitaaaaa.icu/v1alpha1/namespaces/{ns}/myresources/{name}/status` | `GET/PUT/PATCH` | **仅操作 status** |

---

## 5. Controller Runtime 中的用法

### 5.1 更新 status

```go
// 方式 1: Status().Update()
myResource.Status.Phase = "Running"
myResource.Status.ReadyCount = 5
err := r.Status().Update(ctx, myResource)

// 方式 2: Status().Patch()（推荐）
original := myResource.DeepCopy()
myResource.Status.Phase = "Running"
err := r.Status().Patch(ctx, myResource, client.MergeFrom(original))
```

### 5.2 为什么不推荐 Update？

| 方法 | API 调用 | 需要 RV | 冲突风险 |
|------|---------|---------|---------|
| `Status().Update()` | `PUT .../status` | ✅ 需要 | ✅ 有 |
| `Status().Patch()` | `PATCH .../status` | ❌ 不需要 | ❌ 无 |

**最佳实践：用 Patch 而不是 Update**
```go
// 推荐：Patch 不检查 RV，不会 409
original := obj.DeepCopy()
obj.Status.Phase = "Running"
r.Status().Patch(ctx, obj, client.MergeFrom(original))
```

---

## 6. 实验验证

### 6.1 ConfigMap（无 /status）

ConfigMap 没有 `/status` 子资源，所以 `data` 字段就是"spec"，没有分离。

```go
// 普通 Update 更新整个 ConfigMap
cm.Data["key"] = "value"
r.Update(ctx, cm)  // 更新整个资源

// Patch 更新部分字段
patch := []byte(`{"data":{"key":"value"}}`)
r.Patch(ctx, cm, types.MergePatchType, patch)  // 只更新 data
```

**运行实验：**
```bash
cd cmd/status-subresource
go run main.go
```

### 6.2 CRD（有 /status）

对于启用了 `/status` 的 CRD：

```go
// ✅ 只更新 status，不触发 Reconcile
myCR.Status.Phase = "Running"
r.Status().Update(ctx, myCR)

// ❌ 同时更新 spec + status，会触发 Watch
myCR.Spec.Replicas = 5
myCR.Status.Phase = "Running"
r.Update(ctx, myCR)  // 不推荐
```

---

## 7. 核心结论

| 维度 | 普通 Update | /status 子资源 |
|------|------------|---------------|
| **更新范围** | spec + status 一起 | 仅 status |
| **触发 Watch** | ✅ 会 | ❌ 不会 |
| **RBAC 隔离** | ❌ 不能 | ✅ 可以 |
| **多 Controller 协作** | ❌ 冲突 | ✅ 安全 |
| **适用资源** | 所有资源 | 仅 CRD（需启用） |

**一句话总结：**
> `/status` 子资源是 **状态与规格分离** 的机制，让 Controller 更新 status 时不影响 spec，也不会触发 Reconcile。

---

## 8. 下一步

- [ ] `/scale` 子资源：HPA 集成基础
- [ ] 自定义子资源生成：`+kubebuilder:subresource`
