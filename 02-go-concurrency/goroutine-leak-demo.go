package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"
)

// ============================================
// 演示 1：无缓冲 channel → 会泄漏 goroutine
// ============================================

func badLeakExample() int {
	ch := make(chan int) // ❌ 无缓冲！

	go func() {
		time.Sleep(100 * time.Millisecond) // 故意等一下，让超时先触发
		fmt.Println("badLeakExample: 子 goroutine 准备写 channel...")
		ch <- 666 // 👉 如果没人读，这里会永久阻塞！
		fmt.Println("badLeakExample: 子 goroutine 写完了（你永远看不到这行打印）")
	}()

	select {
	case v := <-ch:
		return v
	case <-time.After(50 * time.Millisecond): // 超时先触发！
		fmt.Println("badLeakExample: 超时了，直接返回")
		return -1
	}
}

// ============================================
// 演示 2：有缓冲 1 → 不会泄漏
// ============================================

func goodNoLeakExample() int {
	ch := make(chan int, 1) // ✅ 缓冲 1

	go func() {
		fmt.Println("goodNoLeakExample: 子 goroutine 准备写 channel...")
		ch <- 666 // 👉 缓冲区有空位，不管有没有人读都能写完
		fmt.Println("goodNoLeakExample: 子 goroutine 写完了（你会看到这行打印）")
	}()

	select {
	case v := <-ch:
		return v
	case <-time.After(500 * time.Millisecond):
		fmt.Println("goodNoLeakExample: 超时了，直接返回")
		return -1
	}
}

// ============================================
// 演示 3：context 取消 → 无缓冲也不会泄漏
// ============================================

func ctxNoLeakExample() int {
	ch := make(chan int) // 无缓冲，但用 context 就没事
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	go func() {
		fmt.Println("ctxNoLeakExample: 子 goroutine 准备写 channel...")
		select {
		case ch <- 666:
			fmt.Println("ctxNoLeakExample: 子 goroutine 写完了")
		case <-ctx.Done(): // 超时了，自己退出，不阻塞
			fmt.Println("ctxNoLeakExample: 子 goroutine 收到取消信号，退出")
			return
		}
	}()

	select {
	case v := <-ch:
		return v
	case <-ctx.Done():
		fmt.Println("ctxNoLeakExample: 超时了，直接返回")
		return -1
	}
}

// ============================================
// 打印当前 goroutine 数量
// ============================================

func printGoroutineCount(label string) {
	fmt.Printf("\n[%s] 当前 goroutine 数量: %d\n", label, runtime.NumGoroutine())
}

func main() {
	fmt.Println("=== 演示 1：无缓冲 → 泄漏 ===")
	printGoroutineCount("开始")
	badLeakExample()
	time.Sleep(1 * time.Second) // 等一下看泄漏
	printGoroutineCount("badLeakExample 结束后")
	fmt.Println("👉 goroutine 数量从 1 变成了 2，泄漏了！")

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("=== 演示 2：有缓冲 1 → 不泄漏 ===")
	printGoroutineCount("开始")
	goodNoLeakExample()
	time.Sleep(1 * time.Second)
	printGoroutineCount("goodNoLeakExample 结束后")
	fmt.Println("👉 goroutine 数量回到 1，没有泄漏！")

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("=== 演示 3：context 取消 → 无缓冲也不泄漏 ===")
	printGoroutineCount("开始")
	ctxNoLeakExample()
	time.Sleep(1 * time.Second)
	printGoroutineCount("ctxNoLeakExample 结束后")
	fmt.Println("👉 goroutine 数量回到 1，没有泄漏！")

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("\n总结：")
	fmt.Println("1. 启动 goroutine 写 channel 前，一定要想清楚：")
	fmt.Println("   如果读端退出了，写端会不会堵死？")
	fmt.Println("2. 两种解决方式：")
	fmt.Println("   - 方式 A：给 channel 加缓冲（最简单）")
	fmt.Println("   - 方式 B：写端也监听取消信号（更可控）")
}
