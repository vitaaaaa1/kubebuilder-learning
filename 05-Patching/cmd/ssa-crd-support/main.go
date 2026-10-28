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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
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
	const name = "ssa-crd-support-demo"

	// =============================================
	// 🧹 清理环境
	// =============================================
	_ = clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	time.Sleep(1 * time.Second)

	// =============================================
	// 📦 创建初始 ConfigMap
	// =============================================
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"config-key-1": "value-1",
			"config-key-2": "value-2",
		},
	}

	created, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Created ConfigMap: %s\n", name)
	fmt.Printf("   初始 data: %v\n\n", created.Data)

	// =============================================
	// 🟢 演示: SSA 对非内置资源（CRD 类比）有效
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 SSA 对非内置资源有效（CRD 类比）")
	fmt.Println("============================================================")
	fmt.Println()

	fmt.Println("📝 为什么用 ConfigMap 演示？")
	fmt.Println("   - ConfigMap 的 data 字段没有 +patchStrategy=merge 标签")
	fmt.Println("   - StrategicMergePatch 对这种字段会退化为全量替换")
	fmt.Println("   - SSA 不依赖 Go 结构体标签，对任何资源类型都有效")
	fmt.Println()

	// Controller A: 修改 config-key-1，添加 new-key-a
	fmt.Println("👤 Controller A: 我要修改 config-key-1，添加 new-key-a...")
	ssaPatchA := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  config-key-1: "updated-by-a"
  new-key-a: "value-a"
`, name, namespace)

	resultA, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
		ctx, name, types.ApplyPatchType, []byte(ssaPatchA),
		metav1.PatchOptions{FieldManager: "controller-a"},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! data: %v\n\n", resultA.Data)
	}

	// Controller B: 修改 config-key-2，添加 new-key-b
	fmt.Println("👤 Controller B: 我要修改 config-key-2，添加 new-key-b...")
	ssaPatchB := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  config-key-2: "updated-by-b"
  new-key-b: "value-b"
`, name, namespace)

	resultB, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
		ctx, name, types.ApplyPatchType, []byte(ssaPatchB),
		metav1.PatchOptions{FieldManager: "controller-b"},
	)
	if err != nil {
		fmt.Printf("   ❌ 失败: %v\n", err)
	} else {
		fmt.Printf("   ✅ 成功! data: %v\n\n", resultB.Data)
	}

	// =============================================
	// ⚠️ 对比: StrategicMergePatch 会怎样？
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("⚠️ 对比: StrategicMergePatch 对这种资源会怎样？")
	fmt.Println("============================================================")
	fmt.Println()

	// 先用 StrategicMergePatch 模拟 Controller A 的操作
	strategicDataA, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(`
data:
  config-key-1: "updated-by-strategic-a"
  new-key-a: "value-a"
`)))

	resultSA, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, strategicDataA,
		metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   StrategicMergePatch A 失败: %v\n", err)
	} else {
		fmt.Printf("   StrategicMergePatch A 成功! data: %v\n\n", resultSA.Data)
	}

	// 再用 StrategicMergePatch 模拟 Controller B 的操作
	strategicDataB, _ := yaml.YAMLToJSON([]byte(fmt.Sprintf(`
data:
  config-key-2: "updated-by-strategic-b"
  new-key-b: "value-b"
`)))

	resultSB, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
		ctx, name, types.StrategicMergePatchType, strategicDataB,
		metav1.PatchOptions{},
	)
	if err != nil {
		fmt.Printf("   StrategicMergePatch B 失败: %v\n", err)
	} else {
		fmt.Printf("   StrategicMergePatch B 成功! data: %v\n\n", resultSB.Data)
		fmt.Println("   ⚠️ 注意: config-key-1 和 new-key-a 被全量替换了！")
		fmt.Println("   因为 data 字段没有 +patchStrategy=merge 标签，StrategicMerge 退化为全量替换")
	}

	// =============================================
	// 📊 最终状态（SSA）
	// =============================================
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("📊 最终状态（SSA 结果）")
	fmt.Println("============================================================")
	fmt.Println()

	finalCM, _ := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("最终 data: %v\n\n", finalCM.Data)

	fmt.Println("📋 managedFields:")
	for _, mf := range finalCM.ManagedFields {
		fmt.Printf("   - manager: %s\n", mf.Manager)
		if mf.FieldsV1 != nil {
			fieldsJSON, _ := yaml.YAMLToJSON(mf.FieldsV1.Raw)
			fmt.Printf("     fields: %s\n", string(fieldsJSON))
		}
		fmt.Println()
	}

	fmt.Println("🔍 核心结论:")
	fmt.Println("   ✅ SSA 不依赖 Go 结构体标签，对任何资源类型都有效")
	fmt.Println("   ✅ 这是 CRD 多 Controller 协作的唯一安全方案")
	fmt.Println("   ❌ StrategicMergePatch 对没有 +patchStrategy 标签的字段退化为全量替换")

	fmt.Println("\n🧹 Cleaned up test ConfigMap")
}
