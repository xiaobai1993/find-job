# HTTP 中间件深度解析（Gin + Client）- 面试版

> 🎯 核心考点：Gin 中间件、http.RoundTripper、Client 中间件、和 gRPC 中间件的区别
> 源码位置：`pkg/http/utils/`、`pkg/httpwrap/`

---

## 一、直接回答你的问题

| 问题 | 答案 |
|-----|------|
| **是基于官方 net/http 还是框架？** | **基于 Gin 框架**！从 `gin.HandlerFunc` 和 `github.com/gin-gonic/gin` 可以看出来 |
| **和 gRPC 中间件有什么不同？** | 思想一样都是洋葱模型，但接口和实现方式不同 |
| **httpwrap 是什么？** | 是 HTTP Client 的封装，链式调用风格，类似 Python 的 requests |

---

## 二、HTTP Server 端中间件（Gin 风格）

### 📝 标准模板

```go
func middlewareName(可选参数) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. 👇 前置逻辑：请求进来，还没执行业务
        fmt.Println("请求进来了：", c.Request.URL.Path)

        // 2. 👇 这一行是"穿过去"：调用下一层
        c.Next()

        // 3. 👇 后置逻辑：业务已经执行完了，拿到响应了
        fmt.Println("响应出去了：", c.Writer.Status())
    }
}
```

**和 gRPC 对比：一模一样的三步！只是参数从 `context.Context` 变成了 `*gin.Context`**

---

### 🌰 例子 1：recover 中间件（美餐生产级代码）

```go
func recovery(renderFn func(c *gin.Context, p any)) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 前置：defer 注册 recover
        defer func() {
            if r := recover(); r != nil {
                // 后置：捕获 panic，记录日志，返回 500
                log.For(c.Request.Context()).Error("panic error",
                    log.Any("error", r),
                    log.Stack("panicStack"),
                )
                renderFn(c, r)  // 渲染错误响应
                c.Abort()       // 终止后续中间件
            }
        }()

        // 穿过去执行业务
        c.Next()
    }
}
```

**和 gRPC 的区别：** Gin 有 `c.Abort()` 可以直接终止中间件链，gRPC 没有这个，只能通过返回 error 来终止。

---

### 🌰 例子 2：日志中间件（美餐生产级代码）

```go
func serverLog(ops *ServerLoggerOptions) gin.HandlerFunc {
    return func(c *gin.Context) {
        path := c.Request.URL.Path
        if _, ok := ops.skipMethodsMap[path]; ok {
            c.Next()  // 白名单路径直接跳过
            return
        }

        start := time.Now()

        // 前置：复制请求体（因为 Body 只能读一次）
        requestBody, _ := c.GetRawData()
        c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

        // 前置：包装 ResponseWriter，才能拿到响应体
        bodyWriter := &bodyLogWriter{
            body:           bytes.NewBufferString(""),
            ResponseWriter: c.Writer,
        }
        c.Writer = bodyWriter

        // 后置：defer 记录日志
        defer func() {
            log.AccessLogger.For(c.Request.Context()).Print(ops.Level, "gin log",
                log.String("path", path),
                log.String("method", c.Request.Method),
                log.Int("httpCode", c.Writer.Status()),
                log.Int64("timeCost", time.Since(start).Milliseconds()),
                log.ByteString("req", utils.TruncateBytes(requestBody, 1024)),
                log.ByteString("resp", utils.TruncateBytes(bodyWriter.body.Bytes(), 1024)),
            )
        }()

        // 穿过去执行业务
        c.Next()
    }
}
```

> 💡 **面试高频问题：为什么要包装 ResponseWriter？**
>
> 因为 `gin.ResponseWriter` 的 `Write()` 写完就没了，你拿不到响应体内容。
> 所以要写一个包装类，把写入的内容同时复制一份到 buffer 里，这样日志中间件才能拿到响应体打日志。
>
> 这是 Gin 最经典的面试题，90% 的人写日志中间件都会踩这个坑！

---

### 🌰 例子 3：超时控制中间件

```go
func setTimeout(timeout time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        ctx := c.Request.Context()
        if _, ok := ctx.Deadline(); !ok {
            newCtx, cf := context.WithTimeout(ctx, timeout)
            defer cf()
            // 把新的 ctx 塞回去
            c.Request = c.Request.WithContext(newCtx)
        }
        c.Next()
    }
}
```

---

### 🌰 例子 4：DB 注入中间件

```go
func injectDBIntoCtx() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 把 DB 连接注入到 context 里
        c.Request = c.Request.WithContext(xormv2.WithDB(c.Request.Context()))
        c.Next()
    }
}
```

---

## 三、HTTP Client 端中间件（基于 http.RoundTripper）

这个是很多人不知道的！HTTP Client 也有中间件机制，基于 `http.RoundTripper` 接口。

