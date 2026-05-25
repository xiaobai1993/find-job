# Python 语法完全指南（Go开发者视角）

> 面向有Go基础、零基础Python的开发者。所有复杂语法点均有Go对比，Python特有特性重点标注。

---

## 0. 核心思维差异（先看这个！）

| 维度 | Go | Python |
|------|----|--------|
| 范式 | 静态强类型、编译型、显式优先 | 动态强类型、解释型、简洁优先 |
| 哲学 | 显式大于隐式，一种写法解决一个问题 | 隐式方便，多种写法解决一个问题 |
| 错误处理 | if err != nil 显式处理 | 异常机制 try/except |
| 并发 | goroutine + channel 原生 | 多线程/多进程/协程多种方式 |
| 可变/不可变 | 大部分类型可变 | 分可变/不可变两大阵营 |
| 代码组织 | package 为单位 | module + package |

**Go开发者最容易踩的坑：**
- Python没有分号、没有大括号、靠缩进控制块
- 变量不需要声明类型，运行时才确定类型
- 同一个变量可以赋值不同类型（虽然不推荐）
- 不需要显式return默认返回None

---

## 1. 基础语法

### 1.1 变量声明

```python
# Python
a = 1           # 自动推断类型，相当于Go的 var a = 1
b: int = 1        # 类型注解（可选，运行时不强制！）
c, d = 1, 2      # 多变量同时赋值
a, b = b, a       # 交换变量，Go也有这个特性
x = y = z = 0     # 多个变量赋值同一个值
```

对比Go：
```go
// Go
var a int = 1
a := 1
var a, b int = 1, 2
a, b = b, a
```

⚠️ 注意：Python的类型注解只是给人看的，解释器不检查！运行时还是动态的。

### 1.2 基础类型

| Python类型 | Go对应 | 说明 |
|-----------|--------|------|
| `int` | int | Python的int任意精度，不会溢出 |
| `float` | float64 | |
| `bool` | bool | True/False 首字母大写！ |
| `str` | string | 字符串是不可变的 |
| `None` | nil | None是唯一的空值，类型是NoneType |

⚠️ **真假判断对比：
```python
# Python里以下全是False：
# False, 0, 0.0, "", [], {}, (), set(), None
if not x:  # 这样写最地道
    pass
```

```go
// Go里只有false是false，0和空字符串不能直接当bool用！
if x == 0 || x == "" {}
```

### 1.3 字符串

```python
s = "hello"
s = 'hello'           # 单双引号一样
s = """多行
字符串"""
f"名字是{name}"        # f-string，格式化最常用
"{} {}".format(a, b)
"%d %s" % (1, "a")

# 常用操作
s.upper()
s.lower()
s.strip()
s.split(",")
",".join(["a", "b"])
s.startswith("h")
s.endswith("o")
len(s)                  # 内置函数，不是方法
```

对比Go：
```go
// Go字符串是[]byte的封装，操作都在strings包里
strings.ToUpper(s)
strings.Split(s, ",")
strings.Join([]string{"a","b"}, ",")
len(s)
```

---

## 2. 集合类型（重点！和Go差异最大）

### 2.1 列表 list = Go的slice

```python
lst = [1, 2, 3, "混合类型可以混放！]  # Go的[]interface{}
lst = []
lst = [0] * 5  # 初始化5个0

# 常用操作：
lst.append(4)          # 末尾加元素
lst.insert(0, 0)       # 指定位置插入
lst.pop()               # 删末尾
lst.pop(0)              # 删第一个
lst.remove(2)            # 删值为2的第一个元素
lst.extend([4,5,6])     # 合并另一个列表
lst[0]                 # 索引
lst[0:3]               # 切片！[起始:结束:步长
lst[-1]                  # 最后一个元素
lst[::-1]                  # 反转
3 in lst                 # 判断是否存在，Go里没有这个语法！
len(lst)
```

⚠️ **Go没有的语法：切片操作**
```python
a = [1,2,3,4,5]
a[1:4]      # [2,3,4]  从索引1到3（不包含4）
a[:3]        # [1,2,3]
a[2:]         # [3,4,5]
a[::2]        # [1,3,5] 步长2
a[-3:]         # 最后3个
```

Go没有这么方便的切片语法只能自己写。

### 2.2 元组 tuple = 不可变的list
```python
t = (1, 2, 3)
t = 1, 2, 3      # 括号可以省略
# 解包：
a, b, c = t         # 多赋值其实是元组解包
a, *rest = [1,2,3,4]  # a=1, rest=[2,3,4]
```
⚠️ Go没有元组，函数返回多值本质上是Go的多返回值，但Python元组用得更灵活。

