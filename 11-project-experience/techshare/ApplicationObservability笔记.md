# Application Observability 技术分享笔记

> 美餐内部技术分享，覆盖 Metrics（Prometheus）、Traces（Jaeger）两大可观测性支柱，
> 附真实生产 Case 分析。

---

## 一、可观测性三大支柱

```
Logs    → 发生了什么事（记录具体事件）
Metrics → 系统当前状态/趋势（聚合数字）
Traces  → 一个请求经过了哪些服务、耗时如何（调用链）
```

---

## 二、Metrics —— Prometheus 体系

### 2.1 四种 Metric 类型

| 类型 | 用途 | 关键点 |
|------|------|--------|
| **Counter** | 只增不减的计数器 | 请求总数、错误总数 |
| **Histogram** | 延迟分布、请求大小分布 | 最常用，自带 `_sum`/`_count`，可算百分位 |
| Gauge | 可升可降的当前值 | goroutine 数、内存使用 |
| Summary | 客户端计算百分位 | 生产不推荐，多实例聚合困难 |

> Counter 和 Histogram 是日常最常用的两种。

### 2.2 Prometheus Client 架构

```
Metrics Family ──register──→ Registry ──create──→ Gatherer ──serve──→ HTTP Server
      ↑                                                 │
      └─────────────────── write ─────────────────────┘
```

- **Metrics Family**：你定义的具体指标（如 `grpc_server_handling_seconds`）
- **Registry**：注册中心，管理所有指标
- **Gatherer**：采集时从 Registry 读数据
- **HTTP Server**：暴露 `/metrics` 端点给 Prometheus 拉取

### 2.3 内部实现：`xtelemetry` 库

定义指标（Histogram）：

```go
var grpcServerMetrics = &GrpcServerMetrics{
    handledHistogramVec: promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Subsystem: subsystem,
            Name:      "grpc_server_handling_seconds",
            Help:      "Histogram of response latency (seconds) of gRPC...",
            Buckets:   defaultBuckets,
        },
        []string{"app", "grpc_type", "grpc_service", "grpc_method", "grpc_status_code"},
    ),
}

// 每次请求结束时调用
func (m *GrpcServerMetrics) Handled(appName, svcType, svcName, method, code string, dur time.Duration) {
    ob := mustGetObserver(m.handledHistogramVec, appName, svcType, svcName, method, code)
    ob.Observe(dur.Seconds())
}
```

启动独立的 telemetry HTTP server：

```go
func (tele *Telemetry) StartServer() error {
    mux := http.NewServeMux()
    mux.Handle(metricsEndpoint, promhttp.Handler()) // 暴露 /metrics
    // ...
}
```

### 2.4 指标采集全链路

```
Rollout/Deployment/StatefulSet
          ↓
        Pod（容器暴露 :18888/metrics）
          ↓
    Telemetry Service（K8s Service，port: http-telemetry）
          ↓
     Prometheus / Thanos（定时拉取）
          ↓
  Grafana / Thanos UI / Alerts
```

**配置三件套**：

```
rollout.yaml          → containerPort: 18888, name: metrics
service.yaml          → port: http-telemetry → targetPort: 18888
service-monitor.yaml  → ServiceMonitor，endpoint: http-telemetry
                         匹配标签 stage: telemetry
```

`ServiceMonitor` 是 Prometheus Operator 的 CRD，告诉 Prometheus 去哪里抓指标。

### 2.5 Grafana 查询示例

数据库事务 P90 延迟（毫秒），按 commit/rollback 分组：

```promql
histogram_quantile(
  0.90,
  sum(rate(gdb_dbm_transaction_seconds_bucket{service=~"$service"}[1m]))
  by (le, tx_status)
) * 1000
```

图表效果：commit P90 ≈ 35ms，rollback P90 ≈ 20ms。

### 2.6 Metrics 最佳实践

**1. 标签数量爆炸问题**

```
{grpc_method} × {grpc_service} × {grpc_status_code} × 10个bucket
= 指标数量指数增长
```

> 指标数量超过 **5000** 条 series，ServiceMonitor 会直接丢弃！

- 标签要精简，绝对不能加高基数标签（user_id、request_id 等）
- Histogram 已经包含了 `_sum` 和 `_count`，不要重复定义
- Histogram 是近似值，不要期望 100% 精确

---

## 三、Traces —— Jaeger 体系

### 3.1 核心概念

```
Trace = 一次完整请求的调用树
Span  = 树上的一个节点（一次具体操作）
```

每个 Span 携带：
```
{trace-id} : {span-id} : {parent-span-id} : {flags}
```

真实 Jaeger UI 例子（payment-adapter 的一次请求）：

