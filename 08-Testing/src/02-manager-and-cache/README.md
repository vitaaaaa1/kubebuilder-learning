# 02-manager-and-cache：缓存不是透明的

## 这个目录解决什么问题

00 和 01 用的是**直连客户端**：每次 `Get` 都发一个 HTTP 请求给 apiserver。

真实控制器不是这样——它们从 Manager 拿 client，**读走缓存，写才直连**。这带来两个必须知道的后果：

1. 读快了 **400–500 倍**
2. **刚写进去的对象，马上读可能读不到**

不会这两点，测试就会莫名其妙地间歇性失败。

---

## 三个客户端

| 变量 | 来源 | 读 | 写 |
|---|---|---|---|
| `directClient` | `client.New(cfg, ...)` | apiserver | apiserver |
| `cachedClient` | `mgr.GetClient()` | **缓存** | apiserver |
| `apiReader` | `mgr.GetAPIReader()` | apiserver | （只能读） |

`cachedClient` 就是控制器里 `r.Client` 的真实身份。

---

## `GetClient()` vs `GetAPIReader()`

### 一眼看区别

| | `mgr.GetClient()` | `mgr.GetAPIReader()` |
|---|---|---|
| 返回类型 | `client.Client` | `client.Reader` |
| 读 | **本地缓存** | apiserver |
| 写 | apiserver | **不允许**（编译期就挡住） |
| 一致性 | 可能旧（最多落后一个 watch 延迟） | 强一致（默认读最新） |
| 速度 | ~0.5 µs | ~270 µs |
| 用途 | 控制器日常读 | 需要“立刻看到最新”的场景 |

### 源码：两个独立对象

`pkg/cluster/cluster.go:227-240`：

```go
// ① 带缓存的 client
clientOpts.Cache.Reader = cache
clientWriter, _ := options.NewClient(config, clientOpts)   // → GetClient()

// ② 不带缓存的 client
clientReader, _ := client.New(config, client.Options{      // → GetAPIReader()
	Scheme: options.Scheme,
	Mapper: mapper,
})
```

`GetAPIReader` 返回的是 `client.Reader` 接口：

```go
type Reader interface {
	Get(ctx context.Context, key ObjectKey, obj Object, opts ...GetOption) error
	List(ctx context.Context, list ObjectList, opts ...ListOption) error
}
```

**只有 `Get` / `List`，没有 `Create` / `Update` / `Delete`。**
所以“apiReader 只能读”不是约定，是类型系统保证的。

### 为什么需要两个

缓存用“旧数据”换“快”。但有些场景不能接受旧数据：

| 场景 | 用谁 | 原因 |
|---|---|---|
| 控制器里“看看对象现在什么样” | `GetClient` | 快 400 倍，旧一点无所谓 |
| 写完立刻读自己的写 | `apiReader` 或 `Eventually` | 缓存还没收到 watch 事件 |
| 要拿最新 `resourceVersion` 去 `Update` | `apiReader` | 否则容易 409 |
| 缓存还没同步完成（启动早期） | `apiReader` | 缓存是空的 |
| 排查“缓存里到底有没有” | 两个都读，对比 | — |

### 一个真实坑：旧 RV 导致 409

```go
var cm corev1.ConfigMap
cachedClient.Get(ctx, key, &cm)   // 拿到 resourceVersion = 100
// ... 期间别人更新了，实际已经是 105
cm.Data["x"] = "y"
cachedClient.Update(ctx, &cm)     // ❌ 409 Conflict：RV 过期
```

`Update` 是**乐观锁**：apiserver 要求你带的 RV 等于当前 RV。

控制器里的标准做法是**冲突就重排队**（返回 error，等下一次 Reconcile 用新数据重来）。测试里则要 `Eventually`：

```go
Eventually(func() error {
	var latest corev1.ConfigMap
	if err := apiReader.Get(ctx, key, &latest); err != nil {
		return err
	}
	latest.Data["x"] = "y"
	return cachedClient.Update(ctx, &latest)
}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
```

