# string vs []byte 实战选择与 JSON 序列化差异

---

## 一、到底什么时候用 string，什么时候用 []byte？

这是实际开发中最常纠结的问题，我给你一个清晰的决策树，按优先级来：

---

### ✅ 优先用 string 的场景

#### 1. **需要当 map key 时**
```go
// ✅ 可以
cache := make(map[string]int)

// ❌ 编译错误！slice 不能比较，不能当 map key
cache := make(map[[]byte]int)
```
这个是硬性限制，没有任何商量的余地，需要当 key 就必须用 string。

---

#### 2. **存储可读文本时**
JSON、HTML、日志、SQL 语句、普通字符串，所有人类可读的文本都优先用 string：
```go
// ✅ 用 string，直观，打印/调试方便
type User struct {
    Name  string
    Email string
    Bio   string
}

// ❌ 不要用 []byte，每次打日志都要转 string 很麻烦
type User struct {
    Name  []byte // 打日志要写 string(u.Name)，太丑
}
```

---

#### 3. **需要比较 / 判等时**
```go
// ✅ 直接 == 比较
if token == "xxx" { ... }

// ❌ []byte 不能直接比，要写循环或者用 bytes.Equal
if bytes.Equal(token, []byte("xxx")) { ... }
```

---

#### 4. **需要拼接 / 切割 / 替换等字符串操作时**
标准库 `strings` 包的功能比 `bytes` 包丰富得多，而且大多数 string 操作返回的还是 string，不用来回转。

```go
// ✅ strings 包非常方便
if strings.HasPrefix(s, "http") { ... }
s = strings.TrimSpace(s)
parts := strings.Split(s, ",")

// ❌ bytes 包也有，但结果还是 []byte，传出去别人用还得转
```

---

#### 5. **作为接口参数 / 返回值时**
作为公共 API 的话，string 的语义更清晰，调用方不用关心可变性问题：
```go
// ✅ 接口清晰，调用方知道返回的是只读文本
func GetUserToken(userId int64) (string, error)

// ❌ 调用方可能会想：返回的 []byte 能修改吗？要不要我拷贝一份？
func GetUserToken(userId int64) ([]byte, error)
```

---

### ✅ 优先用 []byte 的场景

#### 1. **需要修改内容时**
string 是不可变的，每次修改都会产生新的字符串和内存拷贝，大字符串场景性能很差：
```go
// ❌ 非常慢！每次 + 都会分配新内存，产生大量 GC 对象
var s string
for i := 0; i < 1000; i++ {
    s += "x"
}

// ✅ 快得多！bytes.Buffer 底层是 []byte，预分配容量
var buf bytes.Buffer
buf.Grow(1000) // 预分配
for i := 0; i < 1000; i++ {
    buf.WriteByte('x')
}
s := buf.String()
```

---

#### 2. **处理二进制数据时**
图片、音视频、压缩包、加密哈希、二进制协议，这些不是「文本」，用 []byte 才是正确的语义：
```go
// ✅ 正确，二进制数据就是字节数组
type File struct {
    Content []byte
    Sha256  []byte // 32字节哈希
}

// ❌ 不要用 string 存二进制，语义不对，而且容易被人当成文本打印出乱码
type File struct {
    Content string // 别人看到 string 会以为是可读文本
}
```

---

#### 3. **高性能场景，需要零拷贝转换时**
网络收包、文件读取、JSON 解析，这些场景下数据本来就是 []byte，如果你只是临时读一下就扔掉，没必要转成 string：
```go
// ✅ 直接在 []byte 上操作，零拷贝
data, _ := os.ReadFile("big.json")
if bytes.Contains(data, []byte("error")) {
    // ...
}

// ❌ 没必要转 string，多了一次拷贝
s := string(data) // 100MB 的文件就拷贝 100MB
if strings.Contains(s, "error") { ... }
```

---

#### 4. **需要复用底层数组时**
[]byte 可以 reset、复用底层数组，避免反复分配：
```go
// ✅ 池化复用，减少 GC
var bufPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 4096)
    },
}

buf := bufPool.Get().([]byte)
// ... 使用 buf ...
buf = buf[:0] // reset，底层数组还在
bufPool.Put(buf) // 放回池子
```

---

### 🏁 一句话总结
> **能确定是人类可读的文本，且不需要修改 → 用 string**
>
> **是二进制数据，或者需要修改、拼接、复用 → 用 []byte**

---

## 二、JSON 序列化时的表现差异（超级大坑！）

这是 90% 的 Go 开发者都踩过的坑，我单独拎出来讲。

### 核心差异：[]byte 会被 base64 编码！string 不会！

```go
type Data struct {
    Str string `json:"str"`
    Bts []byte `json:"bts"`
}

d := Data{
    Str: "hello",
    Bts: []byte("hello"),
}
b, _ := json.Marshal(d)
fmt.Println(string(b))
```

**输出：**
```json
{
    "str": "hello",
    "bts": "aGVsbG8="  // ❗ 被 base64 编码了！不是 "hello"！
}
```

90% 的人第一次看到这个输出都会懵：我存的明明是 "hello"，怎么变成乱码了？

