# kubebuilder-learning

本项目用于学习 Kubernetes Operator 开发，基于 [kubebuilder](https://book.kubebuilder.io/) 框架。

---

## 专题一：CRD API 进阶定义与增强

### 1.1 验证标记 (Validation Markers)

验证标记位于 Go 结构体字段上方，`controller-gen` 会读取它们并生成 **OpenAPI v3 Schema** 写入 CRD。

**为什么需要：** 防止用户输入非法数据，在提交阶段（`kubectl apply`）就拦截错误，避免无效的 Reconcile 循环。

#### 常用标记：

| 标记 | 说明 | 适用类型 | 示例 |
|------|------|----------|------|
| `+kubebuilder:validation:Enum` | 限制为枚举值之一 | 字符串/数字 | `// +kubebuilder:validation:Enum=Running;Stopped` |
| `+kubebuilder:validation:Minimum` | 最小值（含边界） | 数字 | `// +kubebuilder:validation:Minimum=1` |
| `+kubebuilder:validation:Maximum` | 最大值（含边界） | 数字 | `// +kubebuilder:validation:Maximum=100` |
| `+kubebuilder:validation:MultipleOf` | 必须是某数的倍数 | 数字 | `// +kubebuilder:validation:MultipleOf=5` |
| `+kubebuilder:validation:MinLength` | 字符串最小长度 | 字符串 | `// +kubebuilder:validation:MinLength=1` |
| `+kubebuilder:validation:MaxLength` | 字符串最大长度 | 字符串 | `// +kubebuilder:validation:MaxLength=100` |
| `+kubebuilder:validation:Pattern` | 正则表达式匹配 | 字符串 | `// +kubebuilder:validation:Pattern="^[a-z]+$"` |

#### 必填与选填：
- 指针类型（`*string`）**不带** `omitempty` 即为必填。
- 指针类型 **带** `omitempty` 即为选填。

---

### 1.2 打印列 (Printer Columns)

通过 `+kubebuilder:printcolumn` 标记，可以让 `kubectl get <crd-name>` 命令直接显示资源的关键业务信息，而不需要每次都 `kubectl describe` 或 `-o yaml`。

**为什么需要：** 极大提升日常运维和调试效率，一眼看清所有资源状态。

#### 标记格式：

```go
// +kubebuilder:printcolumn:name="列名",type=类型,JSONPath=路径,priority=优先级,description="描述"
```

#### 参数说明：
- **name**: 列表表头显示的列名。
- **type**: 数据类型（`string`, `integer`, `boolean`, `date`）。
- **JSONPath**: 数据在 CR 实例 YAML 中的路径（如 `.spec.message`）。
- **priority**: (可选) 默认为 0。大于 0 的列默认隐藏，需使用 `-o wide` 才显示。

#### 示例：

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=.spec.message01
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=.status.conditions[?(@.type=='Available')].status

type HelloWorld struct { ... }
```

#### JSONPath 进阶：
- `.spec.message01`: 提取 Spec 下的字段。
- `.status.conditions[?(@.type=='Available')].status`: 复杂的查询。表示从 conditions 数组中找到 type 为 `Available` 的那一项，并取其 status 字段。

#### 隐藏列（Priority）：
如果你定义的列太多，表格会变得很长。可以通过设置 `priority`（默认为 0）来控制列的显示。
- **priority=0**：默认显示。
- **priority=1**：隐藏，只有在使用 `-o wide` 时才会显示。

```go
// +kubebuilder:printcolumn:name="HiddenField",type=string,JSONPath=.spec.foo,priority=1
```

这在显示“详细描述”、“内部 ID”等次要字段时非常有用，可以保持默认输出的简洁。

#### 更新步骤：
```bash
make manifests  # 重新生成 CRD YAML（会更新 additionalPrinterColumns 部分）
make install    # 必须更新集群中的 CRD，否则 kubectl get 不会显示新列
```

---

### 1.3 简写与分类 (ShortNames & Categories)

让自定义资源（CRD）像 Kubernetes 内置资源（如 `pod`、`svc`）一样便于使用。

**为什么需要：** 减少命令行输入长度；让资源能够被 `kubectl get all` 统一管理。

#### 标记格式：

在结构体上方添加注释（注意：必须使用 `resource:` 前缀）：

```go
// +kubebuilder:resource:shortName=hw
// +kubebuilder:resource:categories=all
type HelloWorld struct { ... }
```

#### 参数说明：

**`+kubebuilder:resource:shortName=hw`**
- 定义资源的短名称。
- 效果：可以使用 `kubectl get hw` 代替完整的 `kubectl get helloworlds.demo.vitaaaaa.icu`。
- *注意：如果有多个短名称，需要写多行。*

**`+kubebuilder:resource:categories=all`**
- 将资源归入某个类别。
- `all` 是最常用的类别，包含 Kubernetes 内置的核心资源（Pod, Service, Deployment 等）。
- 效果：执行 `kubectl get all` 时会显示你的 CRD。

#### 生成的 CRD YAML 变化：

```yaml
spec:
  names:
    kind: HelloWorld
    plural: helloworlds
    shortNames:
    - hw        # ← 由 +kubebuilder:resource:shortName=hw 生成
    categories:
    - all       # ← 由 +kubebuilder:resource:categories=all 生成
```

#### 更新步骤：
```bash
make manifests  # 重新生成 CRD YAML
kubectl apply -f config/crd/bases/demo.vitaaaaa.icu_helloworlds.yaml # 必须更新集群中的 CRD 定义！

# 测试短名称
kubectl get hw -n test

# 测试类别（应包含 HelloWorld）
kubectl get all -n test
```

---
