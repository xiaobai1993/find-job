# panic/recover 底层原理与最佳实践

---

## 一、先搞懂：panic 到底是什么？

panic 是 Go 语言的运行时异常机制，用来表示程序遇到了无法处理的严重错误，应该终止执行。

很多人把 panic 和 Java 的 exception 搞混，其实完全不一样。panic 设计的初衷就是「让程序崩溃」，而不是「让你捕获然后继续跑」。

---

## 二、panic/recover 核心规则（7 条）

### 规则 1：panic 会终止程序，按调用栈逆序执行 defer

```go
func main() {
    defer fmt.Println("defer 1")
    defer fmt.Println("defer 2")

    panic("出事了！")

    defer fmt.Println("defer 3") // 永远不会执行
}
```

**输出：**
```
defer 2
defer 1
panic: 出事了！
```

**执行顺序：**
1. 遇到 panic
2. 逆序执行当前函数的所有 defer（LIFO 顺序）
3. 退出当前函数，到上层函数继续执行 defer
4. 一直往上，直到 goroutine 最顶层
5. 如果整个过程都没有 recover，整个程序崩溃

---

### 规则 2：recover 只能在 defer 里面调用才有效

```go
// ❌ 错误写法：不在 defer 里，recover 永远返回 nil
func bad() {
    if err := recover(); err != nil {
        fmt.Println("捕获到", err)
    }
    panic("出事了")
}

// ✅ 正确写法：在 defer 里调用
func good() {
    defer func() {
        if err := recover(); err != nil {
            fmt.Println("捕获到", err)
        }
    }()
    panic("出事了")
}
```

**这是最基本也是最重要的规则！** 不在 defer 里的 recover 永远返回 nil，什么都捕获不到。

---

### 规则 3：recover 捕获之后，程序不会崩溃，但是会跳过 panic 后面的代码

```go
func main() {
    defer func() {
        if err := recover(); err != nil {
            fmt.Println("捕获到:", err)
        }
    }()

    panic("出事了")

    fmt.Println("这行永远不会执行！") // ❌ 不会执行
}

fmt.Println("程序正常结束") // ✅ 会执行
```

recover 之后，当前函数会立即返回，不会执行 panic 后面的代码，但是上层函数可以正常继续执行。

---

### 规则 4：panic 可以是任意类型，不一定是 error

```go
panic("字符串错误")    // string
panic(123)             // int
panic(errors.New("error 类型")) // error
panic(struct{}{})      // struct
```

panic 的参数类型是 `interface{}`，什么都可以传。

所以 recover 返回的也是 `interface{}`，需要类型断言：
```go
defer func() {
    if err := recover(); err != nil {
        switch e := err.(type) {
        case string:
            fmt.Println("字符串错误:", e)
        case error:
            fmt.Println("error 类型:", e)
        default:
            fmt.Println("其他类型:", e)
        }
    }
}()
```

---

### 规则 5：defer 里 panic，可以被后面的 defer 捕获

```go
func main() {
    defer func() {
        if err := recover(); err != nil {
            fmt.Println("捕获到第二个 panic:", err)
        }
    }()

    defer func() {
        panic("第二个 panic") // ✅ 会被前面的 defer 捕获
    }()

    panic("第一个 panic")
}
```

**输出：** `捕获到第二个 panic: 第二个 panic`

**关键点：**
- 后面的 panic 会覆盖前面的 panic
- 只有最后一个 panic 能被捕获
- 第一个 panic 永远丢失了！

**这是一个非常危险的特性！** 如果你在 defer 里不小心 panic 了，原来的 panic 就丢了，你根本不知道最初出了什么问题。

---

### 规则 6：recover 只能捕获当前 goroutine 的 panic，跨 goroutine 没用！

```go
// ❌ 错误写法：跨 goroutine 捕获不到
func main() {
    defer func() {
        if err := recover(); err != nil {
            fmt.Println("捕获到", err) // 永远不会执行！
        }
    }()

    go func() {
        panic("子 goroutine 出事了") // 直接崩溃，main 捕获不到！
    }()

    time.Sleep(time.Second)
}
```

**这是 90% 的人都踩过的坑！** 每个 goroutine 都有自己的 defer 栈，recover 只能捕获自己 goroutine 的 panic。

**✅ 正确写法：每个 goroutine 自己处理自己的 panic**
```go
go func() {
    defer func() {
        if err := recover(); err != nil {
            log.Printf("goroutine panic: %v", err)
        }
    }()

    // 业务逻辑
}()
```

**支付业务特别重要：** 所有启动的 goroutine 都必须加 defer recover，不然一个 goroutine  panic 整个服务就挂了！

