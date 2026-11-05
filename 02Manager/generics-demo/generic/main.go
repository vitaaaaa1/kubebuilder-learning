package main

import "fmt"

// ===== 泛型接口：方法签名跟着 T 变 =====

type Store[T any] interface {
	Set(v T)
	Get() T // ← 取出来就是 T
}

type Box[T any] struct{ v T }

func (b *Box[T]) Set(v T) { b.v = v }
func (b *Box[T]) Get() T  { return b.v }

func inc(s Store[int]) int {
	return s.Get() + 1 // ← 不用断言，编译期就知道是 int
}

func main() {
	b := &Box[int]{}
	b.Set(41)
	// b.Set("41") // ← 编译期就报错：cannot use "41" as int value

	fmt.Println(inc(b)) // 42
}
