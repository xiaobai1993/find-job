# XORM SavePoint 与事务消息面试学习指南

> 🎯 面试高频考点，必须吃透

---

## 一、核心结论（先背下来）

**本地消息表的写入，和业务数据的写入，是在同一个数据库事务里的！**

所以：
1. ✅ 最终事务提交了 → 消息表的消息一起提交
2. ❌ 最终事务回滚了 → 消息表的消息一起回滚，完全消失
3. 🔄 嵌套了多少层，最后都是同一个事务里的数据

---

## 二、场景详解（面试时能讲出场景很加分）

### 场景 1：单层事务发消息

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    // 1. 写支付单（业务数据）
    CreatePaymentSlip(ctx, ps)

    // 2. 发事务消息（只是暂存在 tx 里！）
    producer.SendWithTx(ctx, msg)

    return nil
})
```

**内部实际执行顺序：**

```
执行业务代码 → 你调用 SendWithTx
    ↓
消息存在 tx.data 里（还没写库！）
    ↓
你 return nil，外层准备提交
    ↓
🎯 afterSavePoint 回调执行（外层也算一个 SavePoint）
    ↓
从事务里拿出所有消息，批量 INSERT 到本地消息表
    ↓
真正 Commit 数据库事务
    ↓
✅ 支付单 + 消息表，一起提交成功
```

**⚠️ 关键认知：消息写表是在回调里做的，不是你调用 SendWithTx 的时候做的！**

---

### 场景 2：嵌套事务，内层发消息

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    // 外层写支付单
    CreatePaymentSlip(ctx, ps)

    // 内层嵌套事务
    return xormv2.Transaction(ctx, func(ctx context.Context) error {
        // 内层扣库存
        DeductStock(ctx)

        // 内层发消息
        producer.SendWithTx(ctx, stockMsg)
    })
})
```

**内部执行：**

```
内层 return nil
    ↓
🎯 内层 afterSavePoint 回调
    ↓
从内层 tx 拿出消息，INSERT 本地消息表
    ↓
外层 afterSavePoint 回调
    ↓
从外层 tx 拿出消息，INSERT 本地消息表
    ↓
真正 Commit 整个事务
    ↓
✅ 支付单 + 库存 + 两条消息，一起提交成功
```

---

### 场景 3：嵌套事务，内层和外层都发消息

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    producer.SendWithTx(ctx, paymentMsg)  // 存在外层 tx

    return xormv2.Transaction(ctx, func(ctx context.Context) error {
        producer.SendWithTx(ctx, stockMsg)  // 存在内层 tx
    })
})
```

**结果：两条消息都会被写入本地消息表，一起提交！**

---

### 场景 4：嵌套事务，内层回滚了

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    producer.SendWithTx(ctx, paymentMsg)

    err := xormv2.Transaction(ctx, func(ctx context.Context) error {
        producer.SendWithTx(ctx, stockMsg)
        return errors.New("扣库存失败")
    })

    if err != nil {
        EnqueueStockDeduct(ctx)  // 降级，继续往下走
    }

    return nil
})
```

**结果：只有外层的 paymentMsg 在消息表里，内层的 stockMsg 没了！**

**为什么？**
- 内层出错 → afterSavePoint 根本没执行
- 内层 tx.data 里的消息直接被丢弃了，根本没写表
- 外层的 afterSavePoint 正常执行，只写外层的消息

---

### 场景 5：最外层整个事务回滚

```go
err := xormv2.Transaction(ctx, func(ctx context.Context) error {
    producer.SendWithTx(ctx, msg1)

    xormv2.Transaction(ctx, func(ctx context.Context) error {
        producer.SendWithTx(ctx, msg2)
        return nil
    })

    return errors.New("最后一步校验失败")
})
```

**结果：两条消息都没了！完全消失！**

**为什么？**
- 内层消息确实写了，但还在事务里！
- 整个事务 Rollback → 消息表里的两条消息一起回滚了
- 就像从来没写过一样

---

## 三、结果汇总表

| 场景 | 内层消息 | 外层消息 | 最终结果 |
|------|---------|---------|---------|
| 单层事务成功提交 | - | ✅ 存在 | 消息在表里 |
| 单层事务回滚 | - | ❌ 消失 | 完全没消息 |
| 嵌套都成功 | ✅ 存在 | ✅ 存在 | 两条都在 |
| 内层回滚外层继续 | ❌ 消失 | ✅ 存在 | 只有外层的 |
| 最外层整体回滚 | ❌ 消失 | ❌ 消失 | 完全没消息 |

---

## 四、设计的巧妙之处

> **利用了数据库事务的原子性，天然保证了「业务数据」和「消息数据」的一致性！**

- 不需要分布式事务
- 不需要二阶段提交
- 不需要补偿机制
- 就靠同一个数据库事务的原子性，天然保证

**要么都成功，要么都失败，没有中间状态！**

---

## 五、面试标准回答模板

### 面试官问：事务消息怎么保证一致性？

> "本地消息表这个方案的核心就是利用数据库事务的原子性。
>
> 不管嵌套多少层事务，所有业务数据的修改和消息的写入，最终都是在同一个数据库事务里。
>
> 事务提交了，业务数据和消息数据一起生效；事务回滚了，两者一起消失。
>
> 这就从根本上解决了『业务做了但消息没发』或者『消息发了但业务没做』的一致性问题。
>
> 再配合 afterCommit 回调异步发消息和后台任务兜底重试，就构成了一套完整可靠的最终一致性方案。"

### 面试官问：嵌套事务里发消息会怎么样？

> "嵌套事务的消息处理分两种情况。
>
> 如果内层事务成功了，afterSavePoint 回调会把内层的消息写入本地消息表，最后跟着最外层事务一起提交。
>
> 如果内层事务回滚了，afterSavePoint 根本不会执行，内层的消息直接被丢弃，不会写表。
>
> 所以消息的生命周期完全跟它所在的那层事务绑定，回滚了就没了，成功了就跟着大事务一起提交。"

---

## 六、面试官追问预警（提前准备）

### Q1：afterCommit 回调失败了怎么办？

A：回调失败不影响事务结果，消息已经在本地消息表里了。靠后台 Daemon 定时扫描状态为 NEW 的消息重试发送。at least once 保证。

### Q2：消息重复发送怎么办？

A：消费端做幂等。消息带唯一 ID，消费端判断处理过就直接返回成功。

### Q3：为什么不在 SendWithTx 的时候直接写消息表？

A：因为 SendWithTx 之后事务可能还会回滚。如果当时就写了，事务回滚了消息还在，就会发出去一条错误消息。

所以要等到那层事务确定成功了（afterSavePoint）再写，保证消息和那层事务的一致性。

---

## 七、一句话总结

> **消息的生命周期 = 它所在那层事务的生命周期**
>
> 事务成功，消息就在；事务回滚，消息就没。
