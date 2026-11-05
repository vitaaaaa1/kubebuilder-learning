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
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// 版本 B：自己组装，不用 ctrl.NewManager
func main() {
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	cfg := ctrl.GetConfigOrDie()

	// ───────── 手动组装 ─────────
	t0 := time.Now()

	// ① 共享的 HTTP 客户端
	// 把 kubeconfig 解析出来的 *rest.Config，变成一个已经配好 TLS 和认证的 *http.Client
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		setupLog.Error(err, "HTTPClient 失败")
		os.Exit(1)
	}

	// ② RESTMapper
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		setupLog.Error(err, "RESTMapper 失败")
		os.Exit(1)
	}

	// ③ Cache
	cmCache, err := cache.New(cfg, cache.Options{
		Scheme:            scheme,
		Mapper:            mapper,
		HTTPClient:        httpClient,
		DefaultNamespaces: map[string]cache.Config{"test": {}},
	})
	if err != nil {
		setupLog.Error(err, "Cache 失败")
		os.Exit(1)
	}

	// ④ 缓存 client
	cachedClient, err := client.New(cfg, client.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
		Cache:      &client.CacheOptions{Reader: cmCache},
	})
	if err != nil {
		setupLog.Error(err, "cachedClient 失败")
		os.Exit(1)
	}

	// ⑤ 直连 reader
	apiReader, err := client.New(cfg, client.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
	})
	if err != nil {
		setupLog.Error(err, "apiReader 失败")
		os.Exit(1)
	}

	cost := time.Since(t0)

	fmt.Println("========== 版本 B：手动组装 ==========")
	fmt.Printf("组装耗时: %v\n", cost)
	fmt.Printf("cachedClient : %T\n", cachedClient)
	fmt.Printf("apiReader    : %T\n", apiReader)
	fmt.Printf("cache        : %T\n", cmCache)
	fmt.Printf("scheme       : %T\n", scheme)
	fmt.Printf("mapper       : %T\n", mapper)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ⑥ 自己启动缓存并等待同步（Manager 里由 Start 完成）
	go func() {
		if err := cmCache.Start(ctx); err != nil {
			fmt.Println("cache.Start 返回:", err)
		}
	}()
	if !cmCache.WaitForCacheSync(ctx) {
		fmt.Println("缓存同步失败")
		os.Exit(1)
	}

	// 任务 1：直连读
	var direct corev1.ConfigMapList
	if err := apiReader.List(ctx, &direct); err != nil {
		fmt.Println("apiReader.List 失败:", err)
		os.Exit(1)
	}

	// 任务 2：缓存读
	var cached corev1.ConfigMapList
	if err := cachedClient.List(ctx, &cached); err != nil {
		fmt.Println("cachedClient.List 失败:", err)
		os.Exit(1)
	}

	fmt.Printf("直连读取 apiReader     : %d 个 ConfigMap（全集群）\n", len(direct.Items))
	fmt.Printf("缓存读取 cachedClient  : %d 个 ConfigMap（仅 test）\n", len(cached.Items))

	cancel()
	time.Sleep(300 * time.Millisecond)
}
