# Slice 底层原理与常见坑

来源：《Go 专家编程》+ 源码分析 + 面试高频考点

---

## 一、先搞懂：数组 vs 切片

| 特性 | 数组（array） | 切片（slice） |
|------|--------------|--------------|
| 长度 | 固定，是类型的一部分 `[3]int` 和 `[4]int` 是不同类型 | 动态，不是类型的一部分 `[]int` |
| 内存布局 | 连续的栈内存（如果没逃逸） | 是一个结构体，包含 3 个字段 |
| 赋值/传参 | 值拷贝，整个数组完整复制 | 值拷贝，只复制结构体的 3 个字段，不复制底层数组 |
| 能不能扩容 | 不能 | 能，超过容量时自动分配新数组 |

**核心误解**：很多人以为 slice 是引用类型 — 不对！Go 里所有类型传参都是值传递，slice 也不例外。只是复制的 slice 结构体里存的是「底层数组的指针」，所以通过新 slice 改元素，原 slice 也能看到变化。

---

## 二、slice 底层结构

slice 本质就是一个 3 字段的结构体，在 runtime 里定义：

```go
type slice struct {
    array unsafe.Pointer  // 指向底层数组的指针
    len   int             // 当前元素个数
    cap   int             // 底层数组总容量
}
```

就这 3 个字段，一共 24 字节（64位系统），非常轻量。

所以你写 `var s []int`，实际上就是这么个结构体：
- `array = nil`
- `len = 0`
- `cap = 0`

---

## 三、函数传参时到底发生了什么？

这是最容易头大的地方，用一个例子讲清楚：

```go
func modify(s []int) {
    s[0] = 100         // ✅ 改元素，外面能看到
    s = append(s, 2)   // ❌ append 扩容了，外面看不到！
}

func main() {
    s := []int{1}      // len=1, cap=1
    modify(s)
    fmt.Println(s)     // 输出 [100]，不是 [100, 2]
}
```

**为什么会这样？**

1. 函数参数 `s` 是原 slice 的**值拷贝** — 两个 slice 结构体不一样，但 `array` 指针指向同一个底层数组
2. `s[0] = 100` — 改的是同一个底层数组，所以外面能看到
3. `append(s, 2)` — 原 cap=1 不够，触发扩容，分配了新的底层数组，`s` 结构体里的 `array` 指针变了！
4. 但外面的原 slice 结构体还是老的，指针没变，所以看不到 append 的结果

**面试一句话总结**：改元素外面能看到，因为底层数组是同一个；append 扩容了外面看不到，因为指针变了，而且参数是值拷贝。

---

## 四、扩容规则

Go 1.18 之后的扩容策略：

1. **新容量 < 256** → 直接容量翻倍
2. **新容量 >= 256** → 每次增长 25%（`newcap = oldcap + oldcap/4`）

```go
s := make([]int, 0)
// len=0, cap=0 → append 1个 → cap=1
// len=1, cap=1 → append → cap=2
// len=2, cap=2 → append → cap=4
// len=4, cap=4 → append → cap=8
// ... 一直翻倍到 256
// len=256, cap=256 → append → cap=320（256*1.25）
```

**重点**：扩容一定会分配新的底层数组，原数组不会变。所以函数里 append 触发扩容了，外面的 slice 绝对感知不到。

---

## 五、切片共享底层数组的坑

两个 slice 切自同一个数组时，改其中一个可能影响另一个：

```go
a := [5]int{1, 2, 3, 4, 5}
s1 := a[1:3]  // [2, 3], len=2, cap=4（从下标1到数组末尾）
s2 := a[2:4]  // [3, 4], len=2, cap=3

s1[1] = 100   // 改的是同一个数组
fmt.Println(a)    // [1, 2, 100, 4, 5]
fmt.Println(s2)   // [100, 4] — 被影响了！
```

**更隐蔽的坑**：切片 append 没扩容时，会覆盖后面的元素！

```go
a := [5]int{1, 2, 3, 4, 5}
s1 := a[1:3]  // [2, 3], len=2, cap=4

// s1 还有容量，append 不会扩容，直接写到原数组后面
s1 = append(s1, 999)

fmt.Println(a)  // [1, 2, 3, 999, 5] — 原数组第4位被覆盖了！
```

这就是为什么函数里改 slice 元素外面能看到 — 因为底层数组是共享的。

---

## 六、nil slice vs 空 slice

| 类型 | 定义 | array 指针 | len | cap | 用途 |
|------|------|-----------|-----|-----|------|
| nil slice | `var s []int` | nil | 0 | 0 | 表示不存在、零值 |
| 空 slice | `s := []int{}` 或 `make([]int, 0)` | 指向空数组 | 0 | 0 | 表示空集合 |

