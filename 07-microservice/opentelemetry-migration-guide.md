# OpenTracing → OpenTelemetry 升级指南

> 📌 Go 语言微服务可观测性升级实践
> ⏰ 文档编写时间：2023-02-07
> 🔗 SDK: open-telemetry/opentelemetry-go

---

## 一、为什么要升级？

### 面试官问：你们用什么做链路追踪？OpenTracing 和 OpenTelemetry 有什么区别？

> **标准答案：**
>
> "我们之前用的是 OpenTracing，后来全面升级到了 OpenTelemetry（简称 OTEL）。
>
> **核心区别：**
>
> **OpenTracing 只是一个标准**，只定义了 Tracing API，具体实现还是要自己接 Jaeger、Zipkin 这些。
>
> **OpenTelemetry 是 CNCF 的终极方案**，它把三件事统一了：
> 1. ✅ **Trace** - 链路追踪（继承 OpenTracing）
> 2. ✅ **Metrics** - 指标监控（继承 OpenCensus）
> 3. ✅ **Logs** - 日志（正在完善）
>
> **为什么要升级：**
> - OpenTracing 已经不维护了，项目归档了
> - OTEL 是行业标准，所有云厂商都支持
> - 一次埋点，三个信号都能出，不需要重复埋
> - 自动 Instrumentation，很多框架自带集成"

---

## 二、版本要求

| 组件 | 版本 | 说明 |
|-----|------|------|
| Go | ≥ 1.18 | OTEL SDK 用了泛型 |
| go.opentelemetry.io/otel | v1.12.0 | 核心 SDK |
| go.opentelemetry.io/otel/sdk | v1.12.0 | Tracer SDK |
| go.opentelemetry.io/otel/exporters/otlp/otlptrace | v1.16.0 | OTLP 协议导出 |

> ⚠️ 注意：Jaeger exporter 已经 deprecated 了，推荐用 OTLP 协议

---

## 三、升级步骤

### 1. 初始化 Tracer Provider

```go
package main

import (
    "context"
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

func initTracer(ctx context.Context, serviceName string) (*sdktrace.TracerProvider, error) {
    // 1. 创建 OTLP Exporter (gRPC 方式)
    exporter, err := otlptracegrpc.New(ctx)
    if err != nil {
        return nil, err
    }

    // 2. 定义资源元数据
    res, err := resource.New(ctx,
        resource.WithAttributes(
            semconv.ServiceNameKey.String(serviceName),  // 服务名，Jaeger 里显示的
        ),
    )
    if err != nil {
        return nil, err
    }

    // 3. 创建 Tracer Provider
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),          // 批量发送
        sdktrace.WithResource(res),              // 服务元数据
        sdktrace.WithSampler(sdktrace.AlwaysSample()), // 采样策略
    )

    // 4. 设置为全局 Tracer
    otel.SetTracerProvider(tp)

    return tp, nil
}
```

### 2. 优雅关闭

```go
func main() {
    ctx := context.Background()

    // 初始化
    tp, err := initTracer(ctx, "payment-service")
    if err != nil {
        log.Fatal(err)
    }

    // 程序退出时 flush
    defer func() {
        if err := tp.Shutdown(ctx); err != nil {
            log.Printf("Tracer shutdown error: %v", err)
        }
    }()

    // ... 业务代码
}
```

### 3. 创建 Span（和 OpenTracing 很像）

```go
tracer := otel.Tracer("my-tracer")

// 开始一个 Span
ctx, span := tracer.Start(ctx, "handlePayment")
defer span.End()

// 打标签
span.SetAttributes(
    attribute.String("order_id", orderID),
    attribute.Int64("amount", amount),
)

// 记录事件
span.AddEvent("processing payment")

// 记录错误
if err != nil {
    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
}
```

---

## 四、常用框架集成

### 1. Gin HTTP 服务

```go
import (
    "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
    r := gin.New()

    // 加中间件就行，自动追踪所有 HTTP 请求
    r.Use(otelgin.Middleware("payment-service"))

    r.GET("/health", func(c *gin.Context) {
        c.String(200, "ok")
    })
}
```

> ✅ 自动提取的信息：HTTP Method、URL、StatusCode、User-Agent、IP

### 2. gRPC 服务

```go
import (
    "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

func main() {
    s := grpc.NewServer(
        grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()),
        grpc.StreamInterceptor(otelgrpc.StreamServerInterceptor()),
    )
}
```

### 3. gRPC 客户端

```go
conn, err := grpc.Dial(
    "localhost:50051",
    grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
)
```

### 4. GORM 数据库

```go
import (
    "go.opentelemetry.io/contrib/instrumentation/gorm.io/gorm/otelgorm"
)

db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
db.Use(otelgorm.NewPlugin())
```

> ✅ 自动追踪 SQL 语句、执行时间、错误

---

## 五、Context 传递（最重要！）

**跨进程 TraceID 传递：**

```go
// HTTP 客户端：自动把 Trace 注入 Header
req, _ := http.NewRequest("GET", url, nil)
req = req.WithContext(ctx)  // 把带 Span 的 ctx 传进去

// gRPC 客户端：自动把 Trace 注入 Metadata
// otelgrpc 拦截器会自动处理，不需要写代码
```

**跨 Goroutine 传递：**

```go
// ✅ 正确：ctx 要传进去
go func(ctx context.Context) {
    _, span := tracer.Start(ctx, "asyncTask")
    defer span.End()
}(ctx)  // 把父 ctx 传进去

// ❌ 错误：用了 background，链路断了
go func() {
    ctx := context.Background()  // 断了！
    _, span := tracer.Start(ctx, "asyncTask")
}()
```

---

## 六、采样策略

```go
// 1. 全采样（开发环境）
sdktrace.WithSampler(sdktrace.AlwaysSample())

// 2. 不采样（压测环境）
sdktrace.WithSampler(sdktrace.NeverSample())

// 3. 概率采样（生产环境，比如 30%）
sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.3))

// 4. 父 Span 有就采样，没有就不采（默认）
sdktrace.WithSampler(sdktrace.ParentBased(
    sdktrace.TraceIDRatioBased(0.3),
))
```

> 💡 生产环境建议：30% 采样率足够排查问题，还能省存储成本。

---

## 七、面试常见问题

### Q1: OTEL 和 Jaeger 是什么关系？
> A: Jaeger 是 **后端存储和 UI**，OTEL 是 **客户端 SDK + 协议**。现在的架构是：
> ```
> 业务代码 → OTEL SDK 埋点 → OTLP 协议 → Jaeger Collector → Jaeger UI
> ```

### Q2: OTEL 和 OpenCensus 是什么关系？
> A: OpenCensus 是 Google 搞的，OpenTracing 是 Uber 搞的，两家合并了就是 OpenTelemetry，是终极方案。

### Q3: 自动 Instrumentation 和手动埋点怎么选？
> A: 框架层 HTTP/gRPC/DB 用自动埋点就行，关键业务路径手动加自定义 Span，埋业务属性。

---

## 🎯 面试记忆点

```
OpenTelemetry = 行业标准
├── Trace 链路追踪    ← 已稳定
├── Metrics 监控指标  ← 已稳定
└── Logs 日志        ← 完善中

升级理由：
├── OpenTracing 已归档，不再维护
├── OTEL 是 CNCF 毕业项目，所有云厂商支持
└── 一次埋点，三个信号都能出，不需要重复工作
```
