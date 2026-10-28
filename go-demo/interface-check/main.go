package main

import "fmt"

// 1. 定义接口
type Speaker interface {
	Speak() string
}

// 2. 定义结构体
type Dog struct{}

// 3. 实现接口方法
func (d *Dog) Speak() string {
	return "汪汪!"
}

// 4. 🔥 核心验证：告诉编译器 "Dog 必须实现 Speaker"
// 因为 Dog 有 Speak()，所以这行代码编译通过，什么也不发生。
var _ Speaker = (*Dog)(nil)

/*
 (*Dog)(nil)
    这是一个类型转换（Type Conversion）。
     * nil：代表一个空的地址。
     * (*Dog)：代表“指向 Dog 的指针”这个类型。
     * 含义：把 nil 强制转换成 *Dog 类型的指针。
     * 目的：我们不需要真的创建一个 Dog{} 实例，我们只需要拿到 *Dog 这个类型让编译器检查。

  var _ Speaker = ...
    这是一个变量声明（Variable Declaration）。
     * var：声明变量。
     * _：空标识符（Blank Identifier）。意思是“这个变量我赋值后就不用了，丢掉”。
     * Speaker：变量的类型。
     * 含义：声明一个类型为 Speaker 的变量，并把右边的值赋给它。
     * 作用：编译器在赋值时会进行类型检查——“右边的类型（*Dog）能赋值给左边的接口（Speaker）吗？”如果能，说明接口实现了；如果不能，直接报错。
*/

func main() {
	fmt.Println("✅ 编译通过：Dog 实现了 Speaker 接口")
}
