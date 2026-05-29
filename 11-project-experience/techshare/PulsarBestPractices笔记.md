# Pulsar Best Practices 技术分享笔记

> 美餐内部技术分享，系统讲解 Apache Pulsar 的核心机制与生产最佳实践（共 9 条）。

---

## 一、Pulsar 架构回顾

```
Producer → Broker → BookKeeper (持久化存储)
                  ↓
              Consumer
              (via Subscription)
```

- **Broker**：无状态，处理消息路由、消费管理
- **BookKeeper**：有状态，存储消息（Ledger 结构）
- **ZooKeeper**：元数据存储（topic 配置、cursor 位置等）

### Topic 命名规范

```
persistent://tenant/namespace/topic-name

例：persistent://public/titan/event
```

### Partitioned Topic

```
persistent://public/titan/event
    ├── persistent://public/titan/event-partition-0
    ├── persistent://public/titan/event-partition-1
    └── persistent://public/titan/event-partition-2
```

- 每个 partition 独立分配到一个 Broker
- 提高并发吞吐量

---

## 二、四种订阅模式

| 订阅类型 | 消费者数量 | 消息分发规则 | 顺序保证 |
|----------|-----------|-------------|---------|
| **Exclusive** | 只能 1 个 | 全量推给唯一消费者 | 有序 |
| **Failover** | 多个，但只有 1 个活跃 | 主消费者收全部；主挂则 failover 到备 | 有序 |
| **Shared** | 多个，全部活跃 | 轮询分发 | 无序 |
| **Key_Shared** | 多个，全部活跃 | 相同 key 路由到相同消费者 | 同 key 有序 |

> Key_Shared 是最常用的高并发有序消费方案。

---

## 三、Producer 基础

### 发送模式

```go
// 同步发送（等待 broker ack）
msgID, err := producer.Send(ctx, &pulsar.ProducerMessage{Payload: []byte("hello")})

// 异步发送（不阻塞，通过回调得知结果）
producer.SendAsync(ctx, msg, func(msgID pulsar.MessageID, msg *pulsar.ProducerMessage, err error) {
    // callback
})
```

### Best Practices #1：使用 Batching

- 默认开启批量发送（`BatchingEnabled: true`）
- 批量合并多条消息为一个网络包，显著提升吞吐量
- 批量相关参数：
  - `BatchingMaxMessages`：批量最大条数（默认 1000）
  - `BatchingMaxSize`：批量最大字节数（默认 128KB）
  - `BatchingMaxPublishDelay`：最长等待时间（默认 1ms）

### Best Practices #2：Async Send + 控制 Pending Queue

- 异步发送时消息先进入客户端内部 pending queue，满了会阻塞
- 参数：`MaxPendingMessages`（默认 1000）
- 生产不要无限制异步发，要有背压控制

### Best Practices #3：使用 Producer.DeliverAt（延迟消息）

- `DeliverAt`：指定投递时间戳（绝对时间）
- `DeliverAfter`：投递延迟（相对时间）

```go
producer.Send(ctx, &pulsar.ProducerMessage{
    Payload:   []byte("delayed msg"),
    DeliverAt: time.Now().Add(5 * time.Minute),
})
```

> **注意**：使用 DeliverAt 时，Consumer 的 `ReceiverQueueSize` 必须设为 **0**（见 BP#5）

### Best Practices #4：Producer Name / 幂等性

- 默认 producer name 是随机的
- 设置固定 producer name + `EnableIdempotent: true` 可开启 producer 端去重
- Broker 记录每个 (producer name, sequence ID) 防止重复写入

---

## 四、Consumer 工作原理

### Consumer 内部结构

```
Topic → [m1, m2, m3, ...] → Internal Queue → Receive() → Messages
                              ↑
                        Consumer 内部缓冲队列
```

消费者从 Broker 接收到消息先放入 **Internal Queue**，业务代码调用 `Receive()` 从队列取出处理。

### Consumer Flow Control —— Permits 机制

```
Consumer ──send permits 1000──→ Broker
         ←──push 1000 messages──
Consumer ──receive messages──→ Server（业务处理）
         ←──done 500 messages──
Consumer ──ack 500──────────→ Broker
Consumer ──send permits 500─→ Broker
         ←──push 500 messages──
```

**核心概念**：
- Consumer 通过发送 **"Flow" 命令**（携带 permits 数量）告诉 Broker：我还能接收多少条消息
- Broker 最多推送 permits 数量的消息
- 每次 ack 后，Consumer 补充 permits，Broker 继续推
- **Permits = 0 → Broker 停止推送 → Consumer 停止消费**

---

## 五、Best Practices #5：调整 ReceiverQueueSize

