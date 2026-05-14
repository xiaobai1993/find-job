# defer 底层原理与常见坑

---

## 一、先搞懂：defer 到底是什么？

很多人以为 defer 就是「函数返回前执行」，其实远远不止这么简单。defer 是 Go 语言里最容易踩坑的关键字之一，也是面试最高频的考点。

---

## 二、defer 核心规则（4 条）

### 规则 1：LIFO 后进先出执行顺序

```go
func main() {
    defer fmt.Println("1")
    defer fmt.Println("2")
    defer fmt.Println("3")
}
// 输出：
// 3
// 2
// 1
```

多个 defer 注册的顺序和执行顺序**完全相反**，像栈一样：先进后出，后进先出。

---

### 规则 2：defer 注册时，参数立刻求值（超级大坑！）

```go
func main() {
    i := 0
    defer fmt.Println(i)  // ❌ 这里 i 的值已经确定是 0 了！
    i = 100
}
// 输出：0，不是 100！
```

**这是 90% 的人都踩过的坑！**

defer 后面的函数调用，**参数是在注册的时候就计算好的**，不是在 defer 执行的时候计算。

上面的代码等价于：
```go
func main() {
    i := 0
    temp := i  // defer 注册时，立刻把参数的值保存下来
    i = 100
    fmt.Println(temp)  // 执行的时候用保存的值
}
```

---

### 规则 3：return 不是原子操作！return = 赋值 + 调用 defer + 返回

这是面试最高频考点，没有之一。

```go
func f() int {
    i := 0
    defer func() {
        i++  // 这个修改有用吗？
    }()
    return i
}

func main() {
    fmt.Println(f())  // 输出什么？
}
```

**答案：0，不是 1！**

为什么？拆解一下 `return i` 到底做了什么：

```go
// return i 不是原子操作，拆成三步：
func f() int {
    i := 0

    // 第一步：给返回值赋值
    返回值 = i  // 返回值 = 0

    // 第二步：执行 defer
    defer func() {
        i++  // 修改的是局部变量 i，不是返回值！
    }()

    // 第三步：return 返回值
    return 返回值  // 返回 0
}
```

**defer 是在「给返回值赋值」之后，「真正 return」之前执行的。**

---

### 规则 4：命名返回值 vs 匿名返回值，结果完全不一样！

这是规则 3 的延伸，也是最容易搞混的点。

#### 案例 A：匿名返回值
```go
func f() int {  // ❌ 匿名返回值
    i := 0
    defer func() {
        i++
    }()
    return i
}
fmt.Println(f())  // 输出：0
```

#### 案例 B：命名返回值
```go
func f() (i int) {  // ✅ 命名返回值
    i = 0
    defer func() {
        i++
    }()
    return i
}
fmt.Println(f())  // 输出：1！
```

**为什么？区别在哪里？**

拆解命名返回值的情况：
```go
func f() (i int) {
    i = 0  // 返回值就是 i 本身！

    // 第一步：return i 对于命名返回值就是 return（没有额外赋值）
    // 因为返回值变量已经是 i 了

    // 第二步：执行 defer
    defer func() {
        i++  // 修改的就是返回值本身！
    }()

    // 第三步：return 返回值 i，此时 i 已经变成 1 了
    return
}
```

**核心区别：**
- 匿名返回值：return 时会创建一个临时变量，defer 修改不了这个临时变量
- 命名返回值：函数入口就创建了返回值变量，defer 可以直接修改它

这是 Go 面试考 defer 必出的题，一定要背下来！

---

## 三、90% 的人都踩过的 5 个坑

### 坑 1：循环里的 defer（永远的痛）

```go
// ❌ 错误写法：defer 在循环结束后才一起执行，而且还会捕获错误的变量
func main() {
    for i := 0; i < 3; i++ {
        defer fmt.Println(i)
    }
}
// Go 1.21 及以前：输出 2 2 2！
// Go 1.22 及以后：输出 2 1 0！
```

**⚠️ 面试超级加分项：这和 Go 版本有关！**

- **Go 1.21 及以前：循环变量 i 是同一个变量，每次循环复用，三个 defer 保存的都是同一个 i 的引用，循环结束时 i = 2，所以输出三个 2
- **Go 1.22 及以后：每次循环都会创建新的 i 变量，所以三个 defer 保存的是不同的 i，输出 2 1 0

这个改变是 Go 1.22 最重要的语义变化，也是现在面试的高频考点！

**正确写法：传值进去**
```go
// ✅ 正确：每次循环把当前的 i 作为参数传进去
func main() {
    for i := 0; i < 3; i++ {
        defer func(n int) {
            fmt.Println(n)
        }(i)  // 这里的 i 是每次循环的当前值！
    }
}
// 输出：2 1 0
```

**支付业务特别提醒：**
```go
// ❌ 错误：循环里 defer Close()，如果循环 10000 次，内存直接炸
for _, file := range files {
    f, _ := os.Open(file)
    defer f.Close()  // 所有 Close 都等到函数结束才执行，打开的文件句柄越来越多
    // 处理文件
}
```

**解决：把循环体抽成单独的函数，每个函数退出时立即 close**
```go
// ✅ 正确：每个文件处理完立即 close
for _, file := range files {
    func() {
        f, _ := os.Open(file)
        defer f.Close()  // 匿名函数退出时立即 close
        // 处理文件
    }()
}
```

---

### 坑 2：defer 闭包捕获的是引用，不是值

```go
func main() {
    i := 0
    defer func() {
        fmt.Println(i)  // 捕获的是 i 的引用，不是值
    }()
    i = 100
}
// 输出：100！
```

**和规则 2 对比：**
- `defer fmt.Println(i)`：参数是 i，注册时求值，输出 0
- `defer func() { fmt.Println(i) }()`：函数体内引用 i，执行时求值，输出 100

**这是面试最容易搞混的两个写法！** 一定要分清楚。

---

### 坑 3：defer 里面调用有返回值的函数，返回值会被丢弃

```go
func getName() string {
    return "alice"
}

func main() {
    defer getName()  // 返回值被丢弃了，没人接收
}
```

这个是语法规定，defer 后面的函数调用的返回值会被直接丢弃。

**所以：**
```go
// ❌ 你以为能拿到返回值，其实拿不到
defer resp, err := http.Get(url)  // 没用，resp 和 err 后面用不了
```

---

### 坑 4：defer nil 函数会 panic

```go
var f func()

func main() {
    defer f()  // f 是 nil，执行的时候会 panic！
}
```

注册的时候不会 panic，执行的时候才会。所以 defer 的函数可能是 nil 的时候一定要小心。

---

### 坑 5：defer 在 panic 之后注册不会执行

```go
func main() {
    panic("oh no")
    defer fmt.Println("你永远看不到这行")  // 这行永远不会执行
}
```

defer 必须在 panic 发生之前注册才会执行。

---

## 四、defer 底层原理

### 1. _defer 结构体

每个 defer 语句在 runtime 里对应一个 `_defer` 结构体：

```go
type _defer struct {
    siz     int32       // 参数和返回值的大小
    started bool
    sp      uintptr     // 调用 defer 时的栈指针
    pc      uintptr     // 返回地址
    fn      *funcval    // defer 要执行的函数
    _panic  *_panic     // 触发这个 defer 的 panic
    link    *_defer     // 链表指针，指向下一个 defer
}
```

**关键信息：**
- 每个 goroutine 有自己的 defer 链表，头指针存在 `g._defer` 里
- 新注册的 defer 插入链表头部，所以执行时是 LIFO 顺序
- `sp` 记录了注册 defer 时的栈指针，只有同一个栈帧的 defer 才会被执行
- `fn` 就是 defer 后面的函数指针

---

### 2. defer 的生命周期

```go
func f() {
    // 遇到 defer 语句：
    // 1. 创建 _defer 结构体
    // 2. 计算参数值，拷贝到 _defer 旁边的内存
    // 3. 把 _defer 插入到当前 goroutine 的 defer 链表头部

    defer fmt.Println("hello")

    // 函数返回前：
    // 1. 从 defer 链表头部依次取出 _defer
    // 2. 执行 fn 函数
    // 3. 释放 _defer 结构体
}
```

---

### 3. Go 1.13 之前的 defer：堆分配，性能很差

Go 1.13 之前，**所有 defer 都是在堆上分配的**，每次 defer 大概开销是 100ns 左右。

所以以前的最佳实践是：**热路径不要用 defer**。

---

### 4. Go 1.13 之后的优化：栈分配 defer

Go 1.13 引入了一个重大优化：**大部分 defer 可以在栈上分配**，不需要堆分配。

满足以下条件就可以栈分配：
- defer 后面的函数不是闭包，或者闭包没有捕获变量
- 同一个函数里 defer 数量不超过 8 个
- defer 不在循环里

栈分配的 defer 开销只有 **10ns 左右**，比堆分配快了 10 倍！

**现在 defer 已经很便宜了，大部分场景可以放心用。**

---

### 5. Go 1.14 的进一步优化：开放编码（Open Coded）defer

Go 1.14 又引入了更激进的优化：编译器直接把 defer 函数调用内联到 return 前面，完全不需要 runtime 参与。

```go
// 源代码
func f() {
    defer mutex.Unlock()
    // ...
}

// 编译器优化后等价于：
func f() {
    // ...
    mutex.Unlock()  // 直接插在 return 前面，完全没有 defer 开销！
}
```

这种优化后的 defer 开销是 **0**，和直接调用函数一样快。

**但是有条件：**
- 函数内 defer 数量不超过 8 个
- return 和 defer 之间没有 if return
- defer 和 return 之间没有 if return

**面试加分项：** 知道 defer 的三次性能优化历史，说明你真的深入研究过 Go。

---

## 五、defer 性能对比

| Go 版本 | 优化方式 | 每次 defer 开销 |
|---------|---------|----------------|
| 1.12 及以前 | 全部堆分配 | ~100ns |
| 1.13 | 栈分配 defer | ~10ns |
| 1.14+ | 开放编码 defer | ~0ns |

**一句话结论：现在的 defer 已经非常快了，除非是极致性能敏感的场景，否则放心用。**

---

## 六、defer 经典应用场景

### 场景 1：资源释放（最常用）

```go
f, err := os.Open("file.txt")
if err != nil {
    return err
}
defer f.Close()  // 打开成功就 defer close，确保一定会执行
```

### 场景 2：锁的释放

```go
mu.Lock()
defer mu.Unlock()  // 加锁之后立即 defer unlock
// 临界区
```

**支付业务特别重要：** 锁如果不释放，整个系统就死锁了，用 defer 确保无论怎么 panic 锁都会释放。

### 场景 3：panic 捕获

```go
func safeCall() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("recovered from panic: %v", r)
        }
    }()

    // 可能 panic 的代码
    riskyOperation()
}
```

### 场景 4：函数执行时间统计

```go
func trackTime(name string) func() {
    start := time.Now()
    return func() {
        log.Printf("%s took %v", name, time.Since(start))
    }
}

func processOrder() {
    defer trackTime("processOrder")()  // 一行代码就搞定耗时统计
    // 处理订单...
}
```

---

## 七、面试高频问答

| 问题 | 答案 |
|------|------|
| defer 执行顺序是什么？ | LIFO 后进先出，先注册的后执行，后注册的先执行。 |
| defer 后面的函数参数什么时候求值？ | 注册的时候就求值，不是执行的时候。这是最大的坑。 |
| return 和 defer 哪个先执行？ | return 不是原子操作，顺序是：给返回值赋值 → 执行 defer → 真正返回。 |
| 命名返回值和匿名返回值和 defer 交互有什么区别？ | 匿名返回值：defer 修改不了临时返回变量；命名返回值：defer 可以直接修改返回值本身，因为返回值变量在函数入口就创建了。 |
| 循环里的 defer 有什么坑？ | 两个坑：1) 所有 defer 都等到循环结束才执行，资源不及时释放；2) 参数求值问题，容易捕获到错误的变量值。 |
| defer 闭包捕获的是值还是引用？ | 引用，执行的时候才读取变量的当前值。 |
| Go 1.13 defer 做了什么优化？ | 栈分配 defer，大部分 defer 不需要堆分配，性能从 100ns 降到 10ns。 |
| Go 1.14 defer 做了什么优化？ | 开放编码 defer，编译器直接把 defer 内联到 return 前面，零开销。 |
| defer 里面修改返回值需要什么条件？ | 必须是命名返回值，匿名返回值改不了。 |
| defer nil 函数会怎么样？ | 注册的时候不会 panic，执行的时候才会 panic。 |
| panic 之后的 defer 会执行吗？ | panic 之后注册的 defer 不会执行，panic 之前注册的会执行。 |
| defer 有性能问题吗？ | 现在几乎没有了，Go 1.14 之后大部分 defer 都是零开销。只有循环里的 defer 或者超过 8 个 defer 的时候才会有开销。 |
| Go 1.22 defer 循环变量有什么变化？ | Go 1.22 之前：循环变量复用，defer 都捕获同一个变量，输出一样的值；Go 1.22 之后：每次循环创建新变量，defer 捕获不同的变量，输出正确。这是 Go 1.22 最重要的语义变化。 |

---

## 八、一句话总结

> defer 注册时参数立即求值，执行顺序 LIFO；return = 赋值 → 执行 defer → 返回；命名返回值 defer 可以改返回值，匿名返回值不行；Go 1.14 之后 defer 已经零开销，放心用。
