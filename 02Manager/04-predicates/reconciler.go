package main

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LoggingReconciler 三个控制器共用，靠 Name 区分是谁被触发了。
type LoggingReconciler struct {
	Name string
	client.Client
}

func (r *LoggingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("事件通过过滤", "watcher", r.Name)
	return ctrl.Result{}, nil
}
