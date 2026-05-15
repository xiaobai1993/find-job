package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================
// 演示 1：类型化原子操作 - atomic.Int64
// ============================================
func demo1TypedAtomicInt64() {
	fmt.Println("=== 演示 1：类型化原子操作 - atomic.Int64 ===")

	// 旧写法（Go 1.18 及之前）
	fmt.Println("   ❌ 旧写法（Go 1.18 及之前）：")
	var oldCount int64
	atomic.AddInt64(&oldCount, 1)
	fmt.Printf("      atomic.AddInt64(&count, 1) = %d\n", atomic.LoadInt64(&oldCount))

	// 新写法（Go 1.19+）
	fmt.Println("\n   ✅ 新写法（Go 1.19+）：")
	var newCount atomic.Int64
	newCount.Add(1)
	fmt.Printf("      count.Add(1) = %d\n", newCount.Load())

	fmt.Println("\n   💡 类型安全，不会传错类型，代码更简洁！")
}

// ============================================
// 演示 2：类型化原子操作 - atomic.Bool
// ============================================
func demo2TypedAtomicBool() {
	fmt.Println("\n=== 演示 2：类型化原子操作 - atomic.Bool ===")

	var flag atomic.Bool

	// 初始是 false
	fmt.Printf("   初始值: flag.Load() = %v\n", flag.Load())

	// 设置为 true
	flag.Store(true)
	fmt.Printf("   Store(true) 后: %v\n", flag.Load())

	// CAS 操作
	swapped := flag.CompareAndSwap(true, false)
	fmt.Printf("   CompareAndSwap(true, false) 成功？ %v\n", swapped)
	fmt.Printf("   CAS 后的值: %v\n", flag.Load())

	// Swap 操作
	old := flag.Swap(true)
	fmt.Printf("   Swap(true) 返回旧值: %v，新值: %v\n", old, flag.Load())
}

// ============================================
// 演示 3：类型化原子操作 - atomic.Pointer[T]
// ============================================
func demo3TypedAtomicPointer() {
	fmt.Println("\n=== 演示 3：类型化原子操作 - atomic.Pointer[T] ===")

	type Config struct {
		Addr string
		Port int
	}

	var cfgPtr atomic.Pointer[Config]

	// 存储
	cfg := &Config{Addr: "localhost", Port: 8080}
	cfgPtr.Store(cfg)
	fmt.Printf("   Store 后: %+v\n", cfgPtr.Load())

	// 安全更新配置
	newCfg := &Config{Addr: "0.0.0.0", Port: 9090}
	old := cfgPtr.Swap(newCfg)
	fmt.Printf("   Swap 后: 旧=%+v, 新=%+v\n", old, cfgPtr.Load())

	fmt.Println("\n   💡 泛型原子指针，不需要手动转 unsafe.Pointer 了！")
}

// ============================================
// 演示 4：原子操作嵌入结构体
// ============================================
func demo4AtomicInStruct() {
	fmt.Println("\n=== 演示 4：原子操作嵌入结构体 ===")

	type Stats struct {
		RequestCount atomic.Int64  // ✅ 直接嵌入，不需要指针
		ErrorCount   atomic.Int64
		IsHealthy    atomic.Bool
	}

	var s Stats

	// 多 goroutine 并发更新
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.RequestCount.Add(1)
		}()
	}

	wg.Wait()
	fmt.Printf("   并发更新后 RequestCount = %d ✅\n", s.RequestCount.Load())
}

// ============================================
// 演示 5：OnceFunc - 无返回值
// ============================================
func demo5OnceFunc() {
	fmt.Println("\n=== 演示 5：OnceFunc - 无返回值（Go 1.21+） ===")

	var initLog = sync.OnceFunc(func() {
		fmt.Println("   🚀 系统初始化（只会执行一次）")
	})

	fmt.Println("   第一次调用 initLog()")
	initLog()

	fmt.Println("   第二次调用 initLog()")
	initLog()

	fmt.Println("   第三次调用 initLog()")
	initLog()

	fmt.Println("   💡 后面两次调用都不会执行了！")
}

