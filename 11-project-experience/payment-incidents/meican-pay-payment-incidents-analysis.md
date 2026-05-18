# 美餐支付系统事故分析报告

> 📊 收集了 2023-2025 年 7 起支付相关事故，进行深度分析和总结
> 🔍 覆盖：消息系统、ID生成、基础设施、数据库、K8s等多个维度

---

## 一、事故总览

| 日期 | 事故名称 | 级别 | 持续时间 | 影响范围 | 根因分类 |
|-----|---------|-----|---------|---------|---------|
| 2023-04-21 | Pulsar backlog堆积导致支付退款失败 | P1 | 100分钟 | 部分用户支付退款 | 消息系统 |
| 2023-03-04 | Pulsar broker pending requests堆积导致超时 | P1 | 39分钟 | payment服务及依赖链路 | 消息系统 |
| 2024-01-10 | Snowflake ID重复 | P4 | 35小时 | 跨服务数据关联错误 | ID生成 |
| 2024-04-28 | 新支付无法从Pulsar获取消息 | P1 | 109分钟 | meican-pay/payment | 消息系统 |
| 2024-11-18 | generator panic导致餐补支付失败 | P1/P2 | 36分钟 | 新支付餐补(2.5万单) | 基础设施-K8s |
| 2025-05-12 | AWS Spot Rebalance导致支付失败 | P1 | 30分钟 | 微信/支付宝支付(5286报错) | 基础设施-云厂商 |
| 2025-06-05 | CDC初始快照导致数据库CPU100% | P1 | 5分钟 | POS下单(358单退款) | 数据库 |

---

## 二、分类深度分析

### 🔴 消息系统问题（3起 P1）

#### 事故共同点
| 问题 | 出现次数 | 说明 |
|-----|---------|------|
| Pulsar broker异常 | 2次 | pending requests堆积、netty堆外内存泄漏 |
| Subscription未清理导致backlog | 1次 | dapi/bi不再消费但未删subscription |
| 发送消息超时阻塞业务 | 3次 | 消息发送同步阻塞主流程 |

#### 技术深度分析

**1. 为什么Pulsar出问题会直接打挂支付？**

```go
// 支付主流程中同步发送消息 → 这是致命设计问题
func handlePayment() {
    // ... 支付核心逻辑 ...

    // ❌ 同步发送消息，超时直接阻塞整个支付流程
    err := pulsarProducer.Send(ctx, message)
    if err != nil {
        // 消息发送失败 → 整个支付失败
        return err
    }
}
```

**问题本质：**
- 消息发送是**辅助操作**，不应阻塞**核心支付流程**
- 消息系统可用性 < 支付系统可用性要求
- 耦合设计导致消息系统故障直接传导到支付核心

#### 改进措施 ✅

**短期（紧急）：**
```go
// ✅ 异步发送，失败不影响主流程
go func() {
    err := pulsarProducer.Send(ctx, message)
    if err != nil {
        // 记录日志 + 本地重试队列
        log.Error("send failed", err)
        retryQueue.Add(message)
    }
}()

// ✅ 超时时间严格控制
ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
defer cancel()
```

**中期（架构）：**
1. **消息发送降级开关**：Pulsar故障时自动切到本地队列 + 后台同步
2. **Circuit Breaker**：失败率超过阈值时熔断，直接返回成功
3. **多消息集群冗余**：同时写两个独立Pulsar集群，一个失败切另一个

**长期（最佳实践）：**
- Outbox Pattern：先写数据库outbox表，后台异步投递消息
- 消息系统只是"尽力而为"，核心状态不依赖消息是否发送成功

---

### 🟡 ID生成问题（1起 P4）

#### 问题回顾
- **原因**：ECS升级v1.4.0注入agent，导致每个pod有2个IP
- **代码bug**：generator默认取第一个IP作为workerID，所有pod第一个IP相同
- **结果**：不同节点生成相同的snowflake ID

#### 技术深度分析

```go
// ❌ 有问题的IP获取逻辑
func getWorkerID() int64 {
    addrs, _ := net.InterfaceAddrs()
    // 直接取第一个IP → agent注入后第一个IP都是相同的！
    firstIP := addrs[0].String()
    return hashIP(firstIP) % 1024
}
```

