# DeepCopy FAQ: 为什么 MergeFrom 必须手动 DeepCopy？

## 问题

在 controller-runtime 中使用 `client.MergeFrom(original)` 时，**官方并没有提供类似 `MergeFromWithDeepCopy` 的封装**。

你必须显式地手动调用：
```go
original := obj.DeepCopy() // 必须手动！
err := r.Patch(ctx, &obj, client.MergeFrom(original))
```

这也是社区中被吐槽最多的设计之一。

---

## 为什么官方不封装（自动调用 DeepCopy）？

### 1. Go 语言哲学：显式优于隐式 (Explicit is better than implicit)
Go 的设计者不希望框架帮你隐藏昂贵的操作。`DeepCopy` 是一个涉及递归遍历和内存分配的重操作，框架不应该替你“悄悄”做这件事。

### 2. 性能不可控
如果你更新的是一个巨型资源（比如一个包含了海量配置数据的巨大 ConfigMap 或 CRD），隐式 DeepCopy 会消耗大量的 CPU 和内存。
如果框架自动帮你做了，你在遇到性能瓶颈时很难意识到是这一行代码导致的。

### 3. 意图不明确
框架不知道你传入的 `original` 变量到底代表什么：
*   它是你想用来对比的 **“基准版本” (Baseline)**？
*   还是你想修改的 **“目标对象” (Target)**？

如果框架自动做了 DeepCopy，你就失去了“使用不同版本对象进行 Diff”的灵活性。

**结论：** CR 把决定权交给了你。你想对比哪个版本，必须显式传给我。

---

## 替代方案：SSA (Apply)

如果你厌倦了每次都手动写 `DeepCopy()`，**SSA (`client.Apply`)** 是最完美的解脱。

| 方法 | 需要 DeepCopy 吗？ | 说明 |
| :--- | :--- | :--- |
| **MergeFrom** | ✅ **必须** | `original := obj.DeepCopy()` |
| **Update** | ❌ 不需要 | 但容易报 409 Conflict，且覆盖风险大 |
| **SSA (Apply)** | ❌ **不需要** | **推荐**。直接构造新对象声明意图 |

**SSA 为什么不需要？**
因为 SSA 不需要对比“旧对象”。它只关心你“构造了什么新对象”，然后直接发往 API Server 进行字段合并。

```go
// ✅ SSA 的快乐
objApply := ConfigMapApply("name", "ns").WithData(...) // 直接构造新意图
c.Apply(ctx, objApply, client.FieldOwner("my-op"))     // 不需要 DeepCopy！
```
