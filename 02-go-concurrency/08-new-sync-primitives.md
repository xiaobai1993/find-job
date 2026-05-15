# Go 1.19+ 新增同步原语详解

---

## 一、Go 1.19：类型化原子操作（Typed Atomic）

### 1. 是什么？

Go 1.19 在 `sync/atomic` 包新增了一系列**类型化的原子变量**，取代之前需要手动处理指针的写法。

**之前的写法（Go 1.18 及之前）：**
```go
var count int64

// 加
atomic.AddInt64(&count, 1)

// 读
v := atomic.LoadInt64(&count)

// 写
atomic.StoreInt64(&count, 100)

// CAS
atomic.CompareAndSwapInt64(&count, old, new)
```

**Go 1.19+ 新写法：**
```go
var count atomic.Int64

// 加
count.Add(1)

// 读
v := count.Load()

// 写
count.Store(100)

// CAS
count.CompareAndSwap(old, new)
```

**代码更简洁、更安全，不会传错类型！**

---

### 2. 全部新增类型

| 类型 | 说明 | 可用方法 |
|------|------|---------|
| `atomic.Bool` | 原子布尔值 | Load, Store, Swap, CompareAndSwap |
| `atomic.Int32` | 原子 int32 | Load, Store, Swap, CompareAndSwap, Add |
| `atomic.Int64` | 原子 int64 | Load, Store, Swap, CompareAndSwap, Add |
| `atomic.Uint32` | 原子 uint32 | 同上 |
| `atomic.Uint64` | 原子 uint64 | 同上 |
| `atomic.Uintptr` | 原子 uintptr | 同上 |
| `atomic.Pointer[T]` | 泛型原子指针（Go 1.18+） | Load, Store, Swap, CompareAndSwap |

---

### 3. 为什么要引入类型化原子操作？

**解决的痛点：**

1. **类型安全**：不会把 `*int64` 不小心传给 `atomic.AddInt32`，编译器直接报错
2. **代码简洁**：不需要手动取地址，方法调用更直观
3. **内嵌友好**：可以直接嵌入结构体，不需要额外处理指针对齐

**示例：嵌入结构体**
```go
type Stats struct {
    RequestCount atomic.Int64  // ✅ 直接嵌入，不需要是指针
    ErrorCount   atomic.Int64
}

var s Stats
s.RequestCount.Add(1)  // 直接调用，不需要 &
```

---

### 4. 注意事项

1. **零值可用**：`atomic.Int64` 零值就是 0，不需要初始化
2. **不能拷贝**：和所有同步原语一样，值拷贝会失效，go vet 会检查
3. **性能和旧版完全一样**：只是语法糖，底层调用相同的 runtime 函数

---

## 二、Go 1.21：OnceFunc / OnceValue / OnceValues

### 1. 是什么？

Go 1.21 在 `sync` 包新增了三个泛型辅助函数，基于 `Once` 封装，用于只执行一次的场景。

| 函数 | 签名 | 用途 |
|------|------|------|
| `OnceFunc` | `func OnceFunc(f func()) func()` | 无返回值，保证 f 只执行一次 |
| `OnceValue[T]` | `func OnceValue[T any](f func() T) func() T` | 有一个返回值，保证 f 只执行一次 |
| `OnceValues[T1, T2]` | `func OnceValues[T1, T2 any](f func() (T1, T2)) func() (T1, T2)` | 有两个返回值，保证 f 只执行一次 |

---

### 2. 基本用法

**OnceFunc：**
```go
var initLog = sync.OnceFunc(func() {
    log.Println("系统初始化完成")
})

initLog()  // 第一次调用执行
initLog()  // 第二次调用不执行
initLog()  // 第三次也不执行
```

**OnceValue：**
```go
var loadConfig = sync.OnceValue(func() *Config {
    // 只执行一次，从文件加载配置
    data, _ := os.ReadFile("config.json")
    var cfg Config
    json.Unmarshal(data, &cfg)
    return &cfg
})

cfg1 := loadConfig()  // 第一次调用，加载配置
cfg2 := loadConfig()  // 第二次调用，直接返回缓存的结果
// cfg1 和 cfg2 指向同一个对象
```

**OnceValues：**
```go
var loadDB = sync.OnceValues(func() (*sql.DB, error) {
    return sql.Open("mysql", "dsn")
})

db1, err1 := loadDB()  // 第一次调用，连接数据库
db2, err2 := loadDB()  // 第二次调用，直接返回
```

