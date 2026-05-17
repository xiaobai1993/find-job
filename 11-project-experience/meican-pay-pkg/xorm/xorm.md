# XORM - ORM 事务封装

## 核心设计

### 问题：原生 GORM 事务没有回调
```go
// 原生 GORM
err := db.Transaction(func(tx *gorm.DB) error {
    // 事务内操作
    return nil
})

// 提交后想做事情？只能在外面做，但可能失败！
```

### 解决方案：带回调的事务封装
```go
type TX interface {
    DB
    Commit(ctx context.Context) error
    Rollback()
    Parent() TX              // 嵌套事务父节点
    Put(k, v interface{})    // 事务上下文存数据
    Get(k interface{}) (interface{}, bool)
}

type tx struct {
    parent                  TX
    db                      *gorm.DB
    data                    map[interface{}]interface{}

    // 事务提交后回调
    afterCommitCallbacks    []Callback

    // SavePoint 成功回调（嵌套事务）
    afterSavePointCallbacks []Callback
}
```

## 嵌套事务实现

### SavePoint 机制
```go
func (s *tx) Transaction(ctx context.Context, fc func(ctx context.Context) error) error {
    panicked := true
    var savePointName string

    // 1. 创建 SavePoint
    if !s.db.DisableNestedTransaction {
        savePointName = fmt.Sprintf("sp%p", fc)
        s.db.SavePoint(savePointName)

        defer func() {
            // panic 或 error 时 Rollback 到 SavePoint
            if panicked || err != nil {
                s.db.RollbackTo(savePointName)
            }
        }()
    }

    // 2. 执行业务逻辑
    if err == nil {
        subTx := s.newSubTX()
        c := withTX(ctx, subTx)
        if err = fc(c); err == nil {
            // 3. SavePoint 成功，执行回调
            subTx.callAfterSavePointCallbacks(c)
        }
    }

    panicked = false
    return err
}
```

### 事务提交执行回调
```go
func (s *tx) Commit(ctx context.Context) error {
    if s.db.Commit().Error != nil {
        return s.db.Error
    }

    // 提交成功后执行所有回调
    s.callAfterCommitCallbacks(ctx)
    return nil
}
```

## 回调应用：事务消息

### afterSavePoint 回调
```go
// 嵌套事务中，子事务 SavePoint 成功时：
// 把消息从事务上下文取出，写入 DB
func (cb *AfterSavePointCallback) Do(ctx context.Context, tx xorm.TX) {
    // 从事务上下文取出待发送的消息
    messages, ok := tx.Get(txMessageCtxKey)
    if !ok {
        return
    }

    // 批量写入本地消息表（状态 NEW）
    dao.Create(ctx, tx.DB(), messages)
}
```

### afterCommit 回调
```go
// 最外层事务提交成功时：
// 把消息发送到 MQ
func (cb *AfterCommitCallback) Do(ctx context.Context, tx xorm.TX) {
    // 从父事务逐层查找消息
    for t := tx; t != nil; t = t.Parent() {
        messages, ok := t.Get(txMessageCtxKey)
        if ok {
            // 发送到 MQ
            cb.producer.SendBatch(ctx, messages)
            return
        }
    }
}
```

## 上下文传递

### DB/TX 上下文管理
```go
type contextKey string

const (
    rwDBKey contextKey = "rw_db"  // 写库
    roDBKey contextKey = "ro_db"  // 读库
)

// 设置写库
func WithRWDB(ctx context.Context, db DB) context.Context {
    return context.WithValue(ctx, rwDBKey, db)
}

// 获取写库（事务中自动返回 TX）
func GetRWDB(ctx context.Context) DB {
    v := ctx.Value(rwDBKey)
    if db, ok := v.(DB); ok {
        return db
    }
    return globalDB
}

// 判断是否在事务中
func IsTXOpen(ctx context.Context) bool {
    db := GetRWDB(ctx)
    tx, ok := db.(TX)
    return ok && tx.InTransaction()
}
```

## 使用示例

```go
// 初始化 DB，配置事务消息回调
db := xorm.New(gormDB).
    WithAfterSavePointCallbacks(txmsg.NewAfterSavePointCallback(gormDB)).
    WithAfterCommitCallbacks(txmsg.NewAfterCommitCallback(gormDB, mqProducer, 10*time.Second))

ctx := xorm.WithRWDB(context.Background(), db)

// 执行业务事务
err := xorm.Transaction(ctx, func(ctx context.Context) error {
    // 1. 写订单表
    err := xorm.GetRWDB(ctx).Create(&order).Error
    if err != nil {
        return err
    }

    // 2. 发送事务消息（写入本地消息表）
    _, err = producer.SendWithTx(ctx, &mq.SendMessageInput{
        Body:  []byte("order created"),
        Topic: "order-events",
    })
    return err
})

// 事务提交后：
// - afterSavePoint 回调：消息已写入本地表
// - afterCommit 回调：消息发送到 MQ
```

## 思考问题

1. **为什么需要 afterSavePoint 回调？**
   - 嵌套事务场景：子事务成功，外层可能回滚
   - SavePoint 成功时可以先持久化消息
   - 外层回滚时消息也一起回滚

2. **嵌套事务的消息存在哪里？**
   - 存在各自的 subTx.data 中
   - Commit 时从父到子遍历收集所有消息
   - 保证消息不丢失

3. **回调执行失败怎么办？**
   - afterCommit 失败不影响事务结果
   - 靠后台 Daemon 补偿重发
   - 消息状态是 NEW，Daemon 会兜底

4. **回调顺序如何保证？**
   - 按注册顺序执行
   - 事务消息回调应该尽早注册
