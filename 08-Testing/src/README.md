# src/：三个递进的 envtest 沙盒

这里放三个逐步加码的沙盒。**00 打地基，01 加观测，02 加时序**。

```text
src/
├── 00-get-branch-envtest/     # 最小调用：只证明"测试能跑起来"
├── 01-get-branch-envtest/     # 加日志、加错误 dump、加 UID 观察
└── 02-manager-and-cache/      # Manager 与缓存：读快 400 倍，但会滞后
```

前两个目录共用同一个包名 `getbranchenvtest`，第三个是独立的包 `managercache`；**三者互不引用**，各跑各的。

---

## 00-get-branch-envtest —— 最小调用

### 目的

回答一个问题：**一条测试从 `make` 到 `PASS`，中间发生了什么？**

它刻意把变量减到最少：一个控制器、一个分支、一条用例。

### 文件

| 文件 | 行数级别 | 职责 |
|---|---|---|
| `reconciler.go` | ~26 | 被测对象：`Get` + `IgnoreNotFound` |
| `suite_test.go` | ~64 | `TestMain`：起 etcd + apiserver，造 `k8sClient` |
| `env_test.go` | ~35 | 一条用例：对象不存在 → 返回 nil |
| `Makefile` | ~27 | `make setup` / `make test-simple` |
| `README.md` | — | **七层完整链路**（建议先读它） |

### 被测逻辑

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return ctrl.Result{}, nil
}
```

### 一次运行

```bash
cd src/00-get-branch-envtest
make setup        # 一次性
make test-simple
```

```text
=== RUN   TestEnv_ReconcileOnMissingObject
--- PASS: TestEnv_ReconcileOnMissingObject (0.00s)
PASS
ok  .../00-get-branch-envtest  4.7s
```

### 学到的三件事

1. **测试是被生成的 `main` 调起来的**（不是 `m.Run()` 里写了名字）
2. **`NotFound` 是错误，不是空值**：`Get` 返回 `*apierrors.StatusError`
3. **PASS 的判定**：没调用过 `t.Fatalf`，而不是返回了 true

> 完整拆解见该目录下的 [README.md](00-get-branch-envtest/README.md)。

---

## 01-get-branch-envtest —— 加"其他逻辑"

### 目的

在 00 的基础上，练习**测试之外的三类东西**（细节见本目录的 [README.md](01-get-branch-envtest/README.md)）：

- 控制器里怎么打日志
- 错误怎么完整打印
- 对象身份（UID）怎么观察

### 与 00 的差异

| 文件 | 00 | 01 |
|---|---|---|
| `reconciler.go` | 只有 Get 分支 | 多两行日志（`LoggerFrom` + `V(1)`/`Info`） |
| `env_test.go` | 1 条用例 | **3 条用例** + `dumpK8sError` 工具函数 |
| `suite_test.go` | 相同 | 相同 |
| `Makefile` | 相同 | 相同 |

### 新增点 1：日志

```go
log := ctrl.LoggerFrom(ctx)
log.V(1).Info("reconcile 开始", "key", req.NamespacedName.String())

// ... Get 成功之后
log.Info("读到 ConfigMap",
	"name", cm.Name,
	"namespace", cm.Namespace,
	"resourceVersion", cm.ResourceVersion)
```

真实输出（`make test-simple` 时可见）：

```text
DEBUG  reconcile 开始      {"key": "default/simple-1793773811852647000"}
INFO   读到 ConfigMap      {"name": "simple-1793773811852647000", "namespace": "default", "resourceVersion": "197"}
```

要点：

- **接口是 `logr`，实现是 `zap`**
- 用 `ctrl.LoggerFrom(ctx)`，不要用 `ctrl.Log`（前者能带请求字段）
- `Info(msg, kv...)` 的第一个参数**必须是 string**；`log.Info(&cm.Name)` 会编译失败
- `V(1)` 是 debug 级别，`UseDevMode(true)` 下可见

### 新增点 2：完整错误 dump

```go
func dumpK8sError(err error) string { ... }
```

对比两种序列化：

```go
json.Marshal(err)             // 外层包了一层 {"ErrStatus": {...}}
json.Marshal(se.ErrStatus)    // 直接是 apiserver 的 Status
```

真实结果：

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

去掉 `ErrStatus` 外壳，才是**原始响应**的结构。要拿到 `se`，就得用 `errors.As(err, &se)`——这也解释了"为什么传指针的指针"。

### 新增点 3：UID 观察

```go
t.Logf("apiserver 分配的 UID = %s，ResourceVersion = %s", cm.UID, cm.ResourceVersion)
```

实测三条事实：

| 操作 | UID | resourceVersion |
|---|---|---|
| Create | `992c9176-...`（生成） | 192 |
| Update | `992c9176-...`（**不变**） | 193 |
| Delete + 同名 Create | `2fe83bdd-...`（**变了**） | 195 |

**name 是地址，可以重建；UID 是身份证号，永不复用。**
`SetControllerReference` 写进 ownerReferences 的就是 UID，所以 fake（UID 为空）下测不出真实的 GC 行为。

### 三条用例

| 用例 | 验证什么 | 真实输出 |
|---|---|---|
| `TestEnv_ReconcileOnMissingObject` | NotFound → nil | PASS |
| `TestEnv_ReconcileOnExistingObject` | 对象存在 → nil，并打印 UID | `UID = 879a5f56-... RV = 197` |
| `TestEnv_NotFoundIsAnError` | NotFound 确实是错误 | `IsNotFound(err) = true` |

### 运行

```bash
cd src/01-get-branch-envtest
make test-simple
```

```text
=== RUN   TestEnv_ReconcileOnMissingObject
--- PASS (0.00s)
=== RUN   TestEnv_ReconcileOnExistingObject
    env_test.go:105: apiserver 分配的 UID = 879a5f56-...,  ResourceVersion = 197
    INFO 读到 ConfigMap {"name": "simple-...", "namespace": "default", "resourceVersion": "197"}
