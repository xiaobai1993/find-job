# Go 并发模式完全指南：Fan-in/Fan-out/Pipeline/Worker Pool

---

## 一、先搞懂：为什么需要并发模式？

Go 提供了 goroutine 和 channel 这两个强大的原语，但怎么组合它们写出优雅、高效、没有坑的并发代码？这就是并发模式要解决的问题。

**常见的并发场景：**
1. 批量处理 1000 个任务，要快
2. 多个数据源并行查询，汇总结果
3. 流水线处理，一步接一步
4. 控制并发数，不把下游打挂
5. 优雅处理错误和取消

---

## 二、模式 1：Worker Pool（工作池）

最常用的模式，没有之一！控制并发数，防止 goroutine 爆炸。

### 1. 什么时候用？

- 有大量任务要处理
- 需要限制最大并发数
- 防止把数据库/下游服务打挂

### 2. 实现代码

```go
// Worker Pool 标准实现
func workerPool(tasks []Task, workerCount int) []Result {
    // 任务 channel，buffer = 任务数，写的人不会卡住
    taskCh := make(chan Task, len(tasks))
    // 结果 channel
    resultCh := make(chan Result, len(tasks))

    // 1. 先把所有任务塞进去
    for _, task := range tasks {
        taskCh <- task
    }
    close(taskCh)  // 关了，worker 读完就会退出

    // 2. 启动 workerCount 个 goroutine
    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for task := range taskCh {  // 一直读，直到 channel close
                result := process(task)
                resultCh <- result
            }
        }()
    }

    // 3. 所有 worker 跑完了，关 resultCh
    go func() {
        wg.Wait()
        close(resultCh)
    }()

    // 4. 收集所有结果
    var results []Result
    for result := range resultCh {
        results = append(results, result)
    }

    return results
}
```

### 3. 关键设计要点

| 要点 | 为什么要这么做？ |
|------|-----------------|
| **taskCh 有 buffer** | 主线程塞任务不会卡住，塞完可以直接 close |
| **close(taskCh)** | worker `range` 完会自动退出，不会泄漏 |
| **wg.Wait() 后 close(resultCh)** | 收集结果的 `range` 会自动结束，不会死等 |
| **workerCount 可控** | 不会开几万个 goroutine 把系统打挂 |

**面试一句话总结：** Worker Pool 就是"固定数量的 worker，从同一个 channel 拿任务，处理完写结果，所有 worker 跑完关闭结果 channel"。

---

## 三、模式 2：Fan-out/Fan-in（扇出/扇入）

多个 goroutine 并行处理，最后汇总结果。

### 1. 什么时候用？

- 一个任务可以拆成多个独立的子任务
- 多个子任务并行执行，最后汇总
- 比如：查 5 个不同的数据源，最后合并成一个结果

### 2. 实现代码

```go
// Fan-out：把一个输入拆成 N 个 goroutine 并行处理
func fanOut(input <-chan int, workerCount int) []<-chan Result {
    outs := make([]<-chan Result, workerCount)
    for i := 0; i < workerCount; i++ {
        out := make(chan Result)
        outs[i] = out
        go func() {
            for x := range input {
                out <- process(x)
            }
            close(out)
        }()
    }
    return outs
}

// Fan-in：把 N 个 channel 的结果合并到一个 channel
func fanIn(channels ...<-chan Result) <-chan Result {
    var wg sync.WaitGroup
    out := make(chan Result)

    // 每个 channel 起一个 goroutine 收结果
    for _, ch := range channels {
        wg.Add(1)
        go func(c <-chan Result) {
            defer wg.Done()
            for r := range c {
                out <- r
            }
        }(ch)
    }

    // 所有 channel 收完了，关 out
    go func() {
        wg.Wait()
        close(out)
    }()

    return out
}

// 使用
func main() {
    input := make(chan int, 100)
    // 塞 input...

    // 扇出 5 个 worker 并行处理
    outs := fanOut(input, 5)
    // 扇入，合并所有结果
    result := fanIn(outs...)

    // 收所有结果
    for r := range result {
        fmt.Println(r)
    }
}
```

### 3. Fan-out/Fan-in vs Worker Pool 区别

| 模式 | 特点 | 适用场景 |
|------|------|---------|
| **Worker Pool** | 多个 worker 从同一个 channel 抢任务，任务均匀分配 | 大量同类任务，比如批量处理数据 |
| **Fan-out/Fan-in** | 每个 worker 有自己的输入 channel，最后合并 | 任务可以拆成不同类型的子任务，比如多数据源查询 |

---

## 四、模式 3：Pipeline（流水线）

把一个大任务拆成多个步骤，每个步骤一个 goroutine，像流水线一样传下去。