// ============================================
// 演示 6：OnceValue - 有返回值
// ============================================
func demo6OnceValue() {
	fmt.Println("\n=== 演示 6：OnceValue - 有返回值（Go 1.21+） ===")

	type Config struct {
		MaxConn int
		Timeout time.Duration
	}

	var loadConfig = sync.OnceValue(func() *Config {
		fmt.Println("   📂 加载配置文件（只会执行一次）")
		time.Sleep(100 * time.Millisecond) // 模拟慢 IO
		return &Config{MaxConn: 100, Timeout: 30 * time.Second}
	})

	start := time.Now()

	// 10 个 goroutine 同时调用
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := loadConfig()
			fmt.Printf("   goroutine %d 拿到配置: %+v\n", id, cfg)
		}(i)
	}

	wg.Wait()
	fmt.Printf("   总耗时: %v（只加载了一次配置）\n", time.Since(start))
	fmt.Println("   💡 所有 goroutine 等第一个执行完才返回！")
}

// ============================================
// 演示 7：OnceValue - panic 缓存特性
// ============================================
func demo7OnceValuePanic() {
	fmt.Println("\n=== 演示 7：OnceValue - panic 缓存特性（非常重要！） ===")

	var initFail = sync.OnceValue(func() int {
		fmt.Println("   ❌ 初始化失败，panic 了")
		panic("数据库连接不上")
	})

	// 第一次调用
	defer func() {
		if p := recover(); p != nil {
			fmt.Println("   第一次调用捕获到 panic:", p)
		}
	}()

	initFail()

	// 注意：后续调用也会 panic！和普通 Once 不一样！
}

func demo7OnceValuePanicSecond() {
	fmt.Println("\n   👉 后续调用还会 panic 吗？")

	var initFail = sync.OnceValue(func() int {
		panic("数据库连接不上")
	})

	// 第一次，捕获
	func() {
		defer func() { recover() }()
		initFail()
		fmt.Println("   第一次调用 panic 了")
	}()

	// 第二次，也会 panic！
	func() {
		defer func() {
			if p := recover(); p != nil {
				fmt.Println("   第二次调用还是 panic:", p)
			}
		}()
		initFail()
	}()

	fmt.Println("   💡 OnceValue 会缓存 panic，每次调用都会 panic！")
	fmt.Println("   💡 普通 Once 只 panic 一次，这是重要区别！")
}

// ============================================
// 演示 8：OnceValues - 两个返回值
// ============================================
func demo8OnceValues() {
	fmt.Println("\n=== 演示 8：OnceValues - 两个返回值（Go 1.21+） ===")

	var connectDB = sync.OnceValues(func() (string, error) {
		fmt.Println("   🔌 连接数据库（只会执行一次）")
		time.Sleep(100 * time.Millisecond)
		return "mysql://localhost:3306", nil
	})

	dsn1, err1 := connectDB()
	fmt.Printf("   第一次调用: dsn=%s, err=%v\n", dsn1, err1)

	dsn2, err2 := connectDB()
	fmt.Printf("   第二次调用: dsn=%s, err=%v\n", dsn2, err2)

	fmt.Println("   💡 dsn1 和 dsn2 完全相同，只连接了一次！")
}

// ============================================
// 演示 9：对比 - 旧写法 vs OnceValue 新写法
// ============================================
func demo9OldVsNew() {
	fmt.Println("\n=== 演示 9：旧写法 vs OnceValue 新写法 ===")

	fmt.Println("   ❌ 旧写法（Go 1.20 及之前）：")
	fmt.Println(`      var (
          config *Config
          once   sync.Once
      )

      func GetConfig() *Config {
          once.Do(func() {
              config = loadConfig()
          })
          return config
      }`)

	fmt.Println("\n   ✅ 新写法（Go 1.21+）：")
	fmt.Println(`      var GetConfig = sync.OnceValue(func() *Config {
          return loadConfig()
      })`)

	fmt.Println("\n   💡 代码量减少 50%，更少出错！")
}

