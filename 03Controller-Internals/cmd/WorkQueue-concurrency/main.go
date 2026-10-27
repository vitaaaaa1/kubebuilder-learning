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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ConcurrencyReconciler 用于演示并发处理机制
type ConcurrencyReconciler struct {
	client.Client
}

// Reconcile 逻辑：
// 每次处理都会 Sleep 5秒。
// 如果我们设置并发数为 1，处理 3 个资源需要 15 秒。
// 如果我们设置并发数为 5，处理 3 个资源只需要 5 秒。
func (r *ConcurrencyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 1. 标记开始
	startTime := time.Now()
	logger.Info("🚀 Start processing", "name", req.Name, "time", startTime.Format("15:04:05.000"))

	// 2. 模拟耗时操作 (5秒)
	time.Sleep(5 * time.Second)

	// 3. 标记结束
	endTime := time.Now()
	elapsed := endTime.Sub(startTime)
	logger.Info("✅ Finished processing", "name", req.Name, "duration", elapsed.String(), "time", endTime.Format("15:04:05.000"))

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

	mgr, err := ctrl.NewManager(
		ctrl.GetConfigOrDie(),
		ctrl.Options{
			Scheme: scheme,
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					"test": {},
				},
			},
			HealthProbeBindAddress: ":8081",
		},
	)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// 4. Setup the Controller with Concurrency
	// 🌟 关键点：MaxConcurrentReconciles 决定了同时有多少个 Reconcile 在运行
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5, // 允许同时处理 5 个请求
		}).
		Complete(&ConcurrencyReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info(fmt.Sprintf("🚀 Starting Manager with Concurrency (Workers=5)."))
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
