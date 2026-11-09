// 版本 B：自定义事件源（不进 informer，直接往队列塞 Request）
//
// 用来证明：Watch 接的只是“一个能把 Request 塞进队列的生产者”，
// informer 只是其中最常用的一种实现。
//
//	go run ./custom-source
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"bare-metal-controller/11-controller-watch/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache:   cache.Options{DefaultNamespaces: map[string]cache.Config{"test": {}}},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	c, err := controller.New("configmap-controller", mgr, controller.Options{
		Reconciler: &shared.Reconciler{Assembly: "custom-source"},
	})
	if err != nil {
		setupLog.Error(err, "controller.New 失败")
		os.Exit(1)
	}

	// ───────── 唯一不同：事件源是自定义的 ─────────
	if err := c.Watch(&injectedSource{every: 3 * time.Second}); err != nil {
		setupLog.Error(err, "Watch 失败")
		os.Exit(1)
	}
	// ───────── 不同到此为止 ─────────

	setupLog.Info("事件源：自定义 injectedSource")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}

// injectedSource 实现了 source.Source：
// 它不读缓存、不连 apiserver，只是定期往队列里塞一个 Request。
type injectedSource struct {
	every time.Duration
}

func (s *injectedSource) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	log := ctrl.LoggerFrom(ctx).WithName("injectedSource")
	log.Info("Start 被调用，拿到队列", "every", s.every)

	go func() {
		ticker := time.NewTicker(s.every)
		defer ticker.Stop()
		for i := 1; ; i++ {
			select {
			case <-ctx.Done():
				log.Info("Source 停止")
				return
			case <-ticker.C:
				req := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: "test",
						Name:      fmt.Sprintf("injected-%d", i),
					},
				}
				log.Info("往队列塞入 Request", "request", req.NamespacedName.String())
				queue.Add(req)
			}
		}
	}()
	return nil
}

func (s *injectedSource) String() string { return "injected source" }
