// Go 数据竞争与 Race Detector 演示
// 🔴 检测数据竞争: go run -race 12-race-condition-demo.go
// 正常运行（不检测）: go run 12-race-condition-demo.go
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// 1. 最基础的数据竞争
// ============================================================

func demoBasicRace() {
	fmt.Println("=== 1. 最基础的数据竞争 ===")

	var count int

	go func() {
		for i := 0; i < 10000; i++ {
			count++ // 写
		}
	}()

	for i := 0; i < 10000; i++ {
		count++ // 同时写！竞争！
	}

	time.Sleep(100 * time.Millisecond)
	fmt.Printf("count = %d (期望 20000，每次结果可能都不一样!)\n", count)
	fmt.Println("💡 用 go run -race 运行可以检测到竞争!")
	fmt.Println()
}

// ============================================================
// 2. 修复方案 1：Mutex 互斥锁
// ============================================================

func demoMutexFix() {
	fmt.Println("=== 2. 修复方案 1：Mutex 互斥锁 ===")

	var mu sync.Mutex
	var count int

	go func() {
		for i := 0; i < 10000; i++ {
			mu.Lock()
			count++
			mu.Unlock()
		}
	}()

	for i := 0; i < 10000; i++ {
		mu.Lock()
		count++
		mu.Unlock()
	}

	time.Sleep(100 * time.Millisecond)
	fmt.Printf("count = %d (每次都是 20000，正确!)\n", count)
	fmt.Println()
}

// ============================================================
// 3. 修复方案 2：atomic 原子操作
// ============================================================

func demoAtomicFix() {
	fmt.Println("=== 3. 修复方案 2：atomic 原子操作 ===")

	var count int64

	go func() {
		for i := 0; i < 10000; i++ {
			atomic.AddInt64(&count, 1)
		}
	}()

	for i := 0; i < 10000; i++ {
		atomic.AddInt64(&count, 1)
	}

	time.Sleep(100 * time.Millisecond)
	fmt.Printf("count = %d (每次都是 20000，正确!)\n", count)
	fmt.Println()
}

// ============================================================
// 4. 修复方案 3：Channel，不共享内存
// ============================================================

func demoChannelFix() {
	fmt.Println("=== 4. 修复方案 3：Channel，不共享内存 ===")

	countCh := make(chan int, 1)
	countCh <- 0

	go func() {
		for i := 0; i < 10000; i++ {
			count := <-countCh
			count++
			countCh <- count
		}
	}()

	for i := 0; i < 10000; i++ {
		count := <-countCh
		count++
		countCh <- count
	}

	time.Sleep(100 * time.Millisecond)
	count := <-countCh
	fmt.Printf("count = %d (每次都是 20000，正确!)\n", count)
	fmt.Println()
}

// ============================================================
// 5. 修复方案 4：根本不共享，每个 goroutine 自己算
// ============================================================

func demoNoShareFix() {
	fmt.Println("=== 5. 修复方案 4：根本不共享，最后汇总 ===")

	var wg sync.WaitGroup
	resultCh := make(chan int, 2)

	// goroutine 1
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := 0
		for i := 0; i < 10000; i++ {
			count++
		}
		resultCh <- count
	}()

	// goroutine 2
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := 0
		for i := 0; i < 10000; i++ {
			count++
		}
		resultCh <- count
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	total := 0
	for c := range resultCh {
		total += c
	}

	fmt.Printf("total = %d (每次都是 20000，正确!)\n", total)
	fmt.Println()
}

// ============================================================
// 6. 隐蔽的竞争：读也需要加锁！
// ============================================================

func demoReadRace() {
	fmt.Println("=== 6. 隐蔽的竞争：读也需要加锁！ ===")

	var mu sync.Mutex
	var config map[string]string // 注意：map 本身也不能并发读写

	// 写 goroutine
	go func() {
		for i := 0; ; i++ {
			mu.Lock() // ✅ 必须加锁！不加的话读的时候可能读到一半写了
			config = map[string]string{
				"version": fmt.Sprintf("v%d", i),
				"status":  "ok",
			}
			mu.Unlock()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// 读 goroutine
	go func() {
		for i := 0; i < 100; i++ {
			mu.Lock() // ✅ 读也需要加锁！不要以为读就没事！
			v := config["version"]
			_ = v
			mu.Unlock()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	fmt.Println("💡 重要：读也需要加锁！只要有一个写在同时进行，读就是竞争！")
	fmt.Println()
}

// ============================================================
// 7. Map 并发读写直接 panic
// ============================================================

func demoMapPanic() {
	fmt.Println("=== 7. Map 并发读写直接 panic ===")
	fmt.Println("⚠️  Go 1.6+ 检测到 map 并发读写会直接 panic！")
	fmt.Println("💡 这是 Go 的设计，避免带 bug 运行到生产环境")

	m := make(map[int]int)

	// 写
	go func() {
		for i := 0; i < 1000; i++ {
			m[i] = i
		}
	}()

	// 读
	go func() {
		for i := 0; i < 1000; i++ {
			_ = m[i]
		}
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Println("如果没 panic 说明运气好，多运行几次。💡 用 -race 一定能检测到")
	fmt.Println()
}

// ============================================================
// main
// ============================================================

func main() {
	fmt.Println("=====================================================")
	fmt.Println("🔴 请用 go run -race 12-race-condition-demo.go 运行")
	fmt.Println("   这样才能检测到数据竞争！")
	fmt.Println("=====================================================\n")

	demoBasicRace()
	demoMutexFix()
	demoAtomicFix()
	demoChannelFix()
	demoNoShareFix()
	demoReadRace()
	demoMapPanic()

	fmt.Println("✅ 所有示例运行完成！")
	fmt.Println("\n💡 总结：")
	fmt.Println("   1. 所有测试都要跑 -race，包括单元测试！")
	fmt.Println("   2. 读也需要加锁！只要有写同时进行就是竞争！")
	fmt.Println("   3. map 并发读写直接 panic，不要侥幸！")
	fmt.Println("   4. 最简单的方案：不要共享内存，用 channel 通信！")
}
