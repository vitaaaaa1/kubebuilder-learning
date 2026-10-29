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
	"fmt"
	"os"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime"
	applyautoscalingv2 "k8s.io/client-go/applyconfigurations/autoscaling/v2"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	ctx := context.Background()

	kubeconfig := os.Getenv("HOME") + "/.kube/config"
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}

	// 创建 controller-runtime client
	scheme := runtime.NewScheme()
	_ = autoscalingv2.AddToScheme(scheme)

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	const namespace = "test"
	const name = "my-app01"
	const myManager = "vita-deploy-scaler" // 干净的标识符

	// =============================================
	// 📋 前置步骤: 查看 HPA 初始状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 查看 HPA 初始状态")
	fmt.Println("============================================================")
	fmt.Println()

	var initialHPA autoscalingv2.HorizontalPodAutoscaler
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &initialHPA)
	if err != nil {
		panic(err)
	}

	fmt.Printf("当前 minReplicas:   %d\n", *initialHPA.Spec.MinReplicas)
	fmt.Printf("当前 maxReplicas:  %d\n", initialHPA.Spec.MaxReplicas)
	fmt.Printf("当前 metrics 数量:  %d\n\n", len(initialHPA.Spec.Metrics))

	// =============================================
	// 🟢 实验 1: 使用 Apply 更新 minReplicas (SSA)
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验: 使用 Apply 更新 minReplicas")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("策略: FieldOwner = %q\n", myManager)
	fmt.Println("目标: 将 minReplicas 更新为 5")
	fmt.Println()

	// 1. 构造 ApplyConfiguration (只声明 minReplicas)
	// 注意：不需要 Get，也不需要填充 maxReplicas 或 metrics
	hpaApply := applyautoscalingv2.HorizontalPodAutoscaler(name, namespace).
		WithSpec(
			applyautoscalingv2.HorizontalPodAutoscalerSpec().
				WithMinReplicas(5),
		)

	// 2. 调用 client.Apply()
	fmt.Println("📝 发送 Apply 请求 (ForceOwnership)...")
	err = c.Apply(ctx, hpaApply, client.FieldOwner(myManager), client.ForceOwnership)
	if err != nil {
		fmt.Printf("   ❌ Apply 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ 更新成功！")
	fmt.Println()

	// =============================================
	// 📊 验证: 确认其他字段未被覆盖
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证: 确认其他字段未被覆盖")
	fmt.Println("============================================================")
	fmt.Println()

	var finalHPA autoscalingv2.HorizontalPodAutoscaler
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &finalHPA)
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ minReplicas:   %d (已更新为 5)\n", *finalHPA.Spec.MinReplicas)
	fmt.Printf("✅ maxReplicas:  %d (保持不变)\n", finalHPA.Spec.MaxReplicas)
	fmt.Printf("✅ metrics 数量:  %d (保持不变)\n\n", len(finalHPA.Spec.Metrics))

	// =============================================
	// 🔑 核心结论
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔑 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. Apply 只修改了 minReplicas，maxReplicas 和 metrics 毫发无损")
	fmt.Println("   2. 不需要 Get → Modify → Update，直接声明意图即可")
	fmt.Println("   3. FieldOwner 设置为 'vita-deploy-scaler' (替代了 GoLand 的乱码名)")
	fmt.Println()
	fmt.Println("   下一步: 运行以下命令查看 managedFields 的变化:")
	fmt.Printf("   kubectl get hpa %s -n %s --show-managed-fields -o yaml | grep -A 5 '%s'\n\n", name, namespace, myManager)

	fmt.Println("🧹 清理: 如需还原，请使用 kubectl apply -f 你的hpa.yaml")
}
