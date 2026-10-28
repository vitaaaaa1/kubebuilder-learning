# 05 Patching: 资源更新策略

本专题记录 Kubernetes 中如何高效、安全地更新资源，以及不同 Patching 策略的适用场景。

---

## 1. 已学习的 PatchType

### MergePatchType (RFC 7396)
- **目录**: `cmd/patching-demo-merge/`
- **文档**: `docs/patch/client-go/merge-patch.md`
- **核心概念**: JSON Merge Patch，三路合并，最常用。
- **HTTP Content-Type**: `application/merge-patch+json`
- **缺陷验证**: `cmd/patching-demo-merge-limitations/`（数组全量替换问题）

### StrategicMergePatchType (K8s Native)
- **目录**: `cmd/patching-demo-strategic/`
- **文档**: `docs/patch/client-go/strategic-patch.md`
- **核心概念**: 理解 `+patchStrategy=merge`，智能合并数组（containers、volumes）。
- **HTTP Content-Type**: `application/strategic-merge-patch+json`

### JSONPatchType (RFC 6902)
- **目录**: `cmd/json-add-map-key/`, `cmd/json-replace-field/`, `cmd/json-remove-annotation/`, `cmd/json-array-operations/`, `cmd/json-combined-ops/`
- **文档**: `docs/patch/client-go/json-patch.md`
- **核心概念**: 命令式操作列表（add/remove/replace/test），精确操作语义。
- **HTTP Content-Type**: `application/json-patch+json`

### ApplyPatchType (Server-Side Apply, SSA)
- **目录**: `cmd/ssa-basic/`, `cmd/ssa-field-ownership/`, `cmd/ssa-crd-support/`
- **文档**: `docs/patch/client-go/ssa.md`
- **核心概念**: 字段所有权模型（`fieldManager` + `managedFields`），多 Controller 安全协作。
- **HTTP Content-Type**: `application/apply-patch+yaml`

---

## 2. 计划学习的 PatchType

已全部学习完成 ✅

---

## 实验目录结构（按 PatchType 隔离）

```
05-Patching/
├── cmd/
│   ├── conflict-demo-01/                 # Update 409 Conflict 实验
│   ├── conflict-demo-01-retry/           # RetryOnConflict 重试实验
│   ├── patching-demo/                    # Patch 基础实验
│   ├── patching-demo-merge/              # MergePatchType 实验
│   ├── patching-demo-merge-limitations/  # MergePatch 数组缺陷验证
│   ├── patching-demo-strategic/          # StrategicMergePatchType 实验
│   ├── json-add-map-key/                 # JSONPatch: add 操作（map key upsert 语义）
│   ├── json-replace-field/               # JSONPatch: replace 操作（路径必须存在）
│   ├── json-remove-annotation/           # JSONPatch: remove 操作（精确删除）
│   ├── json-array-operations/            # JSONPatch: 数组 add/replace/remove（按索引）
│   ├── json-combined-ops/                # JSONPatch: 组合多个操作（原子执行）
│   ├── ssa-basic/                        # SSA: 基础用法，字段所有权记录
│   ├── ssa-field-ownership/              # SSA: 多 fieldManager 协作，字段隔离
│   └── ssa-crd-support/                  # SSA: 对 CRD 有效（StrategicMerge 做不到）
├── docs/
│   └── patch/
│       └── client-go/
│           ├── request-lifecycle.md      # 请求生命周期全流程
│           ├── merge-patch.md            # MergePatchType 理论+缺陷
│           ├── strategic-patch.md        # StrategicMergePatchType 理论
│           ├── strategic-patch-principles.md # StrategicMerge 底层原理+缺陷
│           ├── json-patch.md             # JSONPatchType 理论+操作语义
│           └── ssa.md                    # ApplyPatchType/SSA 理论+字段所有权
└── README.md                             # 本文件（专题总览）
```
