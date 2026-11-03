package getbranch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Reconciler 是从 08 章主控制器里"切下来"的最小切片：
// 只保留 Reconcile 的第一段 —— Get 失败时该怎么返回。
//
// 后面所有内容（创建 Secret、ownerRef、漂移修复）都被删掉了，
// 目的只有一个：让你在看测试时，眼睛里只有 5 行代码。
type Reconciler struct {
	client.Client
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		// 被测代码本体：这个分支应该"吞掉 NotFound，放过其他错误"。
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 占位：真实控制器这里会创建子对象。本沙盒不关心。
	return ctrl.Result{}, nil
}
