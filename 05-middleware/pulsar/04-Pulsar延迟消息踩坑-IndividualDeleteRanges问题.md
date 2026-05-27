# Pulsar 延迟消息踩坑：individuallyDeletedMessages 问题

> 来源：美餐支付实际生产踩坑，Pulsar 4.0 升级后暴露的延迟消息设计缺陷
> 覆盖：问题现象、根因分析、5种解决方案、Broker 配置调优

---

## 一、问题背景

### 1. 问题是什么？什么场景下触发？

**答：** 这是一个在升级 Pulsar 4.0 后暴露的延迟消息设计缺陷。

**触发场景：** 使用可变延迟时间（如 10 秒到 1 小时不等）的延迟消息，当 `individuallyDeletedMessages` 积累到较大数量（100,000+ 范围）后出现：

| 现象 | 表现 |
|------|------|
| **消息分发停滞** | 消息分发变得极其缓慢，大部分消息无法被分发 |
| **消费者重连失败** | 消费者重连到 Broker 后（负载均衡、网络抖动），消费完全停止 |
| **资源消耗激增** | Broker CPU 从 30% 飙升到 95%，ZooKeeper CPU 从 10% 飙升到 85% |
| **生产环境不可用** | 消费者无法恢复，消息持续积压 |

---

## 二、根因分析

### 2. 什么是 Individual Delete Ranges？为什么延迟消息会产生它？

**答：**

**Individual Delete Ranges 的定义：**

在 Pulsar 中，消息被**乱序确认（out-of-order acknowledgment）**时，Broker 会在游标（cursor）的 `individuallyDeletedMessages` 中记录这些已确认消息的位置范围。

- **正常顺序确认**：消息 1、2、3 按顺序消费，游标直接向前推进，不产生 individual delete ranges
- **乱序确认**：消息 2、4 先被确认，但 1、3 还没确认，这时候 2 和 4 的位置就被记录为 individual delete ranges

**为什么延迟消息会产生大量乱序确认？**

使用可变延迟时间时，消息是按**延迟到期顺序**投递，而不是按**发送顺序**：

```
发送顺序：msg1(延迟1h) → msg2(延迟10s) → msg3(延迟30s) → msg4(延迟5min)
投递顺序：msg2(10s后) → msg3(30s后) → msg4(5min后) → msg1(1h后)
```

msg1 排在最前面（最早发送），但最后被消费，期间 msg2、msg3、msg4 都先确认了，
这就产生了大量的 individual delete ranges，且 msg1 的延迟越长，积累的范围越多。

---

### 3. Pulsar 3.2.4 升级到 4.0 后为什么问题突然暴露？

**答：** 这是一个"修了 bug 暴露了设计缺陷"的案例。

| 版本 | 行为 | 后果 |
|------|------|------|
| **Pulsar 3.2.4** | `individuallyDeletedMessages` 跟踪存在**竞态条件（race condition）**，并发确认时部分范围会"丢失"，不被记录 | 虽然是 bug，但意外地防止了范围的无限增长，问题被掩盖 |
| **Pulsar 4.0** | PR #22966 修复了竞态条件，在锁作用域内进行所有操作；PR #23006 用 RoaringBitmap 优化内存，所有范围都被正确跟踪 | 正确性修复了，但暴露了设计上的缺陷：**没有机制来压缩或清理这些范围** |

> 升级到 4.0 后，之前被 bug 掩盖的问题彻底暴露，且因为 RoaringBitmap 优化让范围跟踪更完整，积累速度更快。

---

### 4. 为什么消费者重连后会完全卡死？这是最核心的问题

**答：** 这是问题最严重的表现，根因在于重连时的序列化/反序列化开销。

**重连触发场景：**
1. Broker 负载均衡（load shedding）
2. Topic 所有权转移（ownership transfer）
3. 网络抖动导致连接中断
4. 消费者主动重连

**重连时发生了什么：**

```
消费者断开连接
    ↓
Broker 需要恢复游标状态
    ↓
从 ZooKeeper/BookKeeper 读取游标元数据
    ↓
反序列化 individuallyDeletedMessages（此时已有 100,000+ 范围，文件 5MB+）
    ↓
ZooKeeper CPU 飙升，反序列化耗时 30+ 秒
    ↓
消费者等待超时，再次重连
    ↓
触发更多反序列化，形成死循环
    ↓
消费完全卡死
```

