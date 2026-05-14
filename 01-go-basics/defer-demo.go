package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ============================================
// 演示 1：LIFO 后进先出执行顺序
// ============================================
func demo1LIFO() {
	fmt.Println("=== 演示 1：LIFO 后进先出 ===")
	defer fmt.Println("1")
	defer fmt.Println("2")
	defer fmt.Println("3")
	fmt.Println("函数正常结束")
	// 输出：
	// 函数正常结束
	// 3
	// 2
	// 1
}

// ============================================
// 演示 2：defer 注册时参数立即求值（最大的坑！）
// ============================================
func demo2ParamEval() {
	fmt.Println("\n=== 演示 2：参数立即求值 ===")

	i := 0
	defer fmt.Println("defer 输出:", i) // 这里 i 的值已经是 0 了！
	i = 100
	fmt.Println("函数结束时 i =", i)
	// 输出：
	// 函数结束时 i = 100
	// defer 输出: 0
}

// ============================================
// 演示 3：匿名返回值 vs 命名返回值（最高频面试题）
// ============================================

// 匿名返回值
func anonymousReturn() int {
	i := 0
	defer func() {
		i++
		fmt.Println("defer 里 i =", i)
	}()
	return i
}

// 命名返回值
func namedReturn() (i int) {
	i = 0
	defer func() {
		i++
		fmt.Println("defer 里 i =", i)
	}()
	return i
}

func demo3ReturnValue() {
	fmt.Println("\n=== 演示 3：匿名返回值 vs 命名返回值 ===")
	fmt.Println("匿名返回值结果:", anonymousReturn()) // 0
	fmt.Println("命名返回值结果:", namedReturn())     // 1
}

// ============================================
// 演示 4：return 拆解三步
// ============================================
func demo4ReturnBreakdown() (result int) {
	fmt.Println("\n=== 演示 4：return 拆解三步 ===")
	fmt.Println("1. 函数开始执行")

	defer func() {
		fmt.Println("3. defer 执行，修改 result")
		result = 100
	}()

	fmt.Println("2. 准备 return 50")
	return 50
}

// ============================================
// 演示 5：循环里的 defer 两个坑
// ============================================
func demo5LoopDeferBad() {
	fmt.Println("\n=== 演示 5a：循环 defer 错误写法（Go 1.22 后语义变化）===")
	fmt.Println("Go 1.21及以前: 三个都是2（变量复用）")
	fmt.Println("Go 1.22及以后: 2 1 0（每次循环创建新变量）")
	for i := 0; i < 3; i++ {
		defer fmt.Println("Go 1.22输出:", i)
	}
}

func demo5LoopDeferGood() {
	fmt.Println("\n=== 演示 5b：循环 defer 正确写法（传值）===")
	fmt.Println("正确写法：defer func(n int) { ... }(i)")
	for i := 0; i < 3; i++ {
		defer func(n int) {
			fmt.Println("正确写法输出:", n)
		}(i) // 这里的 i 是每次循环的当前值
	}
}

// ============================================
// 演示 6：闭包捕获的是引用，不是值
// ============================================
func demo6Closure() {
	fmt.Println("\n=== 演示 6：闭包捕获引用 vs 传参 ===")

	i := 0

	// 闭包捕获引用，执行时才求值
	defer func() {
		fmt.Println("闭包捕获输出:", i)
	}()

	// 直接传参，注册时就求值
	defer fmt.Println("直接传参输出:", i)

	i = 100
	fmt.Println("函数结束时 i =", i)
	// 输出：
	// 函数结束时 i = 100
	// 直接传参输出: 0
	// 闭包捕获输出: 100
}

// ============================================
// 演示 7：defer nil 函数会 panic
// ============================================
func demo7DeferNil() {
	fmt.Println("\n=== 演示 7：defer nil 函数 ===")

	var f func()
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("✅ 果然 panic 了:", r)
		}
	}()
	defer f() // f 是 nil，执行的时候 panic
	fmt.Println("这行会执行")
}

// ============================================
// 演示 8：经典应用场景 - 资源释放
// ============================================
func demo8ResourceRelease() {
	fmt.Println("\n=== 演示 8：经典应用 - 资源释放 ===")

	// 创建临时文件，defer 确保关闭
	f, err := os.CreateTemp("", "defer-demo-*.txt")
	if err != nil {
		fmt.Println("创建文件失败:", err)
		return
	}
	defer f.Close() // 确保文件一定会关闭
	defer os.Remove(f.Name()) // 确保文件一定会删除

	fmt.Println("文件打开成功，defer 会确保文件关闭和删除")
}

// ============================================
// 演示 9：经典应用场景 - 锁释放
// ============================================
func demo9LockRelease() {
	fmt.Println("\n=== 演示 9：经典应用 - 锁释放 ===")

	var mu sync.Mutex

	process := func(name string) {
		mu.Lock()
		defer mu.Unlock() // 无论怎么 panic，锁都会释放
		fmt.Println(name, "拿到锁，defer 确保释放锁")
		// 临界区...
	}

	process("worker1")
	process("worker2")
}

// ============================================
// 演示 10：经典应用场景 - 耗时统计
// ============================================
func trackTime(name string) func() {
	start := time.Now()
	return func() {
		fmt.Printf("%s 耗时: %v\n", name, time.Since(start))
	}
}

func demo10TrackTime() {
	fmt.Println("\n=== 演示 10：经典应用 - 耗时统计 ===")
	defer trackTime("demo10TrackTime")()

	// 模拟业务逻辑
	time.Sleep(100 * time.Millisecond)
	fmt.Println("业务逻辑执行中...")
	time.Sleep(100 * time.Millisecond)
}

// ============================================
// 演示 11：panic 之后注册的 defer 不会执行
// ============================================
func demo11PanicAfterDefer() {
	fmt.Println("\n=== 演示 11：panic 之后的 defer 不执行 ===")

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("捕获到 panic:", r)
		}
	}()

	defer fmt.Println("panic 之前注册的 defer 会执行")

	panic("oh no")

	defer fmt.Println("panic 之后注册的 defer 永远不会执行") // 这行永远不会执行
}

// ============================================
// main
// ============================================
func main() {
	demo1LIFO()
	demo2ParamEval()
	demo3ReturnValue()

	result := demo4ReturnBreakdown()
	fmt.Println("demo4 最终返回值:", result) // 100

	demo5LoopDeferBad()
	demo5LoopDeferGood()
	demo6Closure()
	demo7DeferNil()
	demo8ResourceRelease()
	demo9LockRelease()
	demo10TrackTime()
	demo11PanicAfterDefer()

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有 defer 演示完成！")
	fmt.Println("核心总结：")
	fmt.Println("1. LIFO 执行顺序")
	fmt.Println("2. 参数注册时立即求值，闭包执行时求值")
	fmt.Println("3. return = 赋值 → defer → 返回")
	fmt.Println("4. 命名返回值 defer 可以改返回值，匿名不行")
	fmt.Println("5. 循环里的 defer 一定要传值进去")
}
