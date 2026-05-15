# WaitGroup 完整解析 + 同步类型为什么不能传值

---

## 一、WaitGroup 结构（Go 1.17+ 最新版）

```go
type WaitGroup struct {
    noCopy noCopy        // 禁止值拷贝（go vet 检查）
    state  atomic.Uint64 // 高 32 位 = counter，低 32 位 = waiter 数
    sema   uint32        // 信号量，用于阻塞/唤醒 Wait 的 goroutine
}
```

**一个 uint64 存了两个值：**

```
state (atomic.Uint64)
┌──────────────────────────┬──────────────────────────┐
│     高 32 位: counter      │     低 32 位: waiter       │
│   (还没 Done 的 goroutine)  │  (正在 Wait 阻塞的 goroutine) │
└──────────────────────────┴──────────────────────────┘
```

---

## 二、三个方法背后的操作

### 1. Add(delta) — 原子更新计数器

```go
func (wg *WaitGroup) Add(delta int) {
    // 1. 原子加：把 delta 左移 32 位加到 state（只改高 32 位 counter）
    state := wg.state.Add(uint64(delta) << 32)
    v := int32(state >> 32)  // 取出 counter
    w := uint32(state)       // 取出 waiter 数

    // 2. counter < 0 → panic（Done 比 Add 多了）
    if v < 0 {
        panic("sync: negative WaitGroup counter")
    }

    // 3. 有 waiter 且 counter 从 0 变正 → panic
    //    说明有人在 Wait 的同时，又 Add 了新的任务（上一轮还没结束）
    if w != 0 && delta > 0 && v == int32(delta) {
        panic("sync: WaitGroup misuse: Add called concurrently with Wait")
    }

    // 4. counter > 0 或没有 waiter → 直接返回
    if v > 0 || w == 0 {
        return
    }

    // 5. counter == 0 且 waiter > 0 → 唤醒所有 Wait 的 goroutine！
    wg.state.Store(0)  // 重置 state
    for ; w != 0; w-- {
        runtime_Semrelease(&wg.sema, false, 0) // 释放信号量，逐个唤醒
    }
}
```

**核心流程：**
```
Add(1)  → counter++ → counter > 0  → 返回
Add(-1) → counter-- → counter > 0  → 返回
Add(-1) → counter-- → counter == 0 → 唤醒所有 Wait 的 goroutine！
Add(-1) → counter-- → counter < 0 → panic！
```

---

### 2. Done() — 就是 Add(-1)

```go
func (wg *WaitGroup) Done() {
    wg.Add(-1)
}
```

没有任何额外逻辑，Done 就是减 1。

---

### 3. Wait() — CAS 自旋 + 信号量阻塞

```go
func (wg *WaitGroup) Wait() {
    for {
        state := wg.state.Load()
        v := int32(state >> 32)  // counter
        w := uint32(state)       // waiter

        // 1. counter == 0 → 直接返回，不用等
        if v == 0 {
            return
        }

        // 2. CAS：waiter 数 +1（可能和其他 Wait 竞争，失败就重试）
        if wg.state.CompareAndSwap(state, state+1) {
            // 3. 信号量阻塞，等 Add 中 counter==0 时唤醒
            runtime_SemacquireWaitGroup(&wg.sema)

            // 4. 被唤醒后检查 state
            if wg.state.Load() != 0 {
                panic("sync: WaitGroup is reused before previous Wait has returned")
            }
            return
        }
        // CAS 失败 → state 被别人改了，重试
    }
}
```

**核心流程：**
```
Wait() → 读 state
  → counter == 0? → 直接返回
  → counter > 0?  → CAS(waiter+1) → 信号量阻塞 → 被 Add 唤醒 → 返回
  → CAS 失败?     → 重试循环
```

---

## 三、Wait() 能否被多个 goroutine 调用？

**可以！** 源码明确支持。

```go
var wg sync.WaitGroup
wg.Add(3)

for i := 0; i < 3; i++ {
    go func() { defer wg.Done(); time.Sleep(100 * time.Millisecond) }()
}

// ✅ 多个 goroutine 同时 Wait
go func() {
    wg.Wait()
    fmt.Println("waiter 1: done")
}()
go func() {
    wg.Wait()
    fmt.Println("waiter 2: done")
}()
```

**原理：** 多个 goroutine 同时 Wait 时，各自通过 CAS 把 waiter 计数器 +1。当 counter 归零时，Add 遍历 waiter 数，逐个 `Semrelease` 唤醒。

---

## 四、Wait() 能否被同一个 goroutine 多次调用？

**可以，但必须等上一轮 Wait 返回后才能开始下一轮。**

