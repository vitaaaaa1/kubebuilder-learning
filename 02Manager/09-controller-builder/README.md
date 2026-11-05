# 09-controller-builder · 拆开 NewControllerManagedBy

## 这个章节在学什么

一句话：

> **`NewControllerManagedBy` 用一行代码，替你做了“建控制器 + 接事件源”这五件事。**

两个文件做**完全一样**的事，只是写法不同：

```text
with-builder/   一行：For + Complete
manual/         五步手写
```

跑起来行为一模一样——这就是拆解的意义：**看清那一行展开后是什么。**

## 一、两个文件的差异

**版本 A（with-builder）**

```go
ctrl.NewControllerManagedBy(mgr).
	For(&corev1.ConfigMap{}).
	Complete(&shared.Reconciler{...})
```

**版本 B（manual）**

```go
// 第 1 步：解析 GVK
gvk, _ := apiutil.GVKForObject(&corev1.ConfigMap{}, mgr.GetScheme())

// 第 2 步：控制器名 = kind 小写 + 后缀
name := fmt.Sprintf("%s-controller", strings.ToLower(gvk.Kind))  // "configmap-controller"

// 第 3 步：创建 Controller（内部会 mgr.Add）
c, _ := controller.New(name, mgr, controller.Options{Reconciler: r})

// 第 4 步：建事件源
src := source.Kind[client.Object](mgr.GetCache(),
	&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{})

// 第 5 步：挂到 Controller 上
c.Watch(src)
```

**除这一段，两个文件的其余部分逐字相同**（Manager 配置、Scheme、Reconciler、启动）。

## 二、五步对照表

| `NewControllerManagedBy` 的调用 | 内部做的事 |
|---|---|
| `NewControllerManagedBy(mgr)` | 造一个 Builder，记下 mgr |
| `.For(&ConfigMap{})` | 记下对象类型（此时还不建 source） |
| `.Complete(r)` | 等价于 `Build(r)`，忽略返回的 controller |
| └ `Build` → `doWatch` | 解析 GVK → 建 `source.Kind` → `c.Watch(src)` |
| └ `Build` → `doController` | 名字默认 `strings.ToLower(kind)` → `controller.New` → `mgr.Add(c)` |

也就是说：

```text
For + Complete  =  上面手写的第 1~5 步
```

## 三、实测日志对比

**版本 A（builder）**

```text
INFO setup  装配方式：NewControllerManagedBy + For + Complete
INFO Starting EventSource {"controller":"configmap-controller","controllerGroup":"","controllerKind":"ConfigMap","source":"kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller":"configmap-controller","controllerGroup":"","controllerKind":"ConfigMap"}
INFO Starting workers     {"controller":"configmap-controller","controllerGroup":"","controllerKind":"ConfigMap","worker count":1}
INFO Reconcile 被调用     {"controller":"configmap","ConfigMap":{"name":"demo-builder",...},"assembly":"builder"}
```

**版本 B（manual）**

```text
INFO setup  装配方式：五步手写
INFO Starting EventSource {"controller":"configmap-controller","source":"kind source: *v1.ConfigMap"}
INFO Starting Controller  {"controller":"configmap-controller"}
INFO Starting workers     {"controller":"configmap-controller","worker count":1}
INFO Reconcile 被调用     {"controller":"configmap","object":{"name":"demo-builder",...},"assembly":"manual"}
```

### 行为完全一致

| 项 | builder | manual |
|---|---|---|
| 控制器名 | `configmap-controller` | `configmap-controller` |
| 启动时初始 Reconcile | 2 个（demo-config、kube-root-ca.crt） | 同样 2 个 |
| create / delete 触发 | 各 1 次 | 各 1 次 |
| EventSource / workers | 都有 | 都有 |

### 只差日志字段

| 差异 | 来源 |
|---|---|
| `controllerKind` / `controllerGroup` | Builder 自动生成 log constructor |
| 对象字段名 `ConfigMap`（而非 `object`） | 同上 |
| 名字自动推导 | Builder 用 GVK 的 kind 小写 |

## 四、Builder 额外给的三样东西

手写版本没有的：

| 能力 | 说明 |
|---|---|
| 自动命名 | 默认 kind 小写；`Named("configmap-controller")` 可覆盖 |
| 构建期校验 | 重复 `For`、nil reconciler 会在 `Complete` 报错 |
| 扩展入口 | `Owns` / `Watches` / predicate 都是现成的 |

## 五、只需要记住三句话

```text
1. For 只是“记下类型”，不建任何东西。
2. Complete 才真正建 source + 建 controller + 注册进 Manager。
3. 手写五步和它完全等价，只是少了自动命名和校验。
```

## 六、运行

```bash
cd 02Manager/09-controller-builder

go run ./with-builder
go run ./manual

# 触发事件
kubectl create configmap demo-builder -n test --from-literal=a=1
kubectl delete configmap demo-builder -n test
```

对照两份日志：前四行几乎一样，只有字段多少的差别。

## 七、和 05 章的关系

`05-build-and-add/compare/` 里已经有一组更早的对照（只拆了两步）。
这一章把同样的思路扩展成**完整的五步**，并加入了 GVK 解析和命名推导——那正是 `For` 与 `Complete` 之间的隐藏工作。

## 八、下一步

`Complete` 之后就进入 Controller 本体：

```text
controller.New
  └── workqueue（事件排队、去重）
  └── worker（并发调用 Reconcile）
  └── Watch → source → handler → 入队
```

也就是“事件怎么变成 Reconcile”。