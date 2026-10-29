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
	const name = "json-add-map-key-demo"

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
				"version": "1.0",
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
	// 🟢 演示: `add` 操作 — 添加 map key
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 JSONPatch add 操作 — 添加 map key")
	fmt.Println("============================================================")
	fmt.Println()

	// 1.1 成功：添加不存在的 annotation
	fmt.Println("📝 场景 1: 添加一个不存在的 annotation")
	patchAdd := []map[string]interface{}{
		{"op": "add", "path": "/metadata/annotations/owned-by", "value": "json-patch-demo"},
	}
	patchData, _ := json.Marshal(patchAdd)
	result, err := clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchData, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! annotations/owned-by = %s\n\n", result.Annotations["owned-by"])
	}

	// 1.2 ⚠️ RFC 6902: add 对 map key 是 upsert（不是报错）
	fmt.Println("📝 场景 2: 再次 add 同一个 annotation")
	fmt.Println("   ⚠️ 注意: RFC 6902 对 object member 的 add 是 upsert（已存在则替换）")
	patchAddConflict := []map[string]interface{}{
		{"op": "add", "path": "/metadata/annotations/owned-by", "value": "some-other-controller"},
	}
	patchDataConflict, _ := json.Marshal(patchAddConflict)
	result, err = clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataConflict, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! annotations/owned-by = %s（之前是 json-patch-demo）\n", result.Annotations["owned-by"])
		fmt.Println("   ⚠️ JSONPatch add 对 map key 语义是 upsert，不是 'add-only'！\n")
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
	fmt.Println("   JSONPatch add 对 map key（如 annotation keys）的语义是 upsert")
	fmt.Println("   → 不存在则创建，已存在则替换（和 MergePatch 一样）")
	fmt.Println("   → 不是 'add-only'，不会报错")

	fmt.Println("\n🧹 Cleaned up test Deployment")
}

func ptrInt32(i int32) *int32 {
	return &i
}
