package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================
// 演示 1：查看 GMP 基础信息
// ============================================
func demo1BasicInfo() {
	fmt.Println("=== 演示 1：GMP 基础信息 ===")

	fmt.Printf("   GOMAXPROCS (P 的数量): %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("   当前 goroutine 数量: %d\n", runtime.NumGoroutine())
	fmt.Printf("   逻辑 CPU 核数: %d\n", runtime.NumCPU())

	fmt.Println("\n   💡 P 的数量默认等于 CPU 核数，启动后一般不修改")
	fmt.Println("   💡 M 的数量可以动态变化，Go runtime 自动管理，最多 10000 个")
	fmt.Println("   💡 M 的数量没有直接的 API 查看，通过 pprof sched profile 可以看到")
}

// ============================================
// 演示 2：修改 GOMAXPROCS
// ============================================
func demo2SetGOMAXPROCS() {
	fmt.Println("\n=== 演示 2：修改 GOMAXPROCS ===")

	// 改成 1，变成单 P 调度
	old := runtime.GOMAXPROCS(1)
	fmt.Printf("   旧的 GOMAXPROCS: %d\n", old)
	fmt.Printf("   新的 GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))

	// 改回去
	runtime.GOMAXPROCS(old)
	fmt.Printf("   恢复后 GOMAXPROCS: %d\n", runtime.GOMAXPROCS(0))

	fmt.Println("\n   💡 一般只在程序启动时设置一次，运行时不要改")
	fmt.Println("   💡 Go 1.16+ 容器环境自动读取 cgroups limit")
}

// ============================================
// 演示 3：主动让出 CPU - runtime.Gosched()
// ============================================
func demo3Gosched() {
	fmt.Println("\n=== 演示 3：主动让出 CPU - runtime.Gosched() ===")

	// 设置单 P，效果更明显
	old := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(old)

	var count int32
	done := make(chan struct{})

	// G1: 一直跑
	go func() {
		for atomic.LoadInt32(&count) < 5 {
			// 没有 Gosched 的话，单 P 下这个 G 会一直占着
			// 有 Gosched 的话，会主动让出 CPU 给其他 G
			runtime.Gosched() // ✅ 主动让出
		}
		close(done)
	}()

	// G2: 累加
	for i := 0; i < 5; i++ {
		atomic.AddInt32(&count, 1)
		runtime.Gosched()
		fmt.Printf("   G2 执行，count = %d\n", count)
	}

	<-done
	fmt.Println("   💡 Gosched() 主动让出，两个 G 可以交替执行")
	fmt.Println("   💡 没有 Gosched 的话，单 P 下 G1 会一直占着，G2 拿不到")
}

// ============================================
// 演示 4：抢占式调度 - Go 1.14+ 死循环不会卡住其他 G
// ============================================
func demo4Preemptive() {
	fmt.Println("\n=== 演示 4：抢占式调度 - Go 1.14+ 死循环不会卡住 ===")

	// 设置单 P，效果更明显
	old := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(old)

	done := make(chan struct{})

	// G1: 死循环，没有函数调用
	go func() {
		fmt.Println("   死循环 G 启动...")
		for {
			// 没有函数调用，Go 1.13 及之前会永远占着 P！
			// Go 1.14+ 异步抢占，10ms 后会被抢下来
		}
	}()

	// 给 G1 一点时间启动
	time.Sleep(50 * time.Millisecond)

	// G2: 看能不能拿到 CPU
	start := time.Now()
	go func() {
		fmt.Println("   另一个 G 尝试执行...")
		fmt.Printf("   ✅ 成功执行！等了 %v\n", time.Since(start))
		fmt.Println("   💡 Go 1.14+ 异步抢占生效，死循环不会卡住整个 P！")
		close(done)
	}()

	// 最多等 1 秒，防止老版本 Go 真的卡住
	select {
	case <-done:
		// 正常，抢占成功
	case <-time.After(1 * time.Second):
		fmt.Println("   ❌ 1 秒都没执行，说明 Go 版本 < 1.14，协作式抢占！")
	}

	fmt.Println("   💡 面试加分项：Go 1.14 基于信号的异步抢占是革命性改进！")
}

// ============================================
// 演示 5：工作窃取效果
// ============================================
func demo5WorkStealing() {
	fmt.Println("\n=== 演示 5：工作窃取效果 ===")

	const goroutines = 10000
	const workPerG = 1000

	var wg sync.WaitGroup
	var total int64

	start := time.Now()

	// 一下子创建 1 万个 goroutine，每个做一点计算
	// P0 的本地队列满了就放全局队列，其他 P 会偷过去执行
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sum := 0
			for j := 0; j < workPerG; j++ {
				sum += j
			}
			atomic.AddInt64(&total, int64(sum))
		}()
	}

	wg.Wait()
	fmt.Printf("   %d 个 goroutine 执行完成，总耗时: %v\n", goroutines, time.Since(start))
	fmt.Printf("   当前 goroutine 数量: %d\n", runtime.NumGoroutine())
	fmt.Println("   💡 工作窃取让所有 P 都在干活，不会有的忙死有的闲死")
}

