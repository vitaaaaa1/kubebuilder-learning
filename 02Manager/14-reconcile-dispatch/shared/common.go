// Package shared 只做一件事：把 Manager、缓存、控制器装配好，然后启动。
//
// 六个实验的差异全部在各自 main.go 的 Reconcile 里。
package shared

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const Namespace = "test"

// Start 启动一个只监听 test 下 demo-* ConfigMap 的控制器。
//
//	syncPeriod：缓存多久把对象重新喂一遍（06 用）
//	names：启动前确保存在的演示对象
func Start(r reconcile.Reconciler, opts controller.Options, syncPeriod time.Duration, names ...string) error {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	cfg := ctrl.GetConfigOrDie()

	if len(names) == 0 {
		names = []string{"demo-dispatch"}
	}
	if err := ensure(cfg, scheme, names); err != nil {
		return err
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{Namespace: {}},
			SyncPeriod:        &syncPeriod,
		},
	})
	if err != nil {
		return err
	}

	// 只处理带 demo=dispatch 标签的对象，过滤掉 kube-root-ca.crt、demo-config 等噪音
	onlyDemo := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetLabels()["demo"] == "dispatch"
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		Named("dispatch-demo").
		For(&corev1.ConfigMap{}).
		WithEventFilter(onlyDemo).
		WithOptions(opts).
		Complete(r); err != nil {
		return err
	}

	return mgr.Start(ctrl.SetupSignalHandler())
}

// ensure 用直连客户端创建演示对象（此时缓存还没启动）。
func ensure(cfg *rest.Config, scheme *runtime.Scheme, names []string) error {
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	for _, name := range names {
		key := types.NamespacedName{Namespace: Namespace, Name: name}
		err := c.Get(context.Background(), key, &corev1.ConfigMap{})
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		if err := c.Create(context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: Namespace,
				Labels:    map[string]string{"demo": "dispatch"},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
