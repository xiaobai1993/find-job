// Go goroutine 基础与栈管理演示
// go run 01-goroutine-basics-demo.go
// 检测 goroutine 泄漏: go run -race 01-goroutine-basics-demo.go
package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"time"
)

// ============================================================
// 1. 基础 goroutine 创建
// ============================================================

func demoBasicGoroutine() {
	fmt.Println("=== 1. 基础 goroutine 创建 ===")

	go func() {
		fmt.Println("Hello from goroutine!")
	}()

	time.Sleep(100 * time.Millisecond) // 等 goroutine 执行完
	fmt.Println("Main function done")
	fmt.Println()
}

// ============================================================
// 2. goroutine 泄漏场景 1：channel 没人读
// ============================================================

func demoLeakNoReader() {
	fmt.Println("=== 2. goroutine 泄漏：channel 没人读 ===")

	// ❌ 泄漏：无 buffer channel，没人读，goroutine 永远卡在写
	ch := make(chan int) // 无 buffer！
	go func() {
		fmt.Println("Goroutine 尝试写 channel...")
		ch <- 1 // 永远卡在这！泄漏了！
		fmt.Println("Goroutine 写完了") // 永远不会执行到这里
	}()

	time.Sleep(500 * time.Millisecond)
	fmt.Println("Main 返回了，但那个 goroutine 还卡在 ch <- 1！")
	fmt.Println("💡 修复：用带 buffer 的 channel: make(chan int, 1)")
	fmt.Println()
}

// ============================================================
// 3. goroutine 泄漏场景 2：http 请求没有超时
// ============================================================

func demoLeakNoTimeout() {
	fmt.Println("=== 3. goroutine 泄漏：http 请求没有超时 ===")
	fmt.Println("⚠️  http.DefaultClient 没有超时，对端不返回的话 goroutine 永远卡住")
	fmt.Println("💡 修复：创建 Client 时设置 Timeout")

	client := &http.Client{
		Timeout: 5 * time.Second, // ✅ 设置超时
	}
	fmt.Printf("正确示例：client.Timeout = %v\n", client.Timeout)
	fmt.Println()
}

// ============================================================
// 4. goroutine 泄漏场景 3：time.Ticker 没 Stop
// ============================================================

func demoLeakTicker() {
	fmt.Println("=== 4. goroutine 泄漏：time.Ticker 没 Stop ===")

	// ❌ 泄漏：Ticker 用完没 Stop
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		for range ticker.C {
			fmt.Print(".")
		}
	}()

	time.Sleep(500 * time.Millisecond)
	// ticker.Stop() // ✅ 必须调用 Stop！否则 Ticker 永远跑

	fmt.Println("\nMain 返回了，但 ticker 还在跑！goroutine 泄漏了！")
	fmt.Println("💡 修复：defer ticker.Stop()")
	fmt.Println()
}

// ============================================================
// 5. goroutine 泄漏场景 4：context 没 cancel
// ============================================================

func demoLeakContext() {
	fmt.Println("=== 5. goroutine 泄漏：context 没 cancel ===")
	fmt.Println("⚠️  context.WithCancel 创建的 ctx 用完必须 cancel")
	fmt.Println("💡 修复：defer cancel()")
	fmt.Println()
}

// ============================================================
// 6. 闭包捕获循环变量的经典坑
// ============================================================

func demoClosureLoopVar() {
	fmt.Println("=== 6. 闭包捕获循环变量的经典坑 ===")

	fmt.Println("❌ 错误写法：所有 goroutine 拿到同一个 i：")
	for i := 0; i < 3; i++ {
		go func() {
			fmt.Printf("错误: i = %d\n", i) // 可能都打印 3！
		}()
	}
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n✅ 正确写法：循环内捕获：")
	for i := 0; i < 3; i++ {
		i := i // ✅ 循环内声明，每次迭代新变量
		go func() {
			fmt.Printf("正确: i = %d\n", i)
		}()
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Println()
}

// ============================================================
// 7. 启动 pprof 查看 goroutine 数量
// ============================================================

func startPProf() {
	go func() {
		fmt.Println("=== 7. pprof 查看 goroutine 数量 ===")
		fmt.Println("浏览器打开: http://localhost:6060/debug/pprof/goroutine?debug=1")
		fmt.Println("或者命令行: curl http://localhost:6060/debug/pprof/goroutine?debug=2")
		_ = http.ListenAndServe(":6060", nil)
	}()
}

// ============================================================
// main
// ============================================================

func main() {
	startPProf()

	demoBasicGoroutine()
	demoLeakNoReader()
	demoLeakNoTimeout()
	demoLeakTicker()
	demoLeakContext()
	demoClosureLoopVar()

	fmt.Println("\n✅ 所有示例运行完成！")
	fmt.Println("\n💡 检测泄漏方法：")
	fmt.Println("   go tool pprof http://localhost:6060/debug/pprof/goroutine")
	fmt.Println("   或者看浏览器里的 goroutine 数量一直在涨就是泄漏了")

	fmt.Println("\n按 Ctrl+C 退出...")
	select {} // 保持程序运行，方便 pprof 查看
}