### 1. 什么时候用？

- 处理流程可以分成多个独立的阶段
- 每个阶段可以并行处理
- 比如：读文件 → 解析 → 转换 → 写数据库

### 2. 实现代码

```go
// 阶段 1：生成数据
func gen(nums ...int) <-chan int {
    out := make(chan int, len(nums))
    go func() {
        for _, n := range nums {
            out <- n
        }
        close(out)
    }()
    return out
}

// 阶段 2：平方
func sq(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        for n := range in {
            out <- n * n
        }
        close(out)
    }()
    return out
}

// 阶段 3：加 1
func addOne(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        for n := range in {
            out <- n + 1
        }
        close(out)
    }()
    return out
}

// 使用：串起来
func main() {
    // gen → sq → addOne → 打印
    for v := range addOne(sq(gen(1, 2, 3, 4, 5))) {
        fmt.Println(v)  // 2, 5, 10, 17, 26
    }
}
```

### 3. 流水线 + 并发：每个阶段可以有多个 worker

```go
// sq 阶段并行处理，比如开 4 个 goroutine
func sqConcurrent(in <-chan int) <-chan int {
    // Fan-out 4 个 goroutine
    outs := fanOut(in, 4)
    // Fan-in 合并结果
    return fanIn(outs...)
}
```

**优点：** 每个阶段可以独立调整并发数，瓶颈阶段多开几个 worker。

---

## 五、模式 4：errgroup - 带错误取消的并发

`golang.org/x/sync/errgroup` 是官方提供的，比 `sync.WaitGroup` 更好用！

### 1. 为什么不用 WaitGroup？

WaitGroup 的问题：
- 一个 goroutine 出错了，其他的还在傻跑
- 不能取消
- 错误要自己用 channel 传，麻烦

errgroup 完美解决：
- 一个出错，所有都取消
- 第一个错误会返回
- context 自动传播取消信号

### 2. 使用代码

```go
import "golang.org/x/sync/errgroup"

func parallelGet(urls []string) ([]string, error) {
    g, ctx := errgroup.WithContext(context.Background())

    results := make([]string, len(urls))
    for i, url := range urls {
        i, url := i, url  // 捕获循环变量
        g.Go(func() error {
            // 任何一个出错，ctx 就会 cancel，其他请求都会收到取消信号
            req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
            if err != nil {
                return err
            }
            resp, err := http.DefaultClient.Do(req)
            if err != nil {
                return err
            }
            defer resp.Body.Close()

            data, err := io.ReadAll(resp.Body)
            if err != nil {
                return err
            }
            results[i] = string(data)
            return nil
        })
    }

    // 等所有 goroutine 跑完
    if err := g.Wait(); err != nil {
        return nil, err  // 返回第一个错误
    }

    return results, nil
}
```

**超级好用！90% 的并发场景用 errgroup 就够了！**

---

## 六、模式 5：or-done channel

不知道 channel 什么时候会 close，外部取消了就要退出。

### 1. 什么时候用？

- 你要从一个 channel 读数据，但不知道它什么时候 close
- 同时要监听外部的取消信号（context.Done）
- 每个地方都写一遍 `select { case <-ctx.Done(): return; case v := <-ch: }` 太烦了

### 2. 实现代码

```go
// orDone 封装取消逻辑
func orDone(ctx context.Context, in <-chan T) <-chan T {
    out := make(chan T)
    go func() {
        defer close(out)
        for {
            select {
            case <-ctx.Done():
                return
            case v, ok := <-in:
                if !ok {
                    return
                }
                select {
                case out <- v:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()
    return out
}

// 使用，清爽！
func main() {
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    // 不管 in 什么时候 close，或者 ctx 取消了，都能正常退出
    for v := range orDone(ctx, in) {
        fmt.Println(v)
    }
}
```

《Concurrency in Go》里的经典模式，非常实用！

---

## 七、模式 6：Timeout / Cancel（超时控制）

所有并发操作都必须有超时！没有超时的并发就是耍流氓！

### 1. context.WithTimeout

```go
func doWithTimeout() error {
    // 1 秒超时
    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()

    resultCh := make(chan Result, 1)
    go func() {
        resultCh <- slowOperation()  // 可能跑很久
    }()

    select {
    case result := <-resultCh:
        fmt.Println("成功", result)
        return nil
    case <-ctx.Done():
        return fmt.Errorf("超时了: %w", ctx.Err())
    }
}
```

**重点：defer cancel() 必须写！不然 context 泄漏！**

---

## 八、常见坑与最佳实践

### 坑 1：循环变量捕获错误（90% 的人都踩过！）

