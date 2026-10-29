# `types.JSONPatchType` (RFC 6902 JSON Patch)

本章节记录 client-go 中 `types.JSONPatchType` 的原理、精确操作语义及使用场景。

---

## 1. 基本信息

| 项目 | 值 |
| :--- | :--- |
| **HTTP 方法** | `PATCH` |
| **Content-Type** | `application/json-patch+json` |
| **标准** | [RFC 6902](https://datatracker.ietf.org/doc/html/rfc6902) |
| **client-go 定义** | `k8s.io/apimachinery/pkg/types/types.go` 中 `JSONPatchType PatchType = "application/json-patch+json"` |
| **Controller Runtime 对应** | `client.RawPatch(types.JSONPatchType, patchData)` |

---

## 2. 为什么需要 JSONPatch？

### 前面两个 PatchType 的核心缺陷

| PatchType | 缺陷 |
| :--- | :--- |
| **MergePatchType** | 数组全量替换、map key upsert（无法区分 add/replace） |
| **StrategicMergePatchType** | 只支持 K8s 内置资源、删除需要 `$patch: delete`、map key upsert |

### JSONPatch 的解决方案

JSONPatch 不声明"我想要什么状态"，而是声明"**我要执行什么操作**"：

```json
[
  {"op": "add", "path": "/spec/containers/-", "value": {...}},
  {"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.26"},
  {"op": "remove", "path": "/spec/containers/1"}
]
```

**关键差异**：
- 不是"声明式"（Declarative），而是"命令式"（Imperative）
- 每个操作有**精确的语义**：`add`/`remove`/`replace` 分别对应不同的路径存在性要求
- `remove` 操作直接明确，不需要 `$patch: delete` 这种特殊标记
- 可以**组合多个操作**在一个请求中，原子执行

---

## 3. 操作类型

### 3.1 `add` — 添加

```json
{"op": "add", "path": "/spec/containers/-", "value": {"name": "sidecar", "image": "fluentd"}}
```

| 场景 | 行为 |
| :--- | :--- |
| 路径不存在（顶层字段） | ✅ 创建该字段 |
| 路径是 **map 中的 key**（如 `/metadata/annotations/my-key`） | ✅ **upsert**：key 不存在则创建，**已存在则替换**（和 MergePatch 一样） |
| 路径是数组末尾（`/-`） | ✅ 追加到数组 |
| 路径的**父级不存在** | ❌ **报错** |

**重要澄清**：`add` 对 **嵌套 map key**（如 annotation keys）的语义是 **upsert**（创建或替换），**不是**"已存在则报错"。这是 RFC 6902 的明确规定："If the target location specifies an object member that does exist, that member's value is replaced."

**`add` 真正报错的场景**：
- 路径的父级不存在（如 `add` 到 `/spec/nonexistent/child` 当 `/spec/nonexistent` 不存在）
- 路径是一个标量值的子路径（如 `add` 到 `/spec/replicas/foo` 当 `replicas` 是整数）

**那怎么做到"添加但不覆盖"？** JSONPatch 本身对 map key 做不到（`add` 就是 upsert）。需要在 Controller 中先 `Get` 检查，或用 SSA 的字段所有权。

### 3.1.1 `add` vs MergePatch 对比

| 操作 | MergePatch | JSONPatch `add` |
| :--- | :--- | :--- |
| 添加新 key 到 map | ✅ `{"annotations": {"new-key": "value"}}` | ✅ `{"op": "add", "path": "/metadata/annotations/new-key", "value": "value"}` |
| 覆盖已有 key | ✅ 静默覆盖 | ✅ **同样静默覆盖**（upsert） |
| 添加顶层不存在的字段 | ✅ 创建 | ✅ 创建 |

所以对于 map key 操作，`add` 和 MergePatch 语义**基本一致**。JSONPatch 的真正优势在 `remove`、`replace` 和组合操作。

### 3.2 `remove` — 删除

```json
{"op": "remove", "path": "/metadata/annotations/my-key"}
```

| 场景 | 行为 |
| :--- | :--- |
| 路径存在 | ✅ 删除该字段 |
| 路径不存在 | ❌ **报错** |

**关键**：比 StrategicMerge 的 `$patch: delete` 和 MergePatch 的 `null` 更直观。

### 3.3 `replace` — 替换

```json
{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.26"}
```

| 场景 | 行为 |
| :--- | :--- |
| 路径存在 | ✅ 更新该字段 |
| 路径不存在 | ❌ **报错**（防止误操作） |

**关键**：与 `add` 互补 —— `replace` 要求路径必须存在，`add` 不要求。

### 3.4 `test` — 条件检查

```json
{"op": "test", "path": "/metadata/resourceVersion", "value": "12345"}
```

| 场景 | 行为 |
| :--- | :--- |
| 路径值匹配 | ✅ 继续执行后续操作 |
| 路径值不匹配 | ❌ **报错**，整个 Patch 失败 |

**关键**：可以实现乐观锁（Optimistic Concurrency Control），确保只在预期版本上修改。

**典型用法**：确保只在预期版本上修改：

```json
[
  {"op": "test", "path": "/metadata/resourceVersion", "value": "12345"},
  {"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.26"}
]
```

注意：`test` 会先检查 `resourceVersion` 是否为 `12345`，如果不是则整个 Patch 失败，不会执行后面的 `replace`。

### 3.5 `copy` 和 `move` — 复制/移动

| 操作 | 说明 |
| :--- | :--- |
| `copy` | 复制一个值到另一个路径 |
| `move` | 移动一个值（等于 `remove` + `add`） |

这两个操作在 K8s 中较少使用。

---

## 4. 路径语法

### 4.1 JSON Pointer (RFC 6901)

JSONPatch 使用 **JSON Pointer** 语法指定路径：

```
/metadata/annotations/my-key
/spec/containers/0/image
/data/config.yaml
```

**转义规则**：

| 字符 | 转义写法 |
| :--- | :--- |
| `/`（路径中的斜杠） | `~1` |
| `~`（波浪号本身） | `~0` |

**示例**：如果 annotation 的 key 是 `my.com/key`，路径要写成：

```json
{"op": "add", "path": "/metadata/annotations/my.com~1key", "value": "hello"}
```

### 4.2 数组索引

- **数字**：按索引访问（0-based），如 `/spec/containers/0`
- **`-`**：表示数组末尾，追加操作，如 `/spec/containers/-`

---

## 5. 如何使用

### 5.1 client-go 原生调用

```go
package main

import (
    "context"
    "encoding/json"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    "k8s.io/client-go/kubernetes"
)

func main() {
    // ... 初始化 clientset ...

    // 构造 JSONPatch 操作列表
    // 注意：这是一个数组，不是对象！
    patchOps := []map[string]interface{}{
        // 1. 添加 annotation（如果已存在则报错）
        {"op": "add", "path": "/metadata/annotations/owned-by", "value": "my-controller"},
        // 2. 替换第一个容器的 image
        {"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.26"},
        // 3. 删除第二个容器
        {"op": "remove", "path": "/spec/containers/1"},
    }
    patchData, _ := json.Marshal(patchOps)

    // 发送 JSONPatch 请求
    result, err := clientset.AppsV1().Deployments("default").Patch(
        context.Background(),
        "my-deploy",
        types.JSONPatchType,       // ← 关键：指定 JSONPatch
        patchData,
        metav1.PatchOptions{},
    )
}
```

**发送的 HTTP 请求**：

```http
PATCH /apis/apps/v1/namespaces/default/deployments/my-deploy HTTP/1.1
Content-Type: application/json-patch+json

[
  {"op": "add", "path": "/metadata/annotations/owned-by", "value": "my-controller"},
  {"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.26"},
  {"op": "remove", "path": "/spec/containers/1"}
]
```

### 5.2 Controller Runtime 等价写法

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var deploy appsv1.Deployment
    r.Get(ctx, req.NamespacedName, &deploy)

    // 构造 JSONPatch 操作列表
    patchOps := []map[string]interface{}{
        {"op": "add", "path": "/metadata/annotations/last-updated", "value": time.Now().String()},
    }
    patchData, _ := json.Marshal(patchOps)

    // 发送 JSONPatch 请求
    err := r.Client.Patch(ctx, &deploy, client.RawPatch(types.JSONPatchType, patchData))
    // ↑ 底层等价于: clientset.Patch(ctx, name, types.JSONPatchType, patchData, opts)
}
```

**关键**：Controller Runtime 没有像 `client.MergeFrom` 那样专门的 JSONPatch 辅助函数，需要用 `client.RawPatch` 手动序列化操作列表。

---

## 6. `add` vs `replace` 语义对比

这是 JSONPatch 最核心的差异化能力：

| 操作 | 路径不存在 | 路径已存在 |
| :--- | :--- | :--- |
| **`add`** | ✅ 创建 | ❌ **报错**（防止覆盖） |
| **`replace`** | ❌ **报错**（必须存在） | ✅ 更新 |

**使用场景**：

- 如果你想"初始化一个字段，但别覆盖别人的值" → 用 `add`
- 如果你想"更新一个已知存在的字段" → 用 `replace`

---

## 7. 与 `kubectl patch` 的关系

`kubectl patch` 支持 `--type` 参数：

```bash
# MergePatch（默认）
kubectl patch deploy my-deploy -p '{"metadata":{"annotations":{"key":"value"}}}'

