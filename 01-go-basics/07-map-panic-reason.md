# Go Map 所有 Panic 场景与设计哲学

Go 的 map 是整个语言里 panic 场景最多的类型之一，而且每一个 panic 都是**故意设计**的，不是 bug，背后有明确的工程考量。

---

## 一、所有会触发 Panic 的 7 个场景

### 场景 1：写 nil map
```go
var m map[int]int // nil
m[1] = 10        // ❌ panic: assignment to entry in nil map
```

**为什么这么设计？**

如果不 panic，那你写 `m[1] = 10` 这行代码看起来像是正常执行了，但其实什么都没存进去，后面读的时候读不到，这种「静默失败」比直接 panic 恐怖 100 倍。

> 宁可早失败，不要晚失败；宁可崩溃，不要静默错误。

这是 Go 最核心的设计哲学之一。

---

### 场景 2：并发读写（最常见）
```go
m := make(map[int]int)

go func() {
    for { m[1] = 1 } // 写
}()

go func() {
    for { _ = m[1] } // 读
}()

// ❌ 大概率 panic: concurrent map read and map write
```

**90% 的 Go 程序员都踩过这个坑。**

**为什么这么设计？为什么不像 Java 那样搞个弱一致性的实现？**

三个原因，优先级从高到低：

#### 1. **让 bug 尽早暴露
如果 Go 像 Java HashMap 那样，并发读写只是概率性出问题，大部分时候看起来正常，偶尔数据错乱，那这个 bug 会藏得非常深，可能到了生产环境才偶尔复现，根本查都查不到。

直接 panic 就是让你在开发阶段就发现「哦，我这里没加锁」，而不是带着 bug 上线。

#### 2. **性能优先**
Go 的 map 是为单 goroutine 场景优化的，如果要支持并发安全，需要加锁，每个读写操作都要加锁，那所有使用 map 的地方性能都要变慢。

**95% 的场景下 map 都是单 goroutine 使用的，为了 5% 的场景让 95% 的场景都变慢，不值当。

> Go 的选择是：让 95% 的场景跑满速，5% 的场景需要并发的自己加锁，或者用 `sync.Map`。

#### 3. **增量扩容的实现限制**
map 扩容是增量搬迁的，中间状态非常复杂，如果要支持并发，整个实现复杂度会指数级上升，bug 会非常多，维护成本极高。

**注意：**
- 并发读是可以的，不会 panic
- 只要有一个 goroutine 在写，其他任何读写都会 panic
- 不是竞态检测的问题，是 runtime 故意检查写标志位然后 panic

---

### 场景 3：遍历的时候删除 key 不会 panic！
```go
m := make(map[int]int)
for i := 0; i < 100; i++ {
    m[i] = i
}

// ✅ 这是合法，不会 panic！
for k := range m {
    delete(m, k)
}
```

**很多人以为这是 Go 1.1 之前会 panic，1.2 之后改了，现在是合法操作。

**为什么允许这么设计？**
因为遍历的时候做了特殊处理，删除不会导致内存破坏哈希表的结构。

---

### 场景 4：对已经关闭的 channel 发送数据
```go
ch := make(chan int, 1)
close(ch)
ch <- 1 // ❌ panic: send on closed channel
```

**为什么这么设计？**
如果不 panic，那你发完之后，发送者以为数据发出去了，接收者永远收不到，又是静默失败。

和 nil channel 接收不会 panic，会返回零值 + ok=false：
```go
v, ok := <-ch // ok  // ✅ 不会 panic
```

---

### 场景 5：close 已经 close 的 channel
```go
ch := make(chan int)
close(ch)
close(ch) // ❌ panic: close of closed channel
```

**为什么这么设计？**
谁负责 close 是一种「状态」操作，如果你 close 两次语义上就没这个设计。
如果 close 两次不 panic，那第二个 close 到底是什么意思？是「再关闭」还是「确认关闭」？语义非常模糊。

---

### 场景 6：向 nil channel 发送/接收
```go
var ch chan int
ch <- 1        // ❌ 永久阻塞，不是 panic！
_ = <-ch      // ❌ 永久阻塞，不是 panic！
```

**注意：这里不是 panic，是永久阻塞！**

**为什么这么设计？**
nil channel 通常用在 select 里做「禁用某个分支。比如你想动态禁用某个 case：
```go
var ch chan int
select {
case <-ch: // 这个分支永远不会被选中
    ...
case <-other:
    ...
}
```
这是一个很常用的模式，如果 nil channel 会 panic 的话，这个模式就没法用了。

---

### 场景 7：map 作为函数参数里修改 map 会不会 panic？
```go
func modify(m map[int]int) {
    m[1] = 100 // ✅ 不会 panic，外面能看到修改
}
```

**不会 panic，因为 map 是指针类型，传参拷贝的是指针，指向同一个底层 hmap 对象。**

这是和 slice 最核心的区别之一，很多人搞反了。

---

## 二、设计哲学总结

Go 所有这些 panic 设计，背后都是同一个原则：

> **明确优于静默失败 > 早失败 > 晚失败 > 静默失败 > 错误数据 > 崩溃 > 模糊语义**

| 设计选择 | 背后的考量 |
|---------|----------|
| 写 nil map 直接 panic | 不要让你以为数据存进去了，其实没存 |
| 并发读写直接 panic | 尽早暴露并发 bug，不要带着 bug 藏得太深生产环境出问题查都查不到 |
| 重复 close channel 直接 panic | 语义明确，close 就是「我负责关闭，出问题当时就炸，不要等到数据错乱 |
| nil channel 永久阻塞 | 支持 select 禁用分支的合法模式 |

---

## 三、面试高频问答

| 问题 | 答案 |
|------|------|
| map 哪些操作会 panic？ | 1) 写 nil map；2) 并发读写；3) 其他都不会。 |
| 并发读会不会 panic？ | 不会，只有只要有写的时候才会。 |
| 遍历的时候删除 key 会不会 panic？ | 不会，Go 1.2 之后是合法操作。 |
| 为什么并发读写 panic 是 race detector 导致的吗？ | 不是，是 runtime 本身的写标志位检查，没有 race 也会 panic。 |
| 为什么不像 Java 那样支持并发？ | 95% 场景不需要并发，为了 5% 场景让 95% 场景都变慢不值当。 |
| 为什么不像 sync.Map 不会 panic？ | 因为它内部有锁，而且是空间换时间，读和写用两个 map 两套数据结构不一样。 |
| 怎么避免并发 map panic？ | 读多用 sync.RWMutex，读写都多且 key 基本不变用 sync.Map。 |
| channel 哪些操作会 panic？ | 1) 向已关闭的 channel 发数据；2) close 已经 close 的 channel。 |
| 从已关闭的 channel 读会不会 panic？ | 不会，返回零值 + ok=false。 |
| nil channel 读写会不会 panic？ | 不会，会永久阻塞。 |

---

## 四、一句话总结

> Go 的设计哲学是「宁可让你在开发阶段就崩溃，也不要你在生产环境静默出难以排查的诡异 bug。
>
> Panic 不是惩罚。
>
> 你觉得 Go 故意让你崩溃总比让你不知道什么时候出问题好。
>
> 早崩溃，晚崩溃不如早崩溃。
>
> 静默失败是万恶之源。