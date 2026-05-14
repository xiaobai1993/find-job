package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ============================================
// 演示 1：多个 case 同时就绪，随机选择
// ============================================
func demo1RandomSelect() {
	fmt.Println("=== 演示 1：多个 case 同时就绪，随机选择 ===")

	ch1 := make(chan int, 1)
	ch2 := make(chan int, 1)

	ch1 <- 1
	ch2 <- 2

	// 运行 5 次，看输出是不是随机的
	for i := 0; i < 5; i++ {
		select {
		case v := <-ch1:
			fmt.Printf("第 %d 次: 选中 ch1，值 = %d\n", i+1, v)
			ch1 <- 1 // 放回去
		case v := <-ch2:
			fmt.Printf("第 %d 次: 选中 ch2，值 = %d\n", i+1, v)
			ch2 <- 2 // 放回去
		}
	}
	fmt.Println("💡 每次运行结果都不一样，完全随机！")
}

// ============================================
// 演示 2：没有 default 会阻塞
// ============================================
func demo2NoDefault() {
	fmt.Println("\n=== 演示 2：没有 default 会阻塞（用 goroutine 演示）===")

	ch := make(chan int)
	done := make(chan struct{})

	go func() {
		fmt.Println("goroutine: 进入 select，没有 default，等待...")
		select {
		case <-ch:
			fmt.Println("goroutine: 收到数据，退出")
			close(done)
		}
	}()

	time.Sleep(500 * time.Millisecond)
	fmt.Println("main: 500ms 后才发送数据")
	ch <- 1
	<-done
}

// ============================================
// 演示 3：有 default 不会阻塞
// ============================================
func demo3WithDefault() {
	fmt.Println("\n=== 演示 3：有 default 不会阻塞 ===")

	ch := make(chan int) // 无缓冲，没人写

	select {
	case <-ch:
		fmt.Println("收到数据")
	default:
		fmt.Println("✅ default 分支，立即返回，不阻塞！")
	}
}

// ============================================
// 演示 4：nil channel 永远不会就绪，动态禁用分支
// ============================================
func demo4NilChannel() {
	fmt.Println("\n=== 演示 4：nil channel 动态禁用分支 ===")

	var ch1 chan int // nil
	ch2 := make(chan int, 1)
	ch2 <- 2

	select {
	case v := <-ch1:
		fmt.Println("ch1:", v) // 永远不会执行
	case v := <-ch2:
		fmt.Println("✅ 只有 ch2 被选中，ch1 是 nil 被禁用了，值 =", v)
	}

	// 演示动态开关
	fmt.Println("\n演示动态禁用分支：")
	ch := make(chan int, 1)
	ch <- 100
	pause := false

	for i := 0; i < 5; i++ {
		select {
		case v := <-ch:
			fmt.Printf("第 %d 次: 收到 %d\n", i+1, v)
			if !pause {
				fmt.Println("  → 暂停分支被禁用了！")
				pause = true
			}
		default:
			fmt.Printf("第 %d 次: default\n", i+1)
		}
	}
}

// ============================================
// 演示 5：break 只能跳出 select，不能跳出 for
// ============================================
func demo5Break() {
	fmt.Println("\n=== 演示 5：break 只能跳出 select ===")

	fmt.Println("❌ 错误写法：break 只跳出 select，for 还在跑")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case <-ch:")
	fmt.Println("          break  // 只跳出 select，for 还在继续！死循环！")
	fmt.Println("      }")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法 1：用 return（如果在函数里）")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case <-ch:")
	fmt.Println("          return  // 直接返回整个函数")
	fmt.Println("      }")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法 2：用标签（下一个演示）")
}

// ============================================
// 演示 6：用标签跳出外层 for
// ============================================
func demo6BreakLabel() {
	fmt.Println("\n=== 演示 6：用标签跳出外层 for ===")

loop:
	for i := 0; i < 5; i++ {
		select {
		case <-time.After(50 * time.Millisecond):
			fmt.Printf("第 %d 次，break loop 直接跳出 for\n", i+1)
			break loop // 直接跳到 loop 标签后面，跳出 for
		}
	}
	fmt.Println("✅ 已经跳出 for 循环了")
}

// ============================================
// 演示 7：超时控制（最常用）
// ============================================
func demo7Timeout() {
	fmt.Println("\n=== 演示 7：超时控制 ===")

	// 模拟一个慢操作
	slowFunc := func() chan int {
		ch := make(chan int)
		go func() {
			time.Sleep(2 * time.Second) // 2秒才返回
			ch <- 100
		}()
		return ch
	}

	result := slowFunc()

	select {
	case v := <-result:
		fmt.Println("业务完成，结果 =", v)
	case <-time.After(1 * time.Second): // 1秒超时
		fmt.Println("❌ 1秒超时，业务还没完成！")
	}
}

