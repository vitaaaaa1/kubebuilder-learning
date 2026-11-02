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
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ============================================================
// 🎯 实验目标：演示如何用 Annotation 模拟自定义子资源
// ============================================================
//
// 本实验演示：
// 1. 用户创建资源时，默认处于 "PendingApproval" 状态
// 2. 用户添加 Annotation `demo.vitaaaaa.icu/approved: "true"` 触发"审批"
// 3. Controller 检测到审批信号后，执行审批逻辑（模拟）
// 4. 审批通过后，Status 变为 "Running"
//
// 前置条件（手动执行）：
//   kubectl apply -f cmd/custom-subresource/crd/crd.yaml
//   kubectl apply -f cmd/custom-subresource/crd/sample.yaml
// ============================================================

// ApprovableAppReconciler 用于演示 Annotation 模拟 /approve 子资源
type ApprovableAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	ApprovalAnnotation = "demo.vitaaaaa.icu/approved"
)

func (r *ApprovableAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var app command.ApprovableApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("📦 Reconcile triggered", "name", app.Name, "phase", app.Status.Phase)

	// ============================================================
	// 🔴 实验 1: 观察初始状态
	// ============================================================
	fmt.Println("\n============================================================")
	fmt.Println("🔴 实验 1: 观察初始状态")
	fmt.Println("============================================================")
	fmt.Printf("   Spec.Image:     %s\n", app.Spec.Image)
	fmt.Printf("   Spec.Replicas:  %d\n", app.Spec.Replicas)
	fmt.Printf("   Status.Phase:   %s\n", app.Status.Phase)
	fmt.Printf("   Annotation[%s]: %s\n", ApprovalAnnotation, app.Annotations[ApprovalAnnotation])
	fmt.Println()

	// ============================================================
	// 🟢 实验 2: 检查审批信号
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: 检查审批信号")
	fmt.Println("============================================================")
	fmt.Println()

	approved := app.Annotations[ApprovalAnnotation] == "true"

	if !approved {
		fmt.Println("   📝 资源未被审批（Annotation != \"true\"）")
		fmt.Println("   💡 用户需要添加 Annotation 来触发审批：")
		fmt.Printf("      kubectl annotate approvableapp %s %s=\"true\" -n %s\n", app.Name, ApprovalAnnotation, app.Namespace)
		fmt.Println()

		// 确保 Status 显示 PendingApproval
		if app.Status.Phase != "PendingApproval" {
			patchBase := command.DeepCopyApprovableApp(&app)
			patchBase.Status.Phase = "PendingApproval"
			patchBase.Status.Message = "Waiting for approval"

			if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(&app)); err != nil {
				logger.Error(err, "Failed to patch status")
				return ctrl.Result{}, err
			}

			fmt.Println("   ✅ Status 更新为 PendingApproval")
		} else {
			fmt.Println("   ✅ Status 已经是 PendingApproval")
		}

		return ctrl.Result{}, nil
	}

	fmt.Printf("   ✅ 检测到审批信号: %s=\"true\"\n", ApprovalAnnotation)
	fmt.Println()

	// ============================================================
	// 🟡 实验 3: 执行审批逻辑
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟡 实验 3: 执行审批逻辑")
	fmt.Println("============================================================")
	fmt.Println()

	if app.Status.Phase == "Approved" {
		fmt.Println("   ✅ 资源已审批通过，无需重复审批")
	} else {
		fmt.Println("   📝 执行审批逻辑...")
		fmt.Println("      - 校验配置合法性")
		fmt.Println("      - 检查资源配额")
		fmt.Println("      - 记录审计日志")
		fmt.Println("      - 激活工作负载")

		// 模拟审批逻辑
		patchBase := command.DeepCopyApprovableApp(&app)
		patchBase.Status.Phase = "Approved"
		patchBase.Status.Message = "Approved by custom-subresource demo"

		if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(&app)); err != nil {
			logger.Error(err, "Failed to patch status after approval")
			return ctrl.Result{}, err
		}

		fmt.Println("   ✅ 审批通过！Status 更新为 Approved")
	}
	fmt.Println()

	//// ============================================================
	//// 🔵 实验 4: 模拟"拒绝"场景
	//// ============================================================
	//fmt.Println("============================================================")
	//fmt.Println("🔵 实验 4: 模拟拒绝/撤销审批")
	//fmt.Println("============================================================")
	//fmt.Println()
	//fmt.Println("说明: 如果用户将 Annotation 改为 \"false\" 或删除，Controller 应该撤销审批。")
	//fmt.Println()
	//fmt.Println("你可以测试:")
	//fmt.Printf("  kubectl annotate approvableapp %s %s=\"false\" --overwrite -n %s\n", app.Name, ApprovalAnnotation, app.Namespace)
	//fmt.Printf("  kubectl annotate approvableapp %s %s- -n %s\n", app.Name, ApprovalAnnotation, app.Namespace)
	//fmt.Println()
	//
	//fmt.Println("============================================================")
	//fmt.Println("📊 实验结论")
	//fmt.Println("============================================================")
	//fmt.Println()
	//fmt.Println("   1. ✅ Annotation 可以模拟 /approve 子资源的行为")
	//fmt.Println("   2. ✅ Controller 监听 Annotation 变化（通过 Informer）")
	//fmt.Println("   3. ✅ 审批逻辑完全由 Controller 控制（灵活）")
	//fmt.Println("   4. ❌ 但 Annotation 不能单独做 RBAC 隔离（这是唯一的缺点）")
	//fmt.Println()
	//
	//// 只运行一次
	//os.Exit(0)

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

	// 使用 ResourceVersionChangedPredicate 来捕获 Annotation 变化
	// （因为 Annotation 变化不改变 generation）
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&command.ApprovableApp{}).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		Complete(&ApprovableAppReconciler{Client: mgr.GetClient(), Scheme: scheme}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting Custom Subresource (Annotation-based Approval) Demo.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