### 测试里的建议

**默认用 `apiReader` 做断言，只在专门测缓存行为时才用 `cachedClient`。**

判断口诀：

> 问“最终结果对不对” → `apiReader`
> 问“缓存 / 索引 / 控制器看到什么” → `cachedClient`

#### 用 `apiReader` 的三个典型例子

**① 断言对象被创建**

```go
cm := &corev1.ConfigMap{
	ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
	Data:       map[string]string{"k": "v"},
}
if err := cachedClient.Create(ctx, cm); err != nil {   // 写永远直连
	t.Fatal(err)
}

var got corev1.ConfigMap
if err := apiReader.Get(ctx, key("demo"), &got); err != nil {
	t.Fatalf("对象没创建成功: %v", err)               // ← 确定性
}
if got.Data["k"] != "v" {
	t.Fatalf("内容不对: %v", got.Data)
}
```

用 `cachedClient` 读的话，可能随机得到：

```text
configmaps "demo" not found
```

**② 断言控制器改了 status**

```go
_, err := r.Reconcile(ctx, req)
if err != nil {
	t.Fatal(err)
}

var got MyApp
if err := apiReader.Get(ctx, req.NamespacedName, &got); err != nil {
	t.Fatal(err)
}
if got.Status.Phase != "Ready" {
	t.Fatalf("期望 Ready，实际 %q", got.Status.Phase)
}
```

**③ 断言 ownerReference**

```go
var child corev1.Secret
if err := apiReader.Get(ctx, key("child"), &child); err != nil {
	t.Fatal(err)
}

ref := child.OwnerReferences[0]
if ref.UID != owner.UID {
	t.Fatalf("ownerRef.UID 不对: 期望 %s，实际 %s", owner.UID, ref.UID)
}
```

#### `cachedClient` 的四个正当场景

**① 专门测“缓存最终一致”**

```go
cachedClient.Create(ctx, cm)

Eventually(func() error {
	var got corev1.ConfigMap
	return cachedClient.Get(ctx, key("demo"), &got)
}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
```

这里 `cachedClient` 是**被测对象**，不是工具。

**② 测控制器的视角**

控制器里 `r.Get` 走的就是缓存，所以“控制器能看到什么”只能用 `cachedClient` 验证：

```go
var got corev1.ConfigMap
if err := cachedClient.Get(ctx, key, &got); err != nil {
	t.Fatalf("控制器视角读不到: %v", err)
}
```

**③ 测缓存配置是否生效**

比如只缓存带某个 label 的对象：

```go
mgr, _ := ctrl.NewManager(cfg, ctrl.Options{
	Cache: cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Label: labels.SelectorFromSet(labels.Set{"cache": "yes"}),
			},
		},
	},
})
```

```go
// 不带 label → 缓存里应该没有
var got corev1.ConfigMap
err := cachedClient.Get(ctx, key("no-label"), &got)
if !apierrors.IsNotFound(err) {
	t.Fatalf("期望缓存里没有，实际: %v", err)
}
```

`apiReader` 会绕过这个配置，永远读得到，**测不出问题**。

**④ 测 FieldIndexer（索引只存在于缓存）**

```go
mgr.GetFieldIndexer().IndexField(ctx, &corev1.ConfigMap{}, "data.k",
	func(obj client.Object) []string {
		return []string{obj.(*corev1.ConfigMap).Data["k"]}
	})

var list corev1.ConfigMapList
cachedClient.List(ctx, &list, client.MatchingFields{"data.k": "v"})
```

`apiReader` 不支持本地索引，这条只能用 `cachedClient`。

#### 对照表

| 你要断言的事 | 用谁 | 理由 |
|---|---|---|
| 对象被创建 / 内容正确 | `apiReader` | 确定性 |
| 控制器改了 status | `apiReader` | 写已落盘 |
| ownerRef / UID 正确 | `apiReader` | 依赖最新数据 |
| 缓存最终一致 | `cachedClient` + `Eventually` | 被测对象就是缓存 |
| 缓存配置（label / namespace 过滤） | `cachedClient` | apiReader 绕过配置 |
| 本地索引（`MatchingFields`） | `cachedClient` | 索引在缓存里 |
| “缓存会滞后”本身 | `cachedClient` | 测的就是这个 |

