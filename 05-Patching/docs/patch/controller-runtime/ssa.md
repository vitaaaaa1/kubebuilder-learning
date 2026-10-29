# `client.Apply()` (Server-Side Apply)

本章节记录 controller-runtime 中 `client.Apply()` 的原理、API 演进、真实案例及最佳实践。

---

## 1. 基本信息

| 项目 | 值 |
| :--- | :--- |
| **底层 client-go 类型** | `types.ApplyPatchType` (Server-Side Apply) |
| **Content-Type** | `application/apply-patch+yaml` |
| **合并策略** | 基于字段所有权 (Field Ownership) |
| **API 演进** | 旧 API `client.Apply` (变量) 已废弃，改用 `client.Apply()` (方法) |
| **适用场景** | **多 Controller 协作**、**高频更新**、**声明式资源管理** |

---

## 2. 为什么推荐使用 Apply (SSA)？

在之前的 `client.MergeFrom` 学习中，我们了解到 MergeFrom 存在 **"最后写入者胜出" (Last-Writer-Wins)** 的风险。

**场景复现**：如果你的 Operator 和 ArgoCD 同时修改一个资源，MergeFrom 无法感知冲突，后到达的 Patch 会静默覆盖前者。

**SSA 的解决方案：字段级所有权 (Field Ownership)**
SSA 引入了 `manager` 概念。API Server 会记录每个字段具体属于谁：
- 当 `Manager-A` 尝试修改属于 `Manager-B` 的字段时，API Server 会拒绝并返回 **Conflict**（除非 `ForceOwnership`）。
- 从机制上杜绝了“静默覆盖”。

---

## 3. 核心 API：`ApplyConfiguration`

这是 v0.25.0+ 引入的**类型安全构建器**，专门用于 SSA。

### 3.1 新旧 API 对比

| 维度 | 旧 API (`client.Patch`) | **新 API (`client.Apply`)** ✅ |
| :--- | :--- | :--- |
| **方法名** | `client.Patch(ctx, obj, client.Apply, ...)` | `client.Apply(ctx, obj, ...)` |
| **参数类型** | 普通 Go struct (如 `*corev1.ConfigMap`) | **`runtime.ApplyConfiguration`** |
| **构造方式** | 直接赋值 `cm.Labels = ...` | **链式调用** `.WithLabels(...)` |

### 3.2 为什么需要 `ApplyConfiguration`？

普通 struct 是“可变状态”对象，包含 `CreationTimestamp`、`ResourceVersion` 等 K8s 服务端字段。
`ApplyConfiguration` 是一个“纯粹声明式意图”，不包含服务端管理的字段。

**优势：**
1.  **不需要 Get**：你不需要知道资源的当前状态，只需声明“我想要什么”。
2.  **不需要 DeepCopy**：每次 Apply 都是全新构造的。
3.  **类型安全**：所有字段由 Go 编译器检查。

---

## 4. 真实案例分析：HPA 冲突 (my-app01)

### 4.1 案例背景

在你的集群中，有一个真实的 HPA 资源 `test/my-app01`，被三方势力瓜分：

1.  **`kubectl-client-side-apply`**: 管理 `maxReplicas`, `metrics` (用户手动 apply)。
2.  **`___9go_build_...`**: 管理 `minReplicas` (你的 Scaler Operator，由 GoLand 编译运行)。
3.  **`kube-controller-manager`**: 管理 `status` (K8s 自带)。

### 4.2 冲突产生

当你尝试将 Scaler 的 `FieldOwner` 规范化为 `vita-deploy-scaler` 时，API Server 报错：
```
Apply failed with 1 conflict: conflict with "___9go_build_github_com_vita_deploy_scaler_cmd" using autoscaling/v2: .spec.minReplicas
```

**原因：** `minReplicas` 已经被旧身份（GoLand 编译名）占用了。K8s 认为新身份 `vita-deploy-scaler` 是在“抢地盘”。

### 4.3 解决方案：`ForceOwnership`

在 Operator 身份变更或首次接管资源时，必须使用 `client.ForceOwnership`。

```go
// 代码位置: 05-Patching/cmd/controller-runtime/ssa-hpa-demo/main.go
c.Apply(ctx, hpaApply, 
    client.FieldOwner("vita-deploy-scaler"), 
    client.ForceOwnership) // 强制过户
```

**效果：** API Server 会将 `minReplicas` 的所有权从旧 Manager 转移给新 Manager，后续更新不再需要 `ForceOwnership`（除非有人再次抢）。

---

## 5. 代码示例：改造 Scaler

### ❌ 旧代码 (Update 模式 - 危险)

```go
// 1. Get 获取当前状态
hpa := &autoscalingv2.HorizontalPodAutoscaler{}
s.Get(ctx, key, hpa)

// 2. 修改字段
hpa.Spec.MinReplicas = &config.MinReplicas

// 3. 全量更新
s.Update(ctx, hpa) // 风险：409 Conflict 冲突，可能覆盖其他字段
```

### ✅ 新代码 (SSA 模式 - 推荐)

```go
// 1. 构造声明式意图 (不需要 Get)
hpaApply := applyautoscalingv2.HorizontalPodAutoscaler(name, namespace).
    WithSpec(
        applyautoscalingv2.HorizontalPodAutoscalerSpec().
            WithMinReplicas(config.MinReplicas),
    )

// 2. 声明式应用
s.Apply(ctx, hpaApply, client.FieldOwner("vita-deploy-scaler"))
// 优势：只修改 MinReplicas，不影响 maxReplicas 和 metrics
```

---

## 6. `FieldOwner` 命名规范

`FieldOwner` 是记录在 `managedFields` 中的身份标签，必须**全局唯一且稳定**。

| 推荐格式 | 示例 | 适用场景 |
| :--- | :--- | :--- |
| `<项目名>/<组件名>` | `my-app/scaler` | **最佳实践**：多组件协作清晰 |
| `<项目名>-operator` | `my-app-operator` | 简单 Operator |
| `<项目名>-controller` | `my-app-scaling-controller` | 特定 Controller |

**禁止项：**
- ❌ 不要使用动态值（如 PID、时间戳）。
- ❌ 不要使用过长的自动生成的二进制名（如 `___go_build_...`）。

---

## 7. 总结：与 MergeFrom 的对比

| 维度 | `client.MergeFrom` | `client.Apply` (SSA) |
| :--- | :--- | :--- |
| **所有权** | 无 (最后写入者胜出) | **有 (字段级隔离)** |
| **并发安全** | 需配合 OptimisticLock | **原生支持** |
| **代码复杂度** | 需 Get + DeepCopy | **无需 Get**，直接声明 |
| **误伤风险** | 有 (全量覆盖 Diff 字段) | **无** (只改自己声明的字段) |
| **适用场景** | 独占资源的简单修改 | **多 Controller 协作的标准范式** |

---

*实验代码：`cmd/controller-runtime/ssa-hpa-demo/`*
*上一步：[`MergeFrom 三路合并`](./merge-from.md) | 下一步：[`StrategicMergeFrom`](./strategic-merge-from.md)*
