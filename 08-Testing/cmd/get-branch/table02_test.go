package getbranch

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// ============================================================
// 第 3 步：表驱动 —— 一个函数覆盖全部 4 个场景
//
// 为什么要有这一步：
//   逐条写法（一条测试一个函数）在场景变多后会失控：
//   4 个场景 = 4 个函数 = 4 份几乎一样的准备代码。
//   表驱动把"准备/执行"写一次，把"差异"抽成数据。
//
// 表驱动的三要素：
//   1. 一个 case 结构体：描述"这个场景需要什么、期望什么"
//   2. 一个 []case 切片：所有场景就是数据
//   3. t.Run 循环：每个 case 变成一个独立子测试，可单独跑、可单独报错
// ============================================================

func TestReconcile02_GetBranch(t *testing.T) {
	// 所有场景都用同一个请求对象，靠"预置对象"和"钩子"制造差异
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "demo", Namespace: "default"},
	}

	// 人造错误01: Get方法返回的错误不被 client.IgnoreNotFound 捕捉
	boom := errors.New("apiserver error: xxxxx")
	// 人造错误02: rbac error
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Resource: "configmaps_test"},
		"demo",
		errors.New("rbac denied"),
	)

	tests := []struct {
		name        string             // 子测试名，会显示在 -run 和失败输出里
		objects     []client.Object    // 预置进 fake 的对象（空 = 集群里什么都没有）
		interceptor *interceptor.Funcs // 非空 = 劫持 client 方法，模拟故障
		wantErr     error              // nil = 期望 Reconcile 不报错
	}{
		// 人造错误01
		{
			name:    "apiserver 500 时原样返回错误",
			objects: nil,
			interceptor: &interceptor.Funcs{
				Get: func(_ context.Context,
					_ client.WithWatch,
					_ client.ObjectKey,
					_ client.Object,
					_ ...client.GetOption) error {
					return boom
				},
			},
			wantErr: boom,
		},
		// 人造错误02
		{
			name:    "rbac 权限问题",
			objects: nil,
			interceptor: &interceptor.Funcs{
				Get: func(_ context.Context,
					_ client.WithWatch,
					_ client.ObjectKey,
					_ client.Object,
					_ ...client.GetOption) error {
					return forbidden
				},
			},
			wantErr: forbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ① 准备世界：模板代码，只有两个地方按 case 变化
			builder := fake.NewClientBuilder().
				WithScheme(clientgoscheme.Scheme).
				WithObjects(tt.objects...)
			if tt.interceptor != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptor)
			}
			r := &Reconciler{Client: builder.Build()}

			// ② 摇一下
			_, err := r.Reconcile(context.Background(), req)

			// ③ 检查世界：断言只有一处，但覆盖了两种期望
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
