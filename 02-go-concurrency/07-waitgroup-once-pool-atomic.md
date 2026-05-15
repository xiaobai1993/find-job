# WaitGroup / Once / Pool / Atomic 并发原语全解

***

## 一、WaitGroup：等待一组 goroutine 完成

### 1. 是什么？

WaitGroup 是等待一组 goroutine 全部完成的同步原语，非常适合批量任务等待场景。

```go
type WaitGroup struct {
    // 内部是两个 uint32：计数器 + 等待者数量 + 信号量
    // 用一个 64 位原子操作同时操作两个值
}
```

三个方法：

- `Add(n int)`：计数器 +n
- `Done()`：计数器 -1，等价于 Add(-1)
- `Wait()`：阻塞直到计数器变为 0

***

### 2. 底层原理：64 位原子操作的巧妙设计

**Go 1.17+ 实际结构：**

```go
type WaitGroup struct {
    noCopy noCopy              // 禁止拷贝标记（go vet 检查）
    state  atomic.Uint64       // 高 32 位 = 计数器，低 32 位 = 等待者数量
    sema   uint32              // 信号量
}
```

**Go 1.16 及更早版本的老结构：**

```go
type WaitGroup struct {
    state1 [3]uint32  // 为解决 32 位系统 8 字节对齐问题的巧妙设计
}
```

`state` 一个 uint64 存了两个值：

- **高 32 位**：计数器（还有多少任务没完成）
- **低 32 位**：等待者数量（有多少个 goroutine 在 Wait()）

**为什么这么设计？**

- 可以用一条原子操作同时修改两个值
- 不需要额外的互斥锁，性能极高
- Go 1.19 之后使用类型化 `atomic.Uint64`，更安全更清晰

**Add() 流程：**

1. 把计数器 +n
2. 如果计数器变成 0 了，唤醒所有在 Wait() 的 goroutine

**Wait() 流程：**

1. 检查计数器，如果已经是 0，直接返回
2. 否则等待者数量 +1，阻塞在信号量上

***

### 3. 四个常见坑（90% 的人踩过）

#### 坑 1：Add() 放在 goroutine 里面，Wait() 提前返回

```go
// ❌ 错误写法：Add 放在 goroutine 里面，Wait 可能先执行
func bad() {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        go func() {
            wg.Add(1)  // ❌ 太晚了！Wait 可能已经执行了
            defer wg.Done()
            // 任务
        }()
    }
    wg.Wait()  // 可能计数器还是 0，直接返回了！
}

// ✅ 正确写法：启动 goroutine 之前先 Add
func good() {
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)  // ✅ 先加
        go func() {
            defer wg.Done()
            // 任务
        }()
    }
    wg.Wait()
}
```

**支付业务提醒：** 批量查询账户、批量通知第三方，一定先 Add 再启动 goroutine！

***

#### 坑 2：WaitGroup 是值类型，传参拷贝失效

```go
// ❌ 错误：传值拷贝，两个 WaitGroup 不是同一个
func bad(wg sync.WaitGroup) {
    defer wg.Done()  // 减的是拷贝的计数器！
}

// ✅ 正确：传指针
func good(wg *sync.WaitGroup) {
    defer wg.Done()
}
```

和 Mutex 一样，只要是同步原语，**绝对不能值拷贝**！

***

#### 坑 3：计数器变成负数，直接 panic

```go
var wg sync.WaitGroup
wg.Add(-1)  // ❌ panic: sync: negative WaitGroup counter
```

**设计原因：** 计数器为负说明你的逻辑有 bug，要么 Done 多了，要么 Add 少了，必须立刻崩溃。

***

#### 坑 4：重用 WaitGroup 有风险

```go
var wg sync.WaitGroup

// 第一批任务
wg.Add(1)
go func() { wg.Done() }()
wg.Wait()

// 第二批任务（第一批 Wait 完了再加没问题）
wg.Add(1)  // ✅ 没问题
```

**规则：** 只有等 Wait() 返回了，才能再次调用 Add()。如果 Wait() 还在阻塞，Add() 会 panic。

***

### 4. 典型场景：批量任务并行执行

支付系统常见场景：同时查 3 个下游系统，等所有结果返回才继续。

```go
func BatchQuery(ctx context.Context, orderIDs []string) []*Result {
    var wg sync.WaitGroup
    results := make([]*Result, len(orderIDs))

    for i, id := range orderIDs {
        wg.Add(1)
        go func(i int, id string) {
            defer wg.Done()
            results[i] = querySingle(ctx, id)
        }(i, id)
    }

    wg.Wait()  // 等所有查询完成
    return results
}
```

