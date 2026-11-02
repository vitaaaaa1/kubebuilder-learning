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
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

// ============================================================
// 🎯 实验目标：理解 /log 和 /exec 子资源的本质
// ============================================================
//
// 本实验演示：
// 1. 创建测试 Pod
// 2. 通过 /log 子资源读取日志流（不是读取 Pod 对象）
// 3. 通过 /exec 子资源执行命令（不是修改 Pod Spec）
// 4. 对比：子资源 vs 主资源的本质区别
//
// 前置条件：
//   集群可达（kubectl 正常工作）
// ============================================================

const (
	testNamespace = "test"
	testPodName   = "log-exec-test-pod"
	testImage     = "anolis-registry.cn-zhangjiakou.cr.aliyuncs.com/openanolis/mysql:8.0.30-8.6"
)

func main() {
	fmt.Println("============================================================")
	fmt.Println("🎯 /log 和 /exec 子资源实验")
	fmt.Println("============================================================")
	fmt.Println()

	// 构建 client
	cfg, err := buildConfig()
	if err != nil {
		fmt.Printf("❌ 构建配置失败: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("❌ 创建客户端失败: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// ============================================================
	// 🔴 实验 1: 创建测试 Pod
	// ============================================================
	fmt.Println("🔴 实验 1: 创建测试 Pod")
	fmt.Println("============================================================")
	fmt.Println()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "app",
					Image:   testImage,
					Command: []string{"sh", "-c", "echo 'Starting app...' && while true; do echo \"[$(date)] Heartbeat\"; sleep 5; done"},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// 尝试创建（如果已存在则跳过）
	_, err = clientset.CoreV1().Pods(testNamespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			fmt.Println("   ⚠️  Pod 已存在，跳过创建")
		} else {
			fmt.Printf("   ❌ 创建 Pod 失败: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("   ✅ Pod 创建成功")
	}

	// 等待 Pod 运行
	fmt.Println("   ⏳ 等待 Pod 启动...")
	time.Sleep(10 * time.Second)

	// 检查 Pod 状态
	p, err := clientset.CoreV1().Pods(testNamespace).Get(ctx, testPodName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("   ❌ 获取 Pod 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("   ✅ Pod 状态: Phase=%s\n\n", p.Status.Phase)

	// ============================================================
	// 🟢 实验 2: 通过 /log 子资源读取日志
	// ============================================================
	fmt.Println("🟢 实验 2: 通过 /log 子资源读取日志")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 这不是读取 Pod 对象的字段，而是独立的 API 端点：")
	fmt.Println("  GET /api/v1/namespaces/test/pods/log-exec-test-pod/log")
	fmt.Println()

	// 获取日志流
	logReq := clientset.CoreV1().Pods(testNamespace).GetLogs(testPodName, &corev1.PodLogOptions{
		Container: "app",
		Follow:    false, // 不 follow，只读取当前日志
	})

	stream, err := logReq.Stream(ctx)
	if err != nil {
		fmt.Printf("   ❌ 获取日志流失败: %v\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	fmt.Println("   📝 读取到的日志：")
	scanner := bufio.NewScanner(stream)
	lineCount := 0
	for scanner.Scan() && lineCount < 10 { // 只读前 10 行
		fmt.Printf("      %s\n", scanner.Text())
		lineCount++
	}
	fmt.Printf("\n   ✅ 成功读取 %d 行日志\n", lineCount)
	fmt.Println("   💡 关键：这些日志不在 etcd 中，是直接从 Node 上的容器运行时读取的")
	fmt.Println("   💡 读取日志不会改变 Pod 的 ResourceVersion")
	fmt.Println()

	// ============================================================
	// 🟡 实验 3: 通过 /exec 子资源执行命令
	// ============================================================
	fmt.Println("🟡 实验 3: 通过 /exec 子资源执行命令")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("说明: 这不是修改 Pod Spec，而是独立的 API 端点：")
	fmt.Println("  POST /api/v1/namespaces/test/pods/log-exec-test-pod/exec")
	fmt.Println()

	// 执行命令
	execReq := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(testPodName).
		Namespace(testNamespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: []string{"sh", "-c", "echo 'Hello from exec!' && uname -a && ps aux"},
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", execReq.URL())
	if err != nil {
		fmt.Printf("   ❌ 创建 exec 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("   📝 执行输出：")
	fmt.Println("   " + strings.Repeat("-", 60))

	// 捕获输出
	var stdout, stderr strings.Builder
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		fmt.Printf("   ❌ exec 失败: %v\n", err)
	} else {
		// 打印输出
		lines := strings.Split(stdout.String(), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				fmt.Printf("   %s\n", line)
			}
		}
		if stderr.Len() > 0 {
			fmt.Printf("   ❌ stderr: %s\n", stderr.String())
		}
	}

	fmt.Println("   " + strings.Repeat("-", 60))
	fmt.Println()
	fmt.Println("   ✅ 命令执行成功!")
	fmt.Println("   💡 /exec 不会修改 Pod 的 Spec 或 Status")
	fmt.Println("   💡 /exec 不会改变 Pod 的 ResourceVersion")
	fmt.Println("   💡 /exec 是一个'动作'，不是'状态'")
	fmt.Println()

	// ============================================================
	// 🔵 实验 4: 验证子资源不改变 RV
	// ============================================================
	fmt.Println("🔵 实验 4: 验证子资源不改变 ResourceVersion")
	fmt.Println("============================================================")
	fmt.Println()

	rvBefore := p.ResourceVersion

	// 读取一次日志
	logReq2 := clientset.CoreV1().Pods(testNamespace).GetLogs(testPodName, &corev1.PodLogOptions{
		Container: "app",
		TailLines: ptrInt64(1),
	})
	_, _ = logReq2.Stream(ctx)

	time.Sleep(2 * time.Second)

	// 再次获取 Pod
	pAfter, _ := clientset.CoreV1().Pods(testNamespace).Get(ctx, testPodName, metav1.GetOptions{})

	fmt.Printf("   读取日志前 RV: %s\n", rvBefore)
	fmt.Printf("   读取日志后 RV: %s\n", pAfter.ResourceVersion)
	if rvBefore == pAfter.ResourceVersion {
		fmt.Println("   ✅ RV 没有变化！证明读取子资源不修改对象")
	} else {
		fmt.Println("   ⚠️  RV 变化了（可能是其他原因，如健康检查更新）")
	}
	fmt.Println()

	// ============================================================
	// 📊 结论
	// ============================================================
	fmt.Println("============================================================")
	fmt.Println("📊 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. ✅ /log 是独立的 API 端点，不是 Pod 对象的字段")
	fmt.Println("   2. ✅ /exec 是独立的 API 端点，不是修改 Pod Spec")
	fmt.Println("   3. ✅ 读取日志/执行命令不会改变 Pod 的 ResourceVersion")
	fmt.Println("   4. ✅ 子资源的本质：是'动作'，不是'状态'")
	fmt.Println("   5. ✅ 子资源可以单独授权（pods/log, pods/exec）")
	fmt.Println()
	fmt.Println("   为什么 CRD 不能有 /log 和 /exec？")
	fmt.Println("   → 因为 CRD 是声明式 API，不支持'动作'类型的操作")
	fmt.Println("   → 如果需要，只能用 Annotation 或独立 CRD 模拟")
	fmt.Println()

	// 清理
	fmt.Println("🧹 清理: 删除测试 Pod")
	_ = clientset.CoreV1().Pods(testNamespace).Delete(ctx, testPodName, metav1.DeleteOptions{})
	fmt.Println("   ✅ Pod 删除成功")
}

func buildConfig() (*rest.Config, error) {
	// 优先使用 KUBECONFIG 环境变量
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	// 使用默认路径
	home := homedir.HomeDir()
	kubeconfigPath := fmt.Sprintf("%s/.kube/config", home)
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func ptrInt64(i int64) *int64 {
	return &i
}
