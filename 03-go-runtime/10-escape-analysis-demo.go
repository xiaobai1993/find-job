package main

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ============================================
// 演示 1：返回指针 vs 返回值
// ============================================

type User struct {
	Name string
	Age  int
}

// ❌ 逃逸：返回局部变量指针
func newUserPtr() *User {
	u := User{Name: "alice", Age: 30} // 逃逸到堆！
	return &u
}

// ✅ 不逃逸：返回值
func newUserValue() User {
	u := User{Name: "alice", Age: 30} // 栈上分配
	return u                           // 值拷贝
}

func demo1ReturnPointer() {
	fmt.Println("=== 演示 1：返回指针 vs 返回值 ===")

	p := newUserPtr()
	fmt.Printf("   返回指针: %+v（逃逸到堆）\n", p)

	v := newUserValue()
	fmt.Printf("   返回值:   %+v（栈上分配）\n", v)

	fmt.Println("\n   💡 小结构体优先返回值，避免逃逸！")
}

// ============================================
// 演示 2：闭包引用导致逃逸
// ============================================

func counter() func() int {
	count := 0 // 逃逸到堆！闭包捕获
	return func() int {
		count++
		return count
	}
}

func demo2Closure() {
	fmt.Println("\n=== 演示 2：闭包引用导致逃逸 ===")

	c := counter()
	fmt.Printf("   调用 1: %d\n", c())
	fmt.Printf("   调用 2: %d\n", c())
	fmt.Printf("   调用 3: %d\n", c())

	fmt.Println("\n   💡 闭包捕获的变量 count 逃逸到堆，因为闭包可能在函数返回后执行")
}

// ============================================
// 演示 3：interface{} 导致逃逸
// ============================================

func printAny(v interface{}) { // v 逃逸到堆
	fmt.Printf("   printAny: %v\n", v)
}

func demo3Interface() {
	fmt.Println("\n=== 演示 3：interface{} 导致逃逸 ===")

	x := 42 // 本来在栈上，但传给 interface{} 就逃逸了
	printAny(x)

	fmt.Println("\n   💡 fmt.Println 的参数是 interface{}，所以传值给 fmt 系列函数总是逃逸！")
	fmt.Println("   💡 这也是 fmt.Println 比.WriteString 慢的原因之一")
}

// ============================================
// 演示 4：slice 扩容导致逃逸
// ============================================

func demo4Slice() {
	fmt.Println("\n=== 演示 4：slice 扩容 vs 预分配 ===")

	// ❌ 动态扩容
	var s1 []int
	start := time.Now()
	for i := 0; i < 100000; i++ {
		s1 = append(s1, i)
	}
	t1 := time.Since(start)

	// ✅ 预分配
	start = time.Now()
	s2 := make([]int, 0, 100000)
	for i := 0; i < 100000; i++ {
		s2 = append(s2, i)
	}
	t2 := time.Since(start)

	fmt.Printf("   动态扩容: %v\n", t1)
	fmt.Printf("   预分配:   %v\n", t2)
	fmt.Printf("   预分配快 %.1f 倍\n", float64(t1)/float64(t2))

	fmt.Println("\n   💡 预分配避免扩容拷贝，减少逃逸和 GC 压力！")
}

// ============================================
// 演示 5：大对象逃逸
// ============================================

func demo5BigObject() {
	fmt.Println("\n=== 演示 5：大对象逃逸 ===")

	fmt.Println("   栈空间有限，大对象直接分配到堆：")
	fmt.Println("   make([]byte, 100)     → 栈上分配 ✅")
	fmt.Println("   make([]byte, 100*1024) → 堆上分配 ❌（超过 64KB 阈值）")
	fmt.Println("\n   💡 编译器通常以 64KB 为界，超过则堆分配")
}

// ============================================
// 演示 6：channel 发送导致逃逸
// ============================================

func demo6Channel() {
	fmt.Println("\n=== 演示 6：channel 发送导致逃逸 ===")

	fmt.Println("   发送到 channel 的对象会逃逸：")
	fmt.Println("   ch <- &Data{ID: 1}  → Data 逃逸到堆")
	fmt.Println("   因为接收方可能在另一个 goroutine，无法确定生命周期")

	fmt.Println("\n   💡 channel 传值比传指针逃逸少，但值拷贝有开销")
	fmt.Println("   💡 权衡：小对象传值，大对象传指针")
}

// ============================================
// 演示 7：sync.Pool 复用堆对象
// ============================================

func demo7SyncPool() {
	fmt.Println("\n=== 演示 7：sync.Pool 复用堆对象 ===")

	var bufPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	const iterations = 100000

	// 不使用 Pool
	start := time.Now()
	for i := 0; i < iterations; i++ {
		buf := bytes.NewBuffer(make([]byte, 0, 1024))
		buf.WriteString("hello")
		_ = buf.String()
	}
	noPoolTime := time.Since(start)

	// 使用 Pool
	start = time.Now()
	for i := 0; i < iterations; i++ {
		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		buf.WriteString("hello")
		_ = buf.String()
		bufPool.Put(buf)
	}
	poolTime := time.Since(start)

	fmt.Printf("   不使用 Pool: %v\n", noPoolTime)
	fmt.Printf("   使用 Pool:   %v\n", poolTime)
	fmt.Printf("   Pool 快 %.1f 倍\n", float64(noPoolTime)/float64(poolTime))

	fmt.Println("\n   💡 sync.Pool 复用堆对象，减少分配和 GC 压力！")
}