### 📝 核心原理

`http.Client` 有一个 `Transport` 字段，类型是 `http.RoundTripper` 接口：

```go
type RoundTripper interface {
    RoundTrip(*Request) (*Response, error)
}
```

这就是 HTTP Client 的"handler"！你可以包装它，实现中间件！

---

### 📝 标准模板

```go
// 1. 先定义一个函数类型，实现 RoundTripper 接口
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
    return fn(req)
}

// 2. 中间件函数：包装下一层 RoundTripper
func LoggingMiddleware(logLevel log.Level, next http.RoundTripper) http.RoundTripper {
    return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
        // 前置逻辑：请求发出去之前
        start := time.Now()
        reqBody, _ := io.ReadAll(req.Body)
        req.Body = io.NopCloser(bytes.NewBuffer(reqBody))

        // 穿过去：真正发请求
        resp, err := next.RoundTrip(req)
        if err != nil {
            return nil, err
        }

        // 后置逻辑：拿到响应了
        respBody, _ := io.ReadAll(resp.Body)
        resp.Body = io.NopCloser(bytes.NewBuffer(respBody))

        log.Print(logLevel, "http client log",
            log.String("url", req.URL.String()),
            log.Int("httpCode", resp.StatusCode),
            log.Int64("timeCost", time.Since(start).Milliseconds()),
        )

        return resp, nil
    })
}
```

**是不是和 gRPC Client 中间件 99% 一样？思想完全一样！**

---

### 怎么用？

```go
// 创建一个 http.Client，把 Transport 换成包装后的中间件
client := &http.Client{
    Transport: LoggingMiddleware(log.InfoLevel, http.DefaultTransport),
}

// 正常发请求就行，日志自动打
client.Get("https://example.com")
```

---

## 四、httpwrap：美餐的 HTTP Client 链式调用封装

`pkg/httpwrap/httpwrap.go` 是一个链式调用风格的 HTTP Client 封装，类似 Python 的 requests 或者 Java 的 RestTemplate。

### 使用示例

```go
// 创建请求
req := httpwrap.NewAPIRequest(ctx,
    httpwrap.WithLogLevel(log.DebugLevel),
    httpwrap.WithEnableOTEL(true),
)

// 链式调用
body, err := req.
    Post("https://api.example.com/user").
    Query("id", "123").
    Header("Authorization", "Bearer xxx").
    JSON(map[string]interface{}{
        "name": "tom",
        "age":  18,
    }).
    Exec()
```

### 设计亮点

1. **可选模式（Functional Options）**：所有配置都是可选的，默认值合理
2. **自动 JSON 序列化**：调用 `.JSON()` 自动帮你 Marshal，自动加 Content-Type
3. **自动日志**：默认开启，自动打请求响应日志
4. **自动 OTEL 链路追踪**：默认开启，自动加 trace id
5. **错误自动截断**：响应体太大自动截断，避免日志爆炸

---

## 五、gRPC vs HTTP 中间件对比表

| 维度 | gRPC 中间件 | Gin HTTP 中间件 | HTTP Client 中间件 |
|-----|------------|----------------|-------------------|
| **核心接口** | `grpc.UnaryHandler` | `gin.HandlerFunc` | `http.RoundTripper` |
| **穿透调用** | `handler(ctx, req)` | `c.Next()` | `next.RoundTrip(req)` |
| **终止链** | `return error` | `c.Abort()` | `return error` |
| **上下文** | `context.Context` | `*gin.Context` | `*http.Request.Context()` |
| **拿到响应** | 直接 return `resp` | 包装 ResponseWriter | 包装 RoundTripper |
| **注册方式** | `grpc.ChainUnaryInterceptor` | `r.Use(middleware)` | `client.Transport = middleware` |
| **洋葱模型** | ✅ 是 | ✅ 是 | ✅ 是 |
| **前置+后置** | ✅ 都有 | ✅ 都有 | ✅ 都有 |

> 💡 **面试加分：核心思想完全一样！只是接口不一样！**
>
> 不管是 gRPC、Gin、Echo、Gin、还是 http.Client，中间件的思想都是洋葱模型，都是前置+穿透+后置，只是每个库的接口名字不一样而已。
>
> 理解了这个本质，任何框架的中间件你看一眼就会写。

---

## 六、❌ 常见坑 & 避坑指南

### 坑 1：Gin 日志中间件拿不到响应体

```go
// ❌ 错误：直接用 c.Writer，拿不到响应体
func badLogMiddleware(c *gin.Context) {
    c.Next()
    // 这里拿不到响应体！因为 Write 完就没了！
}

// ✅ 正确：包装 ResponseWriter，同时写入 buffer
type bodyLogWriter struct {
    gin.ResponseWriter
    body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
    w.body.Write(b)  // 同时写到 buffer 里
    return w.ResponseWriter.Write(b)
}
```

