# kubebuilder-learning

本项目用于学习 Kubernetes Operator 开发，基于 [kubebuilder](https://book.kubebuilder.io/) 框架。

---

## 专题一：CRD API 定义

### 1. 从 Go 代码生成 CRD YAML

#### 完整流程

```
api/v1alpha1/helloworld_types.go (Go 代码)
            ↓
      make manifests
            ↓
   controller-gen 工具扫描代码
            ↓
   读取 +kubebuilder: 注释标记
            ↓
   解析 Go 结构体和字段定义
            ↓
   转换为 OpenAPI v3 Schema
            ↓
config/crd/bases/demo.vitaaaaa.icu_helloworlds.yaml
```

#### controller-gen 是什么？

`controller-gen` 是 kubebuilder 提供的代码生成工具，它能：
- 扫描 Go 源码文件
- 识别 `+kubebuilder:` 注释标记
- 将 Go 类型定义转换为 Kubernetes CRD 格式

#### CRD 文件命名规则

文件名：`<group>.<domain>_<plural-kind>.yaml`

例如：`demo.vitaaaaa.icu_helloworlds.yaml`

- `demo.vitaaaaa.icu` = group + domain
- `helloworlds` = kind 的复数形式

#### Go 代码到 CRD YAML 的映射

| Go 代码 | CRD YAML 对应部分 |
|---------|------------------|
| `type HelloWorld struct` | `kind: HelloWorld` |
| `Spec HelloWorldSpec` | `spec` 属性 |
| `Status HelloWorldStatus` | `status` 属性 |
| `// +kubebuilder:object:root=true` | 注册为 Kubernetes 对象 |
| `// +kubebuilder:subresource:status` | `subresources: status: {}` |

#### 字段如何映射到 OpenAPI Schema

```go
// Go 代码
type HelloWorldSpec struct {
    Message *string `json:"message"`
}

// 生成的 YAML
spec:
  properties:
    message:
      type: string
  required:
  - message
```

- `json:"message"` → YAML 中的字段名是 `message`
- `*string`（指针类型）→ 标记为 `required`（因为不是 `omitempty`）
- Go 的 `string` → YAML 的 `type: string`

#### Conditions 字段的特殊标记

```go
// +listType=map
// +listMapKey=type
Conditions []metav1.Condition `json:"conditions,omitempty"`
```

生成的 YAML：
```yaml
conditions:
  type: array
  x-kubernetes-list-map-keys:
  - type
  x-kubernetes-list-type: map
```

这些 `x-kubernetes-*` 扩展告诉 Kubernetes 如何验证列表。

#### make manifests 实际执行的命令

```bash
# 生成 CRD YAML
$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=config/crd/bases

# 生成 RBAC 配置
$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./..." output:rbac:artifacts:config=config/rbac

# 生成 DeepCopy 代码
$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=2026 paths="./..."
```

#### 每次修改 Go 代码后都要执行

```bash
make manifests
```

这样 CRD YAML 才会与 Go 代码保持同步。

### 2. CRD YAML 字段详细解析

每个字段在 Go 代码中都有明确的来源：

- `metadata.name`: 来源于 `{plural}.{group}.{domain}`。
- `spec.group`: 来源于 `groupversion_info.go`。
- `spec.names`: 来源于结构体名称（Kind, Plural, Singular）。
- `spec.scope`: 默认 Namespaced。
- `spec.versions.name`: 来源于包名（如 `v1alpha1`）。
- `openAPIV3Schema.properties`: 来源于结构体字段及注释。

#### 必填与选填的规则

- `*string` + 无 `omitempty` → `required`
- `*string` + 有 `omitempty` → `optional`
- 指针类型允许区分 "用户未设置 (nil)" 和 "用户设置为 0"。

### 3. Status 与 Conditions

在 Kubernetes 中：
- **Spec**: 期望状态（用户定义）
- **Status**: 实际状态（Controller 更新）

#### Conditions 模式

Kubernetes 推荐的标准状态表达方式：

```go
type HelloWorldStatus struct {
    // +listType=map
    // +listMapKey=type
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

- **Type**: 状态维度（如 `Available`, `Progressing`）。
- **Status**: 状态值（`True`, `False`, `Unknown`）。
- **Reason**: 简短的程序化原因（如 `ConfigMapCreated`）。
- **Message**: 详细的给人看的信息。
- **ObservedGeneration**: 标记状态基于哪个 spec generation。

#### 为什么需要 Status Subresource？

通过 `// +kubebuilder:subresource:status` 启用。

- **权限分离**: 分别控制 `helloworlds` 和 `helloworlds/status` 权限。
- **避免冲突**: 用户更新 Spec，Controller 更新 Status，互不影响。
- **避免循环**: 更新 Status 不会触发 Reconcile。

### 4. Reconcile 方法

Controller 的核心逻辑入口。

#### 返回值说明

```go
func (r *HelloWorldReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
```

| 返回值 | 效果 | 使用场景 |
|--------|------|----------|
| `ctrl.Result{}, nil` | 结束循环 | 工作已完成 |
| `ctrl.Result{RequeueAfter: 5s}, nil` | 延迟重试 | 等待资源就绪 |
| `ctrl.Result{}, err` | 报错，自动指数退避重试 | 出现临时错误 |
| `ctrl.Result{}, client.IgnoreNotFound(err)` | 忽略未找到错误 | 资源已被删除 |

#### 如何更新状态

```go
meta.SetStatusCondition(&helloWorld.Status.Conditions, metav1.Condition{
    Type:               "Available",
    Status:             metav1.ConditionTrue,
    ObservedGeneration: helloWorld.Generation,
    Reason:             "TestingStatus",
    Message:            "Status is being updated.",
})

// 注意：必须使用 Status().Update()
if err := r.Status().Update(ctx, &helloWorld); err != nil {
    return ctrl.Result{}, err
}
```
