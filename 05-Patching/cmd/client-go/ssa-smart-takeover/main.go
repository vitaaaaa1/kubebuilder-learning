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
	const name = "ssa-smart-takeover-demo"
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
	// 🧠 步骤 1: 智能判断 (Smart Decision)
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🧠 步骤 1: 智能判断字段归属")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Printf("👤 我的身份: %q\n", myManager)
	fmt.Println("📝 任务: 把 replicas 改为 5")
	fmt.Println("🔍 检查: 先看一眼 replicas 属于谁...")
	fmt.Println()

	existing, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	owner := getOwner(existing, "f:replicas")
	fmt.Printf("🔍 检查结果: replicas 当前属于 %q\n", owner)

	// 决定是否需要 Force
	useForce := (owner != myManager)
	if useForce {
		fmt.Println("⚠️ 字段不属于我 -> 策略: 使用 ForceOwnership 强制接管")
	} else {
		fmt.Println("✅ 字段属于我 -> 策略: 正常 Patch，不需要 Force")
	}
	fmt.Println()

	// =============================================
	// ✍️ 步骤 2: 执行 Patch
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("✍️ 步骤 2: 执行 Patch")
	fmt.Println("============================================================")
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

	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx,
		name,
		types.ApplyPatchType,
		[]byte(patchData),
		metav1.PatchOptions{
			FieldManager: myManager,
			Force:        ptrBool(useForce), // 动态决定是否 Force
		},
	)

	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		if useForce {
			fmt.Println("   ✅ 成功! 我抢夺了 replicas 的所有权")
		} else {
			fmt.Println("   ✅ 成功! 安全更新了 replicas")
		}
		fmt.Println()
	}

	// =============================================
	// 📊 验证最终状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证最终状态")
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
	fmt.Println("   1. 这种模式最稳健：先看后动")
	fmt.Println("   2. 如果字段属于我，不消耗额外的权限 (不用 Force)")
	fmt.Println("   3. 如果字段属于别人，果断使用 Force 接管")
	fmt.Println("   4. 这样写出来的代码对两种场景都兼容")

	fmt.Println("\n🧹 清理: kubectl delete deploy ssa-smart-takeover-demo -n test")
}

func getOwner(obj *appsv1.Deployment, fieldName string) string {
	for _, mf := range obj.ManagedFields {
		if mf.FieldsV1 != nil {
			if strings.Contains(string(mf.FieldsV1.Raw), fieldName) {
				return mf.Manager
			}
		}
	}
	return "Unknown"
}

func ptrInt32(i int32) *int32 {
	return &i
}

func ptrBool(b bool) *bool {
	return &b
}
