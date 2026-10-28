# 06 Subresources: 子资源

本专题记录 Kubernetes 中 Subresources（子资源）的机制、使用场景及最佳实践。

---

## 1. 已学习的 Subresource

### /status 子资源
- **目录**: `cmd/status-subresource/`
- **文档**: `docs/status-subresource.md`
- **核心概念**: 状态与规格分离更新，`r.Status().Update/Patch` 不触发 Reconcile 循环。
- **Marker**: `// +kubebuilder:subresource:status`
- **关键点**: 更新 `/status` 不会修改 `spec`，不会影响其他 Controller 管理的字段。

### /scale 子资源
- **目录**: `cmd/scale-subresource/`
- **文档**: `docs/scale-subresource.md`
- **核心概念**: HPA 集成基础，绕过 `spec.replicas` 所有权检查。
- **API**: `r.SubResource("scale").Update(...)`
- **关键点**: `/scale` 子资源有特殊的 SSA 所有权处理，HPA 原生使用此接口。

---

## 2. 计划学习的 Subresource

| 主题 | 状态 |
|------|------|
| 自定义子资源生成 (`+kubebuilder:subresource`) | ⏳ 计划中 |
| `/status` 与 SSA 的交互 | ⏳ 计划中 |
| `/scale` 与 HPA 集成实验 | ⏳ 计划中 |

---

## 实验目录结构（按 Subresource 类型隔离）

```
06-Subresources/
├── cmd/
│   ├── status-subresource/               # /status 子资源实验
│   ├── scale-subresource/                # /scale 子资源实验
│   └── ...
├── docs/
│   ├── status-subresource.md             # /status 理论+实践
│   ├── scale-subresource.md              # /scale 理论+HPA 集成
│   └── ...
├── go.mod
├── go.sum
└── README.md                             # 本文件（专题总览）
```
