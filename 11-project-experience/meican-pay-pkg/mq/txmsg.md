# MQ 事务消息 (Transactional Message)

## 核心设计

### 解决的问题
数据库事务和消息发送的原子性问题：
- ❌ 先发消息再落库：消息发了但事务回滚
- ❌ 先落库再发消息：落库成功但消息发送失败

### 解决方案：本地消息表 + 异步补偿
```
阶段1：事务内
┌─────────────────────────────────────────┐
│  DB 事务开始                            │
│    1. 执行业务 SQL                      │
│    2. 写入本地消息表（状态: NEW）        │
│  DB 事务提交                            │
└─────────────────────────────────────────┘
           ↓ afterCommit 回调
阶段2：事务提交后
┌─────────────────────────────────────────┐
│  发送消息到 MQ                          │
│  更新消息状态: NEW -> QUEUED            │
└─────────────────────────────────────────┘
           ↓ MQ 发送成功
阶段3：确认完成
┌─────────────────────────────────────────┐
│  更新消息状态: QUEUED -> FINISH         │
└─────────────────────────────────────────┘

阶段4：后台补偿 Daemon
┌─────────────────────────────────────────┐
│  轮询 NEW/QUEUED 状态的超时消息         │
│  重新发送，更新错误次数                 │
│  超过最大重试次数标记为 ERROR           │
└─────────────────────────────────────────┘
```

## 数据模型

### TxMessage 表结构
```go
type TxMessageStatus int

const (
    TxMessageStatusNew    TxMessageStatus = 0 // 新建，待发送
    TxMessageStatusQueued TxMessageStatus = 1 // 已入队，待确认
    TxMessageStatusFinish TxMessageStatus = 2 // 发送完成
    TxMessageStatusError  TxMessageStatus = 3 // 发送失败
)

type TxMessage struct {
    ID         int64             `gorm:"primary_key"`
    Topic      string            // MQ 主题
    Body       []byte            // 消息内容
    Attributes map[string]string // 消息属性（包含 trace、uniqueKey 等）
    Status     TxMessageStatus   // 状态
    ErrorCount int64             // 错误次数
    LastError  sql.NullString    // 最后错误信息
    Backend    string            // MQ 后端: sqs/pulsar
    MessageID  []byte            // MQ 返回的消息 ID
    CreateTime time.Time
    UpdateTime time.Time
}
```

## 核心流程

### 1. 发送事务消息
```go
// 步骤1: 事务内写入本地消息表
func (p *Producer) SendWithTx(ctx context.Context, inputs ...*SendMessageInput) ([]int64, error) {
    tx := xorm.GetTX(ctx)
    msgs := convertToTxMessages(inputs)
    err := dao.Create(ctx, tx.DB(), msgs)  // 状态: NEW
    return ids, err
}

// 步骤2: 事务提交后 afterCommit 回调发送到 MQ
func (cb *AfterCommitCallback) Do(ctx context.Context, tx xorm.TX) {
    // 从事务上下文中取出待发送的消息
    msgs := getMessagesFromTX(tx)
    // 发送到 MQ
    output, _ := producer.SendBatch(ctx, inputs)
    // 更新状态: NEW -> QUEUED
    dao.UpdateStatusToQueued(ctx, cb.db, successIDs)
}

// 步骤3: MQ 发送成功后更新状态
// Daemon 轮询处理，确认发送成功后 QUEUED -> FINISH
```

### 2. 后台 Daemon 补偿
```go
// Daemon 职责:
// 1. 重发超时未发送的消息 (NEW 状态)
// 2. 重发发送失败的消息 (ERROR 状态，error_count < max)
// 3. 清理已完成的过期消息 (FINISH 状态)

func (d *Daemon) runRequeue() {
    // 查找超过 10 秒还未发送的消息
    msgs, _ := dao.FindNeedRequeue(ctx, startTime, endTime, limit)
    // 批量发送
    d.sendBatch(msgs)
}

func (d *Daemon) runErrorRetry() {
    // 查找发送失败但未超过最大重试次数的消息
    msgs, _ := dao.FindLessThanMaxErrorCount(ctx, startTime, maxErrorCount, limit)
    d.sendBatch(msgs)
}

func (d *Daemon) runDeallocate() {
    // 清理 N 天前已完成的消息（归档）
    dao.DeleteExpired(ctx, d.db, expiredTime, maxId)
}
```

## 关键设计点

### 1. 幂等性保证
- 每条消息有唯一键 `uniqueKey`
- 消费者端去重（见 uniqueness.go）

### 2. 多后端支持
- 支持 SQS 和 Pulsar 两种 MQ 后端
- 通过 `Backend` 接口抽象
- 生产者接口统一

### 3. 可观测性
- OpenTelemetry trace 链路传递
- 发送结果 metrics 统计
- 详细日志记录

### 4. 批量优化
- 批量写入数据库
- 批量发送 MQ 消息
- 减少 IO 次数

## 使用示例

```go
// 初始化
db, _ := gorm.Open(...)
txmsg.AutoMigrate(db)

// SQS Producer
sqsProducer := sqs.NewProducer(sqsClient)

// 配置 xorm 事务回调
xdb := xorm.New(db).
    WithAfterSavePointCallbacks(txmsg.NewAfterSavePointCallback(db)).
    WithAfterCommitCallbacks(txmsg.NewAfterCommitCallback(db, sqsProducer, time.Second*10))

ctx := xorm.WithRWDB(context.Background(), xdb)

// 启动后台补偿 Daemon
daemon := txmsg.NewDaemon(txmsg.DaemonConfig{
    DB:       db,
    Producer: sqsProducer,
})
daemon.Run()
defer daemon.Stop()

// 发送事务消息
producer := bizproducer.New(sqsProducer)
_, err := producer.SendWithTx(ctx, &mq.SendMessageInput{
    Body:  []byte("order paid"),
    Topic: "order-events",
})
```

## 思考问题

1. **为什么需要 afterSavePoint 回调？**
   - 嵌套事务场景：子事务成功但外层可能回滚
   - SavePoint 成功时可以先预写消息
   - 外层 commit 时真正发送

2. **Daemon 轮询间隔如何设置？**
   - 支付场景要求高可靠，通常 10s-30s
   - 超时重发窗口要大于业务最大事务执行时间

3. **如何避免消息重复发送？**
   - 数据库唯一键约束（topic + uniqueKey）
   - 消费者端幂等处理
   - 消息版本号 version

4. **消息表数据量大了怎么办？**
   - 按天分表
   - 定期归档 FINISH 状态
   - 冷热数据分离
