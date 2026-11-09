# NewControllerManagedBy 拆解记录

> 覆盖两个目录：
>
> | 目录 | 内容 |
> |---|---|
> | `09-controller-builder/` | `with-builder/` 一行装配 vs `manual/` 五步手写 |
> | `10-multi-controller/` | 一个 Manager 挂两个 Controller |
>
> 版本：controller-runtime `v0.25.2`、client-go / apimachinery `v0.37.1`。
> 所有日志来自真实集群（namespace `test`）。

---

## 1. 一句话职责

> **`NewControllerManagedBy` 是一个“控制器装配器”：先记录配置，直到 `Complete` 才真正创建 Controller、接上事件源、注册进 Manager。**

它**不连接集群**，也不启动任何东西。

```text
NewManager            → 连接 + 缓存 + 生命周期
NewControllerManagedBy → 哪个对象的变化 → 交给哪个 Reconciler
```

---

## 2. 调用链

```text
ctrl.NewControllerManagedBy(mgr)
  └── TypedControllerManagedBy[reconcile.Request](mgr)   造 Builder，只存 mgr
        .For(&ConfigMap{})                               只记录 ForInput
        .Complete(r)
          └── Build(r)
                ├── 校验（r 非空、mgr 非空、For 的延迟错误）
                ├── doController(r)   ← 先建控制器
                │     GVK → 并发数 → 名字 → log constructor
                │     controller.NewTyped → mgr.Add
                └── doWatch()         ← 再接事件源
                      source.TypedKind(cache, obj, EnqueueRequestForObject, predicates)
                      ctrl.Watch(src)
```

> 注意顺序：**先 `doController`，后 `doWatch`**。

---

## 3. 每个方法做了什么

### 3.1 `NewControllerManagedBy(mgr)`

```go
func TypedControllerManagedBy[request comparable](m manager.Manager) *TypedBuilder[request] {
	return &TypedBuilder[request]{mgr: m}
}
```

只做一件事：把 mgr 存进 Builder。

### 3.2 `For(object)`

```go
func (blder *TypedBuilder[request]) For(object client.Object, opts ...ForOption) *TypedBuilder[request] {
	if blder.forInput.object != nil {
		blder.forInput.err = fmt.Errorf("For(...) should only be called once, ...")
		return blder
	}
	input := ForInput{object: object}
	blder.forInput = input
	return blder
}
```

| 行为 | 说明 |
|---|---|
| 只记录 | 不解析 GVK、不建 source |
| 只允许一次 | 第二次只记录错误，**到 `Complete` 才报** |
| 返回值 | 仍是 Builder，便于链式调用 |

### 3.3 `Named(name)`

```go
func (blder *TypedBuilder[request]) Named(name string) *TypedBuilder[request] {
	blder.name = name
	return blder
}
```

覆盖默认名（`strings.ToLower(gvk.Kind)`）。

### 3.4 `Complete(r)`

```go
func (blder *TypedBuilder[request]) Complete(r reconcile.TypedReconciler[request]) error {
	_, err := blder.Build(r)
	return err
}
```

**`Complete` = `Build` + 丢弃返回的 controller。**

---

## 4. `Build` 内部的两大块

### 4.1 `doController(r)`

| 步骤 | 代码 | 结果 |
|---|---|---|
| 取 GVK | `apiutil.GVKForObject(forInput.object, scheme)` | `/v1, Kind=ConfigMap` |
| 并发数 | `globalOpts.GroupKindConcurrency[groupKind]` | 默认 0 → 用 1 |
| 名字 | `blder.name` 或 `strings.ToLower(gvk.Kind)` | `configmap` |
| 日志字段 | `WithValues("controller", name, "controllerGroup", g, "controllerKind", k)` | 见 5.2 |
| 建控制器 | `controller.NewTyped(name, mgr, opts)` | 内部 `mgr.Add(c)` |

### 4.2 `doWatch()`

```go
hdler := &handler.EnqueueRequestForObject{}          // TypedEventHandler[client.Object, Request]
allPredicates := append(globalPredicates, forInput.predicates...)
src := source.TypedKind(blder.mgr.GetCache(), obj, hdler, allPredicates...)
blder.ctrl.Watch(src)
```

- 没有 `For` 时跳过
- 只有 `For` 时，`Owns` 会被拒绝：`Owns() can only be used together with For()`

---

## 5. 09 的实测：一行 vs 五步

### 5.1 两种写法

**Builder**

```go
ctrl.NewControllerManagedBy(mgr).
	Named("configmap-controller").
	For(&corev1.ConfigMap{}).
	Complete(&shared.Reconciler{...})
```

**五步手写**

