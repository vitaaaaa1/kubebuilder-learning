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
	"example/command"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type StatusDemoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *StatusDemoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var app command.DemoApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconcile triggered", "rv", app.ResourceVersion)

	// ❌ 关键：每次写入不同的值（时间戳纳秒级，几乎不可能重复）
	app.Status.Message = fmt.Sprintf("reconciled at %s", time.Now().Format(time.RFC3339Nano))

	// 第二层：让写入值收敛
	// 循环的根因不是“调用了 Update”，而是“每次写入的值必然不同”。
	// 所以根治方法是让 Reconcile 计算出的期望状态是确定性函数，不包含 time.Now() 这类永不重复的值。
	// 比如: 创建一个期望状态，如果当前为期望状态就不要执行 Update 逻辑
	//     即使 Update/Patach 时并没有差异，也是一次api调用

	if err := r.Status().Update(ctx, &app); err != nil {
		logger.Error(err, "status update failed")
		return ctrl.Result{}, err
	}

	time.Sleep(10 * time.Second)

	return ctrl.Result{}, nil
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	// 注册 Scheme
	scheme := runtime.NewScheme()
	_ = command.AddKnownTypes(scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				"test": {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// 注册 DemoApp 的 Scheme（用于 controller-runtime 识别 CRD）
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &command.DemoApp{}, "metadata.name", func(o client.Object) []string {
		return []string{o.GetName()}
	}); err != nil {
		setupLog.Error(err, "unable to index field")
		os.Exit(1)
	}

	// 第一层：Predicate 止血
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&command.DemoApp{}).
		// 这个 demo 的写入是 r.Status().Update()，它不会改变 metadata.generation（前提是 CRD 启用了 status 子资源）
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(&StatusDemoReconciler{Client: mgr.GetClient(), Scheme: scheme}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager with /status Subresource Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
