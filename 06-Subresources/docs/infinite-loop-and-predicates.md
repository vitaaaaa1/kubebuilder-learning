# 无限循环问题与 Predicate 使用方法

## 一、为什么会进入无限循环？

### 1.1 循环链路

```
Reconcile → r.Update()/r.Status().Update() → RV 变化
       → Informer 捕获 Update 事件 → 再次入队 → Reconcile 再次触发
       → 无限循环 ♻️
```

### 1.2 触发条件

| 操作 | 是否改变 RV | Informer 是否触发 | 说明 |
|------|------------|------------------|------|
| 更新 `Spec` | ✅ 是 | ✅ 是 | 最常见 |
| 更新 `Status` | ✅ 是 | ✅ 是 | `.For()` 也监听 Status |
| 更新 `Annotation` | ✅ 是 | ✅ 是 | 本文实验的场景 |
| 更新 `Label` | ✅ 是 | ✅ 是 | 同上 |
| 值相同（幂等） | ❌ 否 | ❌ 否 | API Server 优化：值不变则 RV 不变 |

### 1.3 导致无限循环的代码模式

**模式 1：每次写入不同的值**
```go
app.Annotations["last-reconciled"] = time.Now().Format(time.RFC3339Nano)
r.Update(ctx, &app)  // ← 每次不同 → 持续循环
```

**模式 2：无条件更新 Status**
```go
app.Status.Phase = "Running"
r.Status().Update(ctx, &app)  // ← 如果值不变，自然收敛；否则循环
```

---

## 二、避免无限循环的 5 种方式

### 方式 1：幂等性守卫（最常用）

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app command.DemoApp
    if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // ✅ 幂等性检查：如果已经是目标状态，跳过
    if app.Status.Phase == "Running" {
        return ctrl.Result{}, nil  // 不触发 Update，循环停止
    }

    // 只在需要时才 Update
    app.Status.Phase = "Running"
    return ctrl.Result{}, r.Status().Update(ctx, &app)
}
```

**适用场景：** 你只关心"最终状态"，不关心"更新频率"。

### 方式 2：使用 `GenerationChangedPredicate`（只响应 Spec 变化）

```go
import "sigs.k8s.io/controller-runtime/pkg/predicate"

ctrl.NewControllerManagedBy(mgr).
    For(&command.DemoApp{}).
    WithEventFilter(predicate.GenerationChangedPredicate{}).  // ← 只响应 Spec
    Complete(&MyReconciler{...})
```

**原理：**
- `generation` 只在 **Spec 变化** 时递增（CRD 的 Annotation 变化不改变 generation）
- `GenerationChangedPredicate` 比较 `oldObj.Generation` vs `newObj.Generation`
- 如果相同（如 Status/Annotation 变化），过滤掉事件

**适用场景：** 你的 Controller 只关心 Spec 修改。

### 方式 3：使用 `ResourceVersionChangedPredicate`

```go
ctrl.NewControllerManagedBy(mgr).
    For(&command.DemoApp{}).
    WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
    Complete(&MyReconciler{...})
```

**原理：** 比较 `oldObj.ResourceVersion` vs `newObj.ResourceVersion`。

**适用场景：** 几乎不使用（这是默认行为），除非你想过滤掉某些 RV 不变的事件。

### 方式 4：自定义 Predicate 精确控制

```go
import (
    "sigs.k8s.io/controller-runtime/pkg/event"
    "sigs.k8s.io/controller-runtime/pkg/predicate"
)

// 只响应 Spec.Image 变化
type SpecImageChangedPredicate struct {
    predicate.Funcs
}

func (SpecImageChangedPredicate) Update(e event.UpdateEvent) bool {
    if e.ObjectOld == nil || e.ObjectNew == nil {
        return false
    }
    oldObj := e.ObjectOld.(*command.DemoApp)
    newObj := e.ObjectNew.(*command.DemoApp)
    // 只比较 Spec.Image，忽略其他字段
    return oldObj.Spec.Image != newObj.Spec.Image
}

// 使用
ctrl.NewControllerManagedBy(mgr).
    For(&command.DemoApp{}).
    WithEventFilter(SpecImageChangedPredicate{}).
    Complete(&MyReconciler{...})
```

**适用场景：** 你只关心特定字段（如 `Spec.Image`）的变化。

### 方式 5：使用 `RequeueAfter` 替代持续更新

```go
func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app command.DemoApp
    if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 只在首次初始化
    if app.Status.Phase == "" {
        app.Status.Phase = "Running"
        r.Status().Update(ctx, &app)
    }

    // 定期轮询，而不是持续更新
    return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}