--- PASS (0.00s)
=== RUN   TestEnv_NotFoundIsAnError
    env_test.go:130: 原始错误 = configmaps "definitely-not-exist" not found
    env_test.go:131: apierrors.IsNotFound(err) = true
--- PASS (0.00s)
ok  .../01-get-branch-envtest  3.9s
```

---

## 02-manager-and-cache —— 缓存不是透明的

### 目的

回答一个让无数测试"偶发失败"的问题：**为什么刚创建的对象，用控制器里的 client 读不到？**

### 与 00/01 的唯一区别

`TestMain` 里多了一个 Manager：

```go
mgr, _ := ctrl.NewManager(cfg, ctrl.Options{Scheme: testScheme})
go mgr.Start(ctx)
mgr.GetCache().WaitForCacheSync(ctx)

cachedClient = mgr.GetClient()    // 读 → 缓存
apiReader    = mgr.GetAPIReader() // 读 → apiserver
```

### 三个实验（真实数据）

| 实验 | 结果 |
|---|---|
| A. 速度 | 缓存 `517–675 ns` vs 直连 `265–289 µs` → **392–558 倍** |
| B. 滞后 | 20 次"Create 后立刻缓存读"，**1–3 次 NotFound**；缓存追上耗时 `125–143 µs` |
| C. 证明 | 同一瞬间 `apiReader` 读：**20/20 立即可见** |

### 结论

```text
Create 返回  ≠  缓存已更新
Create 返回  =  数据已在 etcd
```

- **写后立刻读** → `apiReader` 或 `Eventually`
- **普通读** → 用缓存（快 400 倍）
- fake client 是同步 map，**永远复现不出**这两种现象

> 细节、`Eventually` 写法、自测题见 [02-manager-and-cache/README.md](02-manager-and-cache/README.md)。

---

## 对照表

| | 00 | 01 | 02 |
|---|---|---|---|
| 目标 | 测试怎么跑起来 | 日志、错误、身份 | 缓存与一致性 |
| client | 直连 | 直连 | **Manager 缓存 + apiReader** |
| 用例数 | 1 | 3 | 3 |
| 辅助代码 | 无 | `dumpK8sError` | 三个 client |
| 建议顺序 | 先看 | 再看 | 最后 |

**02 是分界线**：从这里开始，测试要考虑"时序"，而不只是"结果"。

---

## 踩坑清单（都是本轮真实遇到的）

| 现象 | 原因 | 处理 |
|---|---|---|
| `KUBEBUILDER_ASSETS=""`，envtest 启动失败 | Makefile 里 `LOCALBIN ?= .../bin # 注释`，**空格进了变量值** | 注释单独一行 |
| `make setup` 输出里混着中文 | recipe 里以 **Tab 开头的 `#`** 是 shell 注释，会被 echo | 删掉或改行首 |
| `cannot use &cm.Name (*string) as string` | `Info(msg, kv...)` 第一个参数必须是 string | 写成 `"name", cm.Name` |
| `errors.As(err, se)` → `panic: target must be a non-nil pointer` | 要修改 `se` 本身，必须传 `&se` | 用 `errors.As(err, &se)` |
| `json.Marshal(err)` 多了 `ErrStatus` 外壳 | `err` 是 `*StatusError`，外壳是它的字段 | `json.Marshal(se.ErrStatus)` |
| 包注释还写着 `Package envtestsimple` | 目录改过名 | 顺手改成 `Package getbranchenvtest` |

---

## 下一步可以加什么

| 方向 | 放在哪 | 涉及知识点 |
|---|---|---|
| CRD 进 envtest | 新建 03 | `CRDDirectoryPaths`、`AddToScheme` |
| Webhook | 新建 04 | `WebhookInstallOptions`、TLS |
| `Eventually` 实战 | 01 或新目录 | 写后读的断言怎么写稳 |
| 表驱动版 | 01 | `t.Run`、`-run` 过滤 |

**建议**：每加一个，复制一个目录、改包名、只动一个变量（比如 client 的构造方式），这样差异一眼可见。