```go
// ✅ 正确：Wait 返回后复用
var wg sync.WaitGroup

// 第一轮
wg.Add(2)
go func() { defer wg.Done() }()
go func() { defer wg.Done() }()
wg.Wait()  // 第一轮等完

// 第二轮（复用同一个 WaitGroup）
wg.Add(3)
go func() { defer wg.Done() }()
go func() { defer wg.Done() }()
go func() { defer wg.Done() }()
wg.Wait()  // 第二轮等完
```

```go
// ❌ 错误：Wait 还没返回就复用
var wg sync.WaitGroup
wg.Add(1)
go func() {
    wg.Wait()       // 第一个 Wait 还在阻塞
    // 此时如果外面又 Add 了新的任务，会 panic！
}()
// 必须等 Wait 返回后，才能开始新一轮 Add
```

**源码中的检测：** Wait 被唤醒后检查 `wg.state.Load() != 0`，如果不为 0 说明有人在前一轮 Wait 还没返回时就 Add 了，直接 panic。

---

## 五、六条使用规则（面试必背）

| 规则 | 说明 | 违反后果 |
|------|------|---------|
| **1. Add 必须在 goroutine 启动前** | `wg.Add(1)` 在 `go func()` 前面 | Wait 可能提前返回 |
| **2. Done 和 Add 必须配对** | 每个 Add(1) 必须对应一个 Done() | counter 不为 0，Wait 永远阻塞 |
| **3. counter 不能为负** | Done 比 Add 多了 | panic: negative WaitGroup counter |
| **4. 不能在 Wait 期间 Add** | 上一轮 Wait 还没返回就 Add | panic: WaitGroup misuse |
| **5. WaitGroup 不能拷贝** | 传值而不是传指针 | 状态不一致，行为未定义 |
| **6. 复用必须等 Wait 返回** | 新一轮 Add 必须在上一轮 Wait 后 | panic: WaitGroup is reused |

---

## 六、常见错误写法

### 错误 1：Add 在 goroutine 里面

```go
// ❌ Wait 可能提前返回
for i := 0; i < 10; i++ {
    go func() {
        wg.Add(1)    // ❌ 可能在 Wait 之后才执行
        defer wg.Done()
    }()
}
wg.Wait()  // 可能 counter 还是 0，直接返回！

// ✅ 正确
for i := 0; i < 10; i++ {
    wg.Add(1)       // ✅ 在启动 goroutine 前加
    go func() {
        defer wg.Done()
    }()
}
wg.Wait()
```

### 错误 2：Add 和 Done 不配对

```go
wg.Add(5)
go func() { wg.Done() }()  // 1
go func() { wg.Done() }()  // 2
go func() { wg.Done() }()  // 3
// 缺 2 次 Done！Wait 永远阻塞！
```

### 错误 3：WaitGroup 值拷贝

```go
// ❌ 值拷贝，两个 WaitGroup 状态不同步
func worker(wg sync.WaitGroup) {  // ❌ 值拷贝！
    defer wg.Done()               // 改的是副本，原 wg 不受影响
}

// ✅ 传指针
func worker(wg *sync.WaitGroup) { // ✅ 指针
    defer wg.Done()               // 改的是同一个对象
}
```

---

## 七、为什么同步类型不能传值，必须传指针？（面试核心！）

### 1. 哪些类型不能传值？

所有包含**内部状态**的同步原语都不能值拷贝：

| 类型 | 有 noCopy | 不能传值的原因 |
|------|-----------|--------------|
| `sync.WaitGroup` | ✅ | counter 和 waiter 状态分裂 |
| `sync.Mutex` | ✅ | 锁状态分裂，两个锁互相独立 |
| `sync.RWMutex` | ✅ | 同上 |
| `sync.Once` | ✅ | done 标记分裂，f 可能执行多次 |
| `sync.Cond` | ✅ | 等待队列分裂 |
| `sync.Pool` | ✅ | 本地缓存分裂 |
| `sync.Map` | ✅ | 内部状态分裂 |
| `atomic.Int64` 等 | ✅ | 原子性被破坏 |

---

### 2. 根本原因：值拷贝 = 状态分裂

**值拷贝会创建一个全新的副本，副本和原件各自独立，互不影响。**

对于普通结构体，值拷贝没问题：

```go
type Point struct{ X, Y int }
p1 := Point{1, 2}
p2 := p1  // ✅ 拷贝没问题，p1 和 p2 各自独立
p2.X = 10  // 不影响 p1
```

但同步原语的**多个 goroutine 必须操作同一个状态**。值拷贝会导致状态分裂：

#### 例子 1：WaitGroup 值拷贝

