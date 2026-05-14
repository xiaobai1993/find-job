package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ============================================
// 演示 1：解锁未加锁的 Mutex 直接 panic
// ============================================
func demo1UnlockPanic() {
	fmt.Println("=== 演示 1：解锁未加锁的 Mutex 直接 panic ===")

	fmt.Println("❌ 解锁未加锁的锁是 fatal error，不是普通 panic，无法 recover！")
	fmt.Println("   源码位置：sync/mutex.go unlockSlow")
	fmt.Println("   func (m *Mutex) unlockSlow(...) {")
	fmt.Println("       if (m.state&mutexLocked) == 0 {")
	fmt.Println("           throw(\"sync: unlock of unlocked mutex\")  // fatal error!")
	fmt.Println("       }")
	fmt.Println("   }")
	fmt.Println("💡 支付业务铁律：加解锁一定要配对，不要跨函数传锁！")
	fmt.Println("💡 代码 Review 重点检查：mu.Unlock() 之前是不是一定 Lock() 过")
}

// ============================================
// 演示 2：Mutex 是值类型，传参拷贝导致失效
// ============================================
func demo2ValueCopy() {
	fmt.Println("\n=== 演示 2：Mutex 是值类型，传参拷贝导致失效 ===")

	var wg sync.WaitGroup
	var mu sync.Mutex
	counter := 0

	// ❌ 错误：传值拷贝，每个 goroutine 用自己的锁
	fmt.Println("❌ 错误写法：传值拷贝，两个 goroutine 用不同的锁")
	wg.Add(2)

	// G1: 传值拷贝 mutex
	go func(mu sync.Mutex, id int) {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			mu.Lock()
			counter++
			mu.Unlock()
		}
		fmt.Printf("  goroutine %d 完成 (拷贝的锁)\n", id)
	}(mu, 1)

	// G2: 用原始的 mutex
	go func(id int) {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			mu.Lock()
			counter++
			mu.Unlock()
		}
		fmt.Printf("  goroutine %d 完成 (原始锁)\n", id)
	}(2)

	wg.Wait()
	fmt.Printf("  预期 counter = 2000，实际 counter = %d\n", counter)
	fmt.Println("  💡 因为拷贝了锁，两个 goroutine 根本没互斥！")

	// ✅ 正确：传指针
	fmt.Println("\n✅ 正确写法：传指针，共享同一个锁")
	counter = 0
	wg.Add(2)

	goodWorker := func(mu *sync.Mutex, id int) {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			mu.Lock()
			counter++
			mu.Unlock()
		}
		fmt.Printf("  goroutine %d 完成\n", id)
	}

	go goodWorker(&mu, 1)
	go goodWorker(&mu, 2)

	wg.Wait()
	fmt.Printf("  预期 counter = 2000，实际 counter = %d ✅\n", counter)
}

// ============================================
// 演示 3：死锁 - 循环等待
// ============================================
func demo3Deadlock() {
	fmt.Println("\n=== 演示 3：死锁 - 循环等待（注释了避免卡死） ===")

	fmt.Println("❌ 死锁场景 1：循环等待")
	fmt.Println("  goroutine A: lock A -> wait lock B")
	fmt.Println("  goroutine B: lock B -> wait lock A")
	fmt.Println("  结果：永远等不到，死锁！")

	fmt.Println("\n❌ 死锁场景 2：自己等自己")
	fmt.Println("  mu.Lock()")
	fmt.Println("  mu.Lock()  // 重入锁，Go 不支持！直接死锁")

	fmt.Println("\n✅ 避免死锁方法：")
	fmt.Println("  1. 所有 goroutine 按固定顺序加锁")
	fmt.Println("  2. 不要持有锁做 IO")
	fmt.Println("  3. 锁粒度越小越好")
	fmt.Println("  4. 使用 TryLock 设置超时（Go 1.18+）")
}

// ============================================
// 演示 4：锁粒度太大影响性能
// ============================================
func demo4LockGranularity() {
	fmt.Println("\n=== 演示 4：锁粒度太大影响性能 ===")

	var mu sync.Mutex
	const workers = 10
	const tasks = 100

	// ❌ 不好：整个函数都加锁
	fmt.Println("❌ 不好：整个函数都加锁，串行执行")
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			// 模拟业务：计算 + IO
			time.Sleep(time.Duration(tasks/workers) * time.Millisecond)
		}()
	}
	wg.Wait()
	fmt.Printf("  耗时: %v (串行)\n", time.Since(start))

	// ✅ 好：只保护临界区
	fmt.Println("\n✅ 好：只保护临界区，并行执行")
	start = time.Now()
	wg.Add(workers)
	result := 0

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			// IO 操作并行做，不需要锁
			time.Sleep(time.Duration(tasks/workers) * time.Millisecond)

			// 只在更新共享变量时加锁
			mu.Lock()
			result++
			mu.Unlock()
		}()
	}
	wg.Wait()
	fmt.Printf("  耗时: %v (并行，只锁临界区) ✅\n", time.Since(start))
}

