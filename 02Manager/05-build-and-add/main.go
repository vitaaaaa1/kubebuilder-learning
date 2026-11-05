// 手动装配：controller.New + Watch
//
// 这两步就是 NewControllerManagedBy().For().Complete() 内部做的事。
package main

import (
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

	// ① 创建 Controller（等价于 Builder 的 Build）
	c, err := controller.New("manual-configmap", mgr, controller.Options{
		Reconciler: &LoggingReconciler{Name: "手动 controller.New", Client: mgr.GetClient()},
	})
	if err != nil {
		setupLog.Error(err, "创建 Controller 失败")
		os.Exit(1)
	}

	// ② 注册事件源（等价于 Builder 的 For）
	//
	// 注意：handler.EnqueueRequestForObject 的类型是
	//   TypedEventHandler[client.Object, reconcile.Request]
	// 所以这里要把对象显式声明成 client.Object，泛型才能对上。
	obj := client.Object(&corev1.ConfigMap{})
	err = c.Watch(source.Kind(
		mgr.GetCache(),
		obj,
		&handler.EnqueueRequestForObject{},
	))
	if err != nil {
		setupLog.Error(err, "Watch 失败")
		os.Exit(1)
	}

	setupLog.Info("手动装配完成：controller.New + Watch")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