---

### 3. 关键特性（非常重要！）

#### 特性 1：并发安全，等待执行完成
和 `Once.Do()` 一样，多个 goroutine 同时调用时，**所有 goroutine 都会等第一个执行完才能返回**。

```go
var getValue = sync.OnceValue(func() int {
    time.Sleep(100 * time.Millisecond)  // 模拟慢操作
    return 42
})

go fmt.Println(getValue())  // G1 调用，开始执行
go fmt.Println(getValue())  // G2 调用，会等 G1 执行完，返回相同的结果
```

#### 特性 2：panic 会缓存，每次调用都会 panic 相同的值
**这是和直接用 Once 最大的区别！**

```go
var f = sync.OnceValue(func() int {
    panic("出错了")
})

f()  // 第一次调用，panic
f()  // 第二次调用，还是 panic！同一个值
// ❌ 普通 Once 只 panic 一次，OnceValue 每次调用都会 panic
```

**源码实现原理：**
```go
func OnceValue[T any](f func() T) func() T {
    var (
        once   Once
        valid  bool   // f 是否正常执行完成
        p      any    // 缓存 panic 的值
        result T      // 缓存返回值
    )
    g := func() {
        defer func() {
            p = recover()
            if !valid {
                panic(p)  // 第一次调用，直接 panic 出去
            }
        }()
        result = f()
        f = nil
        valid = true  // f 正常完成了才设为 true
    }
    return func() T {
        once.Do(g)
        if !valid {
            panic(p)  // 之前 panic 过，每次都重新 panic
        }
        return result
    }
}
```

**设计原因：**
如果 f panic 了，说明初始化失败，后续调用也应该失败，不能返回零值（零值可能是合法值，会掩盖错误）。

---

### 4. 最佳实践

✅ **推荐场景：**
- 单例模式（配置、数据库连接、客户端实例）
- 延迟初始化（懒加载）
- 只需要执行一次的初始化逻辑

❌ **不推荐场景：**
- 需要重试的初始化（Once 系列失败了不会重试）
- 有多个返回值且需要处理错误（用 OnceValue 返回 struct + error）

---

## 三、对比与迁移指南

### 1. 类型化原子操作迁移

```go
// 旧写法
var count int64
atomic.AddInt64(&count, 1)

// 新写法
var count atomic.Int64
count.Add(1)
```

**迁移建议：** 新代码一律用类型化原子操作，老代码可逐步重构。

---

### 2. OnceValue vs 手动实现单例

**手动实现（之前的写法）：**
```go
var (
    config *Config
    once   sync.Once
)

func GetConfig() *Config {
    once.Do(func() {
        config = loadConfig()
    })
    return config
}
```

**OnceValue 写法：**
```go
var GetConfig = sync.OnceValue(func() *Config {
    return loadConfig()
})
```

**代码量减少了 50%，而且不会出错！**

---

## 四、面试高频问答

| 问题 | 答案 |
|------|------|
| 类型化原子操作性能和旧版比怎么样？ | 完全一样，只是语法糖，底层调用相同的 runtime 函数。编译器会直接内联。 |
| OnceFunc 和 Once.Do 有什么区别？ | 最大区别是 panic 缓存：Once.Do 只 panic 一次，OnceFunc 每次调用都会 panic 相同的值。 |
| OnceValue 支持多少个返回值？ | Go 1.21 只提供了 0 个、1 个、2 个返回值的版本。需要更多的话自己封装 struct。 |
| OnceValue 返回的指针可以修改吗？ | 可以，OnceValue 只保证函数只执行一次，不保证返回值不可变。 |
| 类型化原子操作有 Add 方法吗？ | Bool 和 Pointer 没有，Int/Uint 系列有。 |

---

## 五、总结

| Go 版本 | 新增特性 | 解决的问题 |
|---------|---------|-----------|
| **Go 1.19** | 类型化原子操作（Bool/Int32/Int64/Pointer[T]...） | 类型安全、代码简洁、避免手动取地址 |
| **Go 1.21** | OnceFunc/OnceValue/OnceValues | 简化单例/延迟初始化代码，正确处理 panic 场景 |

**最佳实践：**
1. 新代码一律用类型化原子操作，不要用旧的函数形式
2. 单例、延迟初始化场景优先用 OnceValue，不用自己写 once + 变量
3. 注意 OnceValue 的 panic 缓存特性，失败需要重试的场景不要用
