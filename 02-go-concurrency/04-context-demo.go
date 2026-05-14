package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================
// 演示 1：WithCancel 级联取消
// ============================================
func demo1WithCancel() {
	fmt.Println("=== 演示 1：WithCancel 级联取消 ===")

	parent, cancel := context.WithCancel(context.Background())

	// 启动 5 个 goroutine，都监听 parent 的取消信号
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			select {
			case <-parent.Done():
				fmt.Printf("   goroutine %d 收到取消信号，退出\n", id)
			case <-time.After(10 * time.Second):
				fmt.Printf("   goroutine %d 超时\n", id)
			}
		}(i)
	}

	time.Sleep(100 * time.Millisecond)
	fmt.Println("   调用 cancel()...")
	cancel() // 一个 cancel，5 个 goroutine 同时收到！

	wg.Wait()
	fmt.Println("   💡 级联取消：一个信号，所有 goroutine 同时退出！")
}

// ============================================
// 演示 2：WithTimeout 超时控制
// ============================================
func demo2WithTimeout() {
	fmt.Println("\n=== 演示 2：WithTimeout 超时控制 ===")

	// 模拟调用下游支付接口，超时 200ms
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()

	select {
	case <-ctx.Done():
		fmt.Printf("   接口调用超时！耗时: %v\n", time.Since(start))
		fmt.Printf("   错误原因: %v\n", ctx.Err())
	case <-callPaymentAPI():
		fmt.Printf("   接口调用成功！耗时: %v\n", time.Since(start))
	}

	fmt.Println("   💡 支付业务必须加超时，防止下游慢拖垮整个系统！")
}

func callPaymentAPI() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		// 模拟接口耗时 500ms，大于超时 200ms
		time.Sleep(500 * time.Millisecond)
		close(ch)
	}()
	return ch
}

