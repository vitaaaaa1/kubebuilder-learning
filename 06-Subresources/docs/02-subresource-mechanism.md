# `/status` 子资源的“写入隔离”机制

本文档记录 Kubernetes `/status` 子资源在 API Server 端的底层行为，以及它如何保护 Spec 不被意外覆盖。

---

## 1. 核心结论

`UpdateStatus` **不是靠客户端“智能识别”**哪些该改，而是 **API Server 端口的硬过滤**。

即使你在客户端请求中修改了 `Spec`，只要请求发往的是 `/status` 端点，API Server 也会**静默丢弃**对 `Spec` 的变更。

---

## 2. 工作流程：从“全量发送”到“精准入库”

| 阶段 | 行为 | 说明 |
| :--- | :--- | :--- |
| **客户端 (`client-go`)** | 序列化整个 `modified` 对象 | **不拦截！** 即使你偷偷改了 `Spec`，也会原封不动打包成 JSON 发出。 |
| **API Server (`/status` 端点)** | **提取 `status`，丢弃 `spec`** | **核心拦截层。** 服务器发现 URL 路径是 `/status`，会直接忽略 Payload 中的 `spec` 和无关 `metadata`。 |
| **etcd** | 只持久化 `status` | `Spec` 保持原样，`Status` 更新为新值。返回的新对象中 `Spec` 依然是旧值。 |

---

## 3. “偷渡测试”验证 (Smuggling Test)

我们在 `06-Subresources/cmd/status-subresource/main.go` 中进行了实战验证：

```go
// 1. 获取对象
existing := GetHPA()

// 2. 准备“偷渡”
modified := existing.DeepCopy()
modified.Status.CurrentReplicas = 99       // ✅ 合法的 Status 更新
modified.Spec.MinReplicas = ptrInt32(999)  // ❌ 恶意的 Spec 偷渡！

// 3. 发送 UpdateStatus
clientset.UpdateStatus(ctx, modified, ...)
```

**运行结果：**
*   API 返回 `200 OK`（请求成功）。
*   再次 `Get` 发现：`Status.CurrentReplicas` 变成了 `99`，但 `Spec.MinReplicas` **依然是原值**。

**证明：** Kubernetes API Server 充当了最后一道防线，**静默丢弃**了对 Spec 的非法修改。

---

## 4. 为什么这样设计？

### 4.1 安全护栏 (Safety Net)
Controller/Operator 的代码可能很复杂。如果开发者在汇报状态时，因为逻辑错误或 `DeepCopy` 失误意外修改了 `Spec`，子资源机制能确保**用户的配置（Spec）永远不会被状态更新逻辑破坏**。

### 4.2 权限隔离 (RBAC)
符合最小权限原则。Operator 通常只需要 `update status` 权限，而不需要 `update spec` 权限。子资源天然实现了权限切割：
```yaml
- apiGroups: ["my.group.io"]
  resources: ["mycrds/status"]  # 只能改 Status
  verbs: ["update", "patch"]
```

### 4.3 避免自我循环 (Reconcile Loop)
更新 `/status` 不会触发基于 `Spec` 变化的 Watch 事件。这防止了 Controller 陷入 `Reconcile → Update Status → Trigger Watch → Reconcile` 的无限死循环。

---

## 5. 关于 ResourceVersion (RV) 的重要澄清

**❌ 常见误区：** “更新 `/status` 不会改变 ResourceVersion。”

**✅ 真相：** `ResourceVersion` **一定会变！**

*   `ResourceVersion` 是 etcd 的**全局事务版本号**，它绑定的是**整个对象**（Spec + Metadata + Status）。
*   只要你往 API Server 成功写入了一次数据（无论改的是 Spec 还是 Status），etcd 就会递增版本号。
*   **隔离的本质**：RV 变了代表“对象被写入了”，但 API Server **过滤了写入的内容**，只允许 Status 变更落盘。

---

*下一步：[Kubebuilder 开发实践：Marker 与代码调用](./03-kubebuilder-implementation.md)*
