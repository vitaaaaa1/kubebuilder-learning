# kubebuilder-learning

本项目用于学习 Kubernetes Operator 开发，基于 [kubebuilder](https://book.kubebuilder.io/) 框架。

---

## 专题二：Controller 运行机制与底层原理

本专题记录如何从零搭建 Controller，深入理解其底层运行逻辑（Manager、Scheme、Informer 等）。

### 1. Scheme (类型注册与解码)

**定义：**
`Scheme` 是 Kubernetes 中的一种**类型注册机制**。它本质上是一个映射表（Registry），用于建立 **GVK (Group-Version-Kind)** 与 **Go 结构体** 之间的关联。

**为什么需要 Scheme？**
Kubernetes API Server 与 Client 之间的通信是基于 JSON (或 Protobuf) 的。当 Client 从 API Server 接收到一段 JSON 数据时，它需要知道应该把这段数据反序列化（解码）为 Go 语言中的哪个结构体。

**工作原理：**
1.  **创建注册表**: `scheme := runtime.NewScheme()` 初始化一个空的 Scheme 对象。
2.  **注册类型**: `_ = corev1.AddToScheme(scheme)` 将 `corev1` 包下所有资源类型（如 Pod, ConfigMap, Service 等）注册到 Scheme 中。
    *   当 Client 看到 JSON 中的 `kind: "ConfigMap"` 时，Scheme 会告诉它：“请使用 `corev1.ConfigMap` 这个 Go 结构体来解析这段数据。”
3.  **自定义资源**: 如果你的 Controller 需要监听自定义资源（CRD），你也必须注册它们：
    ```go
    import myv1 "github.com/example/api/v1"
    _ = myv1.AddToScheme(scheme) // 注册你的 CRD 类型
    ```

**常见报错：**
如果在 Scheme 中未注册某种类型，Client 尝试操作该类型时会报错：
> `no kind is registered for the type v1.ConfigMap`
> (这意味着 Client 无法识别该资源，通常是因为忘记调用 `AddToScheme`)

**Kubebuilder 中的自动处理：**
在使用 `kubebuilder init` 和 `create api` 生成的脚手架中，`cmd/main.go` 文件已经自动帮我们完成了这个注册过程。它通常长这样：

```go
// cmd/main.go (kubebuilder 自动生成的代码片段)

import (
    // 导入你定义的 API 包
    demov1alpha1 "github.com/vitaaaaa/kubebuilder-learning/api/v1alpha1"
    // ...
)

func main() {
    // ...
    // 初始化 Scheme
    scheme = runtime.NewScheme()
    
    // 1. 注册核心 Kubernetes 资源 (通常是 utilruntime.Must 包裹)
    utilruntime.Must(clientgoscheme.AddToScheme(scheme))
    
    // 2. 注册你的自定义资源 (CRD)
    // kubebuilder 会自动把每个版本的 AddToScheme 加到这里
    utilruntime.Must(demov1alpha1.AddToScheme(scheme))
    // ...
    
    // 之后将 scheme 传入 Manager
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme: scheme,
    })
    // ...
}
```
通过这种机制，你在 `api/` 目录下创建的任何新 API，都会自动被加入全局 Scheme，Controller 从而能够无缝识别和监听你的 CRD。