- Internal Queue 容量 = `ReceiverQueueSize`，默认 **1000**
- 增大可以提升吞吐量（Broker 可以提前预推更多消息，减少等待）
- 但对于 **DeliverAt 延迟消息**，必须设置为 **0**

```go
client.Subscribe(pulsar.ConsumerOptions{
    Topic:             "persistent://public/titan/event",
    SubscriptionName:  "my-sub",
    ReceiverQueueSize: 0,   // 延迟消息场景
})
```

**为什么延迟消息要设 0？**
因为 Internal Queue 里预拉取的消息会在时间未到时就出队，导致延迟失效。设为 0 禁止预拉取，消息到达投递时间才被推送。

---

## 六、Best Practices #6：Consumer 停止消费怎么办

### 现象

```json
"availablePermits": 0,
"unackedMessages": 1000
```

Permits 为 0，说明 Consumer 没有在 ack，导致 Broker 不再推送。

### 根本原因

- 漏 ack 或者 nack 太多
- unackedMessages 达到上限（Backlog Limitation）
- 消息无法被重新投递给其他消费者

### 解决方案

1. **Enable RLQ/DLQ**（Retry Letter Queue / Dead Letter Queue）：消息重试超次数后进 DLQ，不再阻塞消费
2. **Set VisibilityTimeout / NackRedeliveryDelay**：nack 后延迟一段时间再重试，防止立即重投导致堆积

---

## 七、Messages Backlog 与 Unacked 限制

### Backlog 是什么

```
[  Backlog  ] [Buffered in ReceiverQueue] [Unacked messages] [Acked messages]
```

- **Backlog**：还没推送给 Consumer 的消息
- **Unacked**：已推送但还没 ack 的消息

### 上限参数（Broker 配置）

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `MaxUnackedMessagesPerConsumer` | 50,000 | 单个 Consumer 未 ack 上限 |
| `MaxUnackedMessagesPerSubscription` | 200,000 | 整个 Subscription 未 ack 上限 |

> 达到上限后 Broker 会暂停推送，直到 ack 数量降下来。

### Unack Messages Limitation 的代价

- 某个 Consumer 宕机 → 大量消息需要 redeliver
- Broker 需要维护 **非连续 ack 的 gap（holes）**，内存和存储开销大

---

## 八、Acknowledgement State Management

### Mark Delete Position

Broker 用 **mark delete position** 跟踪消费进度。

```
ledger 1: [0][1][2][3][4]   ← 全部 acked（green）
ledger 2: [2]↑ mark delete pos
          [0][1] acked, [2][3][4][5][6] unacked

cursor ledger（持久化）存储离散 ack：
  - (2,4)  → 单独 ack 了 position 4
  - (2,7)
  - (2,8)
```

### Acknowledgement State Management 限制

- `managedLedgerMaxUnackedRangesToPersist`：默认 **10,000**
- 超过限制后，ack 状态只在内存里跟踪（不持久化）
- **Broker 重启后**：从最后一个持久化的 cursor ledger 位置重新投递，已在内存里 ack 的消息会被重投

> 这就是 Pulsar at-least-once 的来源之一。

---

## 九、Best Practices #7：避免 "holes"（ack 空洞）

- 配置 **RLQ/DLQ**：消息重试超次数进 DLQ，不会永远 pending
- 尽量 **ack** 而不是 nack：某些失败不需要重试（如数据格式错误），直接 ack + 记录日志更合理

```
错误的做法：遇到任何错误都 nack → 无限重试 → 堆积 unacked holes
正确的做法：
  - 可重试错误（下游超时）→ nack（with delay）
  - 不可重试错误（数据格式错）→ ack（并告警/记录）
```

---

## 十、Best Practices #8：幂等消费

- Pulsar 是 **at-least-once** 投递语义
- 以下情况会产生重复投递：
  - Consumer 处理完但 ack 前崩溃
  - Broker 重启后重发（ack 状态只在内存）
  - nack 后重投

**应对方案**：
- **幂等设计**：相同消息处理多次结果一致（如 upsert）
- **去重表**：记录已处理的 messageID，重复到来时直接跳过

---

## 十一、Message Lifecycle —— 三个关键概念

### 状态流转

```
Backlog ──消费──→ Acked ──→ Retention（按保留策略清理）
       ──TTL到期──→ TTL Reached ──→ Retention
```

### 数据分区示意

```
[  Retention  ] [  Backlog  ] [ Active Ledger ]
                     ↑
               Sub A mark delete position
```

### 1. Data Retention（数据保留）

- 所有 Subscription 都 ack 后的消息进入 **Retention** 区域
- 策略匹配后：
  - 永久删除
  - Offload 到 tiered storage（S3）

### 2. Message TTL（消息存活时间）

