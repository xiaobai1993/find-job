# 数据竞争与 Race Detector 完全指南

---

## 一、先搞懂：什么是数据竞争（Data Race）？

**定义：两个 goroutine 同时访问同一个内存位置，至少有一个是写操作，并且没有同步。**

简单说就是：两个 goroutine 同时读写同一个变量，没加锁。

**后果：**
- 读到脏数据
- 写的数据丢了
- 程序直接崩溃
- 最恐怖的：看起来一切正常，只是偶尔莫名其妙出问题，查半年都查不出来

**面试一句话总结：数据竞争是并发程序最隐蔽的 bug，随机出现，极难调试，是 Go 程序员的头号敌人。**

---

## 二、最常见的数据竞争场景 Top 6

### 场景 1：最基础的，两个 goroutine 同时读写同一个变量

```go
// ❌ 经典数据竞争！
func main() {
    var count int

    go func() {
        for i := 0; i < 10000; i++ {
            count++  // 写
        }
    }()

    for i := 0; i < 10000; i++ {
        count++  // 同时写！
    }

    time.Sleep(time.Second)
    fmt.Println(count)  // 结果不是 20000，每次都不一样！
}
```

**为什么结果不对？**
`count++` 不是原子操作，实际上是三步：
1. 读 count 的值到寄存器
2. 寄存器 +1
3. 写回内存

两个 goroutine 交叉执行：
- G1 读到 count = 100
- G2 也读到 count = 100
- G1 +1 写回 101
- G2 +1 写回 101
- 加了两次，结果只加了 1！丢了！

---

### 场景 2：遍历 slice 的同时，另一个 goroutine append

```go
// ❌ 数据竞争！
func main() {
    var s []int

    go func() {
        for i := 0; i < 1000; i++ {
            s = append(s, i)  // 写 slice 结构体
        }
    }()

    for i := 0; i < 1000; i++ {
        _ = len(s)  // 读 slice 结构体
        for _, v := range s {  // 读！
            _ = v
        }
    }
}
```

slice 是个结构体（ptr + len + cap），读和写同时进行，可能读到一半写一半，结构体损坏！

---

### 场景 3：map 并发读写（最常见！直接 panic！）

```go
// ❌ Go 1.6+ 直接 panic：concurrent map read and map write
func main() {
    m := make(map[int]int)

    go func() {
        for i := 0; i < 1000; i++ {
            m[i] = i  // 写
        }
    }()

    for i := 0; i < 1000; i++ {
        _ = m[i]  // 读
    }
}
```

**注意：Go 特意做了检测，map 并发读写直接 panic，不让你带 bug 跑！**

这是 Go 最好的设计之一！不然数据竞争的 bug 藏起来你半年都查不出来。

---

### 场景 4：闭包捕获循环变量（经典中的经典！）

```go
// ❌ 数据竞争！
func main() {
    for i := 0; i < 5; i++ {
        go func() {
            fmt.Println(i)  // 读 i，主 goroutine 同时在写 i！
        }()
    }
    time.Sleep(time.Second)
}
```

**所有 goroutine 都打印 5！不是 0,1,2,3,4！**

这不是 bug，这是你写的代码有数据竞争！goroutine 启动的时候，循环可能已经跑完了，i 已经变成 5 了。

---

### 场景 5：返回结构体指针，外面还在改

```go
// ❌ 数据竞争！
func GetConfig() *Config {
    return &globalConfig  // 返回指针
}

// 另一个 goroutine 定时更新
func updateConfig() {
    for {
        globalConfig = newConfig  // 写整个结构体
        time.Sleep(time.Minute)
    }
}

// 外面拿到指针在读
func worker() {
    cfg := GetConfig()
    // 读 cfg 的字段的时候，可能正好在写！
    fmt.Println(cfg.Timeout)  // 竞争！
}
```

指针是万恶之源！返回指针就等于把控制权交出去了，谁知道别人会不会并发改。

---

### 场景 6：interface{} 的并发赋值

```go
// ❌ 隐蔽！interface{} 赋值不是原子的！
var any interface{}

go func() {
    any = 123  // int
}()

go func() {
    any = "hello"  // string
}()

// 可能读到一半 int，一半 string！直接 panic！
```

interface{} 是两个指针（type + data），赋值不是原子的，可能读到一半 type 变了 data 没变，或者反过来，直接崩溃。

---

## 三、怎么发现数据竞争？Race Detector！

Go 内置了世界上最好的数据竞争检测工具：Race Detector。

### 1. 怎么用？加个 -race 参数！

```bash
# 运行程序的时候开
go run -race main.go

# 测试的时候开
go test -race ./...

# 编译的时候开
go build -race -o myapp
```

**就这么简单！加个参数而已！**

---

### 2. Race Detector 输出怎么看？

如果有数据竞争，会打印类似这样的信息：

