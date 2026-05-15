//go:build ignore
// +build ignore

package main

import (
	"context"
	"fmt"
	"time"
)

// ==========================================
// Go Context 完整演示
// 运行：go run context-demo.go
// ==========================================

func main() {
	fmt.Println("========== Go Context 完整演示 ==========")
	fmt.Println()

	fmt.Println("1. 演示：WithCancel 手动取消")
	demoCancel()
	fmt.Println()

	fmt.Println("2. 演示：WithTimeout 超时自动取消")
	demoTimeout()
	fmt.Println()

	fmt.Println("3. 演示：级联取消（父取消，子自动取消）")
	demoCascade()
	fmt.Println()

	fmt.Println("4. 演示：WithValue 传值")
	demoValue()
	fmt.Println()

	fmt.Println("5. 演示：Go 1.21+ WithoutCancel")
	demoWithoutCancel()
	fmt.Println()

	fmt.Println("6. 演示：Go 1.21+ AfterFunc")
	demoAfterFunc()
	fmt.Println()

	fmt.Println("========== 所有演示完成 ✅ ==========")
}

// ------------------------------
// 1. WithCancel 手动取消
// ------------------------------
func demoCancel() {
	ctx, cancel := context.WithCancel(context.Background())

	// 启动 worker
	go func() {
		for {
			select {
			case <-ctx.Done():
				fmt.Println("  worker 收到取消信号，退出！")
				return
			default:
				fmt.Println("  worker 工作中...")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	// 1.5 秒后手动取消
	time.Sleep(1500 * time.Millisecond)
	fmt.Println("  主 goroutine 调用 cancel()")
	cancel()

	// 等一下让 worker 退出
	time.Sleep(600 * time.Millisecond)
}

// ------------------------------
// 2. WithTimeout 超时自动取消
// ------------------------------
func demoTimeout() {
	// 2 秒后自动超时
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel() // 好习惯：即使超时了也手动调用一下

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				fmt.Printf("  收到超时信号，原因: %v\n", ctx.Err())
				done <- struct{}{}
				return
			default:
				fmt.Println("  正在处理请求...")
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	<-done
}

// ------------------------------
// 3. 级联取消
// ------------------------------
func demoCascade() {
	// 父 Context
	parent, cancelParent := context.WithCancel(context.Background())

	// 两个子 Context（都基于父 Context）
	child1, _ := context.WithCancel(parent)
	child2, _ := context.WithCancel(parent)

	// 孙 Context（基于 child1）
	grandchild, _ := context.WithCancel(child1)

	// 启动 3 个 worker，分别用不同的 Context
	go func() {
		<-child1.Done()
		fmt.Println("  child1 worker 退出")
	}()

	go func() {
		<-child2.Done()
		fmt.Println("  child2 worker 退出")
	}()

	go func() {
		<-grandchild.Done()
		fmt.Println("  grandchild worker 退出")
	}()

	fmt.Println("  启动所有 worker，1 秒后取消父 Context")
	time.Sleep(1 * time.Second)

	// 只取消父 Context！
	fmt.Println("  只调用 cancelParent()...")
	cancelParent()

	// 等一下让所有 worker 都退出
	time.Sleep(500 * time.Millisecond)
	fmt.Println("  ✅ 所有 worker 都退出了（级联取消生效）")
}

// ------------------------------
// 4. WithValue 传值
// ------------------------------

// 自定义 key 类型，避免冲突（非常重要！）
type contextKey string

const (
	TraceIDKey contextKey = "trace_id"
	UserIDKey  contextKey = "user_id"
)

func demoValue() {
	// 根 Context
	root := context.Background()

	// 第一层：加 TraceID
	ctx1 := context.WithValue(root, TraceIDKey, "abc123xyz")

	// 第二层：再加 UserID
	ctx2 := context.WithValue(ctx1, UserIDKey, int64(12345))

	// 取值（递归向上找）
	fmt.Printf("  TraceID: %v\n", ctx2.Value(TraceIDKey))
	fmt.Printf("  UserID: %v\n", ctx2.Value(UserIDKey))

	// 父 Context 拿不到子 Context 的值
	fmt.Printf("  ctx1 拿 UserID: %v (nil，因为父不能拿子的值)\n", ctx1.Value(UserIDKey))

	// ❌ 错误演示：用 string 当 key 会冲突
	// ctxA := context.WithValue(root, "key", "valueA")
	// ctxB := context.WithValue(ctxA, "key", "valueB")
	// ctxB.Value("key") → "valueB"，覆盖了！
}

// ------------------------------
// 5. Go 1.21+ WithoutCancel
// ------------------------------
func demoWithoutCancel() {
	// 父 Context 1 秒后超时
	parent, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 创建不可取消的 Context，继承 parent 的值，但不会被取消
	ctx := context.WithoutCancel(parent)

	// 1.5 秒后，parent 已经超时了
	time.Sleep(1500 * time.Millisecond)

	fmt.Printf("  parent.Err(): %v\n", parent.Err())          // 已经超时
	fmt.Printf("  ctx.Err(): %v (nil，没有被取消)\n", ctx.Err()) // 还是正常的

	select {
	case <-ctx.Done():
		fmt.Println("  ctx.Done() 返回了")
	default:
		fmt.Println("  ctx.Done() 没有返回，永远不会取消")
	}
}

// ------------------------------
// 6. Go 1.21+ AfterFunc
// ------------------------------
func demoAfterFunc() {
	ctx, cancel := context.WithCancel(context.Background())

	// Context 取消时执行回调
	stop := context.AfterFunc(ctx, func() {
		fmt.Println("  🎯 AfterFunc 回调被执行！Context 被取消了")
	})
	defer stop() // 可以取消回调注册

	fmt.Println("  2 秒后取消 Context...")
	time.Sleep(2 * time.Second)
	cancel()

	// 等回调执行
	time.Sleep(500 * time.Millisecond)
}
