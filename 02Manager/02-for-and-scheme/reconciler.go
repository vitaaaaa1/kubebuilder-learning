package main

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoggingReconciler 两个控制器共用一个实现，靠 Kind 区分日志。
type LoggingReconciler struct {
	Kind string
	client.Client
}

func (r *LoggingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("事件到达", "reconciler", r.Kind)
	return ctrl.Result{}, nil
}
