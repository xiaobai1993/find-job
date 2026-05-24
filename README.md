的# 面试复习资料

Go 后端程序员 - 支付业务方向面试准备

## 目录结构

### Go 语言

| 目录 | 说明 |
|------|------|
| `01-go-basics/` | Go 语言基础：slice/map/string 底层结构、接口与类型系统、泛型、错误处理、defer/panic/recover 机制 |
| `02-go-concurrency/` | 并发编程：goroutine 调度、channel 原理、sync 包（Mutex/RWMutex/WaitGroup/Once/Pool）、context 用法与原理、errgroup、竞态条件与 data race 检测 |
| `03-go-runtime/` | 运行时原理：GMP 调度模型、GC 三色标记法与演进（v1.3→v1.5+）、内存分配器（tcmalloc 思想）、逃逸分析、栈增长、pprof 性能调优 |

### 数据库

| 目录 | 说明 |
|------|------|
| `04-database/mysql/` | InnoDB 存储引擎、B+ 树索引、事务隔离级别与 MVCC、行锁/间隙锁/Next-Key Lock、主从复制与读写分离、分库分表 |
| `04-database/postgresql/` | MVCC 实现差异（与 MySQL 对比）、JSONB 类型、CTE/窗口函数、分区表、与 MySQL 选型对比（项目 billing/subsidy/meal-point 使用 PG） |
| `04-database/redis/` | 核心数据结构与底层编码、持久化（RDB/AOF）、集群模式（主从/哨兵/Cluster）、缓存穿透/击穿/雪崩、分布式锁实现（Redlock）、Redis 7 新特性 |
| `04-database/dynamodb/` | AWS DynamoDB：分区键/排序键设计、读写容量模式、LSI/GSI、DAX 缓存、与关系型数据库的选型场景 |

### 中间件

| 目录 | 说明 |
|------|------|
| `05-middleware/pulsar/` | Pulsar 架构（Broker/BookKeeper/ZooKeeper）、Topic/Subscription 模式、消息确认与重试、与 Kafka 对比（项目主力 MQ） |
| `05-middleware/sqs/` | AWS SQS：标准队列 vs FIFO 队列、可见性超时、死信队列、长轮询、与 Pulsar/Kafka 的适用场景对比 |
| `05-middleware/kafka/` | Kafka 架构（Broker/Topic/Partition/Consumer Group）、消息顺序性、Exactly-Once 语义、高吞吐原理（零拷贝/页缓存）、面试高频对比项 |

### 分布式系统

| 目录 | 说明 |
|------|------|
| `06-distributed-system/consensus/` | 共识算法：Raft（选举/日志复制/安全性）、Paxos 原理、ZAB 协议、CAP/BASE 理论 |
| `06-distributed-system/distributed-lock/` | 分布式锁：Redis 实现（SET NX + 过期 + Lua 释放）、Redlock 争议、etcd/etcd 锁实现、项目 redis_lock 源码分析 |
| `06-distributed-system/id-generation/` | 分布式 ID：UUID、Snowflake、号段模式、Leaf、项目 idmachine 实现分析 |
| `06-distributed-system/transaction/` | 分布式事务：2PC/3PC、TCC、Saga、本地消息表、事务消息、最大努力通知、支付场景下的选型实践 |
| `06-distributed-system/circuit-breaker/` | 熔断降级：熔断器模式（Closed/Open/Half-Open）、滑动窗口统计、降级策略、hystrix/sentinel/resilience4go |
| `06-distributed-system/rate-limit/` | 限流算法：固定窗口/滑动窗口/令牌桶/漏桶、分布式限流、支付场景下的限流设计 |

### 微服务

| 目录 | 说明 |
|------|------|
| `07-microservice/grpc-protobuf/` | gRPC 通信：Protobuf 编码原理、流式 RPC、拦截器/中间件、grpc-gateway、服务间通信设计、项目 proto 规范实践 |
| `07-microservice/service-discovery/` | 服务注册与发现：客户端发现 vs 服务端发现、健康检查、负载均衡策略、Consul/etcd/Nacos |
| `07-microservice/api-gateway/` | API 网关：路由/鉴权/限流/熔断/日志、BFF 模式（项目 meican-pay-checkout-bff）、Gin 框架网关实现 |
| `07-microservice/service-mesh/` | 服务网格：Sidecar 模式、Istio 架构、数据面（Envoy）与控制面、与微服务框架的对比 |
| `07-microservice/observability/` | 可观测性：日志/指标/链路追踪三大支柱、OpenTelemetry 实践（项目已集成）、Prometheus + Grafana 告警体系 |

### 系统设计（核心业务）

| 目录 | 说明 |
|------|------|
| `08-system-design/payment-system/` | 支付系统：支付流程设计、渠道对接（微信/支付宝）、支付状态机、对账闭环、退款流程、风控设计 |
| `08-system-design/order-system/` | 订单系统：订单状态机、创建/支付/履约/取消流程、超时关单（延迟队列）、订单拆分与合并 |
| `08-system-design/billing-reconciliation/` | 对账系统：对账流程设计（渠道账单 vs 系统账单）、差错处理、长款/短款排查、自动化对账方案 |
| `08-system-design/high-concurrency/` | 高并发设计：缓存架构、异步化、池化技术、批量处理、秒杀场景、支付高峰应对策略 |
| `08-system-design/idempotent/` | 幂等性设计：幂等 Token 方案、数据库唯一约束、状态机幂等、支付场景下的幂等保障（重复支付/重复退款） |

### 基础设施

| 目录 | 说明 |
|------|------|
| `09-infra/docker/` | Docker：镜像分层原理、多阶段构建、Dockerfile 最佳实践、镜像优化 |
| `09-infra/kubernetes/` | Kubernetes：Pod/Service/Deployment/StatefulSet、资源调度、HPA/VPA、Service 与 Ingress、滚动更新策略 |
| `09-infra/terraform/` | Terraform：IaC 原理、状态管理、Module 设计、项目 prod/sandbox 多环境管理实践 |
| `09-infra/argocd/` | ArgoCD：GitOps 理念、Application 管理、Sync 策略、多集群部署、项目 CI/CD 流程 |
| `09-infra/aws/` | AWS：RDS/ElastiCache/DynamoDB/S3/Lambda/ECR/ELB、VPC 网络设计、IAM 权限、成本优化 |
| `09-infra/ci-cd/` | CI/CD：GitHub Actions/GitLab CI、流水线设计、制品管理、灰度发布策略 |

### 其他

| 目录 | 说明 |
|------|------|
| `10-algorithm/` | 算法题：LeetCode 高频题分类整理（数组/链表/树/动态规划/贪心/回溯）、时间复杂度分析 |
| `11-project-experience/` | 项目经验：STAR 法则梳理项目经历、技术难点与解决方案、性能优化案例、踩坑与复盘 |
| `12-interview-qa/` | 面试问答：按专题整理高频面试题与参考答案、模拟面试记录、HR 面与反问环节 |

## 复习优先级建议

1. **高优先级** — Go 语言基础 + 并发 + 运行时（01-03）、支付系统设计与幂等性（08）、分布式事务与锁（06）
2. **中优先级** — 数据库（04）、中间件（05）、微服务（07）、项目经验梳理（11）
3. **按需准备** — 基础设施（09）、算法（10）、通用面试题（12）
