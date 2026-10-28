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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	const name = "ssa-takeover-demo"
	const myManager = "my-controller"

	// =============================================
	// 📋 前置步骤: 模拟 ArgoCD 创建资源并管理 replicas
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 模拟 ArgoCD 创建资源")
	fmt.Println("============================================================")
	fmt.Println()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(1), // ArgoCD 管理 replicas=1
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deploy, metav1.CreateOptions{
		FieldManager: "argocd-controller", // 模拟 ArgoCD
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("✅ ArgoCD 创建了 Deployment，replicas=1，replicas 属于 argocd-controller")
	fmt.Println()

	// =============================================
	// 🚨 业务需求: Controller 必须修改 replicas
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🚨 业务需求: 我必须修改 replicas！")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("📝 场景: 我的 Controller 发现需要紧急扩容到 5 个副本。")
	fmt.Println("   但是 replicas 字段被 ArgoCD 锁定了。")
	fmt.Println("   我必须使用 ForceOwnership 强制接管。")
	fmt.Println()

	patchData := fmt.Sprintf(`
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

	force := true
	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx,
		name,
		types.ApplyPatchType,
		[]byte(patchData),
		metav1.PatchOptions{
			FieldManager: myManager,
			Force:        &force, // ⚠️ 关键：强制抢夺字段所有权
		},
	)

	if err != nil {
		fmt.Printf("   ❌ 修改失败: %v\n", err)
	} else {
		fmt.Println("   ✅ 修改成功! replicas 变成了 5")
		fmt.Println("   ✅ replicas 字段的所有权已经转移给 my-controller")
		fmt.Println()
	}

	// =============================================
	// 📊 验证: 字段所有权已转移
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证: 字段所有权已转移")
	fmt.Println("============================================================")
	fmt.Println()

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("当前 replicas: %d\n\n", *finalDeploy.Spec.Replicas)

	fmt.Println("📋 replicas 字段归属:")
	for _, mf := range finalDeploy.ManagedFields {
		if mf.FieldsV1 != nil && strings.Contains(string(mf.FieldsV1.Raw), "f:replicas") {
			fmt.Printf("   → replicas 属于: %s\n", mf.Manager)
		}
	}

	fmt.Println()
	fmt.Println("🔍 核心结论:")
	fmt.Println("   1. 使用 ForceOwnership 可以成功修改被锁定的字段")
	fmt.Println("   2. 字段所有权会从原管理者（ArgoCD）转移给你")
	fmt.Println("   3. ⚠️ 警告: 这会导致‘拉锯战’！ArgoCD 下次同步时会尝试改回 1")
	fmt.Println("   4. 解决办法: 必须在 ArgoCD 中配置 ignoreDifferences 忽略 replicas")

	fmt.Println("\n🧹 清理: kubectl delete deploy ssa-takeover-demo -n test")
}

func ptrInt32(i int32) *int32 {
	return &i
}
