package envtestbasic

import (
	"fmt"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// testScheme 里注册了 core/v1 等内置类型。
	// envtest 用的是真实 apiserver，类型不全直接 404/序列化失败。
	testScheme *runtime.Scheme

	// k8sClient 是"直连 apiserver"的客户端（没有缓存层）。
	// 01 小节故意这样：断言读到的永远是服务端最新值，测试不会因为缓存没同步而 flaky。
	// 02 小节会换成 Manager.GetClient()（读缓存）+ Manager.GetAPIReader()（直连）的组合，
	// 那时才会遇到"缓存读旧值"的问题。
	k8sClient client.Client
	apiReader client.Reader

	// testCfg 是 envtest 起出来的 apiserver 的 rest.Config。
	// 04 小节要自己 NewManager，所以必须把它从 TestMain 里的局部变量提升为包级变量。
	testCfg *rest.Config

	// testEnv 持有 etcd + kube-apiserver 两个子进程的生命周期。
	testEnv *envtest.Environment
)

// TestMain 是整个包的入口：所有 _test.go 里的用例共用同一个 envtest 实例。
//
// 这样做的代价是"进程级共享"：一个用例污染了对象，另一个用例可能受影响。
// 本章的对策是用唯一的对象名（见 uniqueName），而不是每个用例起一个 apiserver
// —— 起一次大约 3~6 秒，起几十次是不可接受的。
func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))

	// 只跑 fake 系列反例时，没有必要起 apiserver：
	// 一条用例 0.02 秒，改一行代码就能验证分支逻辑。
	// （这个开关由 `make test-fake` 设置；此时任何必须访问 k8sClient 的用例都会空指针 panic，
	//   所以它只应该和 -run TestFake 一起用。）
	if os.Getenv("SKIP_ENVTEST") != "" {
		os.Exit(m.Run())
	}

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "⚠️  未设置 KUBEBUILDER_ASSETS —— envtest 找不到 etcd / kube-apiserver 二进制。")
		fmt.Fprintln(os.Stderr, "    正确跑法（会自动下载并注入路径）：")
		fmt.Fprintln(os.Stderr, "        cd 08-Testing && make test")
		fmt.Fprintln(os.Stderr, "")
	}

	testEnv = &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest 启动失败: %v\n", err)
		fmt.Fprintln(os.Stderr, "提示：先 make setup-envtest && make envtest 准备二进制。")
		os.Exit(1)
	}

	testCfg = cfg

	testScheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		fmt.Fprintf(os.Stderr, "注册内置类型失败: %v\n", err)
		_ = testEnv.Stop()
		os.Exit(1)
	}

	// client.Options 不传 Cache，所以这是直连客户端。
	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造 client 失败: %v\n", err)
		_ = testEnv.Stop()
		os.Exit(1)
	}
	apiReader = k8sClient

	code := m.Run()

	// 不要用 defer：os.Exit 不会执行 defer，etcd 进程会泄漏在后台。
	_ = testEnv.Stop()
	os.Exit(code)
}
