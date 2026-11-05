// 对照组 B：Builder 装配
//
// 与 ../manual/main.go 相比，只有中间标了「唯一不同」的那一段不一样，
// 其余初始化、Reconciler、Manager 配置全部相同。
package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"bare-metal-controller/05-build-and-add/compare/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	// ───────── 以下到 Manager 初始化为止，两个文件完全相同 ─────────
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

	// ───────── 唯一不同：Builder 装配 ─────────
	//
	// 上面 manual 的两步（controller.New + Watch）被合并成这一句。
	// Named("demo-configmap") 只是为了和手动版取一样的名字，方便对比。
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("demo-configmap").
		For(&corev1.ConfigMap{}).
		Complete(&shared.Reconciler{Assembly: "builder", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "Builder 装配失败")
		os.Exit(1)
	}

	setupLog.Info("装配方式：NewControllerManagedBy + For + Complete")
	// ───────── 不同到此为止 ─────────

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
