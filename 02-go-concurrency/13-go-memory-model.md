# Go 内存模型完全解析：Happens-Before 与可见性

---

## 一、先搞懂：什么是内存模型？为什么需要它？

### 1. 问题起源：CPU 和编译器会乱序执行

你以为你的代码是按顺序执行的：
```go
a = 1
b = 2
print(a + b)
```

实际上 CPU 和编译器可能会重新排序：
```go
b = 2  // 先执行 b
a = 1  // 再执行 a
print(a + b)
```

**为什么要乱序？为了快！**
- CPU 有多个执行单元，可以并行执行
- 编译器重排指令优化流水线
- 内存还有缓存，写了不一定立即刷回主存

**单线程没问题**，单线程下 CPU 和编译器保证"as-if-serial"语义，你感知不到乱序。

**多线程就出大事了！** 两个 goroutine 看到的执行顺序可能完全不一样。

---

### 2. 内存模型是干什么的？

**内存模型就是一份契约：它告诉你，在什么条件下，一个 goroutine 对内存的写操作，能被另一个 goroutine 看到。**

没有内存模型，你写的并发程序的行为就是完全未定义的，今天能跑明天可能就崩了。

**面试一句话总结：内存模型定义了多线程环境下内存操作的可见性和顺序保证，没有同步的代码行为是完全不可预测的。**

---

## 二、核心概念：Happens-Before（先行发生）

Happens-Before 是理解内存模型的唯一核心概念，非常简单：

**如果 A happens before B，那么 A 对内存的写操作，对 B 是 100% 可见的。**

反之：如果 A 和 B 没有 happens-before 关系，那么 A 的写对 B 不一定可见，B 可能看到旧值，也可能看到新值，也可能看到一半，完全不确定。

---

## 三、Go 语言保证的 Happens-Before 规则

Go 官方定义了 6 种一定会产生 happens-before 关系的场景。

**这 6 条是 Go 并发的根基，必须背下来！**

---

### 规则 1：goroutine 启动

```go
var a int

func main() {
    a = 1          // A
    go func() {    // go 语句
        print(a)   // B
    }()
}
```

**go 语句 happens before 新 goroutine 内的任何操作。**

所以 A happens before B，B 一定能看到 a = 1。

---

### 规则 2：goroutine 退出

```go
var a int

func f() {
    a = 1          // A
}

func main() {
    go f()
    time.Sleep(time.Second)  // 等 goroutine 退出
    print(a)                 // B
}
```

**goroutine 的退出 happens before 任何检测到它退出的操作。**

所以 A happens before B，B 一定能看到 a = 1。

---

### 规则 3：Channel 发送

```go
var ch = make(chan int, 10)
var a int

func f() {
    a = 1          // A
    ch <- 0        // 发送
}

func main() {
    go f()
    <-ch           // 接收，B
    print(a)
}
```

**对 channel 的发送操作 happens before 对应的接收操作完成。**

所以 A happens before B，B 一定能看到 a = 1。

**不管 channel 是有 buffer 还是无 buffer，这条规则都成立！**

---

### 规则 4：Channel 关闭

```go
var ch = make(chan int)
var a int

func f() {
    a = 1          // A
    close(ch)      // 关闭
}

func main() {
    go f()
    <-ch           // 读关闭的 channel 得到零值，B
    print(a)
}
```

**对 channel 的关闭操作 happens before 接收端收到零值。**

所以 A happens before B，B 一定能看到 a = 1。

---

### 规则 5：Mutex/RWMutex 锁

```go
var mu sync.Mutex
var a int

func f() {
    a = 1          // A
    mu.Unlock()    // 解锁
}

func main() {
    mu.Lock()
    go f()
    mu.Lock()      // B，第二次加锁，等 unlock 完才能拿到
    print(a)
}
```

**对 Mutex 的 Unlock 操作 happens before 下一次 Lock 操作返回。**

所以 A happens before B，B 一定能看到 a = 1。

