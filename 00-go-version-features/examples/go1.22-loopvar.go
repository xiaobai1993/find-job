//go:build go1.22
// +build go1.22

package main

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
)

// ==================== 循环变量修复 ====================

func demoLoopVariableFix() {
	fmt.Println("=== Go 1.22 循环变量修复演示 ===")

	var wg sync.WaitGroup

	fmt.Println("\nGo 1.22+ 每次迭代创建新变量：")
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("   goroutine 打印 i = %d\n", i)
		}()
	}
	wg.Wait()

	fmt.Println("\n💡 Go 1.22 前会全部打印 3，现在正确打印 0,1,2！")
}

// ==================== range over int ====================

func demoRangeInt() {
	fmt.Println("\n=== range over int 演示 ===")

	fmt.Print("   range 5: ")
	for i := range 5 {
		fmt.Printf("%d ", i)
	}
	fmt.Println()

	fmt.Print("   不需要索引: ")
	count := 0
	for range 3 {
		count++
		fmt.Printf("%d ", count)
	}
	fmt.Println()
}

// ==================== math/rand/v2 ====================

func demoRandV2() {
	fmt.Println("\n=== math/rand/v2 演示 ===")

	// 不需要手动 Seed！
	fmt.Println("   IntN(100):", rand.IntN(100))
	fmt.Println("   Float64():", rand.Float64())
	fmt.Println("   Uint32():", rand.Uint32())

	// 随机选择
	n := rand.N[int64](1000)
	fmt.Println("   N[int64](1000):", n)
}

// ==================== HTTP 路由增强 ====================

func demoHTTPRouter() {
	fmt.Println("\n=== net/http 路由增强演示 ===")

	mux := http.NewServeMux()

	// 按方法匹配
	mux.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "GET /items")
	})

	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "POST /items")
	})

	// 路径参数
	mux.HandleFunc("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		fmt.Fprintf(w, "Item ID: %s\n", id)
	})

	fmt.Println("   支持方法匹配: GET /items, POST /items")
	fmt.Println("   支持路径参数: /items/{id}")
}

// ==================== main ====================

func main() {
	demoLoopVariableFix()
	demoRangeInt()
	demoRandV2()
	demoHTTPRouter()

	fmt.Println("\n✅ Go 1.22 所有演示完成")
}
