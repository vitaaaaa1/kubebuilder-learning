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

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	const name = "hpa-status-demo"

	// =============================================
	// 📋 前置步骤: 创建 Deployment (Target) 和 HPA
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📋 前置步骤: 创建 Deployment 和 HPA")
	fmt.Println("============================================================")
	fmt.Println()

	// 1. 创建 Deployment (Target for HPA)
	_, _ = clientset.AppsV1().Deployments(namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-deploy", Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "demo"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"cpu": resource.MustParse("100m")}}}}},
			},
		},
	}, metav1.CreateOptions{})

	fmt.Println("✅ Deployment 创建成功")

	// 2. 创建 HPA
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "demo-deploy",
				APIVersion: "apps/v1",
			},
			MinReplicas: ptrInt32(1),
			MaxReplicas: 5,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptrInt32(50),
						},
					},
				},
			},
		},
	}

	_, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("⚠️ HPA 可能已存在或创建失败: %v\n", err)
	} else {
		fmt.Println("✅ HPA 创建成功")
	}
	fmt.Println()

	time.Sleep(2 * time.Second) // 等待控制器初始化状态

	// =============================================
	// 🔴 实验 1: 观察初始状态 (Status vs Spec)
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🔴 实验 1: 观察初始状态")
	fmt.Println("============================================================")
	fmt.Println()

	existing, err := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("📊 初始状态:\n")
	fmt.Printf("   Spec.MinReplicas: %d\n", *existing.Spec.MinReplicas)
	fmt.Printf("   Status.CurrentReplicas: %d\n", existing.Status.CurrentReplicas)
	fmt.Printf("   Status.Conditions 数量: %d\n\n", len(existing.Status.Conditions))

	// =============================================
	// 🟢 实验 2: 使用 UpdateStatus 更新状态
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: 使用 UpdateStatus 更新 Status")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 通常只有 kube-controller-manager 会更新 Status。")
	fmt.Println("这里我们模拟控制器行为，手动注入一个 Status 字段。")
	fmt.Println()

	// 复制对象以修改 Status
	modified := existing.DeepCopy()

	// 模拟修改 Status (注入一个自定义 Condition 或修改指标)
	// 注意：真实的控制器会计算真实数据，这里为了演示 UpdateStatus API
	modified.Status.CurrentReplicas = 99
	modified.Status.DesiredReplicas = 99

	// 关键：使用 UpdateStatus 而不是 Update
	// 这对应 API 端点: PUT .../horizontalpodautoscalers/{name}/status
	_, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).UpdateStatus(ctx, modified, metav1.UpdateOptions{})
	if err != nil {
		fmt.Printf("   ❌ UpdateStatus 失败: %v\n", err)
	} else {
		fmt.Println("   ✅ UpdateStatus 成功! (Status 被独立更新)")
	}
	fmt.Println()

	// 验证
	final, _ := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	fmt.Printf("📊 验证结果:\n")
	fmt.Printf("   Spec.MinReplicas: %d (未变)\n", *final.Spec.MinReplicas)
	fmt.Printf("   Status.CurrentReplicas: %d (已变为 99)\n\n", final.Status.CurrentReplicas)

	// =============================================
	// 📊 核心结论
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. HPA 拥有独立的 /status 子资源。")
	fmt.Println("   2. UpdateStatus 只更新 Status 部分，不影响 Spec (MinReplicas 等)。")
	fmt.Println("   3. 在 Kubebuilder CRD 中，启用 // +kubebuilder:subresource:status 后，")
	fmt.Println("      你也可以使用 r.Status().Update() 达到同样效果。")
	fmt.Println("   4. 这种分离使得 Operator 可以安全地更新状态，而不干扰用户的 Spec 配置。")
	fmt.Println()

	fmt.Println("🧹 清理: kubectl delete hpa hpa-status-demo -n test")
}

func ptrInt32(i int32) *int32 {
	return &i
}
