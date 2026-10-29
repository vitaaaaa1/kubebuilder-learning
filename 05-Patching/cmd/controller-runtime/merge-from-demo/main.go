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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	ctx := context.Background()

	kubeconfig := os.Getenv("HOME") + "/.kube/config"
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}

	// 创建 controller-runtime client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	const namespace = "test"
	const name = "merge-from-demo"

	// =============================================
	// 📋 前置步骤: 创建 ConfigMap
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 创建 ConfigMap")
	fmt.Println("============================================================")
	fmt.Println()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"app.kubernetes.io/version": "1.0",
			},
		},
		Data: map[string]string{
			"config.yaml": "initial-content",
		},
	}

	err = c.Create(ctx, cm)
	if err != nil {
		panic(err)
	}
	fmt.Println("✅ ConfigMap 创建成功")
	fmt.Println()

	// =============================================
	// 🧠 实验: MergeFrom diff 计算验证
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🧠 实验: MergeFrom diff 计算验证")
	fmt.Println("============================================================")
	fmt.Println()

	// 1. Get 当前资源
	var existingCM corev1.ConfigMap
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &existingCM)
	if err != nil {
		panic(err)
	}

	fmt.Printf("📊 当前 ConfigMap:\n")
	fmt.Printf("   annotations: %v\n", existingCM.Annotations)
	fmt.Printf("   data:        %v\n\n", existingCM.Data)

	// 2. 保存原始副本（关键步骤）
	original := existingCM.DeepCopy()
	fmt.Println("✅ original = existingCM.DeepCopy() 保存了原始版本")
	fmt.Println()

	// 3. 修改字段（模拟你只想改的部分）
	existingCM.Annotations["my-operator/last-updated"] = "2026-10-27"
	existingCM.Data["new-key"] = "new-value"

	fmt.Println("📝 修改了 2 个字段:")
	fmt.Println("   - annotations[my-operator/last-updated] = 2026-10-27")
	fmt.Println("   - data[new-key] = new-value")
	fmt.Println()

	// 4. 计算 diff（手动演示 MergeFrom 内部逻辑）
	originalJSON, _ := json.Marshal(original)
	modifiedJSON, _ := json.Marshal(&existingCM)
	// 使用 json-patch 库计算 MergePatch
	// （这里简化演示，实际 MergeFrom 用的是 evanphx/json-patch/v5）
	fmt.Println("🔍 MergeFrom 内部计算过程:")
	fmt.Printf("   originalJSON  = %d bytes\n", len(originalJSON))
	fmt.Printf("   modifiedJSON  = %d bytes\n", len(modifiedJSON))
	fmt.Println("   diff = CreateMergePatch(originalJSON, modifiedJSON)")
	fmt.Println()

	// 5. 发送 Patch
	fmt.Println("✍️ 发送 Patch（只发送 diff）...")
	err = c.Patch(ctx, &existingCM, client.MergeFrom(original))
	if err != nil {
		fmt.Printf("   ❌ Patch 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ Patch 成功!")
	fmt.Println()

	// 6. 验证最终状态
	fmt.Println("📊 验证最终状态:")
	var finalCM corev1.ConfigMap
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &finalCM)
	if err != nil {
		panic(err)
	}

	fmt.Printf("   annotations: %v\n", finalCM.Annotations)
	fmt.Printf("   data:        %v\n\n", finalCM.Data)

	// =============================================
	// 📊 对比：如果用 Update 会怎样？
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 对比：如果用 Update 会怎样？")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("   Update 发送的 HTTP 请求:")
	fmt.Println("   PUT /api/v1/namespaces/test/configmaps/merge-from-demo")
	fmt.Println("   Body: {完整对象，包括所有字段}")
	fmt.Println()
	fmt.Println("   Patch 发送的 HTTP 请求:")
	fmt.Println("   PATCH /api/v1/namespaces/test/configmaps/merge-from-demo")
	fmt.Println("   Content-Type: application/merge-patch+json")
	fmt.Println("   Body: {只包含修改的字段}")
	fmt.Println()

	// =============================================
	// 🔑 核心结论
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔑 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. MergeFrom(original) 捕获原始版本作为三路合并的 base")
	fmt.Println("   2. 修改字段后，Patch 自动计算 diff，只发送变更部分")
	fmt.Println("   3. API Server 执行三路合并: Original + Server + Modified")
	fmt.Println("   4. 优势: 不覆盖其他 Controller 管理的字段")
	fmt.Println("   5. 缺陷: 数组字段全量替换（不智能合并）")
	fmt.Println()

	fmt.Println("🧹 清理: kubectl delete cm merge-from-demo -n test")
}
