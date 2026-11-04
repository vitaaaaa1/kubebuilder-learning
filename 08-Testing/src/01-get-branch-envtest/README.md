# 01-get-branch-envtest：在最小调用上加逻辑

## 这个目录和 00 的关系

| | [00-get-branch-envtest](../00-get-branch-envtest/) | **01-get-branch-envtest（本目录）** |
|---|---|---|
| 回答的问题 | 一条测试**怎么跑起来** | 在能跑的基础上，**怎么加东西** |
| 被测逻辑 | `Get` + `IgnoreNotFound` | 同上 **+ 两行日志** |
| 用例数 | 1 | **3** |
| 辅助代码 | 无 | `dumpK8sError` |
| 配套文档 | 七层链路（`make` → `PASS`） | **本篇**：日志 / 错误 / UID 三件事 |

**00 讲“怎么跑”，01 讲“跑起来之后看什么”。**

如果还没读过 00 的链路文档，先看 [00-get-branch-envtest/README.md](../00-get-branch-envtest/README.md)。

---

## 起点：被测逻辑只多了两行日志

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap

	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("reconcile 开始", "key", req.NamespacedName.String())

	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("读到 ConfigMap",
		"name", cm.Name,
		"namespace", cm.Namespace,
		"resourceVersion", cm.ResourceVersion)

	return ctrl.Result{}, nil
}
```

行为**没有变化**（NotFound 仍被吞成 nil），只是多了观测点。

---

## 练习 1：在 Reconcile 里打日志

### 框架栈

```text
你的代码
  │  logr.Logger（接口：Info / Error / V）
  ▼
sigs.k8s.io/controller-runtime/pkg/log     ← 全局入口 + ctx 注入
  ▼
sigs.k8s.io/controller-runtime/pkg/log/zap ← 适配器
  ▼
go.uber.org/zap                             ← 真正干活
```

**接口是 `logr`，实现是 `zap`。** 换实现不用改 Reconcile。

### 三个 API

```go
log.Info("消息", "key1", val1, "key2", val2)
log.Error(err, "消息", "key", val)
log.V(1).Info("调试消息", "key", val)
```

| 方法 | 级别 | 用途 |
|---|---|---|
| `Info` | 0 | 状态变化、结果 |
| `V(1).Info` | 1（debug） | 排查细节，生产可关 |
| `Error` | error | 出错，自动带 error 文本 |

**第一个参数必须是 string**，变量要作为 key/value 传。

### 用 `LoggerFrom(ctx)` 而不是 `ctrl.Log`

Manager 调度 Reconcile 前会把一个**带请求字段**的 logger 放进 ctx：

```text
controller / controllerKind / namespace / name / reconcileID
```

用 `LoggerFrom(ctx)` 这些字段自动带上。

本目录的测试**没有 Manager**，ctx 里没有注入 logger，会**回退到全局 logger**——即 `suite_test.go` 里：

```go
logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))
```

### 真实输出

```text
DEBUG  reconcile 开始      {"key": "default/simple-1793773811852647000"}
INFO   读到 ConfigMap      {"name": "simple-1793773811852647000", "namespace": "default", "resourceVersion": "197"}
```

### 踩坑

```go
log.Info(&cm.Name)
```

```text
reconciler.go:36:11: cannot use &cm.Name (value of type *string) as string value in argument to log.Info
```

正确写法：`log.Info("读到 ConfigMap", "name", cm.Name)`。

---

## 练习 2：把错误完整打印出来

### 为什么 `%v` 只有一句话

```go
// k8s.io/apimachinery/pkg/api/errors
type StatusError struct {
	ErrStatus metav1.Status       // 完整信息在这里
}

