package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// 版本 A：调用 ctrl.NewManager
func main() {
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	cfg := ctrl.GetConfigOrDie()

	// ───────── 只需这一句 ─────────
	t0 := time.Now()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache:   cache.Options{DefaultNamespaces: map[string]cache.Config{"test": {}}},
	})
	if err != nil {
		setupLog.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}
	cost := time.Since(t0)

	fmt.Println("========== 版本 A：ctrl.NewManager ==========")
	fmt.Printf("NewManager 耗时: %v\n", cost)
	fmt.Printf("GetClient    : %T\n", mgr.GetClient())
	fmt.Printf("GetAPIReader : %T\n", mgr.GetAPIReader())
	fmt.Printf("GetCache     : %T\n", mgr.GetCache())
	fmt.Printf("GetScheme    : %T\n", mgr.GetScheme())
	fmt.Printf("GetRESTMapper: %T\n", mgr.GetRESTMapper())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Manager 负责启动缓存
	go func() {
		if err := mgr.Start(ctx); err != nil {
			fmt.Println("mgr.Start 返回:", err)
		}
	}()
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		fmt.Println("缓存同步失败")
		os.Exit(1)
	}

	// 任务 1：直连读（apiReader）
	var direct corev1.ConfigMapList
	if err := mgr.GetAPIReader().List(ctx, &direct); err != nil {
		fmt.Println("apiReader.List 失败:", err)
		os.Exit(1)
	}

	// 任务 2：缓存读（cached client）
	var cached corev1.ConfigMapList
	if err := mgr.GetClient().List(ctx, &cached); err != nil {
		fmt.Println("cachedClient.List 失败:", err)
		os.Exit(1)
	}

	fmt.Printf("直连读取 apiReader     : %d 个 ConfigMap（全集群）\n", len(direct.Items))
	fmt.Printf("缓存读取 cachedClient  : %d 个 ConfigMap（仅 test）\n", len(cached.Items))

	cancel()
	time.Sleep(300 * time.Millisecond)
}
