# Channel 实现原理

来源：《Go 专家编程》第一章 1.1 chan

---

## 一、hchan 数据结构

```go
type hchan struct {
    qcount   uint           // 当前队列剩余元素个数
    dataqsiz uint           // 环形队列长度（可存放元素总数）
    buf      unsafe.Pointer // 环形队列指针
    elemsize uint16         // 每个元素大小
    closed   uint32         // 关闭状态标志
    elemtype *_type         // 元素类型
    sendx    uint           // 写入下标 [0, dataqsiz)
    recvx    uint           // 读取下标 [0, dataqsiz)
    recvq    waitq          // 等待读的 goroutine 队列
    sendq    waitq          // 等待写的 goroutine 队列
    lock     mutex          // 互斥锁（不允许并发读写）
}
```

核心三部分：**环形队列缓冲区** + **等待队列** + **互斥锁**

---

## 二、环形队列

channel 内部是一个环形队列作为缓冲区，长度由 `make(chan T, size)` 指定：

- `dataqsiz`：队列总容量
- `qcount`：当前元素个数
- `buf`：指向队列内存
- `sendx`：下一次写入的位置下标
- `recvx`：下一次读取的位置下标

示例：`make(chan int, 6)`，队列里已有 2 个元素：
```
qcount = 2
dataqsiz = 6
sendx = 3  // 下次写到位置 3
recvx = 1  // 下次从位置 1 读
```

---

## 三、等待队列

goroutine 读写 channel 时可能阻塞，被挂到 channel 的等待队列：

- **读阻塞**：缓冲区空 / 无缓冲区 → 挂到 `recvq`
- **写阻塞**：缓冲区满 / 无缓冲区 → 挂到 `sendq`

唤醒规则：
- `recvq` 中阻塞的 goroutine，会被**写入数据的 goroutine** 唤醒
- `sendq` 中阻塞的 goroutine，会被**读取数据的 goroutine** 唤醒

> 特殊情况：`recvq` 和 `sendq` 一般至少一个为空，唯一例外是同一个 goroutine 用 select 同时读写同一个 channel。

---

## 四、写数据流程

向 channel 写数据的逻辑顺序：

1. **有等待读的 goroutine**（`recvq` 非空）
   - 直接从 `recvq` 取出一个 G
   - 把数据直接写到这个 G
   - 唤醒该 G，结束发送

2. **无等待读，但缓冲区有空位**
   - 把数据写入 `buf` 队尾
   - `qcount++`、`sendx++`，结束发送

3. **无等待读，且缓冲区满**
   - 把当前 G 加入 `sendq`
   - 进入睡眠，等待被读 goroutine 唤醒

> 无缓冲 channel（size=0）直接走 1 或 3，不会走 2。

---

## 五、读数据流程

从 channel 读数据的逻辑顺序：

1. **有等待写的 goroutine 且无缓冲区**（`sendq` 非空 + size=0）
   - 直接从 `sendq` 取出一个 G
   - 从该 G 中读出数据
   - 唤醒该 G，结束读取

2. **有等待写的 goroutine 且缓冲区满**（`sendq` 非空 + qcount=dataqsiz）
   - 从 `buf` 队首读出一个元素
   - 从 `sendq` 取出一个 G，把 G 中数据写入 `buf` 队尾
   - 唤醒该 G，结束读取

3. **缓冲区有数据**（`qcount > 0`）
   - 直接从 `buf` 中读出一个元素
   - 结束读取

4. **以上都不满足**
   - 把当前 G 加入 `recvq`
   - 进入睡眠，等待被写 goroutine 唤醒

---

## 六、关闭 channel

关闭 channel 时的行为：

1. 把 `recvq` 中所有读阻塞的 G 全部唤醒，G 读出的值为类型零值
2. 把 `sendq` 中所有写阻塞的 G 全部唤醒，这些 G 会直接 **panic**

### 触发 panic 的 4 种场景

1. 关闭值为 `nil` 的 channel
2. 关闭已经被关闭过的 channel
3. 向已经关闭的 channel 写数据
4. 读 channel 时，写端 goroutine 退出且 channel 未关闭 → range 永久阻塞（系统检测到可能 panic）

---

## 七、常见用法

### 7.1 单向 channel

本质是对 channel 的**使用限制**，类似 C 语言的 `const` 修饰参数，不是真的有"只能读"或"只能写"的 channel 类型：

```go
func readChan(ch <-chan int)   // 参数限定"只能读"
func writeChan(ch chan<- int)  // 参数限定"只能写"
```

调用方传的还是普通 channel，函数内只是被限制了用法。

### 7.2 select

- 可以同时监控多个 channel，任意一个就绪就执行对应 case
- **多个 case 的执行顺序是随机的**
- `default` 分支不会阻塞，读不到数据直接返回（编译时传入"不阻塞"参数）

### 7.3 range

- 持续从 channel 读数据，像遍历数组一样
- channel 没数据时会阻塞当前 goroutine
- **注意**：写端 goroutine 退出但没 close channel → range 永久阻塞

---

## 八、实战案例

### 8.1 控制并发数（Worker Pool）