```go
// ❌ 经典错误：所有 goroutine 都拿到最后一个 i
for i := 0; i < 5; i++ {
    go func() {
        fmt.Println(i)  // 全部打印 5！
    }()
}

// ✅ 正确：循环内捕获
for i := 0; i < 5; i++ {
    i := i  // 循环内声明，每次迭代一个新变量
    go func() {
        fmt.Println(i)  // 0,1,2,3,4 正确
    }()
}
```

**Go 最经典的坑！没有之一！写并发第一就要注意这个！**

---

### 坑 2：goroutine 泄漏，channel 没人 close

```go
// ❌ 泄漏：resultCh 没人读，goroutine 永远卡写
func bad() {
    resultCh := make(chan int)  // 无 buffer！
    go func() {
        resultCh <- 1  // 永远没人读，G 泄漏！
    }()
    // 超时了，直接返回，resultCh 没人读
    return
}

// ✅ 正确：给 channel 加 buffer，写的人不会卡住
resultCh := make(chan int, 1)  // buffer = 1
```

---

### 坑 3：WaitGroup Add 的位置不对

```go
// ❌ 错误：Add 在 goroutine 里面，可能 Wait 先执行完
for i := 0; i < 5; i++ {
    go func() {
        wg.Add(1)  // 太晚了！wg.Wait() 可能已经跑了
        defer wg.Done()
    }()
}
wg.Wait()  // 可能直接就返回了，不等 goroutine

// ✅ 正确：Add 在 goroutine 外面
for i := 0; i < 5; i++ {
    wg.Add(1)  // 先加
    go func() {
        defer wg.Done()
    }()
}
wg.Wait()
```

---

### 最佳实践总结

1. ✅ **90% 场景用 errgroup**，比 WaitGroup 好用一万倍
2. ✅ **所有并发操作必须有超时**，没有超时就是埋雷
3. ✅ **channel 尽量用 buffer**，避免 goroutine 卡住
4. ✅ **循环变量一定要在循环内捕获**
5. ✅ **WaitGroup.Add 一定要在 goroutine 外面调用**
6. ✅ **控制并发数，不要随便开几万个 goroutine**
7. ❌ **不要用共享内存+锁，尽量用 channel 通信**
8. ❌ **不要忽略错误，errgroup 会把错误传给你**

---

## 九、面试高频问答

| 问题 | 答案 |
|------|------|
| Worker Pool 怎么实现？ | 固定数量 worker 从同一个 task channel 读任务，处理完写 result channel，所有 worker 跑完 close result，wg.Wait() 同步。 |
| Fan-out/Fan-in 和 Worker Pool 区别？ | Worker Pool 是多个 worker 抢同一个 channel 的任务；Fan-out 是每个 worker 自己的 channel，最后 Fan-in 合并。 |
| errgroup 比 WaitGroup 好在哪？ | 支持错误传播，一个出错全部取消；返回第一个错误；context 取消自动传播。 |
| 循环变量捕获的经典坑是什么？ | for 循环里的 i 是同一个变量，goroutine 闭包捕获的是同一个引用，执行的时候可能已经变成最后一个值了。要在循环内 `i := i` 再捕获。 |
| 为什么 channel 建议加 buffer？ | 无 buffer channel 读写都要等对方，只要有一方没 ready，另一方就永远卡住，容易 goroutine 泄漏。加个 buffer = 1 就安全很多。 |
| WaitGroup.Add 应该放哪？ | 必须在 goroutine 外面调用，放里面的话可能 Wait 先执行完，Add 还没执行，直接就返回了。 |
| or-done channel 是干什么的？ | 封装取消逻辑，你给我一个 channel 和 context，我给你一个新 channel，context 取消或者原 channel close 了都会自动退出，不用每个地方都写重复的 select。 |
| 并发场景怎么避免 goroutine 泄漏？ | 1. 所有 channel 操作考虑会不会永久阻塞；2. 所有并发加超时；3. 用完的 channel 要 close；4. 用 context 传播取消信号。 |
| Pipeline 模式有什么好处？ | 每个阶段独立，可以独立调整并发数；解耦，每个阶段只干一件事；可以复用组件。 |
| 什么时候用锁，什么时候用 channel？ | Go 官方建议："Share memory by communicating, don't communicate by sharing memory." 大部分场景优先用 channel，性能极致要求的场景用锁/atomic。 |

---

## 十、一句话总结

> Go 并发模式就这几个：Worker Pool 控制并发数，Fan-out/Fan-in 并行汇总，Pipeline 流式处理，errgroup 带错误取消，or-done 封装取消逻辑；记住循环变量要捕获、channel 加 buffer、所有操作加超时，99% 的并发场景都能搞定。
