package getbranchenvtest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	//var se *apierrors.StatusError
	//if errors.As(err, &se) { // err是否属于 *apierrors.StatusError 类型的错误
	//	if b, mErr := json.MarshalIndent(se.ErrStatus, "", "  "); mErr == nil {
	//		return string(b)
	//	}
	//}
	/*
			原始报错:
		        {
		          "ErrStatus": {
		            "metadata": {},
		            "status": "Failure",
		            "message": "configmaps \"definitely-not-exist\" not found",
		            "reason": "NotFound",
		            "details": {
		              "name": "definitely-not-exist",
		              "kind": "configmaps"
		            },
		            "code": 404
		          }
		        }
	*/
	if b, mErr := json.MarshalIndent(err, "", "  "); mErr == nil {
		return string(b)
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

// ============================================================
// 第 2 步：对象存在 → 返回 nil，并观察 apiserver 分配的 UID
//
// 这一步是 fake 与 envtest 的**第一个可见差异**：
//   真 apiserver：Create 之后 UID 立刻有值
//   fake client ：UID 是空字符串（fake 不模拟 UID 分配）
// ============================================================

func TestEnv_ReconcileOnExistingObject(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("simple-%d", time.Now().UnixNano())

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string]string{"k": "v"},
	}
	if err := k8sClient.Create(ctx, cm); err != nil {
		t.Fatalf("创建 ConfigMap 失败: %v", dumpK8sError(err))
	}

	// Create 会把服务端的响应解码回 cm，所以 UID / ResourceVersion 已经有了
	if cm.UID == "" {
		t.Fatal("envtest 下 UID 不应为空（fake 下才会为空）")
	}
	t.Logf("apiserver 分配的 UID = %s，ResourceVersion = %s", cm.UID, cm.ResourceVersion)

	_, err := newReconciler().Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("对象存在时不应报错，实际: %v", err)
	}
}

// ============================================================
// 第 3 步：确认"不存在"是一个错误，而不是空值
//
// 这条用例不调用 Reconcile，直接问客户端：
// Get 一个不存在的对象，到底返回什么？
// 这是理解 client.IgnoreNotFound 的前提。
// ============================================================

func TestEnv_NotFoundIsAnError(t *testing.T) {
	ctx := context.Background()

	var cm corev1.ConfigMap
	err := k8sClient.Get(ctx,
		types.NamespacedName{Name: "definitely-not-exist", Namespace: "default"}, &cm)

	t.Logf("原始错误 = %v", err)
	t.Logf("apierrors.IsNotFound(err) = %v", apierrors.IsNotFound(err))

	if err == nil {
		t.Fatal("期望返回 NotFound 错误，实际是 nil")
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("期望 IsNotFound == true，实际错误类型: %T", err)
	}
}