```
==================
WARNING: DATA RACE
Write at 0x00c00012c008 by goroutine 7:
  main.main.func1()
      /path/to/main.go:10 +0x38

Previous read at 0x00c00012c008 by main goroutine:
  main.main()
      /path/to/main.go:16 +0x88

Goroutine 7 (running) created at:
  main.main()
      /path/to/main.go:8 +0x54
==================
```

一眼就能看懂：
- 哪个 goroutine 在哪一行写
- 哪个 goroutine 在哪一行读
- 谁先谁后
- goroutine 是在哪创建的

---

### 3. Race Detector 的性能开销

| 指标 | 开销 |
|------|------|
| 内存 | 2 ~ 5 倍 |
| CPU | 2 ~ 20 倍 |

**生产环境能不能用？**
- 压测环境：必须开！所有压测都要 -race
- 测试环境：必须开！所有测试 go test -race
- 生产环境：流量小的服务可以开，流量大的别开，扛不住
- 灰度环境：建议开，抓线上的竞争 bug

---

### 4. Race Detector 不是 100% 能检测到

**重要：没检测到不代表没有数据竞争！**

Race Detector 是运行时采样检测的，只有竞争真的发生了才会检测到。

**怎么提高检测率？**
1. 压测的时候开 -race，并发越高越容易触发
2. 跑久一点，时间越长越容易触发
3. 单元测试覆盖所有并发路径
4. 用 go test -race 跑所有测试，CI 必须过

**最佳实践：CI 流水线必须跑 go test -race，没过不能合并！**

---

## 四、怎么修复数据竞争？四种方法

### 方法 1：加互斥锁 sync.Mutex

最常用，最直接：

```go
// ✅ 加锁保护
var mu sync.Mutex
var count int

go func() {
    for i := 0; i < 10000; i++ {
        mu.Lock()
        count++
        mu.Unlock()
    }
}()

mu.Lock()
count++
mu.Unlock()
```

**注意：锁必须保护整个临界区，不要只锁一半！**

---

### 方法 2：用原子操作 sync/atomic

对于简单的基本类型（int、uint、bool、pointer），用 atomic 比锁快：

```go
// ✅ atomic 原子操作，比锁快
var count int64

go func() {
    for i := 0; i < 10000; i++ {
        atomic.AddInt64(&count, 1)
    }
}()

atomic.AddInt64(&count, 1)
```

**什么场景用 atomic：**
- 只有一个变量要保护
- 性能极致要求
- 简单的加减、交换、CompareAndSwap

**什么场景别用 atomic：**
- 多个变量要同时保护（用锁）
- 复杂的临界区逻辑（用锁）

---

### 方法 3：用 channel，不要共享内存

Go 的哲学："Share memory by communicating, don't communicate by sharing memory."

```go
// ✅ 用 channel 传，没有共享内存，自然没有竞争
func main() {
    countCh := make(chan int, 1)
    countCh <- 0

    go func() {
        for i := 0; i < 10000; i++ {
            count := <-countCh
            count++
            countCh <- count
        }
    }()

    for i := 0; i < 10000; i++ {
        count := <-countCh
        count++
        countCh <- count
    }

    time.Sleep(time.Second)
    fmt.Println(<-countCh)  // 20000，正确！
}
```

---

### 方法 4：不要共享，每个 goroutine 自己一份

最好的同步就是不同步！根本不要共享数据：

```go
// ✅ 每个 goroutine 算自己的，最后加起来
func main() {
    var wg sync.WaitGroup
    results := make(chan int, 2)

    // goroutine 1 算自己的
    wg.Add(1)
    go func() {
        defer wg.Done()
        count := 0
        for i := 0; i < 10000; i++ {
            count++
        }
        results <- count
    }()

    // goroutine 2 算自己的
    wg.Add(1)
    go func() {
        defer wg.Done()
        count := 0
        for i := 0; i < 10000; i++ {
            count++
        }
        results <- count
    }()

    // 等两个都算完，加起来
    go func() {
        wg.Wait()
        close(results)
    }()

    total := 0
    for r := range results {
        total += r
    }
    fmt.Println(total)  // 20000，正确！零锁！零竞争！
}
```

**这是最高境界的并发：根本不要共享数据！每个 goroutine 自己算自己的，最后汇总。**

---

## 五、无锁编程的坑

很多人觉得锁慢，追求无锁，用 atomic 写复杂的无锁数据结构。

**我劝你别！**

| 问题 | 说明 |
|------|------|
| **太难写对** | 无锁编程极其难，连大神都经常写错 |
| **内存序问题** | CPU 会乱序执行，你还要考虑 acquire/release 内存序 |
| **很难测试** | 竞争条件只有在特定 CPU、特定负载、特定时候才会触发 |
| **性能收益没那么大** | 大部分场景锁的开销根本不是瓶颈，你写的无锁还不一定比标准库的锁快 |

