# 一次 `make test-simple` 的完整链路

## 这份笔记解决什么问题

你已经知道 当前目录 里有三个文件，也知道它们"能跑通"，但说不清：

- `make test-simple` 到底执行了哪几条命令？
- 三个文件之间没有 `import`，为什么 `env_test.go` 能直接用 `k8sClient`？
- `m.Run()` 为什么能调用到 `TestEnv_ReconcileOnMissingObject`？
- 那 4 秒空白在干什么？

这篇笔记按**执行顺序**从终端一路拆到 `PASS`，每一层只讲"这一层负责什么、下一层从哪接手"。

---

## 0. 全景

```text
终端
 │  make test-simple
 ▼
① Makefile        ── 算变量，拼出一条命令（23ms）
 │  KUBEBUILDER_ASSETS="..." go test . -v -count=1
 ▼
② go test         ── 扫函数、生成 _testmain.go、编译、链接（~0.3s）
 │  ./getbranchenvtest.test -test.v -test.count=1
 ▼
③ TestMain        ── 起 etcd + apiserver，造 k8sClient（~4s）
 │  m.Run()
 ▼
④ env_test.go     ── 造 Reconciler，调 Reconcile（0.00s）
 ▼
⑤ reconciler.go   ── Get → HTTP 404 → IgnoreNotFound → nil
 ▼
⑥ 断言            ── err == nil → 没调 Fatalf → PASS
 ▼
⑦ 收尾            ── Stop() 杀子进程，os.Exit(0)
 ▼
make 退出码 0
```

**一句话**：

> Makefile 把路径算好交给 `go test`；`go test` 扫出测试函数、生成一个 `main` 并编译成二进制；二进制先跑 `TestMain` 起集群造 client，再跑你的测试函数；测试函数调 `Reconcile`，`Reconcile` 发一次 HTTP 拿到 404 并把它变成 `nil`；没有断言失败，于是 PASS。

---

## 1. 第 1 层：Makefile

### 它只有两个目标

| 目标 | 何时跑 | 干什么 |
|---|---|---|
| `make setup` | 新机器一次 | 装 `setup-envtest` + 下载 k8s 二进制 |
| `make test-simple` | 每次 | 注入 `KUBEBUILDER_ASSETS`，跑 `go test` |

> 本章早期还有其他目标（`test` / `test-env` / `test-fake` 等），后来为了让这一节只服务一个沙盒，全部删掉了。

### 语法速查

| 写法 | 含义 |
|---|---|
| `X ?= 值` | **没定义才赋值**（可被命令行/环境变量覆盖） |
| `$(X)` | 取变量 |
| `$(shell 命令)` | **解析期**执行命令，把 stdout 当字符串 |
| `目标:` | 任务名 |
| `目标: 依赖` | 执行前先跑依赖 |
| Tab + 命令 | recipe |
| `.PHONY: 目标` | 声明"这不是文件名"，永远执行 |
| `@命令` | 不打印命令本身 |
| `VAR=值 命令` | 临时环境变量，只对该命令生效 |

### 真实展开

```bash
$ make -n test-simple
KUBEBUILDER_ASSETS="/Users/vitaaaaa/Library/Application Support/io.kubebuilder.envtest/k8s/1.31.0-darwin-arm64" \
  go test . -v -count=1
```

变量来源：

```text
LOCALBIN           ?= $(shell pwd)/bin
ENVTEST            ?= $(LOCALBIN)/setup-envtest
ENVTEST_K8S_VERSION ?= 1.31.x
KUBEBUILDER_ASSETS ?= $(shell $(ENVTEST) use 1.31.x -p path 2>/dev/null)
```

### 为什么是 `-p path`

`setup-envtest` 有三种输出：

```text
$ setup-envtest use 1.31.x -p path
/Users/.../k8s/1.31.0-darwin-arm64              ← 一行干净路径

$ setup-envtest use 1.31.x -p env
export KUBEBUILDER_ASSETS='/Users/.../1.31.0-darwin-arm64'

$ setup-envtest use 1.31.x
Version: 1.31.0                                  ← 三行，没法直接塞变量
OS/Arch: darwin/arm64
Path: /Users/.../1.31.0-darwin-arm64
```

