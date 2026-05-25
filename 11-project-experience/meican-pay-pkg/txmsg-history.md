# txmsg 历史修复与排查线索

## 重要历史问题

1. 重复消息 / 补偿时序
   - `f11fadd` (`fix: repeat msg`): daemon 补偿 `NEW/QUEUED` 消息时增加 stale 时间窗口，避免刚提交后 after-commit callback 还在发送，daemon 又立即补偿导致重复。
   - 当前相关逻辑：`mq/txmsg/v2/dao.go` 的 `FindNeedRequeue` 使用 `create_time >= startTime AND create_time <= endTime`。

2. 批量发送遗漏 / 只处理一批
   - `d2cd01e` (`fix: txmsg send in batch`): daemon 从单次 batch 查询发送，改为循环处理所有可处理消息；并加入 `Order("id ASC")` 稳定扫描顺序。

3. daemon 解锁对象错误
   - `d2cd01e` 同时修复：`runWorker(m redislock.Mutex, ...)` 之前 defer 解锁固定用了 `d.handleStalesMutex.Unlock`，导致 retryErrors 分支拿 `retryErrorsMutex` 却解错锁；修为 `m.Unlock`。

4. daemon 长事务问题
   - `d91dba5` (`fix: txmsg runWorker 移除事务`): 移除 runWorker 对整个补偿发送流程的 `d.db.Transaction(...)` 包裹。发送 MQ 是外部 IO，放在 DB 事务里会导致事务过长、锁/连接占用、超时。

5. txmsgv2 读写分离从库延迟
   - `1c8463d` (`fix: txmsgv2 强制读主`): `txmsg/v2` 补偿查询加 `dbresolver.Write` 强制读主，避免从库延迟读到旧状态造成重复补偿、漏补偿或状态判断不准。
   - 注意：老 `mq/txmsg` 未必有同样 `dbresolver.Write`。

6. `NewAfterCommitCallback` nil map panic
   - `30a81c3` (`fix: NewAfterCommitCallback`): 初始化 `producersMap: map[mq.Backend]mq.Producer{}`，否则 `WithSQSProducer` / `WithPulsarProducer` 写 nil map 会 panic。

7. tracing 调用适配
   - `707d8fe` (`fix: mutex.Lock`): 实际 diff 是 `input.SetTracing(ctx, impl.producer.Backend())` 改为 `input.SetTracing(ctx)`，commit message 不准确。

8. 避免全表扫描
   - `722d9c7` (`fix: avoid full table scan when select stales and errors for txmsg`): 早期 `mq/pulsar/txmsg` 修复，给补偿扫描增加时间范围/索引字段，索引从 `status, update_time` 调整为更贴合扫描的 `status, create_time`。后续 `mq/txmsg`/`txmsg/v2` 延续 `create_time` 范围查询。

9. 移除 DB 悲观锁，改 Redis 分布式锁
   - `c26d8d4` (`refactor: remove txmsg db lock`): 移除 `FOR UPDATE` 查询，使用 Redis mutex 控制 daemon 任务并发；增加 `startPeriod/endPeriod` 时间窗口，避免无限扫历史和 DB 锁竞争。

10. MQ unique key 拼 txmsg id
   - `2fcd282` (`refactor: msg unique key with txmsg id`): 发送前 `SetUniqueKey(fmt.Sprintf("%s-%d", oldUniqueKey, txmsg.ID))`，避免业务传入相同 unique key 时多个 txmsg 在 MQ/消费端唯一性判断互相冲突。

11. txmsgv2 初期兼容修复
   - `16c2b4f` (`fix: txmsgv2`): schema 从 `_txmsgv2` 改回 `_txmsg`，Redis lock 名称从 `txmsgv2-...` 改为 `txmsg-...`，`FormatTxMessageID` 从 `txmsgv2:%d` 改为 `txmsg:%d`，以及导出函数/日志字段命名修正。

## afterSavePointCallback 重复发送问题线索

用户记得 `afterSavePointCallback.Do` 这段曾造成“相同消息重复发送”：

```go
spMsgs := c.getMsgsFromDB(tx)
parentTX := tx.Parent()
msgs := c.getMsgsFromDB(parentTX)
msgs = append(msgs, spMsgs...)
parentTX.Put(msgsKey, msgs)
```

排查结果：`mq/txmsg/v2/after_save_point_callback.go` 这段合并逻辑从 `d59bcc2` 创建后基本没有直接修过去重。更可能对应 `xorm/v2` 的修复：

- `96b225d` (`fix: xormv2 使用 gorm 的 InstanceSet 和 InstanceGet`)
  - `tx.Put`: `s.db.Set(k, v)` -> `s.db.InstanceSet(k, v)`
  - `tx.Get`: `s.db.Get(k)` -> `s.db.InstanceGet(k)`
  - `newSubTX`: `Session(&gorm.Session{})` -> `Session(&gorm.Session{Context: s.db.Statement.Context})`

可能重复链路：
1. 子事务中 `SendWithTx` 创建 txmsg，并通过 `impl.putMessagesToDB(tx, msgs)` 放入当前 TX 的 `msgsKey`。
2. savepoint 成功后，`afterSavePointCallback` 把子事务 `spMsgs` append 到父事务。
3. 若 `xorm/v2` 使用 gorm `Set/Get`，父子 TX 的 data 可能共享/继承，导致父事务在 append 前已经能看到子事务消息。
4. 于是 `msgs = append(msgs, spMsgs...)` 把同一个 txmsg 追加两次，outer commit 的 `afterCommitCallback` 对同一 txmsg 记录发送两次。
5. 改为 `InstanceSet/InstanceGet` 后，父子 TX data 隔离，避免同一消息被重复合并。

老版 `xorm/tx.go` 使用独立 `data map[interface{}]interface{}` 存 TX data，因此不像是这个 gorm `Set/Get` 共享问题。
