// Package shared 放两个版本共用的 Reconciler。
//
// 行为：同一个请求前 2 次失败，第 3 次成功——用来观察重试与退避。
package shared

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Reconciler struct {
	Assembly string

	mu       sync.Mutex
	attempts map[types.NamespacedName]int
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.mu.Lock()
	if r.attempts == nil {
		r.attempts = map[types.NamespacedName]int{}
	}
	r.attempts[req.NamespacedName]++
	n := r.attempts[req.NamespacedName]
	r.mu.Unlock()

	log := ctrl.LoggerFrom(ctx)
	if n <= 2 {
		log.Info("Reconcile 失败（模拟）", "assembly", r.Assembly, "attempt", n)
		return ctrl.Result{}, fmt.Errorf("模拟第 %d 次失败", n)
	}
	log.Info("Reconcile 成功", "assembly", r.Assembly, "attempt", n)
	return ctrl.Result{}, nil
}
