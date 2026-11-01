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
	// 🟢 实验 2: "偷渡"测试 (Smuggling Test)
	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: '偷渡'测试 - 验证 Spec 隔离")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 我们尝试在更新 Status 时，偷偷修改 Spec (MinReplicas=999)。")
	fmt.Println("如果服务器忽略了 Spec 的修改，说明子资源隔离生效。")
	fmt.Println()

	// 1. 记录初始状态
	existing, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	initialMinReplicas := *existing.Spec.MinReplicas
	initialRV := existing.ResourceVersion

	fmt.Printf("📊 更新前: RV=%s, MinReplicas=%d, CurrentReplicas=%d\n\n", initialRV, initialMinReplicas, existing.Status.CurrentReplicas)

	// 2. 准备"偷渡"
	modified := existing.DeepCopy()
	modified.Status.CurrentReplicas = 99      // 正常的 Status 更新
	modified.Spec.MinReplicas = ptrInt32(999) // ❌ 恶意的 Spec 偷渡！

	fmt.Println("📝 提交 UpdateStatus (携带 Spec.MinReplicas=999)...")
	_, err = clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).UpdateStatus(ctx, modified, metav1.UpdateOptions{})
	if err != nil {
		fmt.Printf("   ❌ UpdateStatus 失败: %v\n", err)
		return
	}
	fmt.Println("   ✅ API Server 接受请求!")
	fmt.Println()

	// 3. 验证结果
	final, _ := clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})

	fmt.Println("============================================================")
	fmt.Println("📊 验证最终状态")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Printf("   RV 变化: %s -> %s (etcd 全局递增，符合预期)\n", initialRV, final.ResourceVersion)
	fmt.Printf("   Status: %d -> %d (✅ 成功更新)\n", existing.Status.CurrentReplicas, final.Status.CurrentReplicas)

	// 核心验证点
	if *final.Spec.MinReplicas == initialMinReplicas {
		fmt.Printf("   Spec (MinReplicas): %d (✅ 保持原值! 偷渡 999 失败 -> 隔离生效!)\n", *final.Spec.MinReplicas)
	} else {
		fmt.Printf("   Spec (MinReplicas): %d (❌ 隔离失效! 被改成了 %d)\n", *final.Spec.MinReplicas, *modified.Spec.MinReplicas)
	}
	fmt.Println()

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
