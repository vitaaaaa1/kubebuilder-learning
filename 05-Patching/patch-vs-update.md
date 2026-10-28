# Patch vs Update 核心差异

本章节记录 Kubernetes 中 `Patch` 与 `Update` 两种更新策略的本质区别，以及在不同场景下的选择。

---

## 1. 核心概念对比

| 维度 | `Update()` | `Patch()` |
| :--- | :--- | :--- |
| **HTTP 方法** | `PUT` | `PATCH` |
| **语义** | 全量替换 | 部分更新 |
| **请求体** | 整个资源的完整 JSON | 只包含变更的差异部分 |
| **ResourceVersion** | **必须精确匹配**，否则报 409 Conflict | **不需要**（取决于策略，Merge Patch 不检查） |
| **冲突风险** | 高（其他人改了任何字段都会冲突） | 低（只操作声明的字段） |
| **网络开销** | 大（发送完整 JSON） | 小（只发差异） |
| **心智负担** | 低（改了直接发） | 需要保存 `original` 副本（MergeFrom/Strategic） |

---

## 2. 并发冲突实验

### 场景：两个修改者同时改同一 ConfigMap 的不同字段

```text
初始: RV = 14052754113
    │
    ├── 修改者 A 和 B 都拿到了 RV=14052754113
    │
    ├── B 先 Update 成功 → RV=14052754366
    │
    └── A 还拿着 RV=14052754113 去 Update
         → ❌ 409 Conflict
```

### Update 失败原因

`Update()` 是 **全量替换** 语义。API Server 收到请求后：

1. 检查请求体中的 `ResourceVersion` 是否匹配服务器当前版本
2. 如果匹配 → 全量替换
3. 如果不匹配 → 返回 409 Conflict

```text
API Server 的视角：
"你拿着 RV=14052754113 来替换，但当前已经是 14052754366 了。
 说明在你 Get 之后有人改过它。你的数据可能不是最新的，
 全量替换会覆盖别人的修改。拒绝！"
```

### Patch 成功原因

`Patch()` 是 **部分更新** 语义。API Server 收到请求后：

1. 提取请求体中声明的字段
2. 只更新这些字段，保留其他字段不变
3. 不涉及全量替换，不严格检查 `ResourceVersion`

```text
API Server 的视角：
"你只说要改 metadata.annotations.modified-by。
 好，我只动这个字段，其他字段不动。
 即使有人改过别的，你的操作也不会影响他们。"
```

---

## 3. 代码对比

### Update 写法（需要 Get + 重试）

```go
// 1. 必须先 Get 到完整对象
obj, _ := clientset.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})

// 2. 修改字段
obj.Annotations["key"] = "value"

// 3. 尝试 Update
_, err := clientset.CoreV1().ConfigMaps(ns).Update(ctx, obj, metav1.UpdateOptions{})
if err != nil {
    // 4. 如果 409，必须先 Get 最新，再重试
    // retry.RetryOnConflict(...) {
    //     latest := Get(...)   ← 关键：每次重试都要 Get 最新！
    //     latest.Annotations["key"] = "value"
    //     Update(latest)
    // }
}
```

### Patch 写法（不需要 Get）

```go
// 1. 构造差异数据（只包含要修改的字段）
patchData, _ := json.Marshal(map[string]interface{}{
    "metadata": map[string]interface{}{
        "annotations": map[string]string{
            "modified-by": "A",
        },
    },
})

// 2. 直接 Patch（不需要先 Get）
result, err := clientset.CoreV1().ConfigMaps(ns).Patch(
    ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{},
)
```

---

## 4. ResourceVersion 的本质

`ResourceVersion` 是 **etcd 的全局事务 ID**，由 API Server 自动分配，**永远单调递增**，用户无法控制或回退。

```text
etcd 内部逻辑：
┌──────────────────────────────────────┐
│  每次集群内任何资源发生写操作        │
│  etcd 的全局 Revision + 1            │
│  这个 Revision 就是 ResourceVersion  │
└──────────────────────────────────────┘
```

### 为什么 Update 会无限 409？

```go
retry.RetryOnConflict(retry.DefaultRetry, func() error {
    // ❌ 错误：注释掉了 Get
    // latest := Get(...)

    // obj 的 RV 一直是旧的
    obj.Annotations["key"] = "value"
    _, err := Update(obj)  // ← 永远 409，无限循环
    return err
})
```

**修复**：每次重试必须先 `Get` 最新版本。

---

## 5. 何时用哪个？

| 场景 | 推荐 | 原因 |
| :--- | :--- | :--- |
| **生产环境 / 多 Controller** | `Patch()` | 降低 409 Conflict 率，减少重试 |
| **更新 CRD Status** | `r.Status().Patch()` | 只碰 `/status`，不影响 Spec |
| **多 Operator 协作** | `Patch(Apply)` | SSA 的 Field Ownership 避免冲突 |
| **智能合并列表** | `Patch(StrategicMergeFrom)` | K8s 知道如何合并 Containers 等数组 |
| **学习 / 实验 / 简单脚本** | `Update()` | 代码最简短，心智负担小 |
| **单 Controller 独占资源** | `Update()` | 冲突概率低 |

---

## 6. 总结

| | `Update()` | `Patch()` |
| :--- | :--- | :--- |
| **语义** | "我要把这个资源变成我手里这个样子" | "我只想改这几个字段，其他的别动" |
| **适用** | 简单、独占场景 | 生产、并发场景 |
| **默认推荐** | ❌ | ✅ **默认选择** |

**一句话**：在 Controller 中，**默认用 Patch**，遇到特殊场景再考虑 Update。
