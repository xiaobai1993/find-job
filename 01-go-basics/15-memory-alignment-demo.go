// Go 内存对齐与 struct 布局优化演示
// go run 15-memory-alignment-demo.go
package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"
)

// ============================================================
// 1. 坏的字段顺序 - 浪费内存
// ============================================================

type BadStruct struct {
	a bool   // 1 字节
	b int64  // 8 字节
	c uint16 // 2 字节
}

// ============================================================
// 2. 好的字段顺序 - 内存最优
// ============================================================

type GoodStruct struct {
	b int64  // 8 字节
	c uint16 // 2 字节
	a bool   // 1 字节
}

// ============================================================
// 3. 空结构体的位置影响
// ============================================================

// 空结构体放最后 - 需要额外 padding
type EmptyAtEnd struct {
	a int
	b struct{} // 空结构体放最后
}

// 空结构体放最前 - 不需要额外 padding
type EmptyAtStart struct {
	b struct{} // 空结构体放最前
	a int
}

// ============================================================
// 4. 原子操作对齐问题（32位系统上演示）
// ============================================================

// 64位字段放最后，32位系统上可能不对齐
type BadAtomic struct {
	x int32
	y int64 // 可能只对齐到 4 字节
}

// 64位字段放最前，保证对齐
type GoodAtomic struct {
	y int64 // 放第一个，一定对齐到 8 字节
	x int32
}

// ============================================================
// 演示函数
// ============================================================

func demoStructSize() {
	fmt.Println("=== 1. 字段顺序对大小的影响 ===")
	fmt.Printf("BadStruct  大小: %d 字节\n", unsafe.Sizeof(BadStruct{}))
	fmt.Printf("GoodStruct 大小: %d 字节\n", unsafe.Sizeof(GoodStruct{}))
	fmt.Printf("节省: %d 字节 (%.1f%%)\n",
		unsafe.Sizeof(BadStruct{})-unsafe.Sizeof(GoodStruct{}),
		float64(unsafe.Sizeof(BadStruct{})-unsafe.Sizeof(GoodStruct{}))/float64(unsafe.Sizeof(BadStruct{}))*100,
	)
	fmt.Println()
}

func demoFieldOffset() {
	fmt.Println("=== 2. BadStruct 字段偏移 ===")
	b := BadStruct{}
	fmt.Printf("a 偏移: %d 字节\n", unsafe.Offsetof(b.a))
	fmt.Printf("b 偏移: %d 字节 <- 这里补了 7 字节 padding!\n", unsafe.Offsetof(b.b))
	fmt.Printf("c 偏移: %d 字节\n", unsafe.Offsetof(b.c))
	fmt.Println()

	fmt.Println("=== 3. GoodStruct 字段偏移 ===")
	g := GoodStruct{}
	fmt.Printf("b 偏移: %d 字节\n", unsafe.Offsetof(g.b))
	fmt.Printf("c 偏移: %d 字节\n", unsafe.Offsetof(g.c))
	fmt.Printf("a 偏移: %d 字节\n", unsafe.Offsetof(g.a))
	fmt.Println()
}

func demoEmptyStructPos() {
	fmt.Println("=== 4. 空结构体位置影响 ===")
	fmt.Printf("EmptyAtEnd   大小: %d 字节 <- 额外补了 4 字节 padding!\n", unsafe.Sizeof(EmptyAtEnd{}))
	fmt.Printf("EmptyAtStart 大小: %d 字节 <- 不需要额外 padding\n", unsafe.Sizeof(EmptyAtStart{}))
	fmt.Println()
}

func demoAlignment() {
	fmt.Println("=== 5. 各类型对齐系数 ===")
	fmt.Printf("bool    对齐: %d\n", unsafe.Alignof(bool(false)))
	fmt.Printf("int32   对齐: %d\n", unsafe.Alignof(int32(0)))
	fmt.Printf("int64   对齐: %d\n", unsafe.Alignof(int64(0)))
	fmt.Printf("float64 对齐: %d\n", unsafe.Alignof(float64(0)))
	fmt.Printf("string  对齐: %d\n", unsafe.Alignof(""))
	fmt.Printf("*int    对齐: %d\n", unsafe.Alignof((*int)(nil)))
	fmt.Println()
}

func demoAtomicAlignment() {
	fmt.Println("=== 6. 原子操作对齐演示 ===")

	good := GoodAtomic{}
	atomic.AddInt64(&good.y, 1) // ✅ 安全，y 放第一个保证对齐
	fmt.Printf("GoodAtomic.y 地址: %p\n", &good.y)
	fmt.Printf("GoodAtomic.y 对齐: %d\n", unsafe.Alignof(good.y))

	// 注意：32位系统上 BadAtomic.y 可能不对齐导致崩溃
	// 64位系统上这里没问题，因为内存分配都是 8 字节对齐的
	bad := BadAtomic{}
	atomic.AddInt64(&bad.y, 1) // 在 32位系统上可能 panic
	fmt.Printf("BadAtomic.y  地址: %p\n", &bad.y)
	fmt.Printf("BadAtomic.y  对齐: %d\n", unsafe.Alignof(bad.y))

	fmt.Println("✅ 64位系统上都没问题，32位系统上字段放最后可能崩溃")
	fmt.Println()
}

// ============================================================
// main
// ============================================================

func main() {
	demoStructSize()
	demoFieldOffset()
	demoEmptyStructPos()
	demoAlignment()
	demoAtomicAlignment()

	fmt.Println("✅ 所有示例运行完成！")
}