#### 为什么 `cmd/envtest-basic` 的断言都稳

它用的是：

```go
k8sClient, _ = client.New(cfg, client.Options{Scheme: testScheme})
```

`client.New` 没有缓存，**每个断言都直连**，所以不会 flaky。

---

## suite 比 00 多了什么

00 的 `TestMain` 只起 apiserver，然后 `client.New`。02 在中间插了一个 Manager：

```go
mgr, err := ctrl.NewManager(cfg, ctrl.Options{
	Scheme:  testScheme,
	Metrics: metricsserver.Options{BindAddress: "0"},
})

go func() { _ = mgr.Start(ctx) }()          // ① 后台跑起来

mgr.GetCache().WaitForCacheSync(ctx)        // ② 等首次 List+Watch 完成

cachedClient = mgr.GetClient()              // ③ 读走缓存
apiReader    = mgr.GetAPIReader()           // ④ 读直连
```

三个细节：

- `BindAddress: "0"` 关掉 metrics 端口，否则并行跑测试会抢 8080
- `mgr.Start` 是**阻塞**的，必须放 goroutine
- `WaitForCacheSync` 等的是"所有 informer 完成首次同步"，不是"Manager 启动完"

---

## `ctrl.NewManager` 和 `client.New` 到底差在哪

一句话：**Manager 是"运行时容器"，client 是"访问器"。Manager 内部包含 client。**

### `client.New`：只造一个访问器

```go
c, _ := client.New(cfg, client.Options{Scheme: scheme})
```

```text
c.Get / c.List / c.Create / c.Update / c.Delete
   ↓
每次都直接发 HTTP 给 apiserver
```

没有缓存、没有 informer、没有控制器、没有端口。00 和 01 用的就是它。

### `ctrl.NewManager`：造一整套运行时

```text
ctrl.NewManager
├── cache（informer 工厂）       ← List + Watch，填充本地缓存
├── GetClient()    → 读缓存、写直连
├── GetAPIReader() → 读也直连
├── scheme / RESTMapper
├── event recorder
├── webhook server（占端口）
├── metrics server（占端口）
├── health probe server（占端口）
└── Start(ctx) 生命周期管理
```

### 关键：Manager 内部调了两次 `client.New`

源码 `pkg/cluster/cluster.go:227-240`：

```go
// ① 带缓存的 client → mgr.GetClient()
clientOpts.Cache.Reader = cache
clientWriter, _ := options.NewClient(config, clientOpts)

// ② 不带缓存的 client → mgr.GetAPIReader()
clientReader, _ := client.New(config, client.Options{
	Scheme: options.Scheme,
	Mapper: mapper,
})
```

所以**不是二选一，而是包含关系**：

```text
mgr.GetClient()    = client.New + Cache
mgr.GetAPIReader() = client.New（无 Cache）
```

### 对比

| 能力 | `client.New` | `ctrl.NewManager` |
|---|---|---|
| 读写对象 | ✅ | 通过 `GetClient()` |
| 本地缓存 | ❌ | ✅ 内建 |
| informer / watch | ❌ | ✅ |
| 注册 Controller（`mgr.Add`） | ❌ | ✅ |
| webhook server | ❌ | ✅ |
| metrics / health 端口 | ❌ | ✅ |
| 生命周期（`Start`） | ❌ | ✅ |
| 构造成本 | 极低 | 启动缓存、占端口 |

### 选哪个

| 场景 | 选择 |
|---|---|
| 只测一段业务逻辑（"一次 Get 的结果"） | `client.New` |
| 要测缓存、调度、准入（"系统级行为"） | `ctrl.NewManager` |

### 常见误解

