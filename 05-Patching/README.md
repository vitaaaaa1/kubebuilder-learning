# 05 Patching: 资源更新策略

本专题记录 Kubernetes 中如何高效、安全地更新资源，以及不同 Patching 策略的适用场景。

---

## 1. 已学习的 PatchType

### MergePatchType (RFC 7396)
- **目录**: `cmd/client-go/patching-demo-merge/`
- **文档**: `docs/patch/client-go/merge-patch.md`
- **核心概念**: JSON Merge Patch，三路合并，最常用。
- **HTTP Content-Type**: `application/merge-patch+json`
- **缺陷验证**: `cmd/client-go/patching-demo-merge-limitations/`（数组全量替换问题）

### StrategicMergePatchType (K8s Native)
- **目录**: `cmd/client-go/patching-demo-strategic/`
- **文档**: `docs/patch/client-go/strategic-patch.md`
- **核心概念**: 理解 `+patchStrategy=merge`，智能合并数组（containers、volumes）。
- **HTTP Content-Type**: `application/strategic-merge-patch+json`

### JSONPatchType (RFC 6902)
- **目录**: `cmd/client-go/json-add-map-key/`, `cmd/client-go/json-replace-field/`, `cmd/client-go/json-remove-annotation/`, `cmd/client-go/json-array-operations/`, `cmd/client-go/json-combined-ops/`
- **文档**: `docs/patch/client-go/json-patch.md`
- **核心概念**: 命令式操作列表（add/remove/replace/test），精确操作语义。
- **HTTP Content-Type**: `application/json-patch+json`

### ApplyPatchType (Server-Side Apply, SSA)
- **目录**: `cmd/client-go/ssa-basic/`, `cmd/client-go/ssa-field-ownership/`, `cmd/client-go/ssa-crd-support/`
- **文档**: `docs/patch/client-go/ssa.md`
- **核心概念**: 字段所有权模型（`fieldManager` + `managedFields`），多 Controller 安全协作。
- **HTTP Content-Type**: `application/apply-patch+yaml`

---

## 2. controller-runtime Patch API（学习中）

### client.MergeFrom ✅
- **目录**: `cmd/controller-runtime/merge-from-demo/`
- **文档**: `docs/patch/controller-runtime/merge-from.md`
- **核心概念**: 三路合并 diff 计算，`original := obj.DeepCopy()` → 修改 → `r.Patch(ctx, &obj, client.MergeFrom(original))`
- **底层 client-go**: `types.MergePatchType`（RFC 7396）
- **缺陷验证**: 数组全量替换（同 client-go MergePatch）

---

## 3. 计划学习的 controller-runtime PatchType

| 主题 | 状态 |
|------|------|
| `client.StrategicMergeFrom` | ⏳ 计划中 |
| `client.RawPatch` | ⏳ 计划中 |
| `client.Apply()` (SSA) | ⏳ 计划中 |
| `client.Status().Patch()` | ⏳ 计划中 |

---

## 实验目录结构（按 PatchType 隔离）

```
05-Patching/
├── cmd/
│   ├── client-go/                          # client-go 原生 Patch 实验
│   │   ├── conflict-demo-01/                 # Update 409 Conflict 实验
│   │   ├── conflict-demo-01-retry/           # RetryOnConflict 重试实验
│   │   ├── patching-demo/                    # Patch 基础实验
│   │   ├── patching-demo-merge/              # MergePatchType 实验
│   │   ├── patching-demo-merge-limitations/  # MergePatch 数组缺陷验证
│   │   ├── patching-demo-strategic/          # StrategicMergePatchType 实验
│   │   ├── json-add-map-key/                 # JSONPatch: add 操作（map key upsert 语义）
│   │   ├── json-replace-field/               # JSONPatch: replace 操作（路径必须存在）
│   │   ├── json-remove-annotation/           # JSONPatch: remove 操作（精确删除）
│   │   ├── json-array-operations/            # JSONPatch: 数组 add/replace/remove（按索引）
│   │   ├── json-combined-ops/                # JSONPatch: 组合多个操作（原子执行）
│   │   ├── ssa-basic/                        # SSA: 基础用法，字段所有权记录
│   │   ├── ssa-field-ownership/              # SSA: 多 fieldManager 协作，字段隔离
│   │   ├── ssa-crd-support/                  # SSA: 对 CRD 有效（StrategicMerge 做不到）
│   │   ├── ssa-force-takeover/               # SSA: ForceOwnership 强制接管
│   │   ├── ssa-ownership-conflict/           # SSA: 字段冲突演示
│   │   ├── ssa-smart-takeover/               # SSA: 智能判断是否 Force（反模式）
│   │   ├── ssa-takeover-replicas/            # SSA: 接管 replicas 字段
│   │   ├── ssa-defensive-patch/              # SSA: 防御性 Patch 实验
│   │   ├── ssa-safe-injection/               # SSA: 安全注入 Sidecar
│   │   └── ssa-smart-takeover/               # SSA: 智能判断（反模式演示）
│   └── controller-runtime/                 # controller-runtime 封装 Patch 实验
│       ├── go.mod                          # Go 模块配置
│       └── merge-from-demo/                # MergeFrom diff 计算验证
├── docs/
│   └── patch/
│       ├── client-go/                        # client-go 原生 Patch 文档
│       │   ├── request-lifecycle.md          # 请求生命周期全流程
│       │   ├── merge-patch.md                # MergePatchType 理论+缺陷
│       │   ├── strategic-patch.md            # StrategicMergePatchType 理论
│       │   ├── strategic-patch-principles.md # StrategicMerge 底层原理+缺陷
│       │   ├── json-patch.md                 # JSONPatchType 理论+操作语义
│       │   └── ssa.md                        # ApplyPatchType/SSA 理论+字段所有权
│       └── controller-runtime/               # controller-runtime 封装文档
│           └── merge-from.md                 # MergeFrom 理论+最佳实践
└── README.md                             # 本文件（专题总览）
```
