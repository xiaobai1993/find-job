package main

import (
	"fmt"
	"strings"
	"time"
)

// ============================================
// 演示 1：range 返回的是值拷贝，修改不生效
// ============================================
func demo1ValueCopy() {
	fmt.Println("=== 演示 1：range 返回的是值拷贝 ===")
	nums := []int{1, 2, 3}

	// ❌ 错误写法：修改的是拷贝的值
	for _, v := range nums {
		v *= 10
	}
	fmt.Println("❌ 修改拷贝的值，原数组不变:", nums)

	// ✅ 正确写法：用下标访问原数组
	for i := range nums {
		nums[i] *= 10
	}
	fmt.Println("✅ 用下标修改原数组，生效:", nums)
}

// ============================================
// 演示 2：取遍历变量的地址，都是同一个
// ============================================
func demo2SameAddress() {
	fmt.Println("\n=== 演示 2：取遍历变量的地址，都是同一个 ===")
	nums := []int{1, 2, 3}
	var addrs []*int

	// ❌ 错误写法：取的是遍历变量 v 的地址
	for _, v := range nums {
		addrs = append(addrs, &v)
	}

	fmt.Print("❌ 所有地址指向同一个变量，值都是: ")
	for _, p := range addrs {
		fmt.Print(*p, " ")
	}
	fmt.Println()

	// ✅ 正确写法 1：取原数组元素的地址
	addrs = nil
	for i := range nums {
		addrs = append(addrs, &nums[i])
	}
	fmt.Print("✅ 取原数组元素地址，值正确: ")
	for _, p := range addrs {
		fmt.Print(*p, " ")
	}
	fmt.Println()

	// ✅ 正确写法 2：循环内新建变量
	addrs = nil
	for _, v := range nums {
		v := v // 每次循环新建一个局部变量
		addrs = append(addrs, &v)
	}
	fmt.Print("✅ 循环内新建变量，值正确: ")
	for _, p := range addrs {
		fmt.Print(*p, " ")
	}
	fmt.Println()
}