// ============================================
// 演示 3：WithDeadline 截止时间
// ============================================
func demo3WithDeadline() {
	fmt.Println("\n=== 演示 3：WithDeadline 截止时间 ===")

	// 今天 23:59:59 前必须完成
	deadline := time.Now().Add(300 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	fmt.Printf("   截止时间: %v\n", deadline.Format("15:04:05.000"))

	select {
	case <-ctx.Done():
		fmt.Printf("   截止时间到！%v\n", ctx.Err())
	case <-time.After(500 * time.Millisecond):
		fmt.Println("   任务完成")
	}

	fmt.Println("   💡 WithTimeout = WithDeadline(time.Now() + timeout)")
}

// ============================================
// 演示 4：WithValue 传递请求 ID
// ============================================
func demo4WithValue() {
	fmt.Println("\n=== 演示 4：WithValue 传递请求 ID ===")

	// ❌ 错误：用 string 作为 key，容易冲突
	fmt.Println("❌ 错误：用 string 作为 key，容易冲突")
	fmt.Println("   ctx = context.WithValue(ctx, \"request_id\", \"123\")")
	fmt.Println("   // 另一个包也用 \"request_id\" 作为 key，值就被覆盖了！")

	// ✅ 正确：用私有类型作为 key，保证唯一性
	type contextKey string
	const requestIDKey = contextKey("request_id")

	ctx := context.WithValue(context.Background(), requestIDKey, "pay_20240514_123456")

	// 整个调用链都能拿到
	logRequest(ctx, "创建订单")
	logRequest(ctx, "扣减余额")
	logRequest(ctx, "发送通知")

	fmt.Println("   💡 每一条日志都带请求 ID，出问题一秒定位！")
}

func logRequest(ctx context.Context, action string) {
	type contextKey string
	const requestIDKey = contextKey("request_id")

	requestID := ctx.Value(requestIDKey)
	fmt.Printf("   [request_id=%v] %s\n", requestID, action)
}

// ============================================
// 演示 5：HTTP 中间件传递 Context
// ============================================
func demo5HTTPMiddleware() {
	fmt.Println("\n=== 演示 5：HTTP 中间件传递 Context ===")

	fmt.Println("✅ 标准写法：")
	fmt.Println(`   func RequestIDMiddleware(next http.Handler) http.Handler {`)
	fmt.Println(`       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {`)
	fmt.Println(`           requestID := uuid.New().String()`)
	fmt.Println(`           // 放到 Context 里`)
	fmt.Println(`           ctx := context.WithValue(r.Context(), requestIDKey, requestID)`)
	fmt.Println(`           // 传给下游 handler`)
	fmt.Println(`           next.ServeHTTP(w, r.WithContext(ctx))`)
	fmt.Println(`       })`)
	fmt.Println(`   }`)

	fmt.Println("\n💡 Go 1.7+ http.Request 内置 Context，整个请求链路共享！")
}

// ============================================
// 演示 6：CancelFunc 可以多次调用，幂等
// ============================================
func demo6CancelIdempotent() {
	fmt.Println("\n=== 演示 6：CancelFunc 可以多次调用，幂等 ===")

	ctx, cancel := context.WithCancel(context.Background())

	// 调用 10 次 cancel，都不会 panic
	for i := 0; i < 10; i++ {
		cancel()
	}

	fmt.Println("   调用 10 次 cancel，没有 panic ✅")
	fmt.Println("   ctx.Err() =", ctx.Err())
	fmt.Println("   💡 defer cancel() 放心写，不会重复调用出问题！")
}

// ============================================
// 演示 7：emptyCtx 的 Done() 返回 nil
// ============================================
func demo7EmptyCtx() {
	fmt.Println("\n=== 演示 7：emptyCtx 的 Done() 返回 nil ===")

	ctx := context.Background()

	// nil channel 永远阻塞
	fmt.Println("   ctx.Done() =", ctx.Done())
	fmt.Println("   ctx.Err() =", ctx.Err())

	fmt.Println("   💡 Background() 永远不会取消，适合作为根节点！")
}

// ============================================
// 演示 8：支付业务最佳实践 - 多层级超时
// ============================================
func demo8PaymentTimeout() {
	fmt.Println("\n=== 演示 8：支付业务最佳实践 - 多层级超时 ===")

	// 总超时 1 秒
	totalCtx, cancel := context.WithTimeout(context.Background(), 1000*time.Millisecond)
	defer cancel()

	fmt.Println("   总超时: 1000ms")

	// 第一步：查用户余额，超时 200ms
	balanceCtx, _ := context.WithTimeout(totalCtx, 200*time.Millisecond)
	fmt.Println("   查余额超时: 200ms (继承总超时)")
	_ = balanceCtx

	// 第二步：调用支付渠道，超时 500ms
	payCtx, _ := context.WithTimeout(totalCtx, 500*time.Millisecond)
	fmt.Println("   调用渠道超时: 500ms (继承总超时)")
	_ = payCtx

	fmt.Println("   💡 总超时 < 各步骤超时之和，保证整体不超时！")
	fmt.Println("   💡 任何一步超时，整体自动取消，级联生效！")
}

// ============================================
// 演示 9：常见坑 - Context 放结构体里
// ============================================
func demo9ContextInStruct() {
	fmt.Println("\n=== 演示 9：常见坑 - Context 放结构体里 ===")

	fmt.Println("❌ 错误写法：")
	fmt.Println(`   type Handler struct {`)
	fmt.Println(`       ctx context.Context  // ❌ 不要把 Context 存结构体！`)
	fmt.Println(`   }`)

	fmt.Println("\n✅ 官方约定：Context 作为第一个参数显式传递！")
	fmt.Println(`   func HandleRequest(ctx context.Context, req *Request) error {`)
	fmt.Println(`       // 用 ctx`)
	fmt.Println(`   }`)

	fmt.Println("\n💡 原因：Context 是请求级别的，每个请求有自己的 ctx")
	fmt.Println("   存结构体里就变成全局的了，生命周期完全混乱！")
}

// ============================================
// 演示 10：常见坑 - 忘记调用 cancel 导致 goroutine 泄漏
// ============================================
func demo10ForgetCancel() {
	fmt.Println("\n=== 演示 10：常见坑 - 忘记调用 cancel 导致 goroutine 泄漏 ===")

	fmt.Println("❌ 错误：只拿 ctx，不调用 cancel")
	fmt.Println(`   ctx, _ := context.WithCancel(parent)  // ❌ _ 忽略了 cancel！`)
	fmt.Println(`   go func() {`)
	fmt.Println(`       <-ctx.Done()  // 永远等不到，goroutine 永远阻塞！`)
	fmt.Println(`   }()`)

	fmt.Println("\n✅ 正确：defer 调用 cancel")
	fmt.Println(`   ctx, cancel := context.WithCancel(parent)`)
	fmt.Println(`   defer cancel()  // ✅ 函数退出一定调用！`)

	fmt.Println("\n💡 支付业务：每一个 WithCancel/WithTimeout 后面必须紧跟 defer cancel()！")
	fmt.Println("   go vet 可以检查出来，CI 一定要过！")
}

// ============================================
// main
// ============================================
func main() {
	// 让 unused import 不报错
	_ = http.Handler(nil)

	demo1WithCancel()
	demo2WithTimeout()
	demo3WithDeadline()
	demo4WithValue()
	demo5HTTPMiddleware()
	demo6CancelIdempotent()
	demo7EmptyCtx()
	demo8PaymentTimeout()
	demo9ContextInStruct()
	demo10ForgetCancel()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ Context 所有演示完成！")
	fmt.Println("=== 核心总结 ===")
	fmt.Println("1. WithCancel：级联取消，一个信号通知所有子 goroutine")
	fmt.Println("2. WithTimeout/WithDeadline：超时控制，支付业务必须加！")
	fmt.Println("3. WithValue：用私有类型当 key，防止 key 冲突")
	fmt.Println("4. CancelFunc 是幂等的，可以多次调用")
	fmt.Println("5. Context 永远作为第一个参数显式传递，不要存结构体！")
}
