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

	// Import core k8s types we want to watch
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SimpleReconciler is our bare-metal reconciler
type SimpleReconciler struct {
	client.Client
}

// Reconcile is the heart of the controller.
func (r *SimpleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("🔥 Reconcile triggered!", "namespace", req.Namespace, "name", req.Name)

	// logic
	var cm corev1.ConfigMap
	err := r.Get(ctx, req.NamespacedName, &cm)
	if err != nil {
		return ctrl.Result{}, err
	}
	// 2.
	secretName := cm.Name + "-child-secret"
	var secret corev1.Secret

	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cm.Namespace}, &secret)

	if err != nil && apierrors.IsNotFound(err) {
		logger.Info("✨ Creating child Secret automatically", "name", secretName)
		secretToCreate := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: cm.Namespace,
			},
			Data: map[string][]byte{
				"config-name": []byte(cm.Name), // byte(..) 表示类型转换， 转换为 byte
			},
		}

		// 🌟 核心步骤：建立父子关系
		// 这行代码会自动把 cm 的信息填入 Secret 的 ownerReferences 中
		err := ctrl.SetControllerReference(&cm, secretToCreate, r.Scheme())
		if err != nil {
			return ctrl.Result{}, err
		}

		//
		err = r.Create(ctx, secretToCreate)
		if err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("✅ Child Secret created successfully")
	} else {
		logger.Info("✅ Child Secret already exists")
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
	_ = corev1.AddToScheme(scheme) // 注册 ConfigMap 和其他 corev1 资源

	mgr, err := ctrl.NewManager(
		ctrl.GetConfigOrDie(),
		ctrl.Options{
			Scheme: scheme,
			Metrics: server.Options{
				BindAddress: ":8080",
			},
			LeaderElection:          true,
			LeaderElectionID:        "03-controller-lock",
			LeaderElectionNamespace: "default",
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					"test": {},
				},
			},
			HealthProbeBindAddress:  ":8081",
			GracefulShutdownTimeout: func(d time.Duration) *time.Duration { return &d }(10 * time.Second),
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

	// 4. Setup the Controller
	// 我们将在接下来的实验中修改这里，添加 Owns() 和 Watches()
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(&SimpleReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConfigMap")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager. Press Ctrl+C to stop.")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
