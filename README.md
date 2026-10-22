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

#### 隐藏列（Priority）：
- **priority=0**：默认显示。
- **priority=1**：隐藏，只有在使用 `-o wide` 时才会显示。

---

### 1.3 简写与分类 (ShortNames & Categories)

让自定义资源（CRD）像 Kubernetes 内置资源（如 `pod`、`svc`）一样便于使用。

#### 标记格式：
注意：必须使用 `resource:` 前缀。
```go
// +kubebuilder:resource:shortName=hw
// +kubebuilder:resource:categories=all
type HelloWorld struct { ... }
```

#### 效果：
- **shortName**: 可以使用 `kubectl get hw` 代替全名。
- **categories**: `kubectl get all` 时会显示此 CRD。

---

### 1.4 复杂嵌套类型 (Nested Types)

在真实业务场景中，Spec 往往不是简单的 Key-Value，而是包含嵌套对象、数组、Map 等复杂结构。

#### 1. 嵌套对象 (Nested Object)
```go
type AppConfig struct {
    Port int `json:"port"`
}

type HelloWorldSpec struct {
    Config AppConfig `json:"config"`
}
```
**生成的 YAML**: `config` 变成 `type: object`，其内部字段展开在 `properties` 下。

#### 2. 对象数组 (List of Objects)
```go
type User struct {
    Name string `json:"name"`
    Role string `json:"role"`
}

type HelloWorldSpec struct {
    Users []User `json:"users,omitempty"`
}
```
**生成的 YAML**: `users` 变成 `type: array`，数组元素的定义位于 `items` 字段内。

#### 3. Map 类型 (键值对)
```go
type AppConfig struct {
    // +optional
    Env map[string]string `json:"env,omitempty"`
}
```
**生成的 YAML**: `env` 变成 `type: object`，值的类型定义在 `additionalProperties` 中（例如 `additionalProperties: { type: string }`）。

#### 4. 列表的合并策略 (List Merge Strategy) & 并发覆盖

默认情况下，Kubernetes 更新列表时会**整体替换（原子替换）**。如果多个人同时修改列表，后提交的人会把先提交的人的修改全盘覆盖。

**实验演示（并发覆盖）：**
假设初始 `users` 为 `[Alice=admin, Bob=editor]`。
*   **用户 A** 提交 `user-a.yaml`：把 Alice 权限改为 `viewer`。
*   **用户 B** 提交 `user-b.yaml`：添加 Charlie。*(注意：B 的 YAML 是基于旧数据的，里面 Alice 还是 `admin`)*。

**结果对比：**

| 模式 | 最终结果 | 解释 |
| :--- | :--- | :--- |
| **默认（无标记）** | `[Alice=admin, Bob=editor, Charlie]` | ❌ **并发覆盖**。B 的 `apply` 把整个列表替换了，导致 A 修改 Alice 权限的操作丢失。 |
| **Map 模式** | `[Alice=viewer, Bob=editor, Charlie]` | ✅ **完美共存**。K8s 识别 `name` 是主键，分别处理 A 的“修改”和 B 的“新增”。 |

**代码实现：**
```go
type HelloWorldSpec struct {
    // +listType=map       <-- 开启关联列表（智能合并）
    // +listMapKey=name    <-- 指定 name 为主键
    Users []User `json:"users,omitempty"`
}
```

**什么时候应该使用？**
1. **元素有唯一标识**：如 `name`, `id`, `key`。
2. **需要独立修改**：多个角色/Controller 会分别修改列表中的不同元素。
3. **典型场景**：环境变量 (`Env`), 端口 (`Ports`), 业务规则列表, 用户列表 (`Users`)。

**什么时候不应该使用？**
1. **顺序敏感**：如防火墙规则、Pipeline 步骤（Map 模式不保证元素顺序）。
2. **简单值列表**：如 `tags: ["a", "b"]`（没有主键字段）。
3. **强一致性要求**：列表必须作为一个整体原子出现或消失。
