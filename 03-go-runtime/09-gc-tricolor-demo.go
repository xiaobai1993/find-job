package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// ============================================
// 演示 1：手动触发 GC 并查看 GC 信息
// ============================================
func demo1ManualGC() {
	fmt.Println("=== 演示 1：手动触发 GC 并查看 GC 信息 ===")

	// 分配一些内存
	allocMemory(100 * 1024 * 1024) // 100MB

	fmt.Println("   手动触发 GC 前...")
	runtime.GC() // 手动触发 GC
	fmt.Println("   GC 完成！")

	// 查看 GC 统计
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	fmt.Printf("   NumGC: %d 次\n", stats.NumGC)
	fmt.Printf("   PauseTotalNs: %v 总 STW 时间\n", time.Duration(stats.PauseTotalNs))
	fmt.Printf("   HeapAlloc: %.2f MB\n", float64(stats.HeapAlloc)/1024/1024)
}

func allocMemory(size int) []byte {
	return make([]byte, size)
}

// ============================================
// 演示 2：GODEBUG gctrace 查看 GC 日志
// ============================================
func demo2GCTrace() {
	fmt.Println("\n=== 演示 2：查看 GC 日志 ===")

	fmt.Println("   开启 GC 日志方法：")
	fmt.Println("   export GODEBUG=gctrace=1")
	fmt.Println("   ./your-program")
	fmt.Println("")
	fmt.Println("   GC 日志示例：")
	fmt.Println("   gc 12 @25.125s 0%: 0.018+1.2+0.083 ms clock, 0.14+2.5/1.0/0+0.66 ms cpu, 4->4->1 MB, 5 MB goal, 8 P")
	fmt.Println("")
	fmt.Println("   解读：")
	fmt.Println("   gc 12          → 第 12 次 GC")
	fmt.Println("   @25.125s       → 程序启动 25.125 秒")
	fmt.Println("   0%             → GC 占总 CPU 0%")
	fmt.Println("   0.018+1.2+0.083 → STW1 + 并发标记 + STW2，三个阶段耗时")
	fmt.Println("   4->4->1 MB     → GC 开始 -> GC 结束 -> 存活对象大小")
	fmt.Println("   5 MB goal      → 本次 GC 目标堆大小")
	fmt.Println("   8 P            → P 的数量")

	fmt.Println("\n   💡 生产环境出问题，第一时间看 gctrace！")
}

// ============================================
// 演示 3：GOGC 调优演示
// ============================================
func demo3GOGC() {
	fmt.Println("\n=== 演示 3：GOGC 调优演示 ===")

	fmt.Println("   GOGC 默认值 = 100，表示堆涨到上次的 2 倍触发 GC")
	fmt.Println("")
	fmt.Println("   场景 1：延迟敏感，内存充足")
	fmt.Println("   export GOGC=200  # 涨到 3 倍才触发，GC 频率降一半，内存占用升高")
	fmt.Println("")
	fmt.Println("   场景 2：内存紧张，延迟不敏感")
	fmt.Println("   export GOGC=50   # 涨到 1.5 倍触发，GC 更频繁，内存占用低")
	fmt.Println("")
	fmt.Println("   场景 3：极端情况，几乎不 GC")
	fmt.Println("   export GOGC=off  # 关闭 GC（除非内存真的不够了）")

	fmt.Println("\n   💡 支付业务：延迟敏感，一般 GOGC=200~500")
}

// ============================================
// 演示 4：Go 1.19+ GOMEMLIMIT 软内存限制
// ============================================
func demo4GOMEMLIMIT() {
	fmt.Println("\n=== 演示 4：Go 1.19+ GOMEMLIMIT 软内存限制 ===")

	fmt.Println("   Go 1.19 新增，容器环境神器！")
	fmt.Println("")
	fmt.Println("   用法：")
	fmt.Println("   export GOMEMLIMIT=1GiB  # Go runtime 最多用 1GB 内存")
	fmt.Println("")
	fmt.Println("   工作原理：")
	fmt.Println("   超过软限制后，GC 频率自动升高，尽量把内存控制在限制内")
	fmt.Println("   不是硬限制，真不够还是会 OOM，但概率大大降低")
	fmt.Println("")
	fmt.Println("   容器环境最佳实践：")
	fmt.Println("   容器 limit 设成 1GB，GOMEMLIMIT 设成 900MiB")
	fmt.Println("   export GOMEMLIMIT=900MiB")

	fmt.Println("\n   💡 支付业务必开！再也不用担心 GC 不及时导致 OOM 了！")
	fmt.Println("   💡 这是 Go 最近几年最实用的特性之一！")
}

