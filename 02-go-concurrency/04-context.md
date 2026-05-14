# Context 底层原理与常见坑

---

## 一、先搞懂：Context 到底是什么？

Context 是 Go 语言专门为 goroutine 级别的控制设计的机制，它可以实现：
1. **取消信号传递**：父 goroutine 可以通知所有子 goroutine 退出
2. **超时控制**：设置一个截止时间，到时间自动取消所有子 goroutine
3. **值传递**：在整个调用链上传递共享数据（比如请求 ID、用户信息）

这是 Go 并发编程的核心，也是面试 100% 会考的内容。

---

## 二、Context 接口定义

```go
type Context interface {
    // 返回截止时间，ok=false 表示没有设置截止时间
    Deadline() (deadline time.Time, ok bool)

    // 返回一个 channel，context 取消时这个 channel 会关闭
    Done() <-chan struct{}

    // 返回 context 取消的原因
    Err() error

    // 根据 key 获取存储的值
    Value(key interface{}) interface{}
}
```

只有 4 个方法，非常简洁，但功能非常强大。

---

## 三、四种 Context 类型（面试必问）

Go 官方提供了 4 种 Context 实现，对应 4 个创建函数：

| 类型 | 创建函数 | 用途 |
|------|---------|------|
| emptyCtx | Background() / TODO() | 根节点，空 context，永远不会取消 |
| cancelCtx | WithCancel() | 手动取消，可以级联取消所有子 context |
| timerCtx | WithDeadline() / WithTimeout() | 超时自动取消 |
| valueCtx | WithValue() | 传递值 |

---

## 四、底层原理深度解析

### 1. emptyCtx：根节点

```go
type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time, ok bool) { return }
func (*emptyCtx) Done() <-chan struct{} { return nil }
func (*emptyCtx) Err() error { return nil }
func (*emptyCtx) Value(key interface{}) interface{} { return nil }
```

emptyCtx 什么都不做，永远不会取消，也不存任何值。它的作用只是作为整个 Context 树的根节点。

```go
var (
    background = new(emptyCtx)
    todo       = new(emptyCtx)
)

func Background() Context { return background }
func TODO() Context { return todo }
```

---

### 2. cancelCtx：可取消的 Context（核心）

这是最重要的 Context 类型，也是面试考察的重点。

```go
type cancelCtx struct {
    Context  // 父 context，匿名字段

    mu       sync.Mutex            // 保护下面的字段
    done     chan struct{}         // 懒加载，第一次 cancel 时关闭
    children map[canceler]struct{} // 所有可以取消的子 context
    err      error                 // 取消时设置错误原因
}
```

**核心设计：**
- `done` channel：懒加载，第一次需要的时候才创建
- `children` map：记录所有可以取消的子 context，父 context 取消时，遍历这个 map 把所有子 context 都取消
- `mu` 互斥锁：保护并发安全

#### cancel() 方法：级联取消的核心

```go
func (c *cancelCtx) cancel(removeFromParent bool, err error) {
    c.mu.Lock()

    // 1. 设置错误原因
    c.err = err

    // 2. 关闭 done channel，通知所有等待的 goroutine
    close(c.done)

    // 3. 遍历所有子 context，逐个取消
    for child := range c.children {
        child.cancel(false, err)
    }
    c.children = nil

    c.mu.Unlock()

    // 4. 从父 context 的 children map 中把自己删掉
    if removeFromParent {
        removeChild(c.Context, c)
    }
}
```

**这就是级联取消的原理！**
- 父 context 取消的时候，会遍历所有子 context，逐个调用 cancel()
- 子 context 取消的时候，又会遍历它自己的子 context
- 这样整个树状结构的所有 goroutine 都会被通知到

#### Done() 方法：懒加载 channel

```go
func (c *cancelCtx) Done() <-chan struct{} {
    c.mu.Lock()
    if c.done == nil {
        c.done = make(chan struct{})  // 第一次调用的时候才创建
    }
    d := c.done
    c.mu.Unlock()
    return d
}
```

done channel 是懒加载的，如果从来没有人调用 Done()，这个 channel 永远不会创建，节省内存。

---

### 3. timerCtx：带超时的 Context

timerCtx 是在 cancelCtx 的基础上加了一个定时器。

