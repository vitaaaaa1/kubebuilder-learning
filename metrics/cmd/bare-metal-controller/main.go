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

	// Import core k8s types we want to watch
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// SimpleReconciler is our bare-metal reconciler
// It needs a Client to talk to the API Server
type SimpleReconciler struct {
	client.Client
}

// Reconcile is the heart of the controller.
// It gets called whenever a ConfigMap changes.
func (r *SimpleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get a logger
	logger := log.FromContext(ctx)

	// Log the event
	logger.Info("🔥 Reconcile triggered!", "namespace", req.Namespace, "name", req.Name)

	// In a real controller, we would:
	// 1. Fetch the object: r.Get(ctx, req.NamespacedName, &cm)
	// 2. Check logic/Modify it
	// 3. Update status or child resources

	return ctrl.Result{}, nil
}

func main() {
	// 1. Setup Logger
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	// 2. Create a Scheme
	// Scheme tells the client which types are available.
	// We MUST add corev1 because we are going to watch ConfigMaps.
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// 3. Create a Manager
	// Manager handles: Leader Election, Shared Informers (caching), and starting Controllers.
	// It connects to the cluster using your KUBECONFIG.
	mgr, err := ctrl.NewManager(
		ctrl.GetConfigOrDie(), // 按顺序尝试寻找集群配置
		ctrl.Options{
			Scheme: scheme,
			Metrics: server.Options{
				BindAddress: ":8080", // 在这里设置 Prometheus 抓取端口
			},

			// ===== 实验选项 1: 领导选举 =====
			// 开启后，日志里会出现 "Attempting to acquire leader lease"
			// 只有抢到锁的实例才会真正启动 Controller 干活
			LeaderElection:          true,
			LeaderElectionID:        "my-test-controller-lock", // 锁的名字
			LeaderElectionNamespace: "default",                 // 在本地运行（不在集群的 Pod 里），开启了 LeaderElection（领导选举），但程序不知道该把“锁”（Lease 资源）放在哪个命名空间。
			// 在集群内运行时，它默认会用 Pod 所在的命名空间。但在本地运行时，你需要手动指定一个命名空间。

			// ===== 实验选项 2: 缓存限制 =====
			// 默认情况下 Manager 会缓存所有命名空间的资源。
			// 这里限制它只同步 'test' 命名空间的资源。
			// 效果：极大节省内存，且只会触发 'test' 下的事件
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					"test": {},
				},
			},
		},
	)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// 4. Setup the Controller
	// This is where we tell the Manager: "Hey, watch ConfigMaps!"
	// Note: Because we restricted the Cache to the "test" namespace in Manager Options,
	// this controller will ONLY see ConfigMaps in the "test" namespace automatically.
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Complete(&SimpleReconciler{Client: mgr.GetClient()}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConfigMap")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager. Press Ctrl+C to stop.")

	// 5. Start the Manager
	// This blocks until the process receives a signal.
	// It starts the HTTP health servers, the caches, and the controller workers.
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
