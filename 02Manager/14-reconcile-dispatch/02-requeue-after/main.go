// 情况 2：RequeueAfter —— 过 2 秒再执行一次。
//
//	go run ./02-requeue-after
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
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

func main() {
	if err := shared.Start(&reconciler{}, controller.Options{}, time.Hour); err != nil {
		panic(err)
	}
}