- TTL 仅移动 **mark delete position**（不真正 ack）
- 达到 TTL 的消息被推入 Retention 区域，消费者看不到
- 如需重消费，必须 **reset cursor** 到 TTL 前的位置

> TTL 与 ack 的区别：ack 是消费端操作；TTL 是 Broker 定时推进 mark delete position。

### 3. Backlog Quota（积压配额）

积压超出配额时的策略（三选一）：

| 策略 | 行为 |
|------|------|
| `producer_request_hold` | 阻塞 Producer 发送请求 |
| `producer_exception` | Producer 发送报错 |
| `consumer_backlog_eviction` | **丢弃最旧的 backlog 消息**（默认） |

配额限制：
- 按大小：**5 GB**
- 按时间：**7 Days**

---

## 十二、Best Practices #9："三剑客" 配置

> **Les Trois Mousquetaires**：必须三个一起配，缺一不可。

| 配置 | 用途 |
|------|------|
| **Retention Policy** | 控制 acked 消息保留多久/多大 |
| **Message TTL** | 控制未消费消息的最大存活时间 |
| **Backlog Quota** | 控制 backlog 上限，防止存储爆炸 |

**关联关系**：

```
TTL 推进 mark delete position
    ↓
消息进入 Retention 区域
    ↓
Retention Policy 决定何时清理 / offload

Backlog Quota 在 backlog 超限时强制处理（丢弃 or 阻塞生产）
```

**告警配置**：

```yaml
# PrometheusRule 示例
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
spec:
  groups:
    - name: AppDLQMetricsBacklogCount
      rules:
        - alert: pulsar-pulsar_msg_backlog-count-10
          expr: |
            pulsar_msg_backlog{topic=~"persistent://public/titan/event.*"} > 10
          for: 3m
          labels:
            severity: warning
            app: pagerduty
```

> `pulsar_msg_backlog` 是 Pulsar 暴露的 Prometheus 指标，表示未消费消息数量。

**运维建议**：
- 删除不再使用的 Subscription（废弃 sub 会阻止 backlog 清理，导致存储持续增长）

---

## 十三、内部工具

### DevOps 平台

- 查看 Topic 详情：Partitions、Subscriptions、Producers、Backlog Size
- 支持 Backlog Operation（跳过 backlog、重置 cursor）

### Grafana Dashboard

指标：
- Local publish rate / throughput
- Local delivery rate / throughput
- Topics / Producers / Subscriptions / Consumers count
- **Local backlog**（重点监控）
- Storage Write Latency / Storage Size

### Prometheus / Thanos

```promql
# 查看 Pulsar 各 Pod 入站速率
sum(pulsar_rate_in{cluster="fan2-pulsar0"}) by (pod)
```

---

## 十四、Best Practices 汇总

| # | 主题 | 核心要点 |
|---|------|---------|
| 1 | Batching | 开启批量发送，调 BatchingMaxMessages/Size/Delay |
| 2 | Async + Backpressure | 控制 MaxPendingMessages，不要无限制异步 |
| 3 | DeliverAt 延迟消息 | 指定绝对投递时间 |
| 4 | Producer 幂等 | 固定 producer name + EnableIdempotent |
| 5 | ReceiverQueueSize | 增大提吞吐；DeliverAt 场景必须设为 0 |
| 6 | 消费停滞排查 | 检查 availablePermits=0 + unackedMessages；启用 RLQ/DLQ |
| 7 | 避免 ack holes | 配置 RLQ/DLQ；能 ack 就不要 nack |
| 8 | 幂等消费 | at-least-once → 业务层实现幂等或去重 |
| 9 | 三剑客 | Retention + TTL + Backlog Quota 三个必须同时配 |

---

## 十五、面试可说的点

1. **Consumer Permits 机制**：Consumer 主动发 Flow 命令告知 Broker 可接收数量，Permits 耗尽后 Broker 停推；消费停滞第一步查 `availablePermits`
2. **Ack 空洞问题**：Broker 用 mark delete position + cursor ledger 跟踪离散 ack；`managedLedgerMaxUnackedRangesToPersist` 超限后内存跟踪，重启丢失 → 重投
3. **at-least-once 来源**：Broker 重启、nack 重投、Consumer 崩溃后重投，三种场景都可能重复，业务必须幂等
4. **DeliverAt 坑**：ReceiverQueueSize 必须设 0，否则预拉取队列破坏延迟投递语义
5. **三剑客**：Retention + TTL + Backlog Quota 缺一不可；废弃 Subscription 不删会撑大 backlog
6. **Backlog Quota 默认策略**：consumer_backlog_eviction，直接丢弃最旧消息，生产环境要按实际需求选策略
7. **监控指标**：`pulsar_msg_backlog` 监控 backlog 数量；Grafana 看 publish/delivery rate；DevOps 平台查 Subscription 级别 backlog
