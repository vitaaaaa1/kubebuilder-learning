/*
 * Demo 3: Handler 的概念 (为什么它是空的？)
 *
 * 目标：理解为什么 EnqueueRequestForObject 初始化时大括号里什么都不用填。
 * 核心好处：通用性 (Reusability)
 */
package main

import "fmt"

// ==========================================================
// 1. 定义接口
// ==========================================================

// Object 接口：只要实现了 GetName()，我就承认你是对象
type Object interface {
	GetName() string
}

// ==========================================================
// 2. 模拟不同的 K8s 资源
// ==========================================================

type ConfigMap struct {
	Name string
}

func (c ConfigMap) GetName() string { return c.Name }

type Pod struct {
	Name string
}

func (p Pod) GetName() string { return p.Name }

// ==========================================================
// 3. 对比两种 Handler 设计
// ==========================================================

// ❌ 坏的设计：紧耦合 (Hardcoded)
// 这个 Handler 写死了只能处理 ConfigMap。
type BadHandler struct {
	// 它心里必须知道自己要处理谁，这就限制死了它的复用。
	ResourceType string
}

func (h *BadHandler) Process(obj interface{}) {
	// 每次处理都要做类型检查，非常麻烦且低效
	if cm, ok := obj.(ConfigMap); ok {
		fmt.Println("[Bad] 成功处理 ConfigMap:", cm.Name)
	} else {
		fmt.Println("[Bad] 错误：我只能处理 ConfigMap，你给我个啥？")
	}
}

// ✅ 好的设计：通用 (Generic / Empty)
// 这个 Handler 是空的。它不关心具体是 Pod 还是 ConfigMap。
// 它只知道：“给我一个能 GetName() 的对象，我把名字塞进队列”。
type GoodHandler struct {
	// 空的！什么都不需要知道。
}

// 它接收的是接口 Object，而不是具体类型。
func (h *GoodHandler) Process(obj Object) {
	fmt.Println("[Good] 通用处理成功:", obj.GetName())
}

// ==========================================================
// 4. 测试
// ==========================================================

func main() {
	fmt.Println("--- 演示：为什么 Handler 应该是空的/通用的？ ---")

	// --- 场景 A: 坏的设计 ---
	fmt.Println("\n--- 坏的设计 (BadHandler) ---")
	bad := &BadHandler{ResourceType: "ConfigMap"}

	cm := ConfigMap{Name: "my-config"}
	bad.Process(cm) // ✅ 能工作

	pod := Pod{Name: "my-pod"}
	bad.Process(pod) // ❌ 完蛋，它不认识 Pod，处理失败！

	// --- 场景 B: 好的设计 ---
	fmt.Println("\n--- 好的设计 (GoodHandler) ---")
	// 只需要创建一个空的 Handler 实例
	good := &GoodHandler{}

	// 它可以处理 ConfigMap
	good.Process(cm) // ✅ 没问题

	// 它也可以处理 Pod
	good.Process(pod) // ✅ 完全没问题！

	// 💡 结论：
	// K8s 中有几百种资源。如果 Handler 绑死了类型，我们需要写几百个 Handler。
	// 现在，我们只需要这 1 个空的 Handler，配合不同的 Watch 就能复用所有资源！
}