```
payment-adapter: /api/order/transaction/show   135.1ms  17 spans
├── payment-adapter Http Request               58.42ms
├── payment-adapter gRPC→PaymentBizService     74.14ms
│   ├── meican-pay.payment-biz.prod            66.11ms
│   │   ├── gorm:dao.IdMapping                  2.12ms
│   │   ├── payment.app-meican-pay              7.41ms
│   │   │   ├── gorm:FindByOrder                4.21ms
│   │   │   └── gorm:balance                    1.79ms
│   │   └── subsidy.v2.SubsidySer...            4.79ms
│   └── clientsettingv1.ClientS...             32.07ms
```

一眼看出哪个环节慢、调了哪些下游。

### 3.2 Context Propagation（上下文传播）

Trace 信息通过 HTTP Header 或 gRPC metadata 在服务间传递（B3 格式）：

```
Client Tracer               HTTP/gRPC Header              Server Tracer
┌──────────────┐           ┌──────────────────┐           ┌──────────────┐
│ TraceId      │──inject──→│ X-B3-TraceId     │──extract─→│ TraceId      │
│ ParentSpanId │           │ X-B3-ParentSpanId │           │ ParentSpanId │
│ SpanId       │           │ X-B3-SpanId      │           │ SpanId       │
│ Sampling     │           │ X-B3-Sampled     │           │ Sampling     │
└──────────────┘           └──────────────────┘           └──────────────┘
```

B3 是 Zipkin 定义的标准 Header 格式，Jaeger 兼容。

### 3.3 Go 代码实现

**HTTP 客户端侧**（发请求时注入 trace 信息）：

```go
func makeSomeRequest(ctx context.Context) {
    if span := opentracing.SpanFromContext(ctx); span != nil {
        httpReq, _ := http.NewRequest("GET", "http://myservice/", nil)
        // 把 span 信息注入到 HTTP header
        opentracing.GlobalTracer().Inject(
            span.Context(),
            opentracing.HTTPHeaders,
            opentracing.HTTPHeadersCarrier(httpReq.Header),
        )
        httpClient.Do(httpReq)
    }
}
```

**HTTP 服务端侧**（收请求时提取 trace 信息）：

```go
http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
    // 从 header 里提取父 span context
    wireContext, _ := opentracing.GlobalTracer().Extract(
        opentracing.HTTPHeaders,
        opentracing.HTTPHeadersCarrier(req.Header),
    )
    // 创建本服务的 span，挂在父 span 下
    serverSpan = opentracing.StartSpan(operationName, ext.RPCServerOption(wireContext))
    defer serverSpan.Finish()
    // 存入 context，传给后续逻辑
    ctx := opentracing.ContextWithSpan(context.Background(), serverSpan)
    // ...
})
```

**gRPC 客户端侧**（拦截器自动处理）：

```go
func UnaryClientInterceptor(opts ...Option) grpc.UnaryClientInterceptor {
    return func(parentCtx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        // 1. 创建子 span，inject 到 gRPC metadata
        newCtx, clientSpan := newClientSpanFromContext(parentCtx, o.tracer, method)
        // 2. 调用下游
        err := invoker(newCtx, method, req, reply, cc, opts...)
        // 3. 结束 span
        finishClientSpan(clientSpan, err)
        return err
    }
}
```

`newClientSpanFromContext` 内部逻辑：

```go
// 从 ctx 取父 span
parent := opentracing.SpanFromContext(ctx)
// 创建子 span，ChildOf 关联父子关系
clientSpan := tracer.StartSpan(fullMethodName, opentracing.ChildOf(parentSpanCtx), ...)
// 注入到 gRPC metadata（底层也是 HTTP header 格式）
tracer.Inject(clientSpan.Context(), opentracing.HTTPHeaders, metadataTextMap(md))
// 存入 context
return opentracing.ContextWithSpan(ctxWithMetadata, clientSpan), clientSpan
```

**gRPC 服务端侧**（拦截器）：

```go
func UnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        // 从 gRPC metadata 里 extract 父 span context
        newCtx, serverSpan := newServerSpanFromInbound(ctx, o.tracer, o.traceHeaderName, info.FullMethod)
        resp, err := handler(newCtx, req)
        finishServerSpan(ctx, serverSpan, err)
        return resp, err
    }
}
```

**span 存取 context 的底层**：

```go
// Get：从 context 里取 span
func SpanFromContext(ctx context.Context) Span {
    val := ctx.Value(activeSpanKey)
    if sp, ok := val.(Span); ok { return sp }
    return nil
}

// Set：把 span 存入 context
func ContextWithSpan(ctx context.Context, span Span) context.Context {
    return context.WithValue(ctx, activeSpanKey, span)
}
```

> 这就是为什么 Go 里到处传 `ctx`：ctx 是 trace span 的载体。

### 3.4 Trace 采集全链路

```
Node
├── Pod
│   ├── Container ──UDP──→ Jaeger Agent（DaemonSet，每个 Node 一个）
│   └── Container ──UDP──→ Jaeger Agent
│
└── Jaeger Agent ──send──→ Jaeger Collector → ElasticSearch → JaegerUI
```

