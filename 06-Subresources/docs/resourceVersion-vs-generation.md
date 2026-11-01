# ResourceVersion vs Generation

本文档记录 Kubernetes 中 `metadata.resourceVersion` 和 `metadata.generation` 的本质区别，以及它们如何影响 Controller 的行为。

---

## 1. 一句话总结

| 字段 | 一句话 |
|---|---|
| `resourceVersion` | **etcd 的全局事务 ID**——任何写入都递增，不区分写了什么。 |
| `generation` | **Spec 的版本号**——只有 Spec 变化时才递增，Status 和 Annotation 不影响它。 |

---

## 2. 详细对比

| 维度 | `resourceVersion` | `metadata.generation` |
|---|---|---|
| **谁分配的？** | API Server（底层是 etcd 的全局 Revision） | API Server |
| **什么操作会变？** | **任何写入**（Spec、Status、Annotation） | **只有 Spec** 写入 |
| **Status 更新会变吗？** | ✅ 变 | ❌ 不变 |
| **Annotation 更新会变吗？** | ✅ 变 | 看资源类型（CRD 通常不变） |
| **范围** | **集群全局**（所有资源共用同一个 etcd Revision 池） | **单个资源**（每个资源独立递增） |
| **能手动设置吗？** | ❌ 不能 | ❌ 不能 |
| **会回退吗？** | ❌ 永远递增 | ❌ 永远递增 |
| **典型用途** | 乐观锁（防止并发覆盖）、Informer 事件流 | 判断"用户是否改了期望状态" |
| **Controller 关心它吗？** | Informer 依赖它触发事件 | Controller 用 `GenerationChangedPredicate` 决定是否响应 |

---

## 3. 工作流程：一个完整例子

### 3.1 初始状态

```yaml
metadata:
  resourceVersion: "1000"
  generation: 1
spec:
  replicas: 3
status:
  phase: Pending
```

### 3.2 用户改了 `spec.replicas: 3 → 5`

```yaml
metadata:
  resourceVersion: "1001"    # ✅ 变了（etcd 写入了新数据）
  generation: 2              # ✅ 变了（Spec 变了）
spec:
  replicas: 5
status:
  phase: Pending
```

### 3.3 Controller 更新了 `status.phase: Running`

```yaml
metadata:
  resourceVersion: "1002"    # ✅ 变了（etcd 写入了新数据）
  generation: 2              # ❌ 没变（Status 不影响 generation）
spec:
  replicas: 5
status:
  phase: Running
```

### 3.4 用户打了 Annotation

```yaml
metadata:
  resourceVersion: "1003"    # ✅ 变了
  generation: 2              # CRD 不变（Deployment 等内置资源会变）
  annotations:
    my-annotation: "hello"
spec:
  replicas: 5
status:
  phase: Running
```

---

## 4. 和 Controller 的关系

### 4.1 触发链

```
API Server 写入（Spec 或 Status 或 Annotation）
        ↓
  resourceVersion 递增（必然发生）
        ↓
  Informer 看到 RV 变了 → 发送 Update 事件
        ↓
  检查 Predicate（如果有的话）
        ↓
  Predicate 放行 → 入队 → Reconcile
```

### 4.2 `.For()` 的默认行为

```go
ctrl.NewControllerManagedBy(mgr).
    For(&DemoApp{}).           // ← 没有 Predicate
    Complete(&Reconciler{})
```

**默认只用了 `EnqueueRequestForObject`**——只要 Informer 收到 Update 事件（即 RV 变了），就入队触发 Reconcile。

| 什么变了？ | RV 变吗？ | generation 变吗？ | 默认 `.For()` 触发？ |
|---|---|---|---|
| `spec.replicas` | ✅ | ✅ | ✅ |
| `status.phase` | ✅ | ❌ | ✅ |
| `metadata.annotations` | ✅ | ❌（CRD） | ✅ |

### 4.3 加 `GenerationChangedPredicate` 后

```go
ctrl.NewControllerManagedBy(mgr).
    For(&DemoApp{}).
    WithEventFilter(predicate.GenerationChangedPredicate{}).  // ← 核心
    Complete(&Reconciler{})
```

**`GenerationChangedPredicate` 的源码逻辑**：

```go
func (TypedGenerationChangedPredicate[object]) Update(e event.TypedUpdateEvent[object]) bool {
    return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}
```

| 什么变了？ | generation 变吗？ | 加 Predicate 后触发？ |
|---|---|---|
| `spec.replicas` | ✅ | ✅（generation 变了，放行） |
| `status.phase` | ❌ | ❌（generation 没变，拦截） |
| `metadata.annotations` | ❌（CRD） | ❌ |

**这就是为什么生产环境推荐加它：** 避免"自己改 Status → Informer 触发 → 又进 Reconcile → 又改 Status"的死循环。

---

## 5. 常见误区

### ❌ 误区 1：".For() 默认只监听 Spec 变化"

**不对**。`.For()` 默认监听**任何变化**（RV 变了就触发）。`generation` 只是 `GenerationChangedPredicate` 的过滤指标，不是默认行为。

### ❌ 误区 2："generation 变才会触发 Informer"

**不对**。Informer **只看 `resourceVersion`**。`generation` 是 Predicate 层的过滤条件，Informer 根本不关心它。

### ❌ 误区 3："resourceVersion 是单个资源的版本号"

**不对**。`resourceVersion` 是 **etcd 集群的全局 Revision**，所有资源的写入共享同一个 etcd Revision 池。资源 A 的写入会让资源 B 的 RV 也递增。

---

## 6. 类比理解

| 概念 | 现实类比 | Kubernetes 对应 |
|---|---|---|
| `resourceVersion` | **小区的快递柜编号**——任何人的快递到了，柜子编号都递增。你看编号就知道"有人收了快递"，但不知道是谁。 | 任何写入都递增 |
| `generation` | **你家门的装修次数**——只有你改了自己家的装修（Spec），这个数才变。邻居装修不影响你家。 | 只有 Spec 变化才递增 |

---

## 7. 核心结论

| 维度 | `resourceVersion` | `generation` |
|---|---|---|
| **本质** | etcd 全局事务 ID | Spec 版本号 |
| **触发 Informer？** | ✅ 是（唯一入口） | ❌ 不是 |
| **用于过滤？** | ❌ 不是 | ✅ `GenerationChangedPredicate` |
| **Controller 死循环原因** | Status 更新 → RV 变 → Informer 触发 | Status 更新 → generation 不变 → Predicate 拦截 |

---

*上一步：[独立 RV 与隔离原理](./02-subresource-mechanism.md)*
