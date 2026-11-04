package getbranchenvtest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func newReconciler() *Reconciler {
	return &Reconciler{Client: k8sClient}
}

// dumpK8sError 把 Kubernetes 错误原样打印出来。
//
// 为什么需要它：
//
//	err.Error() 只返回 apiserver Status 里的 Message 字段，
//	而 Code / Reason / Details 这些"原始信息"都还在 *apierrors.StatusError 里。
//	这里把 ErrStatus 直接序列化成 JSON —— 就是 apiserver 当年返回的那个 Status 对象。
func dumpK8sError(err error) string {
	if err == nil {
		return "<nil>"
	}

	var se *apierrors.StatusError
	if errors.As(err, &se) {
		if b, mErr := json.MarshalIndent(se.ErrStatus, "", "  "); mErr == nil {
			return string(b)
		}
	}

	// 不是 StatusError（网络错误、超时等），退化成普通文本
	return err.Error()
}

// TestEnv_ReconcileOnMissingObject 验证：对象不存在时，Reconcile 返回 nil。
//
// 同时把"原始错误"打出来，方便对照：
//  1. 先直接问 apiserver 一次，拿到未经处理的 Status（完整 JSON）
//  2. 再走 Reconcile，确认它把 NotFound 吞成了 nil
func TestEnv_ReconcileOnMissingObject(t *testing.T) {
	ctx := context.Background()
	key := types.NamespacedName{
		Name:      "definitely-not-exist",
		Namespace: "default",
	}

	_, err := newReconciler().Reconcile(ctx, ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("对象不存在时必须返回 nil，实际返回:\n%s", dumpK8sError(err))
	}
}
