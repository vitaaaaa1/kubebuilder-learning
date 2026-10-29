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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
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
	_ = corev1.AddToScheme(scheme)

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	const namespace = "test"
	const name = "ssa-apply-demo"

	// =============================================
	// 🟢 实验 1: 使用 Apply 创建资源
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 1: 使用 Apply 创建资源")
	fmt.Println("============================================================")
	fmt.Println()

	// 1. 构造 ApplyConfiguration（纯粹的声明式配置，不是普通 Go struct）
	cmApply := applycorev1.ConfigMap(name, namespace).
		WithData(map[string]string{"config.yaml": "initial-content"}).
		WithLabels(map[string]string{"app": "ssa-test"})

	fmt.Println("📝 发送 Apply 请求...")
	fmt.Printf("   FieldManager: %q\n", "my-operator")
	fmt.Println()

	// 2. 调用 client.Apply()（自动推断为 SSA）
	err = c.Apply(ctx, cmApply, client.FieldOwner("my-operator"))
	if err != nil {
		fmt.Printf("   ❌ Apply 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ 资源已创建！")
	fmt.Println()

	// =============================================
	// 🟡 实验 2: 使用 Apply 更新资源（幂等性验证）
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟡 实验 2: 使用 Apply 更新资源（幂等性验证）")
	fmt.Println("============================================================")
	fmt.Println()

	// 修改配置
	cmApplyUpdated := applycorev1.ConfigMap(name, namespace).
		WithData(map[string]string{
			"config.yaml": "updated-content",
			"new-key":     "new-value",
		}).
		WithLabels(map[string]string{
			"app":        "ssa-test",
			"updated-by": "my-operator",
		})

	fmt.Println("📝 再次发送 Apply 请求（修改 config.yaml 和 labels）...")
	fmt.Println()

	err = c.Apply(ctx, cmApplyUpdated, client.FieldOwner("my-operator"))
	if err != nil {
		fmt.Printf("   ❌ Apply 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ 资源已安全更新！")
	fmt.Println()

	// =============================================
	// 📊 验证最终状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证最终状态")
	fmt.Println("============================================================")
	fmt.Println()

	var finalCM corev1.ConfigMap
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &finalCM)
	if err != nil {
		panic(err)
	}

	fmt.Printf("   config.yaml: %s\n", finalCM.Data["config.yaml"])
	fmt.Printf("   new-key:     %s\n", finalCM.Data["new-key"])
	fmt.Printf("   labels:      %v\n\n", finalCM.Labels)

	// =============================================
	// 🔑 核心结论
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔑 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. ApplyConfiguration 是纯粹的声明式配置（无 RV/UID 等干扰字段）")
	fmt.Println("   2. client.Apply() 自动处理创建和更新（幂等性）")
	fmt.Println("   3. 不需要 DeepCopy，因为每次都是全新构造的意图")
	fmt.Println("   4. 必须设置 FieldOwner，用于记录字段所有权")
	fmt.Println()

	fmt.Println("🧹 清理: kubectl delete cm ssa-apply-demo -n test")
}
