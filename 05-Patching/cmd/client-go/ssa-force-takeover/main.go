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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	ctx := context.Background()

	kubeconfig := os.Getenv("HOME") + "/.kube/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	const namespace = "test"
	const name = "ssa-force-takeover-demo"
	const myManager = "my-operator"

	// =============================================
	// 📋 前置步骤: 确保 Deployment 已存在
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 确保 Deployment 已存在")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("请先手动创建 Deployment:")
	fmt.Printf("  kubectl create deployment %s --image=nginx:1.25 -n %s --replicas=1\n", name, namespace)
	fmt.Println("创建完成后，按回车继续...")
	fmt.Scanln()

	// =============================================
	// 🔍 步骤 0: 查看 replicas 字段归属
	// =============================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("🔍 步骤 0: 查看 replicas 字段归属")
	fmt.Println("============================================================")
	fmt.Println()

	existing, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Sprintf("Deployment 不存在，请先创建: %v", err))
	}

	currentReplicas := int32(1)
	if existing.Spec.Replicas != nil {
		currentReplicas = *existing.Spec.Replicas
	}
	fmt.Printf("✅ Deployment 已存在，replicas = %d\n\n", currentReplicas)

	fmt.Println("📋 replicas 字段归属:")
	for _, mf := range existing.ManagedFields {
		if mf.FieldsV1 != nil {
			raw := string(mf.FieldsV1.Raw)
			if strings.Contains(raw, "f:replicas") {
				fmt.Printf("   → replicas 属于: %s\n", mf.Manager)
			}
		}
	}
	fmt.Println()

	// =============================================
	// ❌ 步骤 1: 不用 Force，尝试修改 replicas
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("❌ 步骤 1: 尝试修改 replicas（不用 Force）")
	fmt.Println("============================================================")
	fmt.Println()

	patchReplicas := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 5
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        - name: app
          image: nginx:1.25
`, name, namespace, name, name)

	fmt.Printf("📝 %q 想把 replicas 从 %d 改为 5\n\n", myManager, currentReplicas)

	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.ApplyPatchType, []byte(patchReplicas),
		metav1.PatchOptions{FieldManager: myManager}, // ← 没有 Force
	)
	if err != nil {
		fmt.Printf("   ❌ 冲突报错: %v\n\n", err)
	} else {
		fmt.Println("   ✅ 成功了（说明 replicas 没有明确的所属者）")
	}

	fmt.Println("🔍 结论:")
	fmt.Println("   replicas 字段已经有主了，不用 Force 无法修改")
	fmt.Println()

	// =============================================
	// ✅ 步骤 2: 使用 ForceOwnership 强制接管
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("✅ 步骤 2: 使用 ForceOwnership 强制接管 replicas")
	fmt.Println("============================================================")
	fmt.Println()

	force := true
	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.ApplyPatchType, []byte(patchReplicas),
		metav1.PatchOptions{
			FieldManager: myManager,
			Force:        &force, // ← 关键：强制抢夺
		},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Println("   ✅ 成功! replicas 已从原所属者转移给 my-operator")
		fmt.Println()
	}

	// =============================================
	// 📊 验证: 字段所有权变化
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证: replicas 现在属于谁？")
	fmt.Println("============================================================")
	fmt.Println()

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	finalReplicas := int32(0)
	if finalDeploy.Spec.Replicas != nil {
		finalReplicas = *finalDeploy.Spec.Replicas
	}
	fmt.Printf("当前 replicas: %d\n\n", finalReplicas)

	fmt.Println("📋 replicas 字段归属（更新后）:")
	for _, mf := range finalDeploy.ManagedFields {
		if mf.FieldsV1 != nil {
			raw := string(mf.FieldsV1.Raw)
			if strings.Contains(raw, "f:replicas") {
				fmt.Printf("   → replicas 属于: %s\n", mf.Manager)
			}
		}
	}

	fmt.Println()
	fmt.Println("🔍 核心结论:")
	fmt.Println("   1. 当字段有主时，SSA 默认保护它（Conflict）")
	fmt.Println("   2. 使用 Force 可以强制抢夺所有权")
	fmt.Println("   3. 抢夺后，该字段的所有权转移给你")
	fmt.Println("   4. ⚠️ 生产环境慎用 Force，可能引发与原管理者的拉锯战")

	fmt.Println("\n🧹 清理: kubectl delete deploy ssa-force-takeover-demo -n test")
}
