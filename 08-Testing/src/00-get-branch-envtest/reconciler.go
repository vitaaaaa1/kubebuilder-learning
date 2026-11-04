// Package envtestsimple 是最小沙盒：
// 只保留 Reconcile 的 Get 分支，测试全部跑在真 apiserver（envtest）上。
//
// 和 cmd/get-branch/ 的区别：
//
//	get-branch   → 同一个 Reconcile，跑在 fake client 上（纳秒级、无 apiserver）
//	envtest-simple → 同一个 Reconcile，跑在真实 apiserver 上（秒级、有 UID、有 etcd）
//
// 把两边的测试对照着看，就能看清"哪些断言 fake 给不了"。
package getbranchenvtest

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler 只做一件事：Get 到对象，或者把 NotFound 当成"无事可做"。
type Reconciler struct {
	client.Client
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		// 被测的只有这一行：NotFound → nil，其他错误 → 原样返回
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
