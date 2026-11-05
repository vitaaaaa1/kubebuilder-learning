package managercache

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func newCM(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string]string{"k": "v"},
	}
}

func key(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: "default", Name: name}
}

//// ============================================================
//// 实验 A：同一个 Get，缓存 vs 直连，差多少？
//// ============================================================
//
//func TestCacheVsDirectSpeed(t *testing.T) {
//	ctx := context.Background()
//	name := fmt.Sprintf("speed-%d", time.Now().UnixNano())
//
//	// 直连 directClient
//	if err := directClient.Create(ctx, newCM(name)); err != nil {
//		t.Fatalf("创建失败: %v", err)
//	}
//
//	var cm corev1.ConfigMap
//
//	// 缓存 cachedClient
//	// 预热：第一次读会为该类型创建 informer 并等待同步，
//	// 这次的开销不算进平均值。
//	if err := cachedClient.Get(ctx, key(name), &cm); err != nil {
//		t.Fatalf("预热读取失败: %v", err)
//	}
//
//	const n = 1000
//
//	// 直连 directClient 测试
//	start := time.Now()
//	for i := 0; i < n; i++ {
//		if err := directClient.Get(ctx, key(name), &cm); err != nil {
//			t.Fatalf("直连读失败: %v", err)
//		}
//	}
//	direct := time.Since(start) / n
//
//	// 缓存 cachedClient 测试
//	start = time.Now()
//	for i := 0; i < n; i++ {
//		if err := cachedClient.Get(ctx, key(name), &cm); err != nil {
//			t.Fatalf("缓存读失败: %v", err)
//		}
//	}
//	cached := time.Since(start) / n
//
//	t.Logf("缓存读: %8v / 次", cached)
//	t.Logf("直连读: %8v / 次", direct)
//	t.Logf("差距  : %.0f 倍", float64(direct)/float64(cached))
//}

// ============================================================
// 实验 B：Create 之后立刻用缓存读，读得到吗？
// ============================================================

func TestCacheLagAfterCreate(t *testing.T) {
	ctx := context.Background()

	// 预热 informer，否则第一次 Get 会阻塞到同步完成，测不出"滞后"
	// controller-runtime 的缓存是懒加载的：第一次 Get 某个类型时，才去创建对应的 informer
	var warm corev1.ConfigMap
	_ = cachedClient.Get(ctx, key("warmup"), &warm)

	const n = 20
	var laggedAt []int // 哪几轮缓存输了
	var caught []time.Duration

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("lag-%d-%d", time.Now().UnixNano(), i)

		if err := directClient.Create(ctx, newCM(name)); err != nil {
			t.Fatalf("创建失败: %v", err)
		}

		var cm corev1.ConfigMap
		err := cachedClient.Get(ctx, key(name), &cm)
		if err == nil {
			t.Logf("i=%02d  读到    （缓存赢了：watch 事件先到）", i)
			continue
		}
		if !apierrors.IsNotFound(err) {
			t.Fatalf("意外错误: %v", err)
		}

		// 观察到了滞后：开始计时，看缓存多久追上
		laggedAt = append(laggedAt, i)
		start := time.Now()
		var waited time.Duration
		for {
			var tmp corev1.ConfigMap
			if err := cachedClient.Get(ctx, key(name), &tmp); err == nil {
				waited = time.Since(start)
				caught = append(caught, waited)
				break
			}
			if time.Since(start) > 5*time.Second {
				t.Fatal("5 秒内缓存仍未同步")
			}
			time.Sleep(100 * time.Microsecond)
		}
		t.Logf("i=%02d  NotFound（缓存输了：等了 %v 才读到）", i, waited)
	}

	// 直接证据：到 etcd 里数一下，20 轮到底创建了几个不同的对象
	var all corev1.ConfigMapList
	if err := directClient.List(ctx, &all); err == nil {
		var created []string
		for _, item := range all.Items {
			if strings.HasPrefix(item.Name, "lag-") {
				created = append(created, item.Name)
			}
		}
		t.Logf("etcd 里 lag-* 对象共 %d 个（每轮一个）", len(created))
		t.Logf("前 3 个: %v", created[:min(3, len(created))])
	}

	t.Logf("共 %d 轮：赢 %d 轮，输 %d 轮", n, n-len(laggedAt), len(laggedAt))
	if len(laggedAt) > 0 {
		t.Logf("输的轮次: %v", laggedAt)
		t.Logf("等待时长: %v", caught)
	}
	/*
	   cache_test.go:123: i=00  NotFound（缓存输了：等了 139.167µs 才读到）
	   cache_test.go:100: i=01  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=02  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=03  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:123: i=04  NotFound（缓存输了：等了 135.625µs 才读到）
	   cache_test.go:100: i=05  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=06  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=07  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=08  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:123: i=09  NotFound（缓存输了：等了 3.417µs 才读到）
	   cache_test.go:100: i=10  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=11  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=12  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=13  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=14  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=15  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=16  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=17  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=18  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:100: i=19  读到    （缓存赢了：watch 事件先到）
	   cache_test.go:135: etcd 里 lag-* 对象共 20 个（每轮一个）
	   cache_test.go:136: 前 3 个: [lag-1793777479501793000-0 lag-1793777479504097000-1 lag-1793777479504828000-2]
	   cache_test.go:139: 共 20 轮：赢 17 轮，输 3 轮
	   cache_test.go:141: 输的轮次: [0 4 9]
	   cache_test.go:142: 等待时长: [139.167µs 135.625µs 3.417µs]
	*/
}

// ============================================================
// 实验 C：同一个瞬间，apiReader 读得到吗？
//
// 用来证明：数据已经写进 etcd，读不到只是"缓存还没收到事件"。
// ============================================================

func TestAPIReaderSeesWriteImmediately(t *testing.T) {
	ctx := context.Background()

	const n = 20
	visible := 0

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("reader-%d-%d", time.Now().UnixNano(), i)

		if err := directClient.Create(ctx, newCM(name)); err != nil {
			t.Fatalf("创建失败: %v", err)
		}

		var cm corev1.ConfigMap
		if err := apiReader.Get(ctx, key(name), &cm); err == nil {
			visible++
		} else {
			t.Logf("apiReader 也没读到: %v", err)
		}
	}

	t.Logf("%d 次「Create 后立即 apiReader 读」中，%d 次立即可见", n, visible)
}