比串行快 N 倍，支付系统的核心优化手段！

***

## 二、Once：保证只执行一次

### 1. 是什么？

Once 保证某个函数**只执行一次**，哪怕在多个 goroutine 同时调用也只执行一次。

最典型的场景：**单例模式初始化**。

```go
type Once struct {
    done uint32  // 标记是否已经执行过
    m    Mutex   // 保护初始化过程
}
```

只有一个方法：`Do(f func())`

***

### 2. 底层原理：双重检查锁定（Double-Checked Locking）

**Go 1.19+ 实际结构与代码（使用类型化原子操作）：**

```go
type Once struct {
    done atomic.Uint32  // Go 1.19+ 使用类型化原子变量
    m    Mutex
}

func (o *Once) Do(f func()) {
    // 第一次检查：无锁，快速路径
    if o.done.Load() == 0 {
        o.doSlow(f)
    }
}

func (o *Once) doSlow(f func()) {
    o.m.Lock()
    defer o.m.Unlock()

    // 第二次检查：有锁，慢路径
    if o.done.Load() == 0 {
        defer o.done.Store(1)  // defer 保证 f 执行完才设为 1
        f()  // 执行初始化
    }
}
```

**⚠️ 非常重要的设计细节：**
`done.Store(1)` 是用 `defer` 执行的，这保证了：

- **只有 f 正常返回后**，done 才会被设为 1
- 如果 f panic 了，done **也会被设为 1**（和原文说的一致）
- 保证所有调用 `Do()` 的 goroutine **都必须等 f 执行完才能返回**

这就是为什么不能直接用 CAS 实现 Once 的原因：

```go
// ❌ 错误实现！
if o.done.CompareAndSwap(0, 1) {
    f()  // 第一个 goroutine 执行 f
}
// 第二个 goroutine CAS 失败直接返回，不会等 f 执行完！
// 这违反了 Once 的语义保证！
```

**为什么要检查两次？**

- 第一次检查：99% 的情况 done 已经是 1，不用拿锁，直接返回，性能很高
- 第二次检查：拿到锁之后再确认一次，防止并发情况下重复执行

**这就是著名的双重检查锁定模式！**

***

### 3. 三个常见坑

#### 坑 1：Do() 里面递归调用 Do()，死锁

```go
var once sync.Once

func initConfig() {
    once.Do(func() {
        initConfig()  // ❌ 递归调用 Do()，死锁！
    })
}
```

原因：Mutex 不是可重入锁，同一个 goroutine 第二次拿自己已经持有的锁，直接死锁。

***

#### 坑 2：初始化失败了怎么办？

Once 只执行一次，哪怕 f() panic 了，done 也会被设为 1，下次不会再执行了。

```go
// ❌ 问题：初始化 panic 了，下次调用直接返回不执行
var once sync.Once

func Init() {
    once.Do(func() {
        panic("数据库连不上")
    })
}

// ✅ 解决：自己管理重试逻辑
var (
    initDone uint32
    initMu   sync.Mutex
)

func Init() error {
    if atomic.LoadUint32(&initDone) == 1 {
        return nil
    }

    initMu.Lock()
    defer initMu.Unlock()

    if initDone == 1 {
        return nil
    }

    if err := initDatabase(); err != nil {
        return err  // 失败不设标记，下次还可以重试
    }

    atomic.StoreUint32(&initDone, 1)
    return nil
}
```

**支付业务提醒：** 支付网关、数据库连接初始化失败必须支持重试，不能一次失败就永远废了！

***

#### 坑 3：Once 不能重置

Once 设计成只能执行一次，没有 Reset() 方法。如果需要重置，只能自己造轮子。

***

### 4. 典型场景：单例模式

```go
var (
    instance *Config
    once     sync.Once
)

func GetConfig() *Config {
    once.Do(func() {
        // 只执行一次，并发安全
        instance = loadConfigFromFile()
    })
    return instance
}
```

支付系统的配置、数据库连接池、RPC 客户端，都应该这么写。

***

## 三、Pool：对象池，减少 GC 压力

### 1. 是什么？

sync.Pool 是对象复用池，把用过的对象存起来，下次需要的时候直接拿，不用新建，减少 GC 压力。

