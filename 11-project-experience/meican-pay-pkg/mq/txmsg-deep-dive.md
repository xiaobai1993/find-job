# 本地消息表（事务消息）深度解析 - 面试版

> 🎯 核心考点：最终一致性、本地消息表模式、事务原子性、补偿机制、消息幂等
> 源码位置：`pkg/mq/txmsg/`

---

## 一、这是什么？

这是**本地消息表（Local Message Table）**模式的事务消息实现，用来解决「数据库事务」和「消息发送」的原子性问题。

**经典问题：**
```go
// 银行转账
tx.Begin()
userA.balance -= 100  // 扣钱
userB.balance += 100  // 加钱
tx.Commit()

mq.Send("转账成功通知")  // ❌ 这里挂了怎么办？
// 事务已经提交了，但是消息没发出去，数据不一致！
```

**美餐的解决方案：本地消息表 + 事务回调 + 后台补偿**

---

## 二、核心数据结构

### TxMessage 消息表

```go
type TxMessage struct {
    ID          int64             // 自增主键
    Data        string            // 消息内容（JSON 格式，便于人读）
    Topic       string            // 要发送到哪个 Topic
    Status      TxMessageStatus   // 消息状态
    LastError   sql.NullString    // 最后一次错误
    ErrorCount  uint              // 重试次数
    MessageID   []byte            // MQ 返回的消息 ID
    Backend     mq.Backend        // MQ 后端类型（Pulsar/SQS）
    CreateTime  time.Time         // 创建时间
    UpdateTime  time.Time         // 更新时间（扫描补偿用）
}
```

### 状态机流转

```
New  --(事务提交后)-->  Queued  --(发送成功)-->  Finish
                                \
                                 --(发送失败)-->  Error  --(重试)-->  Queued
```

| 状态 | 含义 |
|-----|------|
| **New** | 刚插入数据库，事务还没提交 |
| **Queued** | 事务已提交，待发送 |
| **Finish** | 发送成功 |
| **Error** | 发送失败，等待重试 |

---

## 三、核心流程：四步保证最终一致性

### 第一步：事务内写消息表

```go
func (impl *xormImpl) SendWithTx(ctx context.Context, inputs ...*mq.SendMessageInput) ([]int64, error) {
    // 1. 校验并序列化消息
    txMsg, _ := impl.newTxMessage(input)

    // 2. ✅ 重点：必须在同一个数据库事务里写！
    err := impl.mustRunInTransaction(ctx, func(ctx context.Context) error {
        // 业务数据和消息表在同一个事务里提交
        return impl.createMessagesAndPutMessageToDBData(ctx, txMsgs)
    })

    return ids, err
}
```

**关键点：**
> 业务表 UPDATE + 消息表 INSERT，这两个操作必须在**同一个数据库事务**里！
>
> 要么一起成功，要么一起失败，原子性由数据库保证。

---

### 第二步：事务回调，立即发送

```go
// 事务提交后触发 after_commit 回调
func afterCommit(ctx context.Context) {
    // 从事务上下文里拿出刚才写的消息
    msgs := tx.Get("_txmsgs").([]*TxMessage)

    // 状态更新为 Queued
    dao.UpdateStatus(ctx, msgs, TxMessageStatusNew, TxMessageStatusQueued)

    // 立即发送消息
    for _, msg := range msgs {
        go producer.Send(ctx, msg)  // 异步发送
    }
}
```

**设计思考：**
> 为什么不直接在 SendWithTx 里发？
> - 事务还没提交！发了消息但是事务回滚了，消息就脏了
> - 必须等事务真正提交成功后才能发

---

### 第三步：后台 Daemon 定时补偿

```go
// 后台定时任务，每分钟跑一次
func daemonScan(ctx context.Context) {
    // 找出：超过一定时间还没发出去的消息
    // Status = New 或者 Status = Error
    staleMsgs := dao.FindStaleMessages(ctx, time.Now().Add(-5*time.Minute))

    for _, msg := range staleMsgs {
        // 重试发送
        err := producer.Send(ctx, msg)
        if err != nil {
            msg.ErrorCount++
            msg.LastError = err.Error()
            dao.UpdateStatus(ctx, msg, TxMessageStatusError)
        } else {
            dao.UpdateStatus(ctx, msg, TxMessageStatusFinish)
        }
    }
}
```

**设计亮点：**
> 补偿是最终一致性的关键！
>
> 即使第二步回调里发送失败了（比如 MQ 挂了），后台 Daemon 也会捞出来重试，保证消息最终一定会发出去。

---

### 第四步：消费端幂等（配合 UniquenessChecker）