```go
type timerCtx struct {
    cancelCtx  // 继承 cancelCtx 的所有能力
    timer *time.Timer  // 定时器，到时间自动 cancel
    deadline time.Time // 截止时间
}
```

**核心逻辑：**
1. 创建的时候启动一个定时器
2. 定时器到时间自动调用 cancel()
3. 如果用户手动提前 cancel，定时器会被关闭，避免资源泄漏

WithTimeout() 就是调用 WithDeadline() 实现的：
```go
func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc) {
    return WithDeadline(parent, time.Now().Add(timeout))
}
```

---

### 4. valueCtx：传递值的 Context

```go
type valueCtx struct {
    Context  // 父 context

    key, val interface{}  // 只存一对 key-value！
}
```

**注意！valueCtx 只存一对 key-value！** 不是 map！

你每调用一次 WithValue()，就会创建一层新的 valueCtx，形成一个链表：

```go
func WithValue(parent Context, key, val interface{}) Context {
    if key == nil {
        panic("nil key")
    }
    return &valueCtx{parent, key, val}
}
```

查询的时候是沿着链表往上找，直到找到 key 或者到根节点：

```go
func (c *valueCtx) Value(key interface{}) interface{} {
    if c.key == key {
        return c.val
    }
    return c.Context.Value(key)  // 找不到就问爸爸要
}
```

**这是面试高频追问！很多人以为 valueCtx 里面是个 map，其实是链表！**

**所以 WithValue() 的时间复杂度是 O(n)，n 是链表的长度！**

---

## 五、90% 的人都踩过的 6 个坑

### 坑 1：不要把 Context 存在结构体里

```go
// ❌ 错误写法：把 Context 存结构体里，生命周期混乱
type Handler struct {
    ctx context.Context
}

func (h *Handler) Handle() {
    // 用 h.ctx
}

// ✅ 正确写法：Context 作为第一个参数，显式传递
func Handle(ctx context.Context) {
    // 用 ctx
}
```

Go 官方约定：**Context 永远作为函数的第一个参数显式传递，不要存在结构体里。**

---

### 坑 2：忘记调用 CancelFunc，goroutine 泄漏

```go
// ❌ 错误写法：只拿 ctx，不调用 cancel
ctx, _ := context.WithCancel(parent)
go func() {
    <-ctx.Done()
}()

// ctx 永远不会被 cancel，goroutine 永远阻塞，泄漏！
```

**✅ 正确写法：defer 调用 cancel**
```go
ctx, cancel := context.WithCancel(parent)
defer cancel()  // 函数退出时一定要 cancel！

go func() {
    <-ctx.Done()
}()
```

**支付业务特别提醒：** 只要创建了可取消的 Context，必须 defer 调用 cancel()，不然 goroutine 泄漏几天就能把内存跑满。

---

### 坑 3：WithValue 传太多东西，变成全局变量垃圾桶

```go
// ❌ 错误写法：什么都往 Context 里塞，变成全局变量
ctx = context.WithValue(ctx, "user", user)
ctx = context.WithValue(ctx, "db", db)
ctx = context.WithValue(ctx, "redis", redis)
ctx = context.WithValue(ctx, "config", config)
```

Context 是用来传递**请求作用域**的数据，不是用来传递函数参数的！

**✅ 最佳实践：**
- 只传请求级别的数据：请求 ID、trace ID、用户认证信息
- 不要传业务参数，不要传依赖注入的对象
- key 不要用 string，用自定义的私有类型，避免冲突

```go
// ✅ 正确：用私有类型作为 key，避免冲突
type contextKey string
const requestIDKey contextKey = "request_id"

// 存的时候
ctx = context.WithValue(ctx, requestIDKey, "123456")

// 取的时候
requestID := ctx.Value(requestIDKey).(string)
```

---

### 坑 4：Context 不是线程安全的？不，Value 是只读的！

很多人以为 Context 不是线程安全的，其实 Context 本身是**不可变的**！

你每次 WithCancel() / WithValue() 都是创建**新的** Context 节点，原来的父 Context 完全不变。

所以 Context 可以在多个 goroutine 之间安全共享，不需要加锁。

---

### 坑 5：Done() 返回的是 nil channel，select 永远阻塞

