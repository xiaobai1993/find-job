# string 与 []byte 底层原理与常见坑

---

## 一、先搞懂：底层结构到底是什么？

很多人以为 string 就是「字节数组」，其实不对，两者在 runtime 里是完全不同的结构体。

### 1. string 底层结构（2 个字段）
```go
// runtime/string.go
type StringStruct struct {
    Data unsafe.Pointer // 指向底层字节数组的指针
    Len  int            // 字符串长度
}
```
**只有指针 + 长度，没有容量！**

### 2. []byte 底层结构（3 个字段）
```go
// runtime/slice.go
type SliceHeader struct {
    Data unsafe.Pointer // 指向底层数组的指针
    Len  int            // 当前长度
    Cap  int            // 容量
}
```
**比 string 多了一个 Cap 字段。**

---

### 3. 核心区别对比

| 特性 | string | []byte |
|------|--------|--------|
| 字段数 | 2 个（Data + Len） | 3 个（Data + Len + Cap） |
| 可变性 | ❌ 不可变 | ✅ 可变 |
| 能不能比较 | ✅ == / != | ❌ 不能直接比较，只能比较元素 |
| 能不能当 map key | ✅ 可以 | ❌ 不可以（slice 不能比较） |
| GC 扫描 | 不需要扫描内部指针 | 需要扫描 |

---

## 二、为什么 string 要设计成不可变的？

这是 Go 最基础也最有深意的设计之一，三个核心原因：

### 1. **安全**
如果 string 是可变的，你拿到一个字符串参数，调用者可能在后面偷偷修改它，导致你这边的逻辑出问题，非常难排查。

```go
func process(s string) {
    // 你以为 s 是 "abc"
    // 如果字符串可变，调用者可能在另一个 goroutine 里把 s 改成了 "xyz"
}
```

不可变 = 线程安全，不用加锁就能在多个 goroutine 里安全共享。

### 2. **可哈希，适合当 map key**
如果字符串可变，作为 map key 放进去之后被修改了，就再也找不到这个 key 了，整个哈希表就乱了。

不可变保证了哈希值永远不变，所以 string 是最常用的 map key 类型。

### 3. **内存优化：多个 string 可以共享同一个底层数组**
```go
s := "hello world"
s1 := s[:5]  // "hello"，和 s 共享同一个底层数组！
s2 := s[6:]  // "world"，也共享同一个底层数组！
```
三个 string 共享同一个底层字节数组，只需要存指针和长度，不需要拷贝内存，非常省。

**如果字符串是可变的，就不敢这么优化了——改了 s1，s 和 s2 也会跟着变。**

---

## 三、string ↔ []byte 转换到底有没有拷贝？

这是面试最高频的问题之一，答案是：**默认有拷贝，但是有三种场景可以零拷贝。**

---

### 场景 1：普通转换 → 100% 有拷贝
```go
s := "hello"
b := []byte(s) // ✅ 有拷贝！分配了新的数组
s2 := string(b) // ✅ 有拷贝！又分配了新的数组
```

为什么必须拷贝？
- string 是不可变的，[]byte 是可变的
- 如果不拷贝，你修改了 `b`，`s` 的值也会跟着变，就破坏了 string 的不可变承诺

**拷贝的成本：** O(n)，n 是字符串长度。小字符串无所谓，大字符串（比如几 MB 的日志）频繁转换会有明显的性能开销和 GC 压力。

---

### 场景 2：unsafe 零拷贝转换 → 没有拷贝（高性能场景用）
```go
// 零拷贝：[]byte → string
func BytesToString(b []byte) string {
    return *(*string)(unsafe.Pointer(&b))
}

// 零拷贝：string → []byte（注意：返回的 []byte 绝对不能修改！）
func StringToBytes(s string) []byte {
    return *(*[]byte)(unsafe.Pointer(&struct {
        string
        int
    }{s, len(s)}))
}
```

**原理：** 直接把 slice 的结构体指针强转成 string 的结构体指针，两个类型只差一个 Cap 字段，内存布局兼容。

**大坑！** 零拷贝转换得到的 []byte **绝对不能修改**，因为底层数组是 string 的只读内存，修改会直接 panic：
```go
s := "hello"
b := StringToBytes(s)
b[0] = 'H' // ❌ 直接 panic：unexpected fault address
```

**什么时候用？** 高并发、大字符串、且只读的场景，比如日志库、JSON 解析库、高性能网络库，很多开源库内部都是这么写的。

---

### 场景 3：Go 编译器自动优化的零拷贝场景（面试加分项）
这是 99% 的人不知道的细节，Go 编译器在特定场景下会自动优化，不产生拷贝：

#### 优化 1：map 查找时，用 []byte 当 key 不会拷贝
```go
m := make(map[string]int)
b := []byte("hello")

// ✅ 这里没有拷贝！编译器自动优化了
v := m[string(b)]
```
因为只是查找，不会修改 key，所以不需要拷贝。

**注意：** 赋值的时候还是会拷贝，因为 key 要存进 map：
```go
m[string(b)] = 1 // ❌ 这里还是有拷贝
```

