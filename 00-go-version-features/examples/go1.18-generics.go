//go:build go1.18
// +build go1.18

package main

import (
	"fmt"

	"golang.org/x/exp/slices"
)

// ==================== 泛型函数 ====================

// Print 打印切片元素
func Print[T any](s []T) {
	for _, v := range s {
		fmt.Println(v)
	}
}

// Min 返回最小值
func Min[T Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

// Ordered 可比较类型约束
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~string
}

// ==================== 泛型类型 ====================

// Stack 泛型栈
type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(item T) {
	s.items = append(s.items, item)
}

func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	item := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return item, true
}

// ==================== main ====================

func main() {
	fmt.Println("=== Go 1.18 泛型演示 ===")

	// 1. 泛型函数
	fmt.Println("\n1. 泛型函数 Min:")
	fmt.Println("   Min(3, 5) =", Min(3, 5))
	fmt.Println("   Min(\"apple\", \"banana\") =", Min("apple", "banana"))

	// 2. 泛型栈
	fmt.Println("\n2. 泛型 Stack:")
	var intStack Stack[int]
	intStack.Push(1)
	intStack.Push(2)
	intStack.Push(3)
	for v, ok := intStack.Pop(); ok; v, ok = intStack.Pop() {
		fmt.Printf("   Pop: %v\n", v)
	}

	// 3. slices 包（Go 1.21 进入标准库）
	fmt.Println("\n3. slices 包:")
	s := []int{3, 1, 4, 1, 5}
	slices.Sort(s)
	fmt.Println("   排序后:", s)
	fmt.Println("   Contains 4:", slices.Contains(s, 4))
}
