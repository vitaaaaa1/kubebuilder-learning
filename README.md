# kubebuilder-learning

本项目用于学习 Kubernetes Operator 开发，基于 [kubebuilder](https://book.kubebuilder.io/) 框架。

---

## 专题：API 多版本与存储版本 (Storage Version)

本专题记录了如何在一个全新环境中，从初始化开始，逐步构建多版本 CRD 并设置存储版本的完整过程。

### 1. 初始化与多版本构建

为了模拟真实场景下的 API 演进（从 `v1alpha1` 到 `v1beta1`），我们执行了以下步骤：

1.  **初始化项目骨架**
    ```bash
    kubebuilder init --domain vitaaaaa.icu --repo github.com/vitaaaaa/kubebuilder-learning
    ```
2.  **创建 v1alpha1 版本**
    ```bash
    kubebuilder create api --group demo --version v1alpha1 --kind HelloWorld
    ```
3.  **创建 v1beta1 版本**
    由于是同一个 Kind，控制器文件已存在，因此使用 `--controller=false` 仅生成 API 定义。
    ```bash
    kubebuilder create api --group demo --version v1beta1 --kind HelloWorld --controller=false
    ```

### 2. 核心概念

在 Kubernetes 中，etcd 只能以**一种**格式存储数据。
*   **Storage Version**：决定了数据在 etcd 中长什么样。
*   **自动转换**：API Server 会自动将请求转换为 Storage Version 存入 etcd，并在读取时将 Storage Version 转换回用户请求的版本。

### 3. 设置存储版本

通过 `// +kubebuilder:storageversion` 标记来指定哪个版本是 Storage Version。同一个资源在同一时间只能有一个存储版本。

**操作：**
我们在 `api/v1beta1/helloworld_types.go` 的 `HelloWorld` 结构体上方添加了标记，并确保 `api/v1alpha1/helloworld_types.go` 中**没有**该标记。

```go
// api/v1beta1/helloworld_types.go

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion   <-- 指定此版本为存储版本
type HelloWorld struct { ... }
```

### 4. 验证结果

执行 `make manifests` 后，生成的 CRD YAML (`config/crd/bases/demo.vitaaaaa.icu_helloworlds.yaml`) 显示了版本状态：

*   **v1alpha1**：`served: true`, `storage: false`
*   **v1beta1**：`served: true`, `storage: true`

**结论：** 所有的资源数据将底层以 `v1beta1` 格式存储，但用户依然可以通过 `v1alpha1` 客户端安全地访问数据（API Server 负责自动转换）。
