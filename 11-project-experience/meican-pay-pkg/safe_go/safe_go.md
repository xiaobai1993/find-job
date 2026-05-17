# Safe Go - 安全的 Goroutine

## 解决的问题

直接用 `go func() {}()` 的问题：
- ❌ panic 会导致整个进程崩溃
- ❌ 事务上下文可能被误用（goroutine 中继续使用事务 DB）
- ❌ 没有统一的错误日志

## 核心实现

```go
func Go(ctx context.Context, fn func(ctx context.Context)) {
    go func() {
        // 关键：如果 ctx 中有事务，替换为全局 DB
        if xorm.IsTXOpen(ctx) {
            ctx = xorm.WithRWDB(ctx)
        }
        if xormv2.IsTXOpen(ctx) {
            ctx = xormv2.WithDB(ctx)
        }

        // 包装 panic 恢复
        WithRecover(ctx, fn)()
    }()
}

func WithRecover(ctx context.Context, fn func(ctx context.Context)) func() {
    return func() {
        defer func() {
            if e := recover(); e != nil {
                log.For(ctx).Error("recovered from panic",
                    log.Any("panic", e),
                    log.Stack("stacktrace"),
                )
            }
        }()
        fn(ctx)
    }
}
```

## 设计亮点

### 1. 事务上下文自动剥离

**问题场景：**
```go
// 错误：事务提交后，goroutine 中还在使用事务 DB
xorm.Transaction(ctx, func(ctx context.Context) error {
    db := xorm.GetDB(ctx)
    db.Create(&order)

    // ❌ goroutine 中继续使用事务上下文
    go func() {
        // 事务可能已提交/回滚，DB 连接可能已归还
        db.Create(&log)  // 可能出问题！
    }()
    return nil
})
```

**正确用法：**
```go
xorm.Transaction(ctx, func(ctx context.Context) error {
    db.Create(&order)

    // ✅ safe_go.Go 自动剥离事务，使用全局 DB
    safego.Go(ctx, func(ctx context.Context) {
        xorm.GetDB(ctx).Create(&log)  // 安全！
    })
    return nil
})
```

### 2. 统一 panic 恢复
- panic 不会导致进程崩溃
- 自动记录堆栈信息
- 可配合 ErrGroup 使用

## 使用场景

### 场景1：异步发送通知
```go
func CreateOrder(ctx context.Context, order *Order) error {
    return xorm.Transaction(ctx, func(ctx context.Context) error {
        if err := db.Create(order).Error; err != nil {
            return err
        }

        // 异步发送短信，不影响主流程
        safego.Go(ctx, func(ctx context.Context) {
            sms.Send(order.Phone, "订单创建成功")
        })

        return nil
    })
}
```

### 场景2：配合 ErrGroup
```go
g, ctx := errgroup.WithContext(ctx)

g.Go(safego.ErrGroupFunc(ctx, func(ctx context.Context) error {
    return userService.Query(ctx, userID)
}))

g.Go(safego.ErrGroupFunc(ctx, func(ctx context.Context) error {
    return orderService.Query(ctx, orderID)
}))

return g.Wait()
```

## 思考问题

1. **为什么要剥离事务上下文？**
   - goroutine 执行时外层事务可能已结束
   - 事务 DB 连接可能已归还连接池
   - 继续使用会导致不可预期的行为

2. **WithRecover 只打日志不报问题？**
   - 业务逻辑自行处理错误
   - panic 是程序 bug，应该告警
   - 监控 panic 日志数量

3. **有没有 goroutine 泄露风险？**
   - 没有超时控制，需要业务保证
   - 建议使用带 ctx.Done() 的模式
