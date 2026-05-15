# 2022 面试题整理 — Go 基础篇（1/5）

> 原始面试记录：`last.md`，整理时间 2026-05，部分答案已按最新 Go 版本更新。

---

## Q1：slice 底层是怎么实现的，包含哪些属性

slice 底层是一个结构体，包含三个字段：

```go
type slice struct {
    array unsafe.Pointer  // 指向底层数组的指针
    len   int             // 当前元素个数
    cap   int             // 底层数组的容量
}
```

```
slice header
┌──────────┬──────┬──────┐
│  array   │ len  │ cap  │
│  指针    │  3   │  5   │
└────┬─────┴──────┴──────┘
     ↓
底层数组: [a, b, c, _, _]
              ↑len=3      ↑cap=5
```

**关键点：**
- slice 本身是值类型，但内部持有底层数组的指针
- 多个 slice 可以共享同一个底层数组（切片再切片）
- slice 的赋值是浅拷贝（header 拷贝，底层数组共享）

---

## Q2：slice 扩容的机制是什么

### Go 1.18 之前

- 如果新容量 > 旧容量 × 2，直接用新容量
- 否则，旧容量 < 1024 时翻倍；旧容量 >= 1024 时按 1.25 倍增长
- 最终容量会根据元素大小进行内存对齐

### Go 1.18+（当前）

扩容策略改为更平滑的增长，不再区分 1024 的分界线：

```go
// runtime/slice.go growslice 核心逻辑
newcap := old.cap
doublecap := newcap + newcap

if newLen > doublecap {
    newcap = newLen
} else {
    const threshold = 256
    if old.cap < threshold {
        newcap = doublecap  // 小于 256 直接翻倍
    } else {
        // 平滑增长：1.25x + 192 → 1.33x → 1.5x → ...
        for 0 < newcap && newcap < newLen {
            newcap += (newcap + 3*threshold) / 4
        }
        if newcap <= 0 {
            newcap = newLen
        }
    }
}
```

**最后还要做内存对齐**：根据元素大小向上取整到 `spanClass` 对应的 size class，所以实际分配的 cap 可能比计算值更大。

**扩容时一定会复制底层数组**，旧的 slice 和新的 slice 不再共享底层数组。

---

## Q3：new 和 make 的区别

| | `new(T)` | `make(T, ...)` |
|---|---|---|
| **用途** | 分配零值内存 | 初始化 slice/map/channel |
| **返回值** | `*T`（指针） | `T`（值，不是指针） |
| **适用类型** | 任意类型 | 仅 slice、map、channel |
| **初始化** | 只设零值，不初始化内部结构 | 完整初始化，可直接使用 |

```go
// new
p := new(int)       // *int, 值为 0
s := new([]int)     // *[]int, 指向 nil slice（不能直接用！）
m := new(map[string]int) // *map[string]int, 指向 nil map（不能直接写！）

// make
s := make([]int, 0, 10)   // []int, 已分配底层数组，可直接用
m := make(map[string]int)  // map[string]int, 已初始化，可直接用
ch := make(chan int, 5)    // chan int, 缓冲区大小 5
```

**关键：** `new` 只分配零值，对 slice/map/channel 只会得到 nil，必须用 `make` 才能初始化内部数据结构。

---

## Q4：Go 语言的函数参数是怎么传递的

**Go 只有值传递，没有引用传递。**

传递指针时，也是值传递——拷贝的是指针的值（地址），不是被指向的对象。

```go
// 值传递：拷贝整个结构体
func byValue(u User) {
    u.Name = "changed"  // 改的是副本，不影响原值
}

// 指针传递：拷贝指针（地址值）
func byPointer(u *User) {
    u.Name = "changed"  // 通过指针修改原对象
}

// slice 传递：拷贝 slice header（指针+len+cap）
func bySlice(s []int) {
    s[0] = 999       // ✅ 改了底层数组，原 slice 也能看到
    s = append(s, 1) // ❌ 可能触发扩容，新底层数组，原 slice 不受影响
}

// map 传递：拷贝 map 的指针
func byMap(m map[string]int) {
    m["key"] = 1  // ✅ 改的是同一个 map
}
```

**本质：** 所有参数都是值拷贝。slice/map/channel 因为内部持有指针，所以看起来像"引用传递"，但本质还是值传递。

---

## Q5：Go 语言闭包使用注意的问题