`-p path` 是**为机器读取设计**的。用默认输出会让变量变成三行文本。

### 坑：行内注释会污染变量值

```makefile
LOCALBIN ?= $(shell pwd)/bin # 注释
#                           ↑ 这个空格进变量了
```

结果：

```text
LOCALBIN = ".../00-get-branch-envtest/bin "          ← 尾巴有空格
ENVTEST  = ".../00-get-branch-envtest/bin /setup-envtest"
$(shell ...) → /bin/sh: ".../bin: No such file"
             → 2>/dev/null 吞掉错误
             → KUBEBUILDER_ASSETS=""
             → 测试启动时莫名其妙失败
```

**规矩**：注释要么放行首，要么放单独一行。
检查：`grep -n ' $' Makefile`。

### 这一层的产物

一条命令 + 一个环境变量。**Makefile 不参与测试逻辑。**

---

## 2. 第 2 层：`go test` 怎么找到测试

### 文件分两类

```bash
$ go list -f 'GoFiles:   {{.GoFiles}}
TestGoFiles: {{.TestGoFiles}}' .

GoFiles:     [reconciler.go]
TestGoFiles: [env_test.go suite_test.go]
```

| 分类 | 谁编译 | 后果 |
|---|---|---|
| `GoFiles` | `go build` 和 `go test` 都编 | 进生产二进制 |
| `TestGoFiles` | **只有** `go test` 编 | 不进生产二进制 |

### 收录规则（编译期扫描 AST）

只收满足两条的函数：

```go
func TestXxx(t *testing.T)   // 1. 名字：Test + 大写字母开头
                             // 2. 签名：恰好一个 *testing.T 参数，无返回值
```

违反的后果：

```text
# 名字错
./demo_test.go:7:6: Testfoo has malformed name: first letter after 'Test' must not be lowercase

# 签名错
wrong signature for TestBadSignature, must be: func TestBadSignature(t *testing.T)
```

> `Testfoo` 的报错来自 `go test` 默认启用的 vet。加 `-vet=off` 后它会被**静默忽略**——这是最危险的情况。

### 生成的 `_testmain.go`

`go test` 会在临时目录生成一个 `main` 包（`$WORK/b001/_testmain.go`）：

```go
var tests = []testing.InternalTest{
	{"TestEnv_ReconcileOnMissingObject", _test.TestEnv_ReconcileOnMissingObject},
}

func main() {
	m := testing.MainStart(testdeps.TestDeps{}, tests, benchmarks, fuzzTargets, examples)
	_test.TestMain(m)          // 包里定义了 TestMain 就调它
	os.Exit(int(reflect.ValueOf(m).Elem().FieldByName("exitCode").Int()))
}
```

**表里第二列是函数值，不是字符串。** 字符串只用于 `-run` 过滤和输出。

### 调用链（Go 源码）

```text
生成的 main
  └─ TestMain(m)                        ← 你写的
       └─ m.Run()                        testing/testing.go:2499
            └─ runTests(...)             testing/testing.go:2710
                 └─ t.Run(test.Name, test.F)   ← 顶层测试也是一次 t.Run
                      ├─ 匹配 -run，不匹配就 return
                      └─ tRunner → 新 goroutine → test.F(t)
                           └─ 你的测试函数体
```

### `-run` 拼错不报错

```bash
$ go test -run 'Nope' -v .
testing: warning: no tests to run
PASS
ok  	demo	0.214s [no tests to run]
exit code = 0
```

**注册失败 / 过滤掉 = 不报错。** 所以判断"测试真的跑了"要看输出里有没有 `=== RUN`。

---

## 3. 第 3 层：`TestMain` 起环境

`suite_test.go` 是**基础设施**，不是测试逻辑。它做五件事：

