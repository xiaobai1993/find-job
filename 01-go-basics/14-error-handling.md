# Go 错误处理最佳实践与底层原理

---

## 一、先搞懂：为什么 Go 的 error 设计成这样？

### 1. Go vs 其他语言的错误处理哲学

| 语言 | 错误处理方式 | 优点 | 缺点 |
|------|-------------|------|------|
| **C** | 返回 int 错误码 + errno | 简单，无额外开销 | 容易忽略错误，上下文丢失 |
| **Java/Python** | try-catch 异常 | 强制处理，堆栈完整 | 开销大，控制流不直观，异常捕获滥用 |
| **Rust** | Result<T, E> | 类型安全，强制处理 | 语法稍重 |
| **Go** | 返回 error 接口 | 显式，开销极小，控制流清晰 | 容易忽略，错误包装麻烦 |

**Go 的设计哲学：**
> 错误就是值，和其他值没区别。你想怎么处理就怎么处理，语言不强迫你。

---

### 2. error 接口定义

Go 的 error 就是一个内置接口，只有一个方法：

```go
// src/builtin/builtin.go
type error interface {
    Error() string
}
```

就这么简单！任何实现了 `Error() string` 方法的类型，都可以当作 error。

**面试一句话总结：** error 不是什么特殊的东西，就是一个普通接口，普通值，普通返回值。

---

## 二、errors 包核心功能详解

Go 1.13 引入了 `errors.Is`、`errors.As`、`errors.Unwrap`，彻底改变了错误处理方式。

### 1. errors.Is - 判断错误链中是否包含特定错误

```go
// 基本用法
if errors.Is(err, os.ErrNotExist) {
    fmt.Println("文件不存在")
}
```

**底层原理：**
- 沿着错误链一层层 Unwrap
- 每层都和目标错误比较（== 比较）
- 只要有一层相等，就返回 true

```go
// 伪代码
func Is(err, target error) bool {
    for {
        if err == target {
            return true
        }
        if err = Unwrap(err); err == nil {
            return false
        }
    }
}
```

**典型场景：判断特定错误类型**
```go
var ErrNotFound = errors.New("not found")

func GetUser(id int) (*User, error) {
    // ...
    return nil, fmt.Errorf("get user %d: %w", id, ErrNotFound)
}

// 外面可以判断
err := GetUser(1)
if errors.Is(err, ErrNotFound) {
    // 处理找不到的情况
}
```

---

### 2. errors.As - 把错误转换成特定类型

```go
var pathErr *os.PathError
if errors.As(err, &pathErr) {
    fmt.Println("路径错误:", pathErr.Path)
}
```

**底层原理：**
- 沿着错误链一层层 Unwrap
- 每层都检查类型是否匹配
- 匹配了就赋值，返回 true

**典型场景：提取自定义错误的额外信息**
```go
type BizError struct {
    Code    int
    Message string
}

func (e *BizError) Error() string {
    return fmt.Sprintf("code=%d, msg=%s", e.Code, e.Message)
}

// 使用
err := someFunc()
var bizErr *BizError
if errors.As(err, &bizErr) {
    fmt.Println("业务错误码:", bizErr.Code)
}
```

---

### 3. errors.Unwrap - 解开一层包装

```go
wrapped := fmt.Errorf("wrap: %w", originalErr)
original := errors.Unwrap(wrapped)  // 得到 originalErr
```

`%w` 是 Go 1.13 新增的格式化动词，专门用来包装错误，形成错误链。

---

## 三、错误包装与错误链

### 1. 为什么需要错误包装？

**反例：没有上下文的错误**
```go
// ❌ 差：只返回原始错误，完全不知道哪里出问题
func ReadConfig(path string) ([]byte, error) {
    return os.ReadFile(path)  // 直接返回 open file error
}

// 外面拿到错误："no such file or directory"
// 鬼知道是哪个文件不存在！
```

**正例：附加上下文**
```go
// ✅ 好：包装错误，附加上下文
func ReadConfig(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config %s: %w", path, err)
    }
    return data, nil
}

// 外面拿到错误："read config /etc/config.yaml: no such file or directory"
// 一目了然！
```

**面试高频：%w 和 %v 包装错误的区别？**

| 格式化动词 | 效果 | 能不能 Is/As |
|-----------|------|-------------|
| `%w` | 包装成错误链，保留原错误类型 | ✅ 能 |
| `%v` / `%s` | 转成字符串，原错误类型丢失 | ❌ 不能 |

```go
err := os.ErrNotExist

w1 := fmt.Errorf("wrap: %w", err)
errors.Is(w1, os.ErrNotExist)  // ✅ true

w2 := fmt.Errorf("wrap: %v", err)
errors.Is(w2, os.ErrNotExist)  // ❌ false！因为变成字符串了
```

