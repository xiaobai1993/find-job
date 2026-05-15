// Go 并发模式完全指南：Worker Pool / Pipeline / errgroup / or-done
// go run 11-concurrency-patterns-demo.go
// go run -race 11-concurrency-patterns-demo.go
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// ============================================================
// 1. Worker Pool - 控制并发数
// ============================================================

func workerPoolDemo() {
	fmt.Println("=== 1. Worker Pool 模式 ===")

	tasks := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	workerCount := 3

	taskCh := make(chan int, len(tasks))
	resultCh := make(chan int, len(tasks))

	// 先把任务塞进去
	for _, task := range tasks {
		taskCh <- task
	}
	close(taskCh)

	// 启动 worker
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i + 1
		go func() {
			defer wg.Done()
			for task := range taskCh {
				fmt.Printf("Worker %d 处理任务 %d\n", workerID, task)
				time.Sleep(100 * time.Millisecond) // 模拟处理
				resultCh <- task * 10
			}
		}()
	}

	// 等所有 worker 完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	fmt.Println("\n收集结果:")
	for result := range resultCh {
		fmt.Printf("结果: %d\n", result)
	}

	fmt.Println()
}

// ============================================================
// 2. Pipeline 模式 - 流水线处理
// ============================================================

// 阶段 1: 生成数字
func gen(nums ...int) <-chan int {
	out := make(chan int, len(nums))
	go func() {
		for _, n := range nums {
			out <- n
		}
		close(out)
	}()
	return out
}

// 阶段 2: 平方
func sq(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		for n := range in {
			out <- n * n
		}
		close(out)
	}()
	return out
}

// 阶段 3: 加 1
func addOne(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		for n := range in {
			out <- n + 1
		}
		close(out)
	}()
	return out
}

func pipelineDemo() {
	fmt.Println("=== 2. Pipeline 流水线模式 ===")

	// gen → sq → addOne → print
	for v := range addOne(sq(gen(1, 2, 3, 4, 5))) {
		fmt.Printf("结果: %d\n", v)
	}

	fmt.Println()
}

// ============================================================
// 3. errgroup - 带错误和取消的并发
// ============================================================

func errgroupDemo() {
	fmt.Println("=== 3. errgroup 带错误取消 ===")

	g, ctx := errgroup.WithContext(context.Background())

	urls := []string{
		"https://www.baidu.com",
		"https://www.google.com", // 这个可能超时或失败
		"https://www.github.com",
	}

	for i, url := range urls {
		i, url := i, url
		g.Go(func() error {
			fmt.Printf("开始请求 %d: %s\n", i+1, url)

			// 模拟请求，第 2 个模拟错误
			if i == 1 {
				time.Sleep(100 * time.Millisecond)
				fmt.Printf("请求 %d 失败: 模拟错误\n", i+1)
				return fmt.Errorf("failed to fetch %s", url)
			}

			time.Sleep(300 * time.Millisecond)

			// 检查是否已经取消（因为其他 goroutine 出错了）
			select {
			case <-ctx.Done():
				fmt.Printf("请求 %d 取消: %v\n", i+1, ctx.Err())
				return ctx.Err()
			default:
			}

			fmt.Printf("请求 %d 成功: %s\n", i+1, url)
			return nil
		})
	}

	// 等待所有完成
	if err := g.Wait(); err != nil {
		fmt.Printf("有错误发生: %v\n", err)
	} else {
		fmt.Println("所有请求成功完成")
	}

	fmt.Println()
}

// ============================================================
// 4. Or-Done 模式 - 优雅取消
// ============================================================

// orDone 封装取消逻辑
func orDone[T any](ctx context.Context, in <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func orDoneDemo() {
	fmt.Println("=== 4. Or-Done 模式 ===")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 模拟一个很慢的 channel
	in := make(chan int)
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(200 * time.Millisecond)
			in <- i
		}
		close(in)
	}()

	// 使用 orDone，ctx 超时就自动退出
	for v := range orDone(ctx, in) {
		fmt.Printf("收到: %d\n", v)
	}

	fmt.Println("超时或 channel 关闭，退出循环")
	fmt.Println()
}

// ============================================================
// 5. Fan-out, Fan-in - 扇出扇入
// ============================================================

// 扇出：多个 goroutine 从同一个 channel 读
func fanOut(in <-chan int, n int) []<-chan int {
	outs := make([]<-chan int, n)
	for i := 0; i < n; i++ {
		out := make(chan int)
		outs[i] = out
		go func(id int) {
			defer close(out)
			for n := range in {
				fmt.Printf("Worker %d 处理: %d\n", id, n)
				time.Sleep(100 * time.Millisecond)
				out <- n * n
			}
		}(i)
	}
	return outs
}

// 扇入：把多个 channel 合并成一个
func fanIn(channels ...<-chan int) <-chan int {
	var wg sync.WaitGroup
	out := make(chan int)

	output := func(c <-chan int) {
		defer wg.Done()
		for n := range c {
			out <- n
		}
	}

	wg.Add(len(channels))
	for _, c := range channels {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func fanInOutDemo() {
	fmt.Println("=== 5. Fan-out, Fan-in 扇出扇入 ===")

	in := gen(1, 2, 3, 4, 5, 6, 7, 8)
	outs := fanOut(in, 3) // 3 个 worker 并行处理
	result := fanIn(outs...)

	for v := range result {
		fmt.Printf("结果: %d\n", v)
	}

	fmt.Println()
}

// ============================================================
// main
// ============================================================

func main() {
	workerPoolDemo()
	pipelineDemo()
	errgroupDemo()
	orDoneDemo()
	fanInOutDemo()

	fmt.Println("✅ 所有并发模式运行完成！")
	fmt.Println("\n💡 提示：")
	fmt.Println("   - 90% 场景用 errgroup 就够了")
	fmt.Println("   - Worker Pool 控制并发数，防止打挂下游")
	fmt.Println("   - Pipeline 适合多阶段流式处理")
	fmt.Println("   - or-done 封装取消逻辑，代码更清爽")
}
