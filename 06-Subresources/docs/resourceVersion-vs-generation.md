# ResourceVersion vs Generation

本文档记录 Kubernetes 中 `metadata.resourceVersion` 和 `metadata.generation` 的本质区别，以及它们如何影响 Controller 的行为。

---

## 1. 一句话总结

| 字段 | 一句话 |
|---|---|
| `resourceVersion` | **etcd 的全局事务 ID**——任何写入都递增，不区分写了什么。 |
| `generation` | **Spec 的版本号**——只有 Spec 变化时才递增，Status 和 Annotation 不影响它。 |

---

## 2. 详细对比

| 维度 | `resourceVersion` | `metadata.generation` |
|---|---|---|
| **谁分配的？** | API Server（底层是 etcd 的全局 Revision） | API Server |
| **什么操作会变？** | **任何写入**（Spec、Status、Annotation） | **只有 Spec** 写入 |
| **Status 更新会变吗？** | ✅ 变 | ❌ 不变 |
| **Annotation 更新会变吗？** | ✅ 变 | 看资源类型（CRD 通常不变） |
| **范围** | **集群全局**（所有资源共用同一个 etcd Revision 池） | **单个资源**（每个资源独立递增） |
| **能手动设置吗？** | ❌ 不能 | ❌ 不能 |
| **会回退吗？** | ❌ 永远递增 | ❌ 永远递增 |
| **典型用途** | 乐观锁（防止并发覆盖）、Informer 事件流 | 判断"用户是否改了期望状态" |
| **Controller 关心它吗？** | Informer 依赖它触发事件 | Controller 用 `GenerationChangedPredicate` 决定是否响应 |

---

## 3. 工作流程：一个完整例子

### 3.1 初始状态

```yaml
metadata:
  resourceVersion: "1000"
  generation: 1
spec:
  replicas: 3
status:
  phase: Pending
```

### 3.2 用户改了 `spec.replicas: 3 → 5`

```yaml
metadata:
  resourceVersion: "1001"    # ✅ 变了（etcd 写入了新数据）
  generation: 2              # ✅ 变了（Spec 变了）
spec:
  replicas: 5
status:
  phase: Pending
```

### 3.3 Controller 更新了 `status.phase: Running`

```yaml
metadata:
  resourceVersion: "1002"    # ✅ 变了（etcd 写入了新数据）
  generation: 2              # ❌ 没变（Status 不影响 generation）
spec:
  replicas: 5
status:
  phase: Running
```

### 3.4 用户打了 Annotation

```yaml
metadata:
  resourceVersion: "1003"    # ✅ 变了
  generation: 2              # CRD 不变（Deployment 等内置资源会变）
  annotations:
    my-annotation: "hello"
spec:
  replicas: 5
status:
  phase: Running
```

---

## 4. 和 Controller 的关系

### 4.1 触发链

```
API Server 写入（Spec 或 Status 或 Annotation）
        ↓
  resourceVersion 递增（必然发生）
        ↓
  Informer 看到 RV 变了 → 发送 Update 事件
        ↓
  检查 Predicate（如果有的话）
        ↓
  Predicate 放行 → 入队 → Reconcile
```

### 4.2 `.For()` 的默认行为

```go
ctrl.NewControllerManagedBy(mgr).
    For(&DemoApp{}).           // ← 没有 Predicate
    Complete(&Reconciler{})
```

**默认只用了 `EnqueueRequestForObject`**——只要 Informer 收到 Update 事件（即 RV 变了），就入队触发 Reconcile。

| 什么变了？ | RV 变吗？ | generation 变吗？ | 默认 `.For()` 触发？ |
|---|---|---|---|
| `spec.replicas` | ✅ | ✅ | ✅ |
| `status.phase` | ✅ | ❌ | ✅ |
| `metadata.annotations` | ✅ | ❌（CRD） | ✅ |

### 4.3 加 `GenerationChangedPredicate` 后

```go
ctrl.NewControllerManagedBy(mgr).
    For(&DemoApp{}).
    WithEventFilter(predicate.GenerationChangedPredicate{}).  // ← 核心
    Complete(&Reconciler{})
```

**`GenerationChangedPredicate` 的源码逻辑**：

```go
func (TypedGenerationChangedPredicate[object]) Update(e event.TypedUpdateEvent[object]) bool {
    return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}
```

| 什么变了？ | generation 变吗？ | 加 Predicate 后触发？ |
|---|---|---|
| `spec.replicas` | ✅ | ✅（generation 变了，放行） |
| `status.phase` | ❌ | ❌（generation 没变，拦截） |
| `metadata.annotations` | ❌（CRD） | ❌ |

**这就是为什么生产环境推荐加它：** 避免"自己改 Status → Informer 触发 → 又进 Reconcile → 又改 Status"的死循环。

---

## 5. `r.Status().Update()` 为什么不改变 `generation`