**最佳实践：**
- 99% 的场景用 Mutex 就够了
- 1% 的极致性能场景，用标准库的 sync.Map、atomic.Value，不要自己写无锁
- 除非你是 Dmitry Vyukov（Go 调度器作者）级别的大神，否则别碰自定义无锁数据结构

---

## 六、常见坑与最佳实践

### 坑 1：以为读不需要加锁

```go
// ❌ 大错特错！读也需要加锁！
var mu sync.Mutex
var config *Config

func GetConfig() *Config {
    return config  // 读！也会竞争！
}

func UpdateConfig(c *Config) {
    mu.Lock()
    config = c  // 写
    mu.Unlock()
}
```

**写的同时读也是数据竞争！没有例外！**

不要以为"我只是读一下没关系"，只要有一个写在同时进行，就是竞争，就是 UB（未定义行为）。

---

### 坑 2：锁复制了

```go
// ❌ Mutex 不能复制，一复制锁就废了！
type MyStruct struct {
    mu sync.Mutex
    // ...
}

func bad(m MyStruct) {  // 值传递，Mutex 复制了！
    m.mu.Lock()  // 这把是新复制的锁，根本保护不了原来的数据！
    // ...
}
```

**Mutex 必须传指针！绝对不能值复制！**

---

### 坑 3：defer Unlock 的位置不对

```go
// ❌ 错误：先 defer 再 Lock，panic 的话 Unlock 一个没加的锁
func bad() {
    defer mu.Unlock()  // 先 defer
    mu.Lock()          // 再加锁
    // ...
}

// ✅ 正确：先 Lock 再 defer Unlock
func good() {
    mu.Lock()
    defer mu.Unlock()
    // ...
}
```

---

### 最佳实践总结

1. ✅ **所有测试必须跑 go test -race**，CI 没过不能合并
2. ✅ **压测必须开 -race**，大部分竞争 bug 压测能出来
3. ✅ **优先用 channel，其次用锁，最后考虑 atomic**
4. ✅ **Mutex 必须传指针，绝对不能复制**
5. ✅ **读也需要加锁！没有例外！**
6. ✅ **先 Lock 再 defer Unlock**
7. ❌ **不要自己写无锁数据结构**，99% 的人写不对
8. ❌ **不要返回内部指针**，返回拷贝，或者用 atomic.Value 保护
9. ❌ **不要相信"这里不会并发"的鬼话**，代码是会变的，今天不会不代表明天不会

---

## 七、面试高频问答

| 问题 | 答案 |
|------|------|
| 什么是数据竞争？ | 两个 goroutine 同时访问同一个内存地址，至少有一个是写，没有同步。 |
| 数据竞争有什么后果？ | 读到脏数据、写丢失、程序崩溃、未定义行为，最恐怖的是随机出现极难调试。 |
| 怎么检测数据竞争？ | 加 -race 参数跑，Go 内置 Race Detector，是世界上最好的竞争检测工具。 |
| -race 没检测到就没有数据竞争吗？ | 不是，Race Detector 是运行时采样检测的，只有竞争真的发生了才会检测到，压测久一点更容易触发。 |
| 怎么修复数据竞争？ | 四种方法：1. 加 sync.Mutex；2. 用 sync/atomic 原子操作；3. 用 channel 传数据不要共享；4. 根本不共享，每个 goroutine 自己一份最后汇总。 |
| 读也需要加锁吗？ | 必须！只要同时有一个写在进行，读也是数据竞争，没有例外。 |
| map 并发读写会怎么样？ | Go 1.6+ 直接 panic：concurrent map read and map write，Go 特意做的检测，不让你带 bug 跑。 |
| Mutex 可以复制吗？ | 绝对不能！Mutex 复制之后就变成两把不同的锁了，根本起不到保护作用。必须传指针。 |
| 无锁编程怎么样？ | 极其难写对，99% 的场景不需要，也不建议自己写。标准库的 sync.Map、atomic.Value 可以用，自定义的别写。 |
| 闭包捕获循环变量为什么是数据竞争？ | 所有 goroutine 捕获的是同一个循环变量 i，主 goroutine 在写 i，子 goroutine 在读 i，同时进行，就是典型的竞争。 |
| Race Detector 的性能开销？ | 内存 2~5 倍，CPU 2~20 倍。测试、压测必须开，生产环境看情况。 |
| 为什么写了锁 Race Detector 还报警？ | 锁加错了！可能是锁复制了、锁加的位置不对、临界区没完全覆盖、有地方漏加锁了。 |

---

## 八、一句话总结

> 数据竞争是 Go 并发最隐蔽的 bug，随机出现极难调试；记住所有测试必须跑 go test -race，优先用 channel 其次用锁，读也需要加锁，Mutex 不能复制，不要自己写无锁，99% 的数据竞争 bug 都能避免。