// ============================================
// 演示 8：逃逸分析对 GC 的影响
// ============================================

func demo8EscapeAndGC() {
	fmt.Println("\n=== 演示 8：逃逸分析对 GC 的影响 ===")

	var statsBefore, statsAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&statsBefore)

	// 创建大量逃逸对象
	for i := 0; i < 100000; i++ {
		_ = newUserPtr() // 逃逸到堆
	}

	runtime.GC()
	runtime.ReadMemStats(&statsAfter)

	fmt.Printf("   GC 前堆大小: %.2f MB\n", float64(statsBefore.HeapAlloc)/1024/1024)
	fmt.Printf("   GC 后堆大小: %.2f MB\n", float64(statsAfter.HeapAlloc)/1024/1024)
	fmt.Printf("   GC 次数: %d\n", statsAfter.NumGC-statsBefore.NumGC)

	fmt.Println("\n   💡 逃逸对象越多 → 堆越大 → GC 越频繁 → STW 越多 → 延迟越高")
	fmt.Println("   💡 逃逸分析优化 = GC 优化 = 延迟优化！")
}

// ============================================
// 演示 9：如何用 go build -gcflags="-m" 查看
// ============================================

func demo9HowToCheck() {
	fmt.Println("\n=== 演示 9：如何查看逃逸分析 ===")

	fmt.Println("   命令：")
	fmt.Println("   go build -gcflags=\"-m\" ./...          # 基本逃逸信息")
	fmt.Println("   go build -gcflags=\"-m -m\" ./...       # 详细逃逸原因")
	fmt.Println("   go build -gcflags=\"-m -l\" ./...       # 禁止内联，更清晰")

	fmt.Println("\n   输出示例：")
	fmt.Println("   ./main.go:5:6: &x escapes to heap     # 逃逸！")
	fmt.Println("   ./main.go:10:6: y does not escape     # 不逃逸 ✅")
	fmt.Println("   ./main.go:15:12: User literal escapes  # 结构体字面量逃逸")

	fmt.Println("\n   💡 每次写完代码跑一下 -m，养成习惯！")
}

// ============================================
// 演示 10：逃逸分析完整优化决策树
// ============================================

func demo10DecisionTree() {
	fmt.Println("\n=== 演示 10：逃逸优化决策树 ===")

	fmt.Println("   变量需要分配")
	fmt.Println("     ↓")
	fmt.Println("   变量的引用是否逃出函数作用域？")
	fmt.Println("     ├── 否 → 栈分配 ✅（零开销）")
	fmt.Println("     └── 是 → 逃逸到堆 ❌")
	fmt.Println("           ↓")
	fmt.Println("       能否优化？")
	fmt.Println("           ├── 返回指针 → 改为返回值（小结构体）")
	fmt.Println("           ├── interface{} → 改为具体类型")
	fmt.Println("           ├── 闭包引用 → 改为参数传递")
	fmt.Println("           ├── slice 扩容 → 预分配容量")
	fmt.Println("           ├── 大对象 → sync.Pool 复用")
	fmt.Println("           └── 无法优化 → 接受堆分配")

	fmt.Println("\n   💡 不是所有逃逸都需要优化！只优化热点路径！")
}

// ============================================
// 总结
// ============================================

func printSummary() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 逃逸分析所有演示完成！")
	fmt.Println("=== 核心总结 ===")
	fmt.Println("")
	fmt.Println("【什么是逃逸】")
	fmt.Println("   变量的引用逃出函数作用域，编译器必须分配到堆上")
	fmt.Println("")
	fmt.Println("【六大逃逸场景】")
	fmt.Println("   1. 返回局部变量指针")
	fmt.Println("   2. 发送到 channel")
	fmt.Println("   3. 闭包捕获变量")
	fmt.Println("   4. 赋值给 interface{}（fmt.Println 等）")
	fmt.Println("   5. slice/map 动态扩容")
	fmt.Println("   6. 栈空间 > 64KB")
	fmt.Println("")
	fmt.Println("【优化手段】")
	fmt.Println("   ✅ 小结构体返回值，不返回指针")
	fmt.Println("   ✅ 避免不必要的 interface{}")
	fmt.Println("   ✅ 预分配 slice/map 容量")
	fmt.Println("   ✅ sync.Pool 复用堆对象")
	fmt.Println("   ✅ 用 go build -gcflags=\"-m\" 验证")
	fmt.Println("")
	fmt.Println("【与 GC 的关系】")
	fmt.Println("   逃逸对象越多 → 堆越大 → GC 越频繁 → STW 越多 → 延迟越高")
	fmt.Println("   逃逸分析优化 = GC 优化 = 延迟优化！")
}

// ============================================
// main
// ============================================

func main() {
	demo1ReturnPointer()
	demo2Closure()
	demo3Interface()
	demo4Slice()
	demo5BigObject()
	demo6Channel()
	demo7SyncPool()
	demo8EscapeAndGC()
	demo9HowToCheck()
	demo10DecisionTree()

	printSummary()
}