**RWMutex 也是一样的：Unlock happens before 任何后续的 Lock/RLock。**

---

### 规则 6：Once

```go
var once sync.Once
var a int

func f() {
    a = 1          // A
}

func main() {
    go func() { once.Do(f) }()
    once.Do(f)     // B，第二次 Do 会等第一次执行完
    print(a)
}
```

**once.Do(f) 中 f 的返回 happens before 任何 once.Do(f) 调用返回。**

所以 A happens before B，B 一定能看到 a = 1。

**这就是 sync.Once 线程安全的根本原因！**

---

## 四、没有 Happens-Before 会怎么样？恐怖的例子！

### 例子 1：没有同步的变量读写

```go
var a, b int

func f() {
    a = 1          // 写 a
    b = 2          // 写 b
}

func g() {
    print(b)       // 读 b
    print(a)       // 读 a
}

func main() {
    go f()
    g()
}
```

**你以为可能的输出：0 0 / 1 0 / 1 2 / 2 1**

**实际上还可能输出：2 0！**

为什么？
- CPU 可能重排 f 里的两个写：先写 b = 2，再写 a = 1
- g 正好在中间读，读到 b = 2，a 还是 0
- 完全合法，CPU 和编译器都可以这么做！

**没有同步，一切皆有可能！**

---

### 例子 2：经典的双检查锁（Double-Checked Locking）是错的！

很多人想优化锁的开销，写双检查锁：

```go
// ❌ 错误！Go 内存模型不保证！
var mu sync.Mutex
var config *Config

func GetConfig() *Config {
    if config == nil {          // 第一次检查，无锁！
        mu.Lock()
        defer mu.Unlock()
        if config == nil {      // 第二次检查
            config = &Config{}  // 赋值
        }
    }
    return config
}
```

**为什么错？**

赋值 `config = &Config{}` 可能被重排！实际执行顺序可能是：
1. 分配内存
2. 把指针赋值给 config
3. 初始化 Config 的字段

另一个 goroutine 在第 2 步之后、第 3 步之前拿到 config，指针不是 nil，但里面的字段全是零值！崩溃！

**Java 里用 volatile 可以修，Go 里根本没有 volatile！**

**正确做法：不要搞什么双检查锁，老老实实每次加锁，或者用 sync.Once！**

```go
// ✅ 正确，用 Once
var once sync.Once
var config *Config

func GetConfig() *Config {
    once.Do(func() {
        config = &Config{}
    })
    return config
}
```

Once 是内存模型保证的，100% 安全。

---

### 例子 3：用 flag 控制 goroutine 退出是错的！

```go
// ❌ 错误！没有同步！
var stop bool

func worker() {
    for !stop {  // 读 stop
        // do work
    }
}

func main() {
    go worker()
    time.Sleep(time.Second)
    stop = true  // 写 stop
    // worker 可能永远看不到 stop = true，永远循环！
}
```

**worker 可能永远循环！**

为什么？
- CPU 可能把 stop 缓存到寄存器里，永远不读主存
- 或者编译器把循环优化成 `for { }`，直接去掉 stop 检查

**正确做法：用 channel 或者 atomic！**

```go
// ✅ 正确：用 channel
var stop = make(chan struct{})

func worker() {
    for {
        select {
        case <-stop:
            return
        default:
            // do work
        }
    }
}

func main() {
    go worker()
    time.Sleep(time.Second)
    close(stop)  // 内存模型保证 worker 一定能看到！
}
```

---

## 五、常见的误解和坑

### 坑 1：以为 32 位写是原子的

```go
var x int64

// goroutine 1
x = 0x1234567887654321

// goroutine 2
print(x)  // 可能读到 0x1234567800000000！一半新一半旧！
```

**32 位系统上，64 位变量的写不是原子的！可能写了一半被打断。**

甚至 64 位系统上，不对齐的 64 位写也不是原子的。

**多线程下，任何超过机器字长的变量读写都不是原子的！**

---

### 坑 2：以为有依赖就不会重排

