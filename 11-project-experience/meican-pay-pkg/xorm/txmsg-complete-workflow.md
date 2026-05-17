# 事务消息（txmsg）完整执行流程 - 面试版

> ⚠️ **重要纠正：之前 90% 的人都理解错了！**
>
> ❌ 错误认知：afterSavePoint 回调才把消息写入本地消息表
> ✅ **真实情况：调用 `SendWithTx` 的瞬间，消息就已经写入数据库了！**

---

## 一、核心结论（面试先背这个）

| 动作 | 执行时机 | 做什么 |
|------|---------|--------|
| `SendWithTx` | 你调用的瞬间 | ✅ 立刻 INSERT 到本地消息表（在当前事务内）<br>✅ 同时在 `tx.data` 内存里存一份引用 |
| `afterSavePoint` | 子事务成功时 | ✅ 只是内存里把子事务的消息合并到父事务<br>❌ **不操作数据库！** |
| `afterCommit` | 最外层事务提交成功后 | ✅ 从最外层 `tx.data` 拿出所有消息<br>✅ 开 goroutine 异步发送到 MQ |
| 事务回滚 | 任何一层回滚 | ✅ 数据库事务天然把那层的消息一起回滚 |

---

## 二、完整执行流程图

```
你调用 xormv2.Transaction(ctx, func(ctx context.Context) error {
    ...
    producer.SendWithTx(ctx, msg)  ← 🎯 这里就写数据库了！不是等回调！
    ...
})
```

```
┌─────────────────────────────────────────────────────────────┐
│ 步骤 1：SendWithTx 被调用                                    │
│   ✅ INSERT INTO tx_message (status = NEW)                   │
│   ✅ 同时把消息存在当前 tx.data["_txmsgs"] 里                │
│   （消息现在在事务里，外面还看不见）                          │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│ 步骤 2：子事务执行完毕 return nil                            │
│   🎯 afterSavePoint 回调执行                                 │
│   从当前子 tx.data 拿出消息                                  │
│   合并到父 tx.data["_txmsgs"] 数组里                        │
│   ❌ 不操作数据库！只是内存合并                               │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│ 步骤 3：一层一层往上合并                                     │
│   子 → 父 → 祖父 → ... → 最外层                              │
│   最后所有消息都在最外层 tx.data 里                         │
└──────────────────────────────┬──────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────┐
│ 步骤 4：最外层事务 Commit 成功                               │
│   ✅ 所有业务数据 + 所有消息，一起永久生效                   │
│   🎯 afterCommit 回调执行                                    │
│   从最外层 tx.data 拿出所有消息                              │
│   开 goroutine 异步批量发送到 MQ                             │
│   发送成功 → 更新状态为 Finish                               │
│   发送失败 → 更新状态为 Error，Daemon 兜底重试               │
└─────────────────────────────────────────────────────────────┘
```

---

## 三、5 个核心场景的真实结果

### 场景 1：单层事务，成功提交

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    CreatePaymentSlip(ctx, ps)
    producer.SendWithTx(ctx, msg)  // ← 这里就写消息表了
    return nil
})
```

**结果：**
- SendWithTx 时消息就 INSERT 了（在事务里）
- Commit 后消息永久存在，状态 NEW
- afterCommit 异步发 MQ，成功后更新为 Finish

---

### 场景 2：嵌套事务，都成功

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    CreatePaymentSlip(ctx, ps)
    producer.SendWithTx(ctx, paymentMsg)

    return xormv2.Transaction(ctx, func(ctx context.Context) error {
        DeductStock(ctx)
        producer.SendWithTx(ctx, stockMsg)
    })
})
```

**结果：**
- 两条消息都在各自的 SendWithTx 时 INSERT
- afterSavePoint 把 stockMsg 合并到外层
- afterCommit 两条一起发 MQ
- 最终两条消息都在表里，都发出去

---

### 场景 3：内层回滚，外层继续（经典面试题）

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    producer.SendWithTx(ctx, paymentMsg)  // 外层消息写了

    err := xormv2.Transaction(ctx, func(ctx context.Context) error {
        producer.SendWithTx(ctx, stockMsg)  // 内层消息也写了
        return errors.New("扣库存失败")
    })

    if err != nil {
        EnqueueStockDeduct(ctx)  // 降级，不回滚外层
    }
    return nil
})
```

**结果：**
- ✅ 外层的 paymentMsg 还在
- ❌ 内层的 stockMsg 没了！**被 SavePoint 回滚了！**
- 最终只有 paymentMsg 发出去

**这就是 SavePoint 的核心价值！内层的修改（包括消息表）可以单独回滚！**

---

### 场景 4：最外层整体回滚

```go
xormv2.Transaction(ctx, func(ctx context.Context) error {
    producer.SendWithTx(ctx, msg1)

    xormv2.Transaction(ctx, func(ctx context.Context) error {
        producer.SendWithTx(ctx, msg2)
        return nil
    })

    return errors.New("最后一步失败")
})
```

**结果：**
- 两条消息都 INSERT 了，但都在同一个大事务里
- 整个 Rollback → 两条消息一起消失，跟没写过一样

---

### 场景 5：嵌套 3 层，中间层回滚

```go
外层 Transaction
    发 msg1
    中层 Transaction
        发 msg2
        内层 Transaction
            发 msg3
            return err  ← 内层回滚
        发 msg4
    return nil
