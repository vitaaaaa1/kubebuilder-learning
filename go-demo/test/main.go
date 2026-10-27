package main

import "fmt"

// ---------- 模拟“旧版本”的类型 ----------
// 假设这是 controller-runtime 引入泛型之前的类型
type OldHandler struct{}

// 假设新版本把 OldHandler 改成了别名， 将原有 结构体 升级为了 泛型结构体
// 为了不让客户（调用旧代码的人）失望：你给新引擎贴了一个旧标签（别名），说“这就是原来的那个 OldHandler"。
//type OldHandler = TypedHandler[string]

func (h *OldHandler) Handle(name string) {
	fmt.Println("OldHandler 处理:", name)
}

// ---------- 模拟“新版本”的泛型类型 ----------
// 引入泛型后，底层改成了泛型结构体
type TypedHandler[T any] struct{}

func (h *TypedHandler[T]) Handle(name string) {
	fmt.Println("TypedHandler 处理:", name)
}

// ---------- 关键对比 ----------

// 写法 A：类型别名（有 =）
// EnqueueRequestForObject 和 TypedHandler[string] 是同一个类型
type AliasHandler = TypedHandler[string]

// 写法 B：新类型定义（没有 =）
// NewTypeHandler 是一个全新的类型，和 TypedHandler[string] 不同
type NewTypeHandler TypedHandler[string]

// ---------- 模拟“旧代码” ----------
// 用户在旧版本写的代码，用的是 OldHandler
func oldUserCode(h *OldHandler) {
	h.Handle("旧代码传入的对象")
}

func main() {
	//fmt.Println("========== 1. 别名和原类型是同一个类型 ==========")
	//
	//// 别名可以直接赋值给原类型，反之亦然
	//var a AliasHandler
	//var b TypedHandler[string]
	//a = b // ✅ 编译通过，因为它们是同一个类型
	//b = a // ✅ 编译通过
	//fmt.Println("AliasHandler 和 TypedHandler[string] 可以互相赋值")
	//
	//// 用 %T 打印类型名
	//fmt.Printf("a 的类型: %T\n", a)
	//fmt.Printf("b 的类型: %T\n", b)
	//// 输出都是 main.TypedHandler[string]，说明别名没有创造新类型
	//
	//fmt.Println()
	//fmt.Println("========== 2. 别名可以无缝替换旧类型 ==========")
	//
	//// 旧代码写的是 *OldHandler
	//oldUserCode(&OldHandler{})
	//
	//// 假设新版本把 OldHandler 改成了别名：
	//// type OldHandler = TypedHandler[string]
	//// 那么旧代码 oldUserCode(&OldHandler{}) 依然能编译
	//// 因为 OldHandler 就是 TypedHandler[string]
	//fmt.Println("（把 OldHandler 改成别名后，oldUserCode 的调用不用改）")
	//
	//fmt.Println()
	//fmt.Println("========== 3. 新类型定义不是同一个类型 ==========")
	//
	//var c NewTypeHandler
	//var d TypedHandler[string]
	//// c = d // ❌ 编译错误：cannot use d (variable of type TypedHandler[string]) as NewTypeHandler value
	//// d = c // ❌ 编译错误
	//// 必须显式转换：
	//c = NewTypeHandler(d)       // ✅ 显式转换
	//d = TypedHandler[string](c) // ✅ 显式转换
	//fmt.Println("NewTypeHandler 和 TypedHandler[string] 是不同类型，必须显式转换")
	//
	//fmt.Println()
	fmt.Println("========== 4. 方法集：别名继承，新类型不继承 ==========")

	var a AliasHandler
	var b TypedHandler[string]
	a = b // ✅ 编译通过，因为它们是同一个类型
	b = a // ✅ 编译通过

	// 别名继承了 TypedHandler[string] 的方法
	a.Handle("通过别名调用方法") // ✅

	// 新类型不继承原类型的方法
	var c NewTypeHandler
	// c.Handle("通过新类型调用方法") // ❌ 编译错误：c.Handle undefined
	// 但可以显式转换后调用：
	//TypedHandler[string](c).Handle("显式转换后调用方法")
	/* func (h *TypedHandler[T]) Handle(...)这意味着 Handle 是一个指针方法
	   通过强制类型转换产生的是一个临时值（Temporary Value），它没有固定的内存地址，Go 无法对它取地址，所以无法调用指针方法 */
	(*TypedHandler[string])(&c).Handle("显式转换后调用方法") // ✅ 成功
	fmt.Println("新类型不继承方法，别名继承方法")
}