```go
var wg sync.WaitGroup
wg.Add(2)

// ❌ 值拷贝：wg2 是一个全新的副本，counter 也是 2
// 但 wg2 和 wg 完全独立！
go func(wg2 sync.WaitGroup) {
    defer wg2.Done()  // 改的是 wg2 的 counter: 2→1
    // wg 的 counter 还是 2！
}(wg)

wg.Wait()  // ❌ 永远阻塞！因为 wg 的 counter 永远到不了 0
```

**图解：**
```
原始 wg:  counter=2, waiter=0, sema=0
              ↓ 值拷贝
副本 wg2: counter=2, waiter=0, sema=0  ← 完全独立的一份！

goroutine A 调用 wg2.Done() → 只改 wg2 的 counter: 2→1
goroutine B 调用 wg.Wait()  → 等 wg 的 counter 变 0 → 永远等不到！
```

#### 例子 2：Mutex 值拷贝

```go
var mu sync.Mutex

// goroutine 1 用原始 mu 加锁
mu.Lock()

// ❌ 值拷贝：mu2 是全新的，处于未锁定状态
mu2 := mu
mu2.Unlock()  // ✅ 能解锁（mu2 本来就没锁）

// mu 还是锁定的！死锁！
mu.Lock()  // ❌ 死锁！
```

#### 例子 3：Once 值拷贝

```go
var once sync.Once

// ❌ 值拷贝后，两个 Once 的 done 标记各自独立
once2 := once

once.Do(func() { fmt.Println("第一次") })   // 执行
once2.Do(func() { fmt.Println("又执行了") }) // ❌ 又执行了！
// Once 的保证被打破了！
```

---

### 3. noCopy 机制：编译器如何帮你检测？

Go 的同步原语都嵌入了 `noCopy` 类型：

```go
// noCopy 的实现（在 sync/cond.go 中）
type noCopy struct{}

// Lock/Unlock 是空操作，但实现了 locker 接口
// go vet 通过检查这个接口来发现值拷贝
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
```

**检测方式：**

```bash
# go vet 会检查值拷贝
go vet ./...

# 报错示例
# main.go:10:9: wg copies a WaitGroup by value (sync.WaitGroup must not be copied)
```

**但 go vet 不是编译器强制检查！** 它是静态分析工具，不运行 `go vet` 就检测不到。所以你需要：

1. 写代码时养成习惯：同步类型始终传指针
2. CI/CD 流水线加 `go vet` 检查
3. IDE 安装 lint 插件（gopls 自带）

---

### 4. 还有其他不能传值的类型

除了同步原语，还有一些类型也不能传值：

| 类型 | 不能传值的原因 |
|------|--------------|
| `time.Ticker` | 内部有 goroutine 和 channel |
| `time.Timer` | 同上 |
| `http.Client` | 内部有 Transport 和连接池 |
| `os.File` | 内部有文件描述符，拷贝后 close 会重复关闭 |
| `io.Reader/Writer` 实现 | 如果内部有缓冲区，拷贝后状态不一致 |

---

### 5. 一句话总结

> **同步类型不能传值，因为值拷贝会创建独立副本，导致多个 goroutine 操作不同的状态，同步机制完全失效。传指针保证所有 goroutine 操作同一个对象。**

---

## 八、面试高频问答

| 问题 | 答案 |
|------|------|
| Wait() 能被多个 goroutine 调用吗？ | 能。多个 goroutine 同时 Wait，counter 归零时全部被唤醒。 |
| Wait() 能被同一个 goroutine 调用多次吗？ | 能，但必须等上一轮 Wait 返回后才能开始新一轮 Add。 |
| Add 在 goroutine 里面会怎样？ | Wait 可能提前返回，因为 Add 可能还没执行，counter 已经是 0。 |
| counter 变成负数会怎样？ | panic: negative WaitGroup counter。 |
| 为什么 WaitGroup 不能值拷贝？ | 值拷贝创建独立副本，Done 改的是副本的 counter，原 counter 不变，Wait 永远阻塞。 |
| go vet 能检测值拷贝吗？ | 能，通过 noCopy 机制。但不是编译器强制检查，需要主动运行 go vet。 |
| Mutex 值拷贝会怎样？ | 锁状态分裂，两个独立的锁，互不影响，加锁/解锁完全错乱。 |
| Once 值拷贝会怎样？ | done 标记分裂，f 可能被执行多次，Once 的保证被打破。 |
| 所有同步类型都嵌入了 noCopy 吗？ | 是的。WaitGroup、Mutex、Once、Cond、Pool、Map 都有。 |
| 原子类型（atomic.Int64）也不能拷贝吗？ | 不能！拷贝后原子性被破坏，两个独立的变量各自操作，不再是同一个原子变量。 |
| Go 编译器会强制阻止值拷贝吗？ | 不会。只有 go vet 能检测，编译器不报错。所以必须养成传指针的习惯。 |
