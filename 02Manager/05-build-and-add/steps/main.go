// 分解演示：把 Complete() 拆成可单独执行的步骤，逐步观察每一步的作用
//
//	go run ./steps -mode=new-only    # 只 controller.New
//	go run ./steps -mode=new-watch   # controller.New + Watch
//	go run ./steps -mode=builder     # For + Complete（等价于上面两步）
package main

import (
	"context"
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func main() {
	mode := flag.String("mode", "new-only", "new-only | new-watch | builder")
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

	r := &LoggingReconciler{Name: *mode, Client: mgr.GetClient()}

	switch *mode {
	// ── 步骤 1：只创建控制器 ──────────────────────────────
	// 作用：把 Reconciler 交给 Manager，Manager 会给它建 workqueue 和 worker。
	// 后果：控制器会启动，但没有任何事件源 —— 永远收不到事件。
	case "new-only":
		if _, err := controller.New("step-new-only", mgr, controller.Options{
			Reconciler: r,
		}); err != nil {
			setupLog.Error(err, "controller.New 失败")
			os.Exit(1)
		}
		setupLog.Info("【步骤 1】controller.New：控制器已注册，但没有 Watch")

	// ── 步骤 2：创建 + 注册事件源 ─────────────────────────
	// 作用：告诉控制器“哪些对象的变化要变成事件”。
	// 后果：创建 ConfigMap 才会触发 Reconcile。
	case "new-watch":
		c, err := controller.New("step-new-watch", mgr, controller.Options{
			Reconciler: r,
		})
		if err != nil {
			setupLog.Error(err, "controller.New 失败")
			os.Exit(1)
		}

		obj := client.Object(&corev1.ConfigMap{})
		if err := c.Watch(source.Kind(
			mgr.GetCache(),
			obj,
			&handler.EnqueueRequestForObject{},
		)); err != nil {
			setupLog.Error(err, "Watch 失败")
			os.Exit(1)
		}
		setupLog.Info("【步骤 1+2】controller.New + Watch：事件源已注册")

	// ── 步骤 3：Builder 一次做完 1 和 2 ──────────────────
	// 作用：等价于上面两步，额外帮你做了：
	//   · 用 kind 小写生成控制器名
	//   · 挂上 log constructor（日志里多出 controllerKind 等字段）
	//   · 支持 predicate / Owns / Watches 等扩展
	case "builder":
		if err := ctrl.NewControllerManagedBy(mgr).
			For(&corev1.ConfigMap{}).
			Complete(r); err != nil {
			setupLog.Error(err, "Builder 失败")
			os.Exit(1)
		}
		setupLog.Info("【步骤 1+2 封装】For + Complete")

	default:
		setupLog.Info("未知 mode，可选：new-only / new-watch / builder")
		os.Exit(1)
	}

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}

type LoggingReconciler struct {
	Name string
	client.Client
}

func (r *LoggingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile 被调用", "mode", r.Name)
	return ctrl.Result{}, nil
}