闭包捕获的是变量的**引用**，不是值的副本。

### 问题 1：循环变量（经典坑）

```go
// ❌ Go 1.22 之前：所有 goroutine 输出 10
for i := 0; i < 10; i++ {
    go func() {
        fmt.Println(i)  // 捕获 i 的引用
    }()
}

// ✅ 修复 1：参数传递
for i := 0; i < 10; i++ {
    go func(n int) {
        fmt.Println(n)
    }(i)
}

// ✅ 修复 2：局部拷贝
for i := 0; i < 10; i++ {
    i := i
    go func() {
        fmt.Println(i)
    }()
}

// ✅ Go 1.22+：每次迭代自动创建新变量，无需修复
```

### 问题 2：闭包导致变量逃逸到堆

```go
func counter() func() int {
    count := 0  // 逃逸到堆！闭包捕获
    return func() int {
        count++
        return count
    }
}
```

`count` 原本可以在栈上，但闭包可能在函数返回后执行，所以必须逃逸到堆。

### 问题 3：闭包持有引用导致内存不释放

```go
func leak() func() {
    bigData := make([]byte, 1<<30) // 1GB
    return func() {
        _ = bigData  // 闭包持有 bigData 的引用，1GB 不会被 GC
    }
}
```

---

## Q6：for 循环 + goroutine 输出什么

> 详见 Q5 和 Q1 的专门文档 `12-interview-qa/go-interview-qa.md`

**Go 1.22 之前：** 大概率输出 10 个 `10`（闭包捕获共享变量 `i` 的引用）
**Go 1.22+：** 输出 0~9 随机排列（每次迭代创建新 `i`）

---

## Q7：读写锁和互斥锁的区别，应用场景

| | `sync.Mutex` | `sync.RWMutex` |
|---|---|---|
| **读操作** | 互斥，同一时刻只有一个 goroutine | 共享，多个 goroutine 可同时读 |
| **写操作** | 互斥 | 互斥，写时阻塞所有读和写 |
| **性能** | 简单粗暴 | 读多写少时性能更好 |
| **开销** | 较小 | 稍大（内部状态更复杂） |

**RWMutex 内部结构：**

```go
type RWMutex struct {
    w           Mutex        // 写锁
    writerSem   uint32       // 写等待信号量
    readerSem   uint32       // 读等待信号量
    readerCount atomic.Int32 // 当前读 goroutine 数（正=读者数，负=有写者在等）
    readerWait  atomic.Int32 // 写者等待读者退出的计数
}
```

**写锁的饥饿问题：** Go 1.9+ 的 RWMutex 实现了**写优先**——当有写者在等待时，新的读者会排队，防止写者被持续到来的读者饿死。

**选型：**
- 读写比例 > 10:1 → RWMutex
- 读写差不多或写多读少 → Mutex（RWMutex 的额外开销反而更慢）
- 临界区极短 → Mutex（RWMutex 的内部状态管理开销不划算）

---

## Q8：反复创建销毁对象有没有优化方式 — sync.Pool

sync.Pool 是对象复用池，减少内存分配和 GC 压力。

```go
var bufPool = sync.Pool{
    New: func() any {
        return bytes.NewBuffer(make([]byte, 0, 1024))
    },
}

func processData() {
    buf := bufPool.Get().(*bytes.Buffer)
    buf.Reset()  // ⚠️ 一定要重置！
    defer bufPool.Put(buf)

    buf.WriteString("hello")
    // ...
}
```

**核心特性：**
- 每个 P 有本地池，Get/Put 优先操作本地池，无锁
- GC 时 local → victim（冷备份），下次 GC 清空 victim（两轮 GC 存活时间）
- 没有容量限制，不保证取回

**不能存：** 数据库连接、TCP 连接（GC 不会调 Close，连接泄漏）
**适合存：** bytes.Buffer、临时结构体、JSON 序列化缓冲区（无状态纯内存对象）

---

## Q9：协程并发控制的几种方式

### 1. sync.WaitGroup — 等待一组 goroutine 完成

```go
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        // 任务
    }()
}
wg.Wait()
```

### 2. Channel — 信号通知 / 数据传递

```go
done := make(chan struct{})
go func() {
    // 任务
    close(done)  // 完成后关闭
}()
<-done  // 等待完成
```

### 3. context.Context — 超时控制 / 取消传播

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

