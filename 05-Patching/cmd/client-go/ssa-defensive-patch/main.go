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
	const name = "ssa-defensive-patch-demo"
	const myManager = "my-operator"

	// =============================================
	// 🧹 清理环境
	// =============================================
	_ = clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})

	// =============================================
	// 1️⃣ 建立所有权: 创建 Deployment 并声明我是所有者
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("1️⃣ 建立所有权: 创建 Deployment")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("👤 身份: %s\n", myManager)
	fmt.Println("📝 动作: 创建 Deployment (此时我拥有所有字段)")
	fmt.Println()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(1),
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
		FieldManager: myManager, // <-- 关键：声明身份
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("✅ Deployment 创建成功")
	fmt.Println()

	// =============================================
	// 🔍 2️⃣ 验证逻辑: Get 并检查字段所属权
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔍 2️⃣ 验证逻辑: 检查字段所属权")
	fmt.Println("============================================================")
	fmt.Println()

	existing, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Println("📋 正在检查 replicas 归属...")
	owner := getOwnerOfField(existing, "f:replicas")

	if owner == myManager {
		fmt.Printf("   ✅ 检查通过: replicas 属于 %s\n", owner)
		fmt.Println("   💡 结论: 我可以安全地修改它，不需要 Force")
		fmt.Println()

		// =============================================
		// ✨ 3️⃣ 安全修改: Patch (无 Force)
		// =============================================
		fmt.Println("============================================================")
		fmt.Println("✨ 3️⃣ 安全修改: Patch (无 Force)")
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
			ctx, name, types.ApplyPatchType, []byte(patchData),
			metav1.PatchOptions{FieldManager: myManager}, // <-- 注意：没有 Force
		)
		if err != nil {
			fmt.Printf("   ❌ 失败 (这不应该发生): %v\n", err)
		} else {
			fmt.Println("   ✅ 修改成功! replicas -> 5")
		}
	} else {
		fmt.Printf("   ❌ 检查失败: replicas 属于 %s\n", owner)
		fmt.Println("   💡 结论: 跳过修改，防止冲突")
	}

	fmt.Println()
	fmt.Println("🔍 核心结论:")
	fmt.Println("   只有当你确认自己拥有字段所有权时，Patch 才是 100% 安全的")
	fmt.Println("   在这种情况下，Force 选项是多余的，甚至是危险的（会覆盖别人的更改）")
}

// getOwnerOfField 检查特定字段属于哪个 Manager
// 注意：这里使用简单的字符串匹配作为演示，生产环境应解析 JSON
func getOwnerOfField(deploy *appsv1.Deployment, fieldName string) string {
	for _, mf := range deploy.ManagedFields {
		if mf.FieldsV1 != nil {
			if strings.Contains(string(mf.FieldsV1.Raw), fieldName) {
				return mf.Manager
			}
		}
	}
	return "Unknown (Unowned)"
}

func ptrInt32(i int32) *int32 {
	return &i
}