---

### 规则 7：Go 1.21+ 可以获取完整的 panic 栈

Go 1.21 新增了 `runtime.PanicNilError` 和 `runtime.AddCleanup`，可以更好地处理 panic。

Go 1.23 还新增了 `runtime.Stack` 可以获取完整的调用栈，调试的时候非常有用。

---

## 三、90% 的人都踩过的 5 个坑

### 坑 1：defer 里的 panic 覆盖了原来的 panic

前面讲过了，这是最危险的坑。

**真实案例：**
```go
func handleRequest() {
    defer func() {
        // 释放资源的时候不小心 panic 了
        resource.Close()
    }()

    // 业务逻辑 panic 了
    panic("数据库连接超时")
}
```

结果你只能看到 `resource.Close()` 的 panic，永远看不到原来的「数据库连接超时」，排查问题想死的心都有。

**✅ 最佳实践：defer 里绝对不要产生新的 panic**
```go
defer func() {
    defer func() {
        if err := recover(); err != nil {
            log.Printf("close resource panic: %v", err)
        }
    }()
    resource.Close()
}()
```

---

### 坑 2：recover 之后吞掉错误，假装什么都没发生

```go
// ❌ 非常危险的写法：吞掉所有 panic
func dangerous() {
    defer func() {
        recover() // 什么都不打日志，直接吞掉！
    }()

    // 业务逻辑
}
```

这是 Go 里最恶劣的反模式之一。panic 本来就是表示程序遇到了不可恢复的错误，你直接把它吞了，程序可能已经处于不一致的状态了，还在继续跑，后面会出更诡异的 bug。

**✅ 正确做法：recover 之后必须打日志，并且尽量优雅退出**
```go
defer func() {
    if err := recover(); err != nil {
        log.Printf("PANIC RECOVERED: %v\n%s", err, debug.Stack()) // ✅ 必须打完整栈！
    }
}()
```

---

### 坑 3：把 panic 当普通异常用，到处 panic 到处 recover

很多从 Java 转过来的人，把 Go 的 panic 当 Java 的 Exception 用：

```go
// ❌ 错误示范：把 panic 当异常用
func getUser(id int) (*User, error) {
    if id <= 0 {
        panic("invalid id") // 不要这么写！
    }
    // ...
}

// 调用的地方
func handler() {
    defer func() {
        if err := recover(); err != nil {
            // 返回错误
        }
    }()
    user := getUser(1)
}
```

**Go 的设计哲学是：**
- 预期内的错误用 error 返回
- 预期外的、不可恢复的错误才用 panic

panic 是非常重的操作，会清空整个调用栈，性能很差，而且很难调试。不要把它当普通异常用。

---

### 坑 4：recover 之后直接返回，忽略了 defer 里的资源释放

```go
// ❌ 错误写法：recover 之后直接返回，后面的 defer 还是会执行
func process() {
    defer func() {
        if err := recover(); err != nil {
            log.Println("panic", err)
            return // 这里的 return 只是退出这个 defer 函数，不是退出 process
        }
    }()

    defer releaseLock() // 这个还是会执行！

    panic("出事了")
}
```

注意：defer 里的 return 只是退出 defer 函数本身，不是退出外层函数。外层函数的其他 defer 还是会正常执行。

---

### 坑 5：nil pointer dereference 是 runtime panic，不是你自己 panic 的

```go
var p *int
*p = 10 // ❌ panic: runtime error: invalid memory address or nil pointer dereference
```

这种 runtime 的 panic 和你自己 `panic("xxx")` 是一样的，都可以被 recover 捕获。

常见的 runtime panic 有：
- nil pointer dereference
- index out of range
- slice/array/map index out of bounds
- divide by zero
- send on closed channel
- concurrent map read and map write

---

## 四、panic/recover 底层原理（面试加分项）

### 1. panic 的数据结构

```go
// runtime/runtime2.go
type _panic struct {
    argp      unsafe.Pointer // defer 函数的参数地址
    arg       interface{}    // panic 的参数
    link      *_panic        // 指向上一个 panic（链表）
    pc        uintptr        // 程序计数器
    sp        unsafe.Pointer // 栈指针
    goexit    bool           // 是否是 runtime.Goexit
    aborted   bool           // 是否被 aborted
    recovered bool           // 是否已经被 recover
}
```

**关键点：**
- panic 是链表结构，新的 panic 插在头部
- 所以后面的 panic 会覆盖前面的
- 每个 goroutine 有自己的 panic 链表，存在 `g._panic` 里

---

### 2. recover 到底做了什么？