// ============================================
// 演示 5：RWMutex 读多写少场景性能对比
// ============================================
func demo5RWMutex() {
	fmt.Println("\n=== 演示 5：RWMutex 读多写少场景性能对比 ===")

	const readers = 100
	const writers = 2
	const duration = 200 * time.Millisecond

	// Mutex 性能
	var mu sync.Mutex
	muCount := 0
	muStart := time.Now()
	var wg sync.WaitGroup

	// 写 goroutine
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(muStart) < duration {
				mu.Lock()
				muCount++
				mu.Unlock()
				runtime.Gosched()
			}
		}()
	}

	// 读 goroutine
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(muStart) < duration {
				mu.Lock()
				_ = muCount
				mu.Unlock()
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()
	fmt.Printf("Mutex: 写操作 %d 次，耗时 %v\n", muCount, duration)

	// RWMutex 性能
	var rwmu sync.RWMutex
	rwCount := 0
	rwStart := time.Now()

	// 写 goroutine
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(rwStart) < duration {
				rwmu.Lock()
				rwCount++
				rwmu.Unlock()
				runtime.Gosched()
			}
		}()
	}

	// 读 goroutine（读读不互斥！）
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Since(rwStart) < duration {
				rwmu.RLock()
				_ = rwCount
				rwmu.RUnlock()
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()
	fmt.Printf("RWMutex: 写操作 %d 次，耗时 %v ✅\n", rwCount, duration)
	fmt.Println("💡 RWMutex 读读不互斥，读多写少场景吞吐量高很多！")
	fmt.Println("💡 支付业务配置缓存、字典缓存一定要用 RWMutex！")
}

// ============================================
// 演示 6：自旋 vs 休眠 - 竞争不激烈时自旋更快
// ============================================
func demo6SpinVsSleep() {
	fmt.Println("\n=== 演示 6：自旋 vs 休眠 - 竞争不激烈时自旋更快 ===")

	var mu sync.Mutex
	const iterations = 10000

	start := time.Now()
	for i := 0; i < iterations; i++ {
		mu.Lock()
		// 非常短的临界区，刚释放马上又抢，大概率能自旋拿到
		_ = i
		mu.Unlock()
	}
	fmt.Printf("单 goroutine 加解锁 %d 次，耗时: %v\n", iterations, time.Since(start))
	fmt.Println("💡 没有竞争时，基本都是 CAS 直接成功，不需要进入休眠")
	fmt.Println("💡 多核 CPU 下，短临界区自旋非常高效！")
}

// ============================================
// 演示 7：Go 1.18+ TryLock
// ============================================
func demo7TryLock() {
	fmt.Println("\n=== 演示 7：Go 1.18+ TryLock ===")

	var mu sync.Mutex

	// 主线程先拿到锁
	mu.Lock()
	fmt.Println("主线程拿到了锁")

	// 另一个 goroutine TryLock
	success := make(chan bool)
	go func() {
		success <- mu.TryLock()
	}()

	ok := <-success
	if !ok {
		fmt.Println("✅ TryLock 返回 false，锁被别人拿着，没有阻塞")
	} else {
		mu.Unlock()
	}

	mu.Unlock()
	fmt.Println("💡 TryLock 适合非关键路径，拿不到就放弃，不阻塞")
}

// ============================================
// 演示 8：struct 包含 Mutex 不能值拷贝
// ============================================
func demo8StructWithMutex() {
	fmt.Println("\n=== 演示 8：struct 包含 Mutex 不能值拷贝 ===")

	type Account struct {
		mu      sync.Mutex
		Balance int64
	}

	// ❌ 错误：值拷贝 Account
	fmt.Println("❌ 错误：值拷贝包含 Mutex 的 struct")
	a := &Account{Balance: 100}

	// 用指针没问题
	a.mu.Lock()
	a.Balance += 50
	a.mu.Unlock()
	fmt.Printf("  用指针没问题，Balance = %d\n", a.Balance)

	fmt.Println("  ⚠️  go vet 会检查这种拷贝问题：")
	fmt.Println("    go vet -copylocks ./...")
	fmt.Println("  💡 支付业务的订单、账户结构体都有锁，绝对不能值传！")
}

// ============================================
// 演示 9：Lock 之后忘记 Unlock - 死锁
// ============================================
func demo9ForgetUnlock() {
	fmt.Println("\n=== 演示 9：Lock 之后忘记 Unlock - 死锁（演示原理） ===")

	fmt.Println("❌ 错误写法：中间 return 了没解锁")
	fmt.Println("  func bad() {")
	fmt.Println("      mu.Lock()")
	fmt.Println("      if err != nil {")
	fmt.Println("          return  // ❌ 直接 return，没解锁！")
	fmt.Println("      }")
	fmt.Println("      mu.Unlock()")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法：加锁之后立刻 defer 解锁")
	fmt.Println("  func good() {")
	fmt.Println("      mu.Lock()")
	fmt.Println("      defer mu.Unlock()  // ✅ 函数退出一定执行")
	fmt.Println("      if err != nil {")
	fmt.Println("          return  // 安全，defer 会解锁")
	fmt.Println("      }")
	fmt.Println("  }")

	fmt.Println("\n💡 支付业务铁律：mu.Lock() 下一行一定是 defer mu.Unlock()")
}

