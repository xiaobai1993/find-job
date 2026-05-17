# safe\_go 包 - 资深工程师面试指南

> 🎯 核心考点：并发安全、事务隔离、Context 陷阱、panic 处理最佳实践
>
> 这个包虽然只有 \~70 行代码，但藏了至少 3 个高级工程师常踩的坑，是面试区分度很高的考点。

***

## 一、这个包解决什么问题？

### 问题 1：在事务里开 goroutine 是致命的

```go
// ❌ 危险代码！90% 的高级工程师都踩过这个坑
xorm.Transaction(ctx, func(ctx context.Context) error {
    // 写订单
    CreateOrder(ctx, order)

    // 开 goroutine 发通知
    go func() {
        // 😱 这里还在用父事务的 tx！
        // 父 goroutine 可能已经 commit/rollback 了
        // tx 对象被释放、复用、状态混乱
        SendNotification(ctx, order)  // 💥 高并发下必出诡异的数据问题
    }()

    return nil
})
```

### 问题 2：goroutine panic 会把整个服务搞挂

```go
// ❌ 危险代码
go func() {
    // 这里 panic 了，没有 recover
    // 整个 Go 进程直接退出！
}()
```

### 问题 3：errgroup 里的 panic 不会被捕获

```go
// ❌ 危险代码
eg, ctx := errgroup.WithContext(ctx)
eg.Go(func() error {
    panic("这里 panic 了，errgroup 不会帮你 recover！💥")
})
```

***

## 二、核心代码分析

### 最关键的一行：剥离事务（第 24-36 行）

```go
func Go(ctx context.Context, fn func(ctx context.Context)) {
    go func() {
        // ✅ 如果父 ctx 在事务里，替换成全局普通 DB！
        if xorm.IsTXOpen(ctx) {
            ctx = xorm.WithRWDB(ctx)  // 剥离事务！
        }

        if xormv2.IsTXOpen(ctx) {
            ctx = xormv2.WithDB(ctx)  // 剥离事务！
        }

        WithRecover(ctx, fn)()
    }()
}
```

**这一行的价值：防止子 goroutine 复用父 goroutine 的已提交/回滚事务！**

***

## 三、历史 bug 复盘（面试重点，讲出这个加 20 分）

### Bug 1：ErrGroupFunc 不调用函数（commit c8f9614）

这个是第一版的低级错误，漏加 `()`，函数根本不执行，比较好发现。

***

### Bug 2：ErrGroupFunc 吞了 panic，errgroup 收不到错误（**commit 91bfe6e，这才是真正的大问题！**）

这个 bug 非常隐蔽，90% 的人看不出来：

**修复前的代码（你能看出问题吗？）：**

```go
// ❌ 有严重 bug 的代码！看起来没问题对吧？
func ErrGroupFunc(ctx context.Context, fn func(ctx context.Context) error) func() error {
    return func() (err error) {
        WithRecover(ctx, func(ctx context.Context) {
            err = fn(ctx)  // 😱 如果 fn 这里 panic 了呢？
        })()
        return err
    }
}

// WithRecover wraps the given function with a recover function.
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

**问题在哪？**

- 如果 `fn(ctx)` panic 了，recover 会接住
- 但是 `err = fn(ctx)` 这行根本没执行完！
- `err` 还是零值 `nil`！
- errgroup 收到 `nil`，以为执行成功了！
- **但实际上函数 panic 了，逻辑根本没跑！**

**最坑的后果：**

- log 里可能有一条 panic 日志（还可能被海量日志淹没）
- errgroup 认为一切正常，继续往下跑
- 业务逻辑没执行，但流程走完了
- 数据不一致，出了问题根本查不出来！

**修复方案：新增 WithRecover2，把 panic 转成 error 返回**

```go
var ErrPanic = errors.New("panic")

