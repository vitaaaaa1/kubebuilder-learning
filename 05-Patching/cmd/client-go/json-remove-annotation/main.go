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
	const name = "json-remove-annotation-demo"

	// =============================================
	// 🧹 清理环境
	// =============================================
	_ = clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	time.Sleep(2 * time.Second)

	// =============================================
	// 📦 创建初始 Deployment
	// =============================================
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"version":   "1.0",
				"owned-by":  "json-patch-demo",
				"to-delete": "delete-me",
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

	created, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Created Deployment: %s\n", name)
	fmt.Printf("   初始 annotations: %v\n\n", created.Annotations)

	// =============================================
	// 🟡 演示: `remove` 操作 — 精确删除
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟡 JSONPatch remove 操作 — 精确删除")
	fmt.Println("============================================================")
	fmt.Println()

	// 3.1 成功：删除已存在的 annotation
	fmt.Println("📝 场景 1: 删除 to-delete annotation")
	patchRemove := []map[string]interface{}{
		{"op": "remove", "path": "/metadata/annotations/to-delete"},
	}
	patchDataRemove, _ := json.Marshal(patchRemove)
	result, err := clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataRemove, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		_, exists := result.Annotations["to-delete"]
		if !exists {
			fmt.Printf("   ✅ 成功! to-delete 已被删除\n")
			fmt.Printf("   剩余 annotations: %v\n\n", result.Annotations)
		}
	}

	// 3.2 失败：尝试删除不存在的 annotation
	fmt.Println("📝 场景 2: 尝试删除不存在的 annotation（应该报错）")
	patchRemoveMissing := []map[string]interface{}{
		{"op": "remove", "path": "/metadata/annotations/nonexistent"},
	}
	patchDataRemoveMissing, _ := json.Marshal(patchRemoveMissing)
	_, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataRemoveMissing, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ✅ 预期报错! %v\n\n", err)
	} else {
		fmt.Printf("   ❌ 不应该成功 — JSONPatch remove 要求路径必须存在\n\n")
	}

	// =============================================
	// 📊 最终状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 最终状态")
	fmt.Println("============================================================")

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("最终 annotations: %v\n", finalDeploy.Annotations)

	fmt.Println("\n🔍 核心结论:")
	fmt.Println("   JSONPatch remove 操作比 StrategicMerge 的 $patch: delete 和 MergePatch 的 null 更直观")
	fmt.Println("   → 路径存在时删除，不存在时报错")
	fmt.Println("   → 不需要特殊语法或 null 值")

	fmt.Println("\n🧹 Cleaned up test Deployment")
}

func ptrInt32(i int32) *int32 {
	return &i
}