go func() {
    select {
    case <-ctx.Done():
        fmt.Println("超时或取消:", ctx.Err())
    case result <- doWork():
        fmt.Println("完成:", result)
    }
}()
```

**选型：**

| 方式 | 场景 |
|------|------|
| WaitGroup | 等待一批任务全部完成 |
| Channel | goroutine 间通信、信号通知、扇出扇入 |
| Context | 超时控制、取消传播、请求级生命周期 |

---

## Q10：slice 是线程安全的么，map 是线程安全的么

**slice 不安全：** 并发 append 会破坏 len/cap、底层数组，fatal error。

```go
// ❌ 并发写 slice 会 panic
var s []int
go func() { s = append(s, 1) }()
go func() { s = append(s, 2) }()
```

**map 不安全：** 并发读写直接 fatal error（不是 panic，无法 recover）。

```go
// ❌ fatal error: concurrent map read and map write
m := make(map[string]int)
go func() { m["key"] = 1 }()
go func() { _ = m["key"] }()
```

**并发安全的替代方案：**

| 方案 | 适用场景 |
|------|---------|
| `map + Mutex` | 通用 |
| `map + RWMutex` | 读多写少 |
| `sync.Map` | 读多写少、key 稳定（Go 1.24+ 用 HashTrieMap，性能更好） |

---

## Q11：Go 的内存逃逸是怎么回事

编译器在编译阶段做**逃逸分析**，决定变量分配在栈还是堆。

**栈分配：** 函数返回时自动回收，零开销。
**堆分配：** 需要 GC 回收，有额外开销。

**六大逃逸场景：**

| 场景 | 示例 |
|------|------|
| 返回局部变量指针 | `return &x` |
| 闭包捕获变量 | 闭包引用的局部变量 |
| 赋值给 interface{} | `fmt.Println(x)` |
| 发送到 channel | `ch <- &data` |
| slice/map 动态扩容 | 无预分配的 append |
| 大对象 > 64KB | `make([]byte, 100*1024)` |

**查看逃逸：** `go build -gcflags="-m"`

**优化：** 小结构体返回值不返回指针、预分配容量、避免不必要的 interface{}、sync.Pool 复用堆对象。

---

## Q12：进程、线程、协程的区别，MPG 的实现机制

### 三者对比

| | 进程 | 线程 | 协程（Goroutine） |
|---|---|---|---|
| **调度** | OS 内核调度 | OS 内核调度 | Go 运行时用户态调度 |
| **切换开销** | 大（切换页表等） | 中（切换寄存器/栈） | 极小（~几条指令） |
| **栈大小** | MB 级 | MB 级（默认 1-8MB） | KB 级（初始 2KB，动态增长） |
| **创建开销** | 大 | 中 | 极小（~几百字节） |
| **通信** | IPC（管道/共享内存/信号） | 共享内存 + 锁 | Channel（CSP 模型） |

### GMP 模型

```
G (Goroutine)  — 协程，用户态轻量线程
M (Machine)    — 操作系统线程
P (Processor)  — 逻辑处理器，GOMAXPROCS 控制数量
```

```
    全局队列
  ┌───────────┐
  │ G7 G8 G9  │
  └───────────┘
       ↑ steal
  ┌────┼────┐
  ↓    ↓    ↓
┌────┐┌────┐┌────┐
│ P0 ││ P1 ││ P2 │  ← GOMAXPROCS 个 P
│ G1 ││ G4 ││ G6 │
│ G2 ││ G5 │     │
│ G3 ││    │     │
└─┬──┘└──┬─┘└──┬─┘
  ↓      ↓      ↓
┌────┐┌────┐┌────┐
│ M0 ││ M1 ││ M2 │  ← 操作系统线程
└────┘└────┘└────┘
```

**GMP 调度流程（findrunnable 7 步）：**

1. 本地队列取 G
2. 全局队列取 G（每 61 次调度检查一次，防饥饿）
3. 网络 poller 取 G（net I/O 就绪的）
4. Work Stealing：从其他 P 的本地队列偷一半
5. 再次检查全局队列
6. 再次检查网络 poller
7. 休眠 M（把 P 交出去，M 放入空闲列表）

**抢占式调度三阶段：**
- Go 1.0：基于函数调用的栈检查（无法抢占无函数调用的循环）
- Go 1.14：基于信号的异步抢占（sysmon 发 SIGURG，即使死循环也能抢占）
- Go 1.14+：协作 + 信号双重保障

---

## Q13：channel 的遍历场景，for range 遍历 channel 怎么退出

```go
ch := make(chan int, 10)

