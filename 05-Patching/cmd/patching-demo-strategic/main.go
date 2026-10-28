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
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	const name = "strategic-patch-demo"

	// =============================================
	// 🧹 清理环境
	// =============================================
	_ = clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	time.Sleep(2 * time.Second)

	// =============================================
	// 📦 创建初始 Deployment (只有 1 个容器)
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
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
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
	fmt.Printf("   初始 containers: %v\n\n", getContainerNames(created.Spec.Template.Spec.Containers))

	// =============================================
	// 🟢 演示: StrategicMergePatchType 智能合并数组
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 StrategicMergePatchType：智能合并数组")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("🎯 场景: 两个 Controller 同时想给 Deployment 添加不同的 sidecar 容器")
	fmt.Println("   Controller A 想加: sidecar-logging")
	fmt.Println("   Controller B 想加: sidecar-monitoring")
	fmt.Println()

	// 先拿到基准版本
	baseDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("📋 基准 Deployment containers: %v\n\n", getContainerNames(baseDeploy.Spec.Template.Spec.Containers))

	var wg sync.WaitGroup
	wg.Add(2)

	// 🟢 Controller A: 想加 logging sidecar
	go func() {
		defer wg.Done()
		fmt.Println("👤 Controller A: 我要添加 sidecar-logging...")

		// 构造 Patch: 只声明"我要加 sidecar-logging"
		patchA := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{"name": "sidecar-logging", "image": "fluentd:latest"},
						},
					},
				},
			},
		}
		patchDataA, _ := json.Marshal(patchA)

		time.Sleep(1 * time.Second) // 模拟延迟，确保并发

		result, err := clientset.AppsV1().Deployments(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patchDataA, metav1.PatchOptions{},
		)
		if err != nil {
			fmt.Printf("   ❌ Controller A 失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ Controller A 成功! containers: %v\n",
				getContainerNames(result.Spec.Template.Spec.Containers))
		}
	}()

	// 🔵 Controller B: 想加 monitoring sidecar
	go func() {
		defer wg.Done()
		fmt.Println("👤 Controller B: 我要添加 sidecar-monitoring...")

		// 构造 Patch: 只声明"我要加 sidecar-monitoring"
		patchB := map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []map[string]interface{}{
							{"name": "sidecar-monitoring", "image": "prometheus:latest"},
						},
					},
				},
			},
		}
		patchDataB, _ := json.Marshal(patchB)

		time.Sleep(1 * time.Second)

		result, err := clientset.AppsV1().Deployments(namespace).Patch(
			ctx, name, types.StrategicMergePatchType, patchDataB, metav1.PatchOptions{},
		)
		if err != nil {
			fmt.Printf("   ❌ Controller B 失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ Controller B 成功! containers: %v\n",
				getContainerNames(result.Spec.Template.Spec.Containers))
		}
	}()

	wg.Wait()

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("📊 最终结果")
	fmt.Println("============================================================")

	finalDeploy, _ := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	finalContainers := getContainerNames(finalDeploy.Spec.Template.Spec.Containers)

	fmt.Printf("最终 containers: %v\n", finalContainers)
	fmt.Println()

	// 验证是否两个容器都在
	hasMain := contains(finalContainers, "main-app")
	hasLogging := contains(finalContainers, "sidecar-logging")
	hasMonitoring := contains(finalContainers, "sidecar-monitoring")

	if hasMain && hasLogging && hasMonitoring {
		fmt.Println("✅ 完美! 三个容器都在: main-app + sidecar-logging + sidecar-monitoring")
		fmt.Println("   StrategicMergePatchType 智能地按 name 字段合并了数组，没有丢数据!")
	} else {
		fmt.Printf("❌ 有容器丢失! main=%v, logging=%v, monitoring=%v\n", hasMain, hasLogging, hasMonitoring)
	}

	fmt.Println()
	fmt.Println("🔍 原理: StrategicMergePatchType 理解 K8s 的 +patchStrategy=merge 标记，")
	fmt.Println("         知道 containers 数组应该按 name 字段识别元素，所以能智能合并。")

	// 清理
	//_ = clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	fmt.Println("\n🧹 Cleaned up test Deployment")
}

func getContainerNames(containers []corev1.Container) []string {
	names := make([]string, len(containers))
	for i, c := range containers {
		names[i] = c.Name
	}
	return names
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func ptrInt32(i int32) *int32 {
	return &i
}
