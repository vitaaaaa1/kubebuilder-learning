# 04 Finalizers and Patching: 资源生命周期与更新策略

本专题记录如何安全地管理资源的生命周期（拦截删除）以及如何高效地更新资源（Patching 策略）。

---

## 1. Finalizer（已完成）

**文档**：[finalizers.md](finalizers.md)

**核心概念**：
-   **Finalizer** 是一个字符串标记，告诉 K8s "这个资源不能直接删，得先让我做完收尾工作"。
-   用于防止**孤儿资源**（如云数据库、外部 DNS 记录）在 CR 被删除后残留。

**4 步删除流程**：
1.  用户执行 `kubectl delete`。
2.  K8s 设置 `deletionTimestamp`，资源进入 `Terminating` 状态（软删除）。
3.  Controller 看到 `!deletionTimestamp.IsZero()`，执行清理逻辑。
4.  Controller 移除 Finalizer，K8s 执行物理删除。

**关键 API**：
-   `controllerutil.ContainsFinalizer(...)`: 检查是否包含 Finalizer。
-   `controllerutil.AddFinalizer(...)`: 添加保护。
-   `controllerutil.RemoveFinalizer(...)`: 移除保护（允许删除）。

**实验**：`cmd/finalizer-demo/main.go` 演示如何拦截 ConfigMap 的删除请求。

---

## 2. Patching 策略（待学习）

**计划内容**：
-   `MergeFrom` vs `StrategicMergeFrom` vs `JSONPatch` 的区别。
-   `client.Apply`（Server-Side Apply）的使用场景。
-   `r.Status().Patch()` vs `r.Update()` 的选择。
-   字段冲突与 Ownership 概念。

---

## 实验目录结构

```
04-Finalizers-and-Patching/
├── cmd/
│   └── finalizer-demo/
│       └── main.go          # Finalizer 拦截删除实验
├── finalizers.md            # Finalizer 理论文档
└── README.md                # 本文件（专题总览）
```