```go
type Pool struct {
    local     unsafe.Pointer  // 每个 P 有自己的本地池
    New       func() interface{}  // 池里没对象时调用的创建函数
}
```

两个方法：

- `Get()`：从池里拿一个对象
- `Put(x)`：把对象放回池里

***

### 2. 核心特性（面试必问）

| 特性        | 说明                                              |
| --------- | ----------------------------------------------- |
| **无锁设计**  | 每个 P 有自己的本地池，99% 的情况不用跨 P 抢锁                    |
| **自动清理**  | 每次 GC 把当前池移到 victim 缓存，下一次 GC 才真正清掉（两轮 GC 存活时间） |
| **大小无限**  | 没有容量限制，放多少都可以                                   |
| **不保证取回** | Put 进去的对象，下次 Get 不一定能拿回来                        |

**最容易理解错的一点（GC 清理机制）：**

> ❌ 错误认知 1：Pool 是永久缓存，放进去就一直在
> ❌ 错误认知 2：每次 GC 直接全部清空
> ✅ 正确认知：**两级缓存 + 两轮 GC 清理机制！**

**Pool 清理的实际过程：**

1. **第一次 GC：** 把 `local` 移到 `victim`（相当于冷备份），清空 `local`
2. **第二次 GC：** 清空 `victim`，真正彻底释放

**效果：**

- 放入 Pool 的对象 **至少能活过一轮 GC**
- 极端情况下可以活过两轮 GC
- 这样 GC 前后短暂的对象复用率大大提高

**为什么这么设计？**

- 如果 Pool 永久保存对象，那内存泄漏就太容易了
- 两轮清理比一轮清空更平滑，避免 GC 后瞬间大量分配
- victim 缓存是 Go 1.15 引入的重要性能优化

***

### 3. 底层原理：本地池 + 窃取

sync.Pool 的设计和 Go 调度器的 M-P-G 模型深度绑定：

**Go 1.15+ 实际的两级缓存结构：**

```
                    ┌──────────────────────────────────────────┐
                    │               sync.Pool                  │
                    └──────────────────────────────────────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
  ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
  │    local     │      │    local     │      │    local     │ ← 当前 GC 周期
  │  P0 本地池   │      │  P1 本地池   │      │  P2 本地池   │   活跃缓存
  └──────────────┘      └──────────────┘      └──────────────┘
          │                      │                      │
          └──────────────────────┴──────────────────────┘
                                 │
  ┌───────────────────────────────────────────────────────────┐
  │                        victim                              │ ← 上一轮 GC
  │                      受害者缓存                           │   冷备份
  └───────────────────────────────────────────────────────────┘
```

**每个 P 的本地池内部还有两层：**

1. **private**：当前 P 专用，Get/Put 完全无锁
2. **shared poolChain**：共享链表，本 P 从头部拿，其他 P 从尾部"窃取"

**GC 清理的实际流程：**

1. GC STW 阶段：把 `local` 移到 `victim`，清空 `local`
2. Get 的时候先从 `local` 找，找不到去 `victim` 找，最后才 `New()`
3. 下一轮 GC：清空 `victim`

**性能为什么高？**

- 99% 的情况操作本地池的 private，完全无锁
- victim 缓存大大提高了 GC 前后的对象复用率
- 本地池空了才去其他 P 的共享池偷，锁竞争概率极低

***

### 4. 四个常见坑

#### 坑 1：不要用来存数据库连接、TCP 连接

```go
// ❌ 错误：GC 把连接清掉了，下次 Get 拿到已经关闭的连接
var connPool = sync.Pool{
    New: func() interface{} {
        return db.Open()
    },
}
```

**原因：** GC 会清空 Pool，但是不会调用 Close()，连接就泄漏了！

**正确场景：** 只存**无状态、纯内存**的对象，比如：

- bytes.Buffer
- struct 临时对象
- JSON 序列化缓冲区

***

#### 坑 2：Get 出来的对象有旧数据，忘记重置

```go
var bufPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func bad() {
    buf := bufPool.Get().(*bytes.Buffer)
    // ❌ 忘记 Reset！buf 里有上次用的数据！
    buf.WriteString("hello")
    // ...
    bufPool.Put(buf)
}

func good() {
    buf := bufPool.Get().(*bytes.Buffer)
    buf.Reset()  // ✅ 一定要先重置！
    buf.WriteString("hello")
    // ...
    bufPool.Put(buf)
}
```