// ============================================
// 演示 5：sync.Pool 减少 GC 压力
// ============================================
func demo5SyncPool() {
	fmt.Println("\n=== 演示 5：sync.Pool 减少 GC 压力 ===")

	type Buffer struct {
		Data []byte
	}

	var pool = sync.Pool{
		New: func() interface{} {
			return &Buffer{Data: make([]byte, 4096)}
		},
	}

	const iterations = 100000

	// 不使用 Pool，每次都新建
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_ = &Buffer{Data: make([]byte, 4096)}
	}
	noPoolTime := time.Since(start)

	// 手动 GC 一下
	runtime.GC()
	time.Sleep(10 * time.Millisecond)

	// 使用 Pool，复用对象
	start = time.Now()
	for i := 0; i < iterations; i++ {
		buf := pool.Get().(*Buffer)
		// 使用 buf...
		buf.Data = buf.Data[:0] // Reset
		pool.Put(buf)
	}
	poolTime := time.Since(start)

	fmt.Printf("   不使用 Pool: %v\n", noPoolTime)
	fmt.Printf("   使用 Pool:   %v\n", poolTime)
	fmt.Printf("   性能提升: %.1f 倍！\n", float64(noPoolTime)/float64(poolTime))

	fmt.Println("\n   💡 减少对象分配 = 减少 GC 工作量 = 更低延迟！")
	fmt.Println("   💡 sync.Pool 是支付系统优化性能的第一大杀器！")
}

// ============================================
// 演示 6：小对象 vs 大对象对 GC 的影响
// ============================================
func demo6ObjectSize() {
	fmt.Println("\n=== 演示 6：小对象 vs 大对象对 GC 的影响 ===")

	fmt.Println("   ❌ 大量小对象：")
	fmt.Println("      1000 万个 16 字节的对象，一共才 160MB")
	fmt.Println("      但是 GC 标记要遍历 1000 万个对象，非常慢！")
	fmt.Println("      指针越多，标记越慢！")

	fmt.Println("\n   ✅ 优化：")
	fmt.Println("      1. 用数组代替单个对象，减少指针数量")
	fmt.Println("      2. 用 sync.Pool 复用")
	fmt.Println("      3. 能放栈上就放栈上（值类型，不逃逸）")

	fmt.Println("\n   ❌ 大对象（>32KB）：")
	fmt.Println("      直接从 mheap 分配，不走 P 的本地缓存")
	fmt.Println("      分配慢，还容易造成内存碎片")

	fmt.Println("\n   ✅ 优化：")
	fmt.Println("      大对象池化复用")
	fmt.Println("      避免频繁分配释放大对象")
}

// ============================================
// 演示 7：time.Ticker 泄漏导致内存泄漏
// ============================================
func demo7TickerLeak() {
	fmt.Println("\n=== 演示 7：time.Ticker 泄漏导致内存泄漏 ===")

	fmt.Println("   ❌ 错误写法：time.Tick 永远不会回收！")
	fmt.Println("   for range time.Tick(1 * time.Second) {")
	fmt.Println("       // ...")
	fmt.Println("   }")
	fmt.Println("   // 每个 time.Tick 背后都有一个 goroutine 在跑，泄漏了！")

	fmt.Println("\n   ✅ 正确写法：用完一定要 Stop！")
	fmt.Println("   ticker := time.NewTicker(1 * time.Second)")
	fmt.Println("   defer ticker.Stop()  // ✅ 一定要 Stop！")
	fmt.Println("   for range ticker.C {")
	fmt.Println("       // ...")
	fmt.Println("   }")

	fmt.Println("\n   💡 这是 Go 最常见的坑之一，90% 的项目都踩过！")
}

