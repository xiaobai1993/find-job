# gRPC 中间件深度解析 - 面试版

> 🎯 核心考点：洋葱模型、Server 中间件、Client 中间件、执行顺序、常见坑
> 源码位置：`pkg/grpc/middleware/`

---

## 一、先搞懂核心原理：洋葱模型

这是理解一切的基础，90% 的人说不懂中间件，就是没搞懂这个模型。

```
请求进来的方向
    ↓
[中间件 A] → 前置逻辑（打日志、验证 token、加超时）
    ↓
[中间件 B] → 前置逻辑（透传 metadata、限流）
    ↓
[中间件 C] → 前置逻辑（recover 捕获 panic）
    ↓
[真正的业务 handler] 执行业务逻辑
    ↓
[中间件 C] → 后置逻辑（记录响应、上报 metrics）
    ↓
[中间件 B] → 后置逻辑（统计耗时）
    ↓
[中间件 A] → 后置逻辑（处理错误）
    ↓
响应回去的方向
```

> 💡 **核心洞察：** 每个中间件都能看到完整的请求和响应！
>
> 请求进来时从上往下穿，响应出去时从下往上穿，所以叫洋葱模型。

---

## 二、Server 端中间件：标准写法（100% 背下来）

所有 Server 中间件都是这个模板，一个字都不会差！

### 📝 标准模板

```go
func UnaryServerInterceptor(可选配置参数) grpc.UnaryServerInterceptor {
    // 1. 外层：返回一个函数类型
    return func(
        ctx context.Context,
        req interface{},
        info *grpc.UnaryServerInfo,  // ← 方法信息：FullMethod 就是 /xxx.User/GetUser
        handler grpc.UnaryHandler,   // ← 下一层：要么是下一个中间件，要么是真正的业务 handler
    ) (resp interface{}, err error) {

        // 2. 👇 这里是前置逻辑：请求进来，还没执行业务
        fmt.Println("请求进来了：", info.FullMethod)

        // 3. 👇 这一行是"穿过去"：调用下一层
        // 执行到这里，就进入洋葱的下一层
        resp, err = handler(ctx, req)

        // 4. 👇 这里是后置逻辑：业务已经执行完了，拿到响应了
        fmt.Println("请求出去了：", err)

        return resp, err
    }
}
```

**就这 4 步！所有 Server 中间件都是这个结构。**

---

### 🌰 例子 1：recover 中间件（美餐生产级代码）

```go
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
        // 前置：defer 注册 recover
        defer func() {
            if e := recover(); e != nil {
                // 后置：如果 panic 了，在这里捕获
                err = status.Errorf(codes.Internal, "panic error: %v", e)
                log.For(ctx).Error("panic error",
                    log.String("fullMethod", info.FullMethod),
                    log.NamedError("panicError", err),
                    log.Any("req", req),
                    log.Stack("panicStack"),
                )
            }
        }()

        // 穿过去执行业务
        return handler(ctx, req)
    }
}
```

**设计亮点：**
- 什么都没做，只是 defer 注册了一个 recover 函数
- 业务执行的时候如果 panic 了，defer 会在函数退出前捕获
- 把 panic 转换成正常的 Internal 错误返回给上层
- 这就是为什么 gRPC 服务 panic 不会挂掉的原因！

---

### 🌰 例子 2：超时中间件（美餐生产级代码）

```go
func UnaryServerInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
        // 前置：如果上游没传超时，加一个默认超时
        if _, ok := ctx.Deadline(); !ok {
            log.For(ctx).Debug("set default timeout", log.Int64("timeout", timeout.Milliseconds()))
            var cf context.CancelFunc
            ctx, cf = context.WithTimeout(ctx, timeout)
            defer cf()
        }

        // 穿过去执行业务
        return handler(ctx, req)
    }
}
```

> 💡 面试加分：为什么要判断 `ctx.Deadline()` 存不存在？
>
> 上游可能已经设置了更短的超时，尊重上游的设置，不要硬覆盖。如果上游没设置，才加一个默认的兜底超时。

---

### 🌰 例子 3：错误日志中间件

```go
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
        start := time.Now()

        // 穿过去执行业务
        resp, err = handler(ctx, req)

        // 后置：只有出错才打 error 日志，正常请求打 debug
        if err != nil {
            log.For(ctx).Error("grpc request failed",
                log.String("method", info.FullMethod),
                log.Duration("cost", time.Since(start)),
                log.Error(err),
            )
        }

        return resp, err
    }
}
```

---

## 三、Client 端中间件：标准写法（100% 背下来）

Client 端和 Server 端 **99% 一模一样**，只是参数多了几个！

### 📝 标准模板

