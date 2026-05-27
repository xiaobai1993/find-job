# Pulsar 最佳实践 & 参数调优

> 来源：美餐内部 Pulsar 使用规范 Wiki，实际踩坑沉淀
> 覆盖：吞吐量调优、nack 陷阱、Backlog/Retention、连接数配置

---

## 一、吞吐量调优

### 1. 怎么提升 Pulsar 的吞吐量？

**答：** 有几个维度，从分区、订阅模式、Batching、参数四个层面来调。

---

#### 1.1 增加分区数

单个 partition 理论上能支撑上百万 QPS，但从 Broker 视角看，**每个 partition 只分配给一个 Broker 负责**。极端情况下：

- 单 partition + 流量过大 → 该 Broker CPU 打满，收发速度下降
- 单 partition + Broker 重启 → partition 转移到其他 Broker，客户端感知到 reconnect 和 timeout，可用性下降

**实践：** 核心 Topic 根据流量预估设置合理分区数，不要用默认的单分区。

```bash
# 创建带分区的 Topic
pulsar-admin topics create-partitioned-topic \
  persistent://public/default/payment-notify \
  --partitions 8

# 更新已有 Topic 分区数（只能增加，不能减少）
pulsar-admin topics update-partitions \
  persistent://public/default/payment-notify \
  --partitions 16
```

---

#### 1.2 使用 Shared 或 KeyShared 订阅模式

Shared / KeyShared 模式允许多个 Consumer 同时消费多个 partition，显著提升消费速度。

> ⚠️ **注意：KeyShared 模式下，Delay Message 和 Batching 均无法使用。**

| 订阅模式 | 并发消费 | 支持 Batching | 支持 Delay Message |
|---------|---------|--------------|-------------------|
| Exclusive | ❌ 单消费者 | ✅ | ✅ |
| Shared | ✅ | ✅ | ✅ |
| KeyShared | ✅ | ❌ | ❌ |
| Failover | ❌ 主备切换 | ✅ | ✅ |

---

#### 1.3 开启 Batching（生产端）

Producer 开启 Batching 后，Broker 将一批消息作为整体写入存储，而不是每条消息单独写，显著降低存储 IO。

```go
producer, _ := client.CreateProducer(pulsar.ProducerOptions{
    Topic:               "persistent://public/default/payment-notify",
    BatchingMaxMessages: 1000,               // 最多攒 1000 条发一次
    BatchingMaxPublishDelay: 10 * time.Millisecond, // 最多等 10ms 发一次
    BatchingEnabled:     true,
})
```

**Batching 的三个重要限制：**

1. **无法用于 KeyShared 订阅**
2. **无法用于 Delayed Message（DeliverAfter）**
3. **对 Batch 内某条消息 nack，在 Broker 重启情况下可能导致整个 Batch 重发**

---

#### 1.4 调整 MaxPendingMessages（生产端缓冲）

`MaxPendingMessages` 允许 Producer 在本地缓存一定数量的消息并统一发送，减少与 Broker 的交互次数。

```go
producer, _ := client.CreateProducer(pulsar.ProducerOptions{
    Topic:              "persistent://public/default/payment-notify",
    MaxPendingMessages: 10000, // 默认 1000，根据内存情况适当调大
})
```

> ⚠️ 调大此值会增加 Broker 内存消耗，需要结合可用内存评估。

---

#### 1.5 调整 ReceiverQueueSize（消费端预取）

`ReceiverQueueSize` 控制 Consumer 一次向 Broker 预取多少条消息放在本地缓存队列，调大可以提升消费吞吐。

```go
consumer, _ := client.Subscribe(pulsar.ConsumerOptions{
    Topic:             "persistent://public/default/payment-notify",
    SubscriptionName:  "payment-sub",
    ReceiverQueueSize: 2000, // 默认 1000
})
```

**关键特殊场景：**

> ⚠️ **如果 Topic 使用了 `DeliverAt` / `DeliverAfter` 延迟消息，必须将 `ReceiverQueueSize` 设置为 `0`。**

原因：Consumer 提前把消息拉进缓存队列后，消息在队列里等待而不是在 Broker 等待，导致无法保证准时投递。设置为 0 后，Consumer 不预取，消息完全由 Broker 控制投递时机。

```go
// 延迟消息 Consumer 的正确配置
consumer, _ := client.Subscribe(pulsar.ConsumerOptions{
    Topic:             "persistent://public/default/delay-task",
    SubscriptionName:  "delay-sub",
    ReceiverQueueSize: 0, // 必须设为 0，保证延迟消息准时投递
})
```

