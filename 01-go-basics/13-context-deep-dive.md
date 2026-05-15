# Go Context 深度解析与最佳实践

> 面试 100% 必问！Go 语言最重要的并发控制原语之一。
> 基于 Go 1.22 最新版本讲解。

---

## 一、Context 是什么？解决什么问题？

### 1.1 问题背景

想象一个场景：HTTP 请求来了，启动了 N 个 goroutine 去查数据库、调 RPC、做计算。这时候用户取消了请求（或者超时了），怎么通知所有 goroutine 赶紧退出，释放资源？

```go
func handleRequest(req *http.Request) {
    go func() { // 查数据库
        // 怎么知道请求已经取消了？
    }()

    go func() { // 调 RPC
        // 怎么知道该停止了？
    }()

    go func() { // 做计算
        // ...
    }()
}
```

**没有 Context 之前的解决方案**（都很烂）：
- 全局变量？每个请求不一样，不行
- 每个 goroutine 传个 channel？N 个 goroutine 要传 N 个，累死
- 超时用 time.After？每个地方都要写，而且没法级联取消

**Context 的出现就是为了解决这些问题**：
1. ✅ **级联取消**：父 goroutine 取消，所有子 goroutine 都能收到信号
2. ✅ **超时控制**：自动超时，不需要每个地方写 time.After
3. ✅ **请求级别的传值**：同一个请求的所有 goroutine 共享数据（TraceID、用户信息等）
4. ✅ **线程安全**：多个 goroutine 同时用没问题

---

### 1.2 Context 核心定义

Context 是一个接口，只有 4 个方法：

```go
type Context interface {
    // 返回 Context 被取消的时间（超时时间）
    Deadline() (deadline time.Time, ok bool)

    // 返回一个 channel，Context 被取消时会 close 这个 channel
    Done() <-chan struct{}

    // 返回 Context 被取消的原因
    Err() error

    // 从 Context 中取值（类似 map）
    Value(key any) any
}
```

**核心就是 `Done()` 方法**：返回一个只读 channel，Context 被取消时这个 channel 会被 close。

所有 goroutine 只要监听这个 channel 就行：

```go
func worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():  // 监听取消信号
            fmt.Println("收到取消信号，退出！")
            return
        default:
            // 正常工作
        }
    }
}
```

---

## 二、四种 Context 实现类型（面试必问）

Go 标准库提供了四种 Context 实现，对应四种创建方式：

| 类型 | 创建函数 | 用途 |
|------|---------|------|
| **emptyCtx** | `context.Background()` / `context.TODO()` | 根 Context，永远不会取消 |
| **cancelCtx** | `context.WithCancel(parent)` | 手动取消 |
| **timerCtx** | `context.WithTimeout(parent, timeout)` / `context.WithDeadline(parent, time)` | 超时自动取消 |
| **valueCtx** | `context.WithValue(parent, key, value)` | 传值 |

---

### 2.1 emptyCtx - 空 Context（根节点）

```go
// 这是 emptyCtx 的定义，就是一个 int 别名
type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time, ok bool) { return }
func (*emptyCtx) Done() <-chan struct{} { return nil }      // 返回 nil，永远不会 close
func (*emptyCtx) Err() error { return nil }
func (*emptyCtx) Value(key any) any { return nil }

// 两个常用的根 Context
var (
    background = new(emptyCtx)
    todo       = new(emptyCtx)
)

func Background() Context { return background }
func TODO() Context { return todo }
```

**区别**：
- `Background()`：正常情况用，程序入口、main 函数、测试用
- `TODO()`：不知道用什么的时候先用这个，后面再改

**注意**：这两个是同一个类型的不同实例，功能完全一样，只是语义上的区别。

---

### 2.2 cancelCtx - 可取消的 Context（核心！）

这是最常用的，也是级联取消的核心。

```go
type cancelCtx struct {
    Context                // 嵌入父 Context
    mu       sync.Mutex    // 保护下面字段的锁
    done     chan struct{} // 懒加载，第一次 cancel 时才创建
    children map[canceler]struct{}  // 所有子 Context
    err      error         // 取消原因，cancel 时设置
}
```