**支付业务血的教训：** 用 Pool 存请求结构体，忘记 Reset 导致上次的金额、订单号带到了下一次请求，造成资金故障！

***

#### 坑 3：存大对象导致内存泄漏

```go
// ❌ 不好：每个 buffer 1MB，池里存 1000 个就是 1GB
var bufPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 1024*1024)
    },
}
```

GC 不会立刻清，高并发下池里的对象会越来越多，造成内存飙升。

**解决：** 加大小限制，或者用多级 Pool（小对象一个池，大对象一个池）。

***

#### 坑 4：太小的对象没必要用 Pool

```go
// ❌ 没必要：一个 int 才 8 字节，分配成本极低，Pool 的开销都比这个大
type Point struct { X, Y int }
var pointPool = sync.Pool{...}
```

**经验法则：** 对象大于 1KB，或者分配开销大（比如 map、slice 预分配），才值得用 Pool。

***

### 5. 支付系统典型场景：JSON 序列化缓冲区

```go
var jsonBufPool = sync.Pool{
    New: func() interface{} {
        return bytes.NewBuffer(make([]byte, 0, 4096))  // 预分配 4KB
    },
}

func MarshalJSON(v interface{}) ([]byte, error) {
    buf := jsonBufPool.Get().(*bytes.Buffer)
    buf.Reset()  // ✅ 一定要重置！

    defer jsonBufPool.Put(buf)

    enc := json.NewEncoder(buf)
    if err := enc.Encode(v); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}
```

**效果：** JSON 序列化的 GC 压力降低 90% 以上，支付系统 QPS 提升明显！

***

## 四、Atomic：原子操作，无锁编程

### 1. 是什么？

atomic 包提供了**底层的原子内存操作**，不需要 Mutex，直接用 CPU 指令完成，性能极高。

支持的类型：

**旧版（Go 1.18 及之前）：**

- int32 / uint32
- int64 / uint64
- uintptr
- unsafe.Pointer

**Go 1.19+ 新增类型化原子操作（推荐使用）：**

- `atomic.Bool` - 原子布尔值
- `atomic.Int32` / `atomic.Uint32`
- `atomic.Int64` / `atomic.Uint64`
- `atomic.Uintptr`
- `atomic.Pointer[T]` - 泛型原子指针（Go 1.18+ 泛型支持）

类型化原子操作更安全，不需要手动处理指针和类型转换，是现在的推荐写法！

常见操作：

- Load：原子读
- Store：原子写
- Add：原子加
- Swap：原子交换
- CompareAndSwap（CAS）：比较并交换

***

### 2. 为什么比 Mutex 快？

```go
// Mutex 加锁方式：有上下文切换、排队、唤醒
var mu sync.Mutex
var count int64

func incr() {
    mu.Lock()
    count++
    mu.Unlock()
}

// Atomic 方式：一条 CPU 指令，无锁
var count int64

func incr() {
    atomic.AddInt64(&count, 1)
}
```

**性能对比（大概）：**

- atomic.Add：\~1ns
- Mutex.Lock+Unlock：\~20-30ns
- 差了 **20-30 倍**！

**但注意：** atomic 只能保护单个变量，复杂场景还是要用 Mutex。

***

### 3. 经典应用场景

#### 场景 1：计数器（并发安全）

```go
var requestCount int64

// 每个请求进来 +1
func HandleRequest() {
    atomic.AddInt64(&requestCount, 1)
    // ...
}

// 定时打日志
func LogStats() {
    count := atomic.LoadInt64(&requestCount)
    log.Printf("总请求数: %d", count)
}
```

比用 Mutex 简单太多了！

***

#### 场景 2：自旋锁（非常短的临界区）

```go
type SpinLock int32

func (l *SpinLock) Lock() {
    // CAS 尝试拿锁，拿不到就自旋
    for !atomic.CompareAndSwapInt32((*int32)(l), 0, 1) {
        runtime.Gosched()  // 让出 CPU
    }
}

func (l *SpinLock) Unlock() {
    atomic.StoreInt32((*int32)(l), 0)
}
```

**适用场景：** 临界区只有几条指令，拿不到锁等一小会就拿到了，比 Mutex 性能高。

***

#### 场景 3：Once 的双重检查（前面讲过）

```go
if atomic.LoadUint32(&o.done) == 0 {
    // 拿锁
    if o.done == 0 {
        defer atomic.StoreUint32(&o.done, 1)
        f()
    }
}
```

