package main

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

// ============================================
// 零拷贝转换函数
// ============================================
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func StringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(&struct {
		string
		int
	}{s, len(s)}))
}

// ============================================
// 演示 1：底层结构对比
// ============================================
func demo1Struct() {
	fmt.Println("=== 演示 1：string vs []byte 底层结构 ===")

	s := "hello world"
	b := []byte("hello world")

	sHeader := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bHeader := (*reflect.SliceHeader)(unsafe.Pointer(&b))

	fmt.Printf("string: Data=0x%x, Len=%d\n", sHeader.Data, sHeader.Len)
	fmt.Printf("[]byte: Data=0x%x, Len=%d, Cap=%d\n", bHeader.Data, bHeader.Len, bHeader.Cap)
	fmt.Println("💡 string 只有指针+长度，slice 多了一个 Cap 字段")
	fmt.Println()
}

// ============================================
// 演示 2：普通转换 vs 零拷贝转换（内存地址对比）
// ============================================
func demo2Convert() {
	fmt.Println("=== 演示 2：普通转换 vs 零拷贝转换 ===")

	s := "hello world"
	sHeader := (*reflect.StringHeader)(unsafe.Pointer(&s))

	// 普通转换：有拷贝
	b1 := []byte(s)
	b1Header := (*reflect.SliceHeader)(unsafe.Pointer(&b1))

	// 零拷贝转换：没有拷贝
	b2 := StringToBytes(s)
	b2Header := (*reflect.SliceHeader)(unsafe.Pointer(&b2))

	fmt.Printf("原 string  地址: 0x%x\n", sHeader.Data)
	fmt.Printf("普通转换后地址: 0x%x (不一样！有拷贝)\n", b1Header.Data)
	fmt.Printf("零拷贝后地址  : 0x%x (完全一样！没有拷贝)\n", b2Header.Data)
	fmt.Println()
}

// ============================================
// 演示 3：for range 遍历是按 rune，不是按 byte
// ============================================
func demo3ForRange() {
	fmt.Println("=== 演示 3：for range 遍历不是按 byte！ ===")

	s := "你好世界"
	fmt.Printf("字符串: %s，len=%d（UTF-8 每个中文占 3 字节）\n", s, len(s))

	fmt.Println("for range 遍历（按 rune）:")
	for i, c := range s {
		fmt.Printf("  i=%d, c=%c\n", i, c)
	}

	fmt.Println("for i 遍历（按 byte）:")
	for i := 0; i < len(s); i++ {
		fmt.Printf("  i=%d, b=0x%02x\n", i, s[i])
	}

	fmt.Println("💡 注意：for range 的下标不是连续的！0 → 3 → 6 → 9")
	fmt.Println("   很多人在这里踩坑，以为下标是 +1 递增的")
	fmt.Println()
}

// ============================================
// 演示 4：子字符串共享底层数组
// ============================================
func demo4SharedArray() {
	fmt.Println("=== 演示 4：子字符串共享底层数组 ===")

	s := "hello world"
	s1 := s[:5] // "hello"
	s2 := s[6:] // "world"

	sHeader := (*reflect.StringHeader)(unsafe.Pointer(&s))
	s1Header := (*reflect.StringHeader)(unsafe.Pointer(&s1))
	s2Header := (*reflect.StringHeader)(unsafe.Pointer(&s2))

	fmt.Printf("原字符串 地址: 0x%x\n", sHeader.Data)
	fmt.Printf("s[:5]    地址: 0x%x (和原字符串一样！)\n", s1Header.Data)
	fmt.Printf("s[6:]    地址: 0x%x (和原字符串差 6 字节！)\n", s2Header.Data)

	fmt.Println("💡 三个 string 共享同一个底层字节数组，只拷贝指针和长度，没有拷贝内容")
	fmt.Println("   这就是为什么字符串切片操作非常快，O(1) 复杂度")
	fmt.Println()
}

// ============================================
// 演示 5：零拷贝转换后修改会 panic
// ============================================
func demo5ModifyPanic() {
	fmt.Println("=== 演示 5：零拷贝转换后修改会 panic ===")

	s := "hello world"
	b := StringToBytes(s)

	fmt.Println("注意：零拷贝转换得到的 []byte 绝对不能修改！")
	fmt.Println("现在尝试修改 b[0] = 'H'...")

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("✅ 果然 panic 了: %v\n", r)
			fmt.Println("   原因：string 的底层数组是只读内存页，修改会触发页错误")
		}
	}()

	// 这行一定会 panic
	b[0] = 'H'
	fmt.Println("你永远看不到这行输出")
	fmt.Println()
}

// ============================================
// 演示 6：空字符串 vs nil []byte
// ============================================
func demo6ZeroValue() {
	fmt.Println("=== 演示 6：空字符串 vs nil []byte ===")

	var s string
	var b []byte

	sHeader := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bHeader := (*reflect.SliceHeader)(unsafe.Pointer(&b))

	fmt.Printf("空 string: len=%d, Data=0x%x (nil)\n", len(s), sHeader.Data)
	fmt.Printf("nil []byte: len=%d, Data=0x%x (nil)\n", len(b), bHeader.Data)
	fmt.Println("💡 两者底层指针都是 nil，但类型不一样，一个是 string，一个是 slice")
	fmt.Println()
}

// ============================================
// 演示 7：传参成本极低
// ============================================
func demo7ParamCost() {
	fmt.Println("=== 演示 7：传参成本极低 ===")

	// 1MB 的大字符串
	s := strings.Repeat("x", 1024*1024)
	fmt.Printf("原字符串 len=%d (1MB)\n", len(s))

	// 传参只拷贝 16 字节的结构体头，不拷贝 1MB 内容
	processString(s)

	fmt.Println("💡 即使是 1GB 的字符串，传参也只拷贝 16 字节")
	fmt.Println("   很多人以为传大字符串会很慢，其实不会")
	fmt.Println()
}

func processString(s string) {
	fmt.Printf("  函数收到的字符串 len=%d，传参只拷贝了 16 字节的结构体头\n", len(s))
}

// ============================================
// main
// ============================================
func main() {
	demo1Struct()
	demo2Convert()
	demo3ForRange()
	demo4SharedArray()
	demo5ModifyPanic()
	demo6ZeroValue()
	demo7ParamCost()

	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有演示完成！核心要点：")
	fmt.Println("1. string 是 2 字段结构体，[]byte 是 3 字段")
	fmt.Println("2. 普通转换有拷贝，零拷贝转换没有但绝对不能改")
	fmt.Println("3. for range 遍历 string 是按 rune，不是按 byte")
	fmt.Println("4. 子字符串共享底层数组，切片操作 O(1)")
	fmt.Println("5. 大字符串传参成本极低，只拷 16 字节")
}