#### 优化 2：for range 遍历 []byte 转 string 不会拷贝
```go
b := []byte("hello world")
for i, c := range string(b) { // ✅ 没有拷贝！
    // ...
}
```

#### 优化 3：某些字符串拼接场景
编译器会分析拼接的上下文，能复用底层数组就不拷贝。

---

## 四、90% 的人都踩过的坑

### 坑 1：for range 遍历 string 不是按 byte 遍历！
```go
s := "你好"
for i, c := range s {
    fmt.Printf("i=%d, c=%c\n", i, c)
}
// 输出：
// i=0, c=你
// i=3, c=好
```

**注意：** for range 遍历 string 是按 **rune** 遍历，不是按 byte！
- 中文 UTF-8 编码占 3 个字节，所以第二个字符的下标直接从 0 跳到了 3
- 如果你想按 byte 遍历，要用 for 循环：
```go
for i := 0; i < len(s); i++ {
    fmt.Printf("i=%d, b=%02x\n", i, s[i])
}
```

这是处理中文最常见的坑，很多人下标算错导致乱码。

---

### 坑 2：修改 []byte 会不会影响原 string？
普通转换不会，因为有拷贝；零拷贝转换会，但是修改会 panic。
```go
// 普通转换：互不影响
s := "hello"
b := []byte(s)
b[0] = 'H'
fmt.Println(s) // 还是 "hello"，不变

// 零拷贝转换：千万不要改！
b2 := StringToBytes(s)
b2[0] = 'H' // ❌ panic
```

---

### 坑 3：空字符串 vs 零值 []byte
```go
var s string     // 空字符串，Data = nil，Len = 0
var b []byte     // nil slice，Data = nil，Len = 0，Cap = 0

len(s) == 0  // true
len(b) == 0  // true

// 但是！
s == ""      // true
b == nil     // true
```

两者都是零值，底层指针都是 nil，但类型不一样。

---

### 坑 4：大量转换导致 GC 压力
```go
// ❌ 糟糕的代码：每次循环都做一次 string ↔ []byte 转换，产生大量 GC 对象
for i := 0; i < 1000000; i++ {
    s := getLargeString() // 1MB 的字符串
    b := []byte(s)        // 每次都拷贝 1MB，循环 100 万次就是 1TB！
    process(b)
}
```

**解决：** 如果 process 只是只读，用零拷贝转换；或者尽量在业务层面避免来回转换。

---

### 坑 5：string 和 []byte 的传参成本
```go
func processString(s string)    // 传参只拷贝 16 字节（2 个 int）
func processBytes(b []byte)     // 传参只拷贝 24 字节（3 个 int）
```
两者传参成本都极低，因为拷贝的只是结构体头，不是整个底层数组。很多人以为传大字符串会很慢，其实不会。

---

## 五、最佳实践

1. **默认用 string**：大多数场景下用 string 更安全，不可变、能当 map key、有编译器优化
2. **需要修改的时候才用 []byte**：比如拼接、修改、解析二进制协议
3. **零拷贝转换慎用**：只在性能瓶颈、确认只读的场景用，并且一定要写注释告诉调用者不能修改
4. **避免来回转换**：`string → []byte → string → []byte` 这种代码非常伤性能，能在同一个类型里做完就不要来回转
5. **大字符串拼接用 strings.Builder**：不要用 `+` 拼接大字符串，每次都会产生新的拷贝

---

## 六、面试高频问答

| 问题 | 答案 |
|------|------|
| string 底层结构是什么？ | 2 字段结构体：指向底层字节数组的指针 + 长度，没有容量。 |
| []byte 底层结构是什么？ | 3 字段结构体：指针 + 长度 + 容量，和 slice 一样。 |
| string 为什么设计成不可变的？ | 三个原因：1) 线程安全，不用加锁就能共享；2) 可哈希，适合当 map key；3) 多个 string 可以共享同一个底层数组，节省内存。 |
| string ↔ []byte 转换有没有拷贝？ | 默认有拷贝，因为要保证 string 不可变。三种场景零拷贝：1) unsafe 强转；2) map 查找时的 []byte 转 string；3) for range 遍历时编译器优化。 |
| 零拷贝转换得到的 []byte 能修改吗？ | 绝对不能！底层数组是 string 的只读内存，修改会直接 panic。 |
| for range 遍历 string 是按 byte 遍历吗？ | 不是，是按 rune（Unicode 码点）遍历，中文占 3 个字节，下标会跳。 |
| string 能和 nil 比较吗？ | 不能，string 是值类型，零值是 ""，不是 nil。 |
| []byte 能当 map key 吗？ | 不能，slice 不能比较，所以不能当 map key。 |
| 大字符串传参性能开销大吗？ | 不大，传参只拷贝 16 字节的结构体头，不会拷贝整个底层数组。 |
| 怎么判断字符串是空的？`len(s) == 0` 还是 `s == ""`？ | 一样，编译器会把 `s == ""` 优化成 `len(s) == 0`，性能没有区别。 |

---

## 七、一句话总结
> string 是「只读的字节切片头」，只有指针和长度，不能修改，传参只拷 16 字节，零拷贝转换性能极高但绝对不能改。