// ============================================
// 演示 8：finalizer 导致对象多活一轮 GC
// ============================================
func demo8Finalizer() {
	fmt.Println("\n=== 演示 8：finalizer 导致对象多活一轮 GC ===")

	fmt.Println("   ❌ 给对象加 finalizer：")
	fmt.Println("   runtime.SetFinalizer(obj, func(obj *Obj) {")
	fmt.Println("       // 清理资源")
	fmt.Println("   })")

	fmt.Println("\n   问题：")
	fmt.Println("   有 finalizer 的对象，第一次 GC 不会回收")
	fmt.Println("   只会把 finalizer 丢到一个 goroutine 去跑")
	fmt.Println("   下一轮 GC 才会真正回收")
	fmt.Println("   至少多活一个 GC 周期！")

	fmt.Println("\n   ✅ 最佳实践：")
	fmt.Println("   非必要不要用 finalizer")
	fmt.Println("   用完手动调用 Close()/Cleanup() 方法")
}

// ============================================
// 演示 9：GC 演进历史对比
// ============================================
func demo9GCEvolution() {
	fmt.Println("\n=== 演示 9：GC 演进历史对比 ===")

	fmt.Println("   ┌─────────┬─────────────┬─────────────┬──────────────────┐")
	fmt.Println("   │ Go 版本 │ 算法        │ 典型 STW    │ 里程碑事件       │")
	fmt.Println("   ├─────────┼─────────────┼─────────────┼──────────────────┤")
	fmt.Println("   │ 1.0     │ 串行标记清除 │ 几百ms~几秒 │ 第一个版本       │")
	fmt.Println("   │ 1.3     │ 并行标记清除 │ 几十~几百ms │ 并行标记清除     │")
	fmt.Println("   │ 1.5     │ 插入写屏障   │ 10~50 ms    │ 并发标记！革命性 │")
	fmt.Println("   │ 1.8     │ 混合写屏障   │ 0.5~2 ms    │ 不需要重扫栈！   │")
	fmt.Println("   │ 1.19    │ 混合写屏障   │ 0.1~1 ms    │ GOMEMLIMIT       │")
	fmt.Println("   └─────────┴─────────────┴─────────────┴──────────────────┘")

	fmt.Println("\n   💡 面试加分项：Go 1.8 是 GC 最大的一次提升，STW 降了 10 倍！")
	fmt.Println("   💡 Go 1.19 的 GOMEMLIMIT 是容器环境的救星！")
}

// ============================================
// 演示 10：pprof 排查 GC 问题
// ============================================
func demo10Pprof() {
	fmt.Println("\n=== 演示 10：pprof 排查 GC 问题 ===")

	fmt.Println("   启动 pprof 调试端口：")
	fmt.Println(`   import _ "net/http/pprof"`)
	fmt.Println(`   go func() {`)
	fmt.Println(`       _ = http.ListenAndServe(":6060", nil)`)
	fmt.Println(`   }()`)

	fmt.Println("\n   常用排查命令：")
	fmt.Println("")
	fmt.Println("   # 看堆内存，找哪些对象占的多")
	fmt.Println("   go tool pprof http://localhost:6060/debug/pprof/heap")
	fmt.Println("   > top           # 看占内存最多的函数")
	fmt.Println("   > list funcName # 看具体哪一行分配的")
	fmt.Println("")
	fmt.Println("   # 看 goroutine 泄漏")
	fmt.Println("   go tool pprof http://localhost:6060/debug/pprof/goroutine")
	fmt.Println("")
	fmt.Println("   # 看 GC 停顿时间")
	fmt.Println("   curl http://localhost:6060/debug/pprof/block?debug=1")
	fmt.Println("")
	fmt.Println("   # 看内存分配采样")
	fmt.Println("   go tool pprof http://localhost:6060/debug/pprof/allocs")

	fmt.Println("\n   💡 支付生产环境一定要开 pprof，出问题 1 分钟定位！")
}

