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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	const name = "status-subresource-demo"

	// =============================================
	// 📋 前置步骤: 创建一个 ConfigMap
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 创建 ConfigMap")
	fmt.Println("============================================================")
	fmt.Println()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"key1": "value1",
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}
	fmt.Println("✅ ConfigMap 创建成功")
	fmt.Println()

	// =============================================
	// 🔴 实验 1: 使用普通 Update 更新 status（不会成功）
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔴 实验 1: 使用普通 Update 更新 Data")
	fmt.Println("============================================================")
	fmt.Println()

	// 先 Get
	existing, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	// 修改 Data
	existing.Data["key2"] = "value2"

	// 使用普通 Update
	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		fmt.Printf("   ❌ Update 失败: %v\n", err)
	} else {
		fmt.Println("   ✅ Update 成功! Data 已更新")
	}
	fmt.Println()

	// 验证
	cmAfter, _ := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("📊 验证: Data = %v\n\n", cmAfter.Data)

	//// =============================================
	//// 🟢 实验 2: 使用 Patch 只更新特定字段
	//// =============================================
	//fmt.Println("============================================================")
	//fmt.Println("🟢 实验 2: 使用 MergePatch 更新单个字段")
	//fmt.Println("============================================================")
	//fmt.Println()
	//
	//patchData := []byte(`{"data":{"key3":"value3"}}`)
	//_, err = clientset.CoreV1().ConfigMaps(namespace).Patch(
	//	ctx,
	//	name,
	//	types.MergePatchType,
	//	patchData,
	//	metav1.PatchOptions{},
	//)
	//if err != nil {
	//	fmt.Printf("   ❌ Patch 失败: %v\n", err)
	//} else {
	//	fmt.Println("   ✅ Patch 成功! key3 已添加")
	//}
	//fmt.Println()

	// 验证
	cmAfter2, _ := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("📊 验证: Data = %v\n\n", cmAfter2.Data)

	// =============================================
	// 📊 核心结论
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. ConfigMap 没有 /status 子资源，所以 spec(Data) 和 status 是一体的")
	fmt.Println("   2. 普通 Update 会更新整个资源，触发 Watch 事件")
	fmt.Println("   3. Patch 可以只更新部分字段，减少冲突")
	fmt.Println()
	fmt.Println("   对于 CRD（自定义资源）：")
	fmt.Println("   - 如果启用了 // +kubebuilder:subresource:status")
	fmt.Println("   - spec 和 status 会分离，可以使用 r.Status().Update()")
	fmt.Println("   - 这样更新 status 不会影响 spec，也不会触发 Reconcile")
	fmt.Println()

	fmt.Println("🧹 清理: kubectl delete cm status-subresource-demo -n test")
}

// 等待资源状态稳定
func waitStable(clientset *kubernetes.Clientset, namespace, name string, maxRetries int) {
	for i := 0; i < maxRetries; i++ {
		time.Sleep(500 * time.Millisecond)
		_, err := clientset.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err == nil {
			return
		}
	}
}
