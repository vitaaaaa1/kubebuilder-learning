# Kubebuilder 开发实践：Marker 与 /status 子资源

本文档记录如何在 Kubebuilder 项目中启用和使用 `/status` 子资源。

---

## 1. 启用 /status 子资源

### 1.1 Go 结构体 Marker

在你的 CRD Go 类型定义上方添加 Marker：

```go
// +kubebuilder:subresource:status
type DemoApp struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   DemoAppSpec   `json:"spec,omitempty"`
    Status DemoAppStatus `json:"status,omitempty"`
}
```

**⚠️ 关键注意事项（踩坑记录）：**

1. **Marker 必须紧贴 struct**：`// +kubebuilder:subresource:status` 和 `type DemoApp struct` 之间 **不能有空行**
2. **不能有块注释**：`/* ... */` 会干扰 `controller-gen` 的解析
3. **只生成本地 YAML**：`make manifests` 不会自动更新集群中的 CRD

### 1.2 生成 CRD YAML

```bash
make manifests
```

这会调用 `controller-gen` 生成 `config/crd/bases/<group>_<version>_<kind>.yaml`。

生成的 YAML 中应该包含：

```yaml
spec:
  versions:
    - name: v1alpha1
      subresources:
        status: {}    # 🎯 这就是 Marker 生成的内容
```

### 1.3 应用到集群

```bash
kubectl apply -f config/crd/bases/demo.vitaaaaa.icu_demoapps.yaml
```

验证：

```bash
kubectl get crd demoapps.demo.vitaaaaa.icu -o yaml | grep -A 2 subresources
```

应该看到：
```yaml
subresources:
  status: {}
```

---

## 2. 暴露的 API 端点

启用 `/status` 后，API Server 会暴露 **两个端点**：

| 端点 | 方法 | 用途 |
|------|------|------|
| `/apis/demo.vitaaaaa.icu/v1alpha1/namespaces/{ns}/demoapps/{name}` | `GET/PUT/PATCH` | 操作主资源（spec + metadata） |
| `/apis/demo.vitaaaaa.icu/v1alpha1/namespaces/{ns}/demoapps/{name}/status` | `GET/PUT/PATCH` | **仅操作 status** |

你可以直接用 curl 验证：

```bash
# 获取 status 子资源
kubectl get --raw /apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/status-demo/status | jq
```

---

## 3. Controller Runtime 中的用法

### 3.1 Status().Update() — ❌ 不推荐

```go
// 需要 RV，并发时会 409 Conflict
app.Status.Phase = "Running"
err := r.Status().Update(ctx, app)
```

**为什么不推荐：**
- 检查 `ResourceVersion`，如果对象在 Get 和 Update 之间被其他人修改了，会返回 `409 Conflict`
- 在 Reconcile 循环中，这意味着你需要 `return err` 让 WorkQueue 重试
- 虽然 WorkQueue 会重试，但这增加了不必要的延迟

### 3.2 Status().Patch() — ✅ 推荐

```go
// 不需要 RV 检查，不会 409
original := app.DeepCopy()          // 或者用你的 DeepCopy 函数
app.Status.Phase = "Running"
app.Status.ReadyReplicas = 3
err := r.Status().Patch(ctx, app, client.MergeFrom(original))
```

**为什么推荐：**
- `MergeFrom` 使用 3-way merge，不需要检查 RV
- 即使并发有其他写入，也不会 409 Conflict
- 更安全：只发送变更的部分，不发送整个对象

### 3.3 两种方式的对比

| 方法 | API 调用 | 需要 RV | 冲突风险 | 推荐度 |
|------|---------|---------|---------|--------|
| `Status().Update()` | `PUT .../status` | ✅ 需要 | ✅ 有 | ❌ 不推荐 |
| `Status().Patch()` | `PATCH .../status` | ❌ 不需要 | ❌ 无 | ✅ 推荐 |

---

## 4. "偷渡测试"（Smuggling Test）

当你通过 `/status` 端点更新状态时，即使你在请求 Payload 中偷偷改了 `Spec`，API Server 也会 **静默丢弃** 对 `Spec` 的变更。

### 4.1 实验代码

```go
original := GetDemoApp()
modified := DeepCopyDemoApp(original)

modified.Status.Phase = "Hacked"        // ✅ 合法的 Status 更新
modified.Spec.Image = "hacked:latest"   // ❌ 恶意的 Spec 偷渡！

r.Status().Patch(ctx, modified, client.MergeFrom(original))
```

### 4.2 预期结果

- API 返回 `200 OK`（请求成功）
- 再次 `Get` 发现：`Status.Phase` 变了，但 `Spec.Image` **依然是原值**
- **证明：** API Server 在 `/status` 端点上做了硬过滤

### 4.3 为什么这样设计？

1. **安全护栏（Safety Net）**：即使 Controller 代码有 bug，意外修改了 Spec，用户的配置也不会被破坏
2. **权限隔离（RBAC）**：Operator 可以只有 `update status` 权限，而没有 `update spec` 权限
3. **避免自我循环**：更新 `/status` 不会触发基于 Spec 变化的 Watch 事件

---

## 5. 实验运行指南

本章节的代码位于 `cmd/status-kubebuilder/`：

```text
cmd/status-kubebuilder/
├── types.go        # CRD 类型定义（含 +kubebuilder:subresource:status Marker）
├── crd.yaml        # 手动编写的 CRD YAML（含 subresources.status: {}）
├── sample.yaml     # 测试资源实例
├── main.go         # Controller + 4 个实验
└── go.mod
```

### 5.1 前置步骤

```bash
# 1. 创建 CRD
kubectl apply -f cmd/status-kubebuilder/crd.yaml

# 2. 创建测试资源
kubectl apply -f cmd/status-kubebuilder/sample.yaml

# 3. 验证 CRD 启用了 /status
kubectl get crd demoapps.demo.vitaaaaa.icu -o yaml | grep -A 2 subresources
```

### 5.2 运行实验

```bash
cd cmd/status-kubebuilder
go run .
```

实验会依次执行：
1. 🔴 观察初始状态
2. 🟢 Status().Update() — 需要 RV，有 409 风险
3. 🟡 Status().Patch() — 不需要 RV，无冲突风险
4. 🔵 偷渡测试 — 验证 API Server 隔离

### 5.3 清理

```bash
kubectl delete demoapp status-demo -n test
kubectl delete crd demoapps.demo.vitaaaaa.icu
```

---

## 6. 核心结论

| 维度 | 普通 Update | /status 子资源 |
|------|------------|---------------|
| **更新范围** | spec + status 一起 | 仅 status |
| **触发 Watch** | ✅ 会 | ❌ 不会 |
| **RBAC 隔离** | ❌ 不能 | ✅ 可以 |
| **多 Controller 协作** | ❌ 冲突 | ✅ 安全 |

**最佳实践：在 Controller 中使用 `r.Status().Patch(ctx, obj, client.MergeFrom(original))`**

---

*上一步：[Spec vs Status 哲学](./01-spec-vs-status-philosophy.md)*
*下一步：[集成 HPA](./04-scale-hpa-integration.md)*
