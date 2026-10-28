package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
)

func main() {
	ctx := context.Background()

	// 🎯 实验目标：
	// 两个"修改者"同时改同一个 ConfigMap 的不同字段
	// 1. 用 Update() → 第二个修改者会报 409 Conflict
	// 2. Patch 的逻辑将在新的代码文件中演示

	// =============================================
	// 📡 获取 kubeconfig
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
	const name = "patching-test-cm"

	// =============================================
	// 📦 创建测试 ConfigMap
	// =============================================
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"version": "1",
			},
		},
		Data: map[string]string{
			"key": "original",
		},
	}

	// 清理旧的
	_ = clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	time.Sleep(1 * time.Second)

	created, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("✅ Created ConfigMap: %s\n", name)
	fmt.Printf("   ResourceVersion: %s\n", created.ResourceVersion)
	fmt.Println()

	// =============================================
	// 🔴 场景 1: 用 Update() 模拟并发冲突
	// =============================================
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("🔴 场景 1: Update() 全量替换")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// 两个修改者都先 Get 到同一个版本
	baseCM, _ := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})

	fmt.Printf("📋 两个修改者都拿到了 ResourceVersion: %s\n\n", baseCM.ResourceVersion)

	var wg sync.WaitGroup
	wg.Add(2)

	// 🟢 修改者 A: 修改 annotations
	go func() {
		defer wg.Done()
		fmt.Println("👤 修改者 A: 我要修改 annotations (用 Patch)...")

		// 🔑 构造 Patch 数据 (只包含我们要修改的字段)
		patchData, _ := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]string{
					"modified-by": "A",
				},
			},
		})

		count := 0
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

			time.Sleep(3 * time.Second) // 模拟处理延迟
			fmt.Println(count)
			count++ // Eroor Code: 409 会导致进入无限循环

			result, err := clientset.CoreV1().ConfigMaps(namespace).Patch(
				ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{},
			)
			if err != nil {
				fmt.Printf("   ❌ 修改者 A 失败: %v\n", err)
			} else {
				fmt.Printf("   ✅ 修改者 A 成功! 新 ResourceVersion: %s\n", result.ResourceVersion)
			}
			return nil
		})
		if err != nil {
			return
		}
	}()

	// 🔵 修改者 B: 修改 Data
	go func() {
		defer wg.Done()
		fmt.Println("👤 修改者 B: 我要修改 data...")

		obj := baseCM.DeepCopy()
		obj.Data["new-key"] = "from-B"

		time.Sleep(2 * time.Second) // 模拟处理延迟

		result, err := clientset.CoreV1().ConfigMaps(namespace).Update(ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			fmt.Printf("   ❌ 修改者 B 失败: %v\n", err)
			if statusErr, ok := err.(*errors.StatusError); ok {
				fmt.Printf("   📄 API Server 原始返回: %+v\n", statusErr.ErrStatus)
			}
		} else {
			fmt.Printf("   ✅ 修改者 B 成功! 新 ResourceVersion: %s\n", result.ResourceVersion)
		}
	}()

	wg.Wait()

	// =============================================
	// 🟢 场景 2: Patch() 部分更新
	// =============================================
	// 🔜 Patch 逻辑将在新的代码文件中演示
	fmt.Println("🔜 Patch 逻辑将在新的代码文件中演示")
	fmt.Println()
}