---

## 二、nack 的陷阱

### 2. nack 有什么副作用？为什么要减少使用？

**答：** nack 的副作用比很多人想象的严重，要慎用。

**副作用一：降低 availablePermits，最终停止消费**

Consumer 有一个内部指标 `availablePermits`，代表当前 Consumer 可以向 Broker 拉取的消息数量。

- 频繁 nack → Pulsar 认为该 Consumer 不健康 → 降低 `availablePermits` → Consumer 拉取到的消息越来越少
- `availablePermits` 降到 0 时 → **该 Consumer 完全停止工作**

**副作用二：存储"窟窿"导致消息丢失**

nack 的消息在存储中形成"窟窿"（游标无法向前推进的空洞），这些窟窿：
- 会导致游标（cursor）无法向前移动，占用更多存储空间
- 当窟窿数量超过系统阈值（通常 **200,000**）后，Broker 会将这部分内容**放在内存**而不是持久化
- **Broker 重启时，内存中的这部分数据丢失，导致消息重复投递甚至消息丢失**

**正确处理姿势：**

```go
// ❌ 不推荐：对每条处理失败的消息都 nack
consumer.Nack(msg)

// ✅ 推荐：使用 DLQ / RLQ 处理失败消息，而不是 nack
consumer, _ := client.Subscribe(pulsar.ConsumerOptions{
    Topic:            "persistent://public/default/payment-notify",
    SubscriptionName: "payment-sub",
    DLQ: &pulsar.DLQPolicy{
        MaxDeliveries:   3,                          // 重试 3 次后进死信
        DeadLetterTopic: "payment-notify-dlq",       // 死信 Topic
        RetryLetterTopic: "payment-notify-retry",    // 重试 Topic
    },
})

// 只在非常确定需要重试时才 nack
// 正常的业务失败应通过 DLQ 处理，不要 nack
```

> **结论：** 只在非常确定该消息必须重试时才 nack。大多数场景应使用 DLQ 处理失败消息，让 Pulsar 自动管理重试和死信，不要手动 nack。

---

## 三、消息可靠性

### 3. Pulsar 的投递语义是什么？怎么处理重复消息？

**答：** Pulsar 的投递语义是 **at-least-once（至少一次）**，保证消息不丢，但不保证不重复。

以下场景都可能触发重复投递：
- Consumer 处理完消息但 ack 前挂了，重启后消息重新投递
- Broker 重启，内存中未持久化的 nack 窟窿丢失
- Batch 内某条消息 nack，整个 Batch 重发

**应对方案：** 业务侧必须做幂等处理。

```go
// 消费端幂等处理示例（支付场景）
func handlePaymentMsg(msg pulsar.Message) error {
    var event PaymentEvent
    json.Unmarshal(msg.Payload(), &event)

    // 幂等检查：用业务唯一键去重
    exists, _ := redis.SetNX(ctx, "processed:"+event.OrderID, 1, 24*time.Hour)
    if !exists {
        // 已处理过，直接 ack 跳过
        consumer.Ack(msg)
        return nil
    }

    // 执行业务逻辑
    if err := processPayment(event); err != nil {
        redis.Del(ctx, "processed:"+event.OrderID) // 处理失败，删除去重 key
        return err // 由 DLQ 处理重试
    }

    consumer.Ack(msg)
    return nil
}
```

> **注意：** Pulsar 的 Deduplication 功能是针对 **Producer 发送侧**的去重（防止同一条消息被发送两次），不是消费侧的去重。消费侧去重必须业务自己实现。

---

## 四、Backlog & Retention

### 4. Backlog 和 Retention 是什么？超限了会怎样？

**答：** 这两个概念很重要，搞错了会丢消息。

**Backlog（积压）：**
- 消息发出后**未被消费**时，存储在 Backlog 中
- Backlog 过多时，存储占用增大
- Backlog 超过一定大小（**通常 100M**），消息会转存到 **S3**，再次消费时从 S3 拉取，**消费速度明显变慢**
- Backlog 超过系统阈值（**通常 1G～5G**），触发 **consumer_backlog_eviction**，**超出部分的消息被直接丢弃**

> ⚠️ **Consumer 停止工作超过一段时间，如果积压超过 1G，会丢消息！**