```go
func UnaryClientInterceptor(可选配置参数) grpc.UnaryClientInterceptor {
    // 1. 外层：返回一个函数类型
    return func(
        ctx context.Context,
        method string,          // ← 方法名，比如 /xxx.User/GetUser
        req, reply interface{}, // ← 请求和响应
        cc *grpc.ClientConn,    // ← 客户端连接
        invoker grpc.UnaryInvoker, // ← 下一层：要么是下一个中间件，要么是真正发请求的 invoker
        opts ...grpc.CallOption, // ← 调用选项
    ) error {

        // 2. 👇 这里是前置逻辑：请求发出去之前
        fmt.Println("准备发请求：", method)

        // 3. 👇 这一行是"穿过去"：真正发 RPC 请求
        err := invoker(ctx, method, req, reply, cc, opts...)

        // 4. 👇 这里是后置逻辑：拿到响应了
        fmt.Println("响应回来啦：", err)

        return err
    }
}
```

**也是 4 步！和 Server 端完全一样的思想！**

---

### 🌰 例子 1：Metadata 透传中间件（美餐生产级代码）

```go
// DefaultWhitelist 默认会透传的 metadata
var defaultWhitelist = []string{
    "accept-language",
    "x-mc-image",
}

func UnaryClientInterceptor(whitelist ...string) grpc.UnaryClientInterceptor {
    whitelist = append(whitelist, defaultWhitelist...)
    return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        // 前置：从 incoming ctx 里拿出 metadata，复制到 outgoing ctx 里
        md, ok := metadata.FromIncomingContext(ctx)
        if !ok {
            return invoker(ctx, method, req, reply, cc, opts...)
        }

        for _, key := range whitelist {
            if values := md.Get(key); len(values) > 0 {
                kvs := make([]string, 2*len(values))
                for i, v := range values {
                    kvs[i*2], kvs[i*2+1] = key, v
                }
                ctx = metadata.AppendToOutgoingContext(ctx, kvs...)
            }
        }

        // 穿过去发请求
        return invoker(ctx, method, req, reply, cc, opts...)
    }
}
```

**面试点：为什么需要这个？**
> gRPC 的 context 有两个完全独立的 metadata：
> - `metadata.FromIncomingContext`：从上游请求里拿到的 metadata
> - `metadata.FromOutgoingContext`：发给下游请求的 metadata
>
> 这两个是分开的！不会自动透传，必须手动复制过去，不然下游服务拿不到上游传的语言、trace id 这些东西。

---

### 🌰 例子 2：Client 端超时中间件

```go
func UnaryClientInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
    return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        // 前置：给发往下游的请求加超时
        if _, ok := ctx.Deadline(); !ok {
            var cf context.CancelFunc
            ctx, cf = context.WithTimeout(ctx, timeout)
            defer cf()
        }

        return invoker(ctx, method, req, reply, cc, opts...)
    }
}
```

---

## 四、⚖️ Server vs Client 对比表

| 东西 | Server 中间件 | Client 中间件 |
|-----|-------------|--------------|
| **返回类型** | `grpc.UnaryServerInterceptor` | `grpc.UnaryClientInterceptor` |
| **下一层函数** | `handler grpc.UnaryHandler` | `invoker grpc.UnaryInvoker` |
| **方法信息** | `info *grpc.UnaryServerInfo` | `method string` |
| **返回值** | `(resp interface{}, err error)` | `error` |
| **核心模型** | 洋葱模型，前置+穿透+后置 | 洋葱模型，前置+穿透+后置 |
| **注册函数** | `grpc.ChainUnaryInterceptor` | `grpc.WithChainUnaryInterceptor` |

---

## 五、💥 最重要的点：执行顺序！

这是面试必问，也是最容易踩坑的地方。

### Server 端注册

```go
s := grpc.NewServer(
    // 注意顺序！先写的在外层，后写的在里层
    grpc.ChainUnaryInterceptor(
        recovery.UnaryServerInterceptor(),  // 第 1 层：最外层
        timeout.UnaryServerInterceptor(),   // 第 2 层
        log_error.UnaryServerInterceptor(), // 第 3 层
        auth.UnaryServerInterceptor(),      // 第 4 层：最里层
    ),
)
```

### 实际执行顺序

```
请求进来 → recovery 前置 → timeout 前置 → log 前置 → auth 前置
                                                          ↓
                                                       业务 handler
                                                          ↓
响应出去 ← recovery 后置 ← timeout 后置 ← log 后置 ← auth 后置
```

> ⚠️ **黄金法则：先注册的在外层！**
>
> 请求：先经过外层，后经过内层
> 响应：先经过内层，后经过外层

---

### ✅ 正确的中间件顺序（生产级最佳实践）

| 顺序 | 中间件 | 为什么放这里 |
|-----|--------|------------|
| 1 | **recover** | 最外层，才能捕获所有层的 panic |
| 2 | **日志/耗时统计** | 第二外层，统计最准确的总耗时 |
| 3 | **限流** | 流量控制，太早拒绝不浪费资源 |
| 4 | **超时控制** | 给整个请求链路加超时 |
| 5 | **Auth 验证** | 最后验证 token，验证不过直接返回 |
| 6 | 业务 handler | 最里层 |

---

### ❌ 错误的顺序后果

