# Pulsar 面试题 - 补充问题（基于实际项目）

> 基于美餐支付服务实际代码分析，记录真实踩坑和深度理解

---

## Q1：Shared 订阅模式下，同一条消息会被两个消费者同时收到吗？

**答：正常情况不会，但有几种场景会触发重复投递。**

### 正常情况

Broker 内部维护分发表，M1 一旦发给 Pod-1，就标记为"已分发，等待 Ack"，Pod-2 不会同时收到。

### 会触发重复投递的场景

| 场景 | 原因 | 结果 |
|------|------|------|
| Pod-1 处理超时未 Ack（超过 ackTimeout） | Broker 认为失败，重新投递 | M1 被 Pod-2 再次处理 |
| Pod-1 处理完但 crash，Ack 没发出去 | Broker 检测到断连，重新投递 | M1 被 Pod-2 再次处理 |
| Pod-1 主动 Nack | 业务处理失败，触发重试 | M1 被重新投递 |

### 结论

Pulsar Shared 订阅语义是 **At-Least-Once（至少一次）**，消费端必须做幂等。

---

## Q2：Pulsar 订阅模式是消费端设置还是生产端设置？底层怎么实现的？

**答：消费端设置，生产者完全不感知。**

```go
// 消费端代码（美餐支付实际代码）
consumer, err := pkgpulsar.NewConsumer(cfg.AppName,
    nerdspulsar.NewConsumerOption().
        WithTopics(consumerTopics...).
        WithSubscriptionType(nerdsmq.Shared),  // ← 消费端配置
)

// 生产端：只管发消息，不关心怎么消费
producer.Send(ctx, &pulsar.ProducerMessage{Payload: data})
```

### 底层实现

**Subscription 是 Broker 上的独立对象：**

```
Topic（消息流）
│
├── Subscription: "payment-service"  ← 类型: Shared
│     ├── Consumer: Pod-1
│     ├── Consumer: Pod-2
│     └── Consumer: Pod-3
│
└── Subscription: "audit-service"   ← 类型: Exclusive（另一个服务）
      └── Consumer: Pod-1
```

**消费者第一次连接时：**

```
Pod 发送 Subscribe 请求：
  topic: xxx, subscription_name: "payment-service", type: Shared
        ↓
Broker 检查 subscription 是否存在：
  - 不存在 → 创建，类型记为 Shared
  - 存在，类型一致 → 加入消费者列表
  - 存在，类型不一致 → 报错拒绝（所有 Pod 必须用相同类型！）
```

**重点：** 美餐支付用 `cfg.AppName` 作为 subscription name，所有 Pod 共享同一个 Subscription，Broker 对它们做 Round-Robin 分发，这正是 Shared 模式的工作方式。

---

## Q3：美餐支付幂等机制的设计缺陷与修复方案

### 实际实现（`pkg/mq/uniqueness.go`）

```
消息到达
  ↓ ConcurrencyLocker（Redis 锁，60s）防并发
  ↓ UniquenessChecker.Check()
      ├── mutex.Lock() → Redis key 立即写入（TTL 24h）← 问题根源
      ├── f() 执行业务逻辑
      ├── 失败 → Unlock（释放 key，允许重试）
      └── 成功 → 不 Unlock（key 保留 24h，作为"已处理"标记）
```

### 问题 1（最严重）：Redis key 在业务执行前就已写入

**crash 窗口：**

```
1. mutex.Lock() → Redis key 写入（24h TTL）
2. f() 执行中（DB 事务进行中）
3. 进程 OOM / kill -9 崩溃
   → DB 事务自动回滚 ✓
   → Redis key 仍然存在 ✗
4. 消息重新投递
5. mutex.Lock() 失败 → ErrMsgProcessed → 直接 Ack 丢弃
6. 业务从未成功执行，消息永久丢失（24h 内）
```

**根本原因：** Redis key 是"业务开始"的标记，而不是"业务完成"的标记。

### 问题 2（中等）：解锁失败导致消息卡死 24 小时

代码注释里已说明：
> "当 f 处理失败，解锁操作会重试 11 次，如果最终还是失败，会导致消息无法通过唯一性检查，无法再次消费"

场景：业务处理失败 → Redis 短暂抖动 → 11 次解锁全部失败 → key 保留 24h → 消息等同丢失。

### 问题 3（较小）：Context 超时后解锁 goroutine 失效

`handleMsg` 的 ctx 超时时间为 20s，超时后 `tryToEnsureUnlockSuccess` 启动的解锁 goroutine 拿着已 cancelled 的 ctx 操作 Redis，加剧问题 2。

---

### 修复方案：改为"干完活再占坑"

**核心思路：** 把 `mutex.Lock()`（先占坑）改成业务成功后的 `redis.Set()`（事后标记）。

```go
func (h *uniquenessChecker) Check(ctx context.Context, msg *ReceiveMessageOutput, ignoreErr bool, f func() error) error {
    key := msg.GetUniqueKey()
    if key == "" {
        return f()
    }

    redisKey := fmt.Sprintf("unique-%s-%s", msg.GetTopicNameWithoutPartition(), key)

    // Step 1: 查是否已处理（GET，不加锁）
    exists, err := h.redisClient.Exists(ctx, redisKey).Result()
    if err != nil {
        if !ignoreErr {
            return fmt.Errorf("redis check unique key err: %w", err)
        }
        h.logger.For(ctx).Warn("check key failed, ignoring", log.ErrField(err))
    } else if exists > 0 {
        return ErrMsgProcessed
    }

    // Step 2: 执行业务逻辑
    if err := f(); err != nil {
        return err  // 失败不写 key，允许重试
    }

    // Step 3: 业务成功后才写入标记
    if err := h.redisClient.Set(ctx, redisKey, 1, expiration).Err(); err != nil {
        // 写入失败只打 warn，最坏重复消费一次，业务层幂等兜底
        h.logger.For(ctx).Warn("mark processed failed", log.ErrField(err))
    }

    return nil
}
```

**修复效果：**

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 进程 crash（业务未完成） | key 已存在，消息永久丢失 | key 未写入，正常重试 ✓ |
| Redis 抖动（解锁失败） | 消息卡死 24h | 无需解锁，问题消失 ✓ |
| Context 超时 | 解锁 goroutine 用废弃 ctx | 无解锁 goroutine ✓ |
| 正常重复投递 | 跳过 ✓ | 跳过 ✓ |

### 残余隐患与兜底

Step 1（Exists）和 Step 3（Set）之间仍有极短的竞态窗口。但这个场景（两个不同消息带相同 uniqueKey 同时到达）在 Pulsar 正常运行时不会发生。即使发生，业务层已有兜底：

```go
// TpwPaymentCallback 中的状态机检查
if ss.IsCompleted() {
    return true, nil  // 子单已终态，直接返回
}
unlockFn, err := t.Locker.LockPaymentSlip(ctx, ss.SlipId)  // 支付单维度业务锁
```

---

## 面试要点总结

1. Pulsar 订阅模式是**消费端**配置的，生产者无感知
2. Shared 模式语义是 **At-Least-Once**，消费端必须幂等
3. 用 Redis 做幂等时，**key 必须在业务成功后才写入**，不能提前占坑
4. 两把锁的分工：
   - **并发锁（60s）**：防止同一条消息被两个 goroutine 同时处理
   - **幂等标记（24h）**：防止同一条消息在生命周期内被处理多次