### 5.1 核心原理

**前提：CRD 必须启用 `status` 子资源**

```yaml
# CRD 定义中必须有这个 marker
// +kubebuilder:subresource:status
type DemoApp struct { ... }
```

当 CRD 启用 `status` 子资源后，API Server 将 `/status` 端点和 `/` 端点**完全隔离**：

| 端点 | 能修改什么 | 影响 `generation`？ |
|------|-----------|-------------------|
| `PUT /apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/name` | `spec` + `metadata` | ✅ 是 |
| `PUT /apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/name/status` | **只** `status` | ❌ 否 |

### 5.2 代码层面

```go
// r.Status().Update() 实际调用的是 /status 端点
r.Status().Update(ctx, &app)
// 等价于 client-go:
// client.Put().Resource("demoapps").Name(name).SubResource("status").Body(&app).Do()
```

因为请求路径包含 `.SubResource("status")`，API Server 知道这是 **status-only 写入**，所以：
- ✅ `resourceVersion` 递增（etcd 写入了新数据）
- ❌ `generation` 不变（Spec 没有变化）

### 5.3 如果 CRD **没有**启用 `status` 子资源

```yaml
# 没有 // +kubebuilder:subresource:status marker
type DemoApp struct { ... }
```

此时 `r.Status().Update()` 会返回 **404 Not Found**，因为 `/status` 端点不存在。

如果用 `r.Update()`（非 Status 子资源）更新包含 Status 的对象：
- ✅ `resourceVersion` 递增
- ✅ `generation` **可能递增**（如果 API Server 认为整个 Object 都变了）

### 5.4 实验验证

```bash
# 1. 初始状态
kubectl get demoapp status-demo -n test -o yaml | grep generation
# generation: 1

# 2. 修改 Spec
kubectl patch demoapp status-demo -n test --type merge -p '{"spec":{"replicas":5}}'
kubectl get demoapp status-demo -n test -o yaml | grep generation
# generation: 2  ← 变了

# 3. 运行 Controller，更新 Status
# (r.Status().Update() 将 Phase 改为 "Running")
kubectl get demoapp status-demo -n test -o yaml | grep generation
# generation: 2  ← 没变！
```

### 5.5 实际意义

| 场景 | generation 变吗？ | 会触发 `.For()` 的 Predicate 吗？ |
|------|-----------------|--------------------------------|
| `r.Update()` 改 Spec | ✅ 是 | ✅ `GenerationChangedPredicate` 放行 |
| `r.Status().Update()` 改 Status | ❌ 否 | ❌ `GenerationChangedPredicate` 拦截 |
| `r.Status().Patch()` 改 Status | ❌ 否 | ❌ `GenerationChangedPredicate` 拦截 |

**这就是为什么生产 Controller 推荐加 `GenerationChangedPredicate`：**
- 用户改 Spec → generation 变 → Reconcile 触发 → Controller 更新 Status
- Status 更新 → generation **不变** → Predicate 拦截 → **不会**再次触发 Reconcile
- **避免了"自己写 Status → 自己触发 → 又写 Status"的死循环**

---

## 6. 常见误区

### ❌ 误区 1：".For() 默认只监听 Spec 变化"

**不对**。`.For()` 默认监听**任何变化**（RV 变了就触发）。`generation` 只是 `GenerationChangedPredicate` 的过滤指标，不是默认行为。

### ❌ 误区 2："generation 变才会触发 Informer"

**不对**。Informer **只看 `resourceVersion`**。`generation` 是 Predicate 层的过滤条件，Informer 根本不关心它。

### ❌ 误区 3："resourceVersion 是单个资源的版本号"

**不对**。`resourceVersion` 是 **etcd 集群的全局 Revision**，所有资源的写入共享同一个 etcd Revision 池。资源 A 的写入会让资源 B 的 RV 也递增。

### ❌ 误区 4："`r.Status().Update()` 会让 generation 递增"

**不对**（前提是 CRD 启用了 `status` 子资源）。`/status` 端点是隔离写入，API Server 知道这不涉及 Spec，所以不递增 generation。

---

## 7. 类比理解

| 概念 | 现实类比 | Kubernetes 对应 |
|---|---|---|
| `resourceVersion` | **小区的快递柜编号**——任何人的快递到了，柜子编号都递增。你看编号就知道"有人收了快递"，但不知道是谁。 | 任何写入都递增 |
| `generation` | **你家门的装修次数**——只有你改了自己家的装修（Spec），这个数才变。邻居装修不影响你家。 | 只有 Spec 变化才递增 |

---

## 8. 核心结论

| 维度 | `resourceVersion` | `generation` |
|---|---|---|
| **本质** | etcd 全局事务 ID | Spec 版本号 |
| **触发 Informer？** | ✅ 是（唯一入口） | ❌ 不是 |
| **用于过滤？** | ❌ 不是 | ✅ `GenerationChangedPredicate` |
| **Controller 死循环原因** | Status 更新 → RV 变 → Informer 触发 | Status 更新 → generation 不变 → Predicate 拦截 |

