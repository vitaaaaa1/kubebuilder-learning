package managercache

import (
	"context"
	"fmt"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// 三个客户端，读法不同，写都直连 apiserver。
var (
	testEnv    *envtest.Environment
	testScheme *runtime.Scheme

	ctx    context.Context
	cancel context.CancelFunc

	cachedClient client.Client // mgr.GetClient()     读 → 缓存，写 → apiserver
	apiReader    client.Reader // mgr.GetAPIReader()  读 → apiserver
	directClient client.Client // client.New()        读/写都直连（和 00 一样）
)

func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))

	code, err := run(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "测试环境启动失败: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func run(m *testing.M) (int, error) {
	testScheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		return 0, err
	}

	testEnv = &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		return 0, err
	}

	ctx, cancel = context.WithCancel(context.Background())

	// 和 00 唯一的区别：多了一个 Manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  testScheme,
		Metrics: metricsserver.Options{BindAddress: "0"}, // 关掉 metrics 端口，避免抢端口
	})
	if err != nil {
		_ = testEnv.Stop()
		return 0, err
	}

	// 启动 mgr------------------------------------------------
	go func() {
		// mgr.Start 是阻塞的
		if err := mgr.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "manager 退出: %v\n", err)
		}
	}()

	// 等所有 informer 完成首次 List+Watch，否则缓存还是空的
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		_ = testEnv.Stop()
		return 0, fmt.Errorf("缓存同步失败")
	}
	// --------------------------------------------------------

	cachedClient = mgr.GetClient()
	apiReader = mgr.GetAPIReader()

	directClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		_ = testEnv.Stop()
		return 0, err
	}

	code := m.Run()

	cancel()
	_ = testEnv.Stop()
	return code, nil
}
