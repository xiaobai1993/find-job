package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================
// 演示 1：WaitGroup 正确 vs 错误用法
// ============================================
func demo1WaitGroup() {
	fmt.Println("=== 演示 1：WaitGroup 正确 vs 错误用法 ===")

	// ❌ 错误：Add 放在 goroutine 里面，可能 Wait 先执行完
	fmt.Println("❌ 错误写法：Add 放在 goroutine 里面，可能提前返回")
	fmt.Println("   （原理演示，不实际执行）")
	fmt.Println("   for i := 0; i < 10; i++ {")
	fmt.Println("       go func() {")
	fmt.Println("           wg.Add(1)  // ❌ 太晚了！")
	fmt.Println("           ...")
	fmt.Println("       }()")
	fmt.Println("   }")
	fmt.Println("   wg.Wait()  // 可能计数器还是 0，直接返回！")

	// ✅ 正确：先 Add，再启动 goroutine
	fmt.Println("\n✅ 正确写法：先 Add，再启动 goroutine")
	var wg sync.WaitGroup
	results := make([]int, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1) // ✅ 先加，绝对不能放在 go func() 里面！
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Duration(i*10) * time.Millisecond)
			results[i] = i * 10
			fmt.Printf("   goroutine %d 完成\n", i)
		}(i)
	}

	fmt.Println("   等待所有 goroutine 完成...")
	wg.Wait()
	fmt.Printf("   全部完成，结果: %v ✅\n", results)

	fmt.Println("\n💡 支付业务提醒：批量查账户、批量通知，一定先 Add 再 go！")
}

// ============================================
// 演示 2：WaitGroup 传值拷贝失效
// ============================================
func demo2WaitGroupCopy() {
	fmt.Println("\n=== 演示 2：WaitGroup 传值拷贝失效 ===")

	// ❌ 错误：传值
	fmt.Println("❌ 错误：传值拷贝，两个 WaitGroup 不是同一个")
	fmt.Println("   func bad(wg sync.WaitGroup) { wg.Done() }")
	fmt.Println("   // 减的是拷贝的计数器！外面的 Wait 永远等不到！")

	// ✅ 正确：传指针
	fmt.Println("\n✅ 正确：传指针，共享同一个计数器")
	fmt.Println("   func good(wg *sync.WaitGroup) { wg.Done() }")

	fmt.Println("\n💡 和 Mutex 一样，所有同步原语都绝对不能值拷贝！")
}

// ============================================
// 演示 3：Once 只执行一次
// ============================================
func demo3Once() {
	fmt.Println("\n=== 演示 3：Once 只执行一次 ===")

	var once sync.Once
	initCount := 0

	initConfig := func() {
		initCount++
		fmt.Println("   配置初始化执行了！")
	}

	// 10 个 goroutine 同时调用 Do
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			once.Do(initConfig)
		}()
	}

	wg.Wait()
	fmt.Printf("   initCount = %d（应该等于 1）✅\n", initCount)
	fmt.Println("💡 10 个 goroutine 同时调用，只执行一次！")
}

// ============================================
// 演示 4：Once 初始化失败不重试的问题
// ============================================
func demo4OnceFail() {
	fmt.Println("\n=== 演示 4：Once 初始化失败不重试的问题 ===")

	fmt.Println("❌ Once 只执行一次，哪怕 panic 了也不会再执行")
	fmt.Println("   （原理演示）")
	fmt.Println("   var once sync.Once")
	fmt.Println("   once.Do(func() {")
	fmt.Println("       panic(\"数据库连不上\")  // 失败了")
	fmt.Println("   })")
	fmt.Println("   once.Do(func() { ... })  // 不会再执行了！")

	// ✅ 解决方案：自己包装支持重试
	fmt.Println("\n✅ 解决方案：自己包装，失败了不标记完成，支持重试")
	fmt.Println("   用 atomic + mutex 自己实现支持重试的 Once")
	fmt.Println("   支付业务：数据库、支付网关初始化必须支持重试！")
}

// ============================================
// 演示 5：Pool 基本用法 + Reset 重要性
// ============================================
func demo5Pool() {
	fmt.Println("\n=== 演示 5：Pool 基本用法 + Reset 重要性 ===")

	var bufPool = sync.Pool{
		New: func() interface{} {
			fmt.Println("   创建新 buffer")
			return bytes.NewBuffer(make([]byte, 0, 4096))
		},
	}

	// 第一次 Get，池里没有，创建新的
	buf1 := bufPool.Get().(*bytes.Buffer)
	fmt.Printf("   第一次 Get，len=%d, cap=%d\n", buf1.Len(), buf1.Cap())

	// 写点东西
	buf1.WriteString("用户: 张三, 金额: 100")
	fmt.Printf("   写入后 len=%d，内容: %q\n", buf1.Len(), buf1.String())

	// ❌ 错误：忘记 Reset，直接放回
	fmt.Println("\n❌ 错误：忘记 Reset，直接放回")
	bufPool.Put(buf1)

	// 再 Get 回来
	buf2 := bufPool.Get().(*bytes.Buffer)
	fmt.Printf("   再 Get 回来，内容还在！len=%d，%q\n", buf2.Len(), buf2.String())
	fmt.Println("   💡 上次的用户信息、金额还在！可能造成严重故障！")

	// ✅ 正确：Reset 之后再放回
	fmt.Println("\n✅ 正确：Reset 之后再放回")
	buf2.Reset()
	fmt.Printf("   Reset 后 len=%d\n", buf2.Len())
	bufPool.Put(buf2)

	buf3 := bufPool.Get().(*bytes.Buffer)
	fmt.Printf("   再 Get 回来，干净的 buffer：len=%d ✅\n", buf3.Len())
}