**问题本质：**
- 对基础设施的假设（IP唯一）被打破
- ID生成是核心基础设施，但容错设计不足
- 依赖环境变量而非主动分配workerID

#### 改进措施 ✅

**1. workerID分配方式升级：**
```go
// ✅ 方式1：中心化分配（Redis/ETCD）
func getWorkerIDFromCenter() int64 {
    // 启动时从注册中心申请唯一ID
    id, _ := redis.Incr("snowflake:worker:id")
    return id % 1024
}

// ✅ 方式2：Pod Name哈希（K8s环境Pod Name唯一）
func getWorkerIDFromPodName() int64 {
    podName := os.Getenv("POD_NAME")
    return hash(podName) % 1024
}

// ✅ 方式3：启动参数指定（Deployment配置不同ID）
```

**2. 运行时检测：**
```go
// 启动时检测是否与其他节点workerID冲突
func checkWorkerIDConflict() {
    myID := getWorkerID()
    // 注册到Redis，带过期时间
    ok, _ := redis.SetNX("snowflake:worker:"+myID, podIP, 5*time.Minute).Result()
    if !ok {
        panic("workerID conflict!")
    }
}
```

**3. 全局监控：**
- 实时监控ID重复率（写入唯一索引报错）
- 各服务workerID分布看板

---

### 🟠 基础设施问题（2起 P1）

#### 1. K8s Node回收问题（2024-11-18）
```
问题：
  - Spot实例Rebalance → Node被标记Unschedulable
  - Pod驱逐不成功 → 变成"幽灵Pod"
  - 流量继续路由到死Pod → 85%请求panic

影响：
  - 1万次panic
  - 2.5万单餐补支付失败
```

**核心问题：**
- generator作为核心基础服务，只有2个副本
- 其中一个副本出问题 → 85%流量打到死Pod
- K8s存活探测失败但没有自动摘除流量

**改进：**
- ✅ 核心服务副本数 ≥ 3
- ✅ 配置优雅的Pod Termination流程
- ✅ Readiness Probe检测实际服务可用性
- ✅ 核心服务避免Spot实例

#### 2. AWS Spot大规模Rebalance（2025-05-12）
```
问题：
  - AWS发送大量Rebalance信号
  - 所有app1机器同时变成不可调度
  - ASG降到0后Cluster Autoscaler不再扩容
  - userrelations等核心服务无节点可调度

影响：
  - 5286次报错
  - 微信/支付宝/公众号/小程序全链路支付失败
```

**改进措施已实施：**
1. ✅ 核心服务避免使用Spot
2. ✅ 核心服务加Annotation：`cluster-autoscaler.kubernetes.io/safe-to-evict: "false"`
3. ✅ Spot策略从Lowest Price改为Balanced

**补充建议：**
- 跨可用区部署，避免单AZ故障
- PDB（Pod Disruption Budget）配置：`minAvailable: 50%`
- 多集群灾备：一个集群挂了切另一个

---

### 🔴 数据库问题（1起 P1）

#### CDC初始快照锁竞争
```
问题：
  - Debezium CDC初始快照阶段SELECT全表历史数据
  - 触发意外的锁竞争
  - CPU飙升到100%，所有查询/写入超时

影响：
  - POS下单全面失败
  - 人工拦截退款158单，自动退款200单
```

**核心问题：**
- 对核心业务库做CDC，没有评估影响
- 初始Snapshot在业务高峰期执行
- 没有提前和业务方沟通

**改进：**
1. ✅ 大表CDC不要做初始Snapshot，或用其他方式
2. ✅ Snapshot在业务低峰期执行，随时准备中止
3. ✅ 数据库变更前必须和业务团队沟通
4. ✅ CDC用只读副本做，不要直接连主库

---

## 三、跨事故共性问题

### 🚨 设计层面：耦合过重

| 问题 | 出现事故 | 严重程度 |
|-----|---------|---------|
| 消息发送同步阻塞主流程 | 3起P1 | 🔴 最高 |
| 核心基础服务（generator）单点依赖 | 2起P1 | 🔴 最高 |
| 支付全链路依赖过多中间服务 | 2起P1 | 🟠 高 |

### 🚨 运维层面：变更风险

| 问题 | 出现事故 | 严重程度 |
|-----|---------|---------|
| 基础设施变更无业务感知（ECS升级、K8s升级） | 2起 | 🟠 高 |
| 数据库操作前无业务沟通 | 1起 | 🟡 中 |
| 核心服务用Spot实例 | 1起 | 🟠 高 |

