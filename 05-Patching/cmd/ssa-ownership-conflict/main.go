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
	const name = "ssa-conflict-demo"

	// =============================================
	// 📋 前置步骤: 确保 Deployment 已存在
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 确保 Deployment 已存在")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("请先手动创建 Deployment:")
	fmt.Printf("  kubectl create deployment %s --image=nginx:1.25 -n %s --replicas=1\n", name, namespace)

	// =============================================
	// 🔍 步骤 0: 查看现有字段所有权
	// =============================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("🔍 步骤 0: 查看现有字段所有权")
	fmt.Println("============================================================")
	fmt.Println()

	existing, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Sprintf("Deployment 不存在，请先创建: %v", err))
	}

	fmt.Printf("✅ Deployment 已存在，replicas = %d\n\n", *existing.Spec.Replicas)
	fmt.Println("📋 managedFields（replicas 的归属）:")
	for _, mf := range existing.ManagedFields {
		if mf.FieldsV1 != nil {
			raw := string(mf.FieldsV1.Raw)
			if contains(raw, "replicas") {
				fmt.Printf("   → replicas 属于: %s\n", mf.Manager)
			}
		}
	}
	fmt.Println()

	// =============================================
	// ❌ 步骤 1: Manager-B 尝试修改 replicas（不用 Force）
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("❌ 步骤 1: Manager-B 尝试修改 replicas（不用 Force）")
	fmt.Println("============================================================")
	fmt.Println()

	patchB := fmt.Sprintf(`
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

	fmt.Println("📝 Manager-B 想改 replicas = 5，但不用 Force")
	fmt.Println()

	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.ApplyPatchType, []byte(patchB),
		metav1.PatchOptions{FieldManager: "manager-b"}, // ← 没有 Force
	)
	if err != nil {
		fmt.Printf("   ❌ 冲突报错! %v\n\n", err)
	} else {
		fmt.Println("   ✅ 成功了（不应该发生）")
	}

	fmt.Println("🔍 结论:")
	fmt.Println("   Manager-B 没有用 Force，API Server 拒绝了这次修改")
	fmt.Println("   因为 replicas 字段属于别人，manager-b 不能抢夺")
	fmt.Println()

	// =============================================
	// 📊 验证最终状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证: replicas 现在属于谁？")
	fmt.Println("============================================================")
	fmt.Println()

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("当前 replicas: %d\n\n", *finalDeploy.Spec.Replicas)

	fmt.Println("📋 managedFields 中 replicas 的归属:")
	for _, mf := range finalDeploy.ManagedFields {
		if mf.FieldsV1 != nil {
			raw := string(mf.FieldsV1.Raw)
			if contains(raw, "replicas") {
				fmt.Printf("   → replicas 属于: %s\n", mf.Manager)
			}
		}
	}

	fmt.Println()
	fmt.Println("🔍 核心结论:")
	fmt.Println("   1. SSA 的字段所有权是保护机制，不是锁")
	fmt.Println("   2. 不用 Force → 冲突报错（安全）")
	fmt.Println("   3. 用 Force → 抢夺成功（但会引发拉锯战）")
	fmt.Println("   4. 生产环境应通过配置协调，而不是用 Force 硬抢")

	fmt.Println("\n🧹 清理: kubectl delete deploy ssa-conflict-demo -n test")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
