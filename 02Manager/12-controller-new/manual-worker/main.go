// 版本 B：不用 controller.New，手写队列 + worker 循环
//
// 逻辑与 controller 内部的 processNextWorkItem 一致：
//
//	Get → Reconcile → 失败 AddRateLimited / 成功 Forget → Done
//
//	go run ./manual-worker
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"bare-metal-controller/12-controller-new/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")
	log := ctrl.Log.WithName("manual-worker")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  runtime.NewScheme(),
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache:   cache.Options{DefaultNamespaces: map[string]cache.Config{"test": {}}},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	r := &shared.Reconciler{Assembly: "manual-worker"}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "test", Name: "never-exists"}}

	// ───────── 唯一不同：手写队列 + worker 循环 ─────────
	err = mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		queue := workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](1*time.Second, 10*time.Second),
			workqueue.TypedRateLimitingQueueConfig[reconcile.Request]{Name: "manual-queue"},
		)
		defer queue.ShutDown()

		go func() {
			<-ctx.Done()
			queue.ShutDown()
		}()

		log.Info("事件源往队列塞入 Request", "request", req.NamespacedName.String())
		queue.Add(req)

		for {
			item, shutdown := queue.Get()
			if shutdown {
				return nil
			}

			func() {
				defer queue.Done(item)
				if _, err := r.Reconcile(ctx, item); err != nil {
					log.Error(err, "Reconcile 失败，重新入队")
					queue.AddRateLimited(item)
					return
				}
				queue.Forget(item)
			}()
		}
	}))
	if err != nil {
		setupLog.Error(err, "mgr.Add 失败")
		os.Exit(1)
	}
	// ───────── 不同到此为止 ─────────

	go func() {
		time.Sleep(6 * time.Second)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(os.Interrupt)
	}()

	setupLog.Info("启动 Manager：手写 worker 版本")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
