# range 底层原理与常见坑

---

## 一、先搞懂：range 到底是什么？

range 是 Go 语言专门为「可迭代类型」设计的语法糖。

**Go 1.22 之前**：可以遍历数组、slice、string、map、channel

**Go 1.22 新增**：支持遍历整数（`for i := range 10`）+ 循环变量作用域修复

**Go 1.23 新增**：支持遍历函数迭代器（range over func）

看起来简单，但是坑非常多，90% 的 Go 开发者都踩过至少一个。

---

## 二、range 核心规则（10 条，含新版本特性）

### 规则 1：range 返回的是**值拷贝**，不是引用（最大的坑！）

```go
nums := []int{1, 2, 3}
for _, v := range nums {
    v *= 10  // ❌ 修改的是拷贝的值，不是原数组元素！
}
fmt.Println(nums)  // 还是 [1 2 3]，不是 [10 20 30]！
```

**这是 90% 的人都踩过的坑！**

range 遍历的时候，第二个返回值是「元素的值拷贝」，不是元素的引用。修改它不会影响原数组。

**✅ 正确写法：用下标访问原数组**
```go
for i := range nums {
    nums[i] *= 10  // 直接修改原数组元素
}
fmt.Println(nums)  // [10 20 30]
```

---

### 规则 2：遍历变量的地址永远是同一个（超级经典的坑！）

```go
nums := []int{1, 2, 3}
var addrs []*int

for _, v := range nums {
    addrs = append(addrs, &v)  // ❌ 取的是遍历变量 v 的地址，不是元素的地址！
}

// 打印结果
for _, p := range addrs {
    fmt.Print(*p, " ")
}
// 输出：3 3 3！不是 1 2 3！
```

**为什么？** 因为整个 for range 循环过程中，`v` 是同一个变量，只是每次循环把元素的值拷贝进去。所以 `&v` 永远是同一个地址，最后这个地址里存的是最后一个元素的值 3。

**✅ 正确写法 1：用下标取原元素地址**
```go
for i := range nums {
    addrs = append(addrs, &nums[i])  // 取原数组元素的地址
}
```

**✅ 正确写法 2：在循环内新建变量**
```go
for _, v := range nums {
    v := v  // 每次循环新建一个局部变量
    addrs = append(addrs, &v)
}
```

**这个坑在 goroutine 里更致命！**
```go
// ❌ 错误写法：所有 goroutine 共享同一个 v，最后都打印 3
for _, v := range nums {
    go func() {
        fmt.Println(v)
    }()
}

// ✅ 正确写法：把 v 作为参数传进去
for _, v := range nums {
    go func(v int) {
        fmt.Println(v)
    }(v)
}
```

**支付业务特别提醒：** 这是生产环境最常见的并发 bug 之一，一定要警惕！

---

### 规则 3：遍历 string 的时候，返回的是 rune，不是 byte

```go
s := "你好世界"

for i, c := range s {
    fmt.Printf("i=%d, c=%c\n", i, c)
}
```

**输出：**
```
i=0, c=你
i=3, c=好
i=6, c=世
i=9, c=界
```

**注意：**
- 下标 `i` 不是连续的，中文 UTF-8 占 3 个字节
- `c` 的类型是 rune（int32），不是 byte
- 遇到非法 UTF-8 编码，会返回 U+FFFD（替换字符），不会 panic

---

### 规则 4：遍历 map 的顺序是随机的（面试必问！）

```go
m := map[string]int{"a": 1, "b": 2, "c": 3}

for k, v := range m {
    fmt.Printf("%s=%d ", k, v)
}
// 每次运行输出顺序都不一样！
// 可能是 a=1 b=2 c=3，也可能是 c=3 b=2 a=1
```

**为什么要设计成随机？**
- Go 1 早期版本的 map 遍历是稳定的，结果很多人依赖这个顺序
- 后来 Go 团队故意改成随机的，就是为了让大家不要依赖 map 遍历顺序
- 因为 map 扩容的时候，元素会重新哈希，顺序本来就不稳定

**面试高频追问：为什么要故意随机？**
- 避免开发者依赖遍历顺序，写出脆弱的代码
- 不同版本、不同架构的 Go 实现，map 顺序本来就不一样
- 随机可以暴露更多潜在的 bug

---

### 规则 5：遍历 channel 会一直阻塞，直到 channel 关闭

```go
ch := make(chan int, 3)
ch <- 1
ch <- 2
ch <- 3

// ✅ 遍历 channel，直到 channel 关闭
for v := range ch {
    fmt.Println(v)
}
// 如果 channel 不关闭，这里会一直阻塞，最终死锁！
```

