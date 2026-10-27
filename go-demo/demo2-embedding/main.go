/*
 * Demo 2: 字段提升 (Embedding)
 *
 * 目标：理解 K8s 源码中 ns.Name 为什么等同于 ns.ObjectMeta.Name。
 */
package main

import "fmt"

// ObjectMeta 是 K8s 中通用的元数据结构。
type ObjectMeta struct {
	Name      string
	Namespace string
}

// Namespace 使用了【匿名嵌入】（Anonymous Embedding）包含 ObjectMeta。
type Namespace struct {
	// 注意：这里没有写字段名（如 Meta ObjectMeta），而是直接写了类型名。
	// 这就是“字段提升”的关键。
	ObjectMeta
	Spec string
}

func main() {
	fmt.Println("--- Demo 2: Embedding & Field Promotion ---")

	ns := Namespace{
		ObjectMeta: ObjectMeta{Name: "my-ns"},
		Spec:       "Default",
	}

	fmt.Println("🔍 观察重点：虽然 Name 定义在 ObjectMeta 里，但可以直接在 ns 上访问。")

	// 写法 A：完整路径（老老实实找）
	fmt.Println("完整路径名称:", ns.ObjectMeta.Name)

	// 写法 B：提升路径（编译器帮你简化了，等同于 A）
	// 这就是为什么我们在 K8s 代码里可以直接写 ns.Name 的原因。
	fmt.Println("提升后的名称:", ns.Name)
}