**游标元数据大小对比：**
- 正常情况（< 1000 ranges）：游标元数据 ~10 KB
- 问题情况（> 100,000 ranges）：游标元数据 ~5 MB，且每次重连都要读写这 5MB 数据

---

## 三、复现测试

### 5. 怎么复现这个问题？

**测试环境：**
- Broker 版本：Pulsar 4.0.8
- 客户端：Pulsar Go Client 0.17.0
- OS：Linux aarch64，Java：OpenJDK 17

**生产者代码（模拟可变延迟）：**

```go
producer, _ := client.CreateProducer(pulsar.ProducerOptions{
    Topic: "persistent://public/default/delay-test",
})

// 模拟可变延迟：10秒到1小时随机
delays := []time.Duration{
    10 * time.Second,
    30 * time.Second,
    5 * time.Minute,
    30 * time.Minute,
    1 * time.Hour,
}

for i := 0; i < 100000; i++ {
    delay := delays[rand.Intn(len(delays))]
    producer.Send(ctx, &pulsar.ProducerMessage{
        Payload:      []byte(fmt.Sprintf("msg-%d", i)),
        DeliverAfter: delay,
    })
}
```

**消费者代码：**

```go
consumer, _ := client.Subscribe(pulsar.ConsumerOptions{
    Topic:            "persistent://public/default/delay-test",
    SubscriptionName: "test-sub",
    Type:             pulsar.Shared,
})

for {
    msg, _ := consumer.Receive(ctx)
    // 处理消息
    process(msg)
    consumer.Ack(msg) // 乱序 ack，因为延迟不同，消费顺序和发送顺序不一致
}
```

**观察到的结果：**
1. Individual Delete Ranges 随时间持续激增，无上限
2. 发送约 50,000 条可变延迟消息后，分发开始明显变慢
3. 达到 100,000+ ranges 后，模拟一次 Broker 重启，消费者重连后完全卡死
4. Broker CPU：30% → 95%，ZooKeeper CPU：10% → 85%，持续 30+ 秒

---

## 四、解决方案

### 6. 有哪些解决方案？怎么选？

**方案对比总览：**

| 方案 | 难度 | 效果 | 适用场景 |
|------|------|------|---------|
| 方案1：增加分区数 | 低 | 缓解 | 快速止血，优先实施 |
| 方案2：自定义分区路由 | 中 | 最优 | 长期解决方案 |
| 方案3：延迟切分 | 中 | 好 | 长延迟（> 10 分钟）场景 |
| 方案4：外部延迟调度器 | 高 | 彻底解决 | 复杂延迟管理需求 |
| 方案5：Broker 配置调优 | 低 | 临时缓解 | 配合其他方案 |

---

### 方案1：增加分区数量（推荐优先实施）

**原理：** 将乱序确认分散到更多分区，每个分区的 individual delete ranges 数量减少。

**分区数量估算公式：**
```
目标分区数 = ceil(预期并发延迟消息数 / 每分区可承受的最大 ranges 数)

例如：预期 100,000 条并发延迟消息，每分区承受上限 10,000 ranges
目标分区数 = ceil(100,000 / 10,000) = 10 个分区
```

**操作：**
```bash
# 更新已有 topic 的分区数
pulsar-admin topics update-partitions \
  persistent://public/default/delay-test \
  --partitions 10
```

**优缺点：**
- ✅ 实施简单，无需修改应用代码，立即生效
- ✅ 可随消息量增长继续扩展分区数
- ❌ 增加分区管理复杂度
- ❌ 消息量继续增长时，仍需继续扩展

---

### 方案2：自定义分区路由（最优长期方案）

**原理：** 根据延迟时间将消息路由到不同分区，使每个分区内的消息延迟时间相近，最大程度减少乱序确认。

