# Meican-Pay PKG 学习路线图

## 第一阶段：核心架构（2 天）

### Day 1: 事务消息 + xorm 事务封装
- [x] 阅读 mq/producer.go 理解 Producer 接口设计
- [x] 阅读 mq/txmsg/dao.go 理解消息表结构
- [x] 阅读 xorm/tx.go 理解事务回调机制
- [x] 理解 afterCommit / afterSavePoint 的区别
- [ ] 画一张事务消息完整流程图

### Day 2: 分布式锁 + 安全 goroutine
- [x] 阅读 redis_lock/mutex.go 理解加锁/解锁流程
- [x] 理解自动续期（watch / extend）机制
- [x] 阅读 safe_go/safe_go.go 理解事务上下文剥离
- [ ] 思考：Redlock 算法在支付场景的必要性

## 第二阶段：辅助模块（1 天）

### Day 3: 错误处理 + 可观测性
- [x] 阅读 xerrors/xerrors.go 理解栈包装机制
- [x] 阅读 xotel/otlp.go 理解链路追踪
- [ ] 阅读 metrics/ 目录理解指标设计
- [ ] 思考：支付系统需要哪些核心 metrics

## 第三阶段：实战演练（2 天）

### Day 4: 实现一个简化版事务消息
- [ ] 写一个最小可用的本地消息表
- [ ] 实现 GORM 的 AfterCommit 回调
- [ ] 写一个简单的后台补偿 goroutine

### Day 5: 边界场景测试
- [ ] 事务提交后 MQ 挂了的场景
- [ ] 嵌套事务场景
- [ ] goroutine 中误用事务的场景

---

## 面试高频问题

### 事务消息相关
1. **为什么需要事务消息？不用有什么问题？**
   - 数据库事务和消息发送原子性问题
   - 先发消息后落库：消息发了事务回滚
   - 先落库后发消息：落库成功消息发送失败

2. **事务消息有哪几种实现方案？**
   - 本地消息表（meican-pay 方案）
   - RocketMQ 事务消息（Half Message + Commit/Rollback）
   - 事件溯源（Event Sourcing）

3. **本地消息表的优缺点？**
   - ✅ 简单可靠，不依赖 MQ 特性
   - ✅ 数据库事务保证原子性
   - ❌ 每个服务维护自己的消息表
   - ❌ 消息顺序性难保证

### 分布式锁相关
4. **Redis 分布式锁如何实现？有什么问题？**
   - SET key value NX EX expire_time
   - 问题：主从切换丢锁、时钟漂移、锁过期续期

5. **什么是 Redlock？真的安全吗？**
   - N 个独立 Redis 实例，超过半数加锁成功
   - 争议：Martin Kleppmann vs antirez 的论战
   - 支付场景：单实例 + 主从 + 数据库兜底 足够了

6. **锁的粒度如何选择？**
   - 太粗：并发低
   - 太细：死锁风险（按固定顺序加锁）
   - 支付：通常锁订单 ID 或用户 ID

### 并发安全相关
7. **goroutine 中为什么要剥离事务？**
   - 事务提交后 DB 连接归还连接池
   - goroutine 继续使用会导致不可预期行为
   - safe_go.Go 自动替换为全局 DB

8. **如何保证错误栈不丢失？**
   - 错误产生的最内层 Wrap
   - 上层只判断，不重复 Wrap
   - Wrap 时检查是否已经是 *withStack 类型

---

## 对比其他支付系统

### 美团支付 vs 支付宝
- 支付宝: 自研消息队列 + 事务消息
- 美团: 本地消息表 + Pulsar/SQS
- 共同点: 最终一致性 + 补偿机制

### 设计哲学
- 不信任任何中间件 100% 可靠
- 所有操作都要有补偿
- 数据库是最终的真理来源