```go
// ❌ 以为要自己造 client 再塞给 Manager
c, _ := client.New(cfg, ...)
mgr, _ := ctrl.NewManager(cfg, ...)
```

不需要。Manager 在创建时已经建好了自己的 client 和 cache，直接 `mgr.GetClient()`。

---

## 为什么 `mgr.Start` 要放 goroutine

### 它阻塞，直到 ctx 取消

源码 `pkg/manager/internal.go:466`：

```go
	select {
	case <-ctx.Done():
		return nil            // 正常退出
	case err := <-cm.errChan:
		return err            // 某个组件出错
	}
```

启动完 cache、webhook、metrics、controller 之后，`Start` 就停在这个 `select` 上。

### 不写 `go` 的后果

```text
mgr.Start(ctx)                        // 卡死
mgr.GetCache().WaitForCacheSync(ctx)  // 执行不到
code := m.Run()                       // 测试永远不会开始
→ panic: test timed out after 10m0s
```

### 时序

```text
主 goroutine                          后台 goroutine（go mgr.Start）
     │                                        │
     ├── go mgr.Start(ctx) ─────────────────▶ 启动 cache / metrics / webhooks
     │                                        │
     ├── WaitForCacheSync(ctx) ────等待──────▶ 缓存完成首次 List+Watch
     │                                        │
     ├── m.Run()  ← 测试真正开始              │
     │                                        │
     ├── cancel() ──────────────────────────▶ ctx.Done()
     │                                        │
     ├── testEnv.Stop()                   Start 返回 nil
     ▼                                        ▼
```

和 `http.Server` 一样：

```go
go server.ListenAndServe()   // 阻塞的东西放后台
```

### 为什么错误只是打印

```go
go func() {
	if err := mgr.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "manager 退出: %v\n", err)
	}
}()
```

- 这个 goroutine 里**没有 `*testing.T`**，不能 `t.Fatal`
- `Start` 返回 `nil` 是正常路径（ctx 取消）

生产代码通常用 channel 或 `errgroup` 把错误传回主 goroutine。

### 顺序陷阱

```go
mgr.GetCache().WaitForCacheSync(ctx)   // ❌ 永久阻塞
go mgr.Start(ctx)                      // 还没开始
```

缓存是 `Start` 里才启动的，先等同步会一直等下去。

---

## 实验 A：速度差多少

```go
// 预热一次（第一次 Get 会创建 informer，开销不算进平均）
cachedClient.Get(ctx, key, &cm)

// 各跑 1000 次
```

两次运行的真实数据：

```text
缓存读:    517ns / 次        缓存读:    675ns / 次
直连读: 288.732µs / 次       直连读: 264.61µs / 次
差距  : 558 倍               差距  : 392 倍
```

**缓存读 ≈ 0.5–0.7 µs，直连读 ≈ 260–290 µs，差 400–560 倍。**

原因是直连每次都要：序列化 → TCP → apiserver 认证鉴权 → etcd 查询 → 反序列化。
缓存只是在本地 map 里查一下（外加一次 deep copy）。

> 这个倍率会随机器和对象大小变化，不必记具体数字，记"**三个数量级附近**"。

---

## 实验 B：Create 之后立刻读，读得到吗

```go
for i := 0; i < 20; i++ {
	// 每轮一个全新的对象（名字带纳秒时间戳 + 轮次），否则第二轮会 AlreadyExists
	name := fmt.Sprintf("lag-%d-%d", time.Now().UnixNano(), i)

	directClient.Create(ctx, newCM(name))        // 写进 etcd

	err := cachedClient.Get(ctx, key(name), &cm) // 立刻从缓存读
	if err == nil {
		continue                                 // 缓存赢了：watch 事件先到
	}
	if apierrors.IsNotFound(err) {
		laggedAt = append(laggedAt, i)           // 缓存输了：记下轮次
	}
}
```

真实结果（每次跑都不一样）：