// ✅ 正确：panic 转成 error 返回！
func WithRecover2(ctx context.Context, fn func(ctx context.Context) error) func() error {
    return func() (err error) {
        defer func() {
            if e := recover(); e != nil {
                log.For(ctx).Error("recovered from panic", ...)
                err = fmt.Errorf("%w: %v", ErrPanic, e)  // 给 err 赋值！
            }
        }()
        return fn(ctx)
    }
}
```

**这样修复的好处：**

- errgroup 能正确收到错误
- 可以用 `errors.Is(err, safego.ErrPanic)` 判断是不是 panic 导致的
- 业务可以根据错误类型做不同处理

> 💡 面试超级加分点：这个 bug 特别能体现资深工程师的功力。90% 的人写 recover 都会忘了把错误传出去，结果就是「假成功」，这种问题在线上比直接崩溃还可怕，因为它悄无声息地把数据搞坏了。

***

### Bug 3：事务没有剥离（commit 20e1e2d）

**修复前：**

- 开 goroutine 时，context 原封不动传进去
- 子 goroutine 还在用父事务的 tx

**后果：**

- 父事务已经 commit 了，子 goroutine 还在拿这个 tx 写数据
- MySQL 连接已经被放回连接池了，被其他请求复用
- 会出现：
  1. 数据写到别的事务里
  2. 连接状态混乱，SQL 执行报错
  3. 最坑的是：低并发下没问题，高并发才出现，极难复现

> 💡 面试加分点：这个 bug 是典型的「 works on my machine 」类型，本地测没问题，线上高并发才炸。

***

## 四、面试灵魂拷问（资深工程师必问题）

### Q1：为什么不在开 goroutine 的时候直接把 ctx 里的 DB 去掉，而是替换成全局 DB？

**标准答案：**

> "因为业务代码里到处都是 `MustGetDB(ctx)` 这种写法，如果直接去掉，业务代码会直接 panic。
>
> 替换成全局 DB 的话，业务代码可以正常运行，只是不再在事务里而已，这是符合预期的。
>
> 毕竟 goroutine 里的逻辑本来就不应该跟父事务共享事务边界，它有自己的生命周期。"

***

### Q2：为什么只剥离事务，不剥离 context 里的其他值？比如 trace\_id、log 字段？

**标准答案：**

> "trace 信息和 log 字段是安全的，可以跨 goroutine 共享，链路追踪本来就需要父子 goroutine 有相同的 trace\_id。
>
> 只有事务 tx 对象是有状态的、非线程安全的、有生命周期的，所以只需要剥离这一个。
>
> 这是最小影响原则：只改需要改的，其他保持原样。"

***

### Q3：recover 之后为什么只打 log 不 panic 回去？会不会把错误吞了？

**标准答案：**

> "这个是设计取舍。
>
> 第一，goroutine 里的 panic 如果没人接，会把整个进程搞挂，这是不可接受的。支付服务是核心服务，挂了影响太大。
>
> 第二，吞错误确实有问题，所以提供了 `WithRecover2` 版本，会把 panic 转成 `ErrPanic` error 返回，给业务自己处理。
>
> 第三，日志是必打的，而且有 stacktrace，可以监控告警，不会丢问题。
>
> 总结：默认保活，可选返回错误，给业务选择权。"

***

### Q4：这个实现有什么问题吗？如果是你，会怎么改进？

**加分回答：**

> "我觉得有几个可以改进的地方：
>
> 1. **应该加个告警钩子**：recover 之后不仅打 log，还应该上报到监控系统，比如 Prometheus counter +1，这样可以主动发现问题。
> 2. **应该支持配置是否剥离事务**：有些极端场景用户可能真的想在子 goroutine 里用事务（虽然不推荐，但应该给选项）。
> 3. **应该检测 goroutine 泄漏**：可以加个超时配置，如果 goroutine 跑太久就打 warn log。
> 4. **应该支持 panic 回调**：让业务可以注册自己的 panic 处理逻辑，比如发钉钉告警。"

***

### Q5：如果我在子 goroutine 里再开一层事务呢？会有什么问题？

**标准答案：**

> "没问题，这是正确的做法。
>
> 子 goroutine 剥离了父事务后，拿到的是普通 DB，自己再开 Transaction 就是全新的事务，有自己的生命周期，完全没问题。
>
> 实际上，这才是正确的做法：每个 goroutine 管理自己的事务边界，不要跨 goroutine 共享事务。"

***

## 五、对比：不同方案的优劣

| 方案               | 安全性  | 业务侵入性      | 线上表现        |
| ---------------- | ---- | ---------- | ----------- |
| 直接传 ctx，不剥离      | ❌ 极低 | 0 侵入       | 高并发下必炸，极难调试 |
| 强制业务传新 ctx       | ✅ 高  | 极高，每个地方都要改 | 安全，但容易忘     |
| Go 函数里自动剥离（当前方案） | ✅ 高  | 0 侵入，业务无感  | 安全，符合预期     |

***

## 六、面试标准回答模板

### 面试官问：你们 Go 里开 goroutine 有什么规范吗？

> "我们封装了一个 safego.Go 工具函数，主要解决两个问题：
>
> 第一是自动 panic recover，避免子 goroutine panic 把整个服务搞挂，recover 之后会打带 stacktrace 的 error log，我们有监控告警。
>
> 第二是自动剥离 context 里的数据库事务，因为父 goroutine 的事务可能已经提交或回滚了，子 goroutine 还在用的话会出非常诡异的并发问题，比如连接状态混乱、数据写到别的事务里。我们踩过这个坑，高并发下才出现，非常难调试。
>
> 另外还提供了 errgroup 兼容的版本，会把 panic 转成 error 返回，不会让 errgroup 直接挂掉。
>
> 这个工具我们已经用了好几年，线上避免了无数次服务宕机和数据一致性问题。"

***

## 七、一句话总结

> **跨 goroutine 共享事务 = 给自己埋雷**
>
> safego 这个包的本质就是：用 70 行代码，把一个资深工程师都常踩的坑，变成了团队所有人都不会踩的坑。

***

## 📝 面试要点复盘（考前看一眼）

1. ✅ 讲出「子 goroutine 复用父事务」这个坑
2. ✅ 讲出 3 个历史 bug：
   - ErrGroupFunc 漏加 `()` 不执行（低级）
   - **ErrGroupFunc 吞 panic 返回 nil（高级重点！）**
   - 事务没剥离导致连接状态混乱
3. ✅ 讲出设计取舍：保活 vs 吞错误，最小影响原则
4. ✅ 能说出 2-3 个改进点