```go
func recover() interface{} {
    gp := getg()

    // 不在 defer 里？直接返回 nil
    if gp._defer == nil || !gp._defer.started {
        return nil
    }

    // 没有 panic？返回 nil
    if gp._panic == nil {
        return nil
    }

    p := gp._panic
    if p.goexit {
        return nil
    }

    // 标记 panic 已经被 recovered
    p.recovered = true

    // 修改程序计数器，让 defer 执行完之后跳转到 recovery 代码
    // 而不是继续往上抛 panic

    return p.arg
}
```

recover 本质就是把 panic 的 `recovered` 标记设为 true，然后修改返回地址，让程序不要崩溃。

---

### 3. 为什么 recover 只能在 defer 里调用？

因为 panic 发生的时候，runtime 只会在 defer 函数执行完之后检查 `recovered` 标记。如果你不在 defer 里调用 recover，根本就没有 runtime 会去处理你的 recover。

这是 runtime 故意设计的限制。

---

## 五、最佳实践（支付业务必须遵守！）

### 实践 1：所有 goroutine 入口必须加 defer recover

```go
// ✅ 标准写法：每个 goroutine 都要有
go func() {
    defer func() {
        if err := recover(); err != nil {
            log.Printf("PANIC: %v\nSTACK: %s", err, debug.Stack())
            // 可以在这里上报监控告警
        }
    }()

    // 业务逻辑
}()
```

**记住：一个 goroutine panic 没被 recover，整个进程就没了！**

---

### 实践 2：recover 必须打完整的调用栈，不要只打错误信息

```go
// ❌ 不好：只打错误信息，不知道哪里出的问题
defer func() {
    if err := recover(); err != nil {
        log.Printf("panic: %v", err)
    }
}()

// ✅ 好：打完整栈
defer func() {
    if err := recover(); err != nil {
        log.Printf("PANIC RECOVERED: %v\n%s", err, debug.Stack())
    }
}()
```

没有栈的 panic 日志等于没有日志。

---

### 实践 3：不要在生产环境随便吞 panic

recover 应该只用在最顶层的入口（比如 HTTP handler、RPC handler），不要在业务逻辑深处随便 recover。

业务逻辑深处遇到严重错误就应该直接 panic，让最上层统一处理，而不是在中间层吞掉。

---

### 实践 4：defer 里的代码要绝对安全

defer 里的代码绝对不能产生新的 panic。如果 defer 里的代码可能 panic，必须在 defer 里面再加一层 recover。

```go
defer func() {
    defer func() {
        if err := recover(); err != nil {
            log.Printf("defer panic: %v", err)
        }
    }()
    // 可能 panic 的代码
    riskyClose()
}()
```

---

## 六、面试高频问答

| 问题 | 答案 |
|------|------|
| recover 只能在哪里调用才有效？ | 只能在 defer 函数里调用，其他地方调用永远返回 nil。 |
| panic 之后的代码还会执行吗？ | panic 后面的代码不会执行，但是 defer 会逆序执行。recover 之后上层函数可以继续执行。 |
| 一个 goroutine 的 panic 能被另一个 goroutine recover 吗？ | 不能，每个 goroutine 有自己的 defer 和 panic 栈，跨 goroutine 捕获不到。 |
| defer 里又 panic 了会怎么样？ | 新的 panic 会覆盖旧的 panic，只有最后一个 panic 能被捕获，原来的 panic 就丢了。 |
| panic 的参数可以是什么类型？ | 任意类型，interface{}，所以 recover 返回的也是 interface{}，需要类型断言。 |
| recover 之后程序还能正常运行吗？ | 可以，但是当前函数会立即返回，后面的代码不会执行，上层函数可以正常继续。 |
| runtime panic 能被 recover 捕获吗？ | 可以，nil pointer、index out of range、divide by zero 这些都可以被捕获。 |
| Go 1.21 之后 panic 有什么新特性？ | 新增了 runtime.PanicNilError，可以更精细地控制 nil panic 的行为。 |
| 为什么 Go 设计成 panic 崩溃而不是返回错误？ | 因为有些错误是不可恢复的，程序继续跑反而会出更大的问题，比如数据不一致。panic 就是让程序早点死，早点发现问题。 |
| panic 和 error 应该怎么选？ | 预期内的、用户可以处理的错误用 error；预期外的、不可恢复的严重错误用 panic。不要把 panic 当普通异常用。 |

---

## 七、一句话总结

> panic 是程序的自我毁灭机制，recover 是最后的救生衣；recover 只能在 defer 里用，每个 goroutine 必须自己处理自己的 panic；defer 里绝对不要产生新的 panic，recover 之后必须打完整栈。