**正确用法：** 发送端发完之后关闭 channel
```go
go func() {
    for i := 1; i <= 3; i++ {
        ch <- i
    }
    close(ch)  // ✅ 发完一定要关闭！
}()

for v := range ch {
    fmt.Println(v)
}
// channel 关闭后，遍历自动结束
```

---

### 规则 6：遍历 nil 类型是安全的，不会 panic

```go
var s []int          // nil slice
var m map[string]int // nil map
var ch chan int      // nil channel

for range s {}   // ✅ 不会 panic，只是不执行
for range m {}   // ✅ 不会 panic
for range ch {}  // ❌ 这个会永久阻塞！不是 panic
```

**注意：** nil channel 遍历是永久阻塞，不是 panic。

---

### 规则 7：可以用 `_` 忽略不需要的值

---

### 规则 8：Go 1.22+ —— 每次迭代有自己的循环变量！（史诗级修复）

**这是 Go 1.22 最重要的变化，修复了 10 多年的老坑！**

**Go 1.21 及之前的行为（有坑）：**
```go
var prints []func()
for i := 0; i < 3; i++ {
    prints = append(prints, func() { println(i) })
}
for _, p := range prints {
    p()  // 输出：3 3 3！（所有闭包共享同一个 i）
}
```

**Go 1.22+ 的行为（修复了）：**
```go
var prints []func()
for i := 0; i < 3; i++ {
    prints = append(prints, func() { println(i) })
}
for _, p := range prints {
    p()  // 输出：0 1 2！（每次迭代有自己的 i）
}
```

**面试高频追问：为什么改了？**
- 这个 bug 存在了 10 多年，90% 的 Go 开发者都踩过
- Go 团队通过 poll 发现绝大多数开发者都认为新的行为才是"正确的"
- 旧行为导致了无数生产环境 bug，特别是 goroutine + 闭包的场景

---

### 规则 9：Go 1.22+ —— range 可以遍历整数

```go
// Go 1.22+ 支持，等价于 for i := 0; i < 10; i++
for i := range 10 {
    fmt.Println(i)  // 输出 0, 1, 2, ..., 9
}
```

**注意事项：**
- 整数必须 >= 0，如果 n <= 0，循环不执行
- 支持所有整数类型：int, int8, int16, int32, int64, uint 等
- 特别适合需要执行 N 次的场景：`for range 5 { doSomething() }`

---

### 规则 10：Go 1.23+ —— range 可以遍历函数迭代器（Range Over Func）

这是 Go 自 1.0 以来最大的语法变化之一，支持自定义迭代器。

**基础用法：**
```go
// 定义一个迭代器函数，接受一个 yield 函数
func count(n int) func(yield func(int) bool) {
    return func(yield func(int) bool) {
        for i := 0; i < n; i++ {
            if !yield(i) {  // yield 返回 false 表示调用者要终止
                return
            }
        }
    }
}

// 使用 range 遍历迭代器
for i := range count(5) {
    fmt.Println(i)  // 输出 0, 1, 2, 3, 4
}
```

**支持双返回值：**
```go
// 树的中序遍历迭代器
type Tree[K cmp.Ordered, V any] struct {
    left, right *Tree[K, V]
    key         K
    value       V
}

func (t *Tree[K, V]) Walk(yield func(key K, val V) bool) {
    if t == nil {
        return
    }
    t.left.Walk(yield)
    if !yield(t.key, t.value) {
        return
    }
    t.right.Walk(yield)
}

// 遍历树
for k, v := range t.Walk {
    fmt.Printf("%s = %d\n", k, v)
}
```

**斐波那契迭代器示例：**
```go
func fibo(yield func(int) bool) {
    f0, f1 := 0, 1
    for yield(f0) {
        f0, f1 = f1, f0+f1
    }
}

// 打印小于 1000 的斐波那契数
for x := range fibo {
    if x >= 1000 {
        break  // break 会导致 yield 返回 false
    }
    fmt.Printf("%d ", x)
}
// 输出：0 1 1 2 3 5 8 13 21 34 55 89 144 233 377 610 987
```

**底层原理：**
- 编译器把 `for v := range f` 翻译成：调用 f，传入一个编译器生成的 yield 函数
- yield 函数被调用时，执行一次循环体
- break 会让 yield 返回 false，迭代器收到 false 应该立即终止
- 迭代器返回后，循环结束

```go
// 只需要值，不需要下标
for _, v := range nums { ... }

// 只需要下标，不需要值
for i := range nums { ... }

// 只需要知道有多少个元素，不需要下标和值
count := 0
for range nums {
    count++
}
```

---

## 三、90% 的人都踩过的 7 个坑（含版本兼容注意事项）

### 坑 1：修改遍历变量的值，原数组不变