```go
// 消费消息时，先做唯一性检查
func (h *uniquenessChecker) Check(ctx context.Context, msg *ReceiveMessageOutput, f func() error) error {
    // 用 Redis 分布式锁，保证同一个消息只处理一次
    key := msg.GetUniqueKey()
    mutex := h.locker.NewMutex("unique-" + key, 24*time.Hour, false)

    if err := mutex.Lock(ctx); err != nil {
        if errors.Is(err, redsync.ErrFailed) {
            return ErrMsgProcessed  // 已经处理过了，直接返回成功
        }
    }

    return f()  // 执行业务逻辑
}
```

**为什么消费端也要做？**
> MQ 本身保证「至少投递一次」（at-least-once），不保证「只投递一次」。
>
> 所以消费端必须做幂等，不然重复消费会出问题！

---

## 四、设计亮点 & 面试加分点

### ✅ 亮点 1：消息内容存 JSON，便于人读

```go
Data string `gorm:"column:data;not null;size:65535"`  // 存 JSON，不是二进制
```

> 为什么不存压缩的二进制？
> - 线上排查问题时，DBA 直接 select 就能看到消息内容
> - 不需要解码工具，直接就能知道这个消息是啥
> - 开发效率比那点存储成本重要多了

---

### ✅ 亮点 2：事务上下文透传

```go
const msgsKey = "_txmsgs"

// 写消息表时同时写到事务上下文里
func (impl *xormImpl) putMessagesToDB(tx xorm.TX, msgs []*TxMessage) {
    v, ok := tx.Get(msgsKey)
    if !ok {
        tx.Put(msgsKey, msgs)
        return
    }
    // 追加到已有列表
}

// after_commit 回调时从事务上下文取出来发
```

> 为什么不直接让业务传消息列表给回调？
> - 业务代码不需要知道这个机制的存在
> - 调用 `SendWithTx` 就完事了，剩下的底层自动处理
> - 业务侵入性最小

---

### ✅ 亮点 3：解锁失败的补偿机制

```go
// 消费失败时，解锁要重试 10 次！
func (h *uniquenessChecker) tryToEnsureUnlockSuccess(ctx context.Context, mutex redislock.Mutex) {
    err := mutex.Unlock()
    if err == nil {
        return
    }

    // 解锁失败，后台异步重试 10 次
    safego.Go(ctx, func(ctx context.Context) {
        for i := 0; i < unlockRetries; i++ {
            if h.unlock(ctx, mutex) {
                return
            }
            time.Sleep(unlockTickDuration)
        }
    })
}
```

> 为什么解锁也要重试？
> - Redis 可能临时抖动，解锁调用失败
> - 解锁失败 = 这个 key 24 小时内无法再次处理
> - 等于消息卡死了！
>
> 所以解锁失败必须重试，实在不行就人工介入。

---

### ✅ 亮点 4：自动建表

```go
// AutoMigrate 自动创建 tx_message 表
func AutoMigrate(ctx context.Context, db xorm.DB) error {
    return db.Gorm().WithContext(ctx).AutoMigrate(&TxMessage{})
}
```

> 业务方只要调用一次 `AutoMigrate`，不用自己建表。
> 降低接入成本，也避免建表出错。

---

## 五、常见坑 & 解决方案

### ❌ 坑 1：消息丢了，永远发不出去

**场景：**
- 事务提交成功了
- after_commit 回调还没触发，服务就重启了
- 消息一直是 New 状态，没人管

**解决方案：Daemon 扫描！**
```go
// 找出超过 5 分钟还是 New 状态的消息
WHERE status = 0 AND update_time < NOW() - INTERVAL 5 MINUTE
```

> 只要 Daemon 在跑，消息就不可能丢。

---

### ❌ 坑 2：重复消费

**场景：**
- MQ 投递了消息
- 消费端处理了，但是 ACK 超时没返回
- MQ 认为消费失败，又投递了一次

**解决方案：UniquenessChecker + Redis 锁！**
```go
// 同一个 uniqueKey，24 小时内只处理一次
mutex := h.locker.NewMutex("unique-"+key, 24*time.Hour, false)
```

> 幂等是 MQ 消费的基本素养，任何时候都要做。

---

### ❌ 坑 3：消息一直失败，死循环重试

**场景：**
- 消息本身有问题，比如格式错误
- 每次重试都失败
- Daemon 无限捞，无限重试

**解决方案：ErrorCount + 最大重试次数！**
```go
if msg.ErrorCount > maxRetry {
    // 标记为死信，不重试了，告警人工处理
    alertToSlack(msg)
    return
}
```

> 重试也要有节制，不然数据库和 MQ 都被打满了。

---

### ❌ 坑 4：Daemon 多实例并发扫描

**场景：**
- 部署了 10 个服务实例
- 每个实例都跑 Daemon
- 同一条消息被 10 个实例同时捞出来处理

**解决方案：加分布式锁！或者用 SELECT ... FOR UPDATE SKIP LOCKED**

