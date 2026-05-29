# API 网关 全解 + 支付场景应用（Go + Kong）

> 基于真实 Go 支付收银台项目（checkout），讲 Kong 外部网关 + gRPC 拦截器链内部网关的双层架构

---

## 一、基础概念

### 1. 为什么需要 API 网关？作用是什么？

API 网关是微服务的统一入口，所有请求先过网关，把公共逻辑集中处理，后端服务不用每个都写一遍。

**核心作用：**

1. **路由转发**：统一入口，根据路径、Header、参数路由到对应服务，调用方不需要知道内部地址
2. **统一鉴权**：token 校验、签名验证在网关层集中做
3. **限流熔断**：入口处做全局限流，防止流量打垮后端
4. **黑白名单**：IP 或用户黑名单，直接在网关拦截
5. **日志监控**：所有请求的访问日志、耗时、错误码统一采集
6. **链路追踪**：统一生成 traceId，透传到后端全链路
7. **协议转换**：对外 HTTPS，内部 gRPC，网关做转换
8. **幂等保障**：对支付等关键请求做幂等处理，防重复提交

> 网关是微服务的第一道防线，所有流量都经过这里，是高可用和安全的关键节点。

---

### 2. 你们用的什么网关？为什么选它？

**答：** 两层网关架构：

**第一层：Kong 外部网关（K8s Ingress）**

- Kong 部署在 Kubernetes，作为集群入口，通过 Ingress 资源配置路由
- 外部流量（三方回调、SDK 调用）先经过 Kong，再到内部服务
- 通过 KongPlugin 挂载限流、鉴权等插件

**第二层：checkout gRPC 拦截器链（业务网关层）**

- checkout 服务本身就是收银台网关，对外暴露 HTTP（三方回调）和 gRPC（内部 SDK 调用）两个端口
- gRPC 拦截器链承担了内部的鉴权、幂等、限流等逻辑

**为什么选 Kong：**

- 云原生，和 Kubernetes 生态原生集成，通过 CRD（KongPlugin / KongIngress）声明式配置，运维简单
- 插件丰富：限流、鉴权、日志、熔断开箱即用
- 支持 gRPC 和 HTTP 协议，适合我们的双协议场景
- Redis 后端做分布式限流，多个网关实例共享计数，全局准确

---

## 二、整体架构

### 3. checkout 服务的整体架构是什么？

```text
外部流量（三方支付回调 / 端上 SDK）
        ↓
  Kong API Gateway（K8s Ingress）
  - HTTPS 终止
  - Kong 限流插件（100 req/s per consumer，Redis 后端）
        ↓
  checkout 服务
  ├── HTTP Server（Gin）      ← 三方支付回调
  │   └── /v1/callback/
  │       ├── POST /wechat-notify/:configId
  │       ├── POST /alipay-notify/:configId
  │       ├── POST /abc-notify/:configId
  │       ├── POST /sqb-notify/:configId
  │       ├── POST /allinpay-notify/:configId
  │       └── POST /ccc-notify
  │
  └── gRPC Server            ← 内部 SDK / 商户端调用
      └── 拦截器链（按顺序执行）
          1. extractPlatformFromMD   ← 从 metadata 提取 platform
          2. setMchIDIntoCtx         ← 商户 ID 校验和注入
          3. buildSDKIdempotentInterceptor ← 幂等处理
          4. checkSDKCallerInterceptor    ← SDK 调用方校验（签名/token）
          5. checkPrivateInterceptor      ← 内部私有接口鉴权
          6. buildRateLimiterInterceptor  ← gRPC 层限流
              ↓
      各业务 Service Handler

基础设施：
  - PostgreSQL（GORM，读写分离，dbresolver）
  - Redis（缓存 + 分布式锁 + 限流计数）
  - Pulsar（事务消息 txmsg）
  - gRPC 连接池（下游：tpw / payment / subsidy / member 等多个服务）
  - OTLP（链路追踪 + 指标）
```

---

## 三、gRPC 拦截器链

### 4. gRPC 拦截器链的设计是什么？每一层做什么？

**注册入口：**

```go
// internal/net/grpc/interceptor/interceptor.go
func SetInterceptors(cfg *xgrpc.Config, p *infra.Provider) {
    interceptors := []grpc.UnaryServerInterceptor{
        extractPlatformFromMD,         // 1. 提取平台信息
        setMchIDIntoCtx,               // 2. 商户 ID 校验
        buildSDKIdempotentInterceptor(p), // 3. 幂等
        checkSDKCallerInterceptor,     // 4. SDK 调用方校验
        checkPrivateInterceptor,       // 5. 私有接口鉴权
        buildRateLimiterInterceptor(p),// 6. 限流
    }
}
```

