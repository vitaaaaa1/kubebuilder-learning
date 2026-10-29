package main

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	// 1. 创建一个 ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-config",
			Namespace: "default",
			Annotations: map[string]string{
				"version": "1.0",
			},
		},
		Data: map[string]string{
			"config.yaml": "initial-content",
		},
	}

	// 2. 调用 DeepCopy() — 打断点在这里
	fmt.Println("📌 断点: 即将调用 cm.DeepCopy()")
	original := cm.DeepCopy() // ← 在这里打断点，Step Into 进入调用链
	fmt.Println("✅ DeepCopy() 完成")

	// 3. 验证：修改 original 不影响 cm
	original.Annotations["new-key"] = "new-value"
	fmt.Printf("   cm.Annotations:      %v\n", cm.Annotations)
	fmt.Printf("   original.Annotations: %v\n", original.Annotations)
}