// for range 遍历 channel
for v := range ch {
    fmt.Println(v)
    // channel 被 close 后，range 会自动退出
}
fmt.Println("channel 已关闭，退出遍历")
```

**退出条件：** 只有 `close(ch)` 才能让 `for range` 退出。channel 中没有数据时会阻塞等待，不会退出。

**注意：** 只能由发送方 close，不能在接收方 close，也不能 close 两次。

---

## Q14：有哪些场景可能引发 panic

| 场景 | 示例 |
|------|------|
| 数组/slice 越界 | `s[10]` 而 len(s) < 10 |
| nil 指针解引用 | `var p *int; *p` |
| map 并发读写 | 两个 goroutine 同时读写同一个 map |
| channel 操作已关闭的 channel | 向 closed channel 发送数据 |
| 类型断言失败 | `var i any = "hi"; v := i.(int)` |
| 除以零 | `var n int; fmt.Println(1/n)` |
| WaitGroup 计数器为负 | `wg.Add(-1)` 比 `wg.Add(1)` 多 |
| WaitGroup 复用不当 | Wait 期间 Add |
| Once 递归调用 | `once.Do(func() { once.Do(f) })` 死锁 |
| close 已关闭的 channel | `close(ch)` 两次 |
| 重复 close channel | 同上 |
| slice 越界切片 | `s[5:10]` 而 cap(s) < 10 |

**recover 只能捕获 panic，不能捕获 fatal error（如 map 并发读写）。**

---

## Q15：Go 的继承是怎么实现的，嵌套 struct 的未导出属性能否被访问

Go 没有经典的继承，用**组合（嵌入）**实现类似效果：

```go
type Animal struct {
    Name string
    age  int  // 未导出
}

func (a *Animal) Speak() string {
    return a.Name + " speaks"
}

type Dog struct {
    Animal  // 嵌入 Animal，Dog "继承"了 Animal 的方法
    Breed string
}

func main() {
    d := Dog{Animal: Animal{Name: "Rex", age: 3}, Breed: "Lab"}

    d.Speak()       // ✅ 可以调用（方法提升）
    d.Name          // ✅ 可以访问（Name 是导出的）
    d.Animal.age    // ✅ 可以访问（通过类型名显式访问，如果在同一个包内）
    d.age           // ✅ 同包内可以，❌ 不同包不行
}
```

**规则：**
- 嵌入的字段和方法会被"提升"到外层结构体
- **未导出字段**在不同包中无法直接访问（`d.age` 不行），但可以通过同包的导出方法间接访问
- **同一个包内**，未导出字段可以正常访问（`d.age` 和 `d.Animal.age` 都行）
- Go 的嵌入是组合，不是继承：没有多态，`Dog` 不是 `Animal` 的子类型（但 `Dog` 实现了 `Animal` 实现的所有接口）

---

## Q16：多个协程获取同一份数据，一个成功其他停止

面试官说的是 **singleflight**！

```go
var g singleflight.Group

func getData(key string) (string, error) {
    v, err, _ := g.Do(key, func() (any, error) {
        // 只有第一个调用会真正执行
        time.Sleep(100 * time.Millisecond) // 模拟耗时
        return "data-for-" + key, nil
    })
    return v.(string), err
}