```

**适用场景：** 你需要定期检查资源状态（如健康检查），但不需要每次变化都触发。

---

## 三、Predicate 详解

### 3.1 Predicate 是什么？

Predicate 是一个**过滤器**，在事件进入 WorkQueue 之前拦截它。

```
事件 (Create/Update/Delete)
       ↓
   Predicate?  ← 过滤：返回 true 则放行，false 则丢弃
       ↓
   WorkQueue  ← 只有放行的事件入队
       ↓
   Reconcile
```

### 3.2 Predicate 接口

```go
type Predicate interface {
    Create(event.CreateEvent) bool   // 创建事件过滤
    Update(event.UpdateEvent) bool   // 更新事件过滤
    Delete(event.DeleteEvent) bool   // 删除事件过滤
    Generic(event.GenericEvent) bool // 通用事件过滤
}
```

### 3.3 controller-runtime 内置 Predicate

| Predicate | 用途 | 何时使用 |
|-----------|------|---------|
| `GenerationChangedPredicate{}` | 过滤掉非 Spec 变化 | **最常用** |
| `ResourceVersionChangedPredicate{}` | 过滤掉 RV 不变的事件 | 很少用 |
| `AnnotationChangedPredicate{}` | 只响应 Annotation 变化 | 特殊场景 |
| `LabelChangedPredicate{}` | 只响应 Label 变化 | 特殊场景 |
| `NoReconcilePredicate{}` | 丢弃所有事件 | 调试/测试 |

### 3.4 Predicate 组合使用

```go
ctrl.NewControllerManagedBy(mgr).
    For(&command.DemoApp{}).
    WithEventFilter(predicate.GenerationChangedPredicate{}).   // 过滤 1
    WithEventFilter(predicate.ResourceVersionChangedPredicate{}). // 过滤 2
    Complete(&MyReconciler{...})
```

**多个 Predicate 是 AND 关系：** 所有 Predicate 都返回 `true` 才放行。

### 3.5 自定义 Predicate 模板

```go
type MyPredicate struct {
    predicate.Funcs  // 提供默认实现（全部返回 true）
}

func (MyPredicate) Create(e event.CreateEvent) bool {
    // 过滤 Create 事件
    return true  // 放行
}

func (MyPredicate) Update(e event.UpdateEvent) bool {
    oldObj := e.ObjectOld.(*command.DemoApp)
    newObj := e.ObjectNew.(*command.DemoApp)
    // 自定义逻辑：只响应特定字段变化
    return oldObj.Spec.Image != newObj.Spec.Image
}

func (MyPredicate) Delete(e event.DeleteEvent) bool {
    // 过滤 Delete 事件
    return true
}
```

---

## 四、实验代码：`cmd/WithEventFilter-predicate/main.go`

### 4.1 当前代码导致无限循环的原因

```go
func (r *StatusDemoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var app command.DemoApp
    r.Get(ctx, req.NamespacedName, &app)

    // ❌ 每次写入不同的时间戳 → RV 持续变化 → 持续循环
    app.Annotations["last-reconciled"] = time.Now().Format(time.RFC3339Nano)
    r.Update(ctx, &app)

    return ctrl.Result{}, nil
}
```

### 4.2 修复方式 1：添加幂等性守卫

```go
// 如果 Annotation 已存在，跳过 Update
if _, exists := app.Annotations["last-reconciled"]; exists {
    logger.Info("Already reconciled, skipping")
    return ctrl.Result{}, nil
}
```

### 4.3 修复方式 2：使用 `GenerationChangedPredicate`

```go
ctrl.NewControllerManagedBy(mgr).
    For(&command.DemoApp{}).
    WithEventFilter(predicate.GenerationChangedPredicate{}).  // ← 过滤 Status/Annotation 变化
    Complete(&StatusDemoReconciler{...})
```

**注意：** 这个实验的目的是**故意制造无限循环**来观察行为。如果是生产代码，应该使用上述修复方式。

---

## 五、总结

| 问题 | 解决方式 | 代码模式 |
|------|---------|---------|
| 写入不同值导致循环 | 幂等性守卫 | `if obj.Status == target { return }` |
| Status 变化触发循环 | `GenerationChangedPredicate` | `.WithEventFilter(predicate.GenerationChangedPredicate{})` |
| 只关心特定字段 | 自定义 Predicate | `return old.Spec.X != new.Spec.X` |
| 需要定期轮询 | `RequeueAfter` | `return ctrl.Result{RequeueAfter: 5*time.Minute}, nil` |
| 首次初始化后不再更新 | 条件判断 | `if obj.Status.Phase == "" { Update }` |
