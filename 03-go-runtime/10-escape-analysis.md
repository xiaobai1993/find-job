# Go 内存逃逸分析深度解析

---

## 零、Go 栈 vs 堆：什么分配在哪里？

### 1. 栈和堆的本质区别

| | 栈（Stack） | 堆（Heap） |
|---|---|---|
| **谁管理** | 编译器自动管理 | GC（垃圾回收器）管理 |
| **分配方式** | 移动栈指针，几乎零开销 | malloc 申请，需要找空闲内存块 |
| **回收方式** | 函数返回自动回收（移动栈指针） | GC 标记清除，需要 STW |
| **速度** | 极快（1~2 条 CPU 指令） | 慢（malloc + GC 扫描 + STW，差 10~100 倍） |
| **缓存友好** | ✅ 连续内存，L1 缓存命中率高 | ❌ 分散分配，缓存命中率低 |
| **线程安全** | ✅ 每个 goroutine 独立栈 | ⚠️ 多 goroutine 共享，需要加锁/并发控制 |
| **大小** | goroutine 初始 2KB，最大 1GB | 受系统内存限制 |
| **碎片** | ❌ 无碎片（连续弹栈压栈） | ⚠️ 有碎片（频繁分配释放） |

---

### 2. 一定在栈上的（编译器保证）

#### 2.1 函数内的局部变量（值类型，不逃逸）

```go
func add(a, b int) int {
    result := a + b    // ✅ 栈上，函数返回自动回收
    return result
}
```

#### 2.2 函数内的局部结构体（值类型，不逃逸）

```go
func calc() int {
    p := Point{X: 1, Y: 2}  // ✅ 栈上，16 字节
    return p.X + p.Y
}
```

#### 2.3 编译期能确定大小的小数组/slice

```go
func localArray() int {
    arr := [4]int{1, 2, 3, 4}  // ✅ 栈上，32 字节
    return arr[0]
}

func localSlice() int {
    s := make([]int, 4)  // ✅ 栈上，底层数组也在栈上（小且不逃逸）
    s[0] = 42
    return s[0]
}
```

#### 2.4 for 循环内的临时变量

```go
func sum(nums []int) int {
    total := 0          // ✅ 栈上
    for _, n := range nums {
        total += n      // ✅ n 也是栈上
    }
    return total
}
```

#### 2.5 值类型参数和返回值

```go
func double(x int) int {  // x 在栈上
    return x * 2           // 返回值在调用方的栈上
}
```

#### 2.6 方法接收者为值类型

```go
type Rect struct{ W, H float64 }

func (r Rect) Area() float64 {  // 值接收者，r 在栈上
    return r.W * r.H
}
```

---

### 3. 一定在堆上的（编译器保证）

#### 3.1 全局变量

```go
var globalConfig = &Config{MaxConn: 100}  // ❌ 堆上，程序整个生命周期都存活
var globalCache = make(map[string]string) // ❌ 堆上
```

#### 3.2 通过 new 创建的对象

```go
func create() *int {
    p := new(int)  // ❌ 堆上（new 总是返回指针，这里逃逸了）
    *p = 42
    return p
}
```

**注意：** `new` 不一定都在堆上！如果编译器分析后确定不逃逸，`new` 的对象也可能在栈上：

```go
func localNew() int {
    p := new(int)  // ✅ 可能栈上！因为 p 没有逃出函数
    *p = 42
    return *p      // 返回值，不是指针
}
```

#### 3.3 通过 make 创建且逃逸的引用类型

```go
func createSlice() []int {
    return make([]int, 1000)  // ❌ 堆上，返回了 slice
}

func createMap() map[string]int {
    return make(map[string]int)  // ❌ 堆上，返回了 map
}

func createChan() chan int {
    return make(chan int, 10)  // ❌ 堆上，返回了 channel
}
```

#### 3.4 闭包捕获的变量

```go
func counter() func() int {
    count := 0  // ❌ 堆上，闭包捕获
    return func() int {
        count++
        return count
    }
}
```

#### 3.5 大对象（> 64KB）

```go
func bigAlloc() {
    _ = make([]byte, 100*1024)  // ❌ 堆上，超过 64KB
}
```

#### 3.6 运行时才能确定大小的对象

```go
func dynamicAlloc(n int) []byte {
    return make([]byte, n)  // ❌ 堆上，n 是运行时才知道的
}
```

---

### 4. 可能栈上也可能堆上（取决于逃逸分析）

这是最复杂的部分，同一个写法在不同场景下分配位置不同：

