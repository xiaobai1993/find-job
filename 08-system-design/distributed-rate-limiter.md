# 高可用分布式限流器设计（Go + K8s）

> 对应面试题 Q47，完整方案整理

---

## 一、需求澄清（面试先问）

```
1. 限流粒度？per user / per IP / per API / 全局
2. QPS 量级？万级还是百万级？
3. 精确度要求？严格不超（支付）还是近似 OK（推荐）？
4. Redis 挂了怎么办？fail-open 还是 fail-closed？
5. 限流规则需要热更新吗？
```

---

## 二、算法选型

| 算法 | 优点 | 缺点 | 适用 |
|------|------|------|------|
| 固定窗口 | 最简单 | 窗口边界可 2 倍突发 | 不推荐 |
| 滑动窗口日志（ZSET） | 最精确 | 内存大，每请求存一条 | 低 QPS |
| 滑动窗口计数 | 精确、内存小 | 略有误差 | 严格限流 |
| **令牌桶** | 允许合理突发，最实用 | 实现稍复杂 | API 限流首选 |
| 漏桶 | 最平滑 | 不允许突发 | 流量整形 |

**结论：令牌桶首选，Redis + Lua 原子实现。**

---

## 三、架构选型

### 方案 A：纯 Redis 中心化

```
所有 Pod → Redis Lua 脚本 → Allow/Deny
```

- 优：简单、精确
- 缺：Redis 在请求关键路径，延迟 +1-3ms；Redis 是单点风险

### 方案 B：本地计数 + 异步同步（推荐高 QPS 场景）

```
Pod1 本地计数 ──→ 每 100ms 推到 Redis ←── Pod2 本地计数
                         ↓
                    各 Pod 定期读全局总量 → 更新本地缓存
```

- 优：请求路径完全不碰 Redis，延迟接近 0
- 缺：近似限流，最多落后 100ms

### 方案 C：Sidecar / Envoy（K8s 原生）

- Istio/Envoy 在流量层做限流，业务代码零侵入
- 缺：灵活性低，复杂规则难实现

**面试推荐答：方案 A + circuit breaker + 本地限流兜底（精确场景）；超高 QPS 用方案 B。**

---

## 四、核心实现

### 4.1 令牌桶 — Redis Lua 脚本

```lua
-- KEYS[1]: rate_limit:{key}
-- ARGV[1]: capacity（桶容量）
-- ARGV[2]: rate（每秒放令牌数）
-- ARGV[3]: now（毫秒时间戳）
-- ARGV[4]: requested（本次消耗令牌数）

local bucket   = redis.call('hmget', KEYS[1], 'tokens', 'last_ms')
local tokens   = tonumber(bucket[1]) or tonumber(ARGV[1])
local last_ms  = tonumber(bucket[2]) or tonumber(ARGV[3])
local capacity = tonumber(ARGV[1])
local rate     = tonumber(ARGV[2])
local now      = tonumber(ARGV[3])

-- 按时间差补充令牌
local elapsed = (now - last_ms) / 1000.0
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = 0
if tokens >= tonumber(ARGV[4]) then
    tokens  = tokens - tonumber(ARGV[4])
    allowed = 1
end

redis.call('hmset', KEYS[1], 'tokens', tokens, 'last_ms', now)
redis.call('pexpire', KEYS[1], math.ceil(capacity / rate) * 2000)
return allowed
```

### 4.2 滑动窗口计数 — 两种实现

#### ZSET 精确版（低 QPS 场景）

```
每次请求：
  ZADD  rate:{user}  <now_ms>  <uuid>                   # 插入本次请求
  ZREMRANGEBYSCORE rate:{user} 0 <now_ms - window_ms>   # 删过期记录
  ZCARD rate:{user}                                       # 统计窗口内数量
  如果 count < limit → 放行，否则拒绝
```

缺点：每个请求存一条记录，高 QPS 下内存爆炸。

#### 双窗口近似版（高 QPS 推荐）

核心思路：只存当前和上一个固定窗口的计数，加权估算。

