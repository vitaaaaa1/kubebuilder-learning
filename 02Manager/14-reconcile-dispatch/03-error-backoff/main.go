// 情况 3：返回 error —— 队列自动退避重试（5ms、10ms、20ms…）。
//
//	go run ./03-error-backoff
package main

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"bare-metal-controller/14-reconcile-dispatch/shared"
)

type reconciler struct{}

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile", "at", time.Now().Format("15:04:05.000"))
	return ctrl.Result{}, fmt.Errorf("模拟失败")
}

func main() {
	if err := shared.Start(&reconciler{}, controller.Options{}, time.Hour); err != nil {
		panic(err)
	}
}