---

## 9. 无差异 Update/Patch 仍然是 API 调用

### 9.1 核心原理

即使 `Update()` 或 `Patch()` 发送的数据 **与服务器端完全相同**，它仍然是：

1. 一次完整的 **HTTP 请求**（网络传输）
2. 通过 API Server 的 **鉴权** 和 **准入控制**
3. API Server 执行 **3-way merge** 比较数据
4. 发现无变化 → **不写 etcd** → RV 不变

**代码层面：**
```go
// 即使 app.Status.Phase 已经是 "Running"
app.Status.Phase = "Running"  // 相同的值
r.Status().Update(ctx, &app)  // ← 仍然发送 HTTP PUT
```

### 9.2 验证方法

#### 方法 1：`kubectl --v=8`（最简单）

```bash
# 打印所有 HTTP 请求细节
kubectl patch demoapp status-demo -n test \
  --type merge -p '{"spec":{"replicas":3}}' \
  --v=8 2>&1 | grep "HTTP"
```

输出类似：
```
I1101 12:45:23.123456  12345 round_trippers.go:454] PUT https://.../demoapps/status-demo 200 OK in 45ms
```

**证据**：即使 `replicas` 已经是 `3`，HTTP 请求仍然发出去了。

#### 方法 2：代码里加 HTTP 拦截器（最直观）

```go
import "net/http"

// 在创建 Manager 时加上 WrapTransport
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme: scheme,
})
if err != nil { ... }

// 拦截所有 HTTP 请求
mgr.GetConfig().WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
    return &loggingTransport{rt: rt}
}

type loggingTransport struct {
    rt http.RoundTripper
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    fmt.Printf("📡 HTTP %s %s\n", req.Method, req.URL.Path)
    return t.rt.RoundTrip(req)
}
```

运行后你会看到：
```
📡 HTTP PUT /apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/status-demo
📡 HTTP GET /apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/status-demo
```

**即使数据完全相同，PUT 请求仍然发出。**

#### 方法 3：`kubectl proxy` + `curl`

```bash
# 1. 启动代理
kubectl proxy --port=8080 &

# 2. 获取当前状态（保存为 JSON）
kubectl get demoapp status-demo -n test -o json > /tmp/original.json

# 3. 用相同的 JSON 再次 PUT
curl -X PUT http://localhost:8080/apis/demo.vitaaaaa.icu/v1alpha1/namespaces/test/demoapps/status-demo \
  -H "Content-Type: application/json" \
  -d @/tmp/original.json

# 4. 观察：HTTP 200，但 RV 不变
```

### 9.3 API Server 的处理流程

```
客户端发送 Update/Patch
        ↓
   HTTP 请求到达 API Server
        ↓
   鉴权 (Authentication)
        ↓
   准入控制 (Admission Controllers)
        ↓
   3-way merge 比较数据
        ↓
   ┌────── 有变化？ ──────┐
   │                     │
  是                    否
   │                     │
   ↓                     ↓
写入 etcd           不写 etcd
RV 递增             RV 不变
   │                     │
   ↓                     ↓
Informer 触发          Informer 不触发
        ↓                     ↓
   Reconcile              不触发 Reconcile
```

### 9.4 实际影响

| 场景 | 发出 HTTP？ | etcd 写入？ | RV 变？ | 触发 Reconcile？ |
|------|-----------|-----------|--------|----------------|
| `Update()` 有差异 | ✅ 是 | ✅ 是 | ✅ 是 | ✅ 是 |
| `Update()` 无差异 | ✅ 是 | ❌ 否 | ❌ 否 | ❌ 否 |
| `Patch()` 有差异 | ✅ 是 | ✅ 是 | ✅ 是 | ✅ 是 |
| `Patch()` 无差异 | ✅ 是 | ❌ 否 | ❌ 否 | ❌ 否 |

**关键结论：**
- 即使无差异，**HTTP 请求仍然发出**（有网络开销）
- 但 API Server **不写 etcd** → RV 不变 → **不触发 Reconcile**
- 这就是为什么"幂等性 Update"不会导致无限循环

### 9.5 为什么 Controller 仍然需要幂等性守卫

虽然"无差异 Update"不会触发 Reconcile，但我们仍然推荐添加幂等性守卫：

```go
if app.Status.Phase == "Running" {
    return ctrl.Result{}, nil  // 跳过 Update
}
```

**原因：**
1. **减少不必要的 HTTP 请求**（网络开销）
2. **减少 API Server 负载**（鉴权、准入控制、3-way merge）
3. **降低 409 Conflict 风险**（减少并发写入的概率）

---

*上一步：[独立 RV 与隔离原理](./02-subresource-mechanism.md)*
