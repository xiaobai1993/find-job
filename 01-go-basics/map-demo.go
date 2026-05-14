package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

func main() {
	fmt.Println("=== 演示 1：nil map 读写行为 ===")
	demo1NilMap()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 2：map 传参 — 函数内修改外面能看到 ===")
	demo2FuncParam()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 3：遍历随机性 ===")
	demo3RandomIter()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 4：delete 不释放内存 ===")
	demo4DeleteMemory()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 5：并发读写直接 panic ===")
	demo5Concurrent()

	fmt.Println("\n✅ map 所有演示完成！")
}

// ============================================
// 演示 1：nil map 读写行为
// ============================================
func demo1NilMap() {
	var m map[int]int // nil map

	fmt.Println("nil map 的 len:", len(m)) // 0，没问题

	fmt.Println("读 nil map:")
	v := m[1] // ✅ 不会 panic
	fmt.Printf("  m[1] = %d\n", v)

	fmt.Println("delete nil map:")
	delete(m, 1) // ✅ 不会 panic，什么都不做
	fmt.Println("  完成，没 panic")

	fmt.Println("⚠️  现在尝试写 nil map（会 panic，用 recover 接住）:")
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("  果然 panic 了: %v\n", r)
		}
	}()
	m[1] = 10 // ❌ 写 nil map → panic！
}

// ============================================
// 演示 2：map 传参 — 函数内修改外面能看到
// ============================================
func demo2FuncParam() {
	m := make(map[int]int)
	m[1] = 10

	fmt.Printf("调用函数前: m[1] = %d\n", m[1])

	modifyMap(m) // 传值，但是是指针拷贝

	fmt.Printf("调用函数后: m[1] = %d (被修改了！)\n", m[1])

	fmt.Println("\n💡 和 slice 不一样！map 传的是 *hmap 指针，")
	fmt.Println("   不管是改元素还是触发扩容，外面 100% 能看到")
}

func modifyMap(m map[int]int) {
	m[1] = 999
	// 即使触发扩容了，外面也能看到
	for i := 2; i < 100; i++ {
		m[i] = i
	}
}

// ============================================
// 演示 3：遍历随机性
// ============================================
func demo3RandomIter() {
	m := make(map[int]int)
	for i := 0; i < 5; i++ {
		m[i] = i
	}

	fmt.Println("连续遍历 3 次，顺序都不一样：")
	for i := 0; i < 3; i++ {
		fmt.Printf("  第 %d 次: ", i+1)
		for k := range m {
			fmt.Printf("%d ", k)
		}
		fmt.Println()
	}

	fmt.Println("\n💡 Go 故意设计成随机顺序，防止用户依赖遍历顺序")
}

// ============================================
// 演示 4：delete 不释放内存
// ============================================
func demo4DeleteMemory() {
	const N = 1000000
	m := make(map[int]int, N)

	// 插入 100 万个 key
	for i := 0; i < N; i++ {
		m[i] = i
	}

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	fmt.Printf("插入 100 万 key 后: len=%d, 堆内存占用 ~%.1f MB\n",
		len(m), float64(m1.Alloc)/1024/1024)

	// 逐个 delete 所有 key
	for k := range m {
		delete(m, k)
	}

	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	fmt.Printf("delete 所有 key 后: len=%d, 堆内存占用 ~%.1f MB\n",
		len(m), float64(m2.Alloc)/1024/1024)

	fmt.Println("\n💡 len 变成 0 了，但内存几乎没释放！")
	fmt.Println("   delete 只是标记为空，bucket 和溢出桶还在，")
	fmt.Println("   只有整个 map 被 GC 回收了才会释放内存")

	// ✅ 正确做法：把 map 置为 nil，让 GC 回收整个 map
	m = nil
	runtime.GC()
	var m3 runtime.MemStats
	runtime.ReadMemStats(&m3)
	fmt.Printf("\n把 map 置为 nil 后: 堆内存占用 ~%.1f MB\n",
		float64(m3.Alloc)/1024/1024)
	fmt.Println("✅ 内存真正释放了！")
}

// ============================================
// 演示 5：并发读写直接 panic
// ============================================
func demo5Concurrent() {
	m := make(map[int]int)

	fmt.Println("启动 2 个 goroutine，一个读一个写...")

	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("✅ 果然 panic 了: %v\n", r)
				done <- true
			}
		}()
		for {
			m[1] = 1 // 写
		}
	}()

	go func() {
		for {
			_ = m[1] // 读
		}
	}()

	// 等 panic
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		fmt.Println("没触发 panic（概率问题，再跑一次大概率会触发）")
	}

	fmt.Println("\n💡 不是竞争检测，是 runtime 故意设计的：")
	fmt.Println("   写操作会设置写标志位，读的时候检查到就直接 panic")
}
