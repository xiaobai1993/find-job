// Go 内存模型演示：Happens-Before、可见性、乱序执行
// go run 13-go-memory-model-demo.go
package main

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================
// 1. Goroutine 启动：go 语句 happens before goroutine 内执行
// ============================================================

var goroutineStartVar int

func demoGoroutineStart() {
	fmt.Println("=== 1. Goroutine 启动的 happens-before ===")

	goroutineStartVar = 123 // A

	done := make(chan struct{})
	go func() {
		fmt.Printf("goroutine 中读到: %d\n", goroutineStartVar) // B，一定能看到 A 的写！
		close(done)
	}()

	<-done
	fmt.Println("go 语句 happens before goroutine 内的所有操作，所以一定能看到赋值")
	fmt.Println()
}

// ============================================================
// 2. Channel 发送：发送 happens before 接收完成
// ============================================================

var chanSendVar int

func demoChannelSend() {
	fmt.Println("=== 2. Channel 发送的 happens-before ===")

	ch := make(chan int, 1)

	go func() {
		chanSendVar = 456 // A
		ch <- 1           // 发送
	}()

	<-ch // 接收完成
	fmt.Printf("读到 chanSendVar = %d（一定是 456）\n", chanSendVar) // B，一定能看到 A
	fmt.Println("Channel 发送 happens before 接收完成，所以写一定可见")
	fmt.Println()
}

// ============================================================
// 3. 没有同步的情况下：完全不确定！
// ============================================================

var a, b int

func demoNoSync() {
	fmt.Println("=== 3. 没有同步的情况：结果完全不确定！ ===")

	go func() {
		a = 1 // A
		b = 2 // B，可能被 CPU 乱序！可能先执行 B 再执行 A！
	}()

	go func() {
		fmt.Printf("读到 b=%d, a=%d\n", b, a)
		// 可能出现的情况：
		// 0, 0   - 什么都没读到
		// 1, 2   - 都读到了
		// 0, 2   - 只读到了 b！（乱序的结果！）
		// 2, 1   - 不可能（Go 保证单 goroutine 内顺序一致）
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Println("⚠️  没有同步的情况下，什么都可能发生！")
	fmt.Println("CPU 可能乱序执行，缓存可能不刷新")
	fmt.Println()
}

// ============================================================
// 4. 用 flag 控制循环退出是错的！
// ============================================================

var stopFlag bool

func demoWrongStopFlag() {
	fmt.Println("=== 4. 错误示范：用 flag 控制 goroutine 退出 ===")

	go func() {
		count := 0
		for !stopFlag { // ❌ 没有同步！可能永远看不到 stopFlag = true！
			count++
		}
		fmt.Printf("循环执行了 %d 次后退出\n", count)
	}()

	time.Sleep(100 * time.Millisecond)
	stopFlag = true // ❌ 没有同步！上面的 goroutine 可能永远看不到！
	time.Sleep(100 * time.Millisecond)

	fmt.Println("⚠️  goroutine 可能永远循环不退出！")
	fmt.Println("💡 CPU 缓存了 stopFlag，或者编译器优化把循环变成 for {}")
	fmt.Println("💡 正确做法：用 channel 或 context 控制退出")
	fmt.Println()
}

// ============================================================
// 5. Mutex：Unlock happens before 下一次 Lock
// ============================================================

var mutexVar int

func demoMutex() {
	fmt.Println("=== 5. Mutex：Unlock happens before 下一次 Lock ===")

	var mu sync.Mutex

	mu.Lock()
	go func() {
		mutexVar = 789 // A
		mu.Unlock()    // Unlock
	}()

	mu.Lock() // B，下一次 Lock
	fmt.Printf("读到 mutexVar = %d（一定是 789）\n", mutexVar)
	mu.Unlock()

	fmt.Println("Unlock happens before 后续的 Lock，所以写一定可见")
	fmt.Println()
}

// ============================================================
// 6. Once：f() 返回 happens before 所有 Do 返回
// ============================================================

var onceVar int

func demoOnce() {
	fmt.Println("=== 6. Once：f() 返回 happens before 所有 Do 返回 ===")

	var once sync.Once

	initFunc := func() {
		onceVar = 1000
		fmt.Println("initFunc 执行了")
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			once.Do(initFunc)
			fmt.Printf("Goroutine %d 读到 onceVar = %d\n", id, onceVar)
		}(i)
	}

	wg.Wait()
	fmt.Println("💡 所有 goroutine 读到的 onceVar 一定是 1000！")
	fmt.Println()
}

// ============================================================
// 7. 双检查锁（Double-Checked Locking）在 Go 里是错的！
// ============================================================

type Config struct {
	Version string
	MaxConn int
}

var configMu sync.Mutex
var config *Config

// ❌ 错误的双检查锁！Go 不保证！
func GetConfigWrong() *Config {
	if config == nil { // 第一次检查，无锁
		configMu.Lock()
		defer configMu.Unlock()
		if config == nil { // 第二次检查
			// ❌ 这个赋值可能被乱序！
			// 可能先把指针赋值了，但 Config 的字段还没初始化！
			config = &Config{Version: "1.0", MaxConn: 100}
		}
	}
	return config
}

// ✅ 正确做法：用 Once
var configOnce sync.Once

func GetConfigRight() *Config {
	configOnce.Do(func() {
		config = &Config{Version: "1.0", MaxConn: 100}
	})
	return config
}

func demoDoubleCheckLock() {
	fmt.Println("=== 7. 双检查锁（Double-Checked Locking）是错的！ ===")
	fmt.Println("❌ GetConfigWrong() 可能拿到指针非 nil，但字段都是零值！")
	fmt.Println("✅ GetConfigRight() 用 sync.Once，100% 安全！")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := GetConfigRight()
			fmt.Printf("Goroutine %d: Version=%s, MaxConn=%d\n", id, cfg.Version, cfg.MaxConn)
		}(i)
	}
	wg.Wait()

	fmt.Println("\n💡 Go 没有 volatile！不要写双检查锁！用 sync.Once！")
	fmt.Println()
}

// ============================================================
// main
// ============================================================

func main() {
	demoGoroutineStart()
	demoChannelSend()
	demoNoSync()
	demoWrongStopFlag()
	demoMutex()
	demoOnce()
	demoDoubleCheckLock()

	fmt.Println("✅ 所有示例运行完成！")
	fmt.Println("\n💡 总结：")
	fmt.Println("   1. 没有同步的话，什么都可能发生！")
	fmt.Println("   2. CPU 会乱序，编译器会重排，不要依赖直觉！")
	fmt.Println("   3. 只相信 Go 内存模型保证的那 6 种情况！")
	fmt.Println("   4. 用 channel、Mutex、Once，不要瞎搞！")
	fmt.Println("   5. 双检查锁是错的！用 sync.Once！")
}