- App 发 trace 到**本机** Jaeger Agent（UDP，不阻塞业务）
- Agent 批量转发给中心 Collector
- Collector 存 ElasticSearch
- JaegerUI 查询展示

### 3.5 Traces 最佳实践

**1. 标准 Tag 规范**

```go
// 错误标记
if err != nil {
    ext.Error.Set(serverSpan, true)
    serverSpan.LogFields(
        log.String("event", "error"),
        log.String("message", err.Error()),
    )
}

// HTTP 标准 tag
ext.HTTPMethod.Set(span, req.Method)      // → "http.method"
ext.HTTPUrl.Set(span, req.URL.String())   // → "http.url"
// HTTP 状态码用 "http.status_code"
```

**2. 采样策略**

生产默认采样率 **1‰**，不能全量记录（流量太大）。

```
不要手动设置 JAEGER_SAMPLER_TYPE 环境变量
→ 用远程采样（Remote Sampling），服务端统一配置
```

Remote Sampling 配置示例：

```yaml
sampling:
  options:
    service_strategies:
      - service: jaeger-example
        type: probabilistic
        param: 0.1     # 该服务 10% 采样
        operation_strategies:
          - operation: /getTime
            param: 0.2   # 这个接口 20%
          - operation: /drivers
            param: 0.4
    default_strategy:
      type: probabilistic
      param: 0.2
      operation_strategies:
        - operation: /health-check
          param: 0.0   # 健康检查不采样
        - operation: /metrics
          param: 0.0
        - operation: /healthz
          param: 0.0
```

**强制采样**（排查问题时）：

```go
span := opentracing.SpanFromContext(ctx)
ext.SamplingPriority.Set(span, 999)  // 强制这条链路被采样
```

**3. 特别注意：OpenTracing 已废弃**

> OpenTracing 已经废弃，应迁移到 **OpenTelemetry（OTel）**。
> OTel 是 CNCF 推出的统一标准，同时覆盖 Metrics + Traces + Logs。

---

## 四、真实 Case：诡异的"网络问题"

### 背景

对某 gRPC 服务做 benchmarking 压测，在 Jaeger 里发现可疑 trace。

### 现象

```
client.nation-client → IDMappingService/GetByLegacyID  总耗时 65.67ms

Client 端 span 日志:
  @ 9.55ms   message SENT       ← 客户端发出请求
  @ 65.63ms  message RECEIVED   ← 客户端收到响应

Server 端 span 日志:
  @ 58.44ms  message RECEIVED   ← 服务端才收到请求！
  @ 63.42ms  message SENT
```

客户端 **9.55ms 发出**，服务端 **58.44ms 收到**，中间消失了 **~49ms**。

看起来像网络抖动！

### 排查过程

**Step 1：查 DNS**

Grafana 上 DNS P99 响应 ~1.5ms，P90/P50 均正常，排除 DNS 问题。

**Step 2：查 CPU**

发现 `client.*` Pod 的 CPU 使用率**持续打满**（接近 limit 2 core）。

### 根因

**不是网络问题，是 CPU 打满导致的调度延迟（CPU Throttling + Scheduling Delay）。**

```
9.55ms   → 客户端 goroutine 发出 gRPC 请求
          → CPU 满负荷，goroutine 被操作系统调度出去（preempted）
          → 等待重新被调度...
58.44ms  → 服务端收到并处理完，发回响应
65.63ms  → 客户端 goroutine 重新被调度，读到响应
```

Jaeger 记录的是 **goroutine 执行时的时钟**，不是真实网络传输时间。
goroutine 被调度走的这段时间，对 trace 来说就是"消失"的 49ms。

### 解决方案

给 client 服务设置 **rate limit**，控制并发请求数，让 CPU 不再跑满。

### 经验教训

> 看到 trace 里"莫名其妙的网络延迟"，先检查 CPU 使用率。
> CPU throttling 导致的 scheduling delay 会被误判为网络问题。

---

## 五、总结对照表

| 技术 | 用途 | 核心要点 |
|------|------|---------|
| **Prometheus** | 指标收集 | Counter/Histogram 最常用；标签控制在 5000 series 以内 |
| **Grafana** | 指标可视化 | `histogram_quantile` 算百分位；Thanos 做长期存储 |
| **Jaeger** | 分布式链路追踪 | span 通过 ctx 传递；X-B3 Header 跨服务传播；DaemonSet Agent 收集 |
| **OpenTelemetry** | 未来方向 | OpenTracing 已废弃，迁移到 OTel |

**面试可说的点**：

1. Histogram 标签设计：`{grpc_method} × {service} × {status_code}` 会指数增长，超 5000 被丢弃
2. gRPC tracing：客户端+服务端都要装 interceptor，用 inject/extract 串联链路
3. 生产采样 1‰，强制采样用 `SamplingPriority=999`，健康检查等接口配 0.0
4. CPU throttling 会伪装成网络延迟，trace 的时间戳是 goroutine 执行时间而非真实 IO 时间
5. OpenTracing → OpenTelemetry 迁移背景