### 🚨 监控层面：发现滞后

| 问题 | 说明 |
|-----|------|
| 依赖服务异常监控缺失 | generator挂了3小时才发现 |
| Pulsar backlog报警缺失 | backlog 436K才发现 |
| 业务指标监控不够实时 | 依赖人工反馈才知道支付失败 |

---

## 四、系统性改进建议

### 🏗️ 架构层面

**1. 支付核心「最小依赖原则」**
```
目标：支付流程只依赖2个核心组件
  - 数据库（必须）
  - Redis（分布式锁，可选）

其他所有依赖都要异步化、可降级：
  ✅ 消息发送 → 异步 + 本地重试队列
  ✅ ID生成 → 本地 fallback 生成器
  ✅ 外部服务调用 → 超时 + 熔断 + 降级
```

**2. Chaos Engineering常态化**
- 定期注入Pulsar故障，验证支付是否正常
- 定期Kill generator实例，验证容错
- 定期模拟数据库CPU飙升场景

### 🔧 运维层面

**1. 核心服务「金身原则」**
```
✅ 不用Spot实例
✅ 副本数 ≥ 3，跨可用区
✅ PDB配置，避免批量驱逐
✅ safe-to-evict: false
✅ 独立Node Group，不和非核心服务混部
```

**2. 变更「三级审批制」**
- 一级：核心库操作 → DBA + 业务负责人 + 运维三方审批
- 二级：基础设施工况变更 → 运维 + 业务确认
- 三级：业务侧变更 → 业务负责人审批

### 📊 监控层面

**1. 业务黄金指标实时监控**
```
支付成功率
  ↓ 低于99.5%自动告警
  ↓ 低于99%自动触发降级预案
  ↓ 低于95%自动熔断非核心依赖
```

**2. 依赖服务健康度看板**
- generator: P99延迟、错误率、副本数
- Pulsar: backlog、生产/消费延迟、broker状态
- 数据库: CPU、活跃连接、锁等待

---

## 五、面试必背要点

### 面试官问：你们支付系统遇到过什么大事故？怎么改进的？

> **标准答案：**
>
> "我们在两年内遇到了大概7起支付相关的P1事故，主要分四类：
>
> **第一类是消息系统问题，有3起。** 核心原因是Pulsar发送是同步的，消息系统挂了直接打挂支付。改进是：
> - 消息发送全异步化，失败不阻塞主流程
> - 加了熔断和降级，Pulsar故障时切本地队列
> - 后来引入了Outbox模式，先写库再异步发消息
>
> **第二类是ID生成问题，有1起。** ECS升级注入agent导致所有pod第一个IP相同，snowflake workerID冲突。改进是：
> - workerID不再依赖IP，改成用Pod Name哈希或者中心化分配
> - 启动时检测workerID冲突，有冲突直接panic不让启动
> - 加了ID重复率的实时监控
>
> **第三类是基础设施问题，有2起。** K8s节点回收和AWS Spot大规模Rebalance，导致核心服务没有节点调度。改进是：
> - 核心服务不用Spot实例
> - 加了safe-to-evict annotation和PDB配置
> - 副本数从2台提升到3台以上，跨可用区部署
>
> **第四类是数据库问题，有1起。** CDC初始快照导致CPU100%，锁竞争。改进是：
> - 大表CDC不在主库做，用只读副本
> - 初始快照在低峰期执行，随时可以中止
> - 数据库操作前必须和业务方沟通
>
> **最核心的架构改进是确立了「支付核心最小依赖原则」：**
> 支付主流程只依赖数据库和Redis，其他所有依赖全部异步化、可降级，任何中间服务挂了都不能影响支付本身。"

---

## 📝 总结

| 维度 | 事故数 | 已改进 | 待跟进 |
|-----|-------|-------|-------|
| 消息系统耦合 | 3起 | ⚠️ 部分 | Outbox模式全面落地 |
| ID生成可靠性 | 1起 | ✅ 完成 | - |
| K8s调度可靠性 | 2起 | ✅ 完成 | 多集群灾备 |
| 数据库变更管控 | 1起 | ✅ 完成 | - |

> **核心教训：不要让非核心组件的故障，变成核心业务的灾难。**
