// 版本 A：读 → 改 → 写，遇到冲突直接失败。
//
//	go run ./naive
//	go run ./naive -workers 20
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"bare-metal-controller/13-conflict-retry/shared"

	corev1 "k8s.io/api/core/v1"
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

	var (
		ok        atomic.Int64
		conflicts atomic.Int64
		other     atomic.Int64
	)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := shared.BumpNaive(ctx, c)
			switch {
			case err == nil:
				ok.Add(1)
			case apierrors.IsConflict(err):
				conflicts.Add(1)
			default:
				other.Add(1)
				fmt.Println("其他错误:", err)
			}
		}()
	}

	close(start) // 同时起跑，制造冲突
	wg.Wait()

	final, _ := shared.Current(ctx, c)
	fmt.Printf("并发数=%d  成功=%d  冲突=%d  其他=%d  最终计数=%s\n",
		*workers, ok.Load(), conflicts.Load(), other.Load(), final)
	if other.Load() > 0 {
		os.Exit(1)
	}
}
