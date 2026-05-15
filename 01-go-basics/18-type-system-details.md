# Go 类型系统细节：别名 vs 定义、可比较类型、相等性判断

---

## 一、类型别名 vs 类型定义

这是 Go 1.9 引入的，面试常考！

### 1. 类型定义（Type Definition）

```go
type MyInt int  // 定义一个新类型 MyInt，底层类型是 int
```

**MyInt 和 int 是完全不同的两个类型！**

```go
var a int = 10
var b MyInt = a  // ❌ 编译错误！类型不匹配！
var b MyInt = MyInt(a)  // ✅ 必须显式转换
```

新类型是全新的，所有方法都不会继承，底层类型一样也不能直接赋值。

---

### 2. 类型别名（Type Alias）

```go
type MyInt = int  // MyInt 只是 int 的别名，就是同一个东西！
```

**MyInt 和 int 是完全一样的类型！**

```go
var a int = 10
var b MyInt = a  // ✅ 完全没问题，就是同一个类型
```

别名只是换了个名字，本质完全一样，方法也一样，所有操作都一样。

---

### 3. 对比表

| 特性 | 类型定义 `type T int` | 类型别名 `type T = int` |
|------|-----------------------|-------------------------|
| 是新类型吗？ | ✅ 是全新的类型 | ❌ 就是原类型，换个名字 |
| 需要显式转换吗？ | ✅ 必须 | ❌ 不需要 |
| 继承方法吗？ | ❌ 不继承 | ✅ 完全一样 |
| 可以做类型断言吗？ | 是不同类型 | 是同一个类型 |

---

### 4. 类型别名用来干嘛的？

**场景 1：代码重构，大规模移动代码**

```go
// 把包 A 的类型移到包 B，不破坏老代码
package old

type User = new.User  // 老代码 import old.User 还能继续用
```

**场景 2：渐进式重构**
先把类型别名引进去，慢慢改代码，最后再改成类型定义。

---

## 二、可比较类型 vs 不可比较类型

Go 的类型按能不能做 `==` 比较分成两类。

### 1. 可比较类型

这些类型可以用 `==` 和 `!=`，也可以当 map 的 key：

| 类型 | 能不能比较 | 能不能当 map key |
|------|-----------|-----------------|
| bool | ✅ | ✅ |
| int/uint/float | ✅ | ✅ |
| string | ✅ | ✅ |
| 指针 | ✅ | ✅ |
| channel | ✅ | ✅ |
| interface | ✅ | ✅ |
| 结构体（所有字段都可比较） | ✅ | ✅ |
| 数组（元素可比较） | ✅ | ✅ |

**指针比较：**
```go
a := 1
b := &a
c := &a
fmt.Println(b == c)  // ✅ true，指向同一个地址
```

**结构体比较：**
```go
type Point struct { X, Y int }
p1 := Point{1, 2}
p2 := Point{1, 2}
fmt.Println(p1 == p2)  // ✅ true，所有字段相等
```

---

### 2. 不可比较类型

这些类型不能用 `==`，也不能当 map 的 key：

| 类型 | 能不能比较 | 能不能当 map key |
|------|-----------|-----------------|
| slice | ❌ | ❌ |
| map | ❌ | ❌ |
| function | ❌ | ❌ |

```go
s1 := []int{1, 2, 3}
s2 := []int{1, 2, 3}
fmt.Println(s1 == s2)  // ❌ 编译错误！slice 不能比较

// 唯一例外：可以和 nil 比较
fmt.Println(s1 == nil)  // ✅ 可以，只能和 nil 比
```

**为什么 slice/map/func 不能比较？**
- slice 有 ptr + len + cap，比较语义不明确（比较指针？还是比较内容？）
- Go 不想把这个问题复杂化，直接不让比较
- map 和 func 同理

---

### 3. 经典面试题：空 struct 可以比较吗？

```go
var a, b struct{}
fmt.Println(a == b)  // ✅ true！空 struct 是可比较的！
```

而且所有空 struct 的指针都指向同一个地址：
```go
fmt.Println(&a == &b)  // ✅ true！
```

这就是为什么用 `struct{}` 当 channel 信号零成本的原因。

---

## 三、相等性判断的坑

### 坑 1：interface 比较的陷阱！

**两个 interface 相等的条件：**
1. 动态类型完全一样
2. 动态值相等

```go
var a interface{} = 100    // type=int, value=100
var b interface{} = 100.0  // type=float64, value=100.0

fmt.Println(a == b)  // ❌ false！类型不一样！
```

虽然值看起来一样，但是类型不一样，所以不等。

---

### 坑 2：interface 和 nil 比较的超级大坑！

```go
// ❌ 经典错误！
var p *int = nil
var i interface{} = p

fmt.Println(i == nil)  // ❌ false！什么鬼？！
```

**为什么不是 nil？**

interface{} 内部是两个指针：(type, value)

- `i` 的 type 是 `*int`，不是 nil
- `i` 的 value 是 nil
- 只有 type 和 value 都是 nil 的时候，interface 才等于 nil

**这是 Go 最经典的坑，没有之一！**

**怎么判断 interface 是不是真的 nil？**
```go
import "reflect"

func isNil(i interface{}) bool {
    if i == nil {
        return true
    }
    v := reflect.ValueOf(i)
    switch v.Kind() {
    case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
        return v.IsNil()
    }
    return false
}
```

---

### 坑 3：浮点数比较

```go
fmt.Println(0.1 + 0.2 == 0.3)  // ❌ false！浮点数精度问题
```

