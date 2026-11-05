# 04-predicates · 事件过滤

> 说明：`Owns()` / `Watches()` 之前在 `03Controller-Internals` 已经练过，这里直接跳到 M4。

## 学习目标

搞清一件事：**predicate 是在事件入队之前生效的**。

```text
informer 事件
     │
     ▼
 predicate 过滤  ← 这里就丢掉了，根本不进队列
     │
     ▼
  workqueue → worker → Reconcile
```

不是“进了 Reconcile 再 if 判断”。

## 三个控制器

| 控制器 | 过滤方式 | 放行什么 |
|---|---|---|
| `cm-all` | 无 | 全部事件 |
| `cm-label-changed` | `WithEventFilter(predicate.LabelChangedPredicate{})` | Create / Delete；Update 仅当 **labels 变了** |
| `cm-create-only` | `For(..., builder.WithPredicates(createOnly()))` | 只有 Create |

两种写法的区别：

```go
// 全局：作用于这个控制器的所有事件源（含 Owns/Watches）
WithEventFilter(p)

// 局部：只作用于这一个 For 的对象
For(&corev1.ConfigMap{}, builder.WithPredicates(p))
```

**注意：本节控制器 C 只有 `For` 一个事件源，所以这两种写法效果完全一样。**
用 `For(...)` 只是为了演示这个 API。一旦加上 `Owns()`，区别就出现了：

| 事件 | `For(..., WithPredicates(p))` | `WithEventFilter(p)` |
|---|---|---|
| 主资源（ConfigMap） | 被过滤 | 被过滤 |
| 子资源（Owns 的 Secret） | **不过滤** | **也被过滤** |

源码里每种源都是同样的合并方式（`builder/controller.go` `doWatch()`）：

```go
allPredicates := append([]predicate.Predicate(nil), blder.globalPredicates...)
allPredicates = append(allPredicates, blder.forInput.predicates...)   // Owns 用 own.predicates
src := source.TypedKind(cache, obj, hdler, allPredicates...)
```

所以：**`globalPredicates` 每个源都加一次，`forInput.predicates` 只加给 For。**

## 实测结果（2026-11-04，namespace=test）

```text
16:59:50  create demo-cm
16:59:54  patch data（label 不变）
16:59:57  label 打上 stage=dev
17:00:01  delete demo-cm
```

| 事件 | `cm-all` | `cm-label-changed` | `cm-create-only` |
|---|---|---|---|
| create（无 label） | ✅ | ✅ | ✅ |
| 改 data（label 不变） | ✅ | ❌ | ❌ |
| 改 label | ✅ | ✅ | ❌ |
| delete | ✅ | ✅ | ❌ |

三条日志对照：

```text
16:59:50  {"controller":"cm-all",           "name":"demo-cm"}   ← create
16:59:54  {"controller":"cm-all",           "name":"demo-cm"}   ← 只有 A 收到 data 变更
16:59:57  {"controller":"cm-label-changed", "name":"demo-cm"}   ← 只有 B 收到 label 变更（A 也收到）
17:00:01  {"controller":"cm-label-changed", "name":"demo-cm"}   ← delete 也放行
```

注意 `cm-label-changed` **在 delete 时也触发了**：`LabelChangedPredicate` 只重写了 Update 的判断，
Create / Delete / Generic 默认全部返回 `true`。

## 启动时也有过滤

启动瞬间只有一个 `kube-root-ca.crt`，三个控制器**都收到了**（初始 List 产生的是 Create 事件）：

```text
INFO 事件通过过滤  {"controller":"cm-label-changed","watcher":"B-只标签变化"}
INFO 事件通过过滤  {"controller":"cm-create-only", "watcher":"C-只Create"}
INFO 事件通过过滤  {"controller":"cm-all",         "watcher":"A-无过滤"}
```

如果 `createOnly` 连 Create 都拒绝，那么启动瞬间它一次也不会触发。

## 三个控制器共用一个 informer

日志里有 **3 条** `Starting EventSource`，但它们**共用同一个 ConfigMap informer**：

```text
cache（每个 GVK 一个 informer）
   ├── handler A（无过滤）
   ├── handler B（LabelChanged）
   └── handler C（createOnly）
```

每个 handler 自己带 predicate，过滤后各自入自己的 workqueue。

好处：apiserver 只承受 **一次** List + Watch，不是三次。

## 源码位置

`pkg/builder/controller.go` 的 `doWatch()`：

```go
allPredicates := append([]predicate.Predicate(nil), blder.globalPredicates...)
allPredicates = append(allPredicates, blder.forInput.predicates...)
src := source.TypedKind(blder.mgr.GetCache(), obj, hdler, allPredicates...)
blder.ctrl.Watch(src)
```

predicate 被交给 **source**，由 informer 的事件分发阶段调用，所以：

```text
谓词返回 false → 不入队 → 不占 worker → 不写日志
```

## 常见 predicate

| predicate | 用途 |
|---|---|
| `GenerationChangedPredicate{}` | 只看 spec 变化（要求对象有 `metadata.generation`） |
| `LabelChangedPredicate{}` | 只看 label 变化 |
| `AnnotationChangedPredicate{}` | 只看 annotation 变化 |
| `ResourceVersionChangedPredicate{}` | 过滤掉 resync 造成的假更新 |
| `predicate.Funcs{...}` | 自己写四个函数 |

> `ConfigMap` 没有 `spec`，`generation` 不增长，所以 `GenerationChangedPredicate` 在这里会**全部拒绝**。这也是本节没用它的原因。

## 观察清单

| 现象 | 说明 |
|---|---|
| 改 data 只有 `cm-all` 触发 | predicate 生效点在入队前 |
| 改 label 后 B 被触发 | `LabelChangedPredicate` 比较 labels |
| delete 时 B 仍被触发 | 只重写了 Update 判断 |
| 3 条 `Starting EventSource` | 控制器各注册一个 handler |
| 启动瞬间三个控制器都触发一次 | 初始 List 是 Create 事件 |

## 常见问题

| 现象 | 原因 |
|---|---|
| 控制器完全不触发 | predicate 把所有事件都拒绝了（如 ConfigMap 配 GenerationChanged） |
| 删除后收不到事件 | `DeleteFunc` 返回 false |
| `WithEventFilter` 影响了 `Owns` | 它是控制器级过滤，作用所有事件源 |

## 下一步

- `Build()` / `Complete()` 的完整装配流程（M5）
- `MaxConcurrentReconciles` 与队列行为（M6）
- 控制器进 `LeaderElection` 组还是 `Others` 组（M7）