// ============================================
// 演示 6：goroutine 泄漏演示与排查
// ============================================
func demo6GoroutineLeak() {
	fmt.Println("\n=== 演示 6：goroutine 泄漏演示与排查 ===")

	initial := runtime.NumGoroutine()
	fmt.Printf("   初始 goroutine 数量: %d\n", initial)

	// 泄漏 100 个 goroutine
	for i := 0; i < 100; i++ {
		ch := make(chan int)
		go func() {
			// ❌ 永远没人读，这个 G 永远卡在这里！泄漏！
			ch <- 1
		}()
	}

	time.Sleep(100 * time.Millisecond) // 等 goroutine 启动并阻塞
	afterLeak := runtime.NumGoroutine()

	fmt.Printf("   泄漏后 goroutine 数量: %d（泄漏了 %d 个）\n", afterLeak, afterLeak-initial)

	fmt.Println("\n   怎么排查泄漏：")
	fmt.Println("   1. 启动 pprof: go func() { http.ListenAndServe(\":6060\", nil) }()")
	fmt.Println("   2. 查看: curl http://localhost:6060/debug/pprof/goroutine?debug=1")
	fmt.Println("   3. 看哪个函数的 goroutine 一直在涨")
	fmt.Println("\n   常见泄漏原因：")
	fmt.Println("   - channel 读写没有对端")
	fmt.Println("   - http 请求没有超时")
	fmt.Println("   - 互斥锁死锁")
	fmt.Println("   - time.Timer 没有 Stop")
}

// ============================================
// 演示 7：系统调用对 P 的影响
// ============================================
func demo7Syscall() {
	fmt.Println("\n=== 演示 7：系统调用对 P 的影响 ===")

	old := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(old)

	start := time.Now()
	var wg sync.WaitGroup

	// 两个 goroutine，都做 sleep 系统调用
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			time.Sleep(100 * time.Millisecond) // 系统调用
			fmt.Printf("   goroutine %d 完成\n", id)
		}(i)
	}

	wg.Wait()
	fmt.Printf("   2 个 goroutine 各 sleep 100ms，总耗时: %v\n", time.Since(start))

	fmt.Println("\n   💡 总耗时 100ms 左右，不是 200ms！")
	fmt.Println("   💡 原因：第一个 G 进入 sleep 系统调用超过 20us")
	fmt.Println("      P 被偷出来给第二个 G 用，所以两个是并行的！")
	fmt.Println("   💡 这就是长系统调用释放 P 的机制！")
}

// ============================================
// 演示 8：cgo 创建新的 M
// ============================================
func demo8CgoNewM() {
	fmt.Println("\n=== 演示 8：cgo 调用创建新的 M ===")

	fmt.Println("   cgo 调用会创建新的 M，不会占着 P")
	fmt.Println("   因为 cgo 是阻塞的，P 会解绑去跑其他 G")
	fmt.Println("   ❌ 不加控制的话，1000 个 cgo 调用会创建 1000 个内核线程！")
	fmt.Println("   ✅ 最佳实践：cgo 调用也要做并发控制，用 worker pool")
}

