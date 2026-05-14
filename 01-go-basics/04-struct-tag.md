# Struct Tag 原理与常见坑

---

## 一、先搞懂：tag 到底是什么？

很多人以为 tag 是「注释」或者「字符串常量」——不对，**tag 是 struct 类型的一部分，编译时就存在类型元数据里，运行时通过反射读取。**

```go
type User struct {
    Name string `json:"name" db:"user_name" validate:"required"`
}
```

上面这一串反引号里的内容，不是注释，也不是字符串，是**静态类型元数据**，编译后就固定在类型信息里了，不会随实例变化。

---

## 二、基础语法

### 1. 格式
`key:"value"` 格式，多个 key 用空格分隔：
```go
// 单个 tag
Name string `json:"name"`

// 多个 tag
Name string `json:"name" db:"user_name" validate:"required,min=2,max=20"`

// 同一个 key 多个选项，用逗号分隔
Name string `json:"name,omitempty"`
```

### 2. 为什么必须是反引号？
因为双引号在 Go 字符串里需要转义，反引号是原生字符串字面量，不用转义，写起来方便。你用双引号也可以，就是很难看：
```go
// 可以但不推荐
Name string `json:\"name\"`
```

---

## 三、底层原理：tag 存在哪里？怎么读？

### 1. 存在哪里？
Go 的类型元数据里，每个 struct 字段都对应一个 `runtime.StructField` 结构体：
```go
type StructField struct {
    Name      string    // 字段名
    Type      Type      // 字段类型
    Tag       StructTag // 就是 tag！
    Offset    uintptr   // 字段在 struct 里的内存偏移
    // ...其他字段
}
```

`StructTag` 本质就是个字符串，但是带了解析方法：
```go
type StructTag string

func (tag StructTag) Get(key string) string          // 获取值
func (tag StructTag) Lookup(key string) (string, bool) // Go 1.7+，返回值和是否存在
```

### 2. 怎么读？通过反射
```go
u := User{}
t := reflect.TypeOf(u)
field := t.Field(0) // 取第一个字段

// 读 json tag
jsonTag := field.Tag.Get("json") // "name"

// 判断 tag 是否存在（推荐用 Lookup，不要用 Get == ""）
value, ok := field.Tag.Lookup("json")
```

### 3. 也可以用 unsafe 直接读偏移
因为 tag 存在类型元数据的固定偏移位置，用 unsafe 也能直接读，但不推荐日常用：
```go
// 注意：不同 Go 版本结构体偏移可能变
typePtr := unsafe.Pointer(reflect.TypeOf(u))
// ... 跳过几个字段就能拿到 tag
```

---

## 四、常见的 tag 用法

你每天都在用，只是可能没意识到这些都是 tag：

| tag key | 用途 | 示例 |
|---------|------|------|
| `json` | JSON 序列化/反序列化 | `json:"name,omitempty"` |
| `xml` | XML 序列化 | `xml:"Name"` |
| `db` / `gorm` | 数据库 ORM 映射 | `gorm:"column:user_name;type:varchar(100);not null"` |
| `form` / `uri` | Web 框架参数绑定（Gin/Echo） | `form:"page" uri:"id"` |
| `validate` | 参数校验 | `validate:"required,email,min=6"` |
| `yaml` / `toml` | 配置文件解析 | `yaml:"mysql.host"` |
| `mapstructure` | Viper 等配置库映射 | `mapstructure:"log_level"` |
| `protobuf` | Protobuf 生成代码 | `protobuf:"bytes,1,opt,name=name"` |

---

## 五、90%的人都踩过的坑（面试必考）

### 坑 1：首字母小写，有 tag 也没用！
```go
type User struct {
    name string `json:"name"` // ❌ 首字母小写，反射读不到！
}
```

**为什么？** 小写是私有字段，`reflect` 包是外部包，没有权限访问私有字段的元数据，tag 也读不到。

这是 Go 新手最常见的坑，写了一下午 JSON 序列化出来是空对象，死活找不到原因。

---

### 坑 2：`omitempty` 会把零值也忽略！
支付场景头号大坑，很多人栽过：
```go
type Order struct {
    Amount int `json:"amount,omitempty"` // ❌ 大坑！
}

o := Order{Amount: 0} // 金额本来就是 0
json.Marshal(o) // 输出 {}，amount 字段没了！
```

**omitempty 忽略的是「零值」，不是「空」：**
- 0、false、""、nil、空数组、空 map 都会被忽略
- 支付场景里 Amount = 0 是合法值，绝对不能加 omitempty！

**解决：用指针**
```go
type Order struct {
    Amount *int `json:"amount,omitempty"` // ✅ nil 才忽略，0 不会
}
```

---

### 坑 3：tag 写错一个字母，debug 半天
```go
type User struct {
    Name string `json:"UserName"` // 注意大小写
}
```

json 包是严格匹配的，前端传 `username` 小写，你 tag 写 `UserName`，就是读不到，也不报错，非常难排查。

**建议：统一小写加下划线，不要混用驼峰。**

---

### 坑 4：匿名结构体的 tag 继承与冲突
```go
type Base struct {
    ID int `json:"id"`
}

