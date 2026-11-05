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

// ===== 普通接口版：方法签名里只有 any =====

type Predicate interface {
	Create(obj any) bool
}

type Funcs struct {
	CreateFunc func(any) bool
}

func (f Funcs) Create(obj any) bool {
	if f.CreateFunc != nil {
		return f.CreateFunc(obj)
	}
	return true
}

func main() {
	// 想写一个"只处理有 data 的 ConfigMap"的谓词
	cmPred := Funcs{CreateFunc: func(obj any) bool {
		cm, ok := obj.(ConfigMap) // ← 必须断言
		if !ok {
			return false // ← 类型不对就静默丢弃，没有任何提示
		}
		return cm.Data > 0
	}}

	fmt.Println("ConfigMap:", cmPred.Create(ConfigMap{Name: "cm-a", Data: 1}))
	fmt.Println("Secret:   ", cmPred.Create(Secret{Name: "s1", Size: 5}), "← 静默返回 false")

	// 如果不检查 ok，直接断言：
	badPred := Funcs{CreateFunc: func(obj any) bool {
		return obj.(ConfigMap).Data > 0 // ← 这里会 panic
	}}
	fmt.Println("不检查 ok:", badPred.Create(Secret{Name: "s1", Size: 5}))
}