| 步骤 | 代码 | 作用 |
|---|---|---|
| 1 | `testEnv.Start()` | 起 etcd + kube-apiserver 两个子进程 |
| 2 | `runtime.NewScheme()` | 建"Go 类型 ↔ GVK"表 |
| 3 | `clientgoscheme.AddToScheme()` | 注册 ConfigMap / Secret 等内置类型 |
| 4 | `client.New(cfg, Options{Scheme})` | 造**直连**客户端 `k8sClient` |
| 5 | `m.Run()` | 开始跑测试 |

### envtest 是什么

```text
testEnv.Start()
  ├─ 从 KUBEBUILDER_ASSETS 找 etcd、kube-apiserver
  ├─ 起 etcd（随机端口）
  ├─ 起 kube-apiserver（随机端口，指向 etcd）
  ├─ 等健康
  └─ 返回 *rest.Config（怎么连上它）
```

**没有 kubelet、没有 scheduler、没有 controller-manager。** 所以 Pod 永远不 Running，但 ConfigMap/Secret 的 CRUD 完全正常。

### `KUBEBUILDER_ASSETS` 指向的目录

```text
$ ls -lh ~/Library/.../1.31.0-darwin-arm64/
25M   etcd
86M   kube-apiserver
54M   kubectl
```

### 关键一行

```go
k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
```

**没有 `Cache:` 字段 → 每次读都打 apiserver。** 04 小节的全部内容就是往这里加缓存。

### 为什么 `k8sClient` 是包级变量

```text
./
├── reconciler.go     package envtestsimple
├── suite_test.go     package envtestsimple   ← 在这里赋值
└── env_test.go       package envtestsimple   ← 在这里直接用
```

**一个目录 = 一个包，同包共享标识符，不需要 import。**

---

## 4. 第 4-6 层：测试 → Reconcile → PASS

### 测试函数

```go
func TestEnv_ReconcileOnMissingObject(t *testing.T) {
	ctx := context.Background()

	_, err := newReconciler().Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "definitely-not-exist",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Fatalf("对象不存在时必须返回 nil，实际返回: %v", err)
	}
}
```

`newReconciler()` = `&Reconciler{Client: k8sClient}`——把第 3 层的 client 塞进控制器。

### Reconcile

```go
var cm corev1.ConfigMap
if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
	return ctrl.Result{}, client.IgnoreNotFound(err)
}
return ctrl.Result{}, nil
```

`r.Get` 拆成三件事：

| 部分 | 说明 |
|---|---|
| `r.Get` | `Reconciler` 没定义它，来自嵌入的 `client.Client`（方法提升） |
| `req.NamespacedName` | 查谁：`default` + `definitely-not-exist` |
| `&cm` | 类型信息：`*corev1.ConfigMap` → Scheme 翻成 `/api/v1/configmaps` |

发出**真实的 HTTP 请求**：

```text
GET /api/v1/namespaces/default/configmaps/definitely-not-exist
→ 404
→ *apierrors.StatusError
→ configmaps "definitely-not-exist" not found
```

`client.IgnoreNotFound(err)`：

```text
IsNotFound(err) == true  →  返回 nil
否则                     →  原样返回
```

### PASS 的判定

```text
err = nil
  → if err != nil 条件不成立
  → 函数正常返回
  → tRunner 记录：没有调用过 Fail
  → PASS
```

**"通过"不是返回了 true，而是没调用过 `t.Fatalf`。**

`t.Fatalf` 内部是 `t.FailNow()` → `runtime.Goexit()`，只终止**当前测试的 goroutine**，其他测试照跑。

---

## 5. 第 7 层：收尾

```go
code := m.Run()
_ = testEnv.Stop()
os.Exit(code)      // ← 必须 os.Exit
```

**为什么不用 defer**：`os.Exit` 不执行 defer。写 `defer testEnv.Stop()` 会导致 etcd / apiserver 进程残留在后台——envtest 最常见的事故。

---

## 6. 谁负责什么

