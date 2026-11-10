// 情况 4：Reconcile 里 panic —— 被恢复，进程不死，按错误重试。
//
//	go run ./04-panic-recover
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
	ctrl.LoggerFrom(ctx).Info("Reconcile 即将 panic")
	panic("模拟 panic")
}

func main() {
	// RecoverPanic 默认就是 true，这里不需要额外设置
	if err := shared.Start(&reconciler{}, controller.Options{}, time.Hour); err != nil {
		panic(err)
	}
}