// ============================================
// 演示 9：支付业务最佳实践 - GOMAXPROCS 容器环境
// ============================================
func demo9ContainerBestPractice() {
	fmt.Println("\n=== 演示 9：支付业务最佳实践 - GOMAXPROCS 容器环境 ===")

	fmt.Println("   ❌ Go 1.15 及之前的坑：")
	fmt.Println("      GOMAXPROCS 默认 = 宿主机 CPU 核数，不是容器 limit")
	fmt.Println("      容器限制 2 核，宿主机 32 核 → GOMAXPROCS = 32")
	fmt.Println("      32 个 M 抢 2 个 CPU 核，上下文切换爆炸！")
	fmt.Println("      性能下降 5~10 倍！")

	fmt.Println("\n   ✅ 解决办法：")
	fmt.Println("      1. Go 1.16+ 已自动支持 cgroups，自动读 limit")
	fmt.Println("      2. 老版本用 uber-go/automaxprocs 包：")
	fmt.Println("         import _ \"go.uber.org/automaxprocs\"")

	fmt.Println("\n   💡 支付业务必加！不加的话容器环境性能直接砍半！")
}

// ============================================
// 演示 10：pprof 启动演示
// ============================================
func demo10Pprof() {
	fmt.Println("\n=== 演示 10：pprof 调试调度问题 ===")

	fmt.Println("   启动 pprof 调试端口（后台运行）：")
	fmt.Println(`   go func() {`)
	fmt.Println(`       _ = http.ListenAndServe(":6060", nil)`)
	fmt.Println(`   }()`)

	// 实际启动，注释掉避免端口占用
	// go func() {
	//     _ = http.ListenAndServe(":6060", nil)
	// }()

	fmt.Println("\n   常用调试命令：")
	fmt.Println("   # 查看 goroutine 数量和栈")
	fmt.Println("   curl http://localhost:6060/debug/pprof/goroutine?debug=1")
	fmt.Println("")
	fmt.Println("   # 查看调度延迟")
	fmt.Println("   curl http://localhost:6060/debug/pprof/sched?debug=1")
	fmt.Println("")
	fmt.Println("   # 采集 30 秒 CPU profile")
	fmt.Println("   go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30")

	fmt.Println("\n   💡 支付生产环境一定要开 pprof，出问题 1 分钟定位！")
}

// ============================================
// main
// ============================================
func main() {
	// 让 unused import 不报错
	_ = http.ListenAndServe

	demo1BasicInfo()
	demo2SetGOMAXPROCS()
	demo3Gosched()
	demo4Preemptive()
	demo5WorkStealing()
	demo6GoroutineLeak()
	demo7Syscall()
	demo8CgoNewM()
	demo9ContainerBestPractice()
	demo10Pprof()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ GMP 调度模型所有演示完成！")
	fmt.Println("=== 核心总结 ===")
	fmt.Println("1. G=执行单元，M=执行载体，P=调度队列")
	fmt.Println("2. 找 G 优先级：本地队列 → 全局队列 → 偷其他 P → netpoll → 休眠")
	fmt.Println("3. 工作窃取：随机选 P，偷一半，负载均衡")
	fmt.Println("4. 抢占式调度三阶段：协作式 → 信号 → 寄存器")
	fmt.Println("5. 长系统调用 > 20us 释放 P，短的不释放")
	fmt.Println("6. 容器环境一定要注意 GOMAXPROCS，用 automaxprocs")
	fmt.Println("7. goroutine 泄漏用 pprof 排查，看哪个函数一直在涨")
	fmt.Println("\n=== 支付业务最佳实践 ===")
	fmt.Println("✅ 容器环境必须 import automaxprocs，否则性能砍半")
	fmt.Println("✅ 生产环境必须开 pprof 端口，出问题快速定位")
	fmt.Println("✅ cgo 调用必须做并发控制，避免创建太多 M")
	fmt.Println("✅ 监控 goroutine 数量，持续上涨就是泄漏了")
}
