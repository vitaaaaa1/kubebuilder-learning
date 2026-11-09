// Package shared 放两个 main 共用的 Reconciler，
// 保证除“事件源”外，其余逻辑完全一致。
package shared

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

type Reconciler struct {
	Assembly string
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile 被调用",
		"assembly", r.Assembly,
		"request", req.NamespacedName.String())
	return ctrl.Result{}, nil
}
