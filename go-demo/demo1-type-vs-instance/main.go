/*
 * Demo 1: 类型 (Type) vs 实例 (Instance)
 *
 * 目标：理解为什么不能直接用“类型”去访问字段。
 * 类比：类型是图纸，实例是根据图纸盖出来的房子。
 */
package main

import "fmt"

// Namespace 是一个结构体定义（Type）。
// 它只是一张“图纸”，规定了房子应该有 Name。
type Namespace struct {
	Name string
}

func main() {
	fmt.Println("--- Demo 1: Type vs Instance ---")

	// ❌ 错误写法 (取消注释会报错)：
	// Namespace.Name = "my-ns"
	// 报错信息：Namespace.Name undefined (type Namespace has no field or method Name)
	// 解释：你不能把名字写在图纸上，你得把名字写在具体的房子上。

	// ✅ 正确写法：
	// 1. 实例化 (Instance)：根据图纸 Namespace 造一个具体的对象 ns。
	ns := Namespace{
		Name: "my-ns",
	}

	// 2. 操作实例：现在 ns 在内存中有了实实在在的空间，我们可以读写它的 Name。
	fmt.Println("创建成功！实例名称:", ns.Name)
}