这是 Gin 最经典的坑，面试 10 个有 8 个不知道！

---

### 坑 2：Gin 忘记调用 c.Next()

```go
// ❌ 错误：没调用 c.Next()，后面的中间件和业务都不会执行
func badMiddleware(c *gin.Context) {
    fmt.Println("我只执行前置，不会调用后面的！")
}

// ✅ 正确：必须调用 c.Next()
func goodMiddleware(c *gin.Context) {
    fmt.Println("前置")
    c.Next()
    fmt.Println("后置")
}
```

gRPC 不调用 handler 也会有同样的问题，但 gRPC 里太明显了一般不会忘，Gin 里经常有人忘写 `c.Next()`。

---

### 坑 3：HTTP Client 中间件忘记把 Body 塞回去

```go
// ❌ 错误：读了 Body 但没塞回去，下游拿到的是空 Body
func badMiddleware(next http.RoundTripper) http.RoundTripper {
    return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
        reqBody, _ := io.ReadAll(req.Body)
        // 没塞回去！下游拿不到 Body！
        return next.RoundTrip(req)
    })
}

// ✅ 正确：读完再塞回去
func goodMiddleware(next http.RoundTripper) http.RoundTripper {
    return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
        reqBody, _ := io.ReadAll(req.Body)
        req.Body = io.NopCloser(bytes.NewBuffer(reqBody)) // 塞回去！
        return next.RoundTrip(req)
    })
}
```

---

## 七、美餐的 Gin 中间件默认顺序

```go
func GinMiddlewares(appName string, ...) []gin.HandlerFunc {
    return []gin.HandlerFunc{
        otelgin.Middleware(appName),  // 1. 链路追踪（最外层）
        recovery(recoverRenderFn),    // 2. Panic 捕获
        injectDBIntoCtx(),             // 3. DB 注入
        setTimeout(10*time.Second),    // 4. 超时控制
        serverLog(loggerOptions),      // 5. 日志
    }
}
```

和 gRPC 的最佳实践顺序完全一致！

---

## 八、📝 面试标准回答模板

### 面试官问：Gin 中间件和 gRPC 中间件有什么区别？你写过吗？

> "本质思想是完全一样的，都是洋葱模型，都是前置逻辑+穿透+后置逻辑，只是接口和实现方式不一样。
>
> Gin 中间件是 `gin.HandlerFunc` 类型，穿透调用是 `c.Next()`，可以用 `c.Abort()` 提前终止中间件链。
>
> gRPC 中间件是 `grpc.UnaryHandler` 类型，穿透调用是 `handler(ctx, req)`，终止就是直接 return error。
>
> 还有一个很重要的区别：Gin 要拿响应体打日志的话，必须包装 `ResponseWriter`，因为原生的 Writer 写完就没了，这是 Gin 最经典的坑；而 gRPC 的响应就是返回值，直接就能拿到。
>
> 另外 HTTP Client 也有中间件机制，基于 `http.RoundTripper` 接口，思想也是完全一样的，包装 Transport 就能给所有请求加日志、加超时、加链路追踪。
>
> 我在项目里写过好几个 Gin 中间件，比如 panic recover、日志、超时控制、DB 注入这些，和 gRPC 中间件的思想是通的，只是接口不一样而已。"

---

### 面试官追问：Gin 写日志中间件要注意什么？

> "最核心的就是必须包装 ResponseWriter！因为 Gin 原生的 ResponseWriter 的 Write 方法写完就没了，你拿不到响应体内容。
>
> 所以要写一个包装类，继承 `gin.ResponseWriter`，然后重写 Write 方法，把写入的字节同时复制一份到 buffer 里，这样日志中间件才能拿到响应体打日志。
>
> 另外还有请求体也是一样，`c.GetRawData()` 读了之后也要用 `io.NopCloser` 塞回去，不然业务 handler 拿到的是空 Body。"

---

## 九、一句话总结

> **所有中间件的本质都是洋葱模型：前置逻辑 + 穿透到下一层 + 后置逻辑**
>
> 不管是 gRPC、Gin、Echo、还是 http.Client，思想都是完全一样的，只是接口名字不一样而已。

---

## 📝 面试要点复盘

1. ✅ 所有中间件的本质：洋葱模型
2. ✅ Gin 中间件三步模板：前置 + c.Next() + 后置
3. ✅ gRPC vs Gin 中间件的区别
4. ✅ Gin 日志中间件的经典坑：必须包装 ResponseWriter
5. ✅ HTTP Client 中间件的实现：http.RoundTripper 接口
6. ✅ HTTP Client 中间件的坑：读了 Body 必须塞回去
7. ✅ 美餐的默认中间件顺序
