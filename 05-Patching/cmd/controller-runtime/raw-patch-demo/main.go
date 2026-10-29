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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// jsonPatchOperation 定义 JSON Patch 操作
type jsonPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func main() {
	ctx := context.Background()

	kubeconfig := os.Getenv("HOME") + "/.kube/config"
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	const namespace = "test"
	const name = "raw-patch-demo"

	// =============================================
	// 📋 前置步骤: 创建 Deployment
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 创建 Deployment")
	fmt.Println("============================================================")
	fmt.Println()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": name, "remove-me": "true"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			},
		},
	}

	_ = c.Create(ctx, deploy)
	fmt.Println("✅ Deployment 创建成功")
	fmt.Println()

	// =============================================
	// 🧠 实验: 使用 RawPatch 执行复杂操作
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🧠 实验: 使用 RawPatch 执行精确操作")
	fmt.Println("============================================================")
	fmt.Println()

	// 构造 JSON Patch 数据
	patchOps := []jsonPatchOperation{
		// 1. 精确删除一个 Label
		{Op: "remove", Path: "/metadata/labels/remove-me"},
		// 2. 添加一个新的 Label
		{Op: "add", Path: "/metadata/labels/new-label", Value: "injected"},
		// 3. 修改 Replicas
		{Op: "replace", Path: "/spec/replicas", Value: 3},
	}

	patchData, err := json.Marshal(patchOps)
	if err != nil {
		panic(err)
	}

	fmt.Printf("📝 发送 Patch: %s\n\n", string(patchData))

	var d appsv1.Deployment
	d.Name = name
	d.Namespace = namespace

	// 发送 RawPatch
	err = c.Patch(ctx, &d, client.RawPatch(types.JSONPatchType, patchData))
	if err != nil {
		fmt.Printf("   ❌ Patch 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ Patch 成功!")
	fmt.Println()

	// =============================================
	// 📊 验证结果
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 验证最终状态")
	fmt.Println("============================================================")
	fmt.Println()

	var finalD appsv1.Deployment
	c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &finalD)

	fmt.Printf("   Replicas: %d (已改为 3)\n", *finalD.Spec.Replicas)
	fmt.Printf("   Labels:   %v\n", finalD.Labels)
	fmt.Println()

	fmt.Println("🧹 清理: kubectl delete deploy raw-patch-demo -n test")
}

func ptrInt32(i int32) *int32 {
	return &i
}
