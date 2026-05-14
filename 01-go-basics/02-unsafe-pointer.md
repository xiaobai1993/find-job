# unsafe.Pointer 详解

---

## 一、unsafe.Pointer 是什么？

Go 是强类型语言，普通指针（`*int`、`*string`）不能互相转换，也不能做算术运算。

`unsafe.Pointer` 是 Go 提供的「万能指针」，打破了这些限制：
1. **可以和任意类型的指针互相转换**
2. **可以和 uintptr 互相转换**（uintptr 是整数，能做算术运算）

本质上，`unsafe.Pointer` 就是一个存内存地址的整数，编译器不做类型安全检查。

> 为什么叫 unsafe？因为用了它就绕过了 Go 的类型安全保障，写错了直接内存破坏、panic，甚至静默出问题，非常难调试。所以叫「不安全」。

---

## 二、4 种合法转换规则（官方定义）

### 规则 1：任意类型指针 ↔ unsafe.Pointer
```go
var x int = 10
p := unsafe.Pointer(&x)        // *int → Pointer
y := (*float64)(p)             // Pointer → *float64（强行把 int 当 float64 读）
```
这就是「类型强转」，不同类型指针互相转。

### 规则 2：unsafe.Pointer ↔ uintptr
```go
p := unsafe.Pointer(&x)
addr := uintptr(p)             // Pointer → 整数地址
p2 := unsafe.Pointer(addr)     // 整数地址 → Pointer
```
uintptr 只是个整数，**不是指针**，GC 不会认为它引用了对象。

### 规则 3：uintptr 做运算后必须立刻转回 Pointer
```go
// ✅ 正确：转换和运算在同一个表达式里完成
p := unsafe.Pointer(&arr[0])
next := unsafe.Pointer(uintptr(p) + unsafe.Sizeof(arr[0]))

// ❌ 错误：把 uintptr 存到变量里，中间 GC 可能把原对象回收了
addr := uintptr(p)             // 这一步之后 p 没人用了，GC 可能回收对象
// ... 中间可能 GC ...
next := unsafe.Pointer(addr)   // addr 指向的内存可能已经被释放了！
```
**这是最常见的坑**：uintptr 只是个数字，不会持有对象引用。

### 规则 4：可以获取结构体字段的指针（通过偏移量）
```go
type User struct {
    Name string
    Age  int
}

u := User{Name: "Alice", Age: 18}
p := unsafe.Pointer(&u)
ageP := (*int)(unsafe.Pointer(uintptr(p) + unsafe.Offsetof(u.Age)))
*ageP = 20  // 直接修改 Age 字段
fmt.Println(u.Age)  // 20
```
反射底层就是这么实现的。

---

## 三、5 个典型使用场景

### 场景 1：零拷贝类型转换（性能优化）
最常用的场景：[]byte 和 string 互相转换，不拷贝内存。

```go
// 标准实现，很多高性能库都这么写
func BytesToString(b []byte) string {
    return *(*string)(unsafe.Pointer(&b))
}

func StringToBytes(s string) []byte {
    return *(*[]byte)(unsafe.Pointer(&struct {
        string
        int
    }{s, len(s)}))
}
```
**为什么快？** 正常 `string(b)` 会把底层数组拷贝一份，用 unsafe 直接复用同一个数组，零拷贝。

**风险**：转换后不能修改 []byte，因为 string 是不可变的，改了会 panic。

---

### 场景 2：访问结构体私有字段
可以绕过大小写限制，访问别的包的私有字段。

```go
// 假设别的包定义了：
// type User struct {
//     name string  // 私有！
// }

u := somepackage.NewUser()
p := unsafe.Pointer(&u)
nameP := (*string)(unsafe.Pointer(uintptr(p) + 16)) // 假设 name 偏移 16 字节
fmt.Println(*nameP)  // 读到了私有字段！
```
**注意**：结构体字段顺序可能随 Go 版本、编译参数变化，写死偏移量非常脆弱。

---

### 场景 3：指针运算（绕过边界检查）
遍历数组时，绕过 Go 的边界检查，提升性能。

```go
arr := []int{1, 2, 3, 4, 5}
base := unsafe.Pointer(&arr[0])
size := unsafe.Sizeof(arr[0])

for i := 0; i < len(arr); i++ {
    // 直接算地址，不用 arr[i]，没有边界检查
    p := unsafe.Pointer(uintptr(base) + uintptr(i)*size)
    fmt.Println(*(*int)(p))
}
```
高性能场景才会这么写，正常代码没必要。

