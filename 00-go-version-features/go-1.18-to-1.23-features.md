# Go 1.18 - 1.23 各版本重要特性总结

---

## 目录

- [Go 1.18（2022.03）](#go-118202203) - 泛型、模糊测试、工作区
- [Go 1.19（2022.08）](#go-119202208) - 类型化原子操作、内存模型更新
- [Go 1.20（2023.02）](#go-120202302) - 切片转数组、PGO 预览、错误包装
- [Go 1.21（2023.08）](#go-121202308) - OnceFunc/OnceValue、slog、min/max、clear
- [Go 1.22（2024.02）](#go-122202402) - 循环变量修复、range over int、PGO GA
- [Go 1.23（2024.08）](#go-123202408) - 迭代器、unique 包、Timer GC 优化
- [面试高频问答](#面试高频问答)
- [各版本特性对比总表](#各版本特性对比总表)

---

## Go 1.18（2022.03）

**发布日期：2022年3月15日**

**三大核心特性：**

### 1. 泛型（Generics）⭐⭐⭐⭐⭐

这是 Go 语言历史上最大的一次语法变更。

**基础语法：**
```go
// 定义泛型函数
func Print[T any](s []T) {
    for _, v := range s {
        fmt.Println(v)
    }
}

// 类型约束
func Min[T Ordered](a, b T) T {
    if a < b {
        return a
    }
    return b
}

// 预定义约束：any、comparable、Ordered
```

**泛型类型：**
```go
// 泛型结构体
type Stack[T any] struct {
    items []T
}

func (s *Stack[T]) Push(item T) {
    s.items = append(s.items, item)
}

// 泛型接口
type List[T any] interface {
    Add(item T)
    Get(index int) T
}
```

**标准库新增泛型包：**
- `golang.org/x/exp/slices` - 切片操作
- `golang.org/x/exp/maps` - Map 操作

**面试重点：**
- 泛型是编译期实现，没有运行时开销
- 类型参数不能单独作为方法的接收器类型
- 不支持泛型方法（只能是泛型类型的方法）

---

### 2. 模糊测试（Fuzzing）⭐⭐⭐⭐

自动生成测试用例，发现边界 case 和安全漏洞。

```go
func FuzzReverse(f *testing.F) {
    // 种子语料
    f.Add("hello")
    f.Add("world")

    f.Fuzz(func(t *testing.T, s string) {
        // 测试逻辑
        rev1 := Reverse(s)
        rev2 := Reverse(rev1)
        if s != rev2 {
            t.Errorf("Reverse twice should equal original: %q", s)
        }
    })
}
```

**运行：**
```bash
go test -fuzz=FuzzReverse
```

---

### 3. 工作区模式（Go Workspaces）⭐⭐⭐

同时开发多个 module，不需要 replace 指令。

```bash
# 创建 go.work
go work init ./mod1 ./mod2

# 添加 module
go work use ./mod3

# 查看
go work sync
```

**go.work 文件格式：**
```go
go 1.18

use (
    ./mod1
    ./mod2
)

replace example.com/foo => ../foo
```

---

### 其他重要特性

- `go.mod` 支持 `toolchain` 指令
- `net/http` 支持 HTTP/2 服务器推送
- `strings.Cut` 函数
- `crypto/elliptic` 性能大幅提升

---

## Go 1.19（2022.08）

**发布日期：2022年8月2日**

### 1. 类型化原子操作（Typed Atomic）⭐⭐⭐⭐⭐

**新增类型：**
- `atomic.Bool`
- `atomic.Int32`
- `atomic.Int64`
- `atomic.Uint32`
- `atomic.Uint64`
- `atomic.Uintptr`
- `atomic.Pointer[T]`

**新旧对比：**
```go
// ❌ Go 1.18 及以前
var count int64
atomic.AddInt64(&count, 1)
atomic.LoadInt64(&count)

// ✅ Go 1.19+
var count atomic.Int64
count.Add(1)
count.Load()
```

**嵌入结构体：**
```go
type Stats struct {
    RequestCount atomic.Int64  // 直接嵌入，不需要指针
    ErrorCount   atomic.Int64
    IsHealthy    atomic.Bool
}
```

---

### 2. 内存模型更新 ⭐⭐⭐

Go 内存模型明确了：
- `sync/atomic` 的 Happens-Before 保证
- 与 C++、Rust 等语言内存模型对齐

---

### 3. 其他重要特性

- `go doc` 显示更美观的文档格式
- `sync.Map` 性能优化
- `runtime` 垃圾回收性能提升
- `unix.Getgroups` 返回额外组

---

## Go 1.20（2023.02）

**发布日期：2023年2月1日**

### 1. 切片转数组/数组指针 ⭐⭐⭐⭐

```go
s := []int{1, 2, 3, 4}

// Go 1.20+ 直接转换
a := [3]int(s)       // 切片转数组
p := (*[3]int)(s)    // 切片转数组指针

fmt.Println(a)  // [1 2 3]
fmt.Println(p)  // &[1 2 3]
```

**注意：** 长度不匹配会 panic

---

### 2. Profile-Guided Optimization (PGO) 预览 ⭐⭐⭐⭐

根据运行时 profile 信息优化编译。

```bash
# 生成 profile
go test -cpuprofile cpu.pprof

# 使用 PGO 编译
go build -pgo cpu.pprof -o myapp
```

**效果：** 通常能带来 5-15% 的性能提升。

---

### 3. 错误包装增强 ⭐⭐⭐

`errors.Is` 和 `errors.As` 支持检查错误链中的多个错误：

```go
// 多个错误包装
err := fmt.Errorf("first: %w, second: %w", err1, err2)

errors.Is(err, err1)  // ✅ true
errors.Is(err, err2)  // ✅ true
```

---

### 4. `unsafe` 包新增函数 ⭐⭐⭐

```go
// 获取切片/字符串/数组的长度或容量
unsafe.SliceData(slice []T) *T
unsafe.StringData(str string) *byte
unsafe.Slice(ptr *T, len IntegerType) []T
unsafe.String(ptr *byte, len IntegerType) string
```

---

### 5. 其他重要特性

- `crypto/ecdsa` 性能提升 2 倍
- `go build` 默认禁用 CGO
- `context.WithCancelCause` - 带原因的取消
- `http.ResponseController` - 细粒度控制 HTTP 响应

```go
// WithCancelCause 示例
ctx, cancel := context.WithCancelCause(parent)
cancel(errors.New("timeout"))
fmt.Println(context.Cause(ctx))  // "timeout"
```

---

## Go 1.21（2023.08）

**发布日期：2023年8月8日**

### 1. OnceFunc / OnceValue / OnceValues ⭐⭐⭐⭐⭐

**简化单例模式，替代手动写 once + 变量：**

```go
// ✅ OnceFunc - 无返回值
var initLogger = sync.OnceFunc(func() {
    fmt.Println("初始化日志系统")
})

initLogger()  // 第一次执行
initLogger()  // 第二次不执行

// ✅ OnceValue - 一个返回值
var loadConfig = sync.OnceValue(func() *Config {
    fmt.Println("加载配置文件")
    return &Config{MaxConn: 100}
})

cfg1 := loadConfig()  // 第一次加载
cfg2 := loadConfig()  // 直接返回缓存结果

// ✅ OnceValues - 两个返回值
var connectDB = sync.OnceValues(func() (*sql.DB, error) {
    return sql.Open("mysql", dsn)
})

db, err := connectDB()
```

**⚠️ 重要特性：panic 会缓存**
```go
var f = sync.OnceValue(func() int {
    panic("出错了")
})

f()  // 第一次 panic
f()  // 第二次还会 panic！同一个值
```

---

### 2. `slog` - 结构化日志 ⭐⭐⭐⭐⭐

官方标准日志库，替代 `zap`/`logrus`。

```go
// 基础用法
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
slog.SetDefault(logger)

slog.Info("hello",
    "user", "alice",
    "age", 30,
)
// {"level":"info","msg":"hello","user":"alice","age":30}

// 级别日志
slog.Debug("debug message")
slog.Warn("warning message")
slog.Error("error message", "err", err)

// 上下文日志
logger = logger.With("request_id", "abc123")
logger.Info("processing request")
```

**自定义 Handler：**
```go
opts := &slog.HandlerOptions{
    Level:     slog.LevelDebug,
    AddSource: true,  // 显示调用位置
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        // 自定义属性处理
        return a
    },
}
```

---

### 3. 内置函数 `min` / `max` / `clear` ⭐⭐⭐⭐

```go
// min/max
fmt.Println(min(1, 2, 3))     // 1
fmt.Println(max(1.5, 2.5))    // 2.5

// clear - 清空切片或 map
s := []int{1, 2, 3}
clear(s)
fmt.Println(s)  // [0, 0, 0]  长度不变，元素归零

m := map[string]int{"a": 1, "b": 2}
clear(m)
fmt.Println(m)  // map[]  map 全部清空
```

---

### 4. `slices` / `maps` 包进入标准库 ⭐⭐⭐⭐

```go
import "slices"
import "maps"

// slices 常用函数
s := []int{3, 1, 2}
slices.Sort(s)           // 排序
slices.Contains(s, 1)    // 是否包含
slices.Index(s, 2)       // 查找索引
slices.Equal(s1, s2)     // 比较相等
slices.Clone(s)          // 克隆
slices.Delete(s, 0, 1)   // 删除元素

// maps 常用函数
m1 := map[string]int{"a": 1}
m2 := map[string]int{"b": 2}
maps.Equal(m1, m2)       // 比较相等
maps.Copy(m1, m2)        // 复制
maps.Clone(m1)           // 克隆
maps.DeleteFunc(m1, func(k string, v int) bool {
    return v < 0
})
```

---

### 5. `context.WithDeadlineCause` ⭐⭐⭐

```go
ctx, cancel := context.WithDeadlineCause(parent, deadline, errTooSlow)
defer cancel()

// 超时后可以获取原因
fmt.Println(context.Cause(ctx))  // errTooSlow
```

---

### 6. 其他重要特性

- PGO 正式启用（默认 auto 模式）
- `cmp` 包 - 比较函数
- `testing` 支持 `Testing.V` 日志级别
- 垃圾回收停顿时间进一步降低

---

## Go 1.22（2024.02）

**发布日期：2024年2月6日**

### 1. 循环变量修复 ⭐⭐⭐⭐⭐

**Go 历史上最大的坑之一终于修复！**

**Go 1.21 及以前（有问题）：**
```go
// ❌ 所有 goroutine 打印相同的值
for i := 0; i < 3; i++ {
    go func() {
        fmt.Println(i)  // 可能打印 3,3,3
    }()
}
```

**Go 1.22+（修复后）：**
```go
// ✅ 每次迭代创建新的变量
for i := 0; i < 3; i++ {
    go func() {
        fmt.Println(i)  // 正确打印 0,1,2
    }()
}
```

**面试重点：**
- Go 1.22 自动启用新语义
- `go.mod` 中 `go 1.22` 及以上才生效
- 可以用 `GODEBUG=loopvar=0` 临时关闭

---

### 2. `range` 支持整数 ⭐⭐⭐⭐

```go
// 迭代 5 次，i 从 0 到 4
for i := range 5 {
    fmt.Println(i)  // 0, 1, 2, 3, 4
}

// 不需要索引值
for range 10 {
    fmt.Println("loop")
}
```

---

### 3. PGO 正式发布（GA）⭐⭐⭐⭐

**默认自动启用：**
```bash
# 自动查找当前目录下的 default.pgo
go build -o myapp
```

**效果：**
- 性能提升 2-7%
- 编译时间增加约 10%
- 可以用 `-pgo=off` 关闭

---

### 4. `math/rand/v2` ⭐⭐⭐

**新的随机数 API：**
```go
import "math/rand/v2"

// 不需要 Seed 了！
fmt.Println(rand.IntN(100))  // 0-99
fmt.Println(rand.Float64())  // 0.0-1.0

// 新方法
rand.Uint64()
rand.Uint32()
rand.Int64N(n)
rand.Int32N(n)
```

**重要变化：**
- 自动随机种子（不需要 `rand.Seed(time.Now().UnixNano())`）
- 算法升级到 PCG 家族
- 性能更好，统计质量更高

---

### 5. `net/http` 路由增强 ⭐⭐⭐⭐

**支持方法和路径参数：**
```go
mux := http.NewServeMux()

// 按方法匹配
mux.HandleFunc("GET /items", listItems)
mux.HandleFunc("POST /items", createItem)

// 路径参数（标准库支持，不需要第三方路由）
mux.HandleFunc("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")  // 获取路径参数
    fmt.Fprintf(w, "Item: %s", id)
})

// 通配符
mux.HandleFunc("/files/{rest...}", fileHandler)
```

---

### 6. 其他重要特性

- `for` 循环支持 `range func` 迭代器（预览）
- `go vet` 新增更多检查
- 执行追踪器（tracer）完全重写，性能提升
- 互斥锁 profile 更准确

---

## Go 1.23（2024.08）

**发布日期：2024年8月13日**

### 1. 迭代器（Iterator）正式发布 ⭐⭐⭐⭐⭐

**核心概念：**
```go
// 迭代器类型定义（iter 包）
type Seq[V any] func(yield func(V) bool) bool
type Seq2[K, V any] func(yield func(K, V) bool) bool
```

**基础用法：**
```go
import "iter"

// 生成序列
func count(n int) iter.Seq[int] {
    return func(yield func(int) bool) {
        for i := range n {
            if !yield(i) {
                return
            }
        }
    }
}

// range 直接使用
for i := range count(5) {
    fmt.Println(i)  // 0, 1, 2, 3, 4
}
```

**标准库支持：**
```go
import "maps"
import "slices"

// slices
s := []int{1, 2, 3}
for v := range slices.Values(s) {
    fmt.Println(v)
}

for i, v := range slices.All(s) {
    fmt.Println(i, v)
}

// maps
m := map[string]int{"a": 1, "b": 2}
for k, v := range maps.All(m) {
    fmt.Println(k, v)
}
```

**迭代器组合：**
```go
// 过滤
func Filter[V any](seq iter.Seq[V], f func(V) bool) iter.Seq[V] {
    return func(yield func(V) bool) {
        for v := range seq {
            if f(v) && !yield(v) {
                return
            }
        }
    }
}

// 使用
for v := range Filter(slices.Values(s), func(x int) bool {
    return x > 0
}) {
    fmt.Println(v)
}
```

---

### 2. `unique` 包 - 值规范化 ⭐⭐⭐⭐

**类似"字符串驻留"，减少内存占用：**
```go
import "unique"

// 创建唯一值
s1 := unique.Make("hello")
s2 := unique.Make("hello")

fmt.Println(s1 == s2)  // true  同一个指针
fmt.Println(s1.Value())  // "hello"

// 泛型支持
type Config struct {
    MaxConn int
    Timeout time.Duration
}

c1 := unique.Make(Config{MaxConn: 100})
c2 := unique.Make(Config{MaxConn: 100})
fmt.Println(c1 == c2)  // true
```

**适用场景：**
- 大量重复值的缓存
- 配置对象去重
- 减少 GC 压力

---

### 3. Timer/Ticker 重大改进 ⭐⭐⭐⭐⭐

**两大改进：**

**1) 未 Stop 的 Timer 可以被 GC 回收**
```go
// Go 1.22 及以前 - 内存泄漏！
for {
    select {
    case <-time.After(1 * time.Second):  // ❌ 每次创建的 Timer 不会回收
        // ...
    }
}

// Go 1.23+ - 没问题，不再引用时自动回收 ✅
```

**2) Stop/Reset 行为更可预测**
```go
// 通道变成无缓冲（capacity = 0）
// Stop 后不会有 stale value
timer := time.NewTimer(time.Second)
timer.Stop()

// 现在可以安全地：
select {
case <-timer.C:  // 不会收到过期值
default:
}
```

---

### 4. `cmp.Or` - 取第一个非零值 ⭐⭐⭐

```go
import "cmp"

// 取第一个非零值
name := cmp.Or(user.Name, "anonymous")  // user.Name 空就用 "anonymous"

// 链式调用
value := cmp.Or(a, b, c, defaultVal)

// 等价于以前的：
if name == "" {
    name = "anonymous"
}
```

---

### 5. 其他重要特性

- `unique.Handle[T]` - 唯一值句柄
- `net/http` 路由性能提升
- `go` 命令支持 `GODEBUG` 环境变量更多选项
- 垃圾回收更可预测
- 编译速度提升

---

## 面试高频问答

### Q1: Go 1.18 泛型有哪些限制？
1. 不支持泛型方法（只能是泛型类型的方法）
2. 不支持操作符重载
3. 不支持泛型特化
4. 不能用 `==` 比较两个泛型函数

### Q2: Go 1.22 循环变量修复的原理是什么？
每次循环迭代创建新的变量实例，而不是复用同一个变量。`go.mod` 中版本 >= 1.22 自动启用。

### Q3: OnceValue 和普通 Once 的区别是什么？
- Once：panic 只发生一次，后续调用正常返回
- OnceValue：panic 会被缓存，每次调用都会 panic 相同的值

### Q4: PGO 是什么？对性能有多大提升？
Profile-Guided Optimization，根据运行时 profile 优化编译。通常提升 2-15%。

### Q5: Go 1.23 迭代器和其他语言有什么不同？
- Go 是"拉模式"迭代器，yield 函数控制
- 类型安全，编译期检查
- 可以随时中断迭代（yield 返回 false）

### Q6: 哪个版本的特性面试最常考？
按频率排序：
1. **Go 1.22** - 循环变量修复（必考题）
2. **Go 1.18** - 泛型
3. **Go 1.21** - OnceFunc/OnceValue、slog
4. **Go 1.19** - 类型化原子操作
5. **Go 1.23** - 迭代器（新特性，问的越来越多）

---

## 各版本特性对比总表

| Go 版本 | 发布日期 | 核心特性 | 面试重要性 |
|---------|---------|---------|-----------|
| **1.18** | 2022.03 | 泛型、模糊测试、工作区 | ⭐⭐⭐⭐⭐ |
| **1.19** | 2022.08 | 类型化原子操作、内存模型 | ⭐⭐⭐⭐ |
| **1.20** | 2023.02 | 切片转数组、PGO 预览、errors.Is 多错误 | ⭐⭐⭐ |
| **1.21** | 2023.08 | OnceFunc/OnceValue、slog、min/max、slices/maps | ⭐⭐⭐⭐⭐ |
| **1.22** | 2024.02 | 循环变量修复、range int、PGO GA、rand/v2 | ⭐⭐⭐⭐⭐ |
| **1.23** | 2024.08 | 迭代器、unique 包、Timer GC 优化 | ⭐⭐⭐⭐ |

---

## 升级建议

| 场景 | 推荐版本 | 理由 |
|------|---------|------|
| 新项目 | 1.22+ | 循环变量修复太重要，避免踩坑 |
| 性能敏感 | 1.22+ | PGO 正式版，rand/v2，编译优化 |
| 库开发 | 1.21+ | slog、slices、maps 标准库 |
| 追求稳定 | 1.20+ | 验证充分，bug 少 |
| 想尝鲜 | 1.23 | 迭代器、unique 包 |

---

## 参考来源

- [Go 1.18 Release Notes](https://go.dev/doc/go1.18)
- [Go 1.19 Release Notes](https://go.dev/doc/go1.19)
- [Go 1.20 Release Notes](https://go.dev/doc/go1.20)
- [Go 1.21 Release Notes](https://go.dev/doc/go1.21)
- [Go 1.22 Release Notes](https://go.dev/doc/go1.22)
- [Go 1.23 Release Notes](https://go.dev/doc/go1.23)
