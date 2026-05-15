# Go 内存逃逸分析深度解析

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