// ============================================
// 演示 8：多路信号监听
// ============================================
func demo8MultiSignal() {
	fmt.Println("\n=== 演示 8：多路信号监听 ===")

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan struct{})
	job := make(chan int, 3)

	// 发几个任务
	for i := 1; i <= 3; i++ {
		job <- i
	}

	// 1秒后发停止信号
	go func() {
		time.Sleep(1 * time.Second)
		fmt.Println("\n发送停止信号")
		close(stop)
	}()

	// worker
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				fmt.Println("worker: context 取消，退出")
				return
			case <-stop:
				fmt.Println("worker: 收到停止信号，退出")
				return
			case j := <-job:
				fmt.Printf("worker: 处理任务 %d\n", j)
				time.Sleep(300 * time.Millisecond)
			}
		}
	}()

	<-done
	cancel()
}

// ============================================
// 演示 9：非阻塞读写
// ============================================
func demo9NonBlocking() {
	fmt.Println("\n=== 演示 9：非阻塞读写 ===")

	ch := make(chan int, 1)

	// 非阻塞读
	v, ok := tryRead(ch)
	if ok {
		fmt.Println("读到:", v)
	} else {
		fmt.Println("❌ 非阻塞读：没有数据，立即返回")
	}

	// 写一个
	ch <- 100

	// 再读
	v, ok = tryRead(ch)
	if ok {
		fmt.Println("✅ 非阻塞读：读到", v)
	}

	// 非阻塞写（channel 满了）
	ch <- 200 // 填满了
	ok = tryWrite(ch, 300)
	if ok {
		fmt.Println("写成功")
	} else {
		fmt.Println("❌ 非阻塞写：channel 满了，写不进去，立即返回")
	}
}

func tryRead(ch <-chan int) (int, bool) {
	select {
	case v := <-ch:
		return v, true
	default:
		return 0, false
	}
}

func tryWrite(ch chan<- int, v int) bool {
	select {
	case ch <- v:
		return true
	default:
		return false
	}
}

// ============================================
// 演示 10：time.After 内存泄漏坑
// ============================================
func demo10TimeAfterLeak() {
	fmt.Println("\n=== 演示 10：time.After 内存泄漏坑 ===")

	fmt.Println("❌ 错误写法：")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case <-dataChan:")
	fmt.Println("          process()")
	fmt.Println("      case <-time.After(5min):  // 每次循环都创建新定时器！")
	fmt.Println("          return")
	fmt.Println("      }")
	fmt.Println("  }")
	fmt.Println("\n💡 如果 dataChan 一直有数据，定时器永远不会触发，也不会被 GC，内存泄漏！")

	fmt.Println("\n✅ 正确写法：")
	fmt.Println("  timeout := time.After(5min)  // 循环外面创建一次")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case <-dataChan:")
	fmt.Println("          process()")
	fmt.Println("      case <-timeout:  // 同一个定时器")
	fmt.Println("          return")
	fmt.Println("      }")
	fmt.Println("  }")
}

// ============================================
// 演示 11：向已关闭的 channel 写，default 救不了
// ============================================
func demo11ClosedChannelPanic() {
	fmt.Println("\n=== 演示 11：关闭的 channel 还是会 panic，default 救不了 ===")

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("✅ 果然 panic 了:", r)
			fmt.Println("💡 default 只能避免阻塞，不能避免 panic！")
		}
	}()

	ch := make(chan int)
	close(ch)

	select {
	case ch <- 1: // 向已关闭的 channel 写，panic！
		fmt.Println("写成功")
	default:
		fmt.Println("default") // 永远不会走到这里！
	}
}

// ============================================
// 演示 12：for + select 忙等待吃满 CPU
// ============================================
func demo12BusyWait() {
	fmt.Println("\n=== 演示 12：for + select 忙等待吃满 CPU ===")

	fmt.Println("❌ 错误写法：")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case data := <-dataChan:")
	fmt.Println("          process(data)")
	fmt.Println("      default:  // 没有数据就疯狂跑 default，CPU 100%！")
	fmt.Println("      }")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法 1：去掉 default，让 select 阻塞等待")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case data := <-dataChan:")
	fmt.Println("          process(data)")
	fmt.Println("      }")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法 2：必须非阻塞就加个小 sleep")
	fmt.Println("  for {")
	fmt.Println("      select {")
	fmt.Println("      case data := <-dataChan:")
	fmt.Println("          process(data)")
	fmt.Println("      default:")
	fmt.Println("          time.Sleep(1ms)  // 避免 CPU 打满")
	fmt.Println("      }")
	fmt.Println("  }")
}

// ============================================
// main
// ============================================
func main() {
	demo1RandomSelect()
	demo2NoDefault()
	demo3WithDefault()
	demo4NilChannel()
	demo5Break()
	demo6BreakLabel()
	demo7Timeout()
	demo8MultiSignal()
	demo9NonBlocking()
	demo10TimeAfterLeak()
	demo11ClosedChannelPanic()
	demo12BusyWait()

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有 select 演示完成！")
	fmt.Println("核心总结：")
	fmt.Println("1. 多个 case 就绪随机选")
	fmt.Println("2. 没 default 会阻塞，有 default 立即返回")
	fmt.Println("3. nil channel = 禁用分支，动态开关非常好用")
	fmt.Println("4. break 只跳出 select，要跳出 for 用标签")
	fmt.Println("5. time.After 放循环外面，不然内存泄漏")
	fmt.Println("6. default 救不了关闭的 channel，还是会 panic")
	fmt.Println("7. 空 default 会把 CPU 打满")
}
