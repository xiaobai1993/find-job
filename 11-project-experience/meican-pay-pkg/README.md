# Meican-Pay PKG 学习笔记

## 项目概述

这是美团餐的支付项目核心公共库，包含了支付系统中常用的基础组件。

## 目录结构

```
pkg/
├── cover_matrix/    # 覆盖矩阵（覆盖率相关）
├── grpc/            # gRPC 相关工具和中间件
├── idmachine/       # ID 生成器（雪花算法/分布式ID）
├── metrics/         # 指标监控
├── mq/              # 消息队列（核心！事务消息）
│   ├── bizproducer/ # 业务生产者
│   ├── pulsar/      # Pulsar 实现
│   ├── sqs/         # SQS 实现
│   └── txmsg/       # 事务消息（核心！）
├── redis_lock/      # Redis 分布式锁
├── safe_go/         # 安全 goroutine（带 panic 恢复）
├── stacktrace/      # 栈追踪
├── timecost/        # 耗时统计
├── utils/           # 工具函数
├── xerrors/         # 错误包装（带栈追踪）
├── xorm/            # ORM 封装（事务、回调）
├── xotel/           # OpenTelemetry 封装
└── xtime/           # 时间工具
```

## 核心设计思想

### 1. 事务消息 (Transactional Messaging)
- **问题**: 数据库事务和消息发送的原子性问题
- **解决方案**: 本地消息表 + 异步补偿
- **核心流程**:
  ```
  1. 消息先写入数据库（本地消息表）
  2. 事务提交后回调发送消息到 MQ
  3. 后台 Daemon 轮询补偿发送失败的消息
  ```

### 2. 分布式锁
- 基于 redsync 库实现 Redis 分布式锁
- 支持自动续期（autoRenewal）
- 支持 Redlock 算法（多 Redis 实例）
- 支持批量锁

### 3. 安全 goroutine
- 自动 panic 恢复
- 自动记录错误日志
- 事务上下文自动剥离（避免 goroutine 中误用事务）

### 4. 错误处理
- 错误链 + 栈追踪
- 避免重复包装栈

## 学习重点

| 模块 | 重要性 | 学习建议 |
|------|--------|----------|
| mq/txmsg | ⭐⭐⭐⭐⭐ | 支付系统核心，理解事务消息设计 |
| redis_lock | ⭐⭐⭐⭐⭐ | 分布式锁，理解续期、Redlock |
| xorm | ⭐⭐⭐⭐ | 事务封装、回调机制 |
| safe_go | ⭐⭐⭐ | goroutine 安全实践 |
| xerrors | ⭐⭐⭐ | 错误处理最佳实践 |

---

## 我的疑问

### Q1: txmsg 事务消息的状态机是怎样的？
- New -> Queued -> Finish
- New -> Queued -> Error -> Retry -> Finish

### Q2: 为什么需要 afterSavePoint 和 afterCommit 两个回调？
- afterSavePoint: 嵌套事务 savepoint 成功时回调（子事务成功）
- afterCommit: 最外层事务提交成功时回调

### Q3: Redlock 算法在支付场景的可靠性？
- 生产环境使用多 Redis 实例
- 自动续期避免锁过期
- FIXME: 加锁未超过半数时需要释放已加的锁
