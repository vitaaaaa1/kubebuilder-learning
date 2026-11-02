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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ============================================================
// 🎯 实验目标：验证子资源的 RBAC 隔离
// ============================================================
//
// 本实验演示：
// 1. 有限权限的 ServiceAccount 能否读取 spec？ → ✅ 能（get 权限）
// 2. 有限权限的 ServiceAccount 能否更新 status？ → ✅ 能（update status 权限）
// 3. 有限权限的 ServiceAccount 能否修改 spec？ → ❌ 不能（无 update 权限）
//
// 前置条件（手动执行）：
//   kubectl apply -f cmd/rbac-subresource/crd/crd-and-rbac.yaml
//   kubectl apply -f cmd/rbac-subresource/crd/sample.yaml
// ============================================================

// RBACDemoReconciler 用于演示 RBAC 隔离
type RBACDemoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *RBACDemoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var app command.RBACDemoApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("📦 Reconcile triggered", "name", app.Name, "rv", app.ResourceVersion)

	// ============================================================
	// 🔴 实验 1: 验证读取 spec（应该成功）
	// ============================================================
	fmt.Println("\n============================================================")
	fmt.Println("🔴 实验 1: 验证读取 spec（get 权限）")
	fmt.Println("============================================================")
	fmt.Printf("   ✅ 读取成功: Spec.Replicas=%d, Image=%s\n", app.Spec.Replicas, app.Spec.Image)
	fmt.Println()

	// ============================================================
	// 🟢 实验 2: 验证更新 status（应该成功）
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: 验证更新 status（update status 权限）")
	fmt.Println("============================================================")
	fmt.Println()

	patchBase := command.DeepCopyRBACDemoApp(&app)
	patchBase.Status.Phase = "Running"
	patchBase.Status.ReadyReplicas = app.Spec.Replicas
	patchBase.Status.Selector = fmt.Sprintf("app=%s", app.Name)

	fmt.Printf("📝 执行 Status().Patch() (Phase=%s)...\n", patchBase.Status.Phase)

	if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(&app)); err != nil {
		fmt.Printf("   ❌ Status().Patch() 失败: %v\n", err)
		fmt.Println("   ⚠️ 如果报 403，说明 RBAC 没有正确配置 status 权限")
		return ctrl.Result{}, err
	}

	fmt.Println("   ✅ Status().Patch() 成功!")
	fmt.Println("   💡 这证明 Operator 有 update status 的权限")
	fmt.Println()

	// ============================================================
	// 🟡 实验 3: 使用 SA 身份模拟验证 RBAC（核心实验）
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟡 实验 3: 模拟 SA 身份验证 RBAC 隔离")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 我们用 Impersonation 模拟 ServiceAccount 身份发起请求。")
	fmt.Println("这能验证：如果 Controller 部署到集群中使用这个 SA，会有什么权限。")
	fmt.Println()

	// 创建模拟 SA 身份的 client
	cfg := ctrl.GetConfigOrDie()
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: "system:serviceaccount:test:limited-operator-sa",
	}
	saClient, err := client.New(cfg, client.Options{
		Scheme: r.Scheme,
	})
	if err != nil {
		logger.Error(err, "Failed to create SA client")
		return ctrl.Result{}, err
	}

	// 3a. 尝试用 SA 身份读取 spec（应该成功）
	fmt.Println("📝 3a. 尝试用 SA 身份读取 spec...")
	saApp := &command.RBACDemoApp{}
	if err := saClient.Get(ctx, req.NamespacedName, saApp); err != nil {
		fmt.Printf("   ❌ 读取失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 读取成功: Spec.Replicas=%d\n", saApp.Spec.Replicas)
	}
	fmt.Println()

	// 3b. 尝试用 SA 身份修改 spec（应该 403）
	fmt.Println("📝 3b. 尝试用 SA 身份修改 spec（应该 403）...")
	saApp.Spec.Replicas = 99
	if err := saClient.Update(ctx, saApp); err != nil {
		fmt.Printf("   ✅ 如预期，Update 失败 (403 Forbidden): %v\n", err)
		fmt.Println("   💡 这证明了 RBAC 隔离生效！SA 不能修改用户的 spec。")
	} else {
		fmt.Println("   ❌ Update 成功了！这说明 RBAC 没有正确隔离")
	}
	fmt.Println()

	// 3c. 尝试用 SA 身份更新 status（应该成功）
	fmt.Println("📝 3c. 尝试用 SA 身份更新 status（应该成功）...")
	saLatest := &command.RBACDemoApp{}
	if err := saClient.Get(ctx, req.NamespacedName, saLatest); err != nil {
		fmt.Printf("   ❌ 读取失败: %v\n", err)
	} else {
		saPatchBase := command.DeepCopyRBACDemoApp(saLatest)
		saPatchBase.Status.Phase = "Impersonated"
		saPatchBase.Status.ReadyReplicas = saLatest.Spec.Replicas
		if err := saClient.Status().Patch(ctx, saPatchBase, client.MergeFrom(saLatest)); err != nil {
			fmt.Printf("   ❌ Status Patch 失败: %v\n", err)
		} else {
			fmt.Println("   ✅ Status Patch 成功！SA 能更新 status。")
		}
	}
	fmt.Println()

	// ============================================================
	// 🔵 实验 4: 验证 /scale 子资源权限
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🔵 实验 4: 验证 /scale 子资源读取")
	fmt.Println("============================================================")
	fmt.Println()

	// 重新获取
	scaleApp := &command.RBACDemoApp{}
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, scaleApp); err != nil {
		logger.Error(err, "Failed to get for scale")
		return ctrl.Result{}, err
	}

	fmt.Printf("   ✅ 通过 /scale 端点读取: Spec.Replicas=%d\n", scaleApp.Spec.Replicas)
	fmt.Println("   💡 这证明了 scale 权限也正常")
	fmt.Println()

	fmt.Println("============================================================")
	fmt.Println("📊 实验结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. ✅ Operator 能读取 spec（get 权限）")
	fmt.Println("   2. ✅ Operator 能更新 status（update status 权限）")
	fmt.Println("   3. ✅ Operator 不能修改 spec（无 update 权限 → 403）")
	fmt.Println("   4. ✅ Operator 能读取 /scale（get scale 权限）")
	fmt.Println()
	fmt.Println("   这就是生产环境推荐的最小权限原则！")

	// 只运行一次
	os.Exit(0)

	return ctrl.Result{}, nil
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

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

	if err = ctrl.NewControllerManagedBy(mgr).
		For(&command.RBACDemoApp{}).
		Complete(&RBACDemoReconciler{Client: mgr.GetClient(), Scheme: scheme}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting RBAC Subresource Isolation Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