```go
a = 1
b = a + 1  // 有依赖，不会重排，对的

c = 1
d = 2      // 无依赖，可能重排，可能先写 d 再写 c
```

只有真的有数据依赖的不会重排，独立的操作 CPU 和编译器可以随便重排。

---

### 坑 3：以为 Go 有 volatile

Go 没有 volatile 关键字！不要用 C++/Java 的经验套 Go。

Go 的内存模型里，只有上面列的 6 种场景有 happens-before 保证，其他的都没有。

**想保证可见性，用 channel、Mutex、atomic，不要瞎搞！**

---

## 六、最佳实践总结

1. ✅ **不要瞎搞无锁编程**，99% 的人写不对
2. ✅ **用 channel 同步**，这是 Go 推荐的方式
3. ✅ **用 Mutex**，简单直接不容易错
4. ✅ **用 sync.Once 做单例**，不要写双检查锁
5. ✅ **用 channel 停止 goroutine**，不要用 flag 变量
6. ✅ **多读官方的 Go Memory Model 文档**，不长，几十行而已
7. ❌ **不要写双检查锁**，Go 不保证
8. ❌ **不要用 flag 变量控制 goroutine**，可能永远看不到更新
9. ❌ **不要以为"这里不会有问题"**，CPU 和编译器比你想的疯狂多了

---

## 七、面试高频问答

| 问题 | 答案 |
|------|------|
| 什么是内存模型？ | 内存模型定义了多线程环境下内存操作的可见性和顺序保证，告诉你一个 goroutine 的写什么时候能被另一个 goroutine 看到。 |
| 什么是 Happens-Before？ | 如果 A happens before B，那么 A 对内存的所有写操作对 B 是 100% 可见的。没有 happens-before 关系的话，可见性完全不确定。 |
| Go 有哪些保证 happens-before 的场景？ | 6 种：1. goroutine 启动；2. goroutine 退出；3. channel 发送；4. channel 关闭；5. Mutex Unlock；6. Once.Do。 |
| 双检查锁（Double-Checked Locking）在 Go 里是对的吗？ | 绝对错误！Go 没有 volatile，赋值可能被重排，可能拿到非 nil 但未初始化的指针。用 sync.Once。 |
| 用 bool 变量控制 goroutine 退出是对的吗？ | 错误！没有同步的话，CPU 可能缓存变量，永远看不到更新，或者编译器直接优化掉。用 channel 或者 atomic。 |
| Go 有 volatile 关键字吗？ | 没有。Go 的内存模型不依赖 volatile，要用 channel、Mutex、atomic 来保证可见性。 |
| CPU 和编译器会重排指令吗？ | 会！单线程下保证 as-if-serial 你感知不到，多线程下会出大问题。只有 happens-before 能阻止重排。 |
| int64 的写是原子的吗？ | 32 位系统上不是，可能写一半被打断。64 位系统上对齐的是，不对齐的也不是。 |
| 为什么 sync.Once 是线程安全的？ | 内存模型保证：once.Do(f) 中 f 的返回 happens before 任何 once.Do(f) 调用返回，所以 f 里的初始化对所有调用者都是可见的。 |
| 无 buffer channel 和有 buffer channel，happens-before 保证一样吗？ | 一样！发送 happens before 接收完成，不管有没有 buffer。 |
| 我加了 Sleep 是不是就能看到更新了？ | 不能！Sleep 不是同步原语，没有任何 happens-before 保证，只是碰巧可能看到而已，换个 CPU/编译器可能就不行了。 |
| 为什么 Go 内存模型这么简单？ | Go 的设计哲学是：不要搞复杂的弱内存模型，尽量用 channel 同步，简单不容易错。 |

---

## 八、一句话总结

> Go 内存模型的核心就是 6 条 happens-before 规则，只有这 6 种场景能保证可见性；不要写双检查锁，不要用 flag 控制 goroutine，不要瞎搞无锁编程，老老实实⽤ channel 和 Mutex，99% 的并发 bug 都能避免。
