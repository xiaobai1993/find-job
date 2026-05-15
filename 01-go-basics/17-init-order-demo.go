// Go 包初始化顺序与 init() 函数演示
// go run 17-init-order-demo.go
package main

import (
	"fmt"
)

// ============================================================
// 1. 包级变量初始化顺序
// ============================================================

var a = func() int {
	fmt.Println("初始化变量 a")
	return 10
}()

var b = func() int {
	fmt.Println("初始化变量 b")
	return a + 5
}()

var c = func() int {
	fmt.Println("初始化变量 c")
	return b + 5
}()

// ============================================================
// 2. 多个 init() 函数
// ============================================================

func init() {
	fmt.Println("第一个 init() 执行")
}

func init() {
	fmt.Println("第二个 init() 执行")
}

func init() {
	fmt.Println("第三个 init() 执行")
}

// ============================================================
// 3. 演示循环导入会编译错误
// ============================================================

// 如果你创建另一个包互相 import:
// package pkgA
// import "pkgB"
//
// package pkgB
// import "pkgA"
// 编译报错: import cycle not allowed

// ============================================================
// 4. 演示 init() 里 panic 会缓存
// ============================================================

func demoInitPanic() {
	fmt.Println("\n=== 4. init() 里的错误处理 ===")
	fmt.Println("⚠️  init() 里的 panic 非常难调试，复杂初始化尽量放 main 里!")
}

// ============================================================
// main
// ============================================================

func main() {
	fmt.Println("=== 初始化顺序演示 ===")
	fmt.Println("\n注意看上面的输出顺序:")
	fmt.Println("1. 包级变量按声明顺序初始化 (a → b → c)")
	fmt.Println("2. 然后按声明顺序执行所有 init() 函数")
	fmt.Println("3. 最后执行 main() 函数")

	fmt.Println("\n=== 变量值 ===")
	fmt.Printf("a = %d\n", a)
	fmt.Printf("b = %d\n", b)
	fmt.Printf("c = %d\n", c)

	demoInitPanic()

	fmt.Println("\n✅ 所有示例运行完成！")
	fmt.Println("\n💡 最佳实践:")
	fmt.Println("   - init() 只做简单初始化")
	fmt.Println("   - 复杂逻辑、IO操作放 main 里")
	fmt.Println("   - init() 不要启动 goroutine")
	fmt.Println("   - 不要依赖不同文件的 init() 执行顺序")
}
