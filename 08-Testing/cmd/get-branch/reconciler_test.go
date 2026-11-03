package getbranch

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ============================================================
// 第 1 步：最简单的一条测试 —— 对象不存在时，Reconcile 不能报错
// ============================================================

func TestReconcile_IgnoresNotFound(t *testing.T) {
	// ① 准备世界：一个空的 fake client，里面什么都没有。
	//    这一步不需要 apiserver、不需要 KUBEBUILDER_ASSETS，纯内存。
	c := fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme).
		Build()

	// 创建 Reconciler struct, 才能去调用 Reconcile 方法代码逻辑
	r := &Reconciler{Client: c}

	// ② 摇一下：请求一个绝对不存在的对象。
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "definitely-not-exist", Namespace: "default"},
	})

	// ③ 检查世界：必须是 nil。
	//    如果不是 nil，controller-runtime 会把它当作失败，按速率限制退避重试 ——
	//    对一个已经不存在的对象反复重试，是新手最常见的坑。
	if err != nil {
		t.Fatalf("对象不存在时必须返回 nil，实际返回: %v", err)
	}
}
