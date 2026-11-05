// 对照组 C：手动拆分 ctrl.NewManager
//
// 以 ../builder/main.go 为模板，唯一的区别是：
// 不用 ctrl.NewManager，而是自己组装它内部创建的四个部件：
//
//	RESTMapper → Cache → CachedClient + APIReader
//
// 然后手动启动缓存、手动加 informer handler。
//
// 运行：go run ./compare/split-manager
package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	// ───────── 与 builder/main.go 相同的初始化 ─────────
	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))
	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	restCfg := ctrl.GetConfigOrDie()

	// ───────── 以下：手动做 ctrl.NewManager 内部做的事 ─────────

	// ① HTTPClient（共享连接池）
	httpClient, err := rest.HTTPClientFor(restCfg)
	if err != nil {
		setupLog.Error(err, "创建 HTTPClient 失败")
		os.Exit(1)
	}

	// ② RESTMapper：GVK ↔ 资源路径（Manager 也有一份）
	mapper, err := apiutil.NewDynamicRESTMapper(restCfg, httpClient)
	if err != nil {
		setupLog.Error(err, "创建 RESTMapper 失败")
		os.Exit(1)
	}
	setupLog.Info("【1/4】RESTMapper 就绪")

	// ③ Cache：informer 容器（此时还没启动）
	cmCache, err := cache.New(restCfg, cache.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
		DefaultNamespaces: map[string]cache.Config{
			"test": {},
		},
	})
	if err != nil {
		setupLog.Error(err, "创建 Cache 失败")
		os.Exit(1)
	}
	setupLog.Info("【2/4】Cache 就绪（尚未 Start）")

	// ④ CachedClient：等价于 mgr.GetClient()
	cachedClient, err := client.New(restCfg, client.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
		Cache:      &client.CacheOptions{Reader: cmCache},
	})
	if err != nil {
		setupLog.Error(err, "创建 CachedClient 失败")
		os.Exit(1)
	}

	// ⑤ APIReader：等价于 mgr.GetAPIReader()（不带 Cache）
	apiReader, err := client.New(restCfg, client.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
	})
	if err != nil {
		setupLog.Error(err, "创建 APIReader 失败")
		os.Exit(1)
	}
	setupLog.Info("【3/4】CachedClient + APIReader 就绪")

	ctx := ctrl.SetupSignalHandler()

	// ⑥ Informer + 事件处理器（Controller 内部就是用它）
	informer, err := cmCache.GetInformer(ctx, &corev1.ConfigMap{})
	if err != nil {
		setupLog.Error(err, "获取 Informer 失败")
		os.Exit(1)
	}
	if _, err := informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm, _ := obj.(*corev1.ConfigMap)
			setupLog.Info("informer: Add", "name", cm.Name)
		},
		UpdateFunc: func(_, newObj interface{}) {
			cm, _ := newObj.(*corev1.ConfigMap)
			setupLog.Info("informer: Update", "name", cm.Name)
		},
		DeleteFunc: func(obj interface{}) {
			if cm, ok := obj.(*corev1.ConfigMap); ok {
				setupLog.Info("informer: Delete", "name", cm.Name)
			}
		},
	}); err != nil {
		setupLog.Error(err, "注册事件处理器失败")
		os.Exit(1)
	}
	setupLog.Info("【4/4】Informer 与 Handler 就绪")

	// ⑦ 手动启动缓存 + 等待同步（Manager 帮你做的就是这一步）
	go func() {
		if err := cmCache.Start(ctx); err != nil {
			setupLog.Error(err, "Cache 启动失败")
		}
	}()
	if !cmCache.WaitForCacheSync(ctx) {
		setupLog.Error(nil, "缓存同步失败")
		os.Exit(1)
	}
	setupLog.Info("缓存已同步（这一步在 Manager 里由 Start 自动完成）")

	// ⑧ 两个 client 的读取对比
	var cacheList corev1.ConfigMapList
	if err := cachedClient.List(ctx, &cacheList); err != nil {
		setupLog.Error(err, "cachedClient.List 失败")
	}
	setupLog.Info("cachedClient 读到", "count", len(cacheList.Items))

	var directList corev1.ConfigMapList
	if err := apiReader.List(ctx, &directList); err != nil {
		setupLog.Error(err, "apiReader.List 失败")
	}
	setupLog.Info("apiReader 读到", "count", len(directList.Items))

	setupLog.Info("手动组装完成，等待事件（Ctrl+C 退出）")
	<-ctx.Done()
}