```go
// 自定义路由：按延迟桶分区
type DelayBasedRouter struct {
    numPartitions int
}

func (r *DelayBasedRouter) ChoosePartition(msg *pulsar.ProducerMessage, metadata pulsar.TopicMetadata) int {
    delayAfter, ok := msg.Properties["delay_seconds"]
    if !ok {
        return 0
    }

    delaySec, _ := strconv.Atoi(delayAfter)

    // 按延迟时长分桶，相似延迟的消息进同一分区
    switch {
    case delaySec <= 60:          // 0~1分钟
        return 0
    case delaySec <= 300:         // 1~5分钟
        return 1
    case delaySec <= 1800:        // 5~30分钟
        return 2
    case delaySec <= 3600:        // 30分钟~1小时
        return 3
    default:                       // > 1小时
        return 4 % r.numPartitions
    }
}

// 使用自定义路由创建生产者
producer, _ := client.CreateProducer(pulsar.ProducerOptions{
    Topic:          "persistent://public/default/delay-test",
    MessageRouter:  &DelayBasedRouter{numPartitions: 5},
})

// 发送时带上延迟信息
producer.Send(ctx, &pulsar.ProducerMessage{
    Payload:      payload,
    DeliverAfter: 30 * time.Minute,
    Properties: map[string]string{
        "delay_seconds": "1800",
    },
})
```

**优缺点：**
- ✅ 最大程度减少乱序确认，每个分区的 ranges 最少
- ✅ 可根据实际延迟分布优化路由策略
- ❌ 需要修改生产者代码，需要维护路由逻辑
- ❌ 延迟分布变化时需要调整路由策略

---

### 方案3：延迟切分（适用于长延迟场景）

**原理：** 将长延迟拆分为多个短延迟，每次确认后重新发送，直到达到目标时间，完全消除长延迟导致的乱序确认。

**原始方案（有问题）：**
```go
// 一次性设置1小时延迟，会产生大量乱序确认
producer.Send(ctx, &pulsar.ProducerMessage{
    Payload:      payload,
    DeliverAfter: 1 * time.Hour,  // ❌ 长延迟，最后消费，积累大量 ranges
})
```

**切分方案（解决问题）：**
```go
const maxSingleDelay = 5 * time.Minute // 每段最大延迟 5 分钟

type DelayTask struct {
    Payload       []byte
    TargetTime    time.Time
    TaskID        string
}

// 发送时计算第一段延迟
func sendWithSplit(producer pulsar.Producer, task DelayTask) {
    now := time.Now()
    remaining := task.TargetTime.Sub(now)

    var nextDelay time.Duration
    if remaining <= maxSingleDelay {
        // 最后一段，直接到目标时间
        nextDelay = remaining
    } else {
        // 还未到最后一段，先延迟 maxSingleDelay
        nextDelay = maxSingleDelay
    }

    taskBytes, _ := json.Marshal(task)
    producer.Send(ctx, &pulsar.ProducerMessage{
        Payload:      taskBytes,
        DeliverAfter: nextDelay,
    })
}

// 消费者收到消息后判断是否到达目标时间
func handleMessage(msg pulsar.Message, producer pulsar.Producer) {
    var task DelayTask
    json.Unmarshal(msg.Payload(), &task)

    if time.Now().Before(task.TargetTime.Add(-1 * time.Second)) {
        // 还没到目标时间，重新发送（继续切分）
        sendWithSplit(producer, task)
    } else {
        // 到达目标时间，执行真正的业务逻辑
        doRealWork(task)
    }
    consumer.Ack(msg)
}
```

**适用场景：**
- 延迟时间 > 10 分钟的场景
- 需要**可取消**延迟任务的场景（在重新发送前检查状态）
- 需要**动态调整**延迟时间的场景

**优缺点：**
- ✅ 完全消除长延迟导致的乱序确认
- ✅ 可实现延迟任务的取消和修改
- ❌ 增加消息数量（1 条变 N 条），增加系统复杂度

---

### 方案4：使用外部延迟调度器（适用于复杂场景）

**原理：** 不使用 Pulsar 的 `DeliverAfter`，改用外部系统（Redis、数据库、专门的调度器）管理延迟任务，Pulsar 只负责消息传输。

**架构：**
```
业务服务
  │
  ├─► Redis/DB 存储延迟任务（含目标触发时间）
  │
定时扫描器（每秒轮询）
  │ 到期任务
  ▼
Pulsar Producer（直接发送，不带 DeliverAfter）
  │
  ▼
Pulsar Consumer（普通顺序消费，无乱序问题）
```

