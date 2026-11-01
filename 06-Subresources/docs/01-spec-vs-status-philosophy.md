# Spec vs Status: Kubernetes 的核心设计哲学

本文档记录 Kubernetes 资源模型中 **Spec（期望）** 与 **Status（现实）** 分离的设计哲学，以及为什么 `/status` 子资源是 Operator 开发的基石。

---

## 1. 一句话总结

> **Spec 是用户和 Controller 的"愿望"，Status 是世界对愿望的"回应"。**

- `Spec` = **意图（Intent）**：用户/Controller 声明"我想要什么"。
- `Status` = **观察（Observation）**：Controller 回报"现实是什么"。

这不是字段名的区别，而是 **控制论中"设定值"与"反馈值"** 的分离。

---

## 2. 为什么要分离？

### 2.1 控制论视角：闭环控制系统

Kubernetes 的本质是一个 **闭环控制系统**（Closed-Loop Control System）：

```
  ┌─────────────┐      Spec (设定值)       ┌──────────────┐
  │  用户/Client │ ──────────────────────>  │  Controller  │
  │  (声明意图)  │                          │  (调节器)    │
  └─────────────┘                          └──────┬───────┘
                                                  │
                                                  ▼
                                           ┌──────────────┐
                                           │   集群现实    │
                                           │  (Deployment │
                                           │   Pod Node)   │
                                           └──────┬───────┘
                                                  │
                                  Status (反馈值)  │
                                                  ▼
                                           ┌──────────────┐
                                           │  Controller   │
                                           │  (比较差异)    │
                                           └──────────────┘
```

1. **用户声明 Spec**："我要 3 个副本"。
2. **Controller 观察现实**："现在只有 1 个 Pod"。
3. **Controller 调节**：创建 2 个 Pod。
4. **Controller 汇报 Status**："现在确实是 3 个副本了"。
5. **用户看 Status**：确认愿望已实现。

如果 Spec 和 Status 混在一起，这个闭环就断裂了——你无法区分"我想要什么"和"我得到了什么"。

### 2.2 多 Controller 协作视角

一个资源通常被 **多个 Controller** 关注，它们各自关心的部分不同：

| Controller | 关心什么 | 写入什么 |
|---|---|---|
| **用户/kubectl** | `spec`（配置） | `spec.replicas`, `spec.template` |
| **Deployment Controller** | 期望 vs 现实的差距 | `status.replicas`, `status.readyReplicas` |
| **HPA Controller** | 负载指标 | `spec.replicas`（调整副本数） |
| **VPA Controller** | 资源请求 | `spec.containers[].resources` |

**如果不用 `/status` 子资源：**

```go
// ❌ 危险：Deployment Controller 用普通 Update
deploy.Spec.Replicas = 5          // 不小心改了 spec！
deploy.Status.ReadyReplicas = 3   // 本意只想汇报 status
r.Update(ctx, deploy)             // 同时改了 spec + status
```

这会导致：
1. **触发不必要的 Watch**：Spec 变化会触发 Watch，其他 Controller 误以为用户改了配置。
2. **409 Conflict**：如果 HPA 同时也在改 `spec.replicas`，并发写入冲突。
3. **Reconcile 死循环**：自己改自己的 status → 触发自己的 Watch → 再次 Reconcile → 再次更新 status → 无限循环。

---

## 3. Ownership 归属

### 3.1 Spec 的 Owner：用户 + 自动化 Controller

`Spec` 的所有权是**共享的**，但有明确分工：

| 字段 | 典型 Owner | 说明 |
|---|---|---|
| `spec.replicas` | 用户（初始） → HPA（后续） | 用户声明初始值，HPA 根据负载调整 |
| `spec.template` | 用户 | 用户定义 Pod 模板，Controller 不应修改 |
| `spec.selector` | 用户 | 用户定义标签选择器 |

### 3.2 Status 的 Owner：仅 Controller

`Status` 的所有权**专属于 Controller**，用户不应该直接修改它：

```yaml
# ✅ 用户只改 Spec
kubectl scale deployment my-deploy --replicas=5

# ❌ 用户不应该直接改 Status（没有意义）
# Status 是 Controller 观察后的回报，用户手动改它不会改变现实
```

### 3.3 和 SSA managedFields 的关系

你之前学过 SSA 的 `managedFields`，这里有一个关键对应：

| 维度 | SSA managedFields | /status 子资源 |
|---|---|---|
| **隔离粒度** | 字段级别（`f:spec.f:replicas`） | API 端点级别（`/status` 路径） |
| **冲突检测** | 字段归属冲突时返回 409 | 直接丢弃非 Status 字段 |
| **适用范围** | 所有资源 | 仅启用了子资源的 CRD |

**关键区别：** `/status` 的隔离是 **API Server 端的硬过滤**，不依赖 managedFields。即使你在请求里改了 `spec`，只要 URL 是 `/status`，API Server 就会静默丢弃 `spec` 部分。

---

## 4. 现实类比：恒温器

想象家里的 **恒温空调系统**：

| 概念 | 空调系统 | Kubernetes |
|---|---|---|
| **Spec** | 你在遥控器上设定的温度（26°C） | `spec.replicas: 3` |
| **Status** | 空调显示屏上的室温（当前 24°C） | `status.currentReplicas: 3` |
| **Controller** | 空调内部的温控芯片 | Deployment Controller |
| **现实** | 房间的实际温度 | 集群里的 Pod |

**关键洞察：**
- 你设定 26°C（Spec），但房间可能还没到（Status 显示 24°C）。
- 空调会自动加热/制冷，直到 Status 逼近 Spec。
- 如果你直接用手把显示屏的数字从 24°C 改成 26°C（直接改 Status），房间温度**不会变**——这只是自欺欺人。

这就是为什么用户不应该直接修改 Status：**它只是现实的反映，不是现实本身。**

---

## 5. 总结对比表

| 维度 | Spec | Status |
|---|---|---|
| **语义** | 期望状态（Desired） | 观察状态（Observed） |
| **谁写入** | 用户 / 自动化 Controller | 仅 Controller |
| **谁读取** | Controller（用来计算差距） | 用户 / 监控系统 |
| **变更触发 Watch** | ✅ 会 | ❌ 不会（通过 `/status` 子资源时） |
| **更新方式** | `r.Update()` 或 `r.Patch()` | `r.Status().Update()` 或 `r.Status().Patch()` |
| **并发安全** | 可能冲突（多个 Controller） | 天然隔离（不干扰 Spec） |

---

*下一步：[独立 RV 与隔离原理](./02-subresource-mechanism.md)*
