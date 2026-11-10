// 版本 B：冲突时用 RetryOnConflict 重新读改写。
//
//	go run ./with-retry
//	go run ./with-retry -workers 20
package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"bare-metal-controller/13-conflict-retry/shared"
)

func main() {
	workers := flag.Int("workers", 10, "并发数")
	flag.Parse()

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	if err := shared.Ensure(ctx, c); err != nil {
		panic(err)
	}

	// 默认的 retry.DefaultRetry 只有 5 次、间隔 10ms；
	// 这里并发 10+，给它更多次数以免演示时偶发失败。
	backoff := wait.Backoff{Steps: 100, Duration: 2 * time.Millisecond, Factor: 1.0, Jitter: 0.1}

	var (
		ok       atomic.Int64
		conflict atomic.Int64
		failed   atomic.Int64
	)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			n, err := shared.BumpWithRetry(ctx, c, backoff)
			conflict.Add(int64(n))
			if err != nil {
				failed.Add(1)
				fmt.Println("重试耗尽:", err)
				return
			}
			ok.Add(1)
		}()
	}

	close(start)
	wg.Wait()

	final, _ := shared.Current(ctx, c)
	fmt.Printf("并发数=%d  成功=%d  发生冲突(已重试)=%d  失败=%d  最终计数=%s\n",
		*workers, ok.Load(), conflict.Load(), failed.Load(), final)
}
