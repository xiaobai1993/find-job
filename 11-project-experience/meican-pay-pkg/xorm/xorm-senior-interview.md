# XORM 资深 Go 工程师面试题

> 面向资深 Go 工程师，考察深度：架构设计、边界处理、性能权衡、工程实践

---

## 🔴 基础深度题

### Q1：为什么要把 DB 对象放在 context 里，而不是作为参数传递？有什么优缺点？

**参考答案：**

优点：
1. **代码清爽**：业务函数不用到处传 `db *gorm.DB`，方法签名干净
2. **透明切换**：普通 DB 和事务 TX 实现同一个接口，业务代码无感
3. **强制规范**：避免有人在事务外偷偷用全局 DB

缺点：
1. **类型不安全**：context.Value 返回 interface{}，运行时才 panic
2. **调试困难**：出问题时不知道 context 里到底放了啥
3. **goroutine 安全陷阱**：开 goroutine 时如果还复用这个 context，会导致并发使用同一个 tx

> 资深点可以补充：这是典型的「工程便利性 vs 类型安全」的 trade-off。美餐选择了便利性，因为支付业务迭代快，规范比完美更重要。

---

### Q2：这个封装非 goroutine 安全，注释里写了必须用 safego.Go，为什么？会出什么问题？

**参考答案：**

```go
// 危险代码
xorm.Transaction(ctx, func(ctx context.Context) error {
    go func() {
        // 子 goroutine 还在用同一个 tx！
        xorm.MustGetRWDB(ctx).Create(...)  // ❌ 并发访问同一个 tx
    }()
})
```

问题：
1. GORM 的 TX 对象不是并发安全的，内部状态会乱
2. 父 goroutine 可能已经 Commit/Rollback 了，子 goroutine 还在写
3. 最坑的是：平时没问题，高并发下才出诡异的数据错乱

> 资深点可以补充：safego.Go 应该是内部封装，会自动把 context 里的 tx 剥离，换成普通 DB。这个坑很多团队踩过。

---

## 🟠 架构设计题

### Q3：为什么 afterCommit 回调是顺序执行的，不是并发的？如果回调很慢怎么办？

**参考答案：**

为什么顺序执行：
1. **确定性**：回调之间可能有依赖，顺序执行行为可预测
2. **简单**：并发要处理 error、超时、panic，复杂度飙升
3. **大部分回调很快**：发 MQ、清缓存都是毫秒级

慢了怎么办？
1. **业务自己异步**：回调里开 goroutine 发，不要阻塞 Commit
2. **后台 Daemon 兜底**：回调只是尽力而为，本地消息表 + Daemon 才是可靠保障
3. **超时控制**：每个回调加超时时间

> 资深点可以补充：这又是一个 trade-off。框架选择了简单可靠，把并发的责任交给业务。毕竟事务提交这个路径上，越简单越不容易出 bug。

---

### Q4：为什么 Put/Get 的 key 用 interface{} 不用 string？v2 又改成 string 了，怎么看这个变化？

**参考答案：**

v1 用 interface{} 的原因：
- 避免 key 命名冲突，用包级别的私有变量当 key
  ```go
  var txMessageKey = &struct{}{}  // 只有这个包能拿到
  tx.Put(txMessageKey, messages)
  ```
- string 的话很容易撞 key："msg"、"message"、"tx_msg"

v2 改成 string 的原因：
- GORM 的 InstanceSet 只支持 string
- 用前缀规范避免冲突：`txmsg_`、`cache_`
- 便利性大于安全性，撞 key 的概率其实很低

> 资深点可以补充：这是典型的「理论正确 vs 工程实用」的选择。interface{} 理论上更安全，但实际用起来麻烦，还不能利用 GORM 内置能力。v2 的改动说明团队在向实用主义倾斜。

---

## 🟡 边界情况题

### Q5：如果 afterCommit 回调里 panic 了怎么办？事务已经提交了啊！

**参考答案：**

```go
// 现在的代码有问题！
func (t *tx) callAfterCommitCallbacks(ctx context.Context) {
    for _, callback := range t.afterCommitCallbacks {
        callback.Do(ctx, t)  // ❌ 这里 panic 了怎么办？
    }
}
```

正确做法应该 recover：
```go
func (t *tx) callAfterCommitCallbacks(ctx context.Context) {
    for _, callback := range t.afterCommitCallbacks {
        func() {
            defer func() {
                if r := recover(); r != nil {
                    log.Error("callback panic", r)
                }
            }()
            callback.Do(ctx, t)
        }()
    }
}
```

关键原则：
- **回调 panic 不能影响事务结果**
- 事务提交成功就是成功了，回调失败是另外一回事
- 靠 Daemon 兜底重试，不能因为回调失败把事务搞挂了

> 资深点可以补充：很多人写回调忘了 recover，结果回调 panic 导致整个请求 500，但数据其实已经落库了，前端显示失败但用户钱已经扣了，这是支付系统的 P0 级 bug。

---

### Q6：嵌套事务最多能嵌套多少层？太深了有什么问题？

**参考答案：**

技术限制：
- MySQL 本身没限制 SavePoint 数量
- 但每个 SavePoint 都会占事务日志空间
- 太深了 tx.parent 链太长，遍历会慢

工程建议：
- **不要超过 3 层**
- 超过 3 层说明代码分层有问题
- 真实业务里 2 层就够了：Service → DAO

问题：
- 调试困难，出问题不知道哪层回滚的
- 回调链太长，容易出循环依赖
- SavePoint 名字用函数指针，嵌套太深可能重名？（概率极低）

---

## 🟢 性能权衡题

### Q7：每个事务都创建一个新的 tx 对象，还要塞 context 里，有性能开销吗？值得吗？