// ============================================
// 演示 10：饥饿模式模拟 - 等太久的 goroutine 优先
// ============================================
func demo10StarvationMode() {
	fmt.Println("\n=== 演示 10：饥饿模式模拟 ===")

	var mu sync.Mutex
	var wg sync.WaitGroup
	const goroutines = 5

	fmt.Printf("启动 %d 个 goroutine 同时抢锁...\n", goroutines)
	fmt.Println("观察：每个 goroutine 拿到锁的次数是否公平")

	stats := make([]int, goroutines)
	done := make(chan struct{})

	// 持续抢锁
	for i := 0; i < goroutines; i++ {
		id := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					mu.Lock()
					stats[id]++
					// 持有锁一段时间
					time.Sleep(10 * time.Microsecond)
					mu.Unlock()
					runtime.Gosched()
				}
			}
		}()
	}

	time.Sleep(500 * time.Millisecond)
	close(done)
	wg.Wait()

	fmt.Println("各 goroutine 拿到锁的次数:")
	min, max := stats[0], stats[0]
	for i, count := range stats {
		fmt.Printf("  G%d: %d 次\n", i, count)
		if count < min {
			min = count
		}
		if count > max {
			max = count
		}
	}

	fmt.Printf("最少: %d, 最多: %d, 差值: %d (越小越公平)\n", min, max, max-min)
	fmt.Println("💡 Go 1.9+ 有饥饿模式，不会出现某个 goroutine 永远拿不到锁")
	fmt.Println("💡 等待超过 1ms 就进入饥饿模式，按 FIFO 排队")
}

// ============================================
// 支付业务综合案例：账户转账
// ============================================
func demo11PaymentTransfer() {
	fmt.Println("\n=== 支付业务综合案例：账户转账 ===")

	type Account struct {
		ID      string
		Balance int64
		mu      sync.Mutex
	}

	transfer := func(from, to *Account, amount int64) bool {
		// ✅ 按 ID 顺序加锁，避免循环等待死锁！
		if from.ID < to.ID {
			from.mu.Lock()
			to.mu.Lock()
		} else {
			to.mu.Lock()
			from.mu.Lock()
		}
		defer from.mu.Unlock()
		defer to.mu.Unlock()

		if from.Balance < amount {
			return false
		}

		from.Balance -= amount
		to.Balance += amount
		return true
	}

	a := &Account{ID: "A", Balance: 1000}
	b := &Account{ID: "B", Balance: 1000}

	var wg sync.WaitGroup
	const transfers = 100
	success, fail := 0, 0

	wg.Add(2)
	// A -> B
	go func() {
		defer wg.Done()
		for i := 0; i < transfers; i++ {
			if transfer(a, b, 1) {
				success++
			} else {
				fail++
			}
		}
	}()

	// B -> A
	go func() {
		defer wg.Done()
		for i := 0; i < transfers; i++ {
			if transfer(b, a, 1) {
				success++
			} else {
				fail++
			}
		}
	}()

	wg.Wait()

	fmt.Printf("  转账成功: %d 次，失败: %d 次\n", success, fail)
	fmt.Printf("  最终 A 余额: %d, B 余额: %d\n", a.Balance, b.Balance)
	fmt.Println("  💡 按固定顺序加锁，不会死锁！")
	fmt.Println("  💡 支付业务所有锁一定要有明确的加锁顺序！")
}

// ============================================
// main
// ============================================
func main() {
	demo1UnlockPanic()
	demo2ValueCopy()
	demo3Deadlock()
	demo4LockGranularity()
	demo5RWMutex()
	demo6SpinVsSleep()
	demo7TryLock()
	demo8StructWithMutex()
	demo9ForgetUnlock()
	demo10StarvationMode()
	demo11PaymentTransfer()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 所有 Mutex 演示完成！")
	fmt.Println("=== Mutex 核心总结 ===")
	fmt.Println("1. Mutex 是值类型，绝对不能拷贝，必须传指针")
	fmt.Println("2. mu.Lock() 下一行一定是 defer mu.Unlock()")
	fmt.Println("3. 解锁未加锁的锁直接 panic，注意配对")
	fmt.Println("4. 锁粒度越小越好，不要在锁里做 IO")
	fmt.Println("5. 多个锁按固定顺序加，避免循环等待死锁")
	fmt.Println("6. 读多写少用 RWMutex，读读不互斥")
	fmt.Println("7. Go 1.9+ 有饥饿模式，保证公平性")
	fmt.Println("8. struct 包含 Mutex 也不能值拷贝，用 go vet 检查")
	fmt.Println("\n=== 支付业务铁律 ===")
	fmt.Println("✅ 账户、订单锁一定按 ID 顺序加，防止死锁")
	fmt.Println("✅ 加解锁一定在同一个函数，不要跨函数传锁")
	fmt.Println("✅ goroutine 入口一定要加 defer recover")
	fmt.Println("✅ go vet -copylocks 一定要过 CI")
}
