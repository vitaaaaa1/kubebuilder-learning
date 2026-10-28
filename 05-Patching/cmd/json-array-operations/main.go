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
	const name = "json-array-ops-demo"

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
	fmt.Printf("   初始 containers: %s\n\n", getContainerNames(created.Spec.Template.Spec.Containers))

	// =============================================
	// 🟣 演示 1: `add` 到数组 — 追加容器
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟣 JSONPatch 数组操作 1: add 到数组 — 追加 sidecar")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("📝 场景: 使用 `/-` 语法追加到 containers 数组末尾")
	patchAddContainer := []map[string]interface{}{
		{
			"op":   "add",
			"path": "/spec/containers/-",
			"value": map[string]interface{}{
				"name":  "sidecar-logging",
				"image": "fluentd:latest",
			},
		},
	}
	patchDataAddContainer, _ := json.Marshal(patchAddContainer)
	result, err := clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataAddContainer, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! containers: %v\n\n", getContainerNames(result.Spec.Template.Spec.Containers))
	}

	// =============================================
	// 🟤 演示 2: `replace` 数组元素 — 替换容器 image
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟤 JSONPatch 数组操作 2: replace 数组元素 — 替换 main-app image")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("📝 场景: 使用 /spec/containers/0/image 替换第一个容器的 image")
	patchReplaceImage := []map[string]interface{}{
		{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.26"},
	}
	patchDataReplaceImage, _ := json.Marshal(patchReplaceImage)
	result, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataReplaceImage, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! main-app image: nginx:1.25 → nginx:1.26\n")
		fmt.Printf("   最终 containers: %v\n\n", getContainerNames(result.Spec.Template.Spec.Containers))
	}

	// =============================================
	// ⚫ 演示 3: `remove` 数组元素 — 删除 sidecar
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("⚫ JSONPatch 数组操作 3: remove 数组元素 — 删除 sidecar-logging")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("📝 场景: 使用 /spec/containers/1 删除第二个容器（sidecar-logging）")
	patchRemoveContainer := []map[string]interface{}{
		{"op": "remove", "path": "/spec/containers/1"},
	}
	patchDataRemoveContainer, _ := json.Marshal(patchRemoveContainer)
	result, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataRemoveContainer, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! sidecar-logging 已被删除\n")
		fmt.Printf("   最终 containers: %v\n\n", getContainerNames(result.Spec.Template.Spec.Containers))
	}

	// =============================================
	// 📊 最终状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 最终状态")
	fmt.Println("============================================================")

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("最终 containers: %v\n", getContainerNames(finalDeploy.Spec.Template.Spec.Containers))

	fmt.Println("\n🔍 核心结论:")
	fmt.Println("   JSONPatch 可以按索引精确操作数组元素")
	fmt.Println("   → add 到 /- 追加到数组末尾")
	fmt.Println("   → replace /0/image 替换指定元素")
	fmt.Println("   → remove /1 删除指定元素")
	fmt.Println("   ⚠️ 需要知道确切的数组索引，不如 StrategicMerge 按 name 匹配直观")

	fmt.Println("\n🧹 Cleaned up test Deployment")
}

func getContainerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i, c := range containers {
		names[i] = c.Name
	}
	return names
}

func ptrInt32(i int32) *int32 {
	return &i
}
