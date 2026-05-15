//go:build go1.23
// +build go1.23

package main

import (
	"cmp"
	"fmt"
	"iter"
	"maps"
	"slices"
	"time"
	"unique"
)

// ==================== 迭代器 ====================

func count(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

func evenNumbers(max int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range max {
			if i%2 == 0 {
				if !yield(i) {
					return
				}
			}
		}
	}
}

// Filter 过滤迭代器
func Filter[V any](seq iter.Seq[V], f func(V) bool) iter.Seq[V] {
	return func(yield func(V) bool) {
		for v := range seq {
			if f(v) && !yield(v) {
				return
			}
		}
	}
}

// Map 转换迭代器
func Map[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func demoIterator() {
	fmt.Println("=== Go 1.23 迭代器演示 ===")

	// 1. 基础迭代器
	fmt.Println("\n1. 基础迭代器 count(5):")
	fmt.Print("   ")
	for i := range count(5) {
		fmt.Printf("%d ", i)
	}
	fmt.Println()

	// 2. 偶数迭代器
	fmt.Println("\n2. evenNumbers(10):")
	fmt.Print("   ")
	for i := range evenNumbers(10) {
		fmt.Printf("%d ", i)
	}
	fmt.Println()

	// 3. slices 包迭代器
	fmt.Println("\n3. slices.Values:")
	s := []string{"a", "b", "c"}
	fmt.Print("   ")
	for v := range slices.Values(s) {
		fmt.Printf("%s ", v)
	}
	fmt.Println()

	fmt.Println("\n4. slices.All (索引+值):")
	fmt.Print("   ")
	for i, v := range slices.All(s) {
		fmt.Printf("[%d:%s] ", i, v)
	}
	fmt.Println()

	// 5. maps 包迭代器
	fmt.Println("\n5. maps.All:")
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	for k, v := range maps.All(m) {
		fmt.Printf("   %s -> %d\n", k, v)
	}

	// 6. 迭代器组合
	fmt.Println("\n6. Filter + Map:")
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := Filter(
		Map(slices.Values(nums), func(x int) int { return x * 2 }),
		func(x int) bool { return x > 10 },
	)
	fmt.Print("   ")
	for v := range result {
		fmt.Printf("%d ", v)
	}
	fmt.Println()
}

// ==================== unique 包 ====================

func demoUnique() {
	fmt.Println("\n=== unique 包演示 ===")

	// 字符串驻留
	s1 := unique.Make("hello world")
	s2 := unique.Make("hello world")
	s3 := unique.Make("other string")

	fmt.Printf("   s1 == s2: %v\n", s1 == s2)  // true
	fmt.Printf("   s1 == s3: %v\n", s1 == s3)  // false
	fmt.Printf("   s1.Value(): %q\n", s1.Value())

	// 结构体也可以
	type Config struct {
		MaxConn int
		Timeout time.Duration
	}

	c1 := unique.Make(Config{MaxConn: 100, Timeout: 30 * time.Second})
	c2 := unique.Make(Config{MaxConn: 100, Timeout: 30 * time.Second})

	fmt.Printf("   c1 == c2: %v\n", c1 == c2)  // true
	fmt.Println("   💡 相同的结构体只在内存中存一份！")
}

// ==================== cmp.Or ====================

func demoCmpOr() {
	fmt.Println("\n=== cmp.Or 演示 ===")

	// 取第一个非零值
	var empty string
	name := "alice"

	result := cmp.Or(empty, name, "anonymous")
	fmt.Printf("   cmp.Or(%q, %q, %q) = %q\n", empty, name, "anonymous", result)

	// 链式调用
	var a, b, c int
	c = 42
	fmt.Printf("   cmp.Or(%d, %d, %d) = %d\n", a, b, c, cmp.Or(a, b, c))
}

// ==================== main ====================

func main() {
	demoIterator()
	demoUnique()
	demoCmpOr()

	fmt.Println("\n✅ Go 1.23 所有演示完成")
}