// ============================================
// 演示 3：goroutine 里的 range 变量坑
// ============================================
func demo3Goroutine() {
	fmt.Println("\n=== 演示 3：goroutine 里的 range 变量坑 ===")
	nums := []int{1, 2, 3}

	fmt.Println("❌ 错误写法：所有 goroutine 共享同一个 v，最后都打印 3")
	done := make(chan struct{}, 3)
	for _, v := range nums {
		go func() {
			fmt.Print(v, " ")
			done <- struct{}{}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Println()

	// ✅ 正确写法：把 v 作为参数传进去
	fmt.Println("✅ 正确写法：把 v 作为参数传进去，值正确")
	for _, v := range nums {
		go func(v int) {
			fmt.Print(v, " ")
			done <- struct{}{}
		}(v)
	}
	time.Sleep(100 * time.Millisecond)
	fmt.Println()
}

// ============================================
// 演示 4：遍历 string 返回 rune，下标不连续
// ============================================
func demo4StringRune() {
	fmt.Println("\n=== 演示 4：遍历 string 返回 rune，下标不连续 ===")
	s := "你好世界"

	for i, c := range s {
		fmt.Printf("i=%d, c=%c\n", i, c)
	}
	fmt.Println("💡 注意：下标不是 0,1,2,3，而是 0,3,6,9！每个中文占 3 字节")
}

// ============================================
// 演示 5：map 遍历顺序随机
// ============================================
func demo5MapRandom() {
	fmt.Println("\n=== 演示 5：map 遍历顺序随机 ===")
	m := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}

	fmt.Println("第一次遍历:")
	for k, v := range m {
		fmt.Printf("%s=%d ", k, v)
	}
	fmt.Println()

	fmt.Println("第二次遍历（顺序不一样）:")
	for k, v := range m {
		fmt.Printf("%s=%d ", k, v)
	}
	fmt.Println()
	fmt.Println("💡 Go 故意设计成随机，就是为了让大家不要依赖遍历顺序")
}

// ============================================
// 演示 6：遍历 channel，必须关闭
// ============================================
func demo6Channel() {
	fmt.Println("\n=== 演示 6：遍历 channel，必须关闭 ===")

	ch := make(chan int, 3)

	// 发送端
	go func() {
		for i := 1; i <= 3; i++ {
			ch <- i
			fmt.Println("发送:", i)
		}
		close(ch) // ✅ 发完一定要关闭！不然后面 for range 会死锁
		fmt.Println("channel 已关闭")
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Println("开始遍历 channel:")

	// 接收端
	for v := range ch {
		fmt.Println("收到:", v)
	}

	fmt.Println("✅ channel 关闭，遍历自动结束")
}

// ============================================
// 演示 7：遍历 nil 类型不会 panic
// ============================================
func demo7Nil() {
	fmt.Println("\n=== 演示 7：遍历 nil 类型不会 panic ===")

	var s []int          // nil slice
	var m map[string]int // nil map

	fmt.Println("开始遍历 nil slice...")
	for range s {
		fmt.Println("这行永远不会执行")
	}
	fmt.Println("✅ nil slice 遍历完成，没有 panic")

	fmt.Println("开始遍历 nil map...")
	for range m {
		fmt.Println("这行永远不会执行")
	}
	fmt.Println("✅ nil map 遍历完成，没有 panic")

	fmt.Println("\n⚠️  注意：遍历 nil channel 会永久阻塞，不会 panic")
	fmt.Println("  var ch chan int")
	fmt.Println("  for range ch {}  // 永远阻塞！")
}

// ============================================
// 演示 8：大结构体遍历性能问题
// ============================================
func demo8BigStruct() {
	fmt.Println("\n=== 演示 8：大结构体遍历性能问题 ===")

	type BigStruct struct {
		ID   int
		Data [1024]byte // 1KB
	}

	// 只是演示，不需要实际创建
	fmt.Println("❌ 错误写法：遍历大结构体，每次拷贝 1KB，1000 次就是 1MB")
	fmt.Println("  for _, v := range slice {")
	fmt.Println("      process(v)  // 每次都拷贝整个结构体")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法 1：用下标访问，只拷贝指针")
	fmt.Println("  for i := range slice {")
	fmt.Println("      process(&slice[i])  // 只拷贝 8 字节指针")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法 2：直接存指针")
	fmt.Println("  var slice []*BigStruct")
	fmt.Println("  for _, v := range slice {")
	fmt.Println("      process(v)  // 只拷贝 8 字节指针")
	fmt.Println("  }")
}

// ============================================
// 演示 9：遍历数组会拷贝整个数组
// ============================================
func demo9ArrayCopy() {
	fmt.Println("\n=== 演示 9：遍历数组会拷贝整个数组 ===")

	// 只是演示，不需要实际创建
	fmt.Println("❌ 错误写法：遍历数组，整个数组会被拷贝一次，4MB！")
	fmt.Println("  for _, v := range arr {")
	fmt.Println("      ...  // 先拷贝 4MB 的数组，再遍历")
	fmt.Println("  }")

	fmt.Println("\n✅ 正确写法：转成 slice 再遍历，只拷贝 slice 头 24 字节")
	fmt.Println("  for _, v := range arr[:] {")
	fmt.Println("      ...  // 没有数据拷贝")
	fmt.Println("  }")
}

// ============================================
// 演示 10：遍历过程中 slice 扩容，遍历次数不变
// ============================================
func demo10SliceGrow() {
	fmt.Println("\n=== 演示 10：遍历过程中 slice 扩容，遍历次数不变 ===")

	s := []int{1, 2, 3}

	fmt.Println("开始遍历，初始 len =", len(s))

	count := 0
	for i, v := range s {
		count++
		fmt.Printf("第 %d 次循环: i=%d, v=%d, len(s)=%d\n", count, i, v, len(s))

		// 遍历过程中往 slice 里加元素
		s = append(s, v*10)
	}

	fmt.Printf("✅ 循环结束，共执行 %d 次，最终 len(s)=%d\n", count, len(s))
	fmt.Println("💡 range 开始的时候就保存了 len，后面扩容不影响遍历次数")
}

// ============================================
// main
// ============================================
func main() {
	demo1ValueCopy()
	demo2SameAddress()
	demo3Goroutine()
	demo4StringRune()
	demo5MapRandom()
	demo6Channel()
	demo7Nil()
	demo8BigStruct()
	demo9ArrayCopy()
	demo10SliceGrow()

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有 range 演示完成！")
	fmt.Println("核心总结：")
	fmt.Println("1. range 返回的是值拷贝，修改不生效")
	fmt.Println("2. 不要取遍历变量的地址，永远是同一个")
	fmt.Println("3. goroutine 里不要直接用 range 变量，要作为参数传进去")
	fmt.Println("4. map 遍历顺序随机，不要依赖")
	fmt.Println("5. for range channel 必须关闭，不然死锁")
	fmt.Println("6. 大结构体用下标访问，大数组转 slice 再遍历")
	fmt.Println("7. 遍历过程中 slice 扩容不影响遍历次数")
}
