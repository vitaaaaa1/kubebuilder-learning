package envtestbasic

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapReconciler 是本章的被测对象：监听 ConfigMap，为每个 ConfigMap 维护一个派生的 Secret。
//
// 它刻意保持"刚好能写出三条有意义的断言"的复杂度：
//   - 有子对象           → 可以测 ownerReference
//   - 有"已存在则跳过"   → 可以测幂等（重复 Reconcile 不改 ResourceVersion）
//   - 有"漂移则修复"     → 可以测自愈
//   - 有 NotFound 分支   → 可以测错误处理
//
// 注意：这里没有 main.go，也没有 Manager。本章只测 Reconcile 函数本身，
// 直接调用它比"起一个 Manager 然后 Eventually 等待"更确定、更不容易 flaky。
// Manager + 缓存的测试形态放到 02 小节。
type ConfigMapReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// dataKey 是派生 Secret 里承载"源 ConfigMap 名字"的键。
const dataKey = "config-name"

// ChildSecretName 返回某个 ConfigMap 对应的子 Secret 名字。
// 抽成函数是为了让测试和实现共用同一个命名规则，避免两边各写一遍字符串。
func ChildSecretName(cmName string) string {
	return cmName + "-child-secret"
}

func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		// 对象已经不在了：这不是错误，而是"没什么可做的"。
		// 如果这里原样返回 err，controller-runtime 会把它当成失败并退避重试，
		// 于是删除一个 ConfigMap 会引发一串必然失败的 Reconcile —— 这是新手最常见的坑之一。
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	target := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}

	var secret corev1.Secret
	err := r.Get(ctx, target, &secret)

	switch {
	case apierrors.IsNotFound(err):
		// 子对象不存在 → 创建它，并把 ownerReference 指回 ConfigMap。
		secret = corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      target.Name,
				Namespace: target.Namespace,
			},
			Data: map[string][]byte{dataKey: []byte(cm.Name)},
		}

		// SetControllerReference 会读取 owner 的 UID 写进 ownerReferences。
		// 这一步是本章第一个"真假分水岭"：真实 apiserver 一定分配了 UID，
		// 而 fake client 不会，所以同一行代码在 fake 下会直接报错。
		if err := ctrl.SetControllerReference(&cm, &secret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, &secret); err != nil {
			return ctrl.Result{}, err
		}

		logger.V(1).Info("child Secret created", "secret", target.String())
		return ctrl.Result{}, nil

	case err != nil:
		return ctrl.Result{}, err
	}

	// 子对象已存在。只有当数据漂移时才写回 —— 这一步就是"幂等"的落点：
	// 状态已经符合期望时，一次写入都不发生，ResourceVersion 就不会变。
	if string(secret.Data[dataKey]) != cm.Name {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[dataKey] = []byte(cm.Name)
		if err := r.Update(ctx, &secret); err != nil {
			return ctrl.Result{}, err
		}
		logger.V(1).Info("child Secret repaired", "secret", target.String())
	}

	return ctrl.Result{}, nil
}