固定 N 个 worker 消费任务，防止 goroutine 数量可控：

```go
func workerPool(tasks []func(), workerNum int) {
    taskCh := make(chan func(), len(tasks))
    defer close(taskCh)

    // 启动 worker
    for i := 0; i < workerNum; i++ {
        go func() {
            for task := range taskCh {
                task()
            }
        }()
    }

    // 投递任务
    for _, t := range tasks {
        taskCh <- t
    }
}
```

**适用场景：批量处理任务，控制 goroutine 不暴涨

---

### 8.2 超时控制（结合 time.After）

```go
func withTimeout(ch <-chan int, timeout time.Duration) (int, error) {
    select {
    case v := <-ch:
        return v, nil
    case <-time.After(timeout):
        return 0, errors.New("timeout")
    }
}
```

**注意**：`time.After` 在 select 退出后定时器不会立刻停止，直到超时触发。短超时场景没问题，高并发建议用 `context.WithTimeout` 替代。

---

### 8.3 优雅关闭

```go
func gracefulShutdown() {
    stop := make(chan struct{})

    go func() {
        for {
            select {
            case <-stop:
                fmt.Println("worker exit")
                return
            default:
                // do work
            }
        }
    }()

    close(stop)  // 通知退出
}
```

关闭空 struct 不占内存，专门做信号。

---

### 8.4 解耦生产者消费者

```go
func producer(out chan<- int) {
    for i := 0; i < 10; i++ {
        out <- i
    }
    close(out)  // 生产完必须 close，通知消费端 range 退出
}

func consumer(in <-chan int) {
    for v := range in {  // close 后自动退出循环
        fmt.Println(v)
    }
}
```

**关键**：生产者负责 close，消费者负责感知关闭后退出。

---

### 8.5 异步任务结果收集

```go
func fetchAll() []Result {
    ch := make(chan Result, 3)

    go func() { ch <- fetchA() }()
    go func() { ch <- fetchB() }()
    go func() { ch <- fetchC() }()

    var results []Result
    for i := 0; i < 3; i++ {
        results = append(results, <-ch)
    }
    return results
}
```

---

### 8.6 信号量模式

限制同时执行的并发数：

```go
sem := make(chan struct{}, 3)  // 最多 3 个并发
for _, task := range tasks {
    go func(t Task) {
        sem <- struct{}{}        // 获取信号量
        defer func() { <-sem }() // 释放
        t.Run()
    }(task)
}
```

---

### 8.7 or-done 模式

多个信号任意一个触发就退出：

```go
// 固定 3 个任务，任意一个完成就不等了
func waitAny(ch1, ch2, ch3 <-chan struct{}) <-chan struct{} {
    result := make(chan struct{})
    go func() {
        defer close(result)
        select {
        case <-ch1:
        case <-ch2:
        case <-ch3:
        }
    }()
    return result
}

// 使用：
done1, done2, done3 := make(chan struct{}), make(chan struct{}), make(chan struct{})
go func() { time.Sleep(time.Second); close(done1) }()
go func() { time.Sleep(2 * time.Second); close(done2) }()

<-waitAny(done1, done2, done3)  // 1 秒后就返回，不等另外两个
```

**适用场景**：多数据源竞速、多任务容错，只要有一个成功就继续。

---

### 8.8 防止 goroutine 泄漏

**错误写法**：goroutine 阻塞在 channel 上永远不退出：

```go
func leak() {
    ch := make(chan int)
    go func() {
        v := <-ch  // 永远等不到
        fmt.Println(v)
    }()
    return  // ch 没人写，goroutine 永久阻塞
}
```

**正确写法**：用带缓冲或 done channel 避免阻塞：

```go
func noLeak() {
    ch := make(chan int, 1)  // 缓冲 1，写不阻塞
    go func() {
        ch <- result
    }()
    select {
    case v := <-ch:
        return v
    case <-time.After(time.Second):
        return nil
    }
}
```

---

## 面试高频点

| 问题 | 答案要点 |
|------|----------|
| channel 底层结构？ | hchan：环形队列(buf/qcount/sendx/recvx) + 等待队列(recvq/sendq) + 互斥锁 |
| 有缓冲和无缓冲区别？ | 无缓冲直接 G 到 G 拷贝；有缓冲先走 buf，满了才阻塞 |
| 写 channel 流程？ | 先看有没有阻塞读的 G → 有就直接写给他；没有看 buf 有空位 → 写 buf；都没有就挂 sendq |
| 关闭 channel 会怎样？ | 读的 G 收到零值，写的 G 直接 panic |
| 哪些操作会 panic？ | 关闭 nil channel、重复关闭、向已关闭的写、range 时写端退出不关闭 |
| select 多个 case 顺序？ | 随机 |
| 谁负责 close channel？ | 生产者负责 close，消费者感知退出；不能消费者 close 会导致写 panic |
| 怎么防止 goroutine 泄漏？ | 用带缓冲 channel，或者 done 信号，timeout 避免永久阻塞 |
| time.After 有坑吗？ | select 退出后定时器还在，直到超时才释放，高并发场景用 context |
