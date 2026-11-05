package main

import (
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

func main() {
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	// ① Scheme 里放了什么，For() 就能认什么
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{"test": {}},
		},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	// ② 控制器 A：不写 Named，名字由 kind 推导 → "configmap"
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(&LoggingReconciler{Kind: "ConfigMap", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "创建 ConfigMap 控制器失败")
		os.Exit(1)
	}

	// ③ 控制器 B：Named 覆盖默认名字 → "secret-watcher"
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("secret-watcher").
		For(&corev1.Secret{}).
		Complete(&LoggingReconciler{Kind: "Secret", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "创建 Secret 控制器失败")
		os.Exit(1)
	}

	setupLog.Info("启动两个控制器：configmap（默认名） + secret-watcher（Named）")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