***

### 4. 三个常见坑

#### 坑 1：64 位对齐问题（32 位系统）

```go
// ❌ 32 位系统上有问题：int64 可能没有 8 字节对齐
type Bad struct {
    x int32
    y int64  // 地址可能是 4 的倍数，但不是 8 的倍数
}

func (b *Bad) IncrY() {
    atomic.AddInt64(&b.y, 1)  // ❌ 32 位系统 panic！
}
```

**原因：** x86 32 位系统上，64 位原子操作要求地址必须 8 字节对齐，否则会 panic。

**解决：** 把 64 位字段放在结构体开头，或者手动对齐。

**面试高频追问：** 为什么 WaitGroup 用 `[3]uint32` 而不是直接用 uint64？

- 就是为了解决 32 位系统的对齐问题！
- 三个 uint32 总有一个位置是 8 字节对齐的

***

#### 坑 2：不要用 atomic 操作结构体里的字段

```go
type User struct {
    ID    int64
    Name  string
    Count int64
}

func (u *User) Incr() {
    atomic.AddInt64(&u.Count, 1)  // ✅ 可以，但要小心
}
```

**问题：** 如果 User 被拷贝了，Count 的地址就变了，atomic 操作的是旧地址。

**原则：** atomic 操作的变量最好是全局的，或者指针引用的，不要随便拷贝。

***

#### 坑 3：ABA 问题

```go
// CAS 只比较值，不比较版本
old := atomic.LoadInt64(&val)
// 这期间 val 从 A 变成 B，又从 B 变回 A
atomic.CompareAndSwapInt64(&val, old, new)  // ✅ 成功，但中间有人改过！
```

**解决：** 用版本号，比如 `[32 位版本号][32 位值]`，每次改值版本号 +1，ABA 就检测出来了。

大部分业务场景 ABA 不是问题，但写无锁数据结构的时候一定要注意。

***

## 五、面试高频问答汇总

| 问题                           | 答案                                            |
| ---------------------------- | --------------------------------------------- |
| WaitGroup Add() 应该放在哪里？      | 启动 goroutine **之前**，放在里面可能 Wait 提前返回。         |
| WaitGroup 可以传值吗？             | 绝对不行，是值类型，拷贝就失效了，必须传指针。                       |
| WaitGroup 计数器为负会怎么样？         | 直接 panic，这是设计故意的，说明逻辑有 bug。                   |
| Once 初始化失败怎么办？               | Once 只执行一次，失败了也不会重试，需要自己包装支持重试。               |
| Once Do() 里面递归调用 Do() 会怎么样？  | 死锁，Mutex 不是可重入的。                              |
| sync.Pool 里的对象什么时候被清掉？       | **每次 GC 全部清掉！** Pool 只是临时复用，不是永久缓存。           |
| sync.Pool 可以存数据库连接吗？         | 绝对不行！GC 清掉 Pool 不会调用 Close，连接泄漏了。只能存无状态纯内存对象。 |
| sync.Pool Get 出来的对象需要注意什么？   | **一定要 Reset！** 不然会有上次的旧数据，可能造成严重故障。           |
| atomic 比 Mutex 快多少？          | 大概 20-30 倍，atomic 是一条 CPU 指令，Mutex 有上下文切换开销。  |
| atomic 可以保护多个变量吗？            | 不行，只能保护单个变量，复杂场景还是用 Mutex。                    |
| 32 位系统 atomic 操作 64 位需要注意什么？ | 必须 8 字节对齐，不然会 panic。                          |

***

## 六、并发原语选型决策树

```
需要同步吗？
├── 不需要 → 直接写
└── 需要
    ├── 等一组 goroutine 完成 → WaitGroup
    ├── 只执行一次 → Once
    ├── 对象频繁分配 GC 压力大 → Pool
    ├── 只保护单个变量、性能要求极高 → Atomic
    ├── 读读很多、写写很少 → RWMutex
    └── 其他情况 → Mutex
```

**支付业务黄金法则：**

> 能不用锁就不用锁，能用原子操作就不用 Mutex，能用 RWMutex 就不用 Mutex，锁粒度越小越好。

***

## 七、一句话总结

> WaitGroup 等批量任务，Once 保证只执行一次，Pool 复用对象降 GC，Atomic 无锁性能高；记住四个原语的坑：WaitGroup 先加后启、Once 不能重入、Pool 一定要 Reset、Atomic 注意对齐。

