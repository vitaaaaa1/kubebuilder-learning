// 版本 A：使用 controller.New，观察 worker 循环与失败重试
//
//	go run ./with-controller
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"bare-metal-controller/12-controller-new/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func main() {
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  runtime.NewScheme(),
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache:   cache.Options{DefaultNamespaces: map[string]cache.Config{"test": {}}},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	// ───────── 唯一不同：用 controller.New 建控制器 ─────────
	c, err := controller.New("retry-demo", mgr, controller.Options{
		Reconciler: &shared.Reconciler{Assembly: "with-controller"},
		// 默认是 5ms 起步，这里放大到 1s，方便观察退避间隔
		RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
			1*time.Second, 10*time.Second),
	})
	if err != nil {
		setupLog.Error(err, "controller.New 失败")
		os.Exit(1)
	}

	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test", Name: "never-exists"}}
	err = c.Watch(source.Func(func(ctx context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
		setupLog.Info("事件源往队列塞入 Request", "request", req.NamespacedName.String())
		q.Add(req)
		return nil
	}))
	if err != nil {
		setupLog.Error(err, "Watch 失败")
		os.Exit(1)
	}
	// ───────── 不同到此为止 ─────────

	// 给重试足够时间（5ms、10ms 后成功），然后主动退出
	go func() {
		time.Sleep(6 * time.Second)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(os.Interrupt)
	}()

	setupLog.Info("启动 Manager：controller.New 版本")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
