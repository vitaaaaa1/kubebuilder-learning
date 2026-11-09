// 版本 A：使用内置的 source.Kind（最常见的 Watch 用法）
//
//	go run ./with-source
//	kubectl create configmap demo-watch -n test --from-literal=a=1
package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"bare-metal-controller/11-controller-watch/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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
		Reconciler: &shared.Reconciler{Assembly: "kind-source"},
	})
	if err != nil {
		setupLog.Error(err, "controller.New 失败")
		os.Exit(1)
	}

	// ───────── 唯一不同：事件源是 KindSource ─────────
	src := source.Kind[client.Object](mgr.GetCache(),
		&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{})
	if err := c.Watch(src); err != nil {
		setupLog.Error(err, "Watch 失败")
		os.Exit(1)
	}
	// ───────── 不同到此为止 ─────────

	setupLog.Info("事件源：source.Kind")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
