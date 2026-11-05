package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{"test": {}},
		},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	// 控制器 A：不加任何过滤，create / update / delete 全收
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("cm-all").
		For(&corev1.ConfigMap{}).
		Complete(&LoggingReconciler{Name: "A-无过滤", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "创建 A 失败")
		os.Exit(1)
	}

	// 控制器 B：WithEventFilter —— 全局过滤，只放行“标签发生变化”的事件
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("cm-label-changed").
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.LabelChangedPredicate{}).
		Complete(&LoggingReconciler{Name: "B-只标签变化", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "创建 B 失败")
		os.Exit(1)
	}

	// 控制器 C：For 级过滤 —— 只放行 Create，Update/Delete 全部丢弃
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("cm-create-only").
		For(&corev1.ConfigMap{}).
		WithEventFilter(createOnly()).
		Complete(&LoggingReconciler{Name: "C-只Create", Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "创建 C 失败")
		os.Exit(1)
	}

	setupLog.Info("启动三个控制器：A-无过滤 / B-只标签变化 / C-只Create")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}

// createOnly 返回一个只放行 Create 事件的 predicate。
func createOnly() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}
