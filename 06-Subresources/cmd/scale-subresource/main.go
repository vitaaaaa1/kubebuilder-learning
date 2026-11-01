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

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// 🟢 实验 2: 读取 /scale 子资源（真正的 /scale 端点）
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: 读取 /scale 子资源（真正的 /scale 端点）")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 这里使用 SubResource(\"scale\").Get() 读取，")
	fmt.Println("返回的是 autoscaling/v1.Scale 对象，不是完整的 DemoApp。")
	fmt.Println("这与普通 Get() 的区别:")
	fmt.Println("  - 普通 Get() → 获取整个资源（Spec + Status + Metadata）")
	fmt.Println("  - /scale Get() → 只获取 Scale（replicas + selector）")
	fmt.Println()

	// ✅ 真正的 /scale 读取：返回 autoscaling/v1.Scale 对象
	// SubResource("scale").Get(ctx, obj, scale) 需要两个参数：
	//   - obj: 基础对象（用于定位资源）
	//   - scale: 接收 /scale 返回的结果
	baseObj := &command.DemoApp{}
	baseObj.Name = app.Name
	baseObj.Namespace = app.Namespace

	scale := &autoscalingv1.Scale{}
	if err := r.SubResource("scale").Get(ctx, baseObj, scale); err != nil {
		fmt.Printf("   ❌ 读取 /scale 失败: %v\n", err)
		fmt.Println("   ⚠️ 可能原因：CRD 没有启用 /scale 子资源")
		return ctrl.Result{}, err
	}

	fmt.Printf("   ✅ Scale.Spec.Replicas:   %d\n", scale.Spec.Replicas)
	fmt.Printf("   ✅ Scale.Status.Replicas: %d\n", scale.Status.Replicas)
	fmt.Printf("   ✅ Scale.Status.Selector: %s\n", scale.Status.Selector)
	fmt.Println()
	fmt.Println("   💡 对比：如果用普通 Get() 获取整个 DemoApp：")
	fmt.Printf("      - 需要传输整个对象（更大的 JSON）\n")
	fmt.Printf("      - 包含 Status.Phase, Message 等不需要的字段\n")
	fmt.Printf("      - /scale 只传输 replicas + selector（更小、更快）\n")
	fmt.Println()

	// ============================================================
	// 🟡 实验 3: 写入 /scale 子资源 — 修改 replicas
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("🟡 实验 3: 写入 /scale 子资源 — 修改 replicas")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 这里使用 SubResource(\"scale\").Update() 写入，")
	fmt.Println("与普通的 r.Update() 的区别:")
	fmt.Println("  - 普通 Update() → 更新整个资源（Spec + Metadata）")
	fmt.Println("  - /scale Update() → 只更新 Scale 的 replicas 字段")
	fmt.Println("  - /scale Update() 不需要 ResourceVersion 检查")
	fmt.Println()

	// ✅ 真正的 /scale 写入：构造 Scale 对象，通过 /scale 端点写入
	newScale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: 4, // 修改为 4
		},
	}

	fmt.Printf("📝 执行 SubResource(\"scale\").Update() (replicas: %d → 4)...\n", app.Spec.Replicas)

	// SubResource("scale").Update(ctx, obj, subResource) 需要两个参数：
	//   - obj: 基础对象（用于定位资源）
	//   - subResource: Scale 对象（要写入的数据）
	scaleBase := &command.DemoApp{}
	scaleBase.Name = app.Name
	scaleBase.Namespace = app.Namespace

	if err := r.SubResource("scale").Update(ctx, scaleBase, client.WithSubResourceBody(newScale)); err != nil {
		fmt.Printf("   ❌ 写入 /scale 失败: %v\n", err)
		return ctrl.Result{}, err
	}

	fmt.Println("   ✅ 写入 /scale 成功!")
	fmt.Println("   💡 这会直接修改 Spec.Replicas，不影响其他字段")
	fmt.Println("   💡 与普通 Update() 的区别:")
	fmt.Println("      - 普通 Update() 需要 RV 检查（可能 409）")
	fmt.Println("      - /scale Update() 不需要 RV 检查（无 409 风险）")
	fmt.Println("      - /scale Update() 只发送 {\"spec\":{\"replicas\":4}}")
	fmt.Println("      - 普通 Update() 发送整个对象（更大的 JSON）")
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

	//// ============================================================
	//// 🔵 实验 4: 更新 Status（同步 replicas 到 status）
	//// ============================================================
	//fmt.Println("============================================================")
	//fmt.Println("🔵 实验 4: 更新 Status（同步 replicas 到 status）")
	//fmt.Println("============================================================")
	//fmt.Println()
	//
	//// 模拟 Controller 的职责：将 Spec.Replicas 同步到 Status
	//patchBase := command.DeepCopyDemoApp(verify)
	//patchBase.Status.Replicas = verify.Spec.Replicas
	//patchBase.Status.ReadyReplicas = verify.Spec.Replicas
	//patchBase.Status.Phase = "Running"
	//patchBase.Status.Message = "Scaled via /scale subresource"
	//
	//if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(verify)); err != nil {
	//	fmt.Printf("   ❌ 更新 Status 失败: %v\n", err)
	//	return ctrl.Result{}, err
	//}
	//
	//fmt.Println("   ✅ Status 更新成功!")
	//fmt.Println("   💡 现在 Status.ReadyReplicas 和 Spec.Replicas 一致")
	//fmt.Println("   💡 HPA 可以读取这个值来确认缩放是否生效")
	//fmt.Println()
	//
	//// ============================================================
	//// 🟣 实验 5: 验证 HPA 能否工作
	//// ============================================================
	//fmt.Println("============================================================")
	//fmt.Println("🟣 实验 5: HPA 集成验证")
	//fmt.Println("============================================================")
	//fmt.Println()
	//fmt.Println("说明: HPA 需要集群已安装 metrics-server。")
	//fmt.Println("如果 metrics-server 未安装，HPA 会报 unable to fetch metrics。")
	//fmt.Println()
	//fmt.Println("你可以通过以下命令手动验证 HPA:")
	//fmt.Println("  kubectl get hpa demoapp-hpa -n test -w")
	//fmt.Println("  kubectl get demoapp scale-demo -n test -w")
	//fmt.Println()
	//fmt.Println("HPA 会自动根据 CPU 使用率调整 replicas。")
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
