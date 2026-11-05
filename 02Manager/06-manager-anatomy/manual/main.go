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
	// NewDynamicRESTMapper =
	//		建一个空翻译表 +
	//			一个 discovery 客户端；第一次查询时用聚合发现（/api + /apis）一次性拉全量资源清单，之后全部走内存
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		setupLog.Error(err, "RESTMapper 失败")
		os.Exit(1)
	}

	// ③ Cache
	// cache.New 造一个“informer 容器”：告诉它监听哪些类型、哪些命名空间、用哪个连接——但它不启动任何 informer，也不发一个请求。
	cmCache, err := cache.New(cfg, cache.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,

		DefaultNamespaces: map[string]cache.Config{"test": {}},
		// 只缓存 test 里带某标签的对象
		// DefaultNamespaces: map[string]cache.Config{
		//	"test": {
		//		LabelSelector: labels.SelectorFromSet(labels.Set{"app": "demo"}),
		//	},
		//}
	})
	if err != nil {
		setupLog.Error(err, "Cache 失败")
		os.Exit(1)
	}

	// ④ 缓存 client
	// client.New = 补默认值 + 建四个执行器 + 挂 cache。 它把“用哪个 mapper、哪个连接、读要不要走缓存”固定下来，但不执行任何请求。
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

	/*
		                                        cachedClient（GetClient）	                 apiReader（GetAPIReader）
		构造	                                    client.New(... Cache: {Reader: cache})	     client.New(...) 无 Cache
		Get / List	读 informer                  内存	                                     每次请求 apiserver
		Create/Update/Delete	     			 直连	                                     直连
		延迟	                                     微秒级	                                     毫秒级（实测 245ms 首次）
		一致性	                                 最终一致，可能滞后	                         强一致
		受 DefaultNamespaces 限制	             ✅ 只看 test	                             ❌ 全集群
		需要缓存启动	                             ✅ 否则 ErrCacheNotStarted	                 ❌ 随时可用
		对 apiserver 压力	                     几乎为 0	                                 每次真实请求
	*/

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
	// apiReader 立刻可读；cachedClient 必须等 Start + WaitForCacheSync
	go func() {
		// cmCache.Start(ctx) = 启动所有 informer 的 List+Watch 循环，然后阻塞直到 ctx 取消。 所以要 go 出去；真正的“启动完成”信号是 WaitForCacheSync
		if err := cmCache.Start(ctx); err != nil {
			fmt.Println("cache.Start 返回:", err)
		}
	}()
	//  等所有 informer 完成“首次全量 List”并把数据灌进内存
	// WaitForCacheSync = startWait + 所有 informer 的 HasSynced，两个条件都满足才返回 true。 它是“缓存可用”的唯一信号；Start 只代表“开始拉数据
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
