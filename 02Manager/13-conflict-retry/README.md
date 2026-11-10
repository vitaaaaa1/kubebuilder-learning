# 13-conflict-retry · 冲突（409）与 RetryOnConflict

## 这个章节在解决什么问题

> **两个地方同时改同一个对象，后写的那个会收到 409 Conflict。**
> 要么放弃，要么重新读取后再写——`RetryOnConflict` 就是后者。

## 一、冲突从哪来：ResourceVersion

每个对象都带一个 `metadata.resourceVersion`：

```yaml
metadata:
  name: demo-conflict
  resourceVersion: "12345"
```

`Update` 时，客户端把**读到的那个版本号**一起发出去。服务端对比：

```text
请求里的 RV == 当前 RV   → 接受，RV 自增
请求里的 RV != 当前 RV   → 409 Conflict
```

```text
A 读（RV=100）           B 读（RV=100）
A 写（RV=100）→ 成功      RV 变成 101
                         B 写（RV=100）→ 409 ❌
```

**这就是乐观锁：不提前加锁，写的时候才发现冲突。**

## 二、实测对照

同一个计数器，20 个 goroutine 同时自增一次：

**版本 A：`naive`（冲突直接失败）**

```text
并发数=20  成功=1  冲突=19  其他=0  最终计数=1
```

**19 次自增丢了。**

**版本 B：`with-retry`（冲突后重读重写）**

```text
并发数=20  成功=20  发生冲突(已重试)=170  失败=0  最终计数=20
```

**一次没丢，最终计数正好 20。**

## 三、冲突错误长什么样

```text
err = Operation cannot be fulfilled on configmaps "demo-conflict":
      the object has been modified; please apply your changes to the latest version and try again
IsConflict = true
```

判断方式：

```go
if apierrors.IsConflict(err) { ... }
```

## 四、`RetryOnConflict` 的语义

```go
retry.RetryOnConflict(retry.DefaultRetry, func() error {
	// ① 重新 Get（关键！）
	if err := c.Get(ctx, key, obj); err != nil {
		return err
	}
	// ② 修改
	obj.Data["count"] = ...
	// ③ 写回
	return c.Update(ctx, obj)
})
```

源码（client-go）：

```go
func RetryOnConflict(backoff wait.Backoff, fn func() error) error {
	return OnError(backoff, errors.IsConflict, fn)
}
```

```text
fn 返回 nil            → 成功，退出
fn 返回 Conflict       → 等待后重试整个 fn
fn 返回其他错误         → 立即退出
```

**重试的是整个回调，不是 `Update` 这一行。**

### 默认节奏

```go
var DefaultRetry = wait.Backoff{
	Steps:    5,                 // 最多 5 次
	Duration: 10 * time.Millisecond,
	Factor:   1.0,               // 不递增
	Jitter:   0.1,
}
```

**5 次、每次约 10ms。** 并发很高时可以自定义更多次数（本实验用了 100 次）。

## 五、为什么“只重试 Update”不行

```go
// ❌ 错误示范
err := c.Update(ctx, obj)
if apierrors.IsConflict(err) {
	return c.Update(ctx, obj)   // 对象还是旧 RV，再写还是冲突
}
```

**冲突的根源是对象里的旧 `resourceVersion`，不重新 `Get` 就永远冲突。**

## 六、在 Controller 里的标准写法

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, req.NamespacedName, cm); err != nil {
			return err
		}
		cm.Data["count"] = bump(cm.Data["count"])
		return r.Update(ctx, cm)
	})
	return ctrl.Result{}, err
}
```

### 一个容易踩的坑

`r.Get` 默认走**缓存**。缓存可能还没收到最新版本，于是：

```text
重试时又读到旧 RV → 继续冲突 → 5 次耗尽
```

高并发场景下，重试循环里的读取应该用 `APIReader`：

```go
r.APIReader.Get(ctx, req.NamespacedName, cm)   // 直连 apiserver
```

（`APIReader` 在 [06-manager-anatomy/README.md](../06-manager-anatomy/README.md) 里验证过：读到的是全集群实时数据。）

## 七、Patch 为什么没有这个问题

| 方式 | 带 RV 校验 | 冲突 | 风险 |
|---|---|---|---|
| `Update` | ✅ | 会 409 | 不重试就丢更新 |
| `Update` + `RetryOnConflict` | ✅ | 会 409，但重试 | 无（最终一致） |
| `Patch`（merge） | ❌ | 不冲突 | 可能覆盖别人的改动 |
| `Create` | — | 已存在则 409 | — |

**Patch 只发送你改的字段，不做乐观锁**，所以：
- 适合“我只关心这个字段”
- 不适合“基于当前值做计算”（比如计数器），因为读到的值可能已过期

## 八、三种处理策略

```text
① 快速失败        返回错误，交给上层重试（controller 的队列会自动重试）
② RetryOnConflict 当场重读重写，最多 N 次
③ Patch           根本不检查版本，只改局部
```

**在 Reconcile 里，① 通常就够了**——workqueue 本身就有退避重试（见第 12 章）。
`RetryOnConflict` 更适合“一次调用内必须成功”的场景。

## 九、运行

```bash
cd 02Manager/13-conflict-retry

go run ./naive -workers 20        # 期望：成功 1，冲突 19
go run ./with-retry -workers 20   # 期望：成功 20，最终计数 20
```

两个程序都会先把 `test/demo-conflict` 的计数归零。

## 十、一句话总结

> **冲突是 `resourceVersion` 乐观锁的必然结果；`RetryOnConflict` 的全部秘密就是“冲突后重新 Get，再改一遍”。**