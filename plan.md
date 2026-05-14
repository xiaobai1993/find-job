# 面试复习计划

## 总体原则

- 每个专题产出笔记文件，放在对应目录下，方便回顾
- 以面试高频考点为纲，不追求面面俱到
- 结合项目实战经验回答，有血有肉比纯理论强

---

## 第一阶段：Go 语言（1-2 周）

### 1.1 Go 基础 → `01-go-basics/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 1 | slice | 底层结构（runtime slice）、扩容策略、与数组区别、踩坑（共享底层数组） | `01-slice.md` |
| 2 | map | 底层结构（hmap/bmap）、扩容流程（等量/增量）、遍历随机性、非并发安全 | `02-map.md` |
| 3 | string | 底层结构、byte 与 rune、字符串拼接优化 | `03-string.md` |
| 4 | interface | iface vs eface、类型断言、nil interface 陷阱、接口设计原则 | `04-interface.md` |
| 5 | 泛型 | 类型参数、约束、使用场景与限制 | `05-generics.md` |
| 6 | 错误处理 | error 接口、errors.Is/As、panic/recover、自定义错误类型 | `06-error.md` |
| 7 | defer | 延迟调用顺序、参数求值时机、return 与 defer 执行顺序 | `07-defer.md` |

### 1.2 Go 并发 → `02-go-concurrency/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 8 | goroutine | 与线程区别、栈大小、GMP 调度概览 | `01-goroutine.md` |
| 9 | channel | 底层结构（hchan）、有缓冲/无缓冲、关闭规则、select 随机性 | `02-channel.md` |
| 10 | sync 包 | Mutex/RWMutex（饥饿模式）、WaitGroup、Once、Pool、Map | `03-sync.md` |
| 11 | context | 取消传播、超时控制、值传递、最佳实践 | `04-context.md` |
| 12 | 并发模式 | fan-in/fan-out、pipeline、errgroup（项目 pkg/xerrgroup）、or-done | `05-patterns.md` |
| 13 | 数据竞争 | race detector、常见竞态场景、原子操作 atomic | `06-race-condition.md` |

### 1.3 Go 运行时 → `03-go-runtime/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 14 | GMP 调度 | G/M/P 模型、调度时机（系统调用/协作式抢占/基于信号的抢占）、work stealing | `01-gmp.md` |
| 15 | 内存分配 | tcmalloc 思想、mspan/mcache/mcentral/mheap、逃逸分析规则 | `02-memory.md` |
| 16 | GC | 三色标记、混合写屏障、GC 调优（GOGC）、与 Java GC 对比 | `03-gc.md` |
| 17 | pprof | CPU/heap/goroutine/block 采集、火焰图分析、项目调优案例 | `04-pprof.md` |

---

## 第二阶段：数据库 + 中间件（1-2 周）

### 2.1 MySQL → `04-database/mysql/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 18 | 索引 | B+ 树、聚簇/非聚簇索引、覆盖索引、最左匹配、索引失效场景 | `01-index.md` |
| 19 | 事务 | ACID、隔离级别、MVCC（ReadView/undo log）、快照读与当前读 | `02-transaction.md` |
| 20 | 锁 | 行锁/间隙锁/Next-Key Lock、死锁检测、在线DDL | `03-lock.md` |
| 21 | 架构 | 主从复制（binlog）、读写分离、分库分表、项目实践 | `04-architecture.md` |

### 2.2 Redis → `04-database/redis/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 22 | 数据结构 | 五大基础类型 + 底层编码（ziplist/quicklist/skiplist/hashtable）、Stream | `01-data-structure.md` |
| 23 | 持久化与高可用 | RDB/AOF、主从/哨兵/Cluster、脑裂处理 | `02-persistence-ha.md` |
| 24 | 缓存问题 | 穿透/击穿/雪崩方案、热 Key / 大 Key 处理 | `03-cache-problems.md` |
| 25 | 分布式锁 | SET NX + 过期 + Lua、Redlock 争议、项目 redis_lock 实现分析 | `04-distributed-lock.md` |