---

### 2. 错误链多层嵌套

```go
func A() error {
    return fmt.Errorf("A failed: %w", os.ErrNotExist)
}

func B() error {
    err := A()
    return fmt.Errorf("B failed: %w", err)
}

func C() error {
    err := B()
    return fmt.Errorf("C failed: %w", err)
}

// 错误链：C → B → A → os.ErrNotExist
// errors.Is 可以穿透所有层找到
err := C()
errors.Is(err, os.ErrNotExist)  // ✅ true！
```

---

### 3. 错误包装最佳实践

**✅ 总是包装错误，附加上下文**
```go
// 每一层都加自己的上下文
if err != nil {
    return fmt.Errorf("create user %d: %w", userID, err)
}
```

**❌ 不要只 wrap 不加信息**
```go
// 毫无意义的包装
if err != nil {
    return fmt.Errorf("%w", err)  // 等于没包
}
```

**❌ 不要重复包装同一层**
```go
// 重复包装，冗余
err := foo()
if err != nil {
    err = fmt.Errorf("foo: %w", err)
    err = fmt.Errorf("bar: %w", err)  // 没必要
    return err
}
```

---

## 四、自定义错误类型

### 1. 最简单的自定义错误

```go
var ErrUserNotFound = errors.New("user not found")
var ErrInvalidPassword = errors.New("invalid password")
```

适合全局定义的哨兵错误（sentinel error）。

**缺点：**
- 不能携带额外信息
- 是可变的（var），别人可以修改它

---

### 2. 带额外信息的自定义错误

```go
type BizError struct {
    Code    int
    Message string
    Cause   error  // 嵌套原始错误
}

func (e *BizError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("biz error code=%d: %v, cause=%v", e.Code, e.Message, e.Cause)
    }
    return fmt.Sprintf("biz error code=%d: %v", e.Code, e.Message)
}

// 实现 Unwrap 方法，支持 errors.Is / errors.As
func (e *BizError) Unwrap() error {
    return e.Cause
}
```

**使用：**
```go
func CreateUser(name string) error {
    if len(name) == 0 {
        return &BizError{
            Code:    40001,
            Message: "name is empty",
        }
    }
    // ...
}
```

---

### 3. 哨兵错误 vs 自定义错误类型对比

| 方式 | 优点 | 缺点 | 适用场景 |
|------|------|------|---------|
| **errors.New 哨兵** | 简单，直接用 `==` 比较 | 不能带额外信息 | 全局通用错误 |
| **自定义 struct 类型** | 可携带任意信息，类型安全 | 代码多一点 | 业务错误，需要额外字段 |

---

## 五、错误处理常见模式

### 1. 提前返回（Early Return）

Go 推崇的标准写法：

```go
func doSomething() error {
    err := step1()
    if err != nil {
        return err  // 出错直接返回，不缩进
    }

    err = step2()
    if err != nil {
        return err
    }

    return step3()
}
```

**优点：**
- 正常流程不缩进，可读性好
- 出错立即处理，不积累错误
- 没有 try-catch 的嵌套地狱

---

### 2. defer 中处理错误

```go
func writeFile(path string, data []byte) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer func() {
        // Close 也可能返回错误！
        if closeErr := f.Close(); closeErr != nil {
            log.Printf("close file failed: %v", closeErr)
        }
    }()

    _, err = f.Write(data)
    return err
}
```

**面试高频坑：** defer 里的 Close 错误很容易被忽略！文件写入时，Close 可能会把 buffer flush 到磁盘，这时候也可能出错！

---

### 3. 错误聚合

多个并行操作，收集所有错误：

```go
func parallelDo() error {
    var errs []error
    var mu sync.Mutex

    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(i int) {
            defer wg.Done()
            if err := do(i); err != nil {
                mu.Lock()
                errs = append(errs, err)
                mu.Unlock()
            }
        }(i)
    }
    wg.Wait()

    if len(errs) > 0 {
        // 可以用 errors.Join（Go 1.20+）
        return errors.Join(errs...)
    }
    return nil
}
```

`errors.Join` 是 Go 1.20 新增的，可以把多个错误合并成一个。

---

## 六、Go 版本演进历史

### Go 1.0 ~ Go 1.12 - 原始时代

```go
// 只有 errors.New
err := errors.New("something wrong")

// 包装错误只能转字符串，丢失类型
wrapped := fmt.Errorf("wrap: %v", err)

// 只能用 == 比较
if err == os.ErrNotExist {
    // ...
}
```

**问题：**
- 包装错误后类型信息丢失
- 无法判断原始错误是什么
- 自定义错误比较很麻烦

---

### Go 1.13 - 错误链革命

新增三个核心函数：
- `errors.Is`
- `errors.As`
- `errors.Unwrap`

