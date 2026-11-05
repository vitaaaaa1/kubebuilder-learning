// 版本 A：使用 NewControllerManagedBy
//
// 与 ../manual/main.go 相比，只有中间“装配”这一段不同，
// 其余初始化、Reconciler、Manager 配置完全一致。

/*
	NewManager 负责“和集群建立连接”
	NewControllerManagedBy 负责“告诉 Manager：哪个对象发生变化时，要把 Reconcile 请求交给谁”
*/

package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"bare-metal-controller/09-controller-builder/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	// ───────── 两个文件完全相同 ─────────
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

	// ───────── 唯一不同：一行完成装配 ─────────
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("configmap-controller").
		For(&corev1.ConfigMap{}).
		Complete(&shared.Reconciler{Assembly: "builder", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "装配失败")
		os.Exit(1)
	}

	setupLog.Info("装配方式：NewControllerManagedBy + For + Complete")
	// ───────── 不同到此为止 ─────────

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
