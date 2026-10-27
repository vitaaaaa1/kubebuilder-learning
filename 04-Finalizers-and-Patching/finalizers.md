# Controller 专题 (6): Finalizer 与资源生命周期管理

本章节记录如何通过 **Finalizer** 机制拦截资源删除请求，实现安全的资源清理流程。

## 1. 核心概念

### 1.1 为什么需要 Finalizer？

在 Kubernetes 中，普通的资源（如 ConfigMap、Pod）被删除后会**立即从 etcd 中物理删除**。

但如果你的 Operator 管理的资源需要**额外的清理工作**（例如：删除云数据库、取消 DNS 记录、释放外部 API 配额），直接删除会导致**孤儿资源**（Orphan Resources）残留在外部系统中。

**Finalizer 的本质**：一个字符串标记（如 `my-operator/cleanup`），告诉 K8s："这个资源不能直接删，得先让我做完收尾工作。"

---

### 1.2 软删除 vs 硬删除

| 类型 | 触发方式 | K8s 行为 | 资源状态 |
| :--- | :--- | :--- | :--- |
| **硬删除** | 资源无 Finalizer | 直接从 etcd 移除 | 立即消失 |
| **软删除** | 资源有 Finalizer | 设置 `deletionTimestamp`，资源进入 `Terminating` | 还能被 `Get` 到，但等待清理 |

---

### 1.3 4 步删除流程

当资源包含 Finalizer 时，删除流程如下：

```text
用户执行 kubectl delete
        │
        ▼
┌─────────────────────────────────────────┐
│ 步骤 1: K8s 设置 deletionTimestamp       │
│         资源进入 Terminating 状态         │
│         （软删除，资源还在）              │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│ 步骤 2: Controller 看到 !IsZero()       │
│         触发 Reconcile                  │
│         执行清理逻辑（删除外部资源）     │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│ 步骤 3: Controller 移除 Finalizer 字符串 │
│         调用 Update 写回 API Server      │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│ 步骤 4: K8s 发现 finalizers 为空         │
│         执行物理删除，资源真正消失       │
└─────────────────────────────────────────┘
```

---

## 2. 代码逻辑拆解 (`cmd/finalizer-demo/main.go`)

### 2.1 核心判断：`DeletionTimestamp.IsZero()`

```go
if !cm.DeletionTimestamp.IsZero() {
    // 🛑 正在被删除，执行清理逻辑
} else {
    // 🛡️ 正常资源，添加 Finalizer 保护
}
```

*   **`IsZero() == true`**：资源正常运行，`deletionTimestamp` 为空
*   **`IsZero() == false`**：资源正在被删除，`deletionTimestamp` 已被 K8s 设置

### 2.2 Finalizer API 说明

| API | 作用 |
| :--- | :--- |
| `controllerutil.ContainsFinalizer(&obj, name)` | 检查 `obj.Finalizers` 列表是否包含指定字符串 |
| `controllerutil.AddFinalizer(&obj, name)` | 往 `obj.Finalizers` 追加字符串 |
| `controllerutil.RemoveFinalizer(&obj, name)` | 从 `obj.Finalizers` 删除该字符串 |

### 2.3 两条路径

#### 路径 A：正常资源（第 69-77 行）

```go
if !controllerutil.ContainsFinalizer(&cm, finalizerName) {
    controllerutil.AddFinalizer(&cm, finalizerName)
    if err := r.Update(ctx, &cm); err != nil {
        return ctrl.Result{}, err
    }
}
```

**触发时机**：新创建的 ConfigMap

**效果**：这个 ConfigMap 以后不能被直接删除了，必须先经过清理流程

---

#### 路径 B：正在删除的资源（第 50-65 行）

```go
if !cm.DeletionTimestamp.IsZero() {
    if controllerutil.ContainsFinalizer(&cm, finalizerName) {
        // ---------------------------------------------------------
        // 在这里执行清理逻辑（例如：删除云数据库、取消 DNS 记录等）
        // ---------------------------------------------------------

        controllerutil.RemoveFinalizer(&cm, finalizerName)
        if err := r.Update(ctx, &cm); err != nil {
            return ctrl.Result{}, err
        }
    }
    return ctrl.Result{}, nil
}
```

**触发时机**：用户执行 `kubectl delete cm <name>`

**关键点**：
1.  必须先检查 `ContainsFinalizer`，避免重复清理
2.  清理完成后**必须**移除 Finalizer，否则资源永远卡在 `Terminating` 状态
3.  如果 `Update` 失败，返回 `error`，WorkQueue 会重新重试

---

## 3. 命名规范与最佳实践

### 3.1 Finalizer 命名

Finalizer 应该使用**反向域名**格式，避免与其他 Operator 冲突：

```go
const finalizerName = "kubebuilder-learning.cleanup-handler"
// 或
const finalizerName = "my-operator.io/finalizer"
```

### 3.2 常见错误

| 错误 | 现象 | 修复 |
| :--- | :--- | :--- |
| 忘记移除 Finalizer | 资源永远卡在 `Terminating` | 确保 `RemoveFinalizer` + `Update` 被执行 |
| `Update` 失败后未重试 | Finalizer 未被移除，清理未完成 | 返回 `error`，让 WorkQueue 重试 |
| 清理逻辑报错但未处理 | 外部资源残留 | 在清理逻辑中处理所有错误，确保最终能移除 Finalizer |

---

## 4. 实验验证

通过运行 `cmd/finalizer-demo/main.go`，可以观察到以下现象：

```bash
# 1. 创建一个 ConfigMap
kubectl create cm test-cm --from-literal=key=value

# Controller 日志输出：
# 🛡️ Finalizer added to protect resource.

# 2. 尝试删除
kubectl delete cm test-cm
# 此时命令会卡住，资源不会立即消失

# Controller 日志输出：
# 🛑 Intercepting deletion! Cleaning up external resources...
# ✅ Cleanup done. Finalizer removed. Resource will disappear now.

# 3. 资源真正消失
kubectl get cm test-cm
# Error from server (NotFound): ...
```

---

## 5. 与 OwnerReference 的区别

| 机制 | 触发时机 | 用途 |
| :--- | :--- | :--- |
| **Finalizer** | 资源被删除时 | 拦截删除，执行**外部清理**（云资源、DNS、外部 API） |
| **OwnerReference** | 父资源被删除时 | 自动触发 K8s **级联删除**（子资源跟着删） |

*   **Finalizer** 用于**外部系统**的清理
*   **OwnerReference** 用于**K8s 内部资源**的自动级联删除

---

*下一站：Patching 策略（MergeFrom、StrategicMergeFrom、Server-Side Apply）*
