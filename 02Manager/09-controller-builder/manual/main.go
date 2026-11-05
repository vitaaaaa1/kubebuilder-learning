// 版本 B：把 NewControllerManagedBy 拆成五步手写
//
// 与 ../with-builder/main.go 相比，只有中间“装配”这一段不同，
// 其余初始化、Reconciler、Manager 配置完全一致。
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"bare-metal-controller/09-controller-builder/shared"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func main() {
	// ───────── 两个文件完全相同 ─────────
	noWatch := flag.Bool("no-watch", false, "只创建 Controller，不接事件源")
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

	// ───────── 唯一不同：五步手写 ─────────

	// 第 1 步：解析 GVK（Builder 的 For 内部做的）
	/*
		GVK 是 Kubernetes 资源的“三段式身份证”：Group + Version + Kind
		对应 YAML 里的两个字段
			apiVersion: apps/v1
			kind: Deployment
		拆开就是：
			Group   = "apps"
			Version = "v1"
			Kind    = "Deployment
	*/
	// 从 Go 对象反查它的 GVK：对象本身不知道它是谁，得查 Scheme 注册表
	// 得到:/v1, Kind=ConfigMap
	gvk, err := apiutil.GVKForObject(&corev1.ConfigMap{}, mgr.GetScheme())
	if err != nil {
		setupLog.Error(err, "解析 GVK 失败")
		os.Exit(1)
	}

	// 第 2 步：控制器名 = kind 小写 + "-controller" 后缀
	//  和 Kubernetes 的命名习惯一致，唯一的硬性校验是“非空 + 进程内唯一”
	name := fmt.Sprintf("%s-controller", strings.ToLower(gvk.Kind))

	// 第 3 步：创建 Controller 并注册进 Manager
	// controller.New 负责创建三样东西：
	//	1.一个工作队列（queue）
	//	2.若干 worker
	//	3.一个“事件源列表”
	//	随后把这个控制器注册进 Manager
	/*
			controller
			  ├── Name      "configmap-controller"
			  ├── Queue     workqueue（去重、限速、重试）
			  ├── Workers   MaxConcurrentReconciles 个 goroutine
			  └── Sources   []source.Source     ← 现在是空的
		controller.New = 建队列 + 建 worker + 注册进 Manager；它只准备“处理能力”，不准备“事件来源”。
		source.Kind + Watch 才是把对象变化送进队列的环节
	*/
	c, err := controller.New(
		name,
		mgr,
		controller.Options{
			/*
				字段							默认						作用
				Reconciler					必填						真正处理请求的对象
				MaxConcurrentReconciles		1						worker 数量
				RateLimiter					指数退避（5ms→1000s）		失败重试节奏
				CacheSyncTimeout			2 分钟					等缓存同步的超时
				RecoverPanic				继承 Manager				panic 后是否继续
				SkipNameValidation			false					是否跳过重名检查
			*/
			Reconciler: &shared.Reconciler{
				Assembly: "manual",
				Client:   mgr.GetClient(),
			},
		},
	)
	if err != nil {
		setupLog.Error(err, "controller.New 失败")
		os.Exit(1)
	}

	// 第 4 步：创建事件源（For 内部用 source.Kind 实现）
	//
	// 显式写出类型参数 [client.Object]，避免“类型被推断”的隐式写法。
	// handler.EnqueueRequestForObject 是 TypedEventHandler[client.Object, Request]，
	// 所以 object 也必须是 client.Object，两边才对得上。
	src := source.Kind[client.Object](
		mgr.GetCache(),
		&corev1.ConfigMap{},
		&handler.EnqueueRequestForObject{},
	)

	// 第 5 步：把事件源挂到 Controller 上
	if *noWatch {
		setupLog.Info("已跳过 Watch：只有 worker，没有事件源")
	} else if err := c.Watch(src); err != nil {
		setupLog.Error(err, "Watch 失败")
		os.Exit(1)
	}

	setupLog.Info("装配方式：五步手写")
	// ───────── 不同到此为止 ─────────

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Manager 退出")
		os.Exit(1)
	}
}