**各层职责：**

| 拦截器 | 作用 | 生效范围 |
|--------|------|----------|
| `extractPlatformFromMD` | 从 gRPC metadata 提取 platform 存入 ctx | 所有请求 |
| `setMchIDIntoCtx` | 从 metadata 解析商户 ID，校验商户配置存在，注入 ctx | MchService / LegacyMchService / PayAfterMchService |
| `buildSDKIdempotentInterceptor` | Redis SetNX 幂等，防重复支付 | SDK 支付/充值/退款等写操作 |
| `checkSDKCallerInterceptor` | 校验调用方：用户 ID 或 PaymentSlipToken + RSA 签名 | SDK Service |
| `checkPrivateInterceptor` | clientID/clientSecret 校验，检查接口权限 | Private Service |
| `buildRateLimiterInterceptor` | Redis 令牌桶限流，按 method 单独配置阈值 | 配置了规则的 method |

---

### 5. gRPC 层幂等是怎么实现的？

**幂等的核心问题**：网络抖动时客户端重试，如果服务端重复处理，会导致重复扣款。

**实现方案：Redis SetNX + 请求指纹**

```go
// 1. 计算请求指纹
func calcRequestMD5(reqID, fullMethod string, req proto.Message) (string, error) {
    body, _ := proto.Marshal(req)

    var bf bytes.Buffer
    bf.WriteString(reqID)      // 客户端传的 requestID（来自 gRPC metadata）
    bf.WriteString(fullMethod) // 方法名，区分不同接口
    bf.Write(body)             // 请求体序列化

    sum := md5.Sum(bf.Bytes())
    return hex.EncodeToString(sum[:]), nil
}

// 2. SetNX 占位，防止并发重复处理
ok, err = p.CacheRedis.SetNX(ctx, respCacheKey(reqMD5), "1", 30*time.Second).Result()
if !ok {
    // 触发幂等：从 Redis 取上次的响应直接返回
    return idempotentHandle(ctx, p, reqMD5, req, info)
}

// 3. 处理完成后缓存响应（proto 序列化存 Redis）
respBytes, _ := proto.Marshal(resp.(proto.Message))
p.CacheRedis.Set(ctx, respCacheKey(reqMD5), respBytes, consts.IdempotentCacheExpiration)
```

**关键细节：**

- `requestID` 由客户端生成，存在 gRPC metadata 里，标识同一笔业务请求
- 指纹 = `MD5(requestID + method + proto(req))`，同一请求指纹相同
- 处理中状态用 `"1"` 占位，30s 超时，防止服务宕机后 key 永久占用
- 某些错误码（内部错误、超时）不缓存响应，客户端重试可以重新处理

---

### 6. SDK 调用方校验怎么做的？

SDK 调用支持两种调用方身份：

**类型一：用户直接调用**

```go
if commonv2pb.CallerType_CALLER_TYPE_USER == caller.GetType() {
    // 必须有 userId 或 clientMemberId 之一
    if caller.GetUserId() == nil && caller.GetClientMemberId() == nil {
        return buildErrorResp(info, resultcode.PermissionDenied, ...)
    }
}
```

**类型二：通过 PaymentSlipToken 调用（支付单 token）**

```go
} else if commonv2pb.CallerType_CALLER_TYPE_OTHER == caller.GetType() {
    // 1. 解析 PaymentSlipToken（JWT 格式，用商户 RSA 公钥验签）
    data, ok := checkPaymentSlipToken(ctx, token.GetValue())

    // 2. 校验 token 里的 paymentSlipID 和请求的 paymentSlipID 一致
    if data.PaymentSlipID != r.GetPaymentSlipId() { ... }

    // 3. 从 token 里提取 mchID
    mchID = data.MchID
}
```

**SDK 支付额外的 RSA 签名验证：**

```go
// 对 PayRequest，还要验证 RSA 签名，防止篡改支付金额
func checkSDKPaySign(ctx context.Context, req *sdkpb.PayRequest) (int64, bool) {
    // 从 metadata 提取签名参数（timestamp, nonceStr, signature, mchID）
    params, _ := sign.ParamsFromMD(md)

    // 用商户公钥（或全局支付公钥）验签
    verifier := signsdk.NewSDKPayVerifier(
        req.GetPaymentSlipId(), params.Timestamp, params.NonceStr,
        params.Signature, publicKey,
    )
    verifier.Verify()
}
```

---

### 7. 私有接口鉴权怎么做的？

内部服务调用 checkout 的私有接口，用 clientID + clientSecret 鉴权：

