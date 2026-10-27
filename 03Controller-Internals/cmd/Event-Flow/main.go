/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// EventFlowReconciler 用于演示底层事件流转机制
type EventFlowReconciler struct {
	client.Client
}

func (r *EventFlowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 我们在这里打印 req.Name，看看它到底是谁的名字
	// 是 ConfigMap 的名字？还是 Secret 的名字？
	logger.Info("📦 Event arrived in Reconcile!", "name", req.Name)

	// 这里的逻辑不重要，重要的是观察 req 是从哪里来的
	return ctrl.Result{}, nil
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme) // 注册 ConfigMap
	_ = corev1.AddToScheme(scheme) // Secret 也在 corev1 里

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// =========================================================================
	// 核心实验区：显式展示 For / Owns 背后的真实代码逻辑
	// =========================================================================

	// 定义 Handlers (事件转换器)
	// 当事件进来时，我该把“谁的名字”放入 WorkQueue？

	// 转换器 A：直接把触发事件的 ConfigMap 名字放入队列
	// 这对应了 .For(&corev1.ConfigMap{}) 的行为
	// 注意：EnqueueRequestForObject 是一个结构体，需要取地址
	handlerForConfigMap := &handler.EnqueueRequestForObject{}

	//// 转换器 B：如果 Secret 变了，去查它的 Owner 是谁，把 Owner 的名字放入队列
	//// 这对应了 .Owns(&corev1.Secret{}) 的行为
	//handlerForOwner := handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &corev1.ConfigMap{})

	// =========================================================================
	// 组装 Controller
	// =========================================================================

	if err = ctrl.NewControllerManagedBy(mgr).
		// 这里我们不用 .For() 和 .Owns() 这种语法糖
		// 而是直接用 .Watches() 来展示底层的流转：Source (对象) + Handler (转换器)

		// 实验 1: 监听 ConfigMap -> 使用转换器 A -> 放入 ConfigMap Name
		// 等价于: .For(&corev1.ConfigMap{})
		Watches(
			&corev1.ConfigMap{}, // 监听的源头: 具体是指向 ConfigMap 结构体的指针
			handlerForConfigMap, // 转换器: 当 ConfigMap 变了，请调用这个 Handler
			builder.WithPredicates( // 过滤器: 在把事件交给 Handler 之前，先过一遍这个检查, 只有检查通过（返回 `true`）的事件，Handler 才会工作；否则事件直接丢弃
				predicate.NewPredicateFuncs(
					func(object client.Object) bool {
						return object.GetNamespace() == "test"
					},
				),
			),
		).

		//// 实验 2: 监听 Secret -> 使用转换器 B -> 放入 Owner (ConfigMap) Name
		//// 等价于: .Owns(&corev1.Secret{})
		//Watches(&corev1.Secret{}, handlerForOwner).

		// 因为没用 For()，所以必须手动给 Controller 起个名字
		Named("event-flow-controller").
		Complete(&EventFlowReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager with Explicit Event Flow Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
