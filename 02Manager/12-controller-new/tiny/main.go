// 最小模型：不用 Manager、不用集群、不用 controller.New。
// 只用「队列 + 一个 for 循环」，把 Reconcile 的重试逻辑跑一遍。
//
//	go run ./tiny
package main

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func main() {
	// ① 建一个带退避的队列：失败后 1s、2s、4s… 再重试
	q := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](time.Second, 10*time.Second),
		workqueue.TypedRateLimitingQueueConfig[reconcile.Request]{Name: "tiny"},
	)
	defer q.ShutDown()

	// ② 事件源：往队列里塞一个请求（真实场景里是 informer 塞的）
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "test",
			Name:      "demo",
		},
	}
	fmt.Printf("事件源：塞入 %s\n", req)
	q.Add(req)

	// ③ worker：一个死循环，不断从队列取
	attempt := 0
	for {
		item, shutdown := q.Get() // 队列空就阻塞
		if shutdown {
			return
		}

		func() {
			defer q.Done(item) // 处理完了

			attempt++
			fmt.Printf("%s  第 %d 次处理 %s\n", time.Now().Format("15:04:05"), attempt, item)

			if attempt < 6 {
				fmt.Println("          失败 → 重新入队（退避后重试）")
				q.AddRateLimited(item)
				return
			}
			fmt.Println("          成功 → Forget（清掉重试计数）")
			q.Forget(item)
		}()

		if attempt >= 6 {
			return
		}
	}
}
