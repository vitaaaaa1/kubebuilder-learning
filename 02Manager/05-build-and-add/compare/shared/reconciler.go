// Package shared 放两个 main 共用的 Reconciler。
// 这样 manual/ 和 builder/ 里除了“装配那一段”，其余代码完全一致。
package shared

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	Assembly string
	client.Client
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile 被调用", "assembly", r.Assembly)
	return ctrl.Result{}, nil
}
