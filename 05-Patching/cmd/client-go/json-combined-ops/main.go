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
	const name = "json-combined-ops-demo"

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
	fmt.Printf("   初始 annotations: %v\n", created.Annotations)
	fmt.Printf("   初始 containers: %s\n\n", getContainerNames(created.Spec.Template.Spec.Containers))

	// =============================================
	// 🔴 演示: 组合操作 — 一次 Patch 多个操作
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔴 JSONPatch 组合操作 — 一次 Patch 多个操作")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("📝 场景: 同时执行 add annotation + replace image + remove annotation")
	patchMulti := []map[string]interface{}{
		// 1. 添加新 annotation
		{"op": "add", "path": "/metadata/annotations/patched-by", "value": "json-patch-multi"},
		// 2. 替换容器 image
		{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.27"},
		// 3. 删除旧 annotation
		{"op": "remove", "path": "/metadata/annotations/version"},
	}
	patchDataMulti, _ := json.Marshal(patchMulti)
	result, err := clientset.AppsV1().Deployments(namespace).Patch(
		ctx, name, types.JSONPatchType, patchDataMulti, metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! annotations/patched-by = %s\n", result.Annotations["patched-by"])
		_, versionExists := result.Annotations["version"]
		if !versionExists {
			fmt.Printf("   ✅ 成功! annotations/version 已被删除\n")
		}
		fmt.Printf("   ✅ 成功! main-app image → %s\n", result.Spec.Template.Spec.Containers[0].Image)
		fmt.Printf("   最终 containers: %v\n\n", getContainerNames(result.Spec.Template.Spec.Containers))
	}

	// =============================================
	// 📊 最终状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 最终状态")
	fmt.Println("============================================================")

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("最终 annotations: %v\n", finalDeploy.Annotations)
	fmt.Printf("最终 containers: %v\n", getContainerNames(finalDeploy.Spec.Template.Spec.Containers))

	fmt.Println("\n🔍 核心结论:")
	fmt.Println("   JSONPatch 可以组合多个操作在一个请求中，原子执行")
	fmt.Println("   → 任意一个操作失败，整个 Patch 都会失败")
	fmt.Println("   → 适合需要保证多个操作一致性的场景")

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