```go
func checkPrivateInterceptor(...) {
    // 1. 从 gRPC metadata 取 client-Id 和 client-secret
    clientID := md.Get("client-Id")
    clientSecret := md.Get("client-secret")

    // 2. 查客户端配置（从 DB 加载到内存），校验 secret
    clientConf := client.GetClient(clientID[0], clientSecret[0])

    // 3. 检查该客户端是否有权限访问这个方法
    if !clientConf.CheckAccessAuth(info.FullMethod) {
        return buildErrorResp(info, resultcode.PermissionDenied, ...)
    }
}
```

---

## 四、限流

### 8. 限流是怎么做的？有几层？

**两层限流：**

**第一层：Kong 插件限流（HTTP 入口）**

```yaml
# argo-sandbox/helm/templates/kong-rate-limiting.yaml
plugin: rate-limiting
config:
  second: 100          # 每秒 100 次
  policy: redis        # Redis 后端，分布式，多实例共享计数
  limit_by: consumer   # 按 consumer 维度限流
  fault_tolerant: true # Redis 故障时放行，不影响业务
```

**第二层：gRPC 拦截器限流（业务接口）**

```go
// internal/pkg/ratelimit/rate_limiter.go
// 底层用 go-redis/redis_rate，令牌桶算法，Lua 脚本原子执行
func (r *redisRateLimiter) Acquire(ctx context.Context, tokenBucketNum int, key string) bool {
    res, err := r.limit.Allow(ctx, key, redisrate.PerSecond(tokenBucketNum))
    if err != nil {
        return true // Redis 故障时放行，降级不限流
    }
    return res.Allowed > 0
}

// 按 method 维度配置限流规则，key = method+业务维度
func (l *Interceptor) Limit(ctx context.Context, fullMethod string, req any) bool {
    rule, ok := l.rule[fullMethod] // 没配置规则的接口不限流
    if !ok {
        return false
    }
    key := rateLimitRequest.RateLimitKey() // 业务 key，比如商户 ID
    return !l.limiter.Acquire(ctx, rule.TokenPerSecond, key)
}
```

**两层限流的分工：**

```text
Kong 层：入口总流量控制，防止恶意流量打穿，粒度是 consumer 维度
gRPC 层：接口级精细控制，按 method 配置不同阈值，粒度到商户/用户维度
```

---

## 五、HTTP 回调层

### 9. 三方支付回调是怎么处理的？

checkout 服务的 HTTP 层（Gin）专门处理三方支付回调，不做业务逻辑，只做透传：

```go
// internal/net/http/routes.go
callback := s.Group("/v1/callback")
callback.POST("/wechat-notify/:configId", h.WechatCallback.WechatNotifyToTpw)
callback.POST("/alipay-notify/:configId", h.AlipayCallback.AlipayNotifyToTPw)
callback.POST("/abc-notify/:configId",    h.AbcCallback.AbcPayNotifyToTPw)
callback.POST("/sqb-notify/:configId",    h.SqbCallback.SqbNotifyToTpw)
callback.POST("/allinpay-notify/:configId", h.AllinpayCallback.AllinpayNotifyToTPw)
callback.POST("/ccc-notify",              h.WalletNotify.WalletNotifyToTpw)
```

**以微信回调为例：**

```go
func (w *wechatCallback) WechatNotifyToTpw(ctx *gin.Context) {
    // 1. 解析 URL 参数（configId 区分商户配置）
    configId := ctx.Param("configId")

    // 2. 读取请求体
    body, _ := io.ReadAll(ctx.Request.Body)

    // 3. 提取微信签名头（Serial/Timestamp/Nonce/Signature），后续在 tpw 层验签
    callback := notifystruct.WechatCallback{
        WechatpaySerial:    ctx.Request.Header.Get("Wechatpay-Serial"),
        WechatpayTimestamp: ctx.Request.Header.Get("Wechatpay-Timestamp"),
        WechatpayNonce:     ctx.Request.Header.Get("Wechatpay-Nonce"),
        WechatpaySignature: ctx.Request.Header.Get("Wechatpay-Signature"),
        Body: string(body),
    }

    // 4. 透传给 tpw 服务（第三方支付通道服务）处理
    w.Tpw.Callback(reqCtx, &tpwpb.CallbackRequest{
        TpwConfigId:  configIdInt64,
        Body:         dataStr,
        CallbackType: commonpb.CallbackType_WECHAT_PAY,
    })

    // 5. 返回微信要求的成功格式
    ctx.JSON(http.StatusOK, struct{ Code string; Message string }{"SUCCESS", "成功"})
}
```

**设计原则**：HTTP 层只做接收和透传，不做验签，不做业务逻辑，验签和业务处理都在 tpw 服务里，checkout 只是"搬运工"。

---

## 六、下游服务连接管理