# JSONPatch
kubectl patch deploy my-deploy --type json -p '[{"op": "add", "path": "/metadata/annotations/key", "value": "value"}]'

# StrategicMergePatch
kubectl patch deploy my-deploy --type strategic -p '{"spec":{"template":{"spec":{"containers":[{"name":"sidecar","image":"fluentd"}]}}}}'
```

---

## 8. 适用场景

| 场景 | 示例 | 为什么适合 |
| :--- | :--- | :--- |
| **添加 annotation（不覆盖）** | `{"op": "add", "path": "/metadata/annotations/owned-by", ...}` | `add` 已存在时报错，防止多 Controller 冲突 |
| **删除容器/volume** | `{"op": "remove", "path": "/spec/containers/1"}` | 比 `$patch: delete` 更直观 |
| **更新已知字段** | `{"op": "replace", "path": "/spec/containers/0/image", ...}` | `replace` 要求路径存在，防止误操作 |
| **条件更新** | `{"op": "test", "path": "/metadata/resourceVersion", "value": "12345"}` | 实现乐观锁，只在预期版本上修改 |

---

## 9. ❌ 缺陷与限制

### 缺陷 1：路径是硬编码的

**问题**：JSONPatch 的路径是字符串，编译器无法检查。

**影响**：
- 如果路径写错（如 `/spec/contianers/0/image`），K8s 在**运行时**才报错
- 没有 IDE 提示或编译时检查

### 缺陷 2：不知道"当前状态"

**问题**：JSONPatch 是"盲操作" —— 你不知道当前资源的状态，只是执行操作。

**示例**：如果你想"给所有容器加一个 env"，你需要先 `Get` 拿到当前容器列表，然后计算索引，再构造 `add` 操作。

**对比**：MergePatch 和 StrategicMerge 只需要声明"我想要什么"，不需要关心路径计算。

### 缺陷 3：对嵌套数组不友好

**问题**：如果路径中有嵌套数组（如 `containers` 数组），你需要知道**确切的索引**。

```json
{"op": "add", "path": "/spec/containers/0/env/-", "value": {"name": "MY_ENV", "value": "prod"}}
```

如果你不知道 `main-app` 是第几个容器（索引 0 还是 1？），就无法正确构造路径。

**对比**：StrategicMerge 不需要索引，直接按 `name` 匹配。

### 缺陷 4：没有字段所有权

**问题**：与 MergePatch/StrategicMerge 一样，JSONPatch 不记录"谁"修改了"哪个"字段。

**替代方案**：使用 `ApplyPatchType` (SSA)。

---

## 10. 实验代码结构

每个实验是一个独立的代码目录，一个目录只验证一个概念：

```
05-Patching/cmd/client-go/
├── json-add-map-key/                 # JSONPatch: add 操作（map key 语义）
├── json-replace-field/               # JSONPatch: replace 操作（路径必须存在）
├── json-remove-annotation/           # JSONPatch: remove 操作（精确删除）
├── json-array-operations/            # JSONPatch: 数组 add/replace/remove（按索引）
└── json-combined-ops/                # JSONPatch: 组合多个操作（原子执行）
```

### 10.1 `json-add-map-key/` — add 操作对 map key 的语义

**验证什么**：JSONPatch `add` 对嵌套 map key（如 annotation keys）的语义是 **upsert**（创建或替换），**不是**"已存在则报错"。

**场景**：
1. 添加一个不存在的 annotation → ✅ 成功创建
2. 再次 `add` 同一个 annotation → ✅ 成功**替换**（不是报错）

**核心结论**：
- RFC 6902 第 4.1 节规定："If the target location specifies an object member that does exist, that member's value is **replaced**."
- JSONPatch `add` 对 map key 和 MergePatch 语义**一致**，都是 upsert
- `add` 真正报错的场景：路径的父级不存在、路径是标量值的子路径

---

### 10.2 `json-replace-field/` — replace 操作要求路径存在

**验证什么**：JSONPatch `replace` 要求路径必须存在，不存在时报错。

**场景**：
1. 替换已存在的 annotation → ✅ 成功更新
2. `replace` 一个不存在的 annotation → ❌ 报错（路径不存在）

**核心结论**：
- `replace` 与 `add` 互补：`add` 不要求路径存在，`replace` 要求
- 防止误操作（如打错路径时不会创建奇怪的字段）

---

### 10.3 `json-remove-annotation/` — remove 操作精确删除

**验证什么**：JSONPatch `remove` 操作的删除语义，比 StrategicMerge 的 `$patch: delete` 和 MergePatch 的 `null` 更直观。

**场景**：
1. 删除已存在的 annotation → ✅ 成功删除
2. 删除不存在的 annotation → ❌ 报错（路径不存在）

**核心结论**：
- `remove` 操作直接明确，不需要特殊语法或 null 值
- 路径不存在时报错，防止误删

---

### 10.4 `json-array-operations/` — 数组的 add/replace/remove（按索引）

**验证什么**：JSONPatch 如何通过索引操作数组元素。

**场景**：
1. `add` 到 `/-` → 追加容器到数组末尾
2. `replace` `/spec/containers/0/image` → 替换第一个容器的 image
3. `remove` `/spec/containers/1` → 删除第二个容器

**核心结论**：
- `/-` 语法追加到数组末尾（不需要知道索引）
- 按索引精确操作（`/0`, `/1`）
- **限制**：需要知道确切的数组索引，不如 StrategicMerge 按 `name` 匹配直观

---

### 10.5 `json-combined-ops/` — 组合多个操作（原子执行）

**验证什么**：JSONPatch 可以组合多个操作在一个请求中，原子执行。

**场景**：
- 同时执行：`add` annotation + `replace` image + `remove` annotation

**核心结论**：
- 任意一个操作失败，整个 Patch 都会失败
- 适合需要保证多个操作一致性的场景

---

## 11. 四种 PatchType 对比

| 维度 | MergePatch | StrategicMerge | JSONPatch | SSA (Apply) |
| :--- | :--- | :--- | :--- | :--- |
| **标准** | RFC 7396 | K8s 自定义 | RFC 6902 | K8s 自定义 |
| **语义** | 声明式三路合并 | 声明式智能合并 | 命令式操作列表 | 声明式字段所有权 |
| **数组处理** | 全量替换 | 按 key 智能合并 | 按索引精确操作 | 按 key 智能合并 + 字段所有权 |
| **map key 操作** | upsert | upsert | `add`=upsert, `replace`=必须存在 | 按 fieldManager 区分 |
| **删除** | 需要 `null` | 需要 `$patch: delete` | ✅ `remove` 操作 | 需要 SSA 专用语法 |
| **CRD 支持** | ✅ | ❌ | ✅ | ✅ |
| **多 Controller 安全** | ❌ | ⚠️ 仅内置资源 | ⚠️（操作原子性） | ✅ 最佳 |
| **复杂度** | 低 | 中 | 中 | 高 |

---

## 12. 决策树：何时该用 JSONPatch？

```text
你需要精确删除数组元素吗？
    │
    ├── 是 → ✅ 用 JSONPatchType（remove 操作更直观）
    │
    └── 否 → 你要修改的是 K8s 内置资源的数组字段吗？
                │
                ├── 是 → ✅ 用 StrategicMergePatchType（智能合并）
                │
                └── 否 → 你是多 Controller 协作更新 CRD 吗？
                            │
                            ├── 是 → ✅ 用 ApplyPatchType (SSA)
                            │
                            └── 否 → ✅ 用 MergePatchType（最简单）
```

---

## 13. 总结

| 维度 | JSONPatchType |
| :--- | :--- |
| **HTTP Content-Type** | `application/json-patch+json` |
| **合并策略** | 命令式操作列表（add/remove/replace/test） |
| **核心优势** | `remove` 操作直观、`replace` 要求路径存在、组合操作原子执行 |
| **适合** | 需要精确操作语义的场景、精确删除、按索引修改数组元素 |
| **不适合** | 嵌套数组修改（需要计算索引）、不知道当前状态时、CRD 多 Controller 协作 |
| **Controller Runtime** | `client.RawPatch(types.JSONPatchType, patchData)` |
| **实验代码** | `cmd/client-go/json-add-map-key/`, `cmd/client-go/json-replace-field/`, `cmd/client-go/json-remove-annotation/`, `cmd/client-go/json-array-operations/`, `cmd/client-go/json-combined-ops/` |

---

*前一篇：[StrategicMergePatchType](./strategic-patch-principles.md)（K8s 内置智能合并）*
*下一篇：[ApplyPatchType/SSA](./ssa.md)（CRD 友好的字段所有权，多 Controller 协作）*