```text
i=00  NotFound（缓存输了：等了 2.625µs 才读到）
i=01  读到    （缓存赢了：watch 事件先到）
...
i=09  NotFound（缓存输了：等了 103.792µs 才读到）
i=10  NotFound（缓存输了：等了 137.333µs 才读到）
...
i=17  NotFound（缓存输了：等了 143.042µs 才读到）
i=18  读到    （缓存赢了：watch 事件先到）
i=19  读到    （缓存赢了：watch 事件先到）
etcd 里 lag-* 对象共 20 个（每轮一个）
共 20 轮：赢 16 轮，输 4 轮
输的轮次: [0 9 10 17]
等待时长: [2.625µs 103.792µs 137.333µs 143.042µs]
```

再跑一次可能变成：

```text
共 20 轮：赢 20 轮，输 0 轮
```

**同一份代码、同一台机器，结果完全不同。**

轮次日志用 `t.Logf` 而不是 `fmt.Println`：

| | `fmt.Println` | `t.Logf` |
|---|---|---|
| 带文件名:行号 | ❌ | ✅ |
| 只在 `-v` 时显示 | ❌ 总是显示 | ✅ |
| 失败时自动附在报告里 | ❌ | ✅ |
| 并行测试时归属清晰 | ❌ | ✅ |

**为什么是"有时"**：缓存靠 watch 事件更新，事件从 apiserver 推过来要 **100 多微秒**。
你的 `Create` 返回后立刻 `Get`，就是在和这个延迟赛跑——**赢不赢看运气**。

这正是"间歇性失败"的来源：本地跑 100 次都过，CI 上偶尔红一次。

### 为什么要先预热

循环开始前有一行看起来很奇怪：

```go
var warm corev1.ConfigMap
_ = cachedClient.Get(ctx, key("warmup"), &warm)
```

`"warmup"` 这个对象**不存在**，返回值被 `_` 丢掉。它的唯一作用是**触发 informer 创建**（缓存是懒加载的）。

去掉它之后实测（跑 3 次）：

```text
i=0 首次缓存读耗时 101.590834ms      赢 20 轮，输 0 轮
i=0 首次缓存读耗时 101.793584ms      赢 19 轮，输 1 轮   输的轮次: [7]
i=0 首次缓存读耗时 101.680875ms      赢 19 轮，输 1 轮   输的轮次: [14]
```

两个现象：

1. **第一次读 101 ms**（不是 500 ns）——在等 informer 完成 List + 建 Watch
2. **`i=0` 从不滞后**——那次 List 把刚创建的对象也拉回来了，根本没走 watch

所以不预热的话，第一轮测的是"冷启动"，会得出"缓存不滞后"的错误结论。

```text
预热 = 把「冷启动（List 全量）」和「稳态（watch 增量）」分开
```

更显式的写法：

```go
_, err := mgr.GetCache().GetInformer(ctx, &corev1.ConfigMap{})
```

### 关键认知

```text
Create 返回  ≠  缓存已更新
Create 返回  =  数据已在 etcd
```

---

## 实验 C：证明数据真的写进去了

同一时刻，换 `apiReader` 读：

```text
20 次「Create 后立即 apiReader 读」中，20 次立即可见
```

**结论**：`NotFound` 不是"没写成功"，而是"缓存还没收到事件"。

---

## 两条工程规则

### 规则 1：写后立刻读 → 用 apiReader 或 Eventually

```go
// 方式一：绕开缓存（确定性强，但慢）
var cm corev1.ConfigMap
if err := apiReader.Get(ctx, key, &cm); err != nil { ... }
```

```go
// 方式二：等缓存追上（推荐，贴近真实行为）
deadline := time.Now().Add(5 * time.Second)
for {
	var cm corev1.ConfigMap
	if err := cachedClient.Get(ctx, key, &cm); err == nil {
		break
	}
	if time.Now().After(deadline) {
		t.Fatal("5 秒内缓存仍未同步")
	}
	time.Sleep(5 * time.Millisecond)
}
```

用 gomega 时就是一行：

