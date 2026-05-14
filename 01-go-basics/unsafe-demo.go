package main

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

func main() {
	fmt.Println("=== 演示 1：任意类型指针互相转换 ===")
	demo1TypeConvert()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 2：[]byte ↔ string 零拷贝转换 ===")
	demo2ZeroCopy()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 3：通过偏移量访问结构体字段 ===")
	demo3StructOffset()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 4：指针运算遍历数组（无边界检查） ===")
	demo4PointerArithmetic()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("=== 演示 5：查看 slice 底层真实结构 ===")
	demo5SliceHeader()

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("⚠️  坑演示：uintptr 分两步可能导致野指针 ===")
	demo6UintptrPitfall()

	fmt.Println("\n✅ unsafe.Pointer 所有演示完成！")
}

// ============================================
// 演示 1：任意类型指针互相转换
// ============================================
func demo1TypeConvert() {
	var x int = 0x1234

	// *int → unsafe.Pointer → *[4]byte（把 int 当字节数组看）
	p := unsafe.Pointer(&x)
	bytes := (*[4]byte)(p)

	fmt.Printf("x = %#x\n", x)
	fmt.Printf("内存字节表示: [%#x, %#x, %#x, %#x]\n", bytes[0], bytes[1], bytes[2], bytes[3])
	fmt.Println("💡 小端模式：低字节存在低地址")
}

// ============================================
// 演示 2：[]byte ↔ string 零拷贝转换
// ============================================
func demo2ZeroCopy() {
	b := []byte{'h', 'e', 'l', 'l', 'o'}

	// 标准方式：会拷贝内存
	s1 := string(b)

	// unsafe 方式：零拷贝，复用同一个底层数组
	s2 := *(*string)(unsafe.Pointer(&b))

	fmt.Printf("标准转换 s1: %s\n", s1)
	fmt.Printf("unsafe 转换 s2: %s\n", s2)

	// 看底层数组指针，证明是同一个
	bAddr := (*reflect.SliceHeader)(unsafe.Pointer(&b)).Data
	s1Addr := (*reflect.StringHeader)(unsafe.Pointer(&s1)).Data
	s2Addr := (*reflect.StringHeader)(unsafe.Pointer(&s2)).Data

	fmt.Printf("b 底层数组地址: %x\n", bAddr)
	fmt.Printf("s1 底层数组地址: %x (不一样，拷贝了)\n", s1Addr)
	fmt.Printf("s2 底层数组地址: %x (和 b 一样，零拷贝！)\n", s2Addr)

	fmt.Println("\n⚠️  注意：零拷贝后绝对不能修改 b，否则 string 的不可变性被破坏")
}

// ============================================
// 演示 3：通过偏移量访问结构体字段
// ============================================
func demo3StructOffset() {
	type User struct {
		Name string
		Age  int
	}

	u := User{Name: "Alice", Age: 18}

	// 计算字段偏移量
	nameOffset := unsafe.Offsetof(u.Name)
	ageOffset := unsafe.Offsetof(u.Age)

	fmt.Printf("Name 字段偏移: %d 字节\n", nameOffset)
	fmt.Printf("Age 字段偏移: %d 字节\n", ageOffset)

	// 通过偏移量直接修改字段
	uAddr := unsafe.Pointer(&u)
	ageP := (*int)(unsafe.Pointer(uintptr(uAddr) + ageOffset))
	*ageP = 20 // 直接改 Age

	fmt.Printf("修改后的 Age: %d\n", u.Age)
	fmt.Println("💡 反射库底层就是这么实现的")
}

// ============================================
// 演示 4：指针运算遍历数组（无边界检查）
// ============================================
func demo4PointerArithmetic() {
	arr := []int{10, 20, 30, 40, 50}
	base := unsafe.Pointer(&arr[0])
	elemSize := unsafe.Sizeof(arr[0])

	fmt.Printf("数组元素大小: %d 字节\n", elemSize)

	// 用指针运算遍历，没有边界检查！
	for i := 0; i < len(arr); i++ {
		elemAddr := unsafe.Pointer(uintptr(base) + uintptr(i)*elemSize)
		fmt.Printf("arr[%d] = %d\n", i, *(*int)(elemAddr))
	}

	fmt.Println("💡 没有边界检查，性能更高，但越界了也不会报错，直接读垃圾值")
}

// ============================================
// 演示 5：查看 slice 底层真实结构
// ============================================
func demo5SliceHeader() {
	s := make([]int, 3, 8)
	s[0], s[1], s[2] = 1, 2, 3

	// 把 slice 强转成 SliceHeader，看真实字段
	hdr := (*reflect.SliceHeader)(unsafe.Pointer(&s))

	fmt.Printf("底层数组地址: %x\n", hdr.Data)
	fmt.Printf("真实 len: %d\n", hdr.Len)
	fmt.Printf("真实 cap: %d\n", hdr.Cap)
	fmt.Println("💡 这就是 slice 真正的样子：指针 + len + cap")
}

// ============================================
// 演示 6：uintptr 分两步的坑
// ============================================
func demo6UintptrPitfall() {
	fmt.Println("这个坑不会直接崩溃，但原理很重要：")
	fmt.Println("")
	fmt.Println("❌ 错误写法：")
	fmt.Println("  addr := uintptr(unsafe.Pointer(obj))  // 把地址存成整数")
	fmt.Println("  // 这里 obj 已经没有引用了，GC 可能把它回收！")
	fmt.Println("  p := unsafe.Pointer(addr)             // 野指针！")
	fmt.Println("")
	fmt.Println("✅ 正确写法：")
	fmt.Println("  p := unsafe.Pointer(uintptr(obj) + offset)  // 同一表达式完成")
	fmt.Println("")
	fmt.Println("💡 核心：uintptr 只是个数字，GC 不会认为它引用了对象")
}