type User struct {
    Base       // ✅ 匿名嵌入，id tag 会继承
    Name string `json:"name"`
}
```

**如果有冲突呢？**
```go
type A struct {
    ID int `json:"id"`
}
type B struct {
    ID int `json:"user_id"`
}
type User struct {
    A
    B  // ❌ 两个 ID 字段冲突，JSON 序列化会选哪个？答案：都不序列化！
}
```
字段名冲突时，两个字段都会被忽略，不会报错，又是一个很难排查的坑。

---

### 坑 5：时间格式的坑
```go
type Order struct {
    CreateAt time.Time `json:"create_at"` // ❌ 默认输出 RFC3339 格式，带时区
}
// 输出："2024-05-20T12:00:00+08:00"
```

如果前端只想要 `2024-05-20 12:00:00`，必须指定格式：
```go
type Order struct {
    CreateAt time.Time `json:"create_at" time_format:"2006-01-02 15:04:05"`
}
```
注意：Go 的时间模板必须是 `2006-01-02 15:04:05` 这个固定时间，写错了不会报错，只是时间解析不对。

---

### 坑 6：`-` 忽略字段
```go
type User struct {
    Password string `json:"-"` // ✅ 序列化时完全忽略这个字段，不会输出
}
```
注意是 `"-"`，不是 `""`，也不是 `"omitempty"`。

---

## 六、高级用法：自定义 tag

你可以定义自己的 tag，实现自己的逻辑。比如实现一个 `default` tag，给字段设置默认值：

```go
// 定义 tag 格式：`default:"zhangsan"`
type User struct {
    Name string `default:"zhangsan"`
    Age  int    `default:"18"`
}

// 用反射读 tag 并设置默认值
func SetDefault(v interface{}) {
    t := reflect.TypeOf(v).Elem()
    val := reflect.ValueOf(v).Elem()

    for i := 0; i < t.NumField(); i++ {
        field := t.Field(i)
        defaultVal, ok := field.Tag.Lookup("default")
        if !ok continue

        // 根据字段类型设置值
        switch field.Type.Kind() {
        case reflect.String:
            if val.Field(i).String() == "" {
                val.Field(i).SetString(defaultVal)
            }
        case reflect.Int:
            if val.Field(i).Int() == 0 {
                n, _ := strconv.Atoi(defaultVal)
                val.Field(i).SetInt(int64(n))
            }
        }
    }
}
```

很多 ORM 框架、参数校验框架都是这么实现的。

---

## 七、面试高频问答

| 问题 | 答案 |
|------|------|
| struct tag 是注释吗？存在哪里？ | 不是注释，是 struct 类型元数据的一部分，编译后存在 runtime.StructField 里，运行时通过反射读取。 |
| 小写字段加了 tag 能读到吗？ | 不能。小写是私有字段，反射是外部包，没有权限访问私有字段的元数据。 |
| omitempty 有什么坑？ | 会忽略所有零值：0、false、""、nil 都会被忽略。支付场景的金额、数量字段绝对不能加。 |
| 怎么判断 tag 是否真的设置了？ | 用 `Lookup` 方法，不要用 `Get == ""`，因为 tag 可以显式设置为空串。 |
| 两个匿名结构体字段冲突了，tag 会怎么样？ | 两个字段都会被忽略，不会报错，也不会选某一个。 |
| tag 的值是运行时才能确定吗？ | 不是，编译时就固定在类型元数据里了，同一个类型的所有实例共享同一份 tag。 |
| JSON 序列化时，怎么让一个字段永远不输出？ | tag 写 `json:"-"`。 |
| time.Time 序列化默认是什么格式？能改吗？ | 默认 RFC3339 带时区。可以自定义 MarshalJSON 方法，或者用第三方库的时间类型。 |
| 指针类型和值类型加 omitempty 有什么区别？ | 值类型零值会被忽略，指针类型只有 nil 才会被忽略，0 不会。 |

---

## 八、最佳实践

1. **字段永远大写**，除非你确定这个字段不需要序列化
2. **支付相关的数字字段不要加 omitempty**，0 是合法值
3. **tag 命名统一风格**，要么全下划线，要么全小驼峰，不要混用
4. **不要在 tag 里写复杂逻辑**，tag 是元数据，不是脚本
5. **自定义 tag 要写文档**，告诉团队每个 key 是什么意思
6. **复杂序列化逻辑用自定义 Marshal/Unmarshal**，不要堆 tag
