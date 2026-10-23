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
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// RetryReconciler 用于演示 WorkQueue 的自动重试与退避机制
type RetryReconciler struct {
	client.Client
}

// Reconcile 逻辑：
// 如果 ConfigMap 包含 annotation "trigger-error=true"，则返回错误。
// WorkQueue 捕获错误后，会自动按照指数退避策略（5s, 10s, 20s...）重新放入队列重试。
func (r *RetryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		logger.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("📦 Reconcile triggered", "name", cm.Name, "annotations", cm.Annotations)

	// 检查是否触发了“错误开关”
	if cm.Annotations["trigger-error"] == "true" {
		// ❌ 模拟业务逻辑报错
		err := fmt.Errorf("⚠️ Simulated Error: Something went wrong with ConfigMap %s", cm.Name)
		logger.Error(err, "Business logic failed")

		// 返回 error，WorkQueue 会接管并安排重试
		return ctrl.Result{}, err
	}

	// ✅ 正常情况
	logger.Info("✅ Reconcile succeeded")
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
		Metrics: server.Options{
			BindAddress: ":8080",
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithOptions(controller.Options{
			// 配置自定义的指数退避限速器
			RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](
				10*time.Second,  // BaseDelay: 基础延迟，第一次失败后等 1 秒
				120*time.Second, // MaxDelay: 最大延迟，最长等待不超过 1 分钟
			),
		}).
		Complete(&RetryReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager with WorkQueue Retry Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
