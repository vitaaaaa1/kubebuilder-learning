# 03 · 表驱动测试模板（可直接复制）

> 对应代码：[cmd/get-branch/table_test.go](../cmd/get-branch/table_test.go)

## 什么时候用它

- 同一个函数要验证**多种输入 → 多种期望**
- 每个场景的**准备步骤几乎一样**，只有数据不同
- 你希望失败时能一眼看出"是哪一种场景挂了"

反过来，如果每个场景的准备步骤差异很大（比如要先 Create 再 Patch 再等 Ready），表驱动会退化成 `switch` 地狱，**这时应该拆成几个独立测试函数**。

---

## 模板全文

```go
package getbranch

import (
	"context"
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestReconcile_GetBranch(t *testing.T) {
	// 所有场景共用一个请求对象，差异靠"预置对象"和"钩子"制造
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"},
	}

	tests := []struct {
		name        string             // 子测试名，会显示在 -run 和失败输出里
		objects     []client.Object    // 预置进 fake 的对象（空 = 集群里什么都没有）
		interceptor *interceptor.Funcs // 非空 = 劫持 client 方法，模拟故障
		wantErr     error              // nil = 期望 Reconcile 不报错
	}{
		{
			name:    "对象不存在时忽略 NotFound",
			objects: nil,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ① 准备世界
			builder := fake.NewClientBuilder().
				WithScheme(clientgoscheme.Scheme).
				WithObjects(tt.objects...)
			if tt.interceptor != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptor)
			}
			r := &Reconciler{Client: builder.Build()}

			// ② 摇一下
			_, err := r.Reconcile(context.Background(), req)

			// ③ 检查世界
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("期望不报错，实际返回: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("期望返回 %v，实际返回: %v", tt.wantErr, err)
			}
		})
	}
}
```

---

## 四块结构

### ① `req`：所有场景共用的输入

```go
req := ctrl.Request{
	NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"},
}
```

放在表外面，因为**差异不在请求上**。如果某个场景需要请求不同的对象，就把 `req` 挪进 case 结构体。

### ② case 结构体：只描述"差异"

```go
tests := []struct {
	name        string
	objects     []client.Object
	interceptor *interceptor.Funcs
	wantErr     error
}{ ... }
```

| 字段 | 回答的问题 | 省略时的含义 |
|---|---|---|
| `name` | 我叫什么？ | 必填，否则子测试重名会变成 `#01` |
| `objects` | 开始时集群里有什么？ | `nil` = 空的 |
| `interceptor` | 要注入什么故障？ | `nil` = 用 fake 的真实行为 |
| `wantErr` | 我期望什么错误？ | `nil` = 期望不报错 |

**四个字段就够用了。** 不要提前加 `wantResult`、`wantEvents` 这类没人填的字段。

### ③ 表：所有场景就是数据

```go
{
	name:    "对象不存在时忽略 NotFound",
	objects: nil,
	wantErr: nil,
},
```

加一个场景 = 加一个这样的块，**不用动循环里的代码**。

### ④ `t.Run`：准备 / 摇 / 断言

```go
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		// ① 准备
		// ② 摇
		// ③ 断言
	})
}
```

三段式里的代码只写一遍，`tt.xxx` 是唯一按场景变化的地方。

---

## 怎么加一个新场景（3 步）

以"对象存在时正常返回"为例：

**第 1 步**：想清楚差异在哪——集群里要有一个对象。

**第 2 步**：往表里加一块：

```go
{
	name: "对象存在时正常返回",
	objects: []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		},
	},
	wantErr: nil,
},
```

**第 3 步**：补 import（`corev1`、`metav1`），跑：

```bash
go test ./cmd/get-branch -v
```

> 注意：`objects` 里的对象名和 namespace 必须和 `req` 对得上，否则 `Get` 还是 NotFound。

---

## 两个旋钮

```go
builder := fake.NewClientBuilder().
	WithScheme(clientgoscheme.Scheme).   // 旋钮 1：有 CRD 时换成合并后的 scheme
	WithObjects(tt.objects...)           // 旋钮 2：集群初始状态
if tt.interceptor != nil {
	builder = builder.WithInterceptorFuncs(*tt.interceptor)   // 旋钮 3：注入故障
}
```

| 旋钮 | 什么时候动它 |
|---|---|
| `WithScheme` | 测试对象是 CRD / 自定义类型 |
| `WithObjects` | 需要"对象已存在"的场景 |
| `WithInterceptorFuncs` | 需要模拟 apiserver 故障（500/403/超时） |

`WithInterceptorFuncs` 用 `if` 包一层，是因为**只有部分场景需要它**——这就是 builder 模式最常见的用法。

---

## 断言为什么只写一处

四个场景，但只有**两种期望**：

```text
wantErr == nil   →  断言 err 必须为 nil
wantErr != nil   →  断言 errors.Is(err, wantErr)
```

所以两段 `if` 就够了。如果每个 case 都写自己的断言逻辑，表驱动就退化成"把多个函数塞进一个文件"，没有收益。

**判断标准**：断言里如果出现 `tt.name == "xxx"` 这种分支，说明该拆表了。

---

## 跑法

```bash
# 全部
go test ./cmd/get-branch -v

# 只跑这条表驱动测试
go test ./cmd/get-branch -v -run 'TestReconcile_GetBranch'

# 只跑某一个场景（注意空格会变成下划线）
go test ./cmd/get-branch -v -run 'TestReconcile_GetBranch/对象不存在时忽略_NotFound'

# 强制不吃缓存
go test ./cmd/get-branch -v -count=1
```

输出会带层级：

```text
=== RUN   TestReconcile_GetBranch
=== RUN   TestReconcile_GetBranch/对象不存在时忽略_NotFound
--- PASS: TestReconcile_GetBranch (0.00s)
    --- PASS: TestReconcile_GetBranch/对象不存在时忽略_NotFound (0.00s)
```

---

## 常见错误

| 报错 | 原因 | 解法 |
|---|---|---|
| `declared and not used: req` | 声明了 `req` 但没在循环里用 | 补上 `r.Reconcile(ctx, req)` |
| `declared and not used: builder` | 没调 `.Build()` | 补 `r := &Reconciler{Client: builder.Build()}` |
| `command-line-arguments ... undefined:` | `go test` 传了文件而不是目录 | 改成 `go test ./cmd/get-branch` |
| 子测试名重复变成 `#01` | 两个 case 的 `name` 相同 | 改名字 |
| `cannot use ... as type client.Object` | `objects` 里放了非 K8s 对象 | 只放实现了 `client.Object` 的类型 |
| 该红的测试不红 | 断言太弱（只查 err == nil） | 加更具体的断言，或做变异实验 |

---

## 自测

1. 想让某个场景"集群里已有一个 Secret"，改哪个字段？
2. `wantErr: nil` 和 `wantErr: boom` 分别走断言里的哪个分支？
3. 为什么 `req` 写在循环外面？什么情况下要挪进去？
4. 如果表里有 8 个场景，但准备步骤各不相同，还该用表驱动吗？

答案：① `objects`；② 第一段 `if` / 第二段 `errors.Is`；③ 全场景输入相同，不同则挪进 case；④ 不该，拆成多个测试函数。

---

## 关联

- 模板里那三行初始化的语法：[01-fake-client-syntax.md](01-fake-client-syntax.md)
- 哪些断言该写在这一层：[02-three-test-layers.md](02-three-test-layers.md)
- 索引：[README.md](README.md)