**创建 cancelCtx**：

```go
// 创建一个可取消的 Context，返回 Context 和 取消函数
ctx, cancel := context.WithCancel(context.Background())
defer cancel()  // 一定要 defer cancel！否则资源泄漏
```

**级联取消的原理**：

```go
// 父 Context
parent, cancelParent := context.WithCancel(context.Background())

// 两个子 Context（都基于父 Context）
child1, _ := context.WithCancel(parent)
child2, _ := context.WithCancel(parent)

// 父 Context 取消！
cancelParent()

// child1.Done() 和 child2.Done() 都会被 close！
// 所有监听这两个 child 的 goroutine 都会收到取消信号
```

**为什么能级联取消？**
- 每个 cancelCtx 都保存了自己的 children
- 父 Context 被 cancel 时，会遍历所有 children，一个个 cancel
- 每个 child 再 cancel 自己的 children
- 像树一样，整棵树都被取消

**⚠️ 非常重要：WithCancel 返回的 cancel 函数一定要调用！**
- 不调用的话，cancelCtx 会一直留在父 Context 的 children 里
- 永远不会被 GC，造成内存泄漏
- 最佳实践：`defer cancel()` 紧跟在 WithCancel 后面

---

### 2.3 timerCtx - 超时自动取消的 Context

```go
type timerCtx struct {
    cancelCtx              // 嵌入 cancelCtx，继承所有功能
    timer     *time.Timer  // 超时定时器
    deadline  time.Time    // 截止时间
}
```

**两种创建方式**：

```go
// 方式 1：相对时间，3 秒后超时
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

// 方式 2：绝对时间，2024-12-31 23:59:59 超时
deadline := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
ctx, cancel := context.WithDeadline(context.Background(), deadline)
defer cancel()
```

**超时原理**：
- 创建时启动一个 timer
- timer 到期时自动调用 cancel
- 所以超时和手动调用 cancel 本质是一样的

**Go 1.22 优化**：timerCtx 的 timer 不再需要一直持有引用，GC 更高效。

---

### 2.4 valueCtx - 传值用的 Context

```go
type valueCtx struct {
    Context         // 嵌入父 Context
    key, val any    // 键值对
}
```

**传值**：

```go
// 父 Context
parent := context.Background()

// 包一层，加个 key-value
ctx1 := context.WithValue(parent, "trace_id", "abc123")

// 再包一层，再加个 key-value
ctx2 := context.WithValue(ctx1, "user_id", int64(12345))

// 取值时会递归向上找
fmt.Println(ctx2.Value("trace_id"))  // "abc123"
fmt.Println(ctx2.Value("user_id"))   // 12345
```

**⚠️ valueCtx 的三个大坑（面试 100% 问）**：

#### 坑 1：key 不能是 string 等内置类型！

```go
// ❌ 错误写法：用 string 当 key，容易冲突
ctx := context.WithValue(parent, "trace_id", "abc")

// 两个不同的包都用 "trace_id" 当 key，就会互相覆盖！
```

**正确写法**：自定义 key 类型：

```go
// ✅ 正确：用自定义类型当 key，不会冲突
type contextKey string
const TraceIDKey contextKey = "trace_id"

ctx := context.WithValue(parent, TraceIDKey, "abc123")
value := ctx.Value(TraceIDKey)  // 安全，不会冲突
```

#### 坑 2：valueCtx 是不可变的，每次 WithValue 都是新包装一层

```go
ctx := context.Background()

ctx1 := context.WithValue(ctx, "a", 1)
ctx2 := context.WithValue(ctx1, "b", 2)  // ctx2 包 ctx1

fmt.Println(ctx2.Value("a"))  // 1，递归向上找到的
fmt.Println(ctx1.Value("b"))  // nil！ctx1 没有 b，也不会向下找
```

Context 是**单向链表**，只能向上找父节点的值，不能向下。

