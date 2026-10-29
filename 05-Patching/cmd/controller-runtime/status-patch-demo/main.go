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

	appsv1 "k8s.io/api/apps/v1"
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

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}

	const namespace = "test"
	const name = "status-patch-simple-demo"

	// =============================================
	// 📋 前置步骤: 创建 Deployment
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 创建 Deployment")
	fmt.Println("============================================================")
	fmt.Println()

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
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
	// 🟢 实验: 使用 client.Status().Patch() 更新状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验: 使用 client.Status().Patch() 更新状态")
	fmt.Println("============================================================")
	fmt.Println()

	// 1. Get 当前资源
	var d appsv1.Deployment
	err = c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &d)
	if err != nil {
		panic(err)
	}

	fmt.Printf("📊 更新前: Status.ReadyReplicas = %d\n\n", d.Status.ReadyReplicas)

	// 2. 保存原始副本 (必须 DeepCopy)
	original := d.DeepCopy()

	// 3. 修改状态字段 (模拟 Controller 汇报状态)
	d.Status.ReadyReplicas = 5
	d.Status.AvailableReplicas = 5

	fmt.Println("📝 修改了 Status 字段:")
	fmt.Println("   - ReadyReplicas = 5")
	fmt.Println("   - AvailableReplicas = 5")
	fmt.Println()

	// 4. 使用 Status().Patch() + MergeFrom 发送请求
	fmt.Println("✍️ 发送 Status().Patch() 请求...")
	err = c.Status().Patch(ctx, &d, client.MergeFrom(original))
	if err != nil {
		fmt.Printf("   ❌ Patch 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ Status 更新成功!")
	fmt.Println()

	// 5. 验证结果
	var finalD appsv1.Deployment
	c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &finalD)

	fmt.Printf("📊 验证结果: Status.ReadyReplicas = %d\n", finalD.Status.ReadyReplicas)
	fmt.Printf("📊 验证结果: Status.AvailableReplicas = %d\n", finalD.Status.AvailableReplicas)
	fmt.Println()

	// =============================================
	// 🔑 核心结论
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔑 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. 必须使用 DeepCopy() 保存修改前的状态")
	fmt.Println("   2. client.MergeFrom(original) 计算 diff")
	fmt.Println("   3. Status().Patch() 只发送 diff 到 /status 端点")
	fmt.Println("   4. 不会影响 Spec，不会触发 Spec 变化的 Watch")
	fmt.Println()

	fmt.Println("🧹 清理: kubectl delete deploy status-patch-simple-demo -n test")
}

func ptrInt32(i int32) *int32 {
	return &i
}