**Redis 实现示例：**
```go
// 发送延迟任务到 Redis ZSet（score = 触发时间戳）
func scheduleDelayTask(rdb *redis.Client, task DelayTask, triggerAt time.Time) {
    rdb.ZAdd(ctx, "delay:tasks", &redis.Z{
        Score:  float64(triggerAt.Unix()),
        Member: task.TaskID,
    })
    rdb.Set(ctx, "delay:data:"+task.TaskID, task.Payload, 24*time.Hour)
}

// 定时扫描器：每秒检查到期任务
func scanner(rdb *redis.Client, producer pulsar.Producer) {
    ticker := time.NewTicker(time.Second)
    for range ticker.C {
        now := float64(time.Now().Unix())
        // 获取所有 score <= now 的任务
        tasks, _ := rdb.ZRangeByScore(ctx, "delay:tasks", &redis.ZRangeBy{
            Min: "0",
            Max: fmt.Sprintf("%f", now),
        }).Result()

        for _, taskID := range tasks {
            payload, _ := rdb.Get(ctx, "delay:data:"+taskID).Bytes()
            // 直接发送到 Pulsar，不带延迟，顺序消费无乱序问题
            producer.Send(ctx, &pulsar.ProducerMessage{Payload: payload})
            rdb.ZRem(ctx, "delay:tasks", taskID)
        }
    }
}
```

**适用场景：**
- 需要复杂延迟管理（可取消、可修改、可查询状态）
- 延迟时间跨度很大（秒级到天级）
- 对延迟精度要求高
- 已有现成的调度系统（如 Quartz、XXL-Job）

**优缺点：**
- ✅ 彻底消除 Pulsar 延迟消息问题，Pulsar 只负责传输性能最优
- ✅ 更灵活的延迟管理
- ❌ 需要额外基础设施，系统复杂度增加
- ❌ 需要维护调度器高可用

---

### 方案5：Broker 配置调优（临时缓解，配合其他方案使用）

> ⚠️ **重要提示：这些配置只能缓解问题，不能根本解决。必须配合方案 1 或方案 2 使用。**

**关键配置（broker.conf）：**
```yaml
# 限制游标 individual delete ranges 的最大数量
# 超过限制后 Broker 会强制压缩，防止无限增长
managedLedgerMaxUnackedRangesToPersist: 10000
managedLedgerMaxUnackedRangesToPersistInMetadataStore: 1000

# 调整 BookKeeper 游标元数据写入限制
managedLedgerCursorMaxEntriesPerLedger: 50000

# 关闭自动负载均衡，防止重连触发问题（临时措施）
# ⚠️ 副作用：Broker 宕机后 topic 不会自动迁移，需要手动处理
loadBalancerEnabled: false
```

**各配置的副作用：**
- `loadBalancerEnabled: false`：Broker 间不自动平衡，某个 Broker 宕机后 topic 不会迁移，需要手动处理
- 增大各 limit 值：只是推迟问题出现，当达到新限制时问题依然会发生，同时占用更多内存和 CPU

**建议：** 配置调优作为紧急止血措施，同时并行推进方案 1 或方案 2。

---

## 五、方案选择建议

### 7. 遇到这个问题怎么处理？怎么跟面试官说？

**分阶段处理策略：**

```
第一步（立即）：
  - 方案5：调整 Broker 配置，提高 individuallyDeletedMessages 上限，临时缓解
  - 如果 Broker 已经卡死，手动做一次 topic 所有权转移解除卡死

第二步（当天）：
  - 方案1：增加分区数，将乱序确认分散，立竿见影

第三步（本周）：
  - 根据业务选方案2或方案3
  - 短延迟（< 10分钟）：方案2 自定义分区路由
  - 长延迟（> 10分钟）：方案3 延迟切分
  - 需要取消/修改延迟：方案4 外部调度器
```

> **面试中怎么说这段经验：**
> 我们在升级 Pulsar 4.0 后，延迟消息服务出现了生产事故。根因是 Pulsar 4.0 修复了并发确认的竞态条件，导致 individuallyDeletedMessages 积累无上限，消费者一旦重连就完全卡死。紧急止血是调整 Broker 配置 + 增加分区数。长期方案是按延迟时间做自定义分区路由，让相似延迟的消息进同一分区，最大程度减少乱序确认。这个问题的核心教训是：Pulsar 的延迟消息适合固定延迟场景，可变延迟场景要特别注意 individual delete ranges 的积累问题。

---

## 附录

- [Pulsar Delayed Message Delivery 官方文档](https://pulsar.apache.org/docs/concepts-messaging/#delayed-message-delivery)
- [PR #22966: Make operations on individuallyDeletedMessages in lock scope](https://github.com/apache/pulsar/pull/22966)
- [Issue #25028: Individual delete ranges cause message delivery issues](https://github.com/apache/pulsar/issues/25028)
