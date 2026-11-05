# generics-demo · 泛型接口解决了什么

## 两个版本做同一件事

**普通接口**（`any/main.go`）：

```go
type Store interface {
	Set(v any)
	Get() any          // 取出来永远是 any
}

func inc(s Store) int {
	return s.Get().(int) + 1   // 必须断言
}
```

**泛型接口**（`generic/main.go`）：

```go
type Store[T any] interface {
	Set(v T)
	Get() T            // 取出来就是 T
}

func inc(s Store[int]) int {
	return s.Get() + 1        // 不用断言
}
```

## 运行结果

```bash
$ go run ./any
panic: interface conversion: interface {} is string, not int
```

```bash
$ go run ./generic
42
```

## 差异在哪

| | 普通接口 | 泛型接口 |
|---|---|---|
| 存错类型 | 编译通过，运行时 panic | **编译期报错** |
| 取值 | `s.Get().(int)` | `s.Get()` |
| 接口定义 | 一份，永远是 `any` | 每种 T 一份 `Store[int]` / `Store[string]` |

把 `b.Set("41")` 写进泛型版本：

```text
./main.go:24:8: cannot use "41" (untyped string constant) as int value in argument to b.Set
```

## 一句话

> 普通接口把类型信息丢在 `any` 里，用的人要自己断言；
> **泛型接口把类型参数带进方法签名，用的人不需要断言，编译器替他检查。**

## 和 controller-runtime 的关系

```go
// 普通接口：obj 永远是 client.Object
Get(ctx, key, obj client.Object) error

// 泛型接口：事件类型跟着 T 变
Create(event.TypedCreateEvent[T]) bool
```

`predicate.TypedPredicate[T]` 就是这里的 `Store[T]`，只是方法从 2 个变成了 4 个。