```go
// ❌ 大错特错：auth 在 recover 外面
grpc.ChainUnaryInterceptor(
    auth.UnaryServerInterceptor(),
    recovery.UnaryServerInterceptor(),  // 太靠里了！
)
```

**后果：** auth 中间件里如果 panic 了，recover 捕获不到！服务直接挂掉！

---

## 六、❌ 常见坑 & 避坑指南

### 坑 1：顺序错了导致 panic 没被捕获

上面已经说了，recover 永远放第一个！

---

### 坑 2：中间件里修改了 ctx 但是没传下去

```go
// ❌ 错误：改了 ctx 但是还是用原来的 ctx 调 handler
ctx, cf := context.WithTimeout(ctx, timeout)
defer cf()
return handler(oldCtx, req)  // 传的还是原来的 ctx！白改了！

// ✅ 正确：用新的 ctx
return handler(ctx, req)
```

---

### 坑 3：Client 端忘了把 incoming 转 outgoing

```go
// ❌ 错误：直接拿 incoming 的 ctx 发请求
md, _ := metadata.FromIncomingContext(ctx)
return invoker(ctx, method, ...)  // outgoing 里还是没有 metadata！下游拿不到！

// ✅ 正确：要 AppendToOutgoingContext
ctx = metadata.AppendToOutgoingContext(ctx, kvs...)
return invoker(ctx, method, ...)
```

> 💡 面试加分：这是 gRPC 最经典的新手坑！90% 的人第一次写 metadata 透传都会踩这个。
>
> 因为 incoming 和 outgoing 是两个完全独立的 namespace！不会自动复制！

---

### 坑 4：中间件里做阻塞操作

```go
// ❌ 错误：中间件里睡 1 秒，所有请求都慢 1 秒
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
    return func(...) {
        time.Sleep(1 * time.Second)  // 别这么干！
        return handler(ctx, req)
    }
}
```

中间件是每个请求都要走的，这里慢 1ms，整个服务就慢 1ms！

---

## 七、进阶：Stream 中间件（面试加分项）

上面说的都是 Unary（一元）中间件，还有 Stream（流式）中间件，写法稍微不一样，但思想完全一样：

```go
func StreamServerInterceptor() grpc.StreamServerInterceptor {
    return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
        // 前置逻辑

        // 穿过去
        err := handler(srv, ss)

        // 后置逻辑
        return err
    }
}
```

**和 Unary 的区别：** Stream 没有 `req` 和 `resp`，因为是流式的，一包一包发。

---

## 八、美餐项目里的全部中间件清单

| 目录 | 类型 | 作用 |
|-----|------|------|
| `auth/` | Server | Token 验证 |
| `language/` | Both | 语言透传 |
| `log_error/` | Server | 错误日志上报 |
| `md/` | Client | Metadata 透传 |
| `recover/` | Server | Panic 捕获 |
| `response_error/` | Server | 错误格式统一处理 |
| `timeout/` | Both | 超时控制 |

---

## 九、📝 面试标准回答模板

### 面试官问：说说 gRPC 中间件的原理？你写过吗？

> "gRPC 中间件用的是洋葱模型，本质是高阶函数套娃。
>
> 每一个中间件都是一个函数，它接收下一层的 handler 作为参数，返回一个新的 handler。
>
> 请求进来的时候从上往下一层层穿，每一层可以做前置逻辑，比如打日志、加超时、验证 token；
> 响应出去的时候从下往上一层层穿，每一层可以做后置逻辑，比如统计耗时、处理错误、上报 metrics。
>
> Server 端和 Client 端的写法几乎一模一样，都是四步：
> 1. 外层返回一个 intercept 函数
> 2. 前置逻辑：请求进来/发出去之前做处理
> 3. 调用 handler/invoker 穿透到下一层
> 4. 后置逻辑：拿到响应后做处理
>
> 注册的时候顺序非常重要，先注册的在外层，所以 recover 一定要放在第一个，这样才能捕获所有层的 panic。
>
> 我在项目里写过好几个中间件，比如 panic recover、超时控制、metadata 透传、错误日志上报这些，都是标准写法。
>
> 还有一个很经典的新手坑：Client 端透传 metadata 的时候，一定要记得把 incoming ctx 里的东西复制到 outgoing ctx 里，这两个是分开的，不会自动传。"

---

## 十、一句话总结

> **gRPC 中间件 = 洋葱模型 + 高阶函数 = 前置逻辑 + 穿透到下一层 + 后置逻辑**
>
> Server 和 Client 写法 99% 一样，就是参数名字不一样而已。

---

## 📝 面试要点复盘

1. ✅ 洋葱模型的执行顺序：请求从上到下，响应从下到上
2. ✅ Server 中间件四步模板：返回函数、前置、handler 穿透、后置
3. ✅ Client 中间件四步模板：参数多几个，思想完全一样
4. ✅ 注册顺序：先注册的在外层，recover 永远放第一个
5. ✅ 三个经典坑：顺序、ctx 没传下去、incoming/outgoing 搞混
6. ✅ Metadata 透传的坑：两个 namespace 是分开的，不会自动复制
