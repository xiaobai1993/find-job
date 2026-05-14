package main

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

func main() {
	fmt.Println("=== 演示 1：copy 基本用法 ===")
	demo1BasicCopy()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 2：不 copy vs copy — 共享 vs 独立 ===")
	demo2SharedVsIndependent()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 3：copy 的坑 — dst 长度为 0 ===")
	demo3CopyPitfall()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 4：用 copy 删除元素 ===")
	demo4DeleteWithCopy()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 5：copy 截断关联，让大数组被 GC ===")
	demo5GC()

	fmt.Println("\n✅ copy 所有演示完成！")
}

// ============================================
// 演示 1：copy 基本用法
// ============================================
func demo1BasicCopy() {
	src := []int{1, 2, 3}
	dst := make([]int, len(src)) // ✅ 必须分配长度！

	n := copy(dst, src)

	fmt.Printf("src: %v\n", src)
	fmt.Printf("dst: %v (复制了 %d 个元素)\n", dst, n)

	// 改 dst，src 不受影响
	dst[0] = 100
	fmt.Printf("改 dst[0]=100 后:\n")
	fmt.Printf("src: %v (没变！)\n", src)
	fmt.Printf("dst: %v\n", dst)

	fmt.Println("\n💡 copy 之后两个 slice 有独立的底层数组，互不影响")
}

// ============================================
// 演示 2：不 copy vs copy
// ============================================
func demo2SharedVsIndependent() {
	origin := []int{1, 2, 3, 4, 5}

	// 方式 1：直接切片 — 共享底层数组
	sub1 := origin[1:4]

	// 方式 2：copy — 独立底层数组
	sub2 := make([]int, 3)
	copy(sub2, origin[1:4])

	fmt.Printf("原数组 origin: %v\n", origin)
	fmt.Printf("直接切片 sub1: %v\n", sub1)
	fmt.Printf("copy 出来 sub2: %v\n", sub2)

	// 看底层指针
	fmt.Printf("\n底层数组指针:\n")
	fmt.Printf("origin 指针: %v\n", getSlicePointer(origin))
	fmt.Printf("sub1 指针:   %v (和 origin 一样！)\n", getSlicePointer(sub1))
	fmt.Printf("sub2 指针:   %v (不一样！)\n", getSlicePointer(sub2))

	// 改 sub1
	sub1[0] = 999
	fmt.Printf("\n改 sub1[0]=999 后:\n")
	fmt.Printf("origin: %v (被影响了！)\n", origin)
	fmt.Printf("sub2:   %v (不受影响)\n", sub2)

	fmt.Println("\n💡 直接切片 = 共享底层数组，改一个影响另一个")
	fmt.Println("   copy = 独立数组，互不影响")
}

// ============================================
// 演示 3：copy 的坑 — dst 长度为 0
// ============================================
func demo3CopyPitfall() {
	src := []int{1, 2, 3}

	// ❌ 错误写法：只分配了容量，长度还是 0
	dst1 := make([]int, 0, 3)
	n1 := copy(dst1, src)
	fmt.Printf("dst1 (make 0,3): 复制了 %d 个元素，结果: %v (空的！)\n", n1, dst1)

	// ✅ 正确写法：分配长度
	dst2 := make([]int, 3)
	n2 := copy(dst2, src)
	fmt.Printf("dst2 (make 3):   复制了 %d 个元素，结果: %v (正确)\n", n2, dst2)

	fmt.Println("\n💡 超级重要的坑：copy 看的是 len(dst)，不是 cap(dst)！")
	fmt.Println("   dst len 是多少，最多就复制多少个元素")
}

// ============================================
// 演示 4：用 copy 删除元素
// ============================================
func demo4DeleteWithCopy() {
	s := []int{1, 2, 3, 4, 5}
	i := 2 // 删除索引为 2 的元素（值为 3）

	fmt.Printf("删除前: %v\n", s)

	// 把 i 后面的元素往前覆盖一位
	copy(s[i:], s[i+1:])
	s = s[:len(s)-1]

	fmt.Printf("删除索引 %d 后: %v\n", i, s)
	fmt.Println("\n💡 这是标准的 slice 删除元素写法，不扩容，效率最高")
}

// ============================================
// 演示 5：copy 截断关联，让大数组被 GC
// ============================================
func demo5GC() {
	// 场景：从大切片里只取前 3 个元素
	big := make([]int, 1000000)
	big[0], big[1], big[2] = 1, 2, 3

	// ❌ 直接切片 — big 的底层数组永远不会被 GC，因为 small 还引用着
	small1 := big[:3]

	// ✅ copy 出来 — big 没人用了就会被 GC 回收
	small2 := make([]int, 3)
	copy(small2, big[:3])

	fmt.Printf("small1 (直接切): len=%d, cap=%d, 指针=%v\n",
		len(small1), cap(small1), getSlicePointer(small1))
	fmt.Printf("small2 (copy 出): len=%d, cap=%d, 指针=%v\n",
		len(small2), cap(small2), getSlicePointer(small2))

	fmt.Printf("\nbig 的指针:      %v\n", getSlicePointer(big))
	fmt.Println("small1 和 big 指针一样，共享数组")
	fmt.Println("small2 指针不一样，独立数组，big 可以被 GC")

	fmt.Println("\n💡 生产环境常见优化：大切片只取小部分时，一定要 copy")
	fmt.Println("   否则整个大数组都占着内存不释放！")
}

func getSlicePointer(s []int) uintptr {
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return hdr.Data
}