| 层 | 文件 | 职责 | 你能改什么 |
|---|---|---|---|
| ① | `Makefile` | 算路径、拼命令 | 换版本、换目标 |
| ② | `go.mod` + 命名约定 | 扫函数、生成 main、编译 | 函数名、签名 |
| ③ | `suite_test.go` | 起环境、造 client | client 类型、CRD 路径 |
| ④ | `env_test.go` | 输入 + 断言 | **测试逻辑（主战场）** |
| ⑤ | `reconciler.go` | 被测逻辑 | 业务代码 |

---

## 7. 三个"改这里会怎样"

| 改动 | 后果 |
|---|---|
| `suite_test.go` 的 `client.New(...)` 加 `Cache:` | 读走缓存 → 04 小节 |
| `suite_test.go` 删掉 `m.Run()` | 环境照起 4 秒，**一条测试不跑，却 ok** |
| `Makefile` 的 `KUBEBUILDER_ASSETS` 变空 | envtest 找不到二进制，启动失败 |

---

## 8. 假绿清单（退出码全是 0）

| 事故 | 现象 | 怎么防 |
|---|---|---|
| `TestMain` 忘了 `m.Run()` | 输出 `ok`，一条测试没跑 | 检查最后有 `os.Exit(m.Run())` |
| `-run` 拼错 | `[no tests to run]` | 看有没有 `=== RUN` |
| `Testfoo` + `-vet=off` | 静默消失 | 别关 vet |
| 断言写错（如 `if err != nil { t.Log }`） | 失败也不报 | 用 `t.Fatalf` / `t.Errorf` |

---

## 9. 真实数字

| 阶段 | 耗时 | 体积 |
|---|---|---|
| Makefile 变量展开（`setup-envtest`） | 23 ms | — |
| `go test` 编译链接 | ~0.3 s | — |
| `testEnv.Start()` | ~4 s | 165 MB 二进制 |
| 测试函数本身 | 0.00 s | — |
| **总计** | **~4.2 s** | |

**测试逻辑几乎不花时间，99% 的时间在起环境。**

---

## 10. 自测

1. `make test-simple` 一共执行了几条 shell 命令？分别是什么？
2. 为什么 `make -n` 看不到 `setup-envtest` 那一条？
3. `env_test.go` 为什么不需要 `import` `suite_test.go`？
4. 生成代码里的 `tests` 表，第二列是什么？
5. `TestMain` 里删掉 `m.Run()` 会发生什么？退出码是多少？
6. `r.Get` 里的 `r` 自己没有 `Get` 方法，为什么能调用？
7. `Get` 一个不存在的对象，拿到的是什么？文本是什么？
8. 为什么收尾用 `os.Exit` 而不是 `defer`？

<details>
<summary>答案</summary>

1. 两条：`setup-envtest use 1.31.x -p path`（解析期）和 `go test ...`（recipe）。
2. 它在 `$(shell ...)` 里，属于**解析期**，`-n` 只打印 recipe。
3. 同一个目录 = 同一个包，共享标识符。
4. 函数值（`_test.TestEnv_ReconcileOnMissingObject`），不是字符串。
5. 测试一条都不跑，输出 `ok`，退出码 0。
6. `client.Client` 是嵌入字段，`Get` 是**提升**上来的方法。
7. `*apierrors.StatusError`，文本 `configmaps "definitely-not-exist" not found`。
8. `os.Exit` 不执行 defer，用 defer 会漏杀子进程。

</details>

---

## 11. 命令速查

```bash
cd 08-Testing/src/00-get-branch-envtest

make setup             # 一次性：装工具 + 下载二进制
make test-simple       # 跑沙盒

make -n test-simple    # 只看要执行什么
make -p | grep KUBEBUILDER_ASSETS   # 看变量定义

# 看生成代码
go test -c -work -o /tmp/x.test .
cat $WORK/b001/_testmain.go

# 看文件分类
go list -f '{{.GoFiles}} | {{.TestGoFiles}}' .

# 看 flag 映射
go test -x . 2>&1 | grep '\.test -test'
```