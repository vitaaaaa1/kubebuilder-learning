# 02-for-and-scheme · For() 与 Scheme

## 学习目标

`For()` 这一行决定了三件事：

```go
ctrl.NewControllerManagedBy(mgr).For(&corev1.ConfigMap{}).Complete(r)
                                 ↑
                                 ① 监听哪个 GVK
                                 ② 控制器叫什么名字
                                 ③ Scheme 里必须注册过这个类型
```

本节用两个控制器 + 两个故意失败的实验，把这三件事拆开。

## 目录

```text
02-for-and-scheme/
├── main.go            # 两个控制器：ConfigMap（默认名）+ Secret（Named）
├── reconciler.go      # 共用实现，靠 Kind 区分日志
├── broken-scheme/     # 实验 A：不注册 Scheme 会怎样
├── for-twice/         # 实验 B：For 两次会怎样
└── README.md
```

## 运行

```bash
cd 02Manager/02-for-and-scheme

go run .                    # 正常：两个控制器
go run ./broken-scheme      # 实验 A
go run ./for-twice          # 实验 B

make demo                   # 创建 ConfigMap + Secret
make clean-demo
```

## 实测：两个控制器互不干扰（2026-11-04，namespace=test）

启动：

```text
INFO  setup  启动两个控制器：configmap（默认名） + secret-watcher（Named）

INFO  Starting EventSource  {"controller":"secret-watcher","source":"kind source: *v1.Secret"}
INFO  Starting EventSource  {"controller":"configmap",     "source":"kind source: *v1.ConfigMap"}
INFO  Starting Controller   {"controller":"secret-watcher"}
INFO  Starting workers      {"controller":"secret-watcher","worker count":1}
INFO  Starting Controller   {"controller":"configmap"}
INFO  Starting workers      {"controller":"configmap","worker count":1}
```

两个控制器的 `EventSource` **顺序不固定**（并发启动），但每个都会经历
`EventSource → Controller → workers` 三行。

启动瞬间，各自的初始对象：

```text
INFO 事件到达  {"controller":"configmap",     "name":"kube-root-ca.crt",              "reconciler":"ConfigMap"}
INFO 事件到达  {"controller":"secret-watcher","name":"kube-root-ca.crt-child-secret", "reconciler":"Secret"}
```

创建 `demo-cm` 和 `demo-secret`：

```text
INFO 事件到达  {"controller":"configmap",     "name":"demo-cm",     "reconciler":"ConfigMap"}
INFO 事件到达  {"controller":"secret-watcher","name":"demo-secret", "reconciler":"Secret"}
```

**没有任何交叉**：ConfigMap 的事件不会进 Secret 控制器，反之亦然。

### 名字从哪来

| 写法 | 控制器名 | 日志字段 |
|---|---|---|
| `For(&corev1.ConfigMap{})` | `configmap` | `"controller":"configmap"` |
| `.Named("secret-watcher")` | `secret-watcher` | `"controller":"secret-watcher"` |

默认名 = **kind 的小写形式**（`ConfigMap` → `configmap`）。源码在 `doController` 里
用 `getControllerName(gvk, hasGVK)` 生成，`Named()` 直接覆盖它。

## 实验 A：Scheme 里没注册类型

代码（`broken-scheme/main.go`）故意不调用 `corev1.AddToScheme`：

```go
scheme := runtime.NewScheme()   // 空 Scheme

mgr, _ := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme})
err := ctrl.NewControllerManagedBy(mgr).
	For(&corev1.ConfigMap{}).
	Complete(&LoggingReconciler{})
```

真实输出：

```text
Complete 返回: no kind is registered for the type v1.ConfigMap in scheme "pkg/runtime/scheme.go:111"
```

**关键点**：错误发生在 `Complete`（也就是 `Build`）阶段，不是 `Start` 阶段。

原因：`Build` 要用 `apiutil.GVKForObject(obj, mgr.GetScheme())` 把 Go 类型翻成 GVK，
Scheme 里没有就翻不出来。

## 实验 B：一个 Builder 调用两次 For()

```go
b := ctrl.NewControllerManagedBy(mgr)
b.For(&corev1.ConfigMap{})
b.For(&corev1.Secret{})      // ← 第二次
err := b.Complete(r)
```

真实输出：

```text
Complete 返回: For(...) should only be called once, could not assign multiple objects for reconciliation
```

源码 `builder/controller.go:93-105`：

```go
func (blder *TypedBuilder[request]) For(object client.Object, opts ...ForOption) *TypedBuilder[request] {
	if blder.forInput.object != nil {
		blder.forInput.err = fmt.Errorf("For(...) should only be called once, ...")
		return blder
	}
	...
}
```

**它不在第二次 `For` 时 panic，而是把错误存起来，等 `Complete` 时返回**。

想要多个主资源？那是两个控制器（本节 `main.go` 的做法），或者用 `Watches()`。

## 源码：Build 如何校验

`builder/controller.go:266` 的 `Build` 按顺序检查：

```text
Build(r)
 ├── r == nil?              → "must provide a non-nil Reconciler"
 ├── mgr == nil?            → "must provide a non-nil Manager"
 ├── forInput.err != nil?   → 实验 B 的错误
 ├── doWatch()              → 用 Scheme 解析 GVK（实验 A 的错误在这里）
 └── doController(r)        → controller.New(...) + mgr.Add
```

## 观察清单

| 现象 | 说明 |
|---|---|
| 两个 `EventSource` 顺序不固定 | 控制器并发启动 |
| 每个控制器都有自己的 `Starting workers` | 各自独立的 workqueue + worker |
| 默认名是 kind 小写 | `ConfigMap` → `configmap` |
| `Named()` 覆盖默认名 | 日志里立刻可见 |
| Scheme 缺类型 → `Complete` 返回错误 | 不是 `Start` 时 |
| `For` 两次 → `Complete` 返回错误 | 错误被延迟保存 |

## 常见问题

| 报错 | 原因 |
|---|---|
| `no kind is registered for the type ...` | 忘了 `AddToScheme` |
| `For(...) should only be called once` | 同一个 Builder 写了两次 `For` |
| `must provide a non-nil Manager` | Builder 不是由 `NewControllerManagedBy(mgr)` 创建 |
| `must provide a non-nil Reconciler` | `Complete(nil)` |

## 下一步

- `Owns()`：子对象变化触发父对象
- `Watches()`：任意事件源
- `WithEventFilter`：事件过滤