// ============================================
// 演示 11：GC 调优黄金法则
// ============================================
func demo11GCOptimization() {
	fmt.Println("\n=== 演示 11：GC 调优黄金法则 ===")

	fmt.Println("   🏆 第一法则：最好的 GC 优化就是少分配！")
	fmt.Println("      没有对象 = 没有 GC 工作 = 没有 STW！")

	fmt.Println("\n   具体手段按优先级排序：")
	fmt.Println("")
	fmt.Println("   1. ✅ sync.Pool 复用对象（效果最大，最常用）")
	fmt.Println("   2. ✅ slice/map 预分配容量，避免扩容")
	fmt.Println("   3. ✅ 小结构体用值传递，不要用指针，尽量栈上分配")
	fmt.Println("   4. ✅ 避免频繁创建销毁大对象")
	fmt.Println("   5. ✅ 字符串拼接用 strings.Builder，不要 +")
	fmt.Println("   6. ✅ Go 1.19+ 开 GOMEMLIMIT")
	fmt.Println("   7. ✅ 延迟敏感调大 GOGC")
	fmt.Println("   8. ❌ 不要乱开 GC 相关的实验性参数")

	fmt.Println("\n   💡 Go GC 本身已经非常优秀了，90% 的场景不用调参数")
	fmt.Println("   💡 主要优化方向是：减少对象分配！")
}

// ============================================
// 演示 12：为什么 Go 不用分代 GC？（面试高频！）
// ============================================
func demo12WhyNotGenerational() {
	fmt.Println("\n=== 演示 12：为什么 Go 不用分代 GC？（面试高频！） ===")

	fmt.Println("   面试经常问：Java 都用分代 GC 了，Go 为什么不用？")

	fmt.Println("\n   答案：")
	fmt.Println("")
	fmt.Println("   1. 逃逸分析把大部分小对象都放栈上了")
	fmt.Println("      函数返回的对象不逃逸，直接在栈上分配")
	fmt.Println("      栈不需要 GC，函数返回直接回收")
	fmt.Println("      实际上堆上的对象很多都是长生命周期的")

	fmt.Println("\n   2. 分代 GC 的收益对 Go 不大")
	fmt.Println("      分代 GC 假设：大部分对象朝生夕死")
	fmt.Println("      但是 Go 因为逃逸分析，朝生夕死的对象大部分在栈上")
	fmt.Println("      堆上的对象大部分是长生命周期的")
	fmt.Println("      分代带来的收益不明显，反而增加写屏障开销")

	fmt.Println("\n   3. Go runtime 团队评估过，收益不如其他优化大")
	fmt.Println("      相比分代，混合写屏障、GOMEMLIMIT、分配器优化收益更大")

	fmt.Println("\n   💡 这是面试高频追问，一定要会！")
	fmt.Println("   💡 核心答点：逃逸分析 + 栈上分配 + 分代收益不大")
}

// ============================================
// main
// ============================================
func main() {
	// 让 unused import 不报错
	_ = http.ListenAndServe
	_ = debug.FreeOSMemory

	demo1ManualGC()
	demo2GCTrace()
	demo3GOGC()
	demo4GOMEMLIMIT()
	demo5SyncPool()
	demo6ObjectSize()
	demo7TickerLeak()
	demo8Finalizer()
	demo9GCEvolution()
	demo10Pprof()
	demo11GCOptimization()
	demo12WhyNotGenerational()

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ GC 三色标记所有演示完成！")
	fmt.Println("=== 核心总结 ===")
	fmt.Println("1. 三色标记：白→灰→黑，灰色队列为空标记结束")
	fmt.Println("2. 混合写屏障四条规则：栈涂黑、新对象黑、插入涂灰、删除涂灰")
	fmt.Println("3. 两次 STW：开启写屏障扫栈、关闭写屏障收尾，总共几百 us")
	fmt.Println("4. GOGC 调 GC 频率，Go 1.19+ GOMEMLIMIT 避免 OOM")
	fmt.Println("5. 最好的 GC 优化就是少分配对象！sync.Pool 是第一大杀器")
	fmt.Println("6. 为什么不用分代 GC？逃逸分析把大部分对象放栈上了，分代收益不大")
	fmt.Println("\n=== 支付业务最佳实践 ===")
	fmt.Println("✅ Go 1.19+ 必须开 GOMEMLIMIT = 容器 limit 的 90%")
	fmt.Println("✅ 延迟敏感业务 GOGC 调大到 200~500，减少 GC 频率")
	fmt.Println("✅ 高频使用的小对象一定用 sync.Pool 复用")
	fmt.Println("✅ 生产环境开 gctrace 和 pprof，出问题快速定位")
	fmt.Println("✅ time.Ticker 用完一定要 Stop！")
}