```go
var s1 []int          // nil slice
s2 := []int{}         // 空 slice
s3 := make([]int, 0)  // 空 slice

fmt.Println(len(s1), len(s2), len(s3))  // 都是 0
fmt.Println(cap(s1), cap(s2), cap(s3))  // 都是 0

// json 序列化时不一样
json.Marshal(s1)  // null
json.Marshal(s2)  // []
```

**面试高频**：问两者区别，答出 JSON 序列化的差异就是加分项。

---

## 七、copy 的用法

`copy(dst, src []T)` 是内置函数，专门用来复制 slice，**会分配新的底层数组，两个 slice 不再共享**。

### 基本用法
```go
src := []int{1, 2, 3}
dst := make([]int, len(src))  // 注意：dst 必须先分配好长度！
n := copy(dst, src)           // n = 3，复制了 3 个元素

dst[0] = 100
fmt.Println(src)  // [1 2 3] — 原 slice 不受影响！
fmt.Println(dst)  // [100 2 3]
```

**重要**：`copy` 只会复制 `min(len(dst), len(src))` 个元素，dst 长度不够的话，后面的元素复制不过去。

---

### copy 的 4 个典型使用场景

#### 场景 1：切断子切片与原数组的关联
```go
big := []int{1, 2, 3, 4, 5}
small := make([]int, 3)
copy(small, big[:3])  // ✅ small 有自己独立的底层数组

small[0] = 999
fmt.Println(big)  // [1 2 3 4 5] — 原数组不受影响
```

#### 场景 2：从大切片截取小部分，让大数组能被 GC
```go
big := make([]int, 1000000)
small := big[:3]  // ❌ big 的底层数组不会被 GC，因为 small 还引用着

// ✅ 正确做法
small2 := make([]int, 3)
copy(small2, big[:3])
// 现在 big 没人用了，GC 可以回收整个大数组
```

#### 场景 3：函数返回时，不想让外部修改内部数组
```go
func getData() []int {
    internal := []int{1, 2, 3}
    result := make([]int, len(internal))
    copy(result, internal)
    return result  // ✅ 返回副本，外部怎么改都不影响内部
}
```

#### 场景 4：slice 删除元素（用 copy 覆盖）
```go
// 删除第 i 个元素
s := []int{1, 2, 3, 4, 5}
i := 2  // 删除 3
copy(s[i:], s[i+1:])  // 后面的元素往前覆盖
s = s[:len(s)-1]      // 长度减 1
fmt.Println(s)  // [1 2 4 5]
```
这是标准的删除元素写法，比 append 更高效（不扩容）。

---

### copy 的坑
```go
src := []int{1, 2, 3}
var dst []int  // nil slice，len=0
copy(dst, src) // ❌ 什么都不复制！因为 dst len=0
fmt.Println(dst)  // []，空的
```

**记住**：copy 前一定要用 make 给 dst 分配足够的长度，不是容量，是 len！

---

## 八、常见坑总结

### 坑 1：函数传参 append 了外面看不到
```go
func add(s []int) []int {
    s = append(s, 1)
    return s  // ✅ 必须把新 slice 返回出去
}
```
解决：函数修改 slice 后一定要 return，或者传指针 `*[]int`。

### 坑 2：子切片修改影响原数组/原切片
解决：如果不想共享，用 `copy` 或者 append 触发扩容（不推荐，依赖扩容时机太隐蔽）。

### 坑 3：大切片切出小切片，大数组无法被 GC
```go
big := make([]int, 1000000)
small := big[:3]  // small 还引用着 big 的底层数组
// big 没人用了，但因为 small 还在引用，整个大数组不会被 GC！
```
解决：需要保留小部分时，用 `copy` 复制出来，不要直接切。

### 坑 4：for range 循环里修改元素
```go
for _, v := range s {
    v = 100  // ❌ v 是值拷贝，改了没用
}

for i := range s {
    s[i] = 100  // ✅ 直接改数组元素才有用
}
```

---

## 八、面试高频问答

| 问题 | 答案 |
|------|------|
| slice 是引用类型吗？ | 不是，Go 没有引用类型。slice 是值类型，是一个包含指针+len+cap的结构体，传参是值拷贝。 |
| slice 和 array 区别？ | array 长度固定，是类型的一部分，传参完整拷贝；slice 长度动态，是结构体，传参只拷贝3个字段。 |
| 函数传 slice，改元素外面能看到吗？ | 能，因为底层数组是同一个，指针拷贝了。 |
| 函数里 append 了外面能看到吗？ | 不一定。没扩容且 len<cap 时，改的是原数组但外面 len 没变也看不到；扩容了指针变了完全看不到。总之，append 了就一定要 return。 |
| nil slice 和空 slice 区别？ | nil 的 array 指针是 nil，空 slice 指向空数组；len 和 cap 都是 0；JSON 序列化一个是 null 一个是 []。 |
| 扩容规则？ | Go 1.18+：<256 翻倍，>=256 每次增长 25%。 |
| 子切片会影响原切片吗？ | 会，共享底层数组。除非 append 触发扩容分配了新数组。 |
