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
	const name = "merge-patch-demo-cm"

	// =============================================
	// 🧹 清理环境 & 创建初始 ConfigMap
	// =============================================
	_ = clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	time.Sleep(1 * time.Second)

	baseCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"version": "initial",
			},
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, baseCM, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Created ConfigMap: %s\n\n", name)

	// =============================================
	// 🎯 演示 MergePatchType (RFC 7396)
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔹 MergePatchType (JSON Merge Patch)")
	fmt.Println("============================================================")

	// 📡 HTTP Body (三路合并：Original + Server + Modified)
	mergePatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]string{
				"patched-by": "MergePatchType",
			},
		},
		"data": map[string]string{
			"merge-key": "merge-value",
		},
	}
	mergeData, _ := json.Marshal(mergePatch)
	fmt.Printf("📦 发送 Patch Body:\n%s\n\n", string(mergeData))

	// 🔗 调用: clientset.Patch(ctx, name, types.MergePatchType, data, opts)
	result1, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
		ctx, name, types.MergePatchType, mergeData, metav1.PatchOptions{},
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("✅ 成功! 新 ResourceVersion: %s\n", result1.ResourceVersion)
	printField(result1, "annotations[patched-by]", result1.Annotations["patched-by"])
	printField(result1, "data[merge-key]", result1.Data["merge-key"])
	fmt.Println()

	// =============================================
	// 📊 最终状态验证
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 最终 ConfigMap 状态")
	fmt.Println("============================================================")

	finalCM, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Name: %s\n", finalCM.Name)
	fmt.Printf("ResourceVersion: %s\n", finalCM.ResourceVersion)
	fmt.Printf("Annotations: %v\n", finalCM.Annotations)
	fmt.Printf("Data: %v\n", finalCM.Data)
	fmt.Println()
}

func printField(cm *corev1.ConfigMap, key, value string) {
	fmt.Printf("   🔍 %s = %q\n", key, value)
}