```sql
--  MySQL 8.0+ 支持，抢到锁的才能处理
SELECT * FROM tx_message WHERE ... FOR UPDATE SKIP LOCKED
```

> 或者 Daemon 只跑一个实例（单点但问题不大，挂了也不会丢消息）

---

## 六、和其他事务消息方案的对比

| 方案 | 优点 | 缺点 | 代表产品 |
|-----|------|------|---------|
| **本地消息表** | 简单可靠、不依赖 MQ 特性 | 有额外表、有延迟 | 美餐、绝大部分公司 |
| **RocketMQ 事务消息** | 协议层支持、业务侵入小 | 依赖特定 MQ、实现复杂 | RocketMQ |
| **Seata AT 模式** | 无侵入、自动回滚 | 需要全局事务协调器、性能损耗 | Seata |
| **TCC** | 强一致、可控性高 | 代码侵入大、实现复杂 | 蚂蚁金服 |
| **Saga** | 长事务友好、无锁 | 补偿逻辑难写 | 复杂业务流程 |

---

## 七、面试标准回答模板

### 面试官问：你们怎么保证数据库事务和消息发送的原子性？

> "我们用的是**本地消息表**模式，这是工业界最成熟、最可靠的最终一致性方案。
>
> 整个流程分四步：
>
> **第一步，事务内写消息表。** 业务数据更新和消息表 INSERT 在同一个数据库事务里，由数据库保证原子性，要么一起成功要么一起失败。
>
> **第二步，事务回调立即发送。** 事务提交成功后，触发 after_commit 回调，把刚才写的消息立即异步发送出去，这一步是为了低延迟。
>
> **第三步，后台 Daemon 定时补偿。** 每分钟扫描一次消息表，把超过一定时间还没发出去的消息捞出来重试，这一步是为了可靠性，防止回调那一步因为服务重启或者 MQ 挂了丢消息。
>
> **第四步，消费端幂等。** 因为 MQ 是至少投递一次，所以消费端用 Redis 分布式锁做唯一性检查，同一个消息 24 小时内只处理一次。
>
> 这个方案的好处是：不依赖任何 MQ 的高级特性，Pulsar、SQS、Kafka 都能用，简单可靠，上线这么多年从来没出过一致性问题。
>
> 当然代价是有额外的数据库表，而且是最终一致，不是强一致，但对于支付业务 99% 的场景，最终一致完全够用了。"

---

### 面试官追问：如果 Daemon 也挂了怎么办？消息会不会丢？

> "不会丢，这就是本地消息表最牛逼的地方——消息是存在数据库里的，只要数据库不丢，消息就不丢。
>
> Daemon 挂了，最多是消息发送延迟了，等 Daemon 恢复了，还会把之前没发的消息都捞出来重发。
>
> 极端情况：整个服务集群都挂了，重启后 Daemon 一跑，所有积压的消息都会补发。
>
> 这也是为什么本地消息表比各种「内存事务」可靠得多——内存是易失的，数据库是持久的。
>
> 当然前提是你的数据库有备份，不会真的丢数据 :)

---

### 面试官追问：那消费端的幂等怎么做才好？

> "幂等有几个层级，看业务对一致性的要求：
>
> **第一层：数据库唯一索引。** 最可靠，比如订单号建唯一索引，重复插入直接报错，数据库层面保证不重复。
>
> **第二层：Redis 分布式锁。** 像我们的 UniquenessChecker 这样，同一个 uniqueKey 加锁，24 小时内只处理一次，适合没有唯一键的场景。
>
> **第三层：业务状态机校验。** 比如订单状态是「已支付」，就不能再支付一次，不管消息来多少次，状态机都会拦住。
>
> 我们的做法是三层都做：数据库唯一索引兜底，Redis 锁挡大部分重复请求，业务状态机做最后防线。
>
> 记住：MQ 永远是 at-least-once，任何假设消息只会来一次的代码，早晚会出 bug。"

---

## 八、一句话总结

> **本地消息表 = 事务内写消息 + 提交后立即发送 + 后台定时补偿 + 消费端幂等**
>
> 它不优雅，也不高级，但它是分布式系统里最朴实、最可靠的最终一致性方案。

---

## 📝 面试要点复盘

1. ✅ **为什么需要本地消息表** - 解决事务和消息发送的原子性问题
2. ✅ **四步完整流程** - 写表、回调、补偿、幂等
3. ✅ **关键设计细节** - JSON 存人读、事务上下文透传、解锁重试
4. ✅ **四个经典坑及解决方案** - 丢消息、重复消费、死循环重试、并发扫描
5. ✅ **和其他方案的对比** - 本地消息表 vs RocketMQ 事务消息 vs Seata vs TCC
6. ✅ **最终一致性的哲学** - 接受延迟，换取可靠和简单