```go
ctx := context.Background()

select {
case <-ctx.Done():  // ctx.Done() 返回 nil！nil channel 永远阻塞！
    fmt.Println("永远不会走到这里")
}
```

emptyCtx 的 Done() 永远返回 nil，所以不要 select 一个永远不会取消的 Context。

---

### 坑 6：CancelFunc 可以调用多次，是幂等的

```go
ctx, cancel := context.WithCancel(parent)
cancel()  // 第一次取消
cancel()  // 第二次调用也不会 panic，安全的
cancel()  // 随便调用多少次都行
```

cancel() 是幂等的，多次调用没问题，所以放心 defer 就行。

---

## 六、经典使用场景

### 场景 1：HTTP 请求超时控制

```go
func handler(w http.ResponseWriter, r *http.Request) {
    // 3秒超时
    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    // 把带超时的 ctx 传给下游
    result, err := processRequest(ctx)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    fmt.Fprintf(w, result)
}
```

所有对外的 HTTP 接口都必须加超时！这是支付业务的铁律。

---

### 场景 2：级联取消

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // 启动 10 个 worker，都用同一个 ctx
    for i := 0; i < 10; i++ {
        go worker(ctx, i)
    }

    // 5秒后取消，所有 worker 都会收到信号退出
    time.Sleep(5 * time.Second)
    cancel()
    fmt.Println("所有 worker 都退出了")
}

func worker(ctx context.Context, id int) {
    for {
        select {
        case <-ctx.Done():
            fmt.Printf("worker %d 退出\n", id)
            return
        default:
            // 工作...
        }
    }
}
```

一个 cancel，10 个 goroutine 同时收到通知退出，非常优雅。

---

### 场景 3：传递请求 ID 做链路追踪

```go
func middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 生成请求 ID
        requestID := uuid.New().String()

        // 放到 Context 里，整个调用链都能拿到
        ctx := context.WithValue(r.Context(), requestIDKey, requestID)

        // 打日志带上请求 ID
        log.Printf("[%s] 请求开始", requestID)

        // 传给下游
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

支付系统的每一条日志都必须带请求 ID，不然出问题根本查不到日志。

---

## 七、面试高频问答

| 问题 | 答案 |
|------|------|
| Context 四个方法分别是什么？ | Deadline() 返回截止时间，Done() 返回取消信号 channel，Err() 返回取消原因，Value() 获取存储的值。 |
| 四种 Context 类型是什么？ | emptyCtx（根节点）、cancelCtx（手动取消）、timerCtx（超时取消）、valueCtx（传值）。 |
| cancelCtx 的级联取消原理是什么？ | 每个 cancelCtx 维护一个 children map，记录所有可取消的子 context。父 context 取消时遍历 children map，逐个调用子 context 的 cancel()，形成级联取消。 |
| valueCtx 里面是 map 吗？ | 不是！每个 valueCtx 只存一对 key-value，WithValue() 会创建新节点，形成链表。查询时沿着链表往上找，时间复杂度 O(n)。 |
| Done() channel 是什么时候创建的？ | 懒加载，第一次调用 Done() 的时候才创建。没人调用 Done() 的话永远不会创建，节省内存。 |
| Context 是线程安全的吗？ | 是。Context 是不可变的，每次 WithXxx() 都是创建新节点，原父 Context 不变，可以在多个 goroutine 之间安全共享。 |
| Context 应该存在结构体里还是作为参数传递？ | 官方约定永远作为第一个参数显式传递，不要存在结构体里。 |
| WithValue 的 key 为什么要用私有类型？ | 防止 key 冲突。如果都用 string，不同包可能用相同的 string 作为 key，值就被覆盖了。用私有类型可以保证唯一性。 |
| CancelFunc 可以多次调用吗？ | 可以，是幂等的，多次调用不会 panic。 |
| emptyCtx 的 Done() 返回什么？ | 返回 nil，nil channel 永远阻塞，所以 emptyCtx 永远不会取消。 |

---

## 八、一句话总结

> Context 是 Go 并发控制的灵魂，用树状结构实现级联取消，四种类型各司其职；记住六个坑：不传结构体、不忘 cancel、不乱传值、理解不可变、nil channel 不阻塞、cancel 幂等。