#### 坑 3：不要用 Context 传业务参数！

```go
// ❌ 错误：把 Request 整个塞进去，变成了万恶的全局变量
ctx := context.WithValue(parent, "request", req)

// ✅ 正确：只传请求级别的元数据，而且是只读的
//  TraceID、UserID、TenantID、RequestIP 这类
```

**Go 官方建议**：Context Value 只应该传**请求作用域**的、**只读**的元数据，不要传业务参数！

---

## 三、Context 底层结构（图解）

Context 不是一棵树，而是**多个单向链表**，每个子节点都指向父节点：

```
Background() (emptyCtx)
    │
    ├─ WithCancel() → cancelCtx[1]
    │                   ├─ WithTimeout() → timerCtx[2]
    │                   └─ WithValue() → valueCtx[3]
    │                                      └─ WithCancel() → cancelCtx[4]
    └─ WithValue("trace_id") → valueCtx[5]
```

**取消时的流程**：
1. 调用 cancelCtx[1].cancel()
2. cancelCtx[1] 遍历自己的 children：timerCtx[2] 和 valueCtx[3]
3. 调用 timerCtx[2].cancel() → 停止 timer，close done channel
4. valueCtx[3] 本身不能取消，继续找它的 children：cancelCtx[4]
5. 调用 cancelCtx[4].cancel()
6. 所有 Done() channel 都被 close，所有 goroutine 收到信号

---

## 四、Go 1.21+ Context 新功能

### 4.1 WithoutCancel - 创建一个不可取消的 Context

Go 1.21 新增：

```go
// 父 Context 3 秒后超时
parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

// 创建一个新的 Context，继承 parent 的所有值，但不会被取消
ctx := context.WithoutCancel(parent)

// 3 秒后 parent 被取消了，但 ctx 还是正常的
// ctx.Done() 永远不会返回
```

**用途**：需要脱离请求生命周期继续执行的操作，比如异步写日志、上报指标。

### 4.2 AfterFunc - Context 取消时执行回调

Go 1.21 新增：

```go
ctx, cancel := context.WithCancel(context.Background())

// Context 取消时，自动执行这个函数
stop := context.AfterFunc(ctx, func() {
    fmt.Println("Context 被取消了！执行清理工作")
})

// stop() 可以取消这个回调注册
defer stop()
```

**用途**：Context 取消时做一些清理工作，比监听 `Done()` channel 更方便。

---

## 五、真实业务场景示例

### 场景 1：HTTP 超时控制

```go
func handleUser(w http.ResponseWriter, r *http.Request) {
    // 整个请求 3 秒超时
    ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
    defer cancel()

    // 把带超时的 ctx 传给下游
    user, err := userService.GetUser(ctx, 123)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    json.NewEncoder(w).Encode(user)
}

// UserService 里的数据库查询
func (s *UserService) GetUser(ctx context.Context, id int64) (*User, error) {
    // 数据库查询会自动超时
    rows, err := s.db.QueryContext(ctx, "SELECT * FROM users WHERE id = ?", id)
    if err != nil {
        return nil, err
    }
    // ...
}
```

### 场景 2：级联取消

```go
func main() {
    // 父 Context：5 秒超时
    parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // 启动 3 个 worker，都用同一个 parent ctx
    for i := 0; i < 3; i++ {
        go worker(parent, i)
    }

    // 也可以手动提前取消
    // time.Sleep(2 * time.Second)
    // cancel()

    time.Sleep(6 * time.Second)
    fmt.Println("main 退出")
}

func worker(ctx context.Context, id int) {
    for {
        select {
        case <-ctx.Done():
            fmt.Printf("worker %d 收到取消信号: %v\n", id, ctx.Err())
            return
        default:
            fmt.Printf("worker %d 工作中...\n", id)
            time.Sleep(1 * time.Second)
        }
    }
}
```

**输出**：
```
worker 0 工作中...
worker 1 工作中...
worker 2 工作中...
worker 0 工作中...
worker 1 工作中...
worker 2 工作中...
worker 0 收到取消信号: context deadline exceeded
worker 1 收到取消信号: context deadline exceeded
worker 2 收到取消信号: context deadline exceeded
main 退出
```