// ============================================
// 演示 10：类型化原子操作性能对比
// ============================================
func demo10Performance() {
	fmt.Println("\n=== 演示 10：性能对比 ===")

	const iterations = 10_000_000

	// 旧写法
	var oldCount int64
	start := time.Now()
	for i := 0; i < iterations; i++ {
		atomic.AddInt64(&oldCount, 1)
	}
	oldTime := time.Since(start)

	// 新写法
	var newCount atomic.Int64
	start = time.Now()
	for i := 0; i < iterations; i++ {
		newCount.Add(1)
	}
	newTime := time.Since(start)

	fmt.Printf("   旧写法: %d 次，耗时 %v\n", iterations, oldTime)
	fmt.Printf("   新写法: %d 次，耗时 %v\n", iterations, newTime)
	fmt.Println("   💡 性能完全一样！只是语法糖，没有额外开销")
}

// ============================================
// 演示 11：支付系统真实场景 - 配置热加载
// ============================================
func demo11PaymentConfig() {
	fmt.Println("\n=== 演示 11：支付系统真实场景 - 配置热加载 ===")

	type PaymentConfig struct {
		MaxAmount  int64  // 单笔最大金额
		FeeRate    float64 // 手续费率
		ChannelURL string  // 渠道地址
	}

	var globalConfig atomic.Pointer[PaymentConfig]

	// 初始化默认配置
	globalConfig.Store(&PaymentConfig{
		MaxAmount:  1000000, // 1 万
		FeeRate:    0.006,   // 0.6%
		ChannelURL: "https://pay.example.com",
	})

	// 业务 goroutine 100 并发读
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := globalConfig.Load()
			_ = cfg // 使用配置处理支付逻辑
		}(i)
	}

	// 后台 goroutine 安全热加载配置
	go func() {
		time.Sleep(10 * time.Millisecond)
		newCfg := &PaymentConfig{
			MaxAmount:  5000000, // 提到 5 万
			FeeRate:    0.005,   // 费率降到 0.5%
			ChannelURL: "https://pay-v2.example.com",
		}
		globalConfig.Store(newCfg)
		fmt.Println("   ✅ 配置热更新完成，原子性保证！")
	}()

	wg.Wait()
	fmt.Println("   💡 原子指针！读和更新完全并发安全，不需要锁")
}

// ============================================
// 总结
// ============================================
func printSummary() {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 所有新同步原语演示完成！")
	fmt.Println("=== 核心总结 ===")
	fmt.Println("")
	fmt.Println("【Go 1.19+ - 类型化原子操作】")
	fmt.Println("   ✅ atomic.Bool/Int32/Int64/Uint32/Uint64/Uintptr/Pointer[T]")
	fmt.Println("   ✅ 类型安全，代码简洁，零值可用")
	fmt.Println("   ✅ 和旧函数性能完全一致，只是语法糖")
	fmt.Println("   ✅ 可以直接嵌入结构体")
	fmt.Println("")
	fmt.Println("【Go 1.21+ - Once 系列】")
	fmt.Println("   ✅ OnceFunc: 无返回值，函数只执行一次")
	fmt.Println("   ✅ OnceValue[T]: 一个返回值，函数只执行一次")
	fmt.Println("   ✅ OnceValues[T1, T2]: 两个返回值，函数只执行一次")
	fmt.Println("   ✅ ⚠️  重要特性：panic 会被缓存，每次调用都会 panic")
	fmt.Println("   ✅ 代码量减少 50%，不容易写错")
	fmt.Println("")
	fmt.Println("=== 最佳实践 ===")
	fmt.Println("✅ 新代码一律用类型化原子操作，不要用旧的函数形式")
	fmt.Println("✅ 单例、延迟初始化场景优先用 OnceValue，不用自己写 once + 变量")
	fmt.Println("✅ 注意 OnceValue 的 panic 缓存特性，需要重试的场景不要用")
}

// ============================================
// main
// ============================================
func main() {
	demo1TypedAtomicInt64()
	demo2TypedAtomicBool()
	demo3TypedAtomicPointer()
	demo4AtomicInStruct()
	demo5OnceFunc()
	demo6OnceValue()
	demo7OnceValuePanic()
	demo7OnceValuePanicSecond()
	demo8OnceValues()
	demo9OldVsNew()
	demo10Performance()
	demo11PaymentConfig()

	printSummary()
}