```
时间轴：
  │     上个窗口(prev)      │     当前窗口(cur)      │
  ├────────────────────────┼────────────────────────┤
  0s                      60s                  now=75s

now=75s，窗口=60s：
  当前窗口已过 15s，上个窗口还有 75% 落在滑动窗口内
  估算请求数 = prev_count × 0.75 + cur_count
```

```lua
-- KEYS[1]: 上个窗口 key, KEYS[2]: 当前窗口 key
-- ARGV[1]: now(秒), ARGV[2]: window(秒), ARGV[3]: limit, ARGV[4]: cur_slot

local prev_count = tonumber(redis.call('get', KEYS[1])) or 0
local cur_count  = tonumber(redis.call('get', KEYS[2])) or 0
local elapsed_ratio = (tonumber(ARGV[1]) - tonumber(ARGV[4])) / tonumber(ARGV[2])
local estimated     = prev_count * (1 - elapsed_ratio) + cur_count

if estimated < tonumber(ARGV[3]) then
    redis.call('incr', KEYS[2])
    redis.call('expire', KEYS[2], tonumber(ARGV[2]) * 2)
    return 1
end
return 0
```

只用 2 个 key，内存 O(1)，误差小于 1 个窗口的请求数。

### 4.3 Go 接口设计

```go
type RateLimiter interface {
    Allow(ctx context.Context, key string) (bool, error)
}

type DistributedLimiter struct {
    rdb    *redis.Client
    script *redis.Script
    local  *LocalLimiter   // 本地兜底
    cb     *CircuitBreaker // 熔断器
    cfg    LimiterConfig
}

func (d *DistributedLimiter) Allow(ctx context.Context, key string) (bool, error) {
    // Redis 熔断时降级到本地限流
    if d.cb.IsOpen() {
        return d.local.Allow(ctx, key)
    }

    ctx, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
    defer cancel()

    nowMs := time.Now().UnixMilli()
    result, err := d.script.Run(ctx, d.rdb,
        []string{"rl:" + key},
        d.cfg.Capacity, d.cfg.Rate, nowMs, 1,
    ).Int()

    if err != nil {
        d.cb.RecordFailure()
        return d.local.Allow(ctx, key) // fail-open：本地兜底
    }

    d.cb.RecordSuccess()
    return result == 1, nil
}
```

---

## 五、HA 设计

### 5.1 Redis HA

```
方案 1：Redis Sentinel（主从 + 自动故障转移）
  → go-redis: redis.NewFailoverClient(...)

方案 2：Redis Cluster（分片 + HA，超大 QPS）
  → go-redis: redis.NewClusterClient(...)
```

### 5.2 三级降级策略

```
Redis 正常     →  分布式令牌桶，精确限流
Redis 慢/超时  →  circuit breaker 打开，切本地限流
本地限流       →  单 Pod 维度限流，粒度变粗但不崩溃
极端兜底       →  fail-open（放行）/ fail-closed（拒绝），看业务
```

### 5.3 熔断器状态机

```
Closed(正常) ──错误率超阈值──→ Open(熔断，走本地)
    ↑                               │
    └── 探测成功 ──← Half-Open ←── 超时后放少量请求试探
```

---

## 六、热点 Key 问题

### 问题

高 QPS 的全局 key（如 `/pay` 接口限流）集中打到一个 Redis 节点，成为瓶颈。

### 错误做法：hash tag 分片（在 Redis Cluster 下无意义）

```
rate:{api_pay}:0  ┐
rate:{api_pay}:1  ├── hash("api_pay") → 同一个 slot → 同一个 Redis 节点
rate:{api_pay}:9  ┘
写压力没有分散，分片没有意义。
```

hash tag 的唯一价值是让多个 key 在同一 slot，使 Lua 脚本可以跨 key 原子操作。在 Cluster 下用来分散写压力，这个目标本身达不到。

### 正确做法：本地计数 + 异步同步

**请求路径（每个请求）：**

```
请求进来
  ├── local.Add(1)              # 本地原子加，纯内存
  └── globalSum.Load() < limit  # 读本地缓存，无网络
        ├── true  → 放行
        └── false → 拒绝
```

**后台 goroutine（每 100ms）：**

