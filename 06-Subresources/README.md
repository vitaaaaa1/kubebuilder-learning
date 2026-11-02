# 06 Subresources: 子资源学习路径

本章节记录 Kubernetes 中 Subresources（子资源）的机制、使用场景及最佳实践。

---

## 目录结构

```text
06-Subresources/
├── cmd/
│   ├── status-kubebuilder/               # 实验 1: Status 子资源（Update vs Patch vs 偷渡测试）
│   ├── status-subresource/               # 实验 2: 观察独立 RV 与不触发 Reconcile
│   ├── WithEventFilter-predicate/        # 实验 3: Predicate 与无限循环演示
│   ├── scale-subresource/                # 实验 4: Scale 子资源与 HPA 集成（纯读写 /scale）
│   ├── scale-with-deployment/            # 实验 5: 管理真实 Deployment + HPA 闭环
│   └── rbac-subresource/                 # 实验 6: 子资源的 RBAC 隔离验证
├── docs/
│   ├── 01-spec-vs-status-philosophy.md   # 理论：Spec vs Status 的核心哲学
│   ├── 02-subresource-mechanism.md       # 机制：独立 RV 与不触发 Reconcile
│   ├── 03-kubebuilder-implementation.md  # 实践：Marker、YAML 生成与代码调用
│   ├── 04-scale-hpa-integration.md       # 进阶：Scale 子资源与 HPA 集成
│   ├── infinite-loop-and-predicates.md   # 补充：无限循环问题与 Predicate 使用方法
│   └── resourceVersion-vs-generation.md  # 补充：RV 与 Generation 的本质区别
├── go.mod
├── go.sum
└── README.md                             # 本文件（学习路径总览）
```

---

## 学习进度

| 步骤 | 文档 | 实验 | 状态 |
| :--- | :--- | :--- | :--- |
| **1. 核心哲学** | [Spec vs Status 哲学](./docs/01-spec-vs-status-philosophy.md) | - | ✅ 已完成 |
| **2. 底层机制** | [独立 RV 与隔离原理](./docs/02-subresource-mechanism.md) | `cmd/status-subresource/` | ✅ 已完成 |
| **3. Kubebuilder 开发** | [Marker 与代码实现](./docs/03-kubebuilder-implementation.md) | `cmd/status-kubebuilder/` | ✅ 已完成 |
| **4. Scale 子资源** | [集成 HPA](./docs/04-scale-hpa-integration.md) | `cmd/scale-subresource/` | ✅ 已完成 |
| **5. 完整闭环** | - | `cmd/scale-with-deployment/` | ✅ 已完成 |
| **6. RBAC 隔离** | [子资源的 RBAC](./docs/05-subresource-rbac-isolation.md) | `cmd/rbac-subresource/` | ✅ 已完成 |
| **7. 补充知识** | [RV vs Generation](./docs/resourceVersion-vs-generation.md) | - | ✅ 已完成 |
| **8. 补充知识** | [无限循环与 Predicate](./docs/infinite-loop-and-predicates.md) | `cmd/WithEventFilter-predicate/` | ✅ 已完成 |

---

## 核心概念预览

1.  **Spec vs Status**: 意图（用户想要的） vs 观察（现实发生的）。
2.  **独立 RV**: 更新 `/status` 会产生新的 ResourceVersion，但不会让 Spec 的 RV 变旧。
3.  **避免自我循环**: 更新 `/status` 不会触发 Controller 的 Watch，避免 Reconcile 死循环。
4.  **Scale 子资源**: 让 HPA 能读懂并控制自定义资源的 `replicas` 字段。