### 2.3 字典 dict = Go的map
```python
d = {"name": "张三", "age": 18}
d = dict(name="张三", age=18)

d["name"]              # 取值，key不存在会报错！
d.get("name")           # 取值，不存在返回None，不会报错
d.get("gender", "男")   # 不存在返回默认值
d["age"] = 19          # 赋值
"name" in d             # 判断key是否存在
d.keys()               # 所有key
d.values()             # 所有value
d.items()              # 所有key-value对
d.pop("age")           # 删除key
```

对比Go：
```go
// Go map[]语法：
v, ok := m["key"]  // ok判断是否存在
for k, v := range m {}
```

⚠️ Python字典遍历：
```python
for k, v in d.items():
    print(k, v)
```

### 2.4 集合 set
```python
s = {1, 2, 3}
s = set([1,2,3])
s.add(4)
s.remove(1)
1 in s
# 集合运算：
a & b  # 交集
a | b  # 并集
a - b  # 差集
```
Go里没有内置set，一般用map[T]struct{}模拟。

---

## 3. 控制流

### 3.1 if/else
```python
if x > 0:
    print("positive")
elif x < 0:
    print("negative")
else:
    print("zero")

# 三元表达式（Go没有！）
result = "大于0" if x > 0 else "小于等于0"
```

对比Go没有三元表达式，Go官方故意不加。

### 3.2 for循环（Python只有for，没有while！）

```python
# 普通for循环
for i in range(10):        # 0到9
    print(i)

for i in range(1, 11):      # 1到10
    print(i)

for i in range(0, 10, 2):   # 0,2,4,6,8
    print(i)

# 遍历列表
for item in lst:
    print(item)

# 遍历带索引
for idx, item in enumerate(lst):
    print(idx, item)

# 遍历字典
for k, v in d.items():
    print(k, v)

# while循环（其实是for的变种）
while x < 10:
    x += 1

# break, continue 和Go一样
# for ...:
#     if ...:
#         break
#     continue
```

对比Go：
```go
// Go三种for
for i := 0; i < 10; i++ {}
for i < 10 {}  // while
for {}            // 死循环
for idx, item := range slice {}
```

⚠️ Python没有经典的三段式for循环，只有for ... in ...是唯一写法。

---

## 4. 函数（你最懵的部分！重点！）

### 4.1 基础函数定义

```python
def add(a, b):
    return a + b

# 带类型注解
def add(a: int, b: int) -> int:
    return a + b
```

### 4.2 参数传递的5种形式（重点！！！

#### 1) 位置参数（最普通，和Go一样）
```python
def add(a, b):
    return a + b
add(1, 2)  # 1传给a，2传给b
```

#### 2) 默认参数
```python
def power(x, n=2):  # n默认是2
    return x ** n

power(3)     # 9，用默认n=2
power(3, 3)   # 27，传了就用传的
```

⚠️ **超级大坑！默认参数是可变对象的情况！**
```python
def append_item(item, lst=[]):
    lst.append(item)
    return lst

append_item(1)  # [1]
append_item(2)  # [1, 2]！！！不是[2]！
# 原因：默认参数在函数定义时创建一次，不是每次调用创建！
```
正确写法：
```python
def append_item(item, lst=None):
    if lst is None:
        lst = []
    lst.append(item)
    return lst
```
Go没有这个坑，因为Go没有默认参数这个语法糖。

#### 3) 关键字参数
```python
def person(name, age, gender):
    print(name, age, gender)

person("张三", 18, "男")           # 位置参数
person(name="张三", age=18, gender="男")  # 关键字参数，顺序随便换
person(age=18, name="张三", gender="男")  # 顺序没关系
```
Go没有关键字参数，必须按顺序传。

#### 4) *args 可变位置参数
```python
def sum(*args):  # args是个tuple
    total = 0
    for num in args:
        total += num
    return total

sum(1,2,3,4,5)  # 随便传多少个都行

# 也可以把list/tuple打散传进去
nums = [1,2,3]
sum(*nums)  # 相当于sum(1,2,3)
```
对比Go的可变参数：
```go
func sum(nums ...int) int {
    // nums是[]int
}
sum(1,2,3)
sum(nums...)  // 打散slice传进去
```
原理一样，Python是*args，Go是...T。

#### 5) **kwargs 可变关键字参数
```python
def person(**kwargs):  # kwargs是个dict
    for k, v in kwargs.items():
        print(k, v)

person(name="张三", age=18, gender="男")

# 也可以把dict打散传进去
info = {"name": "张三", "age": 18}
person(**info)
```
⚠️ **Go完全没有这个特性！这是Python特有的！

