// 情况 1：返回空 Result —— 处理完就结束，不再排队。
//
//	go run ./01-result-empty
package main

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"bare-metal-controller/14-reconcile-dispatch/shared"
)

type reconciler struct{}

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile", "at", time.Now().Format("15:04:05.000"))
	return ctrl.Result{}, nil
}

func main() {
	// SyncPeriod 设成 10 小时，避免周期重放干扰观察
	if err := shared.Start(&reconciler{}, controller.Options{}, 10*time.Hour); err != nil {
		panic(err)
	}
}
