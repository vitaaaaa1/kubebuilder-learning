// 一个 Manager，挂两个 Controller，各监听一种资源。
//
//	go run ./10-multi-controller
//	kubectl create configmap demo-cm -n test --from-literal=a=1
//	kubectl create secret generic demo-secret -n test --from-literal=b=2
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
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type reconciler struct{ kind string }

func (r *reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Reconcile 被调用", "kind", r.kind, "request", req.NamespacedName.String())
	return ctrl.Result{}, nil
}

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

	// 控制器 1：监听 ConfigMap
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("configmap-controller").
		For(&corev1.ConfigMap{}).
		Complete(&reconciler{kind: "ConfigMap"}); err != nil {
		setupLog.Error(err, "装配 ConfigMap 控制器失败")
		os.Exit(1)
	}

	// 控制器 2：监听 Secret
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("secret-controller").
		For(&corev1.Secret{}).
		Complete(&reconciler{kind: "Secret"}); err != nil {
		setupLog.Error(err, "装配 Secret 控制器失败")
		os.Exit(1)
	}

	setupLog.Info("已挂载 2 个 Controller")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
