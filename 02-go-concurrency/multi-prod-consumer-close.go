package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ============================================
// 场景：3 个生产者，2 个消费者
// 核心原则：永远是生产者负责关闭 channel，消费者只负责感知关闭
// ============================================

func main() {
	fmt.Println("=== 多生产者多消费者正确关闭 demo ===")

	ch := make(chan int, 10)

	// ------------------------------
	// 方案 1：用 WaitGroup 等所有生产者完成，再统一关闭
	// 这是最清晰、最推荐的写法
	// ------------------------------
	var producerWg sync.WaitGroup

	// 启动 3 个生产者
	for i := 0; i < 3; i++ {
		producerWg.Add(1)
		go func(id int) {
			defer producerWg.Done() // 生产完成就减 1
			for j := 0; j < 5; j++ {
				ch <- id*100 + j
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Printf("生产者 %d 生产完成\n", id)
		}(i)
	}

	// 关键：单独启动一个 goroutine 负责关闭
	// 等所有生产者都完成了，再 close channel
	go func() {
		producerWg.Wait() // 阻塞到所有生产者都调用了 Done()
		close(ch)
		fmt.Println("所有生产者已完成，channel 已关闭")
	}()

	// ------------------------------
	// 启动 2 个消费者
	// ------------------------------
	var consumerWg sync.WaitGroup
	for i := 0; i < 2; i++ {
		consumerWg.Add(1)
		go func(id int) {
			defer consumerWg.Done()
			// range 会自动感知 channel 关闭
			// 关闭后，缓冲区的数据读完，循环就自动退出了
			for num := range ch {
				fmt.Printf("  消费者 %d 收到: %d\n", id, num)
			}
			fmt.Printf("消费者 %d 退出\n", id)
		}(i)
	}

	// 等所有消费者也处理完
	consumerWg.Wait()

	fmt.Println("\n✅ 所有 goroutine 都正常退出了")
	fmt.Printf("当前 goroutine 数量: %d (应该是 1，说明没有泄漏)\n", runtime.NumGoroutine())

	// ============================================
	// 错误写法对比（注释掉的部分是错误的，不要这么写）
	// ============================================
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("❌ 常见错误写法（不要这么写）：")

	fmt.Println("\n错误 1：消费者去 close channel")
	fmt.Println(`  go func() {
      for ... { ch <- x }  // 消费者 close 后，生产者继续写会直接 panic
  }()
  close(ch)  // ❌ 消费者不能 close`)

	fmt.Println("\n错误 2：某个生产者直接 close channel")
	fmt.Println(`  go producer1() { close(ch) }  // ❌ 其他生产者还在写，写完就 panic
  go producer2() { ... }`)

	fmt.Println("\n错误 3：多个生产者都去 close channel")
	fmt.Println(`  每个生产者结束都调用一次 close(ch)  // ❌ 重复 close 直接 panic`)

	// ============================================
	// 总结
	// ============================================
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✅ 正确关闭的 3 个要点：")
	fmt.Println("  1. 永远是生产者负责关闭，消费者只负责读")
	fmt.Println("  2. 多个生产者时，不能让其中某一个单独 close，要用 WaitGroup 等全部完成再 close")
	fmt.Println("  3. close 操作单独放在一个 goroutine 里做，不阻塞主流程")
	fmt.Println("\n✅ 消费者退出方式：")
	fmt.Println("  用 for range 读 channel，close 后缓冲区数据读完自动退出循环")
	fmt.Println("  不要用其他方式判断是否关闭，range 已经帮你处理好了")
}