### 2.3 PostgreSQL → `04-database/postgresql/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 26 | PG 核心 | MVCC 实现（与 MySQL 差异）、JSONB、CTE/窗口函数 | `01-postgresql-core.md` |
| 27 | 选型对比 | PG vs MySQL 适用场景、为什么 billing/subsidy 选了 PG | `02-mysql-vs-pg.md` |

### 2.4 消息队列 → `05-middleware/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 28 | Pulsar | 架构（Broker + BookKeeper）、Subscription 模式、项目使用实践 | `pulsar/01-pulsar.md` |
| 29 | MQ 对比 | Pulsar vs Kafka vs SQS 对比、选型依据 | `pulsar/02-mq-comparison.md` |

---

## 第三阶段：分布式 + 微服务（1 周）

### 3.1 分布式系统 → `06-distributed-system/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 30 | 分布式事务 | 2PC/TCC/Saga/本地消息表/事务消息、支付场景选型 | `transaction/01-distributed-tx.md` |
| 31 | 分布式锁 | Redis/etcd/ZooKeeper 实现、项目 redis_lock 源码 | `distributed-lock/01-dlock.md` |
| 32 | 分布式 ID | Snowflake/号段/Leaf、项目 idmachine 实现 | `id-generation/01-id.md` |
| 33 | 限流熔断 | 四大限流算法、熔断器模式、支付场景应用 | `rate-limit/01-rate-limit.md`、`circuit-breaker/01-circuit-breaker.md` |

### 3.2 微服务 → `07-microservice/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 34 | gRPC + Protobuf | 编码、流式 RPC、拦截器、项目 proto 规范 | `grpc-protobuf/01-grpc.md` |
| 35 | 可观测性 | OpenTelemetry 三大信号、项目链路追踪实践 | `observability/01-observability.md` |

---

## 第四阶段：系统设计 + 项目经验（1-2 周）

### 4.1 系统设计 → `08-system-design/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 36 | 支付系统 | 整体架构、渠道对接（微信/支付宝）、状态机、退款流程 | `payment-system/01-payment-design.md` |
| 37 | 幂等性 | Token/唯一键/状态机幂等、重复支付/重复退款场景 | `idempotent/01-idempotent.md` |
| 38 | 对账系统 | 对账流程、差错处理、自动化方案 | `billing-reconciliation/01-reconciliation.md` |
| 39 | 订单系统 | 状态机、超时关单、订单拆分合并 | `order-system/01-order-design.md` |
| 40 | 高并发 | 缓存/异步/池化/批量、支付高峰策略 | `high-concurrency/01-high-concurrency.md` |

### 4.2 项目经验 → `11-project-experience/`

| 序号 | 主题 | 重点 | 产出 |
|------|------|------|------|
| 41 | 项目梳理 | 每个项目用 STAR 法则写 1-2 页：Situation/Task/Action/Result | `01-projects-star.md` |
| 42 | 技术难点 | 挑 3-5 个最有深度的问题和解决方案 | `02-technical-challenges.md` |
| 43 | 性能优化 | 实际优化案例（接口耗时/内存/GC/SQL） | `03-performance-optimization.md` |

---

## 第五阶段：补短板（按需）

| 优先级 | 内容 | 目录 |
|--------|------|------|
| 高 | DynamoDB 基础（如目标公司用 AWS） | `04-database/dynamodb/` |
| 中 | K8s 核心概念 + 部署实践 | `09-infra/kubernetes/` |
| 中 | Terraform + ArgoCD 工作流 | `09-infra/terraform/`、`09-infra/argocd/` |
| 中 | 算法刷题（每天 1-2 题，贯穿全程） | `10-algorithm/` |
| 低 | 服务发现/网关/服务网格 | `07-microservice/` 其余子目录 |

---

## 日常安排建议

| 时段 | 内容 |
|------|------|
| 上午 | 理论知识（Go 基础/并发/运行时/数据库） |
| 下午 | 系统设计 + 项目经验梳理（输出为主） |
| 晚间 | 算法 1-2 题 + 当天笔记回顾 |

算法建议从第一天就开始，每天保持手感，不要集中突击。
