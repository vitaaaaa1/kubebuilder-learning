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
	"encoding/json"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	ctx := context.Background()

	// =============================================
	// 📡 初始化 clientset
	// =============================================
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
	const name = "ssa-basic-demo"
	const myManager = "my-operator"

	// =============================================
	// 🧹 清理环境
	// =============================================
	_ = clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	time.Sleep(2 * time.Second)

	// =============================================
	// 📦 模拟: Deployment 已由别人创建（如 ArgoCD）
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📦 模拟: Deployment 已由 ArgoCD 创建")
	fmt.Println("============================================================")
	fmt.Println()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"argocd.argoproj.io/tracking-id": "app",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main-app",
							Image: "nginx:1.25",
						},
					},
				},
			},
		},
	}

	// 模拟 ArgoCD 创建，使用 "argocd-controller" 作为 fieldManager
	created, err := clientset.AppsV1().Deployments(namespace).Create(
		ctx, deploy,
		metav1.CreateOptions{FieldManager: "argocd-controller"},
	)
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Deployment 已存在（创建者: argocd-controller）\n")
	fmt.Printf("   初始 replicas: %d\n\n", *created.Spec.Replicas)

	// =============================================
	// 🔍 步骤 1: Get 查看现有字段所有权
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔍 步骤 1: Get 查看现有字段所有权")
	fmt.Println("============================================================")
	fmt.Println()

	existing, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Println("📋 现有 managedFields:")
	for _, mf := range existing.ManagedFields {
		fmt.Printf("   - manager: %s\n", mf.Manager)
		// 用 JSON 显示（FieldsV1 是 JSON 格式）
		if mf.FieldsV1 != nil {
			var fields map[string]interface{}
			if err := json.Unmarshal(mf.FieldsV1.Raw, &fields); err == nil {
				if spec, ok := fields["f:spec"].(map[string]interface{}); ok {
					if replicas, ok := spec["f:replicas"]; ok {
						fmt.Printf("     → 拥有 f:spec.f:replicas\n")
						_ = replicas
					}
				}
			}
		}
	}
	fmt.Println()

	// =============================================
	// ⚡ 步骤 2: Operator 接管字段（使用 SSA + ForceOwnership）
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("⚡ 步骤 2: Operator 接管 replicas 字段")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Printf("👤 我的身份: fieldManager = %q\n", myManager)
	fmt.Println("📝 我要修改: spec.replicas = 3")
	fmt.Println("⚠️ 注意: replicas 当前属于 argocd-controller，需要 ForceOwnership 强制接管")
	fmt.Println()

	ssaPatch := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  annotations:
    managed-by: my-operator
spec:
  replicas: 3
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        - name: main-app
          image: nginx:1.25
`, name, namespace, name, name)

	result, err := clientset.AppsV1().Deployments(namespace).Patch(
		ctx,
		name,
		types.ApplyPatchType,
		[]byte(ssaPatch),
		metav1.PatchOptions{
			FieldManager: myManager,
			Force:        ptrBool(true), // ← 关键：ForceOwnership，强制接管冲突字段
		},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! replicas = %d\n", *result.Spec.Replicas)
		fmt.Printf("   ✅ 成功! 已接管 replicas 字段的所有权\n\n")
	}

	// =============================================
	// 📊 步骤 3: 验证字段所有权变化
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 步骤 3: 验证字段所有权变化")
	fmt.Println("============================================================")
	fmt.Println()

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("最终 replicas: %d\n\n", *finalDeploy.Spec.Replicas)

	fmt.Println("📋 更新后的 managedFields:")
	for _, mf := range finalDeploy.ManagedFields {
		fmt.Printf("   - manager: %s\n", mf.Manager)
		if mf.FieldsV1 != nil {
			var fields map[string]interface{}
			if err := json.Unmarshal(mf.FieldsV1.Raw, &fields); err == nil {
				if spec, ok := fields["f:spec"].(map[string]interface{}); ok {
					if _, ok := spec["f:replicas"]; ok {
						fmt.Printf("     → 拥有 f:spec.f:replicas\n")
					}
				}
				if meta, ok := fields["f:metadata"].(map[string]interface{}); ok {
					if annot, ok := meta["f:annotations"].(map[string]interface{}); ok {
						for k := range annot {
							fmt.Printf("     → 拥有 f:metadata.f:annotations.%s\n", k)
						}
					}
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("🔍 核心结论:")
	fmt.Println("   ✅ 生产环境中，资源通常已由别人创建（如 ArgoCD）")
	fmt.Println("   ✅ Operator 应先用 Get 查看现有 managedFields，了解谁拥有什么")
	fmt.Println("   ✅ 使用 SSA + ForceOwnership 可以安全接管需要的字段")
	fmt.Println("   ✅ ForceOwnership 后，该字段的所有权转移给当前 fieldManager")

	fmt.Println("\n🧹 Cleaned up test Deployment")
}

func ptrInt32(i int32) *int32 {
	return &i
}

func ptrBool(b bool) *bool {
	return &b
}
