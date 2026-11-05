// 对照组 A：手动装配
//
// 与 ../builder/main.go 相比，只有中间标了「唯一不同」的那一段不一样，
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/source"
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

	// ───────── 唯一不同：手动装配 ─────────
	//
	// 第 1 步：创建控制器（名字、Reconciler、并注册进 Manager）
	c, err := controller.New("demo-configmap", mgr, controller.Options{
		Reconciler: &shared.Reconciler{Assembly: "manual", Client: mgr.GetClient()},
	})
	if err != nil {
		setupLog.Error(err, "controller.New 失败")
		os.Exit(1)
	}

	// 第 2 步：注册事件源
	obj := client.Object(&corev1.ConfigMap{})
	if err := c.Watch(source.Kind(
		mgr.GetCache(),
		obj,
		&handler.EnqueueRequestForObject{},
	)); err != nil {
		setupLog.Error(err, "Watch 失败")
		os.Exit(1)
	}

	setupLog.Info("装配方式：手动 controller.New + Watch")
	// ───────── 不同到此为止 ─────────

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