前面讲过了，不再重复。记住：**range 返回的是值拷贝，修改它没用**。

---

### 坑 2：取遍历变量的地址，都是同一个地址（Go 1.22+ 已修复！）

**⚠️ 重要版本兼容性提醒：**

| Go 版本 | for 循环变量 | range 循环变量 |
|---------|-------------|---------------|
| 1.21 及以前 | 全循环共享同一个变量 | 全循环共享同一个变量 |
| 1.22+ | **每次迭代有自己的变量** | **每次迭代有自己的变量** |

**Go 1.21 及以前（有坑）：**
```go
nums := []int{1, 2, 3}
var addrs []*int

for _, v := range nums {
    addrs = append(addrs, &v)  // 都是同一个地址！
}

// 输出：3 3 3
for _, p := range addrs {
    fmt.Print(*p, " ")
}
```

**Go 1.22+（已修复）：**
```go
nums := []int{1, 2, 3}
var addrs []*int

for _, v := range nums {
    addrs = append(addrs, &v)  // 每次迭代的 v 是新变量！
}

// 输出：1 2 3 ✅
for _, p := range addrs {
    fmt.Print(*p, " ")
}
```

**goroutine 场景同样修复了：**
```go
// Go 1.22+ 这样写没问题！每次 goroutine 捕获的是不同的 v
for _, v := range nums {
    go func() {
        fmt.Println(v)  // 正确输出 1, 2, 3（顺序不一定）
    }()
}
```

**面试高频追问：Go 1.22 是怎么修复的？**
- 每次循环迭代开始时，隐式声明一个新的循环变量
- 把上一次迭代的变量值拷贝给新变量
- 所以每次迭代的变量地址都不一样
- 闭包捕获的就是各自迭代的变量了

**⚠️ 兼容警告：** 如果你的代码依赖旧行为（比如故意用同一个变量的地址），升级到 Go 1.22 会出问题！不过这种情况极其罕见。

---

### 坑 3：遍历大 slice 性能问题

```go
type BigStruct struct {
    ID   int
    Data [1024]byte // 1KB 的大结构体
}

var slice []BigStruct // 10000 个元素，总共 10MB

// ❌ 错误写法：每次循环都拷贝 1KB 的结构体，10000 次就是 10GB 的拷贝！
for _, v := range slice {
    process(v)
}
```

**✅ 正确写法 1：用下标访问，不拷贝整个结构体**
```go
for i := range slice {
    process(&slice[i])  // 只拷贝指针，8 字节
}
```

**✅ 正确写法 2：直接存指针**
```go
var slice []*BigStruct // 存指针，遍历的时候只拷贝 8 字节
for _, v := range slice {
    process(v)
}
```

---

### 坑 4：遍历 map 的同时修改 map

```go
m := make(map[int]bool)
for i := 0; i < 10; i++ {
    m[i] = true
}

// ❌ 遍历的同时删除元素，结果不确定
for k := range m {
    if k%2 == 0 {
        delete(m, k)  // 遍历的时候修改 map
    }
}
```

**Go 是允许的，不会 panic，但是遍历的结果不确定**：
- 已经遍历过的 key 被删除了，没关系
- 还没遍历到的 key 被删除了，就不会再遍历到了
- 遍历的时候新增的 key，可能会遍历到，也可能不会

**最佳实践：** 如果需要批量修改 map，先把 key 拷贝到 slice 里，再遍历 slice 修改 map。

---

### 坑 5：遍历 channel 忘记关闭，死锁

前面讲过了，记住：**用 for range 遍历 channel，发送端必须关闭 channel，否则永远阻塞**。

---

### 坑 6：遍历数组的时候，整个数组会被拷贝一次

```go
arr := [1000000]int{} // 100 万个 int，4MB

// ❌ 遍历数组的时候，整个数组会被拷贝一次！4MB 的拷贝！
for _, v := range arr {
    _ = v
}
```

**为什么？** 因为 Go 里数组是值类型，range 会先把数组拷贝一份，再遍历拷贝的那份。

**✅ 正确写法：遍历数组的 slice，不拷贝整个数组**
```go
// 只拷贝数组头，24 字节，不拷贝元素
for _, v := range arr[:] {
    _ = v
}
```

---

## 四、range 底层原理（面试加分项）

### 1. range 遍历 slice/数组的本质

```go
// 你写的代码
for i, v := range s {
    ...
}

// 编译器翻译成的代码
len := len(s)
for i := 0; i < len; i++ {
    v := s[i]  // 这里做了一次值拷贝！
    ...
}
```

所以：
- 遍历过程中 slice 的长度变化不会影响遍历次数（因为 len 是一开始就保存的）
- `v` 是 `s[i]` 的拷贝，修改 `v` 不影响原数组

