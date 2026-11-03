# 01 · 三行初始化：把 fake client 的语法讲透

> 对应代码：`cmd/get-branch/reconciler_test.go` 中的 `TestReconcile_IgnoresNotFound`

## 这段代码要解决什么

```go
c := fake.NewClientBuilder().
	WithScheme(clientgoscheme.Scheme).
	Build()

r := &Reconciler{Client: c}
```

一句话：**造一个"内存版 K8s"，塞给你的控制器，让它以为自己在跟集群说话。**

没有网络、没有 etcd、没有 kube-apiserver。`r.Get()` 查的是一张 map。

---

## 全景：一次测试的三段式

```go
func TestReconcile_IgnoresNotFound(t *testing.T) {
	// ① 准备世界（模板代码，本文件讲的就是它）
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
	r := &Reconciler{Client: c}

	// ② 摇一下（每个测试都不同）
	_, err := r.Reconcile(ctx, ctrl.Request{...})

	// ③ 检查世界（每个测试都不同）
	if err != nil { t.Fatalf(...) }
}
```

**① 是固定的，②③ 才是测试的价值。** 所以这一篇只讲 ①。

---

## 逐行拆解

### ① `fake.NewClientBuilder()`

```go
func NewClientBuilder() *ClientBuilder {
	return &ClientBuilder{}
}
```

| 语法点 | 说明 |
|---|---|
| `fake` | 包名，来自 `sigs.k8s.io/controller-runtime/pkg/client/fake` |
| `NewClientBuilder` | 包级函数，**大写开头表示可被其他包调用** |
| 返回值 `*ClientBuilder` | 指针。后续所有配置都改这个对象 |
| `&ClientBuilder{}` | 空结构体取地址 = "还没配置的 builder" |

到此为止，它**还不是**一个客户端，只是一个"配置收集器"。

### ② `.WithScheme(clientgoscheme.Scheme)`

```go
func (f *ClientBuilder) WithScheme(scheme *runtime.Scheme) *ClientBuilder {
	f.scheme = scheme
	return f            // ← 关键：返回自己
}
```

- **参数类型** `*runtime.Scheme`：一张"Go 类型 ↔ GVK"的注册表。
- **返回值** 又是 `*ClientBuilder`，而且**就是它自己**。

实测（打印指针比较）证明没有新对象产生：

```text
b.WithScheme(...)        -> *fake.ClientBuilder
b2 和 b 是同一个指针吗    -> true
```

### ③ `.Build()`

```go
func (f *ClientBuilder) Build() client.WithWatch { ... }
```

| 语法点 | 说明 |
|---|---|
| 返回类型是**接口** `client.WithWatch` | 不是具体类型，所以你摸不到内部字段 |
| 实际对象是 `*fake.fakeClient` | 小写开头 = **未导出**，只能在包里用 |
| `client.WithWatch` 内嵌 `client.Client` | 所以它"包含"了 Get/List/Create/Update/Patch… 全部方法 |

这就是为什么它能塞进 `Reconciler.Client`（字段类型是 `client.Client`）：**接口赋值只看方法集**，只要实现了就兼容。

---

## 为什么可以一直"点点点"？

这叫 **Builder 模式**。等价写法：

```go
c1 := fake.NewClientBuilder().WithScheme(s).WithObjects(a, b).Build()

// 完全等价：
c2 := fake.NewClientBuilder()
c2.WithScheme(s)      // 返回值扔掉也无所谓，因为改的是 c2 自己
c2.WithObjects(a, b)
c3 := c2.Build()
```

为什么 K8s 生态爱用：可选参数太多（scheme、初始对象、拦截器、索引…）。用函数参数会写成一长串 `nil`，用 builder 就能"要什么写什么"。

---

## `r := &Reconciler{Client: c}` 的三个语法点

```go
r := &Reconciler{Client: c}
```

**① 复合字面量**：`Type{字段: 值}`。

**② `Client` 这个字段名哪来的？**

```go
type Reconciler struct {
	client.Client      // 匿名字段（嵌入接口）
}
```

匿名字段的**字段名 = 类型名**，`client.Client` 的字段名就是 `Client`。

**③ 为什么必须要 `&`？**