// 10 个 goroutine 同时请求同一个 key
// 只有 1 个真正执行，其他 9 个共享结果
for i := 0; i < 10; i++ {
    go func() {
        data, _ := getData("user:123")
        fmt.Println(data)  // 全部输出 "data-for-user:123"
    }()
}
```

**原理：** 同一个 key 的并发请求会被合并（coalesce），第一个请求执行函数，其余请求等待并共享结果。

**与 Once 的区别：**
- Once：永远只执行一次（"once ever"）
- singleflight：同一批并发请求只执行一次，下一批请求会重新执行

---

## Q17：访问其他业务方数据，超过一定时间不再等待

用 **context.WithTimeout**：

```go
func callExternal(ctx context.Context) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, "GET", "http://external/api", nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return "", fmt.Errorf("请求超时")
        }
        return "", err
    }
    defer resp.Body.Close()
    // ...
    return "ok", nil
}
```

**其他超时控制手段：**
- `time.After` + `select`（简单但无法取消底层操作）
- `context.WithTimeout`（推荐，能传播取消信号）
- `context.WithDeadline`（指定绝对时间点）
- HTTP Client 自带 `Timeout` 字段

---

## Q18：sync.Map、sync.Cond、sync.Pool 的作用和实现原理

### sync.Map

并发安全的 map，**读多写少**场景优于 map + RWMutex。

**旧实现（Go 1.9-1.23）：** 双 Map 设计
- read：原子操作无锁读
- dirty：Mutex 保护，新 key 先写这里
- miss 累积到 `len(dirty)` 时 dirty 提升为 read
- 已有 key 更新只需 CAS 修改 entry.p，无锁

**新实现（Go 1.24+）：** HashTrieMap（并发哈希字典树）
- 细粒度分段锁，不相关 key 无竞争
- 无预热期，key 可被 GC 回收

### sync.Cond

条件变量，让 goroutine 等待/通知某个条件成立。

```go
var mu sync.Mutex
cond := sync.NewCond(&mu)

// 等待方
cond.L.Lock()
for !condition() {  // ⚠️ 必须用 for，不能用 if（虚假唤醒）
    cond.Wait()     // 释放锁 → 阻塞 → 被唤醒后重新获取锁
}
// 条件满足，执行逻辑
cond.L.Unlock()

// 通知方
cond.L.Lock()
changeCondition()
cond.Signal()  // 唤醒一个 Wait 的 goroutine
// cond.Broadcast()  // 唤醒所有
cond.L.Unlock()
```

**注意：** 实际开发中很少用 Cond，channel + select 通常更清晰。

### sync.Pool

对象复用池，减少 GC 压力。（详见 Q8）

---

## Q19：互斥锁、读写锁是如何实现的

### Mutex（Go 1.9+ 正常+饥饿模式）

```go
type Mutex struct {
    state int32  // 锁状态位
    sema  uint32 // 信号量
}
```

**state 的位布局：**
```
┌─────────┬──────────┬──────────┬─────────────────────┐
│ 31..3   │   bit2   │   bit1   │       bit0          │
│ 等待者数 │ 饥饿标志 │ 唤醒标志 │  锁定标志           │
└─────────┴──────────┴──────────┴─────────────────────┘
```

**加锁流程：**

```
Lock()
  │
  ├── 快速路径：CAS(state, 0, 1) → 成功则拿到锁
  │
  └── 慢路径：lockSlow()
        │
        ├── 正常模式：
        │     ├── 新来 goroutine 自旋尝试抢锁（最多 4 次）
        │     ├── 抢不到 → 排队等待，被唤醒后还要和新生竞争
        │     └── 新来的 goroutine 更容易抢到锁（性能好，但可能饿死老 goroutine）
        │
        └── 饥饿模式（等待者等待 > 1ms）：
              ├── 新来的 goroutine 直接排队，不竞争
              ├── 被唤醒的 goroutine 直接获得锁
              └── 等待者只剩 1 个 或 等待时间 < 1ms → 切回正常模式
```

**解锁流程：**

```
Unlock()
  │
  ├── 快速路径：原子减 state-1 → 没有等待者则直接返回
  │
  └── 慢路径：unlockSlow()
        ├── 有等待者 → 唤醒一个
        ├── 饥饿模式 → 直接交给等待队列第一个
        └── 正常模式 → 唤醒一个，让它和新来的竞争
```

### RWMutex

```go
type RWMutex struct {
    w           Mutex        // 写锁
    writerSem   uint32       // 写等待信号量
    readerSem   uint32       // 读等待信号量
    readerCount atomic.Int32 // 读 goroutine 数（正=读者数，负=有写者在等）
    readerWait  atomic.Int32 // 写者需要等待的读者数
}
```

**RLock：** readerCount+1，如果有写者在等（readerCount < 0）则阻塞在 readerSem
**RUnlock：** readerCount-1，如果 readerWait > 0 则 readerWait-1，到 0 时唤醒写者
**Lock（写）：** readerCount 减去一个大数（1<<30），标记有写者在等，等所有读者退出
**Unlock（写）：** readerCount 恢复，唤醒所有等待的读者

**写优先机制：** 写者来时把 readerCount 变为负数，新的读者看到后直接排队，不会被后续读者饿死。