### 场景 3：TraceID 全链路传递

```go
// 定义 TraceID 的 key（自定义类型，避免冲突）
type traceKey string
const TraceIDKey traceKey = "trace_id"

// HTTP 中间件：生成 TraceID 并放入 Context
func TraceMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        traceID := generateTraceID()  // 生成唯一 ID

        // 把 TraceID 放入 Context
        ctx := context.WithValue(r.Context(), TraceIDKey, traceID)

        // 用新的 ctx 继续处理请求
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// 业务代码里到处都能用
func handleOrder(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // 打日志自动带 TraceID
    log.Printf("[%s] 开始处理订单", ctx.Value(TraceIDKey))

    // 调 RPC 时把 ctx 传过去，下游也能拿到 TraceID
    order, err := orderService.CreateOrder(ctx, req)

    log.Printf("[%s] 订单处理完成", ctx.Value(TraceIDKey))
}
```

---

## 六、Context 常见坑（面试高频）

### ❌ 坑 1：把 Context 存在结构体里

```go
// ❌ 错误！不要这么写
type MyService struct {
    ctx context.Context  // 把 Context 存起来当字段用
    db  *gorm.DB
}

func (s *MyService) CreateOrder(req *OrderReq) error {
    return s.db.WithContext(s.ctx).Create(req).Error
}
```

**为什么不对**：
- Context 是**请求级别的**，每个请求应该有自己的 Context
- 结构体是全局的，这样所有请求共享一个 Context，A 请求取消了 B 请求也挂了

**正确写法**：每个方法第一个参数传 ctx：

```go
// ✅ 正确
func (s *MyService) CreateOrder(ctx context.Context, req *OrderReq) error {
    return s.db.WithContext(ctx).Create(req).Error
}
```

> **Go 官方规范**：不要把 Context 存在结构体里，应该作为每个函数的第一个参数传入！

### ❌ 坑 2：忘了调用 cancel 函数

```go
// ❌ 错误：只拿 ctx，不调用 cancel
ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)

// ✅ 正确：一定 defer cancel
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()  // 紧跟在 WithTimeout 后面
```

不调用 cancel 会导致 cancelCtx 一直留在父节点的 children map 里，永远不会被 GC，内存泄漏。

### ❌ 坑 3：传 nil Context

```go
// ❌ 错误：不要传 nil
user, err := userService.GetUser(nil, 123)

// ✅ 正确：不知道传什么就用 TODO()
user, err := userService.GetUser(context.TODO(), 123)
```

很多标准库函数遇到 nil Context 会直接 panic！

### ❌ 坑 4：用 Context 传可变对象

```go
// ❌ 错误：传指针，别人可以修改
user := &User{ID: 123}
ctx := context.WithValue(parent, UserKey, user)

// 某个 goroutine 偷偷改了
user.Name = "hacker"

// ✅ 正确：传值类型，或者确保是不可变的
ctx := context.WithValue(parent, UserKey, *user)
```

Context Value 应该是只读的，多个 goroutine 同时修改会有并发问题。

### ❌ 坑 5：Context 传太多东西，变成垃圾桶

```go
// ❌ 错误：什么都往 Context 里塞
ctx = context.WithValue(ctx, "request", req)
ctx = context.WithValue(ctx, "db", db)
ctx = context.WithValue(ctx, "redis", redis)
ctx = context.WithValue(ctx, "config", config)
// Context 变成了万恶的全局变量！
```

**Context 应该只传请求级别的元数据**：TraceID、UserID、TenantID、RequestIP。
**业务参数应该显式传参**！

---

## 七、Context 最佳实践总结

### ✅ 最佳实践 1：Context 作为函数第一个参数

```go
// Go 社区约定俗成：第一个参数叫 ctx，类型是 context.Context
func DoSomething(ctx context.Context, arg1 int, arg2 string) error {
    // ...
}
```