func (e *StatusError) Error() string {
	return e.ErrStatus.Message    // ← %v 只显示这个
}
```

### `dumpK8sError` 的两种写法

```go
json.Marshal(err)              // → {"ErrStatus": { ... }}   多一层外壳
json.Marshal(se.ErrStatus)     // → { ... }                  apiserver 原始 Status
```

`err` 的实际类型是 `*apierrors.StatusError`，直接把 `err` 序列化，输出的是**这个结构体的字段**：

```json
{
  "ErrStatus": {
    "metadata": {},
    "status": "Failure",
    "message": "configmaps \"definitely-not-exist\" not found",
    "reason": "NotFound",
    "details": { "name": "definitely-not-exist", "kind": "configmaps" },
    "code": 404
  }
}
```

去掉外壳才是原始响应。取 `se` 的唯一办法：

```go
var se *apierrors.StatusError
if errors.As(err, &se) {
	b, _ := json.MarshalIndent(se.ErrStatus, "", "  ")
	...
}
```

### 为什么是 `&se`（指针的指针）

```text
se         类型 *StatusError      值 nil → 指向对象
&se        类型 **StatusError     指向"装指针的那个变量"
```

`errors.As` 内部要做的是 `*target = 找到的对象`：

| target 类型 | `*target` 类型 | 能写入 `*StatusError` 吗 |
|---|---|---|
| `*StatusError` | `StatusError`（结构体） | ❌ |
| `**StatusError` | `*StatusError` | ✅ |

传 `se` 会 panic：

```text
errors: target must be a non-nil pointer
```

### fallback

不是所有错误都来自 apiserver（网络、超时、context 取消），`errors.As` 返回 false 时退化成 `err.Error()`，避免对 nil 取字段。

---

## 练习 3：观察 UID

`TestEnv_ReconcileOnExistingObject` 会打印：

```text
apiserver 分配的 UID = 879a5f56-3975-41e3-973e-1526addb8282，ResourceVersion = 197
```

实测三条事实：

| 操作 | UID | resourceVersion |
|---|---|---|
| Create | `992c9176-...`（生成） | 192 |
| Update | `992c9176-...`（**不变**） | 193 |
| Delete + 同名 Create | `2fe83bdd-...`（**变了**） | 195 |

**name 是地址（可重建），UID 是身份证号（永不复用）。**

为什么重要：`ctrl.SetControllerReference` 写进 `ownerReferences` 的就是 UID，GC 靠它判断 owner 是否还在。fake client 不生成 UID，所以 **ownerRef / GC 的断言只能在 envtest 里写**。

---

## 三条用例

| 用例 | 验证什么 | 输出 |
|---|---|---|
| `TestEnv_ReconcileOnMissingObject` | 不存在 → 返回 nil | PASS |
| `TestEnv_ReconcileOnExistingObject` | 存在 → nil，并观察 UID | `UID = 879a5f56-... RV = 197` |
| `TestEnv_NotFoundIsAnError` | NotFound 是错误而不是空值 | `IsNotFound(err) = true` |

第三条**不调用 `Reconcile`**：它直接问客户端"Get 不存在的对象返回什么"，用来钉死 `IgnoreNotFound` 的前提。

---

## 运行

```bash
cd 08-Testing/src/01-get-branch-envtest

make setup        # 一次性（若 ../bin 里还没有 setup-envtest）
make test-simple
```

```text
=== RUN   TestEnv_ReconcileOnMissingObject
--- PASS: TestEnv_ReconcileOnMissingObject (0.00s)
=== RUN   TestEnv_ReconcileOnExistingObject
    env_test.go:105: apiserver 分配的 UID = 879a5f56-..., ResourceVersion = 197
    INFO 读到 ConfigMap {"name": "simple-...", "namespace": "default", "resourceVersion": "197"}
--- PASS: TestEnv_ReconcileOnExistingObject (0.00s)
=== RUN   TestEnv_NotFoundIsAnError
    env_test.go:130: 原始错误 = configmaps "definitely-not-exist" not found
    env_test.go:131: apierrors.IsNotFound(err) = true
--- PASS: TestEnv_NotFoundIsAnError (0.00s)
PASS
ok  .../01-get-branch-envtest  3.9s
```

只想跑一条：

```bash
KUBEBUILDER_ASSETS="$(../bin/setup-envtest use 1.31.x -p path)" \
  go test . -run TestEnv_NotFoundIsAnError -v
```

---

## 踩坑清单（本目录真实遇到的）

| 现象 | 原因 | 处理 |
|---|---|---|
| `cannot use &cm.Name (*string) as string` | `Info(msg, kv...)` 第一参数必须是 string | 写成 `"name", cm.Name` |
| `panic: errors: target must be a non-nil pointer` | 传了 `se` 而不是 `&se` | `errors.As(err, &se)` |
| JSON 多了 `ErrStatus` 外壳 | 序列化的是 `err`（结构体），不是内部 Status | `json.Marshal(se.ErrStatus)` |
| `make setup` 输出里混着中文注释 | recipe 里 **Tab 开头的 `#`** 被 shell 当注释并 echo | 删掉或移到行首 |
| 文件头注释还写 `Package envtestsimple` | 目录改过名 | 改成 `Package getbranchenvtest` |

---

## 在这个目录还能加什么

| 想练 | 怎么加 | 涉及 |
|---|---|---|
| 故意失败 | `reconciler.go` 改成 `return ctrl.Result{}, err` | 看 `dumpK8sError` 在失败分支的输出 |
| 表驱动 | 新建 `table_test.go` | `t.Run`、`-run 'TestX/case'` |
| 负向断言 | 断言 `IsNotFound(err) == true` | 证明错误类型没被吞 |
| 缓存对比 | 新建 `02-...` 目录 | `mgr.GetClient()` vs `GetAPIReader()` |

**建议**：新实验放新目录（`02-`、`03-`），保持 01 是"能跑的最小加法样例"。