#### 参数顺序规则（必须遵守！）：
```
位置参数 → *args → 默认参数 → **kwargs
```
示例：
```python
def demo(a, b, *args, c=10, d=20, **kwargs):
    print(a, b, args, c, d, kwargs)

demo(1, 2, 3, 4, c=100, x=1, y=2)
# a=1, b=2, args=(3,4), c=100, d=20, kwargs={'x':1, 'y':2}
```

### 4.3 返回值

```python
def calc(a, b):
    return a + b, a - b  # 多返回值，本质返回tuple

sum_val, sub_val = calc(10, 5)  # 解包
result = calc(10, 5)  # result是个tuple
```
对比Go多返回值是语言原生，Python是tuple伪装的。

### 4.4 匿名函数 lambda
```python
add = lambda x, y: x + y
add(1, 2)

# 常用在排序、过滤等地方
sorted(lst, key=lambda x: x["age"])
```
Go没有匿名函数这么方便的写法，只能写func。

---

## 5. 面向对象

### 5.1 类定义
```python
class Person:
    # 构造函数
    def __init__(self, name, age):
        self.name = name
        self.age = age

    def say_hello(self):
        print(f"我是{self.name}")

p = Person("张三", 18)
p.say_hello()
```
⚠️ **所有方法第一个参数必须是self！！！** 相当于Go的receiver，但是必须显式写出来！Go的receiver也是隐式的，Python必须显式写self。

对比Go：
```go
type Person struct {
    Name string
    Age  int
}

func (p *Person) SayHello() {
    fmt.Println(p.Name)
}
```

### 5.2 继承
```python
class Student(Person):
    def __init__(self, name, age, grade):
        super().__init__(name, age)  # 调用父类构造
        self.grade = grade

    def say_hello(self):
        super().say_hello()  # 调用父类方法
        print(f"我是{self.grade}年级学生")
```
Go没有继承，用结构体嵌入模拟继承。

### 5.3 魔术方法（Python特有！Go完全没有！
```python
class Person:
    def __init__(self, name, age):
        self.name = name
        self.age = age

    def __str__(self):  # 相当于Go的String()方法
        return f"Person(name={self.name}, age={self.age})"

    def __len__(self):  # len(p)的时候调用
        return self.age

    def __eq__(self, other):  # ==的时候调用
        return self.name == other.name and self.age == other.age

    def __add__(self, other):  # +的时候调用
        return Person(self.name + other.name, self.age + other.age)
```
这些双下划线开头结尾的方法叫魔术方法，Python大量用在自定义类型让自定义类型表现得像内置类型一样。

---

## 6. Python特有高级特性（Go完全没有的！重点！）

### 6.1 装饰器（重点！！！你看代码会卡壳的地方大多是这个！）

作用：在不修改原函数代码的情况下，给函数增加功能。

```python
# 最简单的装饰器
def log_decorator(func):
    def wrapper(*args, **kwargs):
        print("函数执行前")
        result = func(*args, **kwargs)
        print("函数执行后")
        return result
    return wrapper

@log_decorator
def add(a, b):
    return a + b

# 等价于：add = log_decorator(add)
```
带参数的装饰器：
```python
def log(tag):
    def decorator(func):
        def wrapper(*args, **kwargs):
            print(f"[{tag}] 函数执行前")
            result = func(*args, **kwargs)
            return result
        return wrapper
    return decorator

@log("DEBUG")
def add(a, b):
    return a + b
```
⚠️ 你在仓库里看到的@something开头的全是装饰器！相当于给下面的函数包了一层。Go没有这个语法糖，要实现只能手动包一层函数。

常见装饰器：
- @property：把方法变成属性
- @classmethod：类方法
- @staticmethod：静态方法
- @lru_cache：缓存函数结果
- @dataclass：自动给类加__init__等方法

```python
from functools import lru_cache
from dataclasses import dataclass

@dataclass
class Person:
    name: str
    age: int

# 自动生成__init__, __repr__, __eq__等
p = Person("张三", 18)
```

### 6.2 生成器 generator
```python
# 生成器函数，用yield返回
def count(n):
    for i in range(n):
        yield i  # 每次yield一次返回一个，下次从这里继续

for num in count(5):
    print(num)
```
⚠️ **和return不一样，yield不会结束函数，下次调用继续执行。生成器是惰性的，不会一次性生成所有元素，省内存。

Go没有生成器，要实现类似效果只能用channel模拟。

### 6.3 推导式（Python特有的！！！写代码最常用！）

