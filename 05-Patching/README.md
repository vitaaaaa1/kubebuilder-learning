# 05 Patching: 资源更新策略

本专题记录 Kubernetes 中如何高效、安全地更新资源，以及不同 Patching 策略的适用场景。

---

## 计划学习内容

### 1. Patching 策略对比

| 策略 | API | 特点 | 适用场景 |
| :--- | :--- | :--- | :--- |
| **MergeFrom** | `client.MergeFrom(original)` | 三路合并（Three-Way Merge） | 最常用，适合大多数更新场景 |
| **StrategicMergeFrom** | `client.StrategicMergeFrom(original)` | K8s 原生合并策略，支持 `+patchStrategy` | 需要智能合并列表字段（如 Containers） |
| **JSONPatch** | `client.RawPatch(...)` | 精确的 JSON Patch 操作（RFC 6902） | 需要原子性操作、细粒度控制 |
| **Server-Side Apply** | `client.Apply` | K8s 1.18+ 推荐，Field Ownership 模型 | 多 Controller 协作，避免冲突 |

### 2. 核心问题

-   `r.Update()` vs `r.Patch()` 的区别？
-   何时使用 `r.Status().Patch()`？
-   Field Owner 冲突如何处理？
-   什么是三路合并（Three-Way Merge）？

---

## 实验目录结构（待填充）

```
05-Patching/
├── cmd/
│   ├── mergefrom-demo/       # MergeFrom 实验
│   ├── strategic-demo/       # StrategicMergeFrom 实验
│   └── ssa-demo/             # Server-Side Apply 实验
├── patching.md               # Patching 理论文档（待创建）
└── README.md                 # 本文件（专题总览）
```
