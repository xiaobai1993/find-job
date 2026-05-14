package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

// ============================================
// Slice 各种场景演示
// ============================================

func main() {
	fmt.Println("=== 演示 1：数组 vs 切片传参 ===")
	demo1ArrayVsSlice()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 2：函数传参，改元素 vs append ===")
	demo2FuncParam()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 3：切片共享底层数组 ===")
	demo3SharedArray()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 4：append 没扩容时覆盖原数组 ===")
	demo4AppendNoGrow()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 5：nil slice vs 空 slice ===")
	demo5NilVsEmpty()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ 所有演示完成！看上面的输出理解差异")
}

// ============================================
// 演示 1：数组 vs 切片传参
// ============================================
func demo1ArrayVsSlice() {
	// 数组
	arr := [3]int{1, 2, 3}
	modifyArray(arr)
	fmt.Printf("数组传参后，原数组没变: %v\n", arr) // 还是 [1 2 3]

	// 切片
	s := []int{1, 2, 3}
	modifySlice(s)
	fmt.Printf("切片传参后，改元素能看到: %v\n", s) // 变成 [100 2 3]
}

func modifyArray(a [3]int) {
	a[0] = 100 // 改的是拷贝，外面看不到
}

func modifySlice(s []int) {
	s[0] = 100 // 改的是同一个底层数组，外面能看到
}

// ============================================
// 演示 2：函数传参，改元素 vs append
// ============================================
func demo2FuncParam() {
	s := []int{1} // len=1, cap=1
	fmt.Printf("调用前: len=%d, cap=%d, s=%v\n", len(s), cap(s), s)

	result := modifyAndAppend(s)
	fmt.Printf("调用后，原 slice: len=%d, cap=%d, s=%v\n", len(s), cap(s), s)
	fmt.Printf("调用后，返回的 slice: len=%d, cap=%d, result=%v\n", len(result), cap(result), result)

	fmt.Println("\n💡 说明：append 触发了扩容，分配了新数组，所以原 slice 看不到新元素")
	fmt.Println("   必须把新 slice return 出来才能用")
}

func modifyAndAppend(s []int) []int {
	s[0] = 100                // 改元素，外面能看到
	s = append(s, 2)          // 扩容了！s 的指针变了，外面看不到
	return s                   // ✅ 必须 return
}

// ============================================
// 演示 3：切片共享底层数组
// ============================================
func demo3SharedArray() {
	a := [5]int{1, 2, 3, 4, 5}
	s1 := a[1:3] // [2, 3]
	s2 := a[2:4] // [3, 4]

	fmt.Printf("原数组: %v\n", a)
	fmt.Printf("s1 = a[1:3]: %v\n", s1)
	fmt.Printf("s2 = a[2:4]: %v\n", s2)

	// 改 s1 的元素
	s1[1] = 999
	fmt.Println("\n执行 s1[1] = 999 后:")
	fmt.Printf("原数组: %v\n", a)  // 变了！
	fmt.Printf("s2: %v\n", s2)    // 也变了！

	fmt.Println("\n💡 说明：两个切片共享同一个底层数组，改一个影响另一个")
}

// ============================================
// 演示 4：append 没扩容时，覆盖原数组后面的元素
// ============================================
func demo4AppendNoGrow() {
	a := [5]int{1, 2, 3, 4, 5}
	s1 := a[1:3] // [2, 3], len=2, cap=4（从下标1到数组末尾有4个位置）

	fmt.Printf("原数组: %v\n", a)
	fmt.Printf("s1: len=%d, cap=%d, value=%v\n", len(s1), cap(s1), s1)

	// s1 cap=4，还有空间，append 不会扩容，直接写到原数组后面！
	s1 = append(s1, 999)

	fmt.Println("\n执行 s1 = append(s1, 999) 后:")
	fmt.Printf("原数组: %v\n", a)  // 变成 [1 2 3 999 5]！原数组第4位被覆盖了
	fmt.Printf("s1: %v\n", s1)

	fmt.Println("\n💡 说明：append 没扩容时，写的还是原底层数组")
	fmt.Println("   这是最隐蔽的坑！")
}

// ============================================
// 演示 5：nil slice vs 空 slice
// ============================================
func demo5NilVsEmpty() {
	var s1 []int          // nil slice
	s2 := []int{}         // 空 slice
	s3 := make([]int, 0)  // 空 slice

	fmt.Printf("s1 (nil): len=%d, cap=%d, is nil=%v\n", len(s1), cap(s1), s1 == nil)
	fmt.Printf("s2 ({}): len=%d, cap=%d, is nil=%v\n", len(s2), cap(s2), s2 == nil)
	fmt.Printf("s3 (make): len=%d, cap=%d, is nil=%v\n", len(s3), cap(s3), s3 == nil)

	// JSON 序列化差异
	j1, _ := json.Marshal(s1)
	j2, _ := json.Marshal(s2)
	j3, _ := json.Marshal(s3)
	fmt.Printf("\nJSON 序列化差异:\n")
	fmt.Printf("s1 JSON: %s\n", j1)  // null
	fmt.Printf("s2 JSON: %s\n", j2)  // []
	fmt.Printf("s3 JSON: %s\n", j3)  // []

	// 看底层结构（用 unsafe 看真实字段）
	fmt.Printf("\n底层指针（array 字段）:\n")
	fmt.Printf("s1 array pointer: %v\n", getSlicePointer(s1))  // 0x0（nil）
	fmt.Printf("s2 array pointer: %v\n", getSlicePointer(s2))  // 有地址，不是 nil
	fmt.Printf("s3 array pointer: %v\n", getSlicePointer(s3))  // 有地址，不是 nil

	fmt.Println("\n💡 面试高频：nil 和空 slice 的 JSON 序列化不一样！")
}

// 用 unsafe 获取 slice 的 array 指针
func getSlicePointer(s []int) uintptr {
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return hdr.Data
}