---

### 场景 4：和 CGO / 系统调用交互
C 语言的 `void*` 对应 Go 的 `unsafe.Pointer`，跨语言调用必须用它。

```go
// 调用 C 函数
// void* malloc(size_t size);
ptr := C.malloc(C.size_t(1024))
// ptr 类型是 unsafe.Pointer
```
系统调用传参数也经常需要把指针转成 uintptr。

---

### 场景 5：读写未导出的 runtime 结构
比如你想知道 slice 真正的底层数组地址：

```go
s := []int{1, 2, 3}
hdr := (*reflect.SliceHeader)(unsafe.Pointer(&s))
fmt.Printf("底层数组地址: %x\n", hdr.Data)
fmt.Printf("len: %d, cap: %d\n", hdr.Len, hdr.Cap)
```
我们之前讲 slice 底层结构时，就是这么看真实字段的。

---

## 四、常见坑 & 注意事项

### 坑 1：uintptr 不会阻止 GC 回收对象
```go
// ❌ 错误写法
p := &Obj{}
addr := uintptr(unsafe.Pointer(p))
// 这里 p 已经没有引用了，GC 可能把 Obj 回收了
// 后面再用 addr 就是访问已释放的内存！
runtime.GC()
obj := (*Obj)(unsafe.Pointer(addr))  // 崩溃！
```

**解决**：不要把 uintptr 存到变量里，转换和运算必须在同一个表达式完成。

---

### 坑 2：内存对齐问题
不是所有类型都能随便转，比如把 `*int` 转成 `*int64` 在 32 位系统上可能因为对齐问题崩溃。

```go
var x int32 = 10
p := unsafe.Pointer(&x)
y := (*int64)(p)  // ❌ x 只有 4 字节，读 8 字节会越界！
```

---

### 坑 3：破坏内存安全，静默出错
用了 unsafe 之后，编译器不帮你做任何检查，写错了不会报错，只会：
- 直接 panic
- 读到垃圾值
- 静默修改了不相关的内存，程序逻辑错乱（最难调试）

比如：算偏移量算错了，写的时候把旁边的字段覆盖了，你可能很久之后才发现。

---

### 坑 4：Go 版本升级可能不兼容
runtime 的结构体布局、字段顺序、扩容策略都可能变，你写的 unsafe 代码在 Go 1.19 能用，Go 1.20 可能直接炸。

---

## 五、什么时候不该用 unsafe？

**99% 的场景你都不需要 unsafe！**

不要用的情况：
1. ✗ 只是想做类型转换，没有性能瓶颈
2. ✗ 觉得「这样写很酷」
3. ✗ 不想写几行拷贝代码
4. ✗ 为了省一点点内存

必须用的情况：
1. ✓ 极致性能优化，基准测试证明拷贝确实是瓶颈
2. ✓ 和 C 语言 / 系统调用交互
3. ✓ 实现反射、序列化框架等底层库
4. ✓ 调试、看底层结构（就像我们之前看 slice header）

---

## 六、面试高频问答

| 问题 | 答案 |
|------|------|
| unsafe.Pointer 和 uintptr 区别？ | Pointer 是指针类型，GC 会认为它引用了对象；uintptr 只是个整数，GC 不管。Pointer 不能做运算，uintptr 可以。 |
| 为什么 uintptr 转 Pointer 不能分两步？ | 中间 GC 可能把原对象回收了，uintptr 变成野指针。必须在同一个表达式里完成转换和运算。 |
| 用 unsafe 有什么风险？ | 破坏类型安全、内存越界、野指针、GC 回收导致崩溃、版本升级不兼容。出了问题非常难调试。 |
| unsafe 真的不安全吗？ | 对编译器来说是不安全的（不做检查），但只要你用对了，就是安全的。标准库大量用 unsafe。 |
| 为什么 string 转 []byte 用 unsafe 更快？ | 正常转换会拷贝整个底层数组，O(n)；unsafe 只是复制结构体的 3 个字段，O(1)，零拷贝。 |
| 零拷贝转换后能修改吗？ | 不能！string 是不可变的，修改对应的 []byte 会导致未定义行为，可能直接 panic。 |

---

## 七、总结

`unsafe.Pointer` 是一把非常锋利的手术刀：
- 用对了能获得极致性能，解决普通 Go 代码解决不了的问题
- 用错了就是定时炸弹，不知道什么时候炸

核心原则：**能不用就不用，不得不用的时候，尽量少用，集中使用，写好注释。**
