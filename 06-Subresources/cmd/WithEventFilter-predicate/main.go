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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ============================================================
// 🎯 实验目标：验证 controller-runtime 中 /status 子资源的使用
// ============================================================
//
// 本实验演示：
// 1. Status().Update() — 需要 RV，有 409 冲突风险
// 2. Status().Patch() — 不需要 RV，无冲突风险（推荐）
// 3. "偷渡测试" — 尝试在更新 status 时偷偷改 spec，验证 API Server 隔离
//
// 前置条件（手动执行）：
//   kubectl apply -f cmd/status-kubebuilder/crd.yaml
//   kubectl apply -f cmd/status-kubebuilder/sample.yaml
// ============================================================

// StatusDemoReconciler 用于演示 /status 子资源的更新
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

	logger.Info("📦 Reconcile triggered", "name", app.Name, "rv", app.ResourceVersion)

	// ============================================================
	// 🔴 实验 1: 观察初始状态
	// ============================================================
	fmt.Println("\n============================================================")
	fmt.Println("🔴 实验 1: 观察初始状态")
	fmt.Println("============================================================")
	fmt.Printf("   Spec.Image:     %s\n", app.Spec.Image)
	fmt.Printf("   Spec.Replicas:  %d\n", app.Spec.Replicas)
	fmt.Printf("   Status.Phase:   %s\n", app.Status.Phase)
	fmt.Printf("   Status.Message: %s\n\n", app.Status.Message)

	// ============================================================
	// 🟢 实验 2: Status().Update() — 需要 RV，有 409 风险
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: Status().Update() — 需要 RV，有 409 风险")
	fmt.Println("============================================================")
	fmt.Println()

	// 模拟"并发"场景：先 Get，然后故意等一会儿再 Update
	// 这期间如果其他地方也在改这个资源，就会 409 冲突
	originalForUpdate := &command.DemoApp{}
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, originalForUpdate); err != nil {
		logger.Error(err, "Failed to Get for Update demo")
		return ctrl.Result{}, err
	}

	// 修改 status
	originalForUpdate.Status.Phase = "Running"
	originalForUpdate.Status.Message = "Updated via Status().Update() - requires RV"

	fmt.Printf("📝 执行 Status().Update() (RV=%s)...\n", originalForUpdate.ResourceVersion)

	if err := r.Status().Update(ctx, originalForUpdate); err != nil {
		fmt.Printf("   ❌ Status().Update() 失败 (409 Conflict): %v\n", err)
		fmt.Println("   ⚠️ 这就是为什么我们不推荐 Status().Update()")
	} else {
		fmt.Println("   ✅ Status().Update() 成功!")
		fmt.Println("   ⚠️ 但如果并发有其他写入，这里会 409 Conflict")
	}
	fmt.Println()

	// 等一下，让实验更清晰
	time.Sleep(2 * time.Second)

	// ============================================================
	// 🟡 实验 3: Status().Patch() — 不需要 RV，无冲突风险（推荐）
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟡 实验 3: Status().Patch() — 不需要 RV，无冲突风险（推荐）")
	fmt.Println("============================================================")
	fmt.Println()

	// 1. 获取原始对象
	originalForPatch := &command.DemoApp{}
	if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, originalForPatch); err != nil {
		logger.Error(err, "Failed to Get for Patch demo")
		return ctrl.Result{}, err
	}

	// 2. DeepCopy 作为基准（显式调用 DeepCopyDemoApp 避免 ObjectMeta 冲突）
	patchBase := command.DeepCopyDemoApp(originalForPatch)

	// 3. 只修改 Status
	patchBase.Status.Phase = "Running"
	patchBase.Status.Message = "Updated via Status().Patch() - no RV check needed!"
	patchBase.Status.ReadyReplicas = app.Spec.Replicas // 假设全部就绪

	fmt.Println("📝 执行 Status().Patch() (MergeFrom)...")

	if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(originalForPatch)); err != nil {
		fmt.Printf("   ❌ Status().Patch() 失败: %v\n", err)
	} else {
		fmt.Println("   ✅ Status().Patch() 成功!")
		fmt.Println("   💡 MergeFrom 只需要 RV 用于 3-way merge，不会 409")
	}
	fmt.Println()

	//// ============================================================
	//// 🔵 实验 4: "偷渡测试" — 尝试在更新 status 时改 spec
	//// ============================================================
	//fmt.Println("============================================================")
	//fmt.Println("🔵 实验 4: '偷渡测试' — 验证 API Server 隔离")
	//fmt.Println("============================================================")
	//fmt.Println()
	//fmt.Println("说明: 我们尝试在 Patch Status 时，偷偷修改 Spec.Image")
	//fmt.Println("如果服务器忽略了 Spec 的修改，说明子资源隔离生效。")
	//fmt.Println()
	//
	//smuggleOriginal := &command.DemoApp{}
	//if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, smuggleOriginal); err != nil {
	//	logger.Error(err, "Failed to Get for smuggling test")
	//	return ctrl.Result{}, err
	//}
	//
	//smuggleBase := command.DeepCopyDemoApp(smuggleOriginal)
	//
	//// 合法的 Status 更新
	//smuggleBase.Status.Phase = "Hacked"
	//smuggleBase.Status.Message = "试图偷渡 Spec.Image 的修改!"
	//
	//// ❌ 恶意的 Spec 偷渡
	//oldImage := smuggleBase.Spec.Image
	//smuggleBase.Spec.Image = "hacked-image:latest"
	//
	//fmt.Printf("📝 执行 Status().Patch() (携带 Spec.Image=%s)...\n", smuggleBase.Spec.Image)
	//
	//if err := r.Status().Patch(ctx, smuggleBase, client.MergeFrom(smuggleOriginal)); err != nil {
	//	fmt.Printf("   ❌ Patch 失败: %v\n", err)
	//} else {
	//	fmt.Println("   ✅ Patch 请求被接受!")
	//}
	//
	//// 验证结果
	//time.Sleep(1 * time.Second)
	//final := &command.DemoApp{}
	//if err := r.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, final); err != nil {
	//	logger.Error(err, "Failed to Get final state")
	//	return ctrl.Result{}, err
	//}
	//
	//fmt.Println("\n📊 验证最终状态:")
	//fmt.Printf("   Status.Phase:   %s (✅ 成功更新)\n", final.Status.Phase)
	//if final.Spec.Image == oldImage {
	//	fmt.Printf("   Spec.Image:     %s (✅ 保持原值! 偷渡失败 -> 隔离生效!)\n", final.Spec.Image)
	//} else {
	//	fmt.Printf("   Spec.Image:     %s (❌ 被改成了 %s -> 隔离失效!)\n", final.Spec.Image, smuggleBase.Spec.Image)
	//}
	//fmt.Println()
	//
	//// ============================================================
	//// 📊 核心结论
	//// ============================================================
	//fmt.Println("============================================================")
	//fmt.Println("📊 核心结论")
	//fmt.Println("============================================================")
	//fmt.Println()
	//fmt.Println("   1. Status().Update() 需要 RV，并发时可能 409 Conflict → 不推荐")
	//fmt.Println("   2. Status().Patch(MergeFrom) 不需要 RV 检查，无冲突风险 → 推荐")
	//fmt.Println("   3. 即使在 Patch 时偷偷改了 Spec，API Server 也会静默丢弃 → 隔离生效")
	//fmt.Println("   4. 在 Kubebuilder 生成的 Controller 中，")
	//fmt.Println("      使用 r.Status().Patch(ctx, obj, client.MergeFrom(original)) 是最佳实践")
	//fmt.Println()
	//
	//// 只运行一次，然后退出
	//os.Exit(0)

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

	if err = ctrl.NewControllerManagedBy(mgr).
		For(&command.DemoApp{}).
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