```go
Eventually(func() error {
	return k8sClient.Get(ctx, key, &cm)
}, 5*time.Second, 100*time.Millisecond).Should(Succeed())
```

### 规则 2：普通读 → 用缓存

控制器里 99% 的读都是"看看对象现在长什么样"，不要求读到最新。用缓存。

---

## 常见误区

| 误区 | 实际 |
|---|---|
| "`mgr.GetClient()` 写也走缓存" | 写**永远**直连 apiserver |
| "`WaitForCacheSync` 之后缓存就是最新的" | 只保证"同步过一次"，之后靠 watch |
| "fake client 也能复现缓存行为" | fake 是同步 map，没有缓存层，**永远复现不出** |
| "第一次 Get 慢是因为缓存冷" | 第一次会**创建 informer 并等同步**，慢在这一步 |

---

## 运行

```bash
cd 08-Testing/src/02-manager-and-cache
make test-simple
```

```text
=== RUN   TestCacheVsDirectSpeed
    缓存读:    517ns / 次
    直连读: 288.732µs / 次
    差距  : 558 倍
--- PASS (0.42s)
=== RUN   TestCacheLagAfterCreate
    20 次「Create 后立即缓存读」中，3 次返回 NotFound
    滞后时缓存追上耗时: [137.25µs 142.542µs 124.959µs]
--- PASS (0.01s)
=== RUN   TestAPIReaderSeesWriteImmediately
    20 次「Create 后立即 apiReader 读」中，20 次立即可见
--- PASS (0.02s)
ok  .../02-manager-and-cache  4.8s
```

退出时会看到一行警告：

```text
watch of *v1.ConfigMap ended with: an error on the server
("unable to decode an event from the watch stream: context canceled")
```

这是 `testEnv.Stop()` 取消 ctx 导致的，**无害**。

---

## 自测

1. 为什么 `cachedClient.Create` 之后 `cachedClient.Get` 可能读不到，但 `apiReader.Get` 读得到？
2. `WaitForCacheSync` 等的是什么？等完之后缓存会一直是最新的吗？
3. 你的测试里有一条"创建 ConfigMap 后立刻断言它存在"，用 `cachedClient` 写。CI 上偶发失败，最可能的原因是什么？怎么改？
4. `ctrl.NewManager` 和 `client.New` 是什么关系？各返回什么？
5. `go mgr.Start(ctx)` 里的 `go` 能去掉吗？为什么？
6. 循环前的 `cachedClient.Get(ctx, key("warmup"), &warm)` 删掉会怎样？
7. 断言"对象创建成功"时，为什么推荐 `apiReader` 而不是 `cachedClient`？

<details>
<summary>答案</summary>

1. 写直连 apiserver，读走本地 informer 缓存；缓存更新依赖 watch 事件，有 100µs 级延迟。
2. 等的是"所有 informer 完成首次 List + Watch 建立"。之后靠 watch 增量更新，仍可能瞬时落后。
3. 写后立刻用缓存读，撞上了 watch 延迟。改成 `apiReader` 读，或用 `Eventually` 重试。
4. 包含关系：Manager 是运行时容器，内部调了两次 `client.New`，分别得到 `GetClient()`（带缓存）和 `GetAPIReader()`（直连）。
5. 不能。`Start` 会阻塞到 ctx 取消，去掉后主 goroutine 卡死，`m.Run()` 永远执行不到，最终测试超时。
6. 第一轮会花约 100 ms 创建 informer，且那次用 List 拿到了刚创建的对象，导致 `i=0` 永远不滞后，测不出真实延迟。
7. 缓存可能落后一个 watch 延迟，断言会随机失败；`apiReader` 直连 apiserver，结果确定。测缓存行为时才用 `cachedClient`。

</details>

---

## 下一步

| 方向 | 涉及 |
|---|---|
| 03-crd-envtest | `CRDDirectoryPaths`、自定义类型、`AddToScheme` |
| 04-webhook-envtest | TLS、`WebhookInstallOptions` |
| 把 `Eventually` 用进 01 | 写后读的断言改写 |