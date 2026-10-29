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
	const name = "ssa-safe-injection-demo"
	const myManager = "my-operator"

	// =============================================
	// 📋 前置步骤: 确保 Deployment 已存在
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 确保 Deployment 已存在")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("请先手动创建 Deployment:")
	fmt.Printf("  kubectl create deployment %s --image=nginx:1.25 -n %s\n", name, namespace)
	fmt.Println("创建完成后，按回车继续...")
	fmt.Scanln()

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

	fmt.Printf("✅ Deployment 已存在\n\n")
	fmt.Println("📋 现有 managedFields（部分）:")
	for _, mf := range existing.ManagedFields {
		fmt.Printf("   - manager: %s\n", mf.Manager)
		if mf.FieldsV1 != nil && strings.Contains(string(mf.FieldsV1.Raw), "containers") {
			fmt.Printf("     → 拥有 containers 数组\n")
		}
	}
	fmt.Println()

	// =============================================
	// ✅ 生产实践: 注入 Sidecar / Annotation（不抢字段）
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("✅ 生产实践: 注入 Sidecar 和 Annotation（安全协作）")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Printf("👤 我的身份: fieldManager = %q\n", myManager)
	fmt.Println("📝 我要做的事:")
	fmt.Println("   1. 添加 annotation: injected-by: my-operator")
	fmt.Println("   2. 注入 sidecar 容器: sidecar-logger")
	fmt.Println("   ⚠️ 注意: 我不碰 replicas，不抢别人的字段")
	fmt.Println()

	patchInject := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  annotations:
    injected-by: my-operator
spec:
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        # 我必须声明完整的容器列表（包含原有的 + 新的）
        - name: app
          image: nginx:1.25
        - name: sidecar-logger
          image: fluentd:latest
`, name, namespace, name, name)

	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.ApplyPatchType, []byte(patchInject),
		metav1.PatchOptions{FieldManager: myManager}, // ← 没有 Force！
	)
	if err != nil {
		fmt.Printf("   ❌ 失败（不应该发生）: %v\n", err)
	} else {
		fmt.Println("   ✅ 成功! 注入了 sidecar 和 annotation")
		fmt.Println()
	}

	// =============================================
	// 📊 验证: 字段所有权分布
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证: 字段所有权分布")
	fmt.Println("============================================================")
	fmt.Println()

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})

	fmt.Println("📋 更新后的 managedFields:")
	for _, mf := range finalDeploy.ManagedFields {
		raw := string(mf.FieldsV1.Raw)
		fmt.Printf("   - manager: %s\n", mf.Manager)

		if strings.Contains(raw, "replicas") {
			fmt.Printf("     → 拥有 replicas\n")
		}
		if strings.Contains(raw, "injected-by") {
			fmt.Printf("     → 拥有 annotations/injected-by\n")
		}
		if strings.Contains(raw, "sidecar-logger") {
			fmt.Printf("     → 拥有 containers/sidecar-logger\n")
		}
		if strings.Contains(raw, "\"app\"") {
			fmt.Printf("     → 拥有 containers/app\n")
		}
	}

	fmt.Println()
	fmt.Println("🔍 核心结论:")
	fmt.Println("   1. Operator 注入新字段时，不需要 Force")
	fmt.Println("   2. kubectl 依然拥有 replicas（我没碰它）")
	fmt.Println("   3. 我拥有了 sidecar-logger 和 injected-by 的所有权")
	fmt.Println("   4. 这就是生产环境中多组件安全协作的正确姿势")

	fmt.Println("\n🧹 清理: kubectl delete deploy ssa-safe-injection-demo -n test")
}