### 10. 下游 gRPC 服务怎么管理的？

用 **gRPC 连接池** 管理下游连接，避免频繁建连销连的开销：

```go
// internal/infra/provider.go
// payment 相关服务共享同一个连接池
paymentRPCPool, _ := grpcutils.NewGRPCPool(
    cfg.RPCClients.PaymentServiceAddr,
    poolConfig,
    grpcutils.WithJWT(app.Credentials.Identifier.JWT),
    grpcutils.WithTelemetry(app.Telemetry),
)

// 监控连接池状态
monitor := metrics.NewPoolMonitor(metrics.NewRPCPoolMetricInfo("payment", paymentRPCPool))
monitor.StartMonitoring()

// 多个 service client 复用同一个 pool
rechargeService, _             = rpcclient.NewRechargeService(paymentRPCPool)
paymentMeicanServiceClient, _  = rpcclient.NewPaymentMeicanServiceClient(paymentRPCPool)
paymentWebServiceClientV2, _   = rpcclient.NewPaymentWebServiceClient(paymentRPCPool)
paymentQuickServiceClientV2, _ = rpcclient.NewPaymentQuickServiceClient(paymentRPCPool)
```

连接池配置（来自 config）：

```text
InitConn:     初始连接数
MaxConn:      最大连接数
DialTimeout:  建连超时
IdleTimeout:  空闲连接超时（自动释放）
PingInterval: 心跳检测间隔（保活）
PingTimeout:  心跳超时
```

---

## 七、高可用与踩坑

### 11. 网关层怎么保证高可用？

**部署层面：**

1. **多实例部署**：checkout 在 K8s 上跑多个 Pod，前面是 Kong，单个 Pod 挂了不影响整体
2. **HPA 自动扩容**：配置了 HPA，流量高峰时自动扩 Pod，配合 Kong 的限流防止扩太慢时被打穿
3. **Argo Rollout 灰度发布**：用 Argo Rollout 做金丝雀发布，新版本先接少量流量，观察没问题再全量，出问题立刻回滚
4. **优雅退出**：K8s 滚动更新时，收到 SIGTERM 先停止接收新请求，等存量请求处理完再退出

**限流降级：**

- Redis 故障时，Kong 的 `fault_tolerant: true` 让限流降级放行，不影响业务
- gRPC 层限流 Redis 故障时也降级放行（`return true`），业务优先

**连接池监控：**

- gRPC 连接池有 metrics 监控，连接数异常告警

---

### 12. 踩过什么坑？

**坑 1：微信回调请求体超过默认限制**

微信回调的 body 里包含加密的业务数据，有时比较大，超过了默认的请求体大小限制，网关直接返回 413，微信重试几次后放弃，支付结果迟迟不更新。

- 解决：针对回调接口调大请求体限制，并在 Kong 层也同步调整 `client_max_body_size`

**坑 2：gRPC 幂等的错误码要细分**

最初所有错误都缓存响应，导致内部错误（数据库超时、下游不可用）也被缓存，客户端重试拿到的是旧的错误响应，实际上问题已经恢复了，但还是返回错误。

- 解决：区分不缓存的错误码（`InternalError`、`WalletPayTimeout`），这些错误不缓存，让客户端能真正重试：

```go
var errorCodesForNotCachingResp = map[int32]struct{}{
    resultcode.InternalError.Value():    {},
    resultcode.WalletPayTimeout.Value(): {},
}
```

**坑 3：gRPC 拦截器链顺序出错导致鉴权绕过**

早期拦截器顺序不对，幂等拦截器在鉴权之前执行，导致未鉴权的请求也触发了幂等逻辑，泄露了 Redis key 的状态信息。

- 解决：严格约定拦截器顺序，鉴权拦截器必须在幂等拦截器之前，且用代码注释标注顺序依赖关系

**坑 4：限流 key 设计不合理**

最初限流 key 只用了 method 名，同一个 method 全局共享一个计数，导致一个大商户的高频请求把其他所有商户的配额都用完了，其他商户报限流错误。

- 解决：限流 key 改为 `method + 商户 ID`，每个商户独立计数，互不影响

---

## 面试答题技巧

网关这块，重点突出三点：

1. **双层架构**：Kong 做入口流量控制，gRPC 拦截器链做业务层精细控制，各层分工清晰，说出来就知道你对网关设计有系统性思考

2. **幂等设计**：支付场景幂等是核心，Redis SetNX + 请求指纹 + 区分不缓存的错误码，这套组合能说清楚就非常有说服力

3. **踩坑经历**：特别是错误码区分不缓存、限流 key 粒度这两个坑，都是真实踩过的，面试官一听就知道是生产经验，不是背题