所有语言都有这个问题，不要直接用 `==` 比较浮点数，要用差值小于某个 epsilon。

---

### 坑 4：结构体里有不可比较的字段

```go
type T struct {
    X []int  // slice 不可比较
}

t1 := T{X: []int{1}}
t2 := T{X: []int{1}}
fmt.Println(t1 == t2)  // ❌ 编译错误！结构体有不可比较字段，整个结构体都不能比较！
```

只要结构体里有一个不可比较的字段（slice/map/func），整个结构体就不能比较了。

---

## 四、类型转换 vs 类型断言

### 1. 类型转换（Type Conversion）

两个类型底层类型一样，互相转换：

```go
type MyInt int

var a int = 10
b := MyInt(a)  // ✅ 类型转换，编译时检查
```

**特点：**
- 编译时检查
- 不成功就编译错误
- 用于两个有相同底层类型的类型之间

---

### 2. 类型断言（Type Assertion）

对 interface 用，把 interface 转成具体类型：

```go
var i interface{} = 100

// 单返回值
a := i.(int)  // ✅ 对，a = 100
b := i.(string)  // ❌ 类型不对，直接 panic！

// 双返回值，安全
b, ok := i.(string)  // ✅ 安全，ok = false，不会 panic
```

**特点：**
- 运行时检查
- 单返回值不对就 panic
- 双返回值不会 panic，ok 表示成功与否
- 只能对 interface 用

---

### 3. 对比表

| 特性 | 类型转换 | 类型断言 |
|------|---------|---------|
| 什么时候用？ | 两个类型，底层类型相同 | 对 interface 用，转成具体类型 |
| 什么时候检查？ | 编译时 | 运行时 |
| 不成功怎么样？ | 编译错误 | panic（单返回值）或 ok=false（双返回） |

---

## 五、常见坑与最佳实践

### 坑 1：把类型别名当类型定义用

```go
// 你以为是定义新类型，实际上是别名
type MyInt = int

// 然后你想给 MyInt 加方法
func (m MyInt) Foo() {}  // ❌ 编译错误！不能给 int 加方法！
```

别名就是原类型，不能给原类型在别的包加方法。

---

### 坑 2：结构体里放了 slice 还想比较

```go
type User struct {
    Name string
    Tags []string  // slice 不可比较
}

u1 == u2  // ❌ 编译错误！
```

**解决：自己写 Equal 方法**
```go
func (u User) Equal(other User) bool {
    if u.Name != other.Name {
        return false
    }
    // 自己比较 slice
    if len(u.Tags) != len(other.Tags) {
        return false
    }
    for i := range u.Tags {
        if u.Tags[i] != other.Tags[i] {
            return false
        }
    }
    return true
}
```

或者用 `reflect.DeepEqual`（性能差一点）：
```go
reflect.DeepEqual(u1, u2)  // ✅ 可以比较任何东西
```

---

### 最佳实践总结

1. ✅ **需要全新类型用类型定义**，需要兼容旧代码用类型别名
2. ✅ **interface 和 nil 比较要小心**，type 非 nil 即使 value nil 也不等于 nil
3. ✅ **不要直接比较浮点数**，用差值小于 epsilon
4. ✅ **结构体不可比较就自己写 Equal 方法**，或者用 reflect.DeepEqual
5. ✅ **类型断言尽量用双返回值版本**，不要 panic
6. ❌ **不要拿 slice/map/func 用 == 比较**，除了和 nil 比
7. ❌ **不要滥用类型别名**，大部分场景你需要的是类型定义

---

## 六、面试高频问答

| 问题 | 答案 |
|------|------|
| 类型别名和类型定义的区别？ | 类型定义 `type T int` 是全新的类型，需要显式转换；类型别名 `type T = int` 就是原类型的另一个名字，完全一样。 |
| 哪些类型是可比较的？ | bool、数值、string、指针、channel、interface、所有字段都可比较的结构体、元素可比较的数组。 |
| 哪些类型不可比较？ | slice、map、function。只能和 nil 比较。 |
| interface 等于 nil 的条件？ | 必须动态类型和动态值都是 nil。只有 value 是 nil 不等于 nil。这是 Go 最经典的坑。 |
| 空 struct 可以比较吗？ | 可以，所有空 struct 都相等，而且指针也指向同一个地址。 |
| 类型转换和类型断言的区别？ | 类型转换编译时检查，用于相同底层类型的类型之间；类型断言运行时检查，用于把 interface 转成具体类型。 |
| 结构体里有 slice 还能比较吗？ | 不能，只要有一个不可比较的字段，整个结构体都不可比较。 |
| 怎么比较两个 slice 相等？ | 自己写循环比较，或者用 reflect.DeepEqual。 |
| 类型别名可以加方法吗？ | 如果是在同一个包定义的别名可以，给别的包的类型别名不能加方法。 |
| 为什么 slice 不能比较？ | 比较语义不明确（比地址还是比内容），Go 直接不让比较，避免歧义。 |
| channel 可以比较吗？ | 可以，两个 channel 指向同一个通道对象时相等。 |
| float 可以比较吗？ | 语法上可以，但精度问题几乎永远会出问题，不要直接比较浮点数。 |

---

## 七、一句话总结

> Go 类型系统记住：类型定义是新类型，类型别名就是同一个东西；slice/map/func 不可比较；interface 等于 nil 要 type 和 value 全 nil；类型转换编译时检查，类型断言运行时检查。