**参考答案：**

开销分析：
- tx 对象本身很小：几个指针 + 一个 slice + 一个 map
- context.WithValue 是不可变对象，创建开销极低
- 一次 GC 压力可以忽略

对比收益：
- 工程便利性提升巨大
- 避免了大量手工传 DB 的重复代码
- 减少了「在事务外用了错的 DB」的 bug

结论：
- **完全值得**，这点性能开销可以忽略不计
- 支付系统首先要的是正确性和可维护性，性能是其次
- 真要优化也应该优化 SQL，而不是这个

---

### Q8：为什么 SavePoint 的名字用函数指针 `fmt.Sprintf("sp%p", fc)`，不用自增 ID 或者 UUID？

**参考答案：**

```go
savePointName = fmt.Sprintf("sp%p", fc)  // 为什么这么写？
```

好处：
1. **天然唯一**：同一个函数递归调用，指针也不一样
2. **不需要维护全局计数器**：无状态，不用加锁
3. **调试友好**：出问题可以看到是哪个函数的 SavePoint
4. **零成本**：指针转字符串开销极低

缺点：
- 可读性一般，人眼看不出来是啥
- 理论上有碰撞概率，但实践中不可能

> 资深点可以补充：这是非常巧妙的设计，用函数指针的唯一性做天然 ID，不需要任何额外状态管理。很多人想不出来还能这么玩。

---

## 🔵 对比延伸题

### Q9：如果让你重新设计这个封装，你会做哪些改进？

**参考答案：**

我会加这些东西：
1. **回调 recover + 超时**：现在的实现没有 recover，太危险
2. **回调失败告警**：回调失败了要打 metrics + 告警
3. **goroutine 安全检测**：检测到在子 goroutine 用 tx 直接 panic
4. **事务超时**：事务超过一定时间自动回滚，避免长事务
5. **嵌套层级检测**：超过 3 层打 warn log
6. **可观测性**：事务时长、回调时长、回滚次数都打 metrics

但我不会加这些：
- ❌ 回调并发执行：复杂度太高，没必要
- ❌ 分布式事务支持：超出这个封装的边界
- ❌ 更多传播级别：REQUIRES_NEW 之类的，太复杂用不上

---

### Q10：这个设计和 Spring 的声明式事务有什么异同？

**参考答案：**

相同点：
- 都解决了「代码不用到处判断是不是在事务里」的问题
- 都有事务传播机制
- 都支持回调/钩子

不同点：
- Spring 是 AOP 实现，自动代理，Go 是手动封装
- Spring 有 7 种传播级别，这个只有 1 种（REQUIRED）
- Spring 的回调是注解式的，这个是注册式的
- Spring 是框架级别的，这个是库级别的，更轻量

> 资深点可以补充：本质上都是「透明事务」思想，只是实现手段不同。Go 没有 AOP，只能用函数式编程 + context 来实现类似的效果。

---

## 🟣 实战场景题

### Q11：支付场景下，回调里发 MQ 失败了怎么办？设计一个可靠的方案。

**参考答案：**

三保险方案：

```
1. afterCommit 尽力发（实时性）
   ↓
2. 本地消息表状态（可靠性）
   ↓
3. Daemon 轮询重试（兜底）
   ↓
4. 超过 N 次失败告警（人工介入）
```

关键点：
- afterCommit 只是优化延迟，不是可靠保证
- 真正的可靠性来自本地消息表 + 兜底任务
- 一定要有幂等，因为消息可能重复发

---

### Q12：怎么在这个封装基础上实现「只读事务」？

**参考答案：**

方案一：context 标记
```go
ctx = WithReadOnly(ctx)
xorm.Transaction(ctx, func(ctx context.Context) error {
    // 里面检测到只读，遇到写操作直接 panic
})
```

方案二：DB 接口方法
```go
type DB interface {
    ReadOnlyTransaction(ctx context.Context, fc func(ctx context.Context) error) error
}
```

实现关键点：
- 只读事务可以自动走从库
- 检测到写操作直接拒绝，提前发现问题
- 可以加事务级别优化：SET TRANSACTION READ ONLY

---

## ⚫ 灵魂拷问题

### Q13：这个封装看起来挺好用，为什么其他团队很少这么做？你觉得这个设计的最大隐患是什么？

**参考答案：**

最大隐患：**透明得有点过头了，容易被滥用**

```go
// 有人会这么写，完全不知道自己在事务里
func SomeService(ctx context.Context) error {
    // 这里已经在事务里了，但写代码的人不知道
    // 然后在里面调了一个 RPC，超时 10 秒
    // 结果长事务占着连接，把数据库连接池打满了
}
```

其他团队不这么做的原因：
1. 很多团队没有这么复杂的嵌套事务需求
2. 有人觉得 context 放 DB 是 anti-pattern
3. 对类型安全有执念，宁愿到处传 DB

> 资深点可以补充：这是典型的「团队文化决定架构设计」。美餐支付业务迭代快，团队熟，信任度高，所以可以接受这种看起来有点「magic」的设计。如果是大团队多人协作，可能还是宁愿啰嗦一点，显式传 DB。

---

## 📋 面试回答加分项

回答任何问题时，提到这些点自动加分：

| 加分点 | 体现的能力 |
|-------|-----------|
| 提到 trade-off / 权衡 | 架构思维，不是非黑即白 |
| 提到业务场景 | 不是空想，能结合实际 |
| 提到边界情况 / 异常处理 | 考虑周全，有实战经验 |
| 提到可观测性 / metrics / 告警 | 生产环境思维，不是玩具代码 |
| 提到不同方案的适用场景 | 不钻牛角尖，灵活变通 |
| 提到历史演进原因（v1 vs v2） | 能理解设计的演进过程 |