### ✅ 最佳实践 2：不要存 Context 到结构体

除非这个结构体的生命周期和 Context 完全一致（比如一个只服务于本次请求的临时对象）。

### ✅ 最佳实践 3：WithCancel/WithTimeout 后面紧跟 defer cancel

```go
ctx, cancel := context.WithTimeout(parent, 3*time.Second)
defer cancel()  // 下一行就写 defer
```

### ✅ 最佳实践 4：Value 用自定义类型当 key

```go
type myKey string
const UserIDKey myKey = "user_id"
```

### ✅ 最佳实践 5：不要传 nil Context，不知道就用 TODO()

```go
DoSomething(context.TODO(), 123, "abc")
```

### ✅ 最佳实践 6：Context Value 只传只读元数据

- ✅ TraceID、UserID、TenantID、RequestIP
- ❌ 业务参数、数据库连接、配置对象

---

## 八、面试标准答案模板

**面试官问**：讲讲 Go 的 Context？有什么用？有哪些坑？

**标准回答**：

> Context 是 Go 语言用来做并发控制的核心原语，主要解决三个问题：级联取消、超时控制、请求级传值。
>
> **Context 有四种实现类型**：
> 1. `emptyCtx`：根节点，Background() 和 TODO() 就是这个，永远不会取消
> 2. `cancelCtx`：可手动取消的，WithCancel 创建，级联取消的核心
> 3. `timerCtx`：超时自动取消的，WithTimeout 和 WithDeadline 创建
> 4. `valueCtx`：用来传值的，WithValue 创建
>
> **底层原理**：Context 本质是单向链表，每个子节点指向父节点。取消时父节点会遍历所有 children，一个个取消，实现级联取消。
>
> **常见的坑**：
> 1. **不要把 Context 存在结构体里**，应该作为每个函数的第一个参数传入
> 2. **WithCancel 返回的 cancel 函数一定要调用**，否则内存泄漏
> 3. **WithValue 的 key 不要用 string 等内置类型**，要用自定义类型避免冲突
> 4. **不要用 Context 传业务参数**，只应该传请求级别的只读元数据
> 5. **不要传 nil Context**，不知道传什么就用 context.TODO()
>
> **Go 1.21 之后还新增了**：`WithoutCancel()` 创建不可取消的 Context，`AfterFunc()` 取消时执行回调。

---

## 九、Context 相关面试题汇总

### Q1：Context 是线程安全的吗？为什么？

**答**：是线程安全的。因为：
- Context 是不可变的（immutable），WithCancel/WithValue 都是创建新的 Context，不会修改原有的
- cancelCtx 内部有 `sync.Mutex` 保护字段修改
- 所以多个 goroutine 同时用同一个 Context 没问题

### Q2：Context 的 Done() channel 是谁 close 的？什么时候 close？

**答**：cancel 函数调用时 close。不管是手动调用 cancel，还是 timer 超时自动调用 cancel，最终都会走到同一个 cancel 函数里，在里面 close done channel。

### Q3：父 Context 取消了，子 Context 还能用吗？

**答**：子 Context 也会被取消（级联取消）。但是子 Context 里的值还是可以正常读的，只是 Done() channel 被 close 了。

### Q4：Context 怎么实现的级联取消？

**答**：每个 cancelCtx 都维护了一个 children map，存了所有基于它创建的子 Context。父节点被 cancel 时，会遍历所有 children，一个个调用它们的 cancel 方法，递归下去整棵树都被取消。

### Q5：为什么 WithValue 的 key 不能用 string？应该用什么？

**答**：用 string 当 key 容易冲突，不同的包可能用了相同的字符串 key，互相覆盖。应该用自定义的类型（比如 `type myKey string`）当 key，这样不同包的 key 类型不一样，不会冲突。

---

## 十、推荐阅读

- Go 官方博客：https://go.dev/blog/context
- Context 源码：`src/context/context.go`（只有 500 行，非常推荐读）

---

**文档位置**：`01-go-basics/13-context-deep-dive.md`
