package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// ============================================
// 演示 1: sync.Once - 失败了也不重试
// ============================================

func demoOnceFailNoRetry() {
	fmt.Println("=== 演示 1: sync.Once - 失败了也不重试 ===")

	var once sync.Once
	var count int32

	// 模拟一个会失败的初始化
	initFunc := func() {
		atomic.AddInt32(&count, 1)
		fmt.Printf("   执行初始化，第 %d 次尝试\n", count)
		panic("数据库连接失败")
	}

	// 第一次调用 - panic
	func() {
		defer func() {
			if p := recover(); p != nil {
				fmt.Printf("   第一次调用捕获 panic: %v\n", p)
			}
		}()
		once.Do(initFunc)
	}()

	// 第二次调用 - 不会再执行了！
	func() {
		defer func() {
			if p := recover(); p != nil {
				fmt.Printf("   第二次调用捕获 panic: %v\n", p)
			} else {
				fmt.Println("   第二次调用：什么都没做！")
			}
		}()
		once.Do(initFunc)
	}()

	fmt.Printf("   💡 总共只执行了 %d 次，失败了也不重试！\n", count)
	fmt.Println("   💡 这是 Once 和 SingleFlight 最大的区别！")
}

// ============================================
// 演示 2: singleflight - 失败了下次还会重试
// ============================================

func demoSingleflightFailRetry() {
	fmt.Println("\n=== 演示 2: singleflight - 失败了下次还会重试 ===")

	var g singleflight.Group
	var count int32

	// 模拟一个会失败的数据库查询
	queryDB := func() (interface{}, error) {
		atomic.AddInt32(&count, 1)
		fmt.Printf("   查询数据库，第 %d 次尝试\n", count)
		return nil, errors.New("数据库超时")
	}

	// 第一次调用
	_, err, _ := g.Do("user:123", queryDB)
	fmt.Printf("   第一次调用结果: %v\n", err)

	// 第二次调用 - 还会执行！
	_, err, _ = g.Do("user:123", queryDB)
	fmt.Printf("   第二次调用结果: %v\n", err)

	// 第三次调用 - 还会执行！
	_, err, _ = g.Do("user:123", queryDB)
	fmt.Printf("   第三次调用结果: %v\n", err)

	fmt.Printf("   💡 总共执行了 %d 次，失败了会重试！\n", count)
}

// ============================================
// 演示 3: singleflight 合并并发请求
// ============================================

func demoSingleflightMergeRequests() {
	fmt.Println("\n=== 演示 3: singleflight 合并并发请求 ===")

	var g singleflight.Group
	var count int32

	// 模拟慢查询
	slowQuery := func() (interface{}, error) {
		atomic.AddInt32(&count, 1)
		fmt.Printf("   🔍 真正执行查询，第 %d 次\n", count)
		time.Sleep(100 * time.Millisecond) // 模拟慢
		return "result", nil
	}

	// 10 个 goroutine 同时查同一个 key
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			result, err, _ := g.Do("user:123", slowQuery)
			fmt.Printf("   goroutine %d 拿到结果: %v, err: %v\n", id, result, err)
		}(i)
	}

	wg.Wait()
	fmt.Printf("   💡 10 个并发请求，只真正执行了 %d 次查询！\n", count)
	fmt.Println("   💡 这就是防止缓存击穿的原理！")
}

// ============================================
// 演示 4: Once 用于单例，SingleFlight 用于防击穿
// ============================================

func demoUseCase() {
	fmt.Println("\n=== 演示 4: 典型使用场景对比 ===")

	fmt.Println("   ✅ sync.Once 适用场景：")
	fmt.Println("      - 加载配置文件")
	fmt.Println("      - 初始化数据库连接池")
	fmt.Println("      - 单例模式")
	fmt.Println("      - 只需要做一次的初始化")

	fmt.Println("\n   ✅ singleflight 适用场景：")
	fmt.Println("      - 防止缓存击穿")
	fmt.Println("      - 高并发下合并重复请求")
	fmt.Println("      - 外部 API 调用去重")
	fmt.Println("      - 数据库查询合并")
}

// ============================================
// 面试高频问答
// ============================================

func printInterviewQuestions() {
	fmt.Println("\n=== 面试高频问答 ===")

	fmt.Println("\n   Q: Once 和 SingleFlight 都能让函数只执行一次？")
	fmt.Println("   A: ❌ 错！")
	fmt.Println("      - Once：全局意义上的一次，这辈子只执行一次")
	fmt.Println("      - SingleFlight：同一时间点的一次，不同时间还会执行")

	fmt.Println("\n   Q: 初始化失败了怎么办？")
	fmt.Println("   A:")
	fmt.Println("      - Once：失败了就失败了，永远不会重试")
	fmt.Println("      - SingleFlight：失败了下次还会重试")

	fmt.Println("\n   Q: 能缓存结果吗？")
	fmt.Println("   A:")
	fmt.Println("      - Once：不缓存，需要自己存变量")
	fmt.Println("      - SingleFlight：同一批并发调用共享结果，不同批次不共享")

	fmt.Println("\n   Q: singleflight 的 Do 方法第三个返回值是什么？")
	fmt.Println("   A: shared bool，表示结果是否被多个调用者共享")
}

func main() {
	demoOnceFailNoRetry()
	demoSingleflightFailRetry()
	demoSingleflightMergeRequests()
	demoUseCase()
	printInterviewQuestions()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 总结：")
	fmt.Println("   sync.Once     = 这辈子只做一次")
	fmt.Println("   singleflight  = 同一时间只做一次")
	fmt.Println("   完全不是一个东西！")
}