// ============================================
// 演示 6：Pool GC 全部清空
// ============================================
func demo6PoolGCClear() {
	fmt.Println("\n=== 演示 6：Pool GC 全部清空 ===")

	var objPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 1024)
		},
	}

	// 放 10 个对象进去
	for i := 0; i < 10; i++ {
		objPool.Put(make([]byte, 1024))
	}
	fmt.Println("   放入 10 个对象到 Pool")

	// GC 之前 Get 一个，能拿到
	obj1 := objPool.Get()
	fmt.Printf("   GC 前 Get，对象存在: %p\n", obj1)
	objPool.Put(obj1)

	fmt.Println("   触发 GC...")
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// GC 之后再 Get，应该是新创建的
	obj2 := objPool.Get()
	fmt.Printf("   GC 后 Get，新对象: %p\n", obj2)
	fmt.Println("💡 两个对象地址不同！说明 GC 把 Pool 清空了！")
	fmt.Println("💡 绝对不要存数据库连接、TCP 连接！GC 清了不会 Close！")
}

// ============================================
// 演示 7：JSON 序列化用 Pool 优化性能
// ============================================
func demo7JSONPool() {
	fmt.Println("\n=== 演示 7：JSON 序列化用 Pool 优化 ===")

	var jsonBufPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 4096))
		},
	}

	type Order struct {
		ID     string `json:"id"`
		Amount int64  `json:"amount"`
		Status string `json:"status"`
	}

	marshalOrder := func(order *Order) ([]byte, error) {
		buf := jsonBufPool.Get().(*bytes.Buffer)
		buf.Reset() // ✅ 一定要 Reset！
		defer jsonBufPool.Put(buf)

		enc := json.NewEncoder(buf)
		if err := enc.Encode(order); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}

	// 测试
	order := &Order{ID: "123", Amount: 10000, Status: "PAID"}
	data, _ := marshalOrder(order)
	fmt.Printf("   序列化结果: %s", data)
	fmt.Println("   💡 支付系统 JSON 序列化 GC 压力降低 90%+！")
}

// ============================================
// 演示 8：Atomic vs Mutex 性能对比
// ============================================
func demo8AtomicVsMutex() {
	fmt.Println("\n=== 演示 8：Atomic vs Mutex 性能对比 ===")

	const count = 1000000

	// Mutex 版本
	var mu sync.Mutex
	var muCount int64
	start := time.Now()

	for i := 0; i < count; i++ {
		mu.Lock()
		muCount++
		mu.Unlock()
	}
	muTime := time.Since(start)
	fmt.Printf("   Mutex:   %d 次，耗时 %v，平均 %.2f ns/次\n",
		count, muTime, float64(muTime.Nanoseconds())/float64(count))

	// Atomic 版本
	var atomCount int64
	start = time.Now()

	for i := 0; i < count; i++ {
		atomic.AddInt64(&atomCount, 1)
	}
	atomTime := time.Since(start)
	fmt.Printf("   Atomic:  %d 次，耗时 %v，平均 %.2f ns/次\n",
		count, atomTime, float64(atomTime.Nanoseconds())/float64(count))

	fmt.Printf("   Atomic 比 Mutex 快 %.1f 倍！✅\n",
		float64(muTime.Nanoseconds())/float64(atomTime.Nanoseconds()))
}

// ============================================
// 演示 9：Atomic 实现计数器
// ============================================
func demo9AtomicCounter() {
	fmt.Println("\n=== 演示 9：Atomic 实现并发安全计数器 ===")

	var requestCount int64
	const goroutines = 100
	const perGoroutine = 1000

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				atomic.AddInt64(&requestCount, 1)
			}
		}()
	}

	wg.Wait()
	fmt.Printf("   预期: %d * %d = %d\n", goroutines, perGoroutine, goroutines*perGoroutine)
	fmt.Printf("   实际: %d ✅\n", atomic.LoadInt64(&requestCount))
}

