package main

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingReconciler struct {
	Name string
	client.Client
}

func (r *LoggingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile 被调用", "mode", r.Name)
	return ctrl.Result{}, nil
}
