package main

import "fmt"

// ===== 普通接口：方法签名写死成 any =====

type Store interface {
	Set(v any)
	Get() any // ← 取出来永远是 any
}

type Box struct{ v any }

func (b *Box) Set(v any) { b.v = v }
func (b *Box) Get() any  { return b.v }

func inc(s Store) int {
	return s.Get().(int) + 1 // ← 必须断言，类型错了运行时才炸
}

func main() {
	b := &Box{}
	b.Set("41") // 存了个字符串，编译器不管

	fmt.Println(inc(b))
}
