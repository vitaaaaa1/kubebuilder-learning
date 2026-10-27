package main

import (
	"fmt"
)

// 泛型结构体（模板）
type Box[T any] struct {
	Value T
}

// 具体类型（泛型实例化）
// Box[string] 是具体类型

// 具体类型的别名
type StringBox = Box[string]

func main() {
	// ✅ 泛型结构体必须填类型参数
	b1 := Box[string]{Value: "hello"}
	b2 := Box[int]{Value: 42}
	fmt.Println(b1.Value, b2.Value)

	// ✅ 别名直接用，不需要填类型参数
	sb := StringBox{Value: "world"}
	fmt.Println(sb.Value)

	// ❌ 下面这行编译错误：StringBox 不是泛型，没有类型参数
	// sb2 := StringBox[int]{Value: 1}
	// 报错：cannot use generic type StringBox without instantiation
	// 或：StringBox is not a generic type
}

//// ① 泛型结构体（模板）
//type TypedEnqueueRequestForObject[object client.Object] struct{}
//
//// ② 泛型实例化（具体类型）
//TypedEnqueueRequestForObject[client.Object]
//
//// ③ 具体类型的别名
//type EnqueueRequestForObject = TypedEnqueueRequestForObject[client.Object]