```

**结果：**
- ✅ msg1、msg2、msg4 存在
- ❌ msg3 被回滚没了

---

## 四、为什么要这么设计？三个核心理由

### 理由 1：天然利用事务原子性（最核心）
**消息在 SendWithTx 时就写库，和业务数据在同一个事务里！**
- 不需要等回调，不需要额外处理
- 数据库事务天然保证：要么都成功，要么都失败
- 这是本地消息表方案的灵魂

### 理由 2：避免 commit 后查消息表的性能问题
如果不在 tx.data 里存一份，commit 后怎么知道这个事务产生了哪些消息？

❌ 错误做法：`SELECT * FROM tx_message WHERE create_time > xxx`
- 慢
- 会把其他事务的消息也查出来
- 高并发下根本不可行

✅ 正确做法：内存里存一份，commit 后直接拿，零成本

### 理由 3：嵌套事务消息追踪
嵌套 N 层，每层都发消息，最后怎么全部找出来发？

靠 afterSavePoint 一层一层往上合并，最后都到最外层的 tx.data 里，
最外层 afterCommit 一次性拿全，一次性发。

---

## 五、v1 vs v2 版本的区别

核心逻辑完全一样，只有一个区别：**消息存在哪里**

| 对比项 | v1 | v2 |
|--------|-----|-----|
| 消息写库时机 | SendWithTx 时 | SendWithTx 时 |
| afterSavePoint 做什么 | 内存合并消息 | 内存合并消息 |
| 内存存储位置 | 自己维护的 `tx.data map[interface{}]interface{}` | GORM 内置的 `InstanceSet` |

v2 只是用了 GORM 自带的能力，不再自己维护 map，更可靠。

---

## 六、面试标准回答模板

### 面试官问：事务消息的完整流程是怎样的？

> "事务消息的执行分四个阶段。
>
> 第一，调用 SendWithTx 的时候，消息就立刻被插入到本地消息表了，这是在当前数据库事务里的，跟业务数据一起。
>
> 第二，每一层子事务成功后，afterSavePoint 回调会把那层的消息在内存里合并到父事务，这个阶段不操作数据库，只是内存合并。
>
> 第三，一层一层往上合并，直到最外层事务。
>
> 第四，最外层事务提交成功后，afterCommit 回调从最外层 tx 的内存里拿出所有消息，开 goroutine 异步发送到 MQ。
>
> 如果任何一层回滚，因为消息是在数据库事务里写的，自然跟着那层一起回滚了，不需要额外处理。
>
> 这个设计的好处是天然利用了数据库事务的原子性，保证了业务数据和消息数据的一致性，同时内存合并的方式性能也很好。"

### 面试官问：内层事务回滚了，内层发的消息怎么办？

> "内层发的消息会跟着内层 SavePoint 一起回滚，就像从来没发过一样。
>
> 因为消息是在 SendWithTx 的时候就写入内层事务里的，内层回滚了，那层的所有修改包括消息表的修改都会被回滚。
>
> 而且内层的 afterSavePoint 根本不会执行，所以消息也不会被合并到外层。
>
> 最终只有成功提交的那几层的消息才会留下来发出去。"

---

## 七、面试官追问预警

### Q1：SendWithTx 就写库了，如果外层回滚怎么办？

A：因为写库是在事务里的，外层回滚，消息自然跟着回滚，就像没写过一样。

### Q2：如果 afterCommit 回调执行的时候服务挂了怎么办？

A：消息已经在表里，状态是 NEW。后台有 Daemon 定时扫描超过一定时间的 NEW 消息，兜底重试发送。

### Q3：为什么不在 afterSavePoint 才写消息表？

A：有两个问题：
1. 如果 SendWithTx 和 afterSavePoint 之间有数据库操作失败，消息就丢了，因为根本没写库
2. afterSavePoint 才写的话，消息的 create_time 会晚于业务数据，逻辑上不对

### Q4：消息重复发送怎么办？

A：消息带唯一 ID（txmsg:xxx），消费端做幂等。at least once 保证。

### Q5：如果一个事务里发 100 条消息，内存会不会爆？

A：一般不会，事务里不应该发这么多消息。真的多的话会分批，而且 tx 是请求级别的，请求结束就释放了。

---

## 八、一句话终极总结

> **SendWithTx 写库，afterSavePoint 合并内存，afterCommit 发 MQ，回滚靠数据库天然保证。**
