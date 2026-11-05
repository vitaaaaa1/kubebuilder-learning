package main

import "fmt"

type ConfigMap struct {
	Name string
	Data int
}

type Secret struct {
	Name string
	Size int
}

// ===== 泛型接口版：方法签名里带 T =====

type Predicate[T any] interface {
	Create(obj T) bool
}

type Funcs[T any] struct {
	CreateFunc func(T) bool
}

func (f Funcs[T]) Create(obj T) bool {
	if f.CreateFunc != nil {
		return f.CreateFunc(obj)
	}
	return true
}

func main() {
	// 同样的需求：只处理有 data 的 ConfigMap
	cmPred := Funcs[ConfigMap]{CreateFunc: func(cm ConfigMap) bool {
		return cm.Data > 0 // ← 直接访问字段，不用断言
	}}

	fmt.Println("ConfigMap:", cmPred.Create(ConfigMap{Name: "cm-a", Data: 1}))
	fmt.Println("ConfigMap:", cmPred.Create(ConfigMap{Name: "cm-b", Data: 0}))

	// 把 Secret 传进去会怎样？
	// cmPred.Create(Secret{Name: "s1", Size: 5})  // ← 编译期就报错
}

// 编译器会给出：
// cannot use Secret{...} (value of struct type Secret) as ConfigMap value in argument to cmPred.Create
