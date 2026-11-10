// 情况 5：MaxConcurrentReconciles=3 —— 三个对象并行处理。
//
//	go run ./05-concurrent-workers
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
	log := ctrl.LoggerFrom(ctx)
	log.Info("开始处理（睡 2s）", "name", req.Name, "at", time.Now().Format("15:04:05.000"))
	time.Sleep(2 * time.Second)
	log.Info("处理完成", "name", req.Name, "at", time.Now().Format("15:04:05.000"))
	return ctrl.Result{}, nil
}

func main() {
	if err := shared.Start(
		&reconciler{},
		controller.Options{MaxConcurrentReconciles: 3},
		time.Hour,
		"demo-a", "demo-b", "demo-c",
	); err != nil {
		panic(err)
	}
}
