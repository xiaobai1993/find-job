//go:build ignore

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	// 模拟美餐的 xerrgroup
	safego "go.planetmeican.com/meican-pay/pkg/safe_go"
)

// ============================================================
// 第一部分：官方 errgroup 完整 demo（所有方法展示）
// ============================================================

func officialErrgroupDemo() {
	fmt.Println("===== 官方 errgroup 完整演示 =====")

	// Demo 1: 基础用法
	fmt.Println("\n1. 基础用法：")
	g1 := new(errgroup.Group)
	for i := 0; i < 3; i++ {
		i := i
		g1.Go(func() error {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("  task %d done\n", i)
			return nil
		})
	}
	if err := g1.Wait(); err != nil {
		fmt.Printf("  error: %v\n", err)
	} else {
		fmt.Println("  all tasks done successfully")
	}

	// Demo 2: WithContext + 错误取消
	fmt.Println("\n2. WithContext + 错误取消（第一个错误会取消所有 goroutine）：")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	g2, ctx := errgroup.WithContext(ctx)
	for i := 0; i < 5; i++ {
		i := i
		g2.Go(func() error {
			select {
			case <-ctx.Done():
				fmt.Printf("  task %d canceled: %v\n", i, ctx.Err())
				return ctx.Err()
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
				if i == 2 {
					fmt.Printf("  task %d return error\n", i)
					return errors.New("task 2 failed")
				}
				fmt.Printf("  task %d done\n", i)
				return nil
			}
		})
	}
	if err := g2.Wait(); err != nil {
		fmt.Printf("  first error: %v\n", err)
		fmt.Printf("  context cause: %v\n", context.Cause(ctx))
	}

	// Demo 3: SetLimit 并发控制（Go 1.20+ 新增）
	fmt.Println("\n3. SetLimit 并发控制（限制最多 2 个并发）：")
	g3 := new(errgroup.Group)
	g3.SetLimit(2) // ✅ 限制最多同时跑 2 个 goroutine

	for i := 0; i < 5; i++ {
		i := i
		g3.Go(func() error {
			fmt.Printf("  task %d start (concurrent running)\n", i)
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("  task %d done\n", i)
			return nil
		})
	}
	g3.Wait()
	fmt.Println("  all tasks done, limited to 2 concurrent")

	// Demo 4: TryGo 非阻塞启动（Go 1.20+ 新增）
	fmt.Println("\n4. TryGo 非阻塞启动（满了直接返回 false）：")
	g4 := new(errgroup.Group)
	g4.SetLimit(2)

	successCount := 0
	failCount := 0
	for i := 0; i < 5; i++ {
		i := i
		if g4.TryGo(func() error {
			fmt.Printf("  task %d started successfully\n", i)
			time.Sleep(200 * time.Millisecond)
			return nil
		}) {
			successCount++
		} else {
			failCount++
			fmt.Printf("  task %d failed to start (limit reached)\n", i)
		}
	}
	g4.Wait()
	fmt.Printf("  started: %d, rejected: %d\n", successCount, failCount)

	// Demo 5: WithCancelCause 获取取消原因（Go 1.20+ 新增）
	fmt.Println("\n5. WithCancelCause 取消原因获取：")
	g5, ctx5 := errgroup.WithContext(context.Background())
	g5.Go(func() error {
		return errors.New("database connection failed")
	})
	g5.Wait()
	fmt.Printf("  cancel cause: %v\n", context.Cause(ctx5))
}

// ============================================================
// 第二部分：美餐 xerrgroup 定义（和真实源码完全一致）
// ============================================================

type Group struct {
	*errgroup.Group // ✅ 结构体嵌入官方 errgroup
}

func WithContext(ctx context.Context) (*Group, context.Context) {
	group, ctx := errgroup.WithContext(ctx)
	return &Group{Group: group}, ctx
}

// 美餐只重写了 Go 方法，其他方法全部继承！
func (g *Group) Go(ctx context.Context, fn func(ctx context.Context) error) {
	g.Group.Go(safego.ErrGroupFunc(ctx, fn)) // 自动套 safego
}

// ============================================================
// 第三部分：美餐 xerrgroup demo - 重点！
// ============================================================

func meicanXerrgroupDemo() {
	fmt.Println("\n\n===== 美餐 xerrgroup 完整演示 =====")

	// Demo 1: 基础用法（和官方一样）
	fmt.Println("\n1. 基础用法 + 自动 panic recover：")
	g1, ctx := WithContext(context.Background())
	for i := 0; i < 3; i++ {
		i := i
		g1.Go(ctx, func(ctx context.Context) error {
			if i == 1 {
				panic("oh no, task 1 panic!") // ✅ 会被 safego recover！
			}
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("  task %d done\n", i)
			return nil
		})
	}
	if err := g1.Wait(); err != nil {
		fmt.Printf("  error (panic was recovered!): %v\n", err)
		fmt.Printf("  is ErrPanic: %v\n", errors.Is(err, safego.ErrPanic))
	}

	// Demo 2: ✅ SetLimit 直接用！不需要改 xerrgroup 代码！
	fmt.Println("\n2. SetLimit 直接用官方方法（xerrgroup 完全没写这个方法！）：")
	g2 := &Group{Group: new(errgroup.Group)}
	g2.SetLimit(2) // ✅ 直接调用嵌入的 errgroup 的 SetLimit！

	for i := 0; i < 5; i++ {
		i := i
		g2.Go(context.Background(), func(ctx context.Context) error {
			fmt.Printf("  task %d start\n", i)
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("  task %d done\n", i)
			return nil
		})
	}
	g2.Wait()
	fmt.Println("  SetLimit works out of the box!")

	// Demo 3: ✅ TryGo 直接用！也不需要改代码！
	fmt.Println("\n3. TryGo 直接用官方方法（xerrgroup 也没写这个方法！）：")
	g3 := &Group{Group: new(errgroup.Group)}
	g3.SetLimit(2)

	successCount := 0
	failCount := 0
	for i := 0; i < 5; i++ {
		i := i
		if g3.TryGo(func() error {
			fmt.Printf("  task %d started\n", i)
			time.Sleep(200 * time.Millisecond)
			return nil
		}) {
			successCount++
		} else {
			failCount++
			fmt.Printf("  task %d rejected\n", i)
		}
	}
	g3.Wait()
	fmt.Printf("  started: %d, rejected: %d\n", successCount, failCount)
	fmt.Println("  TryGo works out of the box!")
}

// ============================================================
// 第四部分：关键对比总结
// ============================================================

func summary() {
	fmt.Println("\n\n===== 核心结论：嵌入 vs 重写 =====")
	fmt.Println("")
	fmt.Println("✅ 美餐 xerrgroup 用结构体嵌入官方 errgroup")
	fmt.Println("✅ 官方新增 SetLimit、TryGo 等方法，xerrgroup 自动获得！")
	fmt.Println("✅ 不需要改一行 xerrgroup 的代码！")
	fmt.Println("")
	fmt.Println("❌ 如果当年美餐是把官方源码复制过来改的：")
	fmt.Println("❌ 每次官方加新方法，都要手动合并代码")
	fmt.Println("❌ 官方的 bugfix 也要手动同步")
	fmt.Println("❌ 维护成本极高")
	fmt.Println("")
	fmt.Println("💡 这就是为什么资深工程师会选择『嵌入复用』而不是『重写』")
	fmt.Println("💡 最小代码量，最大收益，零维护成本")
}

func main() {
	officialErrgroupDemo()
	meicanXerrgroupDemo()
	summary()
}