```go
gvk, _ := apiutil.GVKForObject(&corev1.ConfigMap{}, mgr.GetScheme())
name := fmt.Sprintf("%s-controller", strings.ToLower(gvk.Kind))
c, _ := controller.New(name, mgr, controller.Options{Reconciler: r})
src := source.Kind[client.Object](mgr.GetCache(), &corev1.ConfigMap{},
	&handler.EnqueueRequestForObject{})
c.Watch(src)
```

### 5.2 日志差异

```text
builder: {"controller":"configmap-controller","controllerGroup":"","controllerKind":"ConfigMap","ConfigMap":{"name":"demo-config",...}}
manual : {"controller":"configmap-controller","object":{"name":"demo-config",...}}
```

差异全部来自 Builder 的 `LogConstructor`：

```text
controllerGroup / controllerKind      ← doController 里加的
以 kind 命名的对象字段（ConfigMap）    ← log.WithValues(gvk.Kind, klog.KRef(...))
```

**业务行为完全一致**：同样的控制器名、同样 2 个启动 Reconcile、create/delete 各触发一次。

### 5.3 跳过 Watch 的对照

给 `manual` 加了 `-no-watch`：

```text
INFO 已跳过 Watch：只有 worker，没有事件源
INFO Starting Controller {"controller": "configmap-controller"}
INFO Starting workers    {"controller": "configmap-controller", "worker count": 1}
```

**没有 EventSource，也没有任何 Reconcile。**
说明 `controller.New` 只准备“处理能力”，`Watch` 才接上“事件来源”。

---

## 6. 10 的实测：一个 Manager 挂两个 Controller

```go
ctrl.NewControllerManagedBy(mgr).Named("configmap-controller").For(&corev1.ConfigMap{}).Complete(&reconciler{kind: "ConfigMap"})
ctrl.NewControllerManagedBy(mgr).Named("secret-controller").For(&corev1.Secret{}).Complete(&reconciler{kind: "Secret"})
```

启动：

```text
INFO 已挂载 2 个 Controller
INFO Starting EventSource {"controller": "configmap-controller", "source": "kind source: *v1.ConfigMap"}
INFO Starting EventSource {"controller": "secret-controller",    "source": "kind source: *v1.Secret"}
INFO Starting Controller  {"controller": "configmap-controller"}
INFO Starting Controller  {"controller": "secret-controller"}
```

事件：

```text
create configmap demo-cm   →  Reconcile {"controller":"configmap-controller","request":"test/demo-cm"}
create secret demo-secret  →  Reconcile {"controller":"secret-controller","request":"test/demo-secret"}
```

**共享**：cache、client、连接池、LeaderElection 组。
**独立**：名字、队列、worker、Reconciler。

---

## 7. 关键结论

1. `For` 只记录，不创建任何东西。
2. `Complete` = `Build`，真正落地。
3. `Build` 先建控制器（`mgr.Add`），再接 source（`Watch`）。
4. 名字默认 `strings.ToLower(Kind)`，可由 `Named` 覆盖，且进程内必须唯一。
5. `controller.New` 之后没有任何 Reconcile，直到 `Watch` 挂上 source。
6. 一个 Manager 可以挂多个 Controller，各自独立。
7. 只有 `For` + `Owns/Watches` 才是“一个控制器监听多种资源”。

---

## 8. 常见误解

**Q：`For` 会发起 API 请求吗？**

不会。它只保存对象指针。GVK 解析在 `Build` 里做。

**Q：`Complete` 返回的 error 什么时候出现？**

`Build` 的校验阶段：Reconciler 为 nil、重复 `For`、`Owns` 没有 `For` 等。

**Q：一个 Controller 能监听两个主资源吗？**

不能。`For` 只能调用一次；第二个资源要用 `Owns`/`Watches`，且事件会映射到主对象。

**Q：多个 Controller 会重复启动 informer 吗？**

不会。它们共用 `mgr.GetCache()`，同类型 informer 只起一次。

---

## 9. 和前面章节的关系

```text
06-manager-anatomy      造 Manager（cache/client/mapper）
08-manager-runnables     mgr.Add 分组、启动顺序
09-controller-builder    Controller 怎么被造出来（本篇）
10-multi-controller      多个 Controller 并存
```

`controller.New` 内部调用 `mgr.Add`，所以每个 Controller 都进入 **LeaderElection 组**，
在缓存同步完成后才启动——这正是 08 章验证的顺序。

---

## 10. 运行

```bash
cd 02Manager

# 09：一行 vs 五步
cd 09-controller-builder
go run ./with-builder
go run ./manual
go run ./manual -no-watch        # 只建控制器，不接事件源

# 10：两个控制器
cd ../10-multi-controller
go run .
kubectl create configmap demo-cm -n test --from-literal=a=1
kubectl create secret generic demo-secret -n test --from-literal=b=2
```