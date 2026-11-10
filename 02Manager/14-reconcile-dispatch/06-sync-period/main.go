// 情况 6：SyncPeriod=2s —— 没有对象变化，也会每 2 秒触发一次 Reconcile。
//
//	go run ./06-sync-period
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
	// 关键差异：SyncPeriod = 2s
	if err := shared.Start(&reconciler{}, controller.Options{}, 2*time.Second); err != nil {
		panic(err)
	}
}