```python
# 列表推导式
[x for x in range(10) if x % 2 == 0]
# 等价于：
result = []
for x in range(10):
    if x % 2 == 0:
        result.append(x)

# 字典推导式
{x: x**2 for x in range(5)}
# {0:0, 1:1, 2:4, 3:9, 4:16}

# 集合推导式
{x**2 for x in range(5)}
# {0, 1, 4, 9, 16}

# 生成器表达式
(x**2 for x in range(10))
```
Go完全没有推导式语法，都得自己写循环。

### 6.4 上下文管理器 with语句
```python
# 最常见的：打开文件
with open("file.txt", "r") as f:
    content = f.read()
# 离开with块自动关闭文件，不用手动关！
```
自己实现上下文管理器：
```python
class MyContext:
    def __enter__(self):
        print("进入")
        return self
    def __exit__(self, exc_type, exc_val, exc_tb):
        print("退出")
```
Go没有这个语法，Go用defer。

### 6.5 切片赋值
```python
a = [1,2,3,4,5]
a[1:3] = [10, 20, 30]
# a变成 [1, 10, 20, 30, 4, 5]
```

---

## 7. 模块和包

```python
# 导入整个模块
import math
math.sqrt(4)

# 导入模块别名
import numpy as np

# 导入模块里的具体函数/类
from math import sqrt, pow

# 导入模块里所有东西（不推荐）
from math import *
```

对比Go的import包。

---

## 8. 异常处理

```python
try:
    result = 10 / 0
except ZeroDivisionError as e:
    print(e)
except ValueError as e:
    print(e)
else:
    print("没有异常才执行")
finally:
    print("不管有没有异常都执行")
```
对比Go的if err != nil。Python用try/except，Go显式返回err。

主动抛异常：
```python
raise ValueError("参数不对")
```

---

## 9. 常用内置函数

| 函数 | 作用 |
|------|------|
| `len(x)` | 长度 |
| `type(x)` | 看类型 |
| `isinstance(x, type)` | 判断类型 |
| `range(n)` | 生成范围 |
| `enumerate(iter)` | 带索引遍历 |
| `zip(a, b)` | 两个集合一起遍历 |
| `map(func, iter)` | 每个元素应用函数 |
| `filter(func, iter)` | 过滤元素 |
| `sorted(iter)` | 排序 |
| `reversed(iter)` | 反转 |
| `sum(iter)` | 求和 |
| `max(iter)`, `min(iter)` | 最大最小 |
| `any(iter)` | 有一个True就True |
| `all(iter)` | 全True才True |
| `print()` | 打印 |

---

## 10. 你看代码最常见的看不懂的语法

### 10.1 海象运算符 :=
```python
# Python 3.8+
if (n := len(a)) > 10:
    print(f"列表太长了，有{n}个元素")
```
赋值同时可以用在表达式里，Go没有。

### 10.2 类型注解进阶
```python
from typing import List, Dict, Optional, Union

def process(data: List[int]) -> Dict[str, int]:
    pass

def get(name: Optional[str] = None):
    # Optional就是可以是str也可以是None
```

### 10.3 不定长参数传参技巧
```python
def func(a, b, c, d):
    pass

args = [1, 2]
kwargs = {"c":3, "d":4}
func(*args, **kwargs)  # 等价于func(1,2,c=3,d=4)
```

### 10.4 函数是第一公民
函数可以当参数传、当返回值返回、赋值给变量，这一点Go也有，Python用得更普遍。

---

## 11. Go开发者最容易犯的错误Top10

1. **缩进错误**：Python缩进是语法，不是风格，4空格
2. **可变默认参数**：上面说的大坑
3. **==和is搞混**：==判断值相等，is判断是不是同一个对象
   ```python
   a = [1,2,3]
   b = [1,2,3]
   a == b  # True
   a is b  # False，两个不同对象
   ```
4. **字符串是不可变的**：改字符串必须生成新的
5. **整数除法是//不是/**：3 // 2 = 1，3 / 2 = 1.5
6. **for循环变量作用域**：Python for循环里的变量外面也能访问到
7. **循环里修改列表**：遍历列表的时候删元素会出问题
8. **GIL锁**：Python多线程不能利用多核CPU，CPU密集型要用多进程
9. **深拷贝浅拷贝**：
   ```python
   import copy
   copy.copy(x)  # 浅拷贝
   copy.deepcopy(x)  # 深拷贝
   ```
10. **None判断用is None，不是== None

---

## 12. 看仓库代码快速上手技巧

你看到不懂的语法90%是：
1. @装饰器
2. *args/**kwargs
3. 列表/字典推导式
4. 生成器yield
5. 魔术方法__xxx__
6. with语句
7. 海象运算符:=
8. 类型注解

碰到不懂的直接查上面对应章节就行。