```go
func (r *Reconciler) Reconcile(...)   // 指针接收者
```

方法定义在 `*Reconciler` 上，所以你必须给一个指针。写 `Reconciler{Client: c}` 会编译报错：

```text
cannot use r (variable of type Reconciler) as ... Reconcile method has pointer receiver
```

**顺带解释一个看起来像魔法的事**：为什么 `r.Get(ctx, ...)` 能调？因为嵌入的 `client.Client` 的方法被**提升（promotion）**成了 `Reconciler` 的方法。`r.Get` 就是 `r.Client.Get`。

---

## 剩下的四行

```go
_, err := r.Reconcile(
	context.Background(),
	ctrl.Request{NamespacedName: types.NamespacedName{
		Name: "definitely-not-exist", Namespace: "default",
	}},
)

if err != nil { t.Fatalf("...: %v", err) }
```

| 语法点 | 说明 |
|---|---|
| `_, err := f()` | 函数返回两个值；`_` 是空白标识符，表示"第一个返回值我不要" |
| `:=` | 声明并赋值，类型自动推断 |
| `types.NamespacedName{...}` | 只有 Namespace + Name 两个字段，K8s 里到处用它定位对象 |
| `ctrl.Request{NamespacedName: ...}` | Reconcile 的入参，本质是"哪个对象出事了" |
| `context.Background()` | 空的 Context：没有超时、没有取消信号，作为默认值用 |
| `t.Fatalf` | 打印并**立刻终止**本条测试；`t.Errorf` 只记错、继续跑 |

---

## 三个实验（建议都跑一遍）

### 实验 1：亲眼看类型

```go
t.Logf("%T", c)
```

输出 `*fake.fakeClient`。小写开头说明它是未导出类型——**所以你只能通过接口使用它**，这也是 `Build()` 声明返回接口的原因。

### 实验 2：删掉 `WithScheme`

```go
c := fake.NewClientBuilder().Build()
```

再跑测试，会报类似：

```text
no kind is registered for the type v1.ConfigMap in scheme
```

因为 fake 不知道 `ConfigMap` 对应哪个 GVK，就无法在内存里存它。

### 实验 3：去掉 `&`

```go
r := Reconciler{Client: c}
```

编译失败，报指针接收者不匹配。这就是为什么模板里永远写 `&Reconciler{...}`。

---

## 报错对照表

| 报错 | 原因 | 解法 |
|---|---|---|
| `no kind is registered for the type ... in scheme` | Scheme 里没有这个类型 | 加 `YourAPIV1.AddToScheme(scheme)` |
| `cannot use ... Reconcile method has pointer receiver` | 少写了 `&` | 改成 `&Reconciler{...}` |
| `invalid memory address or nil pointer dereference` | `Build()` 之后没赋值给字段 | 检查是否忘了 `Client: c` |
| 测试通过但线上不对 | 用了 fake 去测 apiserver 才有的行为 | 见 [三层测试边界](02-three-test-layers.md) |

---

## 可直接复制的模板

```go
func TestXxx(t *testing.T) {
	c := fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme). // 旋钮 1：有 CRD 时加自己的 scheme
		WithObjects( /* 初始对象 */ ).      // 旋钮 2：需要"已存在"的场景才写
		Build()
	r := &Reconciler{Client: c}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "x", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("...: %v", err)
	}
}
```

需要 import：

```go
import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)
```

---

## 自测（不看上文回答）

1. `Build()` 返回的是具体类型还是接口？为什么这么设计？
2. `.WithScheme()` 的返回值如果扔掉，配置会丢吗？为什么？
3. `Reconciler` 里没写 `Get` 方法，为什么 `r.Get()` 能编译通过？
4. 为什么必须 `&Reconciler{...}` 而不是 `Reconciler{...}`？

答案分别落在：③接口赋值、②Builder 改自身、匿名字段方法提升、指针接收者。

---

## 关联

- 三层测试的边界：[02-three-test-layers.md](02-three-test-layers.md)
- Scheme 是什么：`01Scheme/README.md`（第 01 章）
- 后续小节：Manager + 缓存，`mgr.GetClient()` vs `mgr.GetAPIReader()`（见 [索引](README.md) 第 04 条）