#### 4.1 make([]T, n) — 大小决定

```go
func small() int {
    s := make([]int, 10)    // ✅ 栈上，小且局部使用
    return s[0]
}

func big() {
    _ = make([]int, 10000)  // ❌ 堆上，太大
}

func returned() []int {
    s := make([]int, 10)    // ❌ 堆上，返回了引用
    return s
}
```

#### 4.2 new(T) — 是否逃逸决定

```go
func localNew() int {
    p := new(int)   // ✅ 栈上，不逃逸
    *p = 42
    return *p
}

func escapeNew() *int {
    p := new(int)   // ❌ 堆上，返回指针
    *p = 42
    return p
}
```

#### 4.3 &T{} — 是否逃逸决定

```go
func localVar() int {
    u := &User{Name: "alice"}  // ✅ 栈上，不逃逸
    return u.Age
}

func escapeVar() *User {
    u := &User{Name: "alice"}  // ❌ 堆上，返回指针
    return u
}
```

#### 4.4 slice 元素是指针还是值

```go
func valueSlice() int {
    s := []int{1, 2, 3}      // ✅ 可能栈上，值类型
    return s[0]
}

func ptrSlice() {
    s := []*User{             // ❌ 堆上，每个元素都是指针
        {Name: "alice"},
        {Name: "bob"},
    }
    _ = s
}
```

---

### 5. Go 各类型默认分配位置

| 类型 | 默认分配 | 说明 |
|------|---------|------|
| `int`, `float64`, `bool` 等基本类型 | **栈** | 值类型，函数内局部使用时在栈上 |
| `string` | **视情况** | string 结构在栈上（指针+长度），底层数组在堆上 |
| `array`（数组）| **栈** | 值类型，大小编译期确定 |
| `struct`（小，不逃逸）| **栈** | 值类型，函数内局部使用 |
| `struct`（大或逃逸）| **堆** | 逃逸或超过 64KB |
| `slice` | **视情况** | slice 结构在栈上（指针+长度+容量），底层数组视逃逸情况 |
| `map` | **堆** | 引用类型，`make` 创建的 map 总在堆上 |
| `channel` | **堆** | 引用类型，`make` 创建的 channel 总在堆上 |
| `interface{}` | **堆** | 赋值给 interface 的值逃逸到堆 |
| `func`（闭包）| **堆** | 闭包捕获的变量在堆上 |
| `pointer`（局部不逃逸）| **栈** | 指向栈上对象 |
| `pointer`（逃逸）| **堆** | 指向堆上对象 |

---

### 6. 值类型 vs 引用类型（面试必区分！）

| | 值类型 | 引用类型 |
|---|---|---|
| **种类** | int, float, bool, string, array, struct | slice, map, channel, pointer, func, interface |
| **赋值行为** | 拷贝整个值 | 拷贝指针（共享底层数据） |
| **默认分配** | 栈（不逃逸时） | 堆（make 创建） |
| **函数传参** | 值拷贝，不影响原值 | 指针拷贝，修改会影响原值 |
| **零值** | 各类型零值（0, "", false） | nil |

**注意：string 是值类型，但底层数据在堆上！**

```go
s1 := "hello"       // s1 结构在栈上，底层数据在只读段
s2 := s1            // 值拷贝，s2 和 s1 共享底层数据（COW）
s3 := s1 + " world" // 新字符串，底层数据在堆上
```

---

### 7. goroutine 栈的特殊之处

Go 的栈和 C 的栈有很大区别：

| 特性 | C 栈 | Go goroutine 栈 |
|------|------|-----------------|
| 大小 | 固定（通常 1~8MB） | 动态增长（2KB → 最大 1GB） |
| 增长方式 | 不可增长，溢出就崩溃 | 自动扩容，每次 2 倍 |
| 位置 | 操作系统分配 | Go runtime 分配（可移动！） |
| 线程模型 | 1 线程 = 1 栈 | 1 goroutine = 1 栈，多个 goroutine 复用同一线程栈 |

**栈扩容过程：**
```
goroutine 调用函数
    ↓
检查栈空间够不够（stackguard0）
    ↓
不够 → runtime.morestack()
    ↓
分配新栈（2 倍大小）
    ↓
拷贝旧栈数据到新栈
    ↓
调整所有指针指向新栈
    ↓
继续执行
```

**这就是为什么 Go 不怕深递归（栈不够会自动扩），而 C 会 stack overflow！**

---

### 8. 一个完整的例子：同一个变量，不同写法的分配位置

```go
type Config struct {
    MaxConn int
    Timeout int
}

// ✅ 栈上：值类型，局部使用
func stackAlloc() int {
    c := Config{MaxConn: 100, Timeout: 30}
    return c.MaxConn
}

// ❌ 堆上：返回指针
func heapAlloc() *Config {
    c := Config{MaxConn: 100, Timeout: 30}
    return &c
}

// ✅ 栈上：new 但不逃逸
func stackNew() int {
    c := new(Config)
    c.MaxConn = 100
    return c.MaxConn  // 返回值，不是指针
}

// ❌ 堆上：赋值给 interface
func heapInterface() {
    c := Config{MaxConn: 100, Timeout: 30}
    var i interface{} = c  // 逃逸到堆
    _ = i
}

// ❌ 堆上：闭包捕获
func heapClosure() func() int {
    c := Config{MaxConn: 100, Timeout: 30}
    return func() int {
        return c.MaxConn  // 闭包捕获 c，逃逸到堆
    }
}

// ✅ 栈上：值接收者方法
func (c Config) GetMaxConn() int {  // c 在栈上
    return c.MaxConn
}
```

---

### 9. 栈和堆分配场景速查表

| 写法 | 分配位置 | 条件 |
|------|---------|------|
| `x := 42` | ✅ 栈 | 局部变量，不逃逸 |
| `x := "hello"` | ✅ 栈（结构）+ 堆（数据） | string 底层数据在堆 |
| `arr := [4]int{1,2,3,4}` | ✅ 栈 | 小数组，不逃逸 |
| `s := []int{1,2,3}` | 视情况 | 不逃逸且小→栈；逃逸或大→堆 |
| `m := make(map[string]int)` | ❌ 堆 | map 总在堆上 |
| `ch := make(chan int)` | ❌ 堆 | channel 总在堆上 |
| `p := new(int)` | 视情况 | 不逃逸→栈；逃逸→堆 |
| `p := &Config{...}` | 视情况 | 不逃逸→栈；逃逸→堆 |
| `c := Config{...}` | ✅ 栈 | 值类型，不逃逸 |
| `return &x` | ❌ 堆 | 返回局部变量指针 |
| `return x` | ✅ 栈 | 返回值拷贝 |
| `ch <- &x` | ❌ 堆 | 发送到 channel |
| `fmt.Println(x)` | ❌ 堆 | interface{} 参数 |
| `var global = ...` | ❌ 堆 | 全局变量 |
| `make([]byte, 100KB)` | ❌ 堆 | 超过 64KB |
| 闭包 `func() { x++ }` | ❌ 堆 | 闭包捕获 x |

---

## 一、什么是逃逸分析？

### 一句话总结

**编译器决定一个变量分配在栈上还是堆上的过程，就叫逃逸分析。**

### 为什么重要？

| 分配位置 | 回收方式 | 开销 | 影响 GC |
|---------|---------|------|---------|
| **栈** | 函数返回自动回收 | 几乎为零（移动栈指针） | ❌ 不影响 |
| **堆** | GC 标记清除回收 | 昂贵（需要 GC 扫描、STW） | ✅ 增加 GC 压力 |

**核心原则：能放栈上的，绝不放堆上！**

---

## 二、什么是"逃逸"？

**一个变量的引用被函数外部持有，编译器无法保证它只在栈上安全使用，就必须分配到堆上。这就是"逃逸"。**

```
func foo() *int {
    x := 42       // x 逃逸到堆上了！
    return &x     // 返回了 x 的指针，foo 返回后栈帧销毁，x 必须在堆上存活
}
```

**不逃逸的情况：**
```
func bar() int {
    x := 42       // x 不逃逸，在栈上
    return x      // 返回的是值拷贝，x 随 bar 返回自动回收
}
```

---

## 三、如何查看逃逸分析结果？

```bash
# 方法 1：go build 加 -gcflags
go build -gcflags="-m" ./...

# 方法 2：更详细，显示逃逸原因
go build -gcflags="-m -m" ./...

# 方法 3：只看逃逸，不编译
go build -gcflags="-m -l" ./...   # -l 禁止内联，更清晰
```

**输出示例：**
```
./main.go:5:6: &x escapes to heap       # x 逃逸到堆
./main.go:10:6: y does not escape       # y 不逃逸，在栈上
```

---

## 四、六大逃逸场景（面试必背！）

### 场景 1：返回指针 ⭐⭐⭐⭐⭐

```go
// ❌ 逃逸：返回局部变量指针
func NewUser() *User {
    u := User{Name: "alice"}  // 逃逸到堆！
    return &u
}

// ✅ 不逃逸：返回值
func NewUserValue() User {
    u := User{Name: "alice"}  // 栈上分配
    return u                  // 值拷贝
}
```

**分析：** 函数返回后，栈帧销毁，`&u` 指向的内存必须存活，所以 `u` 逃逸到堆。

---

### 场景 2：发送到 channel ⭐⭐⭐⭐

```go
// ❌ 逃逸：发送到 channel
func sendToCh(ch chan *Data) {
    d := &Data{ID: 1}  // 逃逸到堆！
    ch <- d             // d 被发送到 channel，接收方可能在另一个 goroutine
}

// ✅ 不逃逸：channel 传值
func sendValueToCh(ch chan Data) {
    d := Data{ID: 1}   // 可能不逃逸
    ch <- d
}
```

**分析：** channel 的接收方可能在另一个 goroutine，编译器无法确定 `d` 何时被消费，所以必须堆分配。

---

### 场景 3：闭包引用 ⭐⭐⭐⭐⭐

```go
// ❌ 逃逸：闭包捕获变量
func Counter() func() int {
    count := 0           // 逃逸到堆！
    return func() int {
        count++          // 闭包引用了 count
        return count
    }
}
```

**分析：** 闭包可能在函数返回后很久才被调用，`count` 必须在堆上存活。

---

### 场景 4：interface 类型 ⭐⭐⭐⭐⭐

```go
// ❌ 逃逸：赋值给 interface
func printAny(v interface{}) {
    fmt.Println(v)  // v 逃逸到堆！
}

// 具体例子
func demo() {
    x := 42         // 本来在栈上，但...
    fmt.Println(x)  // fmt.Println 参数是 interface{}，x 逃逸到堆！
}
```

**分析：** `interface{}` 内部用指针存储值，编译器无法确定值的生命周期，必须堆分配。

**这就是为什么 `fmt.Println` 总是导致逃逸！**

---

### 场景 5：slice 扩容 / append ⭐⭐⭐⭐

```go
// ❌ 逃逸：slice 容量不确定，可能扩容
func growSlice() []int {
    var s []int       // 逃逸！
    for i := 0; i < 1000; i++ {
        s = append(s, i)  // append 可能扩容，底层数组地址变了
    }
    return s
}

// ✅ 不逃逸：预分配容量
func fixedSlice() []int {
    s := make([]int, 0, 1000)  // 预分配，可能不逃逸
    for i := 0; i < 1000; i++ {
        s = append(s, i)
    }
    return s  // 注意：返回 slice 本身还是会逃逸
}

// ✅ 真正不逃逸：不返回，只在函数内使用
func localSlice() int {
    s := make([]int, 1000)  // 栈上分配
    s[0] = 42
    return s[0]             // 返回值，不逃逸
}
```

---

### 场景 6：栈空间不足 ⭐⭐⭐

```go
// ❌ 逃逸：栈帧太大
func bigAlloc() {
    // 超过一定大小（通常 64KB），编译器直接分配到堆
    _ = make([]byte, 100*1024)  // 100KB，逃逸到堆！
}

// ✅ 不逃逸：小的栈分配
func smallAlloc() {
    _ = make([]byte, 100)  // 100 字节，栈上分配
}
```

**阈值：** Go 编译器通常以 64KB 为界，超过则堆分配。

---

## 五、逃逸分析核心规则总结

| 场景 | 是否逃逸 | 原因 |
|------|---------|------|
| 返回局部变量指针 | ✅ 逃逸 | 栈帧销毁后指针失效 |
| 发送到 channel | ✅ 逃逸 | 接收方不确定何时消费 |
| 闭包捕获变量 | ✅ 逃逸 | 闭包可能在函数返回后执行 |
| 赋值给 interface{} | ✅ 逃逸 | interface 内部用指针存储 |
| fmt.Println 等函数 | ✅ 逃逸 | 参数是 interface{} |
| slice/map 动态扩容 | ✅ 逃逸 | 底层数组地址可能变化 |
| 栈空间 > 64KB | ✅ 逃逸 | 栈空间有限 |
| 返回值类型 | ❌ 不逃逸 | 值拷贝，栈上安全 |
| 函数内局部使用 | ❌ 不逃逸 | 编译器能确定生命周期 |
| 编译期确定大小的 slice | ❌ 不逃逸 | 小的可以在栈上分配 |

---

## 六、实战：如何优化逃逸？

### 优化 1：返回值代替返回指针

```go
// ❌ 逃逸
func getUser() *User {
    return &User{Name: "alice"}  // 堆分配
}

// ✅ 不逃逸
func getUser() User {
    return User{Name: "alice"}   // 栈分配，值拷贝
}
```

**什么时候用指针？**
- 结构体很大（> 64 字节），拷贝开销大 → 用指针
- 结构体很小（< 64 字节），拷贝开销小 → 用值
- 需要修改原对象 → 用指针

---

### 优化 2：避免 interface{} 参数

```go
// ❌ 逃逸
func log(msg interface{}) {
    fmt.Println(msg)  // 逃逸
}

// ✅ 不逃逸
func logString(msg string) {
    fmt.Println(msg)  // string 也逃逸... 因为 fmt.Println 参数是 interface{}
}

// ✅ 真正不逃逸
func logString(msg string) {
    os.Stdout.WriteString(msg + "\n")  // 不经过 interface{}
}
```

---

### 优化 3：预分配 slice/map

```go
// ❌ 可能逃逸
s := []int{}
for i := 0; i < 1000; i++ {
    s = append(s, i)
}

// ✅ 预分配
s := make([]int, 0, 1000)
for i := 0; i < 1000; i++ {
    s = append(s, i)
}
```

---

### 优化 4：小结构体用值传递

```go
type Point struct{ X, Y int }  // 16 字节

// ✅ 值传递，不逃逸
func distance(p1, p2 Point) float64 {
    // ...
}

type BigConfig struct {          // 几百字节
    Data [1024]byte
    Name string
    // ...
}

// ✅ 大结构体用指针
func processConfig(c *BigConfig) {
    // ...
}
```

---

### 优化 5：sync.Pool 复用堆对象

```go
var bufPool = sync.Pool{
    New: func() interface{} {
        return bytes.NewBuffer(make([]byte, 0, 1024))
    },
}

func process() {
    buf := bufPool.Get().(*bytes.Buffer)
    defer bufPool.Put(buf)
    buf.Reset()
    // 使用 buf...
}
```

---

## 七、逃逸分析 vs 内联

**内联（Inlining）是逃逸分析的前置优化。**

```
函数 A 调用函数 B，B 返回 &x
  ↓
如果没有内联：编译器只看 B 的签名，返回指针 → x 逃逸
  ↓
如果内联了：编译器看 A+B 的完整代码，可能发现 &x 没有逃出 A → 不逃逸！
```

**所以 `-l` 禁止内联后，逃逸会变多！**

```bash
# 正常编译（内联 + 逃逸分析）
go build -gcflags="-m" ./...

# 禁止内联（逃逸变多）
go build -gcflags="-m -l" ./...
```

---

## 八、面试高频问答

| 问题 | 答案 |
|------|------|
| 什么是逃逸分析？ | 编译器决定变量分配在栈还是堆的过程。 |
| 为什么要做逃逸分析？ | 栈分配零开销，堆分配需要 GC 回收，影响性能。 |
| 哪些情况会逃逸？ | 返回指针、channel 发送、闭包捕获、interface{}、动态扩容、大对象。 |
| fmt.Println 为什么导致逃逸？ | 参数是 interface{} 类型，编译器无法确定值的生命周期。 |
| 栈和堆分配的开销差多少？ | 栈：移动栈指针，几乎为零；堆：malloc + GC 扫描 + STW，差距 10~100 倍。 |
| 怎么查看逃逸分析结果？ | `go build -gcflags="-m"` |
| 返回值和返回指针哪个好？ | 小结构体（<64B）返回值，大结构体返回指针。 |
| 内联对逃逸分析有什么影响？ | 内联是逃逸分析的前置优化，内联后可能减少逃逸。 |
| 闭包为什么导致逃逸？ | 闭包捕获的变量可能在函数返回后被使用，必须堆分配。 |
| Go 的逃逸分析和 C++ 的区别？ | C++ 程序员手动决定栈/堆（new vs 局部变量），Go 编译器自动决定。 |

---

## 九、一句话总结

> 逃逸分析的核心思想：编译器尽可能把变量分配在栈上（零开销），只有当变量的引用可能"逃出"函数作用域时才分配到堆上（GC 回收）。记住六大逃逸场景，用 `go build -gcflags="-m"` 验证，优先用值传递、预分配、避免 interface{}，面试基本满分。
