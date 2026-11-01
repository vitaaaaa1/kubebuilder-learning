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
	"k8s.io/apimachinery/pkg/types"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ============================================================
// 🎯 实验目标：验证 CRD 的 /scale 子资源
// ============================================================
//
// 本实验演示：
// 1. 读取 /scale 子资源 — 获取 replicas 和 selector
// 2. 写入 /scale 子资源 — 通过 scale 端点修改 replicas
// 3. HPA 集成 — 创建 HPA 并观察它自动调整 replicas
//
// 前置条件（手动执行）：
//   kubectl apply -f cmd/scale-subresource/crd.yaml
//   kubectl apply -f cmd/scale-subresource/sample.yaml
// ============================================================

// ScaleDemoReconciler 用于演示 /scale 子资源的 Controller
type ScaleDemoReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ScaleDemoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var app command.DemoApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("📦 Reconcile triggered", "name", app.Name, "rv", app.ResourceVersion)

	// ============================================================
	// 🔴 实验 1: 观察初始状态
	// ============================================================
	fmt.Println("\n============================================================")
	fmt.Println("🔴 实验 1: 观察初始状态")
	fmt.Println("============================================================")
	fmt.Printf("   Spec.Replicas:     %d\n", app.Spec.Replicas)
	fmt.Printf("   Status.Replicas:   %d\n", app.Status.Replicas)
	fmt.Printf("   Status.ReadyReplicas: %d\n", app.Status.ReadyReplicas)
	fmt.Printf("   Status.Selector:   %s\n", app.Status.Selector)
	fmt.Printf("   Status.Phase:      %s\n", app.Status.Phase)
	fmt.Println()

	// ============================================================
	// 🟢 实验 2: 读取 /scale 子资源
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: 读取 /scale 子资源")
	fmt.Println("============================================================")
	fmt.Println()

	// 重新获取最新的对象
	scaleLatest := &command.DemoApp{}
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, scaleLatest); err != nil {
		logger.Error(err, "Failed to Get for scale read demo")
		return ctrl.Result{}, err
	}

	fmt.Printf("   ✅ Spec.Replicas:      %d (相当于 Scale.Spec.Replicas)\n", scaleLatest.Spec.Replicas)
	fmt.Printf("   ✅ Status.ReadyReplicas: %d (相当于 Scale.Status.Replicas)\n", scaleLatest.Status.ReadyReplicas)
	fmt.Printf("   ✅ Status.Selector:    %s (相当于 Scale.Status.Selector)\n", scaleLatest.Status.Selector)
	fmt.Println()

	// ============================================================
	// 🟡 实验 3: 写入 /scale 子资源 — 修改 replicas
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟡 实验 3: 写入 /scale 子资源 — 修改 replicas")
	fmt.Println("============================================================")
	fmt.Println()

	// 重新获取最新的对象（避免 RV 冲突）
	latest := &command.DemoApp{}
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, latest); err != nil {
		logger.Error(err, "Failed to Get latest object")
		return ctrl.Result{}, err
	}

	// 通过 /scale 端点修改 replicas
	fmt.Printf("📝 执行 Scale.SubResource().Update() (replicas: %d → 4)...\n", latest.Spec.Replicas)

	// 直接修改 latest 对象的 spec，然后通过 scale 子资源写入
	latest.Spec.Replicas = 4
	if err := r.Update(ctx, latest); err != nil {
		fmt.Printf("   ❌ 写入 replicas 失败: %v\n", err)
		return ctrl.Result{}, err
	}

	fmt.Println("   ✅ 写入 replicas 成功!")
	fmt.Println("   💡 这会直接修改 Spec.Replicas，不影响其他字段")
	fmt.Println()

	// 验证修改结果
	time.Sleep(2 * time.Second)
	verify := &command.DemoApp{}
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, verify); err != nil {
		logger.Error(err, "Failed to Get final state")
		return ctrl.Result{}, err
	}

	fmt.Println("📊 验证最终状态:")
	fmt.Printf("   Spec.Replicas:   %d (✅ 应该变成 4)\n", verify.Spec.Replicas)
	fmt.Printf("   Status.Replicas: %d (Controller 尚未同步)\n", verify.Status.Replicas)
	fmt.Println()

	// ============================================================
	// 🔵 实验 4: 更新 Status（同步 replicas 到 status）
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🔵 实验 4: 更新 Status（同步 replicas 到 status）")
	fmt.Println("============================================================")
	fmt.Println()

	// 模拟 Controller 的职责：将 Spec.Replicas 同步到 Status
	patchBase := command.DeepCopyDemoApp(verify)
	patchBase.Status.Replicas = verify.Spec.Replicas
	patchBase.Status.ReadyReplicas = verify.Spec.Replicas
	patchBase.Status.Phase = "Running"
	patchBase.Status.Message = "Scaled via /scale subresource"

	if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(verify)); err != nil {
		fmt.Printf("   ❌ 更新 Status 失败: %v\n", err)
		return ctrl.Result{}, err
	}

	fmt.Println("   ✅ Status 更新成功!")
	fmt.Println("   💡 现在 Status.ReadyReplicas 和 Spec.Replicas 一致")
	fmt.Println("   💡 HPA 可以读取这个值来确认缩放是否生效")
	fmt.Println()

	// ============================================================
	// 🟣 实验 5: 验证 HPA 能否工作
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟣 实验 5: HPA 集成验证")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: HPA 需要集群已安装 metrics-server。")
	fmt.Println("如果 metrics-server 未安装，HPA 会报 unable to fetch metrics。")
	fmt.Println()
	fmt.Println("你可以通过以下命令手动验证 HPA:")
	fmt.Println("  kubectl get hpa demoapp-hpa -n test -w")
	fmt.Println("  kubectl get demoapp scale-demo -n test -w")
	fmt.Println()
	fmt.Println("HPA 会自动根据 CPU 使用率调整 replicas。")

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

	if err = ctrl.NewControllerManagedBy(mgr).
		For(&command.DemoApp{}).
		Complete(&ScaleDemoReconciler{Client: mgr.GetClient(), Scheme: scheme}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Manager with /scale Subresource Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
