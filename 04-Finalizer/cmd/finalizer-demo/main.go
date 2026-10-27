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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const finalizerName = "kubebuilder-learning/finalizer"

// FinalizerReconciler 用于演示如何拦截删除请求
type FinalizerReconciler struct {
	client.Client
}

func (r *FinalizerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("📦 Reconcile triggered", "name", cm.Name)

	// 🔍 核心逻辑：检查是否正在被删除
	if !cm.DeletionTimestamp.IsZero() {
		// 1. 检查是否有我们的 Finalizer
		if controllerutil.ContainsFinalizer(&cm, finalizerName) {
			logger.Info("🛑 Intercepting deletion! Cleaning up external resources...")

			// ---------------------------------------------------------
			// 在这里执行清理逻辑（例如：删除云数据库、取消 DNS 记录等）
			// ---------------------------------------------------------
			time.Sleep(20 * time.Second)

			// 2. 清理完成后，移除 Finalizer
			controllerutil.RemoveFinalizer(&cm, finalizerName)
			/*

				>>
				kind: ConfigMap
				metadata:
				  creationTimestamp: "2026-10-27T02:51:01Z"
				  finalizers:
				  - kubebuilder-learning/finalizer
				  name: finalizers-test
				  namespace: test
				  resourceVersion: "14051058411"
				  uid: bfddcee2-ecc4-45b6-bceb-7cb9246e9331

				>> 执行删除资源操作后
				apiVersion: v1
				kind: ConfigMap
				metadata:
				  creationTimestamp: "2026-10-27T02:51:01Z"
				  deletionGracePeriodSeconds: 0
				  deletionTimestamp: "2026-10-27T02:51:09Z"  // 携带删除标签
				  finalizers:   // controller 监测到携带了 deletionTimestamp 标签后，再执行删除 finalizers 标签操作
				  - kubebuilder-learning/finalizer
				  name: finalizers-test
				  namespace: test
				  resourceVersion: "14051059489"
				  uid: bfddcee2-ecc4-45b6-bceb-7cb9246e9331
			*/

			if err := r.Update(ctx, &cm); err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
			logger.Info("✅ Cleanup done. Finalizer removed. Resource will disappear now.")
		}
		return ctrl.Result{}, nil
	}

	// 3. 如果没有 Finalizer，加上它
	if !controllerutil.ContainsFinalizer(&cm, finalizerName) {
		controllerutil.AddFinalizer(&cm, finalizerName)
		if err := r.Update(ctx, &cm); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		logger.Info("🛡️ Finalizer added to protect resource.")
	}

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

	if err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(&FinalizerReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager with Finalizer Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