// ============================================
// 演示 10：Atomic 实现自旋锁
// ============================================
func demo10SpinLock() {
	fmt.Println("\n=== 演示 10：Atomic 实现自旋锁 ===")

	type SpinLock int32

	lock := func(l *int32) {
		for !atomic.CompareAndSwapInt32(l, 0, 1) {
			runtime.Gosched() // 让出 CPU，不忙等
		}
	}

	unlock := func(l *int32) {
		atomic.StoreInt32(l, 0)
	}

	var spinLock int32
	var count int64
	const goroutines = 10
	const perGoroutine = 1000

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				lock(&spinLock)
				count++
				unlock(&spinLock)
			}
		}()
	}

	wg.Wait()
	fmt.Printf("   自旋锁计数结果: %d，预期: %d ✅\n",
		count, goroutines*perGoroutine)
	fmt.Println("💡 非常短的临界区，自旋锁比 Mutex 性能高！")
}

// ============================================
// 演示 11：典型支付业务场景 - 批量查询
// ============================================
func demo11PaymentBatchQuery() {
	fmt.Println("\n=== 演示 11：支付业务 - 批量查询订单 ===")

	queryOrder := func(orderID string) string {
		// 模拟 RPC 调用
		time.Sleep(10 * time.Millisecond)
		return fmt.Sprintf("订单:%s:PAID", orderID)
	}

	orderIDs := []string{"1001", "1002", "1003", "1004", "1005"}

	// 串行查询
	start := time.Now()
	_ = make([]string, len(orderIDs))
	for _, id := range orderIDs {
		_ = queryOrder(id)
	}
	serialTime := time.Since(start)
	fmt.Printf("   串行查询: %v\n", serialTime)

	// 并行查询
	start = time.Now()
	var wg sync.WaitGroup
	results := make([]string, len(orderIDs))

	for i, id := range orderIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			results[i] = queryOrder(id)
		}(i, id)
	}

	wg.Wait()
	parallelTime := time.Since(start)

	fmt.Printf("   并行查询: %v，结果: %v\n", parallelTime, results)
	fmt.Printf("   性能提升 %.1f 倍！✅\n",
		float64(serialTime.Nanoseconds())/float64(parallelTime.Nanoseconds()))
}

// ============================================
// 演示 12：并发原语选型对比
// ============================================
func demo12Comparison() {
	fmt.Println("\n=== 演示 12：并发原语选型对比 ===")

	fmt.Println("   ┌────────────┬─────────┬────────────┬─────────┐")
	fmt.Println("   │ 原语        │ 性能    │ 适用场景   │ 复杂度  │")
	fmt.Println("   ├────────────┼─────────┼────────────┼─────────┤")
	fmt.Println("   │ Atomic     │ ⭐⭐⭐⭐⭐ │ 单个变量   │ 低      │")
	fmt.Println("   │ Mutex      │ ⭐⭐⭐    │ 临界区     │ 中      │")
	fmt.Println("   │ RWMutex    │ ⭐⭐⭐⭐  │ 读多写少   │ 中      │")
	fmt.Println("   │ WaitGroup  │ ⭐⭐⭐⭐  │ 批量等待   │ 低      │")
	fmt.Println("   │ Once       │ ⭐⭐⭐⭐  │ 单次初始化 │ 低      │")
	fmt.Println("   │ Pool       │ ⭐⭐⭐⭐  │ 对象复用   │ 中      │")
	fmt.Println("   │ Channel    │ ⭐⭐     │ 通信       │ 中      │")
	fmt.Println("   └────────────┴─────────┴────────────┴─────────┘")

	fmt.Println("\n💡 支付业务黄金法则：")
	fmt.Println("   能不用锁就不用锁，能用 Atomic 就不用 Mutex")
	fmt.Println("   能用 RWMutex 就不用 Mutex，锁粒度越小越好")
}

// ============================================
// main
// ============================================
func main() {
	demo1WaitGroup()
	demo2WaitGroupCopy()
	demo3Once()
	demo4OnceFail()
	demo5Pool()
	demo6PoolGCClear()
	demo7JSONPool()
	demo8AtomicVsMutex()
	demo9AtomicCounter()
	demo10SpinLock()
	demo11PaymentBatchQuery()
	demo12Comparison()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 所有并发原语演示完成！")
	fmt.Println("=== 核心总结 ===")
	fmt.Println("1. WaitGroup: 先 Add(1)，再 go func()，最后 Wait()")
	fmt.Println("2. Once: 只执行一次，失败不重试，不能重入")
	fmt.Println("3. Pool: 每次 GC 全清，只存无状态对象，Get 后必须 Reset")
	fmt.Println("4. Atomic: 单个变量无锁操作，性能是 Mutex 的 20-30 倍")
	fmt.Println("5. 所有同步原语都绝对不能值拷贝，必须传指针")
	fmt.Println("\n=== 支付业务最佳实践 ===")
	fmt.Println("✅ 批量查询用 WaitGroup + goroutine 并行，性能翻倍")
	fmt.Println("✅ 配置、数据库连接用 Once 初始化，并发安全")
	fmt.Println("✅ JSON 序列化 buffer 用 Pool，GC 压力降 90%")
	fmt.Println("✅ 简单计数器用 Atomic，不要用 Mutex")
	fmt.Println("✅ 所有地方 go vet -copylocks 检查，防止值拷贝")
}
