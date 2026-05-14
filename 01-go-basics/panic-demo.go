package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// ============================================
// 演示 1：panic 执行顺序
// ============================================
func demo1PanicOrder() {
	fmt.Println("=== 演示 1：panic 执行顺序 ===")

	defer fmt.Println("defer 1")
	defer fmt.Println("defer 2")

	fmt.Println("正常代码执行")

	// 注释掉避免程序崩溃
	// panic("出事了！")

	// fmt.Println("这行永远不会执行")
	fmt.Println("💡 panic 之后，逆序执行 defer，然后退出函数")
}

// ============================================
// 演示 2：recover 只能在 defer 里才有效
// ============================================
func demo2RecoverOnlyInDefer() {
	fmt.Println("\n=== 演示 2：recover 只能在 defer 里才有效 ===")

	// ❌ 不在 defer 里，永远返回 nil
	if err := recover(); err != nil {
		fmt.Println("❌ 不在 defer 里也能捕获？不可能！", err)
	} else {
		fmt.Println("❌ 不在 defer 里的 recover 永远返回 nil")
	}

	// ✅ 在 defer 里才有效
	func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Println("✅ defer 里 recover 成功:", err)
			}
		}()

		panic("测试 panic")
	}()
}

// ============================================
// 演示 3：recover 之后，panic 后面的代码不会执行
// ============================================
func demo3RecoverSkip() {
	fmt.Println("\n=== 演示 3：recover 之后，panic 后面的代码不会执行 ===")

	func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Println("✅ 捕获到:", err)
			}
		}()

		panic("出事了")

		fmt.Println("❌ 这行永远不会执行！")
	}()

	fmt.Println("✅ 上层函数可以正常继续执行")
}

// ============================================
// 演示 4：defer 里的 panic 覆盖原来的 panic
// ============================================
func demo4DeferPanicOverride() {
	fmt.Println("\n=== 演示 4：defer 里的 panic 覆盖原来的 panic ===")

	defer func() {
		if err := recover(); err != nil {
			fmt.Println("✅ 捕获到的是第二个 panic:", err)
			fmt.Println("💡 原来的第一个 panic 永远丢失了！")
		}
	}()

	defer func() {
		panic("第二个 panic") // 覆盖了第一个 panic
	}()

	panic("第一个 panic")
}

// ============================================
// 演示 5：跨 goroutine 的 panic 捕获不到
// ============================================
func demo5CrossGoroutine() {
	fmt.Println("\n=== 演示 5：跨 goroutine 的 panic 捕获不到 ===")

	done := make(chan struct{})

	// 外层的 recover 捕获不到子 goroutine 的 panic
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("❌ 外层捕获到:", err) // 永远不会执行
		}
	}()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Println("✅ 子 goroutine 自己捕获到:", err)
				close(done)
			}
		}()

		panic("子 goroutine panic")
	}()

	<-done
	fmt.Println("💡 每个 goroutine 必须自己处理自己的 panic！")
}

// ============================================
// 演示 6：recover 之后必须打完整栈
// ============================================
func demo6StackLog() {
	fmt.Println("\n=== 演示 6：recover 之后必须打完整栈 ===")

	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("❌ 只打错误信息，不知道哪里出的问题: %v\n", err)
			fmt.Println("\n✅ 打完整调用栈，问题在哪一目了然:")
			fmt.Printf("PANIC: %v\n%s", err, debug.Stack())
		}
	}()

	func() {
		func() {
			panic("深层 panic")
		}()
	}()
}

// ============================================
// 演示 7：runtime panic 也能被捕获
// ============================================
func demo7RuntimePanic() {
	fmt.Println("\n=== 演示 7：runtime panic 也能被捕获 ===")

	// nil pointer
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("✅ 捕获到 nil pointer panic:", err)
		}
	}()

	var p *int
	*p = 10 // nil pointer dereference
}

// ============================================
// 演示 8：goroutine 入口标准写法
// ============================================
func demo8GoroutineTemplate() {
	fmt.Println("\n=== 演示 8：goroutine 入口标准写法 ===")

	done := make(chan struct{})

	// ✅ 标准写法：每个 goroutine 开头必须加 defer recover
	go func() {
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("✅ goroutine panic 被捕获: %v\n%s", err, debug.Stack())
			}
			close(done)
		}()

		// 业务逻辑
		fmt.Println("goroutine 正常执行")
		panic("业务逻辑 panic")
	}()

	<-done
	fmt.Println("💡 支付业务所有 goroutine 必须这么写！")
}

// ============================================
// 演示 9：defer 里的代码也要加 recover
// ============================================
func demo9DeferSafe() {
	fmt.Println("\n=== 演示 9：defer 里的代码也要加 recover ===")

	defer func() {
		if err := recover(); err != nil {
			fmt.Println("✅ 外层捕获到:", err)
		}
	}()

	defer func() {
		// ✅ defer 里面的代码也要加 recover，不然会覆盖原来的 panic
		defer func() {
			if err := recover(); err != nil {
				fmt.Println("✅ defer 内部捕获到自己的 panic:", err)
			}
		}()

		fmt.Println("defer 释放资源")
		panic("释放资源的时候也 panic 了")
	}()

	panic("业务逻辑 panic")
}

// ============================================
// 演示 10：panic 可以是任意类型
// ============================================
func demo10AnyType() {
	fmt.Println("\n=== 演示 10：panic 可以是任意类型 ===")

	testPanic := func(p interface{}) {
		defer func() {
			if err := recover(); err != nil {
				switch e := err.(type) {
				case string:
					fmt.Printf("✅ 字符串 panic: %s\n", e)
				case int:
					fmt.Printf("✅ int panic: %d\n", e)
				case error:
					fmt.Printf("✅ error panic: %v\n", e)
				default:
					fmt.Printf("✅ 其他类型 panic: %T %v\n", e, e)
				}
			}
		}()
		panic(p)
	}

	testPanic("字符串错误")
	testPanic(123)
	testPanic(fmt.Errorf("error 类型"))
	testPanic(struct{ X int }{X: 100})
}

// ============================================
// main
// ============================================
func main() {
	demo1PanicOrder()
	demo2RecoverOnlyInDefer()
	demo3RecoverSkip()
	demo4DeferPanicOverride()
	demo5CrossGoroutine()
	demo6StackLog()
	demo7RuntimePanic()
	demo8GoroutineTemplate()
	demo9DeferSafe()
	demo10AnyType()

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有 panic/recover 演示完成！")
	fmt.Println("核心总结：")
	fmt.Println("1. recover 只能在 defer 里调用才有效")
	fmt.Println("2. 每个 goroutine 必须自己处理自己的 panic")
	fmt.Println("3. defer 里的 panic 会覆盖原来的 panic")
	fmt.Println("4. recover 之后必须打完整的调用栈")
	fmt.Println("5. 不要把 panic 当普通异常用")
	fmt.Println("6. 支付业务所有 goroutine 入口必须加 defer recover")
}
