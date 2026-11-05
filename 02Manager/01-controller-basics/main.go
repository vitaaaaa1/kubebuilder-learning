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
	// ① 日志：--zap-log-level=debug 可调级别
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	// ② Scheme：告诉 client 有哪些 Go 类型可用
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	// ③ Manager：连集群（读 ~/.kube/config 或 KUBECONFIG）
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,

		// 关掉 metrics 端口，避免与本机其他进程冲突
		Metrics: metricsserver.Options{BindAddress: "0"},

		// 只缓存 test 命名空间：减少内存、减少噪声
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				"test": {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	// ④ 把 Reconciler 挂到 Manager 上：监听 ConfigMap
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(&SimpleReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "创建 Controller 失败")
		os.Exit(1)
	}

	setupLog.Info("Manager 启动：监听 test 命名空间的 ConfigMap")

	// ⑤ 启动（阻塞）。Ctrl+C 触发优雅关闭
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
