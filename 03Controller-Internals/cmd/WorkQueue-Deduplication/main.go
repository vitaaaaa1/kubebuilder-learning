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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// WorkQueueReconciler 用于演示 WorkQueue 机制
type WorkQueueReconciler struct {
	client.Client
}

// Reconcile 是工作队列取出的请求实际执行的地方
func (r *WorkQueueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. 获取最新的 ConfigMap
	// 这一步至关重要，因为我们想看看 WorkQueue 给我们的是哪个版本的数据
	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		logger.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	}

	// 2. 获取 Annotation 里的值，看看当前处理到了哪个版本
	val := cm.Annotations["update"]
	if val == "" {
		val = "N/A (未设置)"
	}

	logger.Info("📦 WorkQueue popped a request!  --> 当前 update 的值是: " + val)

	// 3. 模拟耗时操作：睡 5 秒
	// 这样我们就有机会在它干活的时候，疯狂触发新的事件，看看 WorkQueue 是否会合并请求
	time.Sleep(5 * time.Second)

	logger.Info("✅ Work finished!")
	return ctrl.Result{}, nil
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

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

	// 这里我们将配置 Controller，演示 WorkQueue 的特性
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(&WorkQueueReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager with WorkQueue Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
