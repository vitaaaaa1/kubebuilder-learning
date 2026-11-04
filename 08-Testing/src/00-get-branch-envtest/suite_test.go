package getbranchenvtest

import (
	"fmt"
	"os"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// 三个包级变量：整个包的测试共用一套 apiserver。
var (
	testScheme *runtime.Scheme // Go 类型 ↔ GVK 的注册表
	k8sClient  client.Client   // 直连 apiserver 的客户端（不经过缓存）
	testEnv    *envtest.Environment
)

// TestMain 是整个包的入口：先起 etcd + kube-apiserver，再跑所有 Test*。
//
// 注意这里没有 defer：os.Exit 不会执行 defer，用 defer 关环境会导致
// etcd / apiserver 进程泄漏在后台。
func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stderr)))

	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "⚠️  KUBEBUILDER_ASSETS 未设置，envtest 找不到 etcd / kube-apiserver。")
		fmt.Fprintln(os.Stderr, "    正确跑法：cd 08-Testing && make test-simple")
		fmt.Fprintln(os.Stderr, "")
	}

	testEnv = &envtest.Environment{}

	cfg, err := testEnv.Start() // 起两个子进程，约 3 秒
	if err != nil {
		fmt.Fprintf(os.Stderr, "envtest 启动失败: %v\n", err)
		fmt.Fprintln(os.Stderr, "提示：先 make setup-envtest && make envtest")
		os.Exit(1)
	}

	testScheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		fmt.Fprintf(os.Stderr, "注册内置类型失败: %v\n", err)
		_ = testEnv.Stop()
		os.Exit(1)
	}

	// 不传 Cache 字段 → 直连客户端。04 小节讲的缓存就是在这里"缺席"的。
	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造 client 失败: %v\n", err)
		_ = testEnv.Stop()
		os.Exit(1)
	}

	code := m.Run()

	_ = testEnv.Stop()
	os.Exit(code)
}
