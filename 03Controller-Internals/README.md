# 03 Controller: 运行机制与 WorkQueue

本专题深入 Controller 的核心逻辑：它是如何把 API Server 的事件转化为代码中的 `Reconcile` 调用的。

## 1. 核心概念

1.  **For()**: 定义**主资源 (Primary Resource)**。主资源的创建/更新/删除会**直接**触发 Reconcile。
2.  **Owns()**: 定义**子资源 (Child Resource)**。通过 OwnerReference 关联。当子资源变化时，Controller 会找到其 Owner（主资源），并把 Owner 的 Key 放入队列。
3.  **Watches()**: 定义**外部资源 (External Resource)**。用于监听那些与主资源没有 Owner 关系，但逻辑上相关的资源。

## 2. 实验记录

### 实验 1: Owns() - 所有权绑定
*   **目标**: 验证修改 ConfigMap 创建的 Secret 能否触发 ConfigMap 的 Reconcile。
*   **代码**: `.Owns(&corev1.Secret{})`
*   **原理**: 当 Secret 变化，Informer 发送事件 -> EventHandler 找到 Secret 的 Owner (ConfigMap) -> 将 ConfigMap 的 Key 放入 WorkQueue。

### 实验 2: Watches() - 外部资源监听
*   **目标**: 验证修改无关的 Deployment 能否触发 ConfigMap 的 Reconcile。
*   **代码**: `.Watches(&appsv1.Deployment{}, handler.EnqueueRequestsFromMapFunc(...))`
*   **原理**: 允许自定义映射逻辑（MapFunc），将任意事件映射为一个或多个 Reconcile 请求。
