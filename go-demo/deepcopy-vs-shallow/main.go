package main

import (
	"fmt"
)

// 模拟 K8s 资源结构
type ConfigMap struct {
	Name        string
	Annotations map[string]string
	Data        map[string]string
}

// DeepCopy 方法（模拟 K8s 生成的版本）
func (in *ConfigMap) DeepCopy() *ConfigMap {
	if in == nil {
		return nil
	}
	out := &ConfigMap{
		Name: in.Name,
	}
	if in.Annotations != nil {
		out.Annotations = make(map[string]string)
		for k, v := range in.Annotations {
			out.Annotations[k] = v
		}
	}
	if in.Data != nil {
		out.Data = make(map[string]string)
		for k, v := range in.Data {
			out.Data[k] = v
		}
	}
	return out
}

func main() {
	// 初始资源
	original := &ConfigMap{
		Name: "my-config",
		Annotations: map[string]string{
			"version": "1.0",
		},
		Data: map[string]string{
			"config.yaml": "initial-content",
		},
	}

	fmt.Println("============================================================")
	fmt.Println("🔴 实验 1: 直接赋值（:=）— 浅拷贝")
	fmt.Println("============================================================")
	fmt.Println()

	// 直接赋值：复制 struct 字段，但 map/slice 共享
	shallowCopy := *original // 值复制（不是指针复制）

	fmt.Println("修改前:")
	fmt.Printf("   original.Annotations:  %v\n", original.Annotations)
	fmt.Printf("   shallowCopy.Annotations: %v\n", shallowCopy.Annotations)
	fmt.Println()

	// 修改 shallowCopy 的 map
	shallowCopy.Annotations["new-key"] = "new-value"

	fmt.Println("修改 shallowCopy.Annotations 后:")
	fmt.Printf("   original.Annotations:  %v  ← ❌ 也被改了！\n", original.Annotations)
	fmt.Printf("   shallowCopy.Annotations: %v\n", shallowCopy.Annotations)
	fmt.Println()
	fmt.Println("   原因：map 是引用类型，:= 复制的是指针，不是内容")
	fmt.Println()

	// 验证：struct 字段（Name）是独立的
	shallowCopy.Name = "modified-name"
	fmt.Printf("   original.Name:  %s  ← ✅ 没变\n", original.Name)
	fmt.Printf("   shallowCopy.Name: %s\n", shallowCopy.Name)
	fmt.Println("   struct 字段（Name）是值类型，:= 复制了值，但 map/slice 不是")
	fmt.Println()

	// =============================================
	fmt.Println("============================================================")
	fmt.Println("🟢 实验 2: DeepCopy() — 深拷贝")
	fmt.Println("============================================================")
	fmt.Println()

	deepCopy := original.DeepCopy()

	fmt.Println("修改前:")
	fmt.Printf("   original.Annotations:  %v\n", original.Annotations)
	fmt.Printf("   deepCopy.Annotations: %v\n", deepCopy.Annotations)
	fmt.Println()

	// 修改 deepCopy 的 map
	deepCopy.Annotations["another-key"] = "another-value"

	fmt.Println("修改 deepCopy.Annotations 后:")
	fmt.Printf("   original.Annotations:  %v  ← ✅ 没变！\n", original.Annotations)
	fmt.Printf("   deepCopy.Annotations: %v\n", deepCopy.Annotations)
	fmt.Println()
	fmt.Println("   DeepCopy() 递归复制了所有嵌套数据，包括 map/slice")
	fmt.Println()

	// =============================================
	fmt.Println("============================================================")
	fmt.Println("📊 核心结论")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("   1. := 复制 struct 字段，但 map/slice 仍然共享底层数据")
	fmt.Println("   2. DeepCopy() 递归复制所有嵌套数据，完全独立")
	fmt.Println("   3. K8s 中的 MergeFrom(original) 需要 DeepCopy()")
	fmt.Println("      因为 original 必须保存 Get 时的版本，用于计算 diff")
	fmt.Println("      如果是 :=，修改 obj 后 original 也会被影响 → diff 为空")
	fmt.Println()
}