```
├── 计算增量 delta = local - lastPush
├── 写 Redis：SET rate:api_pay:{pod-id} {delta}  # 写自己的 key
├── pipeline 读所有 pod 的 key                    # 读各 pod 数据
└── 求和 → globalSum.Store(total)                 # 更新本地缓存
```

```go
type LocalSyncLimiter struct {
    local     atomic.Int64 // 本地计数（累计，不清零）
    lastPush  atomic.Int64 // 上次推送值
    globalSum atomic.Int64 // 全局总量缓存
    podKey    string       // "rate:api_pay:pod-abc123"
    limit     int64
}

func (l *LocalSyncLimiter) Allow() bool {
    l.local.Add(1)
    return l.globalSum.Load() < l.limit
}

func (l *LocalSyncLimiter) syncLoop(ctx context.Context) {
    for range time.Tick(100 * time.Millisecond) {
        current := l.local.Load()
        delta := current - l.lastPush.Load()
        if delta > 0 {
            rdb.Set(ctx, l.podKey, current, 10*time.Second)
            l.lastPush.Store(current)
        }

        // pipeline 读所有 pod 的 key 求和
        total := l.sumAllPods(ctx)
        l.globalSum.Store(total)
    }
}
```

各 Pod 写自己独立的 key，key 自然散落到不同 Redis 节点，真正分散写压力。

**代价：** 近似限流，最多短暂超限约 `pods数 × 100ms内的QPS`，精确场景不适用。

---

## 七、K8s 部署

### 7.1 独立微服务（推荐）

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rate-limiter
spec:
  replicas: 3
  strategy:
    type: RollingUpdate   # 滚动更新，零停机
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
spec:
  scaleTargetRef:
    name: rate-limiter
  minReplicas: 3
  maxReplicas: 20
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          averageUtilization: 60
```

### 7.2 限流规则热更新（ConfigMap Watch）

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rate-limit-rules
data:
  rules.yaml: |
    - key: "user"
      capacity: 100
      rate: 10
    - key: "api:/pay"
      capacity: 50
      rate: 5
```

```go
// 监听 ConfigMap 变化，不重启 Pod 即可更新规则
informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
    UpdateFunc: func(old, new interface{}) {
        cm := new.(*v1.ConfigMap)
        limiter.UpdateConfig(parseConfig(cm.Data))
    },
})
```

### 7.3 Sidecar 注入（Istio 场景）

```
MutatingAdmissionWebhook → 自动向每个 Pod 注入 rate-limiter sidecar
Sidecar 拦截 inbound 流量 → 检查限流 → 转发给主容器
适合：业务代码零侵入，但灵活性低
```

---

## 八、方案对比与 Tradeoff

| 维度 | 纯 Redis | 本地 + 异步同步 |
|------|---------|--------------|
| 精确度 | 精确 | 近似（误差 ≤ pods × sync间隔内QPS） |
| 请求延迟 | +1-3ms | 接近 0 |
| Redis 压力 | 每请求 1 次 | 每 100ms 1 次 |
| Redis 挂了 | 需要降级方案 | 本地计数继续工作 |
| 实现复杂度 | 低 | 中 |
| 适用场景 | 支付/严格限流 | 高 QPS / 近似限流 |

---

## 九、面试追问预判

**Q：Redis Cluster 下多个 key 怎么用 Lua 原子操作？**
→ hash tag：`{user_123}` 强制多个 key 落同一 slot，Lua 才不报 CROSSSLOT 错误。

**Q：滑动窗口 ZSET 和双窗口计数哪个好？**
→ 看 QPS。低 QPS 要精确用 ZSET；高 QPS 内存受限用双窗口近似计数。

**Q：本地同步间隔怎么定？**
→ 100ms-500ms。越短越准但 Redis 压力越大，根据精确度要求和 QPS 调整。

**Q：pod 重启后本地计数丢了怎么办？**
→ 重启后 globalSum 从 Redis 读回初始值，本地从 0 开始重新累计，短暂影响可接受。

**Q：Redis 挂了用 fail-open 还是 fail-closed？**
→ 内部服务（推荐、搜索）用 fail-open，保可用性；支付、安全类用 fail-closed 或本地严格限流。