**原因：** JSON 标准里没有「字节数组」这个类型，Go 的 JSON 库约定把 `[]byte` 序列化成 base64 编码的字符串。

---

### 坑 1：前端收到 base64，不知道怎么解码
```go
type Response struct {
    Token []byte `json:"token"` // ❌ 前端收到的是 base64 字符串，不是你以为的原始 token
}
```
前端：「你这个 token 怎么是乱码？」
你：「我本地打印明明是正常的啊？」

---

### 坑 2：反序列化也会自动解码，跨语言调用直接炸
```go
// Go 端发出去的是 base64
resp := []byte("123456")
json.Marshal(resp) // "MTIzNDU2"

// Java/Python 端收不到原始的 123456，收到的是 MTIZNDU2
// 它们不会自动帮你解码 base64，直接就解析失败
```

---

### 坑 3：空 []byte vs nil []byte 序列化结果不一样
```go
type Data struct {
    Empty []byte `json:"empty"` // len=0, cap=0, 不是 nil
    Nil   []byte `json:"nil"`   // 是 nil
}
```

**序列化结果：**
```json
{
    "empty": "",       // 空串
    "nil": null        // null
}
```
这个差异在和前端交互时会有问题，JS 处理 `""` 和 `null` 的逻辑完全不一样。

---

### 坑 4：`json.RawMessage` 是 []byte 的别名，也有同样的问题
很多人以为 `json.RawMessage` 是特殊类型，其实它就是 `type RawMessage []byte`，同样会被 base64 编码：
```go
type Response struct {
    Data json.RawMessage `json:"data"` // ❌ 也会被 base64！
}
```
**正确写法：** 必须用指针：
```go
type Response struct {
    Data *json.RawMessage `json:"data"` // ✅ 用指针就不会被 base64
}
```

---

### ✅ 正确的序列化选择

| 你的原始数据 | 想要的 JSON 输出 | 应该用什么类型 |
|-------------|----------------|--------------|
| `[]byte("hello")` | `"hello"` | 转成 `string` |
| `[]byte("hello")` | `"aGVsbG8="` | 保留 `[]byte` |
| `[]byte{0x01, 0x02, 0x03}` | `"AQID"` | 保留 `[]byte`（二进制数据本来就应该 base64） |
| 动态 JSON | `{"a":1}` | `*json.RawMessage`（必须用指针） |

---

### ✅ 最佳实践：不要在对外的 API 里用 []byte 当字段
```go
// ❌ 对外接口绝对不要这么写，前端/其他语言会炸
type APIResponse struct {
    Token  []byte `json:"token"`
    UserID []byte `json:"user_id"`
}

// ✅ 全部转成 string，明确语义
type APIResponse struct {
    Token  string `json:"token"`
    UserID string `json:"user_id"`
}
```

**只有一种情况可以用 []byte 当 JSON 字段：** 你就是想传输二进制数据，并且接收方知道要 base64 解码，比如图片、加密数据等。

---

## 三、性能对比：什么时候转，什么时候不转？

很多人会纠结「转 string 会不会很慢？」，我给你一个量化的参考：

| 场景 | 成本 | 建议 |
|------|------|------|
| 10 字节以下的小字符串转 string | 纳秒级，几乎无感 | 随便转，怎么清晰怎么来 |
| 1KB 左右的字符串转 string | 微秒级，1μs 左右 | 大部分场景没问题 |
| 1MB 以上的大字符串转 string | 毫秒级，1ms 左右 | 尽量避免，能在 []byte 上操作就在 []byte 上操作 |
| 循环 100 万次每次都转 | 秒级，非常慢 | 绝对要避免，能在循环外转就在循环外转 |

**核心原则：** 业务逻辑清晰优先，先写对再考虑优化。不要为了省几微秒的性能，把代码写得晦涩难懂，出了 bug 更贵。

---

## 四、面试高频问答

| 问题 | 答案 |
|------|------|
| 什么时候用 string，什么时候用 []byte？ | 可读文本、当 map key、需要比较、作为接口参数 → 用 string；二进制数据、需要修改拼接、高性能零拷贝、需要复用数组 → 用 []byte。 |
| []byte JSON 序列化会变成什么？ | 会被自动 base64 编码，不是原始字节内容。 |
| 空 []byte 和 nil []byte JSON 序列化有区别吗？ | 有，空的序列化成 `""`，nil 序列化成 `null`。 |
| json.RawMessage 也会被 base64 吗？ | 会，因为它本质就是 `type RawMessage []byte`，必须用指针才不会。 |
| 为什么 JSON 库要这么设计？ | JSON 标准没有字节数组类型，只能序列化成字符串，base64 是二进制转字符串的标准方式。 |
| 我就是想让 []byte 序列化成普通字符串怎么办？ | 自定义 `MarshalJSON` 方法，或者转成 string 再序列化。 |
| 大字符串转 []byte 性能影响大吗？ | 1MB 以上会有明显的拷贝开销，ms 级。小字符串可以忽略。 |

---

## 五、一句话总结

> **对外 API 字段永远不要用 []byte，会被 base64！除非你就是想传二进制。**
>
> **内部代码：文本用 string，二进制/需要修改用 []byte。**