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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// ============================================================
// 🎯 目标：验证 Webhook 是否成功拦截了非法的 /status 写入
// ============================================================
//
// 本工具使用 kubeconfig 连接到集群，模拟一个“恶意”的 Patch 请求：
//   kubectl patch scalableapp my-scalable-app --subresource=status \
//     -p '{"status":{"phase":"HACKED"}}'
//
// 预期结果：
//   - 如果 Webhook 正常运行：请求被拒绝（Error 包含 "admission webhook"）。
//   - 如果 Webhook 未运行：请求成功（非法值写入）。
//
// 前置条件：
//   1. 集群中已部署 ScalableApp CRD 和 ValidatingWebhookConfiguration
//   2. 集群中存在名为 my-scalable-app 的资源
// ============================================================

func main() {
	fmt.Println("============================================================")
	fmt.Println("🛡️  Subresource Webhook 集成验证工具")
	fmt.Println("============================================================")
	fmt.Println()

	// 1. 加载 Kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		// Fallback to default kubeconfig path
		home := homedir.HomeDir()
		config, err = clientcmd.BuildConfigFromFlags("", home+"/.kube/config")
		if err != nil {
			fmt.Printf("❌ 加载 kubeconfig 失败: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("✅ 成功加载 kubeconfig")

	// 2. 创建 Dynamic Client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("❌ 创建客户端失败: %v\n", err)
		os.Exit(1)
	}

	// 3. 定义目标资源 GVR
	// Group: demo.vitaaaaa.icu, Version: v1alpha1, Resource: scalableapps
	gvr := schema.GroupVersionResource{
		Group:    "demo.vitaaaaa.icu",
		Version:  "v1alpha1",
		Resource: "scalableapps",
	}

	namespace := "test"
	name := "my-scalable-app" // 默认测试资源名
	ctx := context.Background()

	// 4. 检查资源是否存在
	fmt.Printf("🔍 检查资源 %s/%s 是否存在...\n", namespace, name)
	_, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("❌ 获取资源失败: %v\n", err)
		fmt.Println("💡 请先创建测试资源：kubectl apply -f cmd/scale-with-deployment/crd/sample.yaml")
		os.Exit(1)
	}
	fmt.Println("✅ 资源存在")

	// 5. 尝试发送恶意 Patch（模拟攻击）
	fmt.Println()
	fmt.Println("⚔️  尝试发送非法 Patch: status.phase = \"HACKED\"")
	fmt.Println("   (如果 Webhook 正常，这应该被拒绝)")
	fmt.Println()

	// 构造 Patch 数据
	patchData := []byte(`{"status":{"phase":"HACKED","message":"I am an attacker"}}`)

	// 执行 Patch，指定子资源为 "status"
	_, err = dynamicClient.Resource(gvr).Namespace(namespace).Patch(
		ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{}, "status",
	)

	// 6. 分析结果
	if err != nil {
		if strings.Contains(err.Error(), "admission webhook") {
			fmt.Println("============================================================")
			fmt.Println("🎉 验证成功！Webhook 拦截了非法请求")
			fmt.Println("============================================================")
			fmt.Printf("   错误信息: %v\n", err)
			fmt.Println("   结论: 你的 Webhook Server 正在正常工作，非法写入被拒绝。")
		} else {
			fmt.Println("============================================================")
			fmt.Println("⚠️  请求失败，但原因不明")
			fmt.Println("============================================================")
			fmt.Printf("   错误信息: %v\n", err)
			fmt.Println("   结论: 可能是 RBAC 权限不足或其他网络问题。")
		}
	} else {
		fmt.Println("============================================================")
		fmt.Println("❌ 验证失败！请求被接受了")
		fmt.Println("============================================================")
		fmt.Println("   结论: Webhook 没有生效（未部署或配置错误）。")
		fmt.Println("   非法值 \"HACKED\" 可能已经被写入 etcd。")
		fmt.Println("   请检查 ValidatingWebhookConfiguration 和 Webhook Server 状态。")
	}
}