新增格式化动词 `%w`：
```go
wrapped := fmt.Errorf("wrap: %w", err)  // ✅ 保留错误链
```

---

### Go 1.20 - errors 包增强

新增：
- `errors.Join` - 合并多个错误
- `errors.ErrUnsupported` - 标准哨兵错误

---

## 七、常见坑与最佳实践

### 坑 1：忽略错误

```go
// ❌ 超级严重的坑！错误被完全吞掉！
os.WriteFile("a.txt", data, 0644)  // 不检查错误
```

**后果：** 静默失败，出问题根本不知道哪里错了。

**正确：** 每个错误都必须处理！要么 return，要么 log，要么 panic（只在初始化时）。

---

### 坑 2：到处 panic

```go
// ❌ 反模式：业务代码里 panic
func getUser(id int) *User {
    user, err := db.Query(id)
    if err != nil {
        panic(err)  // 业务代码不要 panic！
    }
    return user
}
```

**Go 约定：**
- ✅ `main` 函数初始化阶段可以 panic（配置文件不存在这种致命错误）
- ❌ 业务代码绝对不能 panic，必须 return error
- ✅ 库代码内部可以 panic，但对外必须 recover 转成 error 返回

---

### 坑 3：错误包装冗余

```go
// ❌ 每一层都包装一次，日志里一堆重复信息
// "level 3: level 2: level 1: original error"
func level1() error { return fmt.Errorf("level 1: %w", original) }
func level2() error { return fmt.Errorf("level 2: %w", level1()) }
func level3() error { return fmt.Errorf("level 3: %w", level2()) }
```

**最佳实践：** 每一层只加自己这层的上下文，不要重复。

---

### 坑 4：哨兵错误导出后被修改

```go
// ❌ 危险：var 导出，别人可以赋值修改
var ErrNotFound = errors.New("not found")

// 别人可以干这个：
ErrNotFound = nil  // 💣 整个世界都崩了！
```

**解决：** 不导出，提供判断函数；或者用自定义类型而不是值。

```go
// ✅ 不导出哨兵，提供判断函数
var errNotFound = errors.New("not found")

func IsNotFound(err error) bool {
    return errors.Is(err, errNotFound)
}
```

---

### 最佳实践总结

1. ✅ **每个错误都处理**，不要忽略
2. ✅ **总是包装错误**，附加上下文信息
3. ✅ **用 `%w` 包装**，保留错误链
4. ✅ **业务代码用 error，不要 panic**
5. ✅ **用 `errors.Is` / `errors.As` 判断错误**，不要用 `==`
6. ✅ **defer Close 也要检查错误**
7. ❌ **不要重复包装**同一层错误
8. ❌ **不要只 log 不 return**，也不要既 log 又 return（重复log）

---

## 八、面试高频问答

| 问题 | 答案 |
|------|------|
| error 是什么类型？ | error 是一个内置接口，只有 `Error() string` 一个方法。 |
| `%w` 和 `%v` 包装错误的区别？ | `%w` 包装成错误链，保留类型信息，支持 errors.Is/As；`%v` 转成字符串，类型丢失。 |
| errors.Is 怎么工作的？ | 沿着错误链一层层 Unwrap，每层都和目标用 `==` 比较，找到就返回 true。 |
| errors.As 怎么工作的？ | 沿着错误链一层层 Unwrap，检查类型是否匹配，匹配就赋值返回 true。 |
| 为什么要包装错误？ | 附加上下文，方便定位问题。只返回原始错误的话，外面不知道是哪里出的问题。 |
| 业务代码里可以 panic 吗？ | 约定是不可以。panic 只用在程序初始化阶段的致命错误，业务代码必须返回 error。 |
| defer 里的 Close 错误需要处理吗？ | 需要。文件 Close 时会 flush buffer，也可能出错（比如磁盘满了），至少要 log 一下。 |
| Go 1.13 错误处理有什么变化？ | 新增 errors.Is/As/Unwrap，新增 `%w` 格式化动词，支持错误链。 |
| Go 1.20 errors 包有什么增强？ | 新增 errors.Join 合并多个错误，新增 ErrUnsupported 哨兵错误。 |
| 哨兵错误有什么缺点？ | 不能携带额外信息，是 var 可变的，导出后可能被别人修改。 |
| 自定义错误类型需要实现什么？ | 必须实现 `Error() string`，如果要支持错误链还要实现 `Unwrap() error`。 |

---

## 九、一句话总结

> Go 的 error 就是普通接口普通值，Go 1.13 引入错误链后用 `%w` 包装，用 `errors.Is`/`errors.As` 判断；永远不要忽略错误，永远附加上下文，业务代码不要 panic，这就是 Go 错误处理的最佳实践。
