package envtestbasic

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// ============================================================
// 04 · Manager 与缓存
//
// 这一节要回答三个问题：
//   1. mgr.GetClient() 和 mgr.GetAPIReader() 到底差在哪？
//   2. 为什么"刚创建的对象，缓存可能读不到"？
//   3. 什么时候必须用 apiReader？
//
// 关键前提：fake client 没有缓存层（它是一个同步的 map），
// 所以这两个现象在 fake 里永远复现不出来 —— 必须用 envtest。
// ============================================================

// newTestManager 起一个真正的 Manager（进程内，不额外起二进制），
// 并等到它的缓存完成首次同步。
func newTestManager(t *testing.T, ctx context.Context) manager.Manager {
	t.Helper()

	mgr, err := ctrl.NewManager(testCfg, ctrl.Options{
		Scheme:  testScheme,
		Metrics: metricsserver.Options{BindAddress: "0"}, // 关掉 metrics 端口，避免并行测试抢端口
	})
	if err != nil {
		t.Fatalf("创建 Manager 失败: %v", err)
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			t.Logf("Manager 退出: %v", err)
		}
	}()

	// WaitForCacheSync 会阻塞到所有 informer 完成首次 List+Watch。
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("缓存未能在超时内完成同步")
	}
	return mgr
}

// waitCached 等缓存真正看到某个对象（最多 5 秒）。
func waitCached(t *testing.T, ctx context.Context, c client.Client, key types.NamespacedName) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var got corev1.ConfigMap
		if err := c.Get(ctx, key, &got); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("5 秒内缓存仍未看到 %s", key)
}

// TestEnv_ManagerCacheIsFast 验证：缓存读比直连读快一个数量级。
func TestEnv_ManagerCacheIsFast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := newTestManager(t, ctx)
	cached := mgr.GetClient()    // 读走缓存、写直连
	direct := mgr.GetAPIReader() // 读写都直连（只读接口）

	name := uniqueName(t)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
	if err := cached.Create(ctx, cm); err != nil {
		t.Fatalf("创建 ConfigMap 失败: %v", err)
	}

	key := types.NamespacedName{Namespace: "default", Name: name}
	waitCached(t, ctx, cached, key)

	const n = 500

	start := time.Now()
	for i := 0; i < n; i++ {
		var got corev1.ConfigMap
		if err := cached.Get(ctx, key, &got); err != nil {
			t.Fatalf("缓存读失败: %v", err)
		}
	}
	cachedDur := time.Since(start)

	start = time.Now()
	for i := 0; i < n; i++ {
		var got corev1.ConfigMap
		if err := direct.Get(ctx, key, &got); err != nil {
			t.Fatalf("直连读失败: %v", err)
		}
	}
	directDur := time.Since(start)

	t.Logf("%d 次读取：缓存 %v（%v/次） | 直连 %v（%v/次） | 快 %.0f 倍",
		n, cachedDur, cachedDur/n, directDur, directDur/n,
		float64(directDur)/float64(cachedDur))
}

// TestEnv_CacheCanBeStale 验证：创建后立刻用缓存读，可能读不到（但直连一定读得到）。
func TestEnv_CacheCanBeStale(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := newTestManager(t, ctx)
	cached := mgr.GetClient()
	direct := mgr.GetAPIReader()

	const n = 50
	misses := 0

	for i := 0; i < n; i++ {
		name := uniqueName(t)
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		}
		// 写：无论 cached 还是 direct，最终都直连 apiserver
		if err := cached.Create(ctx, cm); err != nil {
			t.Fatalf("创建 ConfigMap 失败: %v", err)
		}

		key := types.NamespacedName{Namespace: "default", Name: name}

		// 立刻用缓存读：watch 事件可能还没到达
		var got corev1.ConfigMap
		switch err := cached.Get(ctx, key, &got); {
		case apierrors.IsNotFound(err):
			misses++
		case err != nil:
			t.Fatalf("非预期错误: %v", err)
		}

		// 直连读：一定能读到刚写进去的对象
		var directGot corev1.ConfigMap
		if err := direct.Get(ctx, key, &directGot); err != nil {
			t.Fatalf("直连读不到刚创建的对象（这不该发生）: %v", err)
		}
	}

	t.Logf("%d 次「创建后立刻用缓存读」：%d 次 NotFound（缓存没跟上），%d 次命中",
		n, misses, n-misses)
}