**Retention（保留）：**
- 消息**消费完成后**进入 Retention 存储
- 用于支持新出现的 Consumer 用 `SubscriptionPosition=Earliest` 回溯历史消息
- Retention 和 Backlog 独立计算

**MessageTTL：**
- 配合 Backlog 使用，规定消息的最大生命周期（通常 1 天）
- 超过 TTL 的消息即使在 Backlog 中也会被清除

**生产规范：**
```bash
# 查看 namespace 的 backlog/retention 配置
pulsar-admin namespaces get-backlog-quotas public/payment

# 设置 backlog 配额（超出后触发驱逐）
pulsar-admin namespaces set-backlog-quota public/payment \
  --limit 5G \
  --policy consumer_backlog_eviction

# 设置 MessageTTL
pulsar-admin namespaces set-message-ttl public/payment --messageTTL 86400  # 1天
```

---

## 五、连接数配置

### 5. MaxConnectionPerBroker 为什么要调？默认值有什么问题？

**答：** 这个参数默认值是 **1**，生产环境必须调大，否则高并发下会超时。

**问题背景：**

客户端通常同时依赖多个 Topic，这些 Topic 的 partitions 可能分布在多个 Broker 上。默认 `MaxConnectionPerBroker=1` 时，客户端对每个 Broker 只建一个连接，会导致：
- **负载不均衡**：所有请求挤在一个连接上
- **可用性降低**：单连接故障影响所有 Topic

**更严重的问题（Pulsar 3.1.x）：**

Broker 端对单连接的 `pendingSendRequest` 有限制（默认 **1000**）。如果客户端瞬间向单连接发送大量消息，同时 Broker 存储出现网络延迟、ledger rollover 等情况，`pendingSendRequest` 打满后，**Broker 直接停止响应该连接**，体现为客户端发送/接收超时。

> 虽然 3.2.x 对此行为做了修改，但仍存在一定概率，不可完全依赖。

**建议配置：**

```go
client, _ := pulsar.NewClient(pulsar.ClientOptions{
    URL: "pulsar://broker:6650",

    // 设置为你依赖的 Topic 数量
    // 虽然会消耗更多资源，但能保证可用性和吞吐量
    MaxConnectionsPerBroker: 10, // 依赖 10 个 Topic 左右时的推荐值
})
```

**选取建议：**
- 依赖 Topic 数量少（< 5）：设置为 2～3
- 依赖 Topic 数量中等（5～20）：设置为 Topic 数量
- 依赖 Topic 数量多（> 20）：设置为 20，再多连接数收益递减，反而消耗资源

---

## 六、总结：参数调优速查表

| 参数 | 位置 | 默认值 | 推荐值 | 说明 |
|------|------|--------|--------|------|
| `BatchingEnabled` | Producer | true | true | 开启，提升吞吐 |
| `BatchingMaxMessages` | Producer | 1000 | 500～2000 | 根据消息大小调整 |
| `BatchingMaxPublishDelay` | Producer | 1ms | 5～10ms | 延迟换吞吐 |
| `MaxPendingMessages` | Producer | 1000 | 5000～10000 | 注意内存消耗 |
| `ReceiverQueueSize` | Consumer | 1000 | 1000～5000 | 延迟消息场景必须设为 **0** |
| `MaxConnectionsPerBroker` | Client | 1 | 依赖 Topic 数 | 必须调大，默认值不够用 |
| Topic 分区数 | Topic | 1 | 按流量估算 | 单分区会成为单 Broker 瓶颈 |

---

## 七、面试常见追问

**Q：Pulsar 和 Kafka 的 nack 行为有什么不同？**

Kafka 没有 nack 概念，消费失败直接让 offset 不提交，等下次 poll 重新消费。Pulsar 有显式的 nack，但 nack 会影响 `availablePermits`，滥用会导致 Consumer 停止工作，这是 Pulsar 特有的陷阱，Kafka 没有这个问题。

**Q：Consumer 停了一段时间恢复，消息还在吗？**

不一定。要看 Backlog 是否超过了驱逐阈值（通常 1G～5G）。如果积压的消息超过阈值，触发了 `consumer_backlog_eviction`，超出部分的历史消息会被丢弃。生产环境应配置 Backlog 告警，Consumer 停止工作要及时发现。

**Q：延迟消息消费不准时怎么排查？**

先检查 `ReceiverQueueSize` 是否为 0。如果不是 0，Consumer 会提前把消息拉进本地缓存队列，消息在队列里等待，不是在 Broker 按时投递，导致实际消费时间比预期早或晚。