---

### 2. range 遍历 map 的本质

```go
// 你写的代码
for k, v := range m {
    ...
}

// 编译器翻译成的代码
it := runtime.mapiterinit(&m)  // 初始化迭代器，随机选一个 bucket 开始
for {
    k, v := runtime.mapiternext(&it)  // 取下一个元素
    if k == nil {
        break  // 遍历完了
    }
    ...
}
```

关键点：
- 迭代开始的时候会随机选一个 bucket 作为起点
- 所以每次遍历顺序都不一样
- 遍历过程中 map 扩容，迭代器会自动处理，但结果不确定

---

### 3. range 遍历 channel 的本质

```go
// 你写的代码
for v := range ch {
    ...
}

// 编译器翻译成的代码
for {
    v, ok := <-ch
    if !ok {
        break  // channel 关闭了，退出循环
    }
    ...
}
```

所以 `for range ch` 等价于不停的 `<-ch`，直到 channel 关闭。

---

## 五、面试高频问答（含新版本问题）

| 问题 | 答案 |
|------|------|
| range 遍历的时候修改元素值会生效吗？ | 不会，range 返回的是值拷贝，修改的是拷贝。要修改原数组需要用下标访问。 |
| for range 里取遍历变量的地址，为什么都是同一个？ | **Go 1.21 及以前：** 是的，整个循环共享一个变量。**Go 1.22+：** 不是了，每次迭代有自己的变量，地址都不一样。 |
| goroutine 里用 range 变量有什么坑？ | **Go 1.21 及以前：** 闭包捕获的是同一个遍历变量的引用，goroutine 执行的时候值可能已经变了。解决方法是把变量作为参数传进去。**Go 1.22+：** 这个坑已经被彻底修复了！ |
| Go 1.22 对循环变量做了什么改动？ | 每次迭代有自己的循环变量，而不是整个循环共享一个。彻底修复了闭包 + goroutine 的经典 bug。 |
| Go 1.22 循环变量改动的原理是什么？ | 每次迭代开始时隐式声明新变量，把上一次迭代的值拷贝进去，所以地址都不一样。 |
| Go 1.22+ range 可以遍历整数吗？ | 可以！`for i := range 10` 等价于 `for i := 0; i < 10; i++`，i 从 0 到 9。 |
| Go 1.23+ Range Over Func 是什么？ | range 支持遍历函数迭代器，迭代器函数接受一个 yield 函数，调用 yield 就产生一个迭代值。彻底改变了 Go 的自定义集合设计模式。 |
| Range Over Func 的 yield 返回 false 表示什么？ | 表示调用者要终止迭代（比如 break 了），迭代器应该立即返回。 |
| map 遍历顺序是稳定的吗？ | 不是，Go 故意设计成随机的，就是为了让大家不要依赖遍历顺序。 |
| 为什么 map 遍历要设计成随机的？ | 避免开发者依赖不稳定的顺序，map 扩容的时候元素会重新哈希，顺序本来就不确定。 |
| for range channel 什么时候结束？ | channel 关闭的时候自动结束。如果 channel 永远不关闭，会一直阻塞。 |
| 遍历 nil slice/map 会 panic 吗？ | 不会，只是不执行循环。但是遍历 nil channel 会永久阻塞。 |
| 遍历大数组有什么性能问题？ | 数组是值类型，range 会把整个数组拷贝一遍。大数组遍历应该转成 slice 再遍历。 |
| 遍历过程中 slice 扩容了，遍历次数会变吗？ | 不会，range 开始的时候就保存了 len，遍历过程中 slice 扩容 len 变大也不会影响遍历次数。 |
| 遍历 map 的同时修改 map 会怎么样？ | 不会 panic，但是结果不确定。新增的 key 可能遍历到也可能不会，删除的 key 如果还没遍历到就不会再遍历了。 |

---

## 六、版本演进时间线

| Go 版本 | 变化内容 |
|---------|---------|
| 1.0 | range 支持 array, slice, string, map, channel |
| 1.22 | 🎯 **修复 10 年 bug**：每次迭代有自己的循环变量<br>🎯 **新增**：range 支持遍历整数 |
| 1.23 | 🎯 **重大特性**：Range Over Func，支持自定义迭代器 |

---

## 七、一句话总结

> **经典规则**：range 是语法糖，返回值拷贝；大结构体用下标访问，大数组转 slice 再遍历；map 顺序随机，channel 遍历必须关闭。<br><br>**新版本（1.22+）**：循环变量 bug 已修复，goroutine 里可放心用；新增整数遍历、函数迭代器，写代码更优雅。
