//go:build go1.21
// +build go1.21

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"sync"
	"time"
)

// ==================== OnceFunc/OnceValue/OnceValues ====================

func demoOnceFunc() {
	fmt.Println("\n=== OnceFunc 演示 ===")

	var initLog = sync.OnceFunc(func() {
		fmt.Println("   🚀 日志系统初始化（只执行一次）")
	})

	initLog() // 执行
	initLog() // 不执行
	initLog() // 不执行
}

func demoOnceValue() {
	fmt.Println("\n=== OnceValue 演示 ===")

	type Config struct {
		MaxConn int
		Timeout time.Duration
	}

	var loadConfig = sync.OnceValue(func() *Config {
		fmt.Println("   📂 加载配置文件（只执行一次）")
		time.Sleep(100 * time.Millisecond)
		return &Config{MaxConn: 100, Timeout: 30 * time.Second}
	})

	// 多个 goroutine 同时调用
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := loadConfig()
			fmt.Printf("   goroutine %d 拿到配置: %+v\n", id, cfg)
		}(i)
	}
	wg.Wait()
}

func demoOnceValuePanic() {
	fmt.Println("\n=== OnceValue panic 缓存演示 ===")

	var initFail = sync.OnceValue(func() int {
		fmt.Println("   ❌ 初始化失败")
		panic(errors.New("database connection failed"))
	})

	// 第一次调用 panic
	func() {
		defer func() {
			if p := recover(); p != nil {
				fmt.Println("   第一次捕获 panic:", p)
			}
		}()
		initFail()
	}()

	// 第二次调用还会 panic！
	func() {
		defer func() {
			if p := recover(); p != nil {
				fmt.Println("   第二次还是 panic:", p)
			}
		}()
		initFail()
	}()

	fmt.Println("   💡 panic 会被缓存！每次调用都会 panic")
}

// ==================== slog 结构化日志 ====================

func demoSlog() {
	fmt.Println("\n=== slog 演示 ===")

	// JSON 格式
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("server started",
		"port", 8080,
		"env", "production",
	)
}

// ==================== min/max/clear ====================

func demoBuiltin() {
	fmt.Println("\n=== min/max/clear 演示 ===")

	// min/max
	fmt.Printf("   min(1, 5, 3) = %v\n", min(1, 5, 3))
	fmt.Printf("   max(1.5, 2.5, 0.5) = %v\n", max(1.5, 2.5, 0.5))

	// clear
	s := []int{1, 2, 3}
	clear(s)
	fmt.Printf("   clear([]int{1,2,3}) = %v\n", s) // [0 0 0]

	m := map[string]int{"a": 1, "b": 2}
	clear(m)
	fmt.Printf("   clear(map) = %v\n", m) // map[]
}

// ==================== slices/maps ====================

func demoSlicesMaps() {
	fmt.Println("\n=== slices/maps 包演示 ===")

	s := []int{3, 1, 4, 1, 5}

	// 排序
	slices.Sort(s)
	fmt.Println("   排序后:", s)

	// 包含
	fmt.Println("   Contains 4:", slices.Contains(s, 4))

	// 比较
	s2 := []int{1, 1, 3, 4, 5}
	fmt.Println("   Equal:", slices.Equal(s, s2))

	// maps
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"a": 1, "b": 2}
	fmt.Println("   maps.Equal:", maps.Equal(m1, m2))
}

// ==================== main ====================

func main() {
	fmt.Println("=== Go 1.21 新特性演示 ===")

	demoOnceFunc()
	demoOnceValue()
	demoOnceValuePanic()
	demoBuiltin()
	demoSlicesMaps()
	// demoSlog()  // 注释掉避免输出太多
}
