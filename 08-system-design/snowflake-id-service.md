# 高可用雪花 ID 生成服务设计

> 面试经典题：设计一个分布式唯一 ID 生成服务，各业务线统一调用，同时支持业务个性化需求，如何保证高可用？

---

## 一、需求澄清（面试先问清楚）

```
1. ID 全局唯一还是业务域内唯一？        → 全局唯一（跨服务不重复）
2. 是否需要有序？                       → 趋势递增即可（不要求严格连续）
3. 吞吐量要求？                         → 单服务百万 QPS，全局千万级
4. 业务个性化指什么？                   → 各业务线需要不同的 epoch、位宽、业务前缀
5. 是否需要多语言 SDK？                 → Go/Java/Python 都要支持
6. 时钟回拨怎么处理？                   → 不能产生重复 ID，允许短暂等待
7. 服务挂了业务能降级自己生成吗？        → 能，SDK 要有本地降级模式
```

---

## 二、雪花算法基础

### 1. 标准 Snowflake 64-bit 结构

```
 0 | 41 bits timestamp | 10 bits worker | 12 bits seq
 ^       ^                   ^                ^
符号位  毫秒级时间戳         机器ID           序列号
(恒0)  (69年不重复)      (最多1024节点)   (每ms 4096个)

示例：
+----+-------------------------------------------+----------+----------+
| 0  | 0000000000000000000000000000000000000000 1 | 0000000001 | 000000000001 |
+----+-------------------------------------------+----------+----------+

解码示例（Go）:
timestamp = (id >> 22) + epoch        // 41 bits
workerID  = (id >> 12) & 0x3FF        // 10 bits
sequence  = id & 0xFFF                // 12 bits
```

### 2. 各位段的权衡

| 位段 | 标准 | 可调整方向 | 影响 |
|------|------|-----------|------|
| 时间戳 | 41 bits | 减少 → 增加节点/序列数 | 减1bit = 寿命减半 |
| 机器ID | 10 bits | 5+5（DC+Worker）或单段 | 决定最大节点数 |
| 序列号 | 12 bits | 增加 → 单机更高 QPS | 每 ms 最大 4096 个 |
| 业务位 | 0 bits | 可从时间戳借位 | 区分业务线/租户 |

---

## 三、整体架构设计

```
┌──────────────────────────────────────────────────────────────────────┐
│                         调用方                                        │
│   Order-Svc    Payment-Svc    User-Svc    外部合作方                  │
│      │               │            │           │                       │
│   Go SDK          Go SDK      Java SDK    HTTP/gRPC                   │
└──────┼───────────────┼────────────┼───────────┼───────────────────────┘
       │               │            │           │
       ▼               ▼            ▼           ▼
┌──────────────────────────────────────────────────────────────────────┐
│                     ID 生成服务集群（无状态 × N）                      │
│                                                                       │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
│   │  id-svc-0   │  │  id-svc-1   │  │  id-svc-2   │   ...           │
│   │ workerID=1  │  │ workerID=2  │  │ workerID=3  │                 │
│   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                 │
│          │                │                │                          │
│          └────────────────┼────────────────┘                         │
│                           │                                          │
│              Worker ID 注册/租约管理                                  │
└───────────────────────────┼──────────────────────────────────────────┘
                            │
              ┌─────────────┴──────────────┐
              │                            │
       ┌──────▼──────┐             ┌───────▼──────┐
       │   etcd/ZK    │             │    Redis      │
       │ Worker ID 池  │             │  分段缓存池   │
       │ 租约 + 续租   │             │ (可选降级方案) │
       └─────────────┘             └──────────────┘
```

**架构要点：**
- ID 生成服务本身**无状态**，唯一有状态的是 workerID 的获取
- workerID 通过 etcd 租约机制分配，服务启动时抢占，崩溃后自动释放
- SDK 内置本地降级，无需每次请求都走网络

---

## 四、核心设计：WorkerID 分配

### 方案对比

| 方案 | 原理 | 优点 | 缺点 |
|------|------|------|------|
| 静态配置 | 手动给每个实例配 workerID | 简单 | 扩缩容要手动维护，容易冲突 |
| K8s StatefulSet | Pod 名称 `id-svc-{N}` 取尾号 | 天然唯一，重启 ID 不变 | 只适合 K8s 环境 |
| **etcd 租约（推荐）** | 启动时抢 /workers/{id}，带 TTL | 自动回收、高可用 | 依赖 etcd |
| Redis SETNX 池 | 维护可用 ID 池，SETNX 抢占 | 实现简单 | Redis 单点风险 |
| DB 自增 | 每次启动从 DB 申请 | 最简单 | 高可用依赖 DB |

### 推荐方案：etcd 租约

```go
// 启动时注册 Worker ID
func (s *Server) acquireWorkerID(ctx context.Context) (int64, error) {
    lease, err := s.etcd.Grant(ctx, 30) // TTL 30s
    if err != nil {
        return 0, err
    }

    // 尝试注册 0-1023 中的一个
    for id := int64(0); id < 1024; id++ {
        key := fmt.Sprintf("/snowflake/workers/%d", id)
        txn := s.etcd.Txn(ctx).
            If(clientv3.Compare(clientv3.Version(key), "=", 0)). // key 不存在
            Then(clientv3.OpPut(key, s.instanceID, clientv3.WithLease(lease.ID))).
            Commit()
        resp, err := txn.Do()
        if err == nil && resp.Succeeded {
            // 启动续租 goroutine，防止租约过期
            go s.keepAlive(ctx, lease.ID)
            return id, nil
        }
    }
    return 0, errors.New("no available worker ID")
}

// 定期续租（每 10s 续一次，TTL 30s，留 3 倍余量）
func (s *Server) keepAlive(ctx context.Context, leaseID clientv3.LeaseID) {
    ch, _ := s.etcd.KeepAlive(ctx, leaseID)
    for range ch {
        // 自动续租，channel 关闭说明 etcd 连接断了
    }
    // 触发重新选举 workerID 或服务降级
    s.triggerFailover()
}
```

### K8s 环境优化

```yaml
# 使用 StatefulSet，Pod 名天然带序号
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: id-svc
spec:
  replicas: 8
  # Pod 名称: id-svc-0, id-svc-1, ..., id-svc-7
  # workerID = 从 Pod 名解析尾号，无需 etcd
  template:
    spec:
      containers:
      - name: id-svc
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        # workerID = atoi(strings.Split(POD_NAME, "-")[2])
```

---

## 五、核心设计：时钟回拨处理

> 这是雪花算法最难的问题，面试必考

### 什么是时钟回拨？

NTP 同步时，系统时间可能向前跳（正常）或**向后回拨**（危险）。
如果时间回拨 1ms，新生成的 ID 时间戳 < 已生成的 ID 时间戳 → **ID 重复**。

### 处理策略（三层防御）

```
时钟回拨检测
    │
    ├── 回拨 ≤ 5ms → 等待时间追上去 (spin wait)
    │
    ├── 回拨 5-100ms → 切换到备用位段
    │   用 sequence 高位暂时标记「回拨态」，
    │   牺牲 sequence 空间换时间
    │
    └── 回拨 > 100ms → 拒绝服务 + 告警
        返回 error，让调用方降级重试
```

```go
type Snowflake struct {
    mu           sync.Mutex
    epoch        int64  // 自定义纪元（ms）
    workerID     int64
    sequence     int64
    lastTimestamp int64
}

func (s *Snowflake) NextID() (int64, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    now := time.Now().UnixMilli()

    if now < s.lastTimestamp {
        diff := s.lastTimestamp - now

        if diff <= 5 {
            // 策略1：短暂等待
            time.Sleep(time.Duration(diff+1) * time.Millisecond)
            now = time.Now().UnixMilli()
        } else if diff <= 100 {
            // 策略2：用 workerID 高位扩展位规避（牺牲部分 sequence）
            // 实际项目中可以切换到备用 workerID 段
            return 0, fmt.Errorf("clock skew %dms, retry", diff)
        } else {
            // 策略3：严重回拨，拒绝服务
            return 0, fmt.Errorf("clock skew too large: %dms, service degraded", diff)
        }
    }

    if now == s.lastTimestamp {
        s.sequence = (s.sequence + 1) & 0xFFF // 12 bits，溢出回 0
        if s.sequence == 0 {
            // 当前 ms 内序列号用尽，等下一 ms
            for now <= s.lastTimestamp {
                now = time.Now().UnixMilli()
            }
        }
    } else {
        s.sequence = 0
    }

    s.lastTimestamp = now

    id := ((now - s.epoch) << 22) | (s.workerID << 12) | s.sequence
    return id, nil
}
```

---

## 六、核心设计：业务个性化

各业务线可能有不同的需求，通过**配置化位段 + 命名空间**解决：

### 6.1 Bit Layout 配置化

```go
// 每个业务线可定义自己的 ID 规则
type IDSchema struct {
    Name          string // 业务名，如 "payment", "order"
    Epoch         int64  // 自定义纪元，ms（不同业务有不同的"0时刻"）
    TimestampBits int    // 时间戳位数，默认 41
    WorkerIDBits  int    // 机器 ID 位数，默认 10
    SequenceBits  int    // 序列号位数，默认 12
    BizBits       int    // 业务标识位（从 WorkerID 借位）
    BizID         int64  // 业务标识值（区分订单/支付/用户）
}

// 支付业务：需要大 sequence（高并发），用 13 bits
var PaymentSchema = IDSchema{
    Name:          "payment",
    Epoch:         1700000000000, // 2023-11-15 作为纪元
    TimestampBits: 41,
    WorkerIDBits:  9,   // 512 节点足够
    SequenceBits:  13,  // 每 ms 8192 个，应对支付洪峰
    BizBits:       1,   // 1 bit 区分支付单/退款单
}

// 订单业务：需要嵌入区域信息
var OrderSchema = IDSchema{
    Name:          "order",
    Epoch:         1700000000000,
    TimestampBits: 41,
    WorkerIDBits:  6,   // 64 节点
    SequenceBits:  12,
    BizBits:       4,   // 4 bits 区分大区（华东/华北/华南/海外）
}
```

### 6.2 调用方式：统一服务 vs 本地 SDK

```
         ┌──────────────────────────────────┐
         │         调用方选择               │
         │                                  │
         │  高性能场景 (>10w QPS)            │
         │  ┌──────────────────────────┐    │
         │  │  嵌入式 SDK（本地生成）   │    │
         │  │  - 无网络开销            │    │
         │  │  - worker ID 从环境变量  │    │
         │  │    或 etcd 获取后缓存    │    │
         │  └──────────────────────────┘    │
         │                                  │
         │  跨语言 / 低频调用               │
         │  ┌──────────────────────────┐    │
         │  │  gRPC/HTTP API           │    │
         │  │  - 支持批量申请 N 个 ID  │    │
         │  │  - SDK 本地预取缓存池    │    │
         │  └──────────────────────────┘    │
         └──────────────────────────────────┘
```

### 6.3 批量预取（减少 RPC 次数）

```go
// SDK 内部维护本地 ID 池
type IDPool struct {
    pool   chan int64
    schema string
    client IDServiceClient
}

func (p *IDPool) prefetch(ctx context.Context) {
    // 当池子剩余 < 20% 时，异步批量拉取
    ids, _ := p.client.BatchGenerate(ctx, &BatchReq{
        Schema: p.schema,
        Count:  1000, // 每次取 1000 个
    })
    for _, id := range ids {
        p.pool <- id
    }
}

func (p *IDPool) Get(ctx context.Context) (int64, error) {
    if len(p.pool) < cap(p.pool)/5 {
        go p.prefetch(ctx)
    }
    select {
    case id := <-p.pool:
        return id, nil
    case <-ctx.Done():
        return 0, ctx.Err()
    }
}
```

---

## 七、高可用设计

### 7.1 故障场景 & 应对

| 故障 | 影响 | 应对方案 |
|------|------|---------|
| 单节点宕机 | workerID 租约自动过期释放，其他节点继续服务 | etcd 租约 TTL，K8s 健康检查自动重启 |
| etcd 不可用 | 新节点无法注册 workerID | 已运行节点继续服务；降级用 SDK 本地模式（用 Pod IP 末位作 workerID） |
| 时钟回拨 | 可能产生重复 ID | 三层回拨防御（见第五节） |
| 流量突增 | sequence 耗尽，等待下 ms | 横向扩容节点，SDK 批量预取分摊压力 |
| 网络分区 | 部分服务不可达 | SDK 本地降级：用 `ip hash + 进程内计数` 生成临时 ID |
| 服务滚动发布 | 旧 Pod 关闭，workerID 短暂空缺 | StatefulSet 保证 workerID 不变；优雅关闭等待 30s 让 in-flight 请求完成 |

### 7.2 降级策略（SDK 本地模式）

```go
// 服务不可达时的本地降级
func (c *Client) generateLocally() (int64, error) {
    // 用本机 IP 最后 2 字节 + 进程 PID 低位 作为临时 workerID
    // 风险：多实例可能冲突，但比服务不可用强
    localWorkerID := (getLocalIPLast2Bytes() ^ (os.Getpid() & 0x3)) & 0x3FF
    return c.localSnowflake.WithWorkerID(localWorkerID).NextID()
}
```

### 7.3 K8s 部署架构

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: id-service
spec:
  replicas: 6         # 3 个 AZ × 2 副本
  serviceName: id-svc
  podManagementPolicy: Parallel
  template:
    spec:
      topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule  # 强制跨 AZ 分布
      containers:
      - name: id-svc
        resources:
          requests: { cpu: "500m", memory: "256Mi" }
          limits:   { cpu: "2000m", memory: "512Mi" }
        readinessProbe:
          grpc: { port: 9090 }
          initialDelaySeconds: 3
        livenessProbe:
          grpc: { port: 9090 }
          periodSeconds: 5
---
# PodDisruptionBudget：保证滚动发布时至少 4 个实例在线
apiVersion: policy/v1
kind: PodDisruptionBudget
spec:
  minAvailable: 4
  selector:
    matchLabels:
      app: id-service
```

---

## 八、接口设计

```protobuf
syntax = "proto3";

service IDService {
    // 生成单个 ID
    rpc Generate(GenerateReq) returns (GenerateResp);
    // 批量生成（最多 10000 个）
    rpc BatchGenerate(BatchReq) returns (BatchResp);
    // 解析 ID（调试用）
    rpc Decode(DecodeReq) returns (DecodeResp);
}

message GenerateReq {
    string schema = 1;  // 业务 schema 名，如 "payment"，空 = 默认 schema
}

message GenerateResp {
    int64 id = 1;
    int64 timestamp_ms = 2;  // 方便调用方日志记录
}

message BatchReq {
    string schema = 1;
    int32  count = 2;   // 最多 10000
}

message BatchResp {
    repeated int64 ids = 1;
}

message DecodeResp {
    int64  id = 1;
    int64  timestamp_ms = 2;
    int64  worker_id = 3;
    int64  sequence = 4;
    string schema = 5;
    string time_human = 6; // "2024-01-15 10:23:45.123"
}
```

---

## 九、监控 & 告警

```
关键指标（Prometheus）：

# ID 生成 QPS
snowflake_generate_total{schema, worker_id}

# 时钟回拨次数（告警阈值 > 0）
snowflake_clock_skew_total{worker_id, direction="backward"}

# sequence 耗尽次数（说明单 ms 超过 4096/8192 个请求，需扩容）
snowflake_sequence_overflow_total{worker_id}

# Worker ID 注册状态（0=未注册，1=正常，告警阈值 = 0）
snowflake_worker_registered{worker_id, instance}

# etcd 续租延迟
snowflake_etcd_keepalive_latency_ms

告警规则：
- 时钟回拨 > 0 次/min  → P1，立刻检查 NTP
- sequence 溢出 > 100/min → P2，扩容节点
- etcd 连接断开 > 10s → P1，触发本地降级
- 任意节点 workerID 丢失 → P0，可能产生重复 ID
```

---

## 十、面试高频追问

### Q1：为什么不用 UUID？

UUID 没有顺序性，作为数据库主键会导致 B+ 树频繁分裂（随机写）；且长度 128 bit，存储和索引开销大。雪花 ID 64 bit、趋势递增，顺序写友好，InnoDB 性能好 10 倍以上。

### Q2：时钟回拨超过 100ms 怎么办？

三级处理：
1. **≤5ms**：spin wait 等时间追上来
2. **5-100ms**：返回 error 让业务重试（SDK 切换到其他节点）
3. **>100ms**：服务降级 + 立刻告警，检查 NTP，人工介入

生产中也可以用**扩展位**规避：把 sequence 最高 2 bits 作为「回拨计数器」，回拨后计数器+1，既不重复又不等待，代价是 sequence 可用数量减少 75%。

### Q3：单个 workerID 每秒最大 QPS 是多少？

标准 12 bits sequence：`4096 个/ms × 1000 = 409.6w QPS`
13 bits sequence：`8192 个/ms × 1000 = 819.2w QPS`
实际单机远达不到这个值（网络、序列化开销），但嵌入式 SDK 本地生成可以接近理论值。

### Q4：如何保证跨数据中心的唯一性？

将 10 bits workerID 拆成 `5 bits datacenter + 5 bits machine`，不同 DC 的 datacenter 值不同，天然隔离。Meican 用的方式是 Region 标识写入 etcd key 前缀：`/snowflake/{region}/workers/{id}`。

### Q5：SDK 预取池会不会导致 ID 空洞？

会。预取了 1000 个 ID，进程崩溃后这批 ID 浪费掉，下次从新的 sequence 开始，会有不连续。
**这是可以接受的**：雪花 ID 只保证趋势递增和全局唯一，不保证连续无空洞。如果业务需要严格连续，改用数据库 AUTO_INCREMENT（但不适合高并发）。

### Q6：与美团 Leaf、百度 UidGenerator 的对比？

| | 标准 Snowflake | 美团 Leaf-Snowflake | 百度 UidGenerator |
|--|--|--|--|
| 依赖 | etcd/ZK | ZooKeeper | 无（数据库分配 workerID）|
| 时钟回拨 | 需自行处理 | 弱依赖 ZK 时间 | 用未来时间解决 |
| 业务定制 | 灵活 | 固定 | 有 delta/cache 两种 |
| 特点 | 最通用 | 美团生产验证 | 提前申请未来序列号 |

美餐选型建议：自研 + etcd，K8s StatefulSet 部署，workerID 从 Pod 序号取，省掉 etcd 依赖的复杂度。

### Q7：如何做 ID 解析（时间反查）？

```go
func Decode(id int64, schema IDSchema) IDInfo {
    ts := (id >> (schema.WorkerIDBits + schema.SequenceBits)) + schema.Epoch
    workerID := (id >> schema.SequenceBits) & ((1 << schema.WorkerIDBits) - 1)
    seq := id & ((1 << schema.SequenceBits) - 1)
    return IDInfo{
        Timestamp: time.UnixMilli(ts),
        WorkerID:  workerID,
        Sequence:  seq,
    }
}
// 作用：日志排查时可以从 ID 直接知道生成时间、哪个节点生成的
```

---

## 十一、本地嵌入方案深度解析

> 对应面试追问：如果不用中心化 generator 服务，每个 Pod 本地生成 ID，workerID 怎么管理？

### 11.1 为什么本地嵌入更优

| 维度 | 中心化 generator（美餐现状）| 本地嵌入生成 |
|---|---|---|
| 网络开销 | 有（SDK 批量预取摊薄）| **零**，完全本地 |
| 故障影响范围 | 全公司 SPOF | 单服务隔离 |
| 延迟 | 受 buffer 耗尽影响 | 纳秒级，无上限 |
| 时钟回拨处理 | server 端统一 | 各服务自己处理 |
| 运维复杂度 | 低（一套服务）| 中（每个服务管自己 workerID）|

### 11.2 Pod 生命周期与 workerID 绑定

核心机制：**etcd lease（租约）**，workerID 和 Pod 的存活状态绑定。

```
Pod 启动
  └── etcd.Grant(TTL=30s) → 拿到 leaseID
      └── 扫描 /snowflake/{svc}/workers/0~1023，原子抢占一个空位
          └── 成功 → workerID 确定，开始本地生成 ID
              └── 后台 goroutine 每 10s KeepAlive 续租

Pod 正常关闭（滚动发布/缩容）
  └── 主动 etcd.Revoke(leaseID)
      └── key 立刻消失 → workerID 立刻释放回池子

Pod 被强杀（OOM / 节点宕机）
  └── 续租 goroutine 停止
      └── 30s 后 lease 自动过期 → key 消失 → workerID 自动释放
```

滚动发布时新旧 Pod 并存，不会冲突：

```
发布前:  Pod-old(workerID=3) 运行
发布中:  Pod-old(workerID=3) + Pod-new(workerID=7) 同时存在 → 不同 workerID，ID 不重复
发布后:  Pod-old 释放 workerID=3，只剩 Pod-new(workerID=7)
```

### 11.3 多 Pod 同时启动的抢占算法

新 Pod 启动时从 0 开始扫描，**用 etcd 事务做原子性抢占**，保证多个 Pod 并发注册时不会拿到同一个 workerID：

```go
func acquireWorkerID(ctx context.Context, etcd *clientv3.Client, svc string, max int64) (int64, error) {
    lease, _ := etcd.Grant(ctx, 30) // TTL 30s

    // 优化：先 List 已占用的 ID，跳过，减少无效事务
    resp, _ := etcd.Get(ctx, fmt.Sprintf("/snowflake/%s/workers/", svc), clientv3.WithPrefix())
    occupied := map[int64]bool{}
    for _, kv := range resp.Kvs {
        id, _ := strconv.ParseInt(path.Base(string(kv.Key)), 10, 64)
        occupied[id] = true
    }

    for id := int64(0); id < max; id++ {
        if occupied[id] {
            continue // 已知被占用，跳过
        }
        key := fmt.Sprintf("/snowflake/%s/workers/%d", svc, id)

        // 原子事务：IF key 不存在 THEN PUT（带租约）
        txnResp, _ := etcd.Txn(ctx).
            If(clientv3.Compare(clientv3.Version(key), "=", 0)). // key 不存在
            Then(clientv3.OpPut(key, podName, clientv3.WithLease(lease.ID))).
            Commit()

        if txnResp.Succeeded {
            go keepAlive(ctx, etcd, lease.ID) // 启动续租
            return id, nil
        }
        // 事务失败 = 被别人抢了，继续试下一个
    }
    return -1, errors.New("no available workerID")
}
```

> **为什么 List 之后还要用事务？**
> List 和 PUT 之间有时间窗口，两个 Pod 可能同时 List 到同一个空位，都以为可以抢。
> etcd 事务（Compare-And-Swap）是线性一致的，只有一个会成功，另一个失败后自动跳到下一个。

### 11.4 workerID 位数不够用怎么办

全公司几十个服务、每个服务多个 Pod，同时在线的 Pod 可能有几百上千个。
标准 10 bits = 1024，在大规模 K8s 场景下可能不够。

**方案一：扩大 node bits，压缩 sequence bits**

```
标准:    | 41bits timestamp | 10bits worker | 12bits seq |  → 1024 Pod，4096/ms
调整后:  | 41bits timestamp | 12bits worker | 10bits seq |  → 4096 Pod，1024/ms
```

4096 个 Pod 同时在线，每 Pod 每 ms 生成 1024 个，总吞吐 4096×1024×1000 = 42 亿/秒，
对任何公司都绰绰有余。

**方案二：按服务隔离 workerID 空间（推荐）**

```
etcd 路径按服务名隔离：
/snowflake/checkout/workers/    → checkout 服务自己的池（0-63，6bits）
/snowflake/payment/workers/     → payment 服务自己的池（0-63，6bits）
/snowflake/order/workers/       → order 服务自己的池（0-63，6bits）
```

ID 结构嵌入服务标识：

```
| timestamp 41bits | serviceID 6bits | workerID 6bits | seq 10bits |
                      ↑ 64个服务         ↑ 每服务64个Pod  ↑ 1024/ms
```

好处：
- 每个服务独立，workerID 空间互不影响
- 从 ID 可以直接解码出是哪个服务生成的，排查问题方便
- 某个服务扩容不影响其他服务的 workerID 池

---

## 十二、美餐真实实现分析（kiwi/generator）

> 对照理论设计，看美餐是怎么实际落地的

### 12.1 整体架构

```
业务服务（checkout/payment/order...）
  └── go.planetmeican.com/meican-pay/pkg/idmachine
        ├── [dev/test] localid: GenIDWithTime(now) + atomic 自增
        └── [prod]     generator-sdk
              └── gRPC → kiwi/generator 服务（3 Pod，app-kiwi namespace）
                              └── etcd 管理 workerID 租约
                              └── PostgreSQL（辅助持久化）
```

### 12.2 SDK 预取机制

```
SDK 初始化（New()）
  └── fulfill()：批量 RPC 预填满本地 buffer
        └── 4096 / 100 = 约 40 次 RPC，填满后才返回
            └── 若 server 不可达，直接 panic（启动失败）

运行时
  └── produce() goroutine（无限循环）
        └── batch(100) → gRPC → 取 100 个 → 写入 chan(4096)
              └── chan 满时自然阻塞，不会无限 RPC

业务调用 Generate(ctx)
  └── select { case id := <-cache.Chan }  ← 直接从本地 channel 取，几乎零延迟
```

关键参数：

| 参数 | 值 | 含义 |
|---|---|---|
| BufferSize | 4096 | 本地最多缓存 4096 个 ID |
| BatchSize | 100 | 每次 RPC 批量取 100 个 |
| InitConn/MaxConn | 2/5 | gRPC 连接池大小 |
| PingInterval | 1min | gRPC keepalive 心跳 |

### 12.3 美餐雪花 ID 位段

```
shiftBits = 19
epoch     = 2017-08-03 18:48:03 +0800（美餐自定义纪元）

位段结构（从 localid 注释推断）：
| timestamp(ms, 相对于epoch) ~45bits | dc 4bits | node 5bits | seq 10bits |
                                       ↑ ConfigMap 写死 dc=2

GenIDWithTime(t) = (t.UnixMilli - epoch) << 19  // 仅时间戳部分，低 19 位为 0
GetTimeFromID(id) = (id >> 19) + epoch           // 从 ID 反解时间
```

### 12.4 localid（dev/test）不是真正的雪花

```go
// pkg/idmachine/localid/localid.go
func (s client) Generate(ctx context.Context) (sdk.SnowFlakeID, error) {
    return sdk.SnowFlakeID(GenIDWithTime(xtime.Now()) + atomic.AddUint64(&randomNum, 1)), nil
}
// 时间戳部分按雪花格式 ✅
// 低 19 bits 是 atomic 自增计数器，不区分 dc/node/seq ❌
// 只保证单进程唯一，多实例 dev 环境有冲突风险
```

### 12.5 JSON 序列化为字符串

```go
func (n SnowFlakeID) MarshalJSON() ([]byte, error) {
    return []byte(`"` + n.String() + `"`), nil
}
```

雪花 ID 是 uint64，最大 ~1.8×10¹⁹，超过 JS `Number.MAX_SAFE_INTEGER`（2⁵³ ≈ 9×10¹⁵），
JSON 序列化成数字会被前端精度截断，序列化成字符串是正确做法。

### 12.6 已知问题

| 问题 | 描述 | 风险 |
|---|---|---|
| 全公司 SPOF | 所有业务共用 3 个 generator Pod | 挂了全公司 ID 生成受影响 |
| 无鉴权 | `sdk.New("", "", addr)` clientID/Secret 传空 | 靠内网隔离，无 quota 控制 |
| produce() 无退避 | RPC 失败直接重试，热循环打日志 | 不影响业务（有 buffer），但日志噪音大 |
| dc 硬编码 | configmap `dc: 2`，prod/sandbox 相同 | 多地域扩展时需注意 |
| nodeID 不回收（待确认）| etcd rawPath 末尾是 `/seq`，疑似单调递增 | 3 个 Pod 规模下 5bits 空间够用 |

### 12.7 扩容影响（3 Pod → 6 Pod）

| 维度 | 变化 |
|---|---|
| ID 吞吐上限 | 翻倍（3072/ms → 6144/ms）|
| 可用性 | 明显提升，容忍更多 Pod 同时故障 |
| SDK 体感 | 几乎无变化（本地 buffer 挡住了大部分影响）|
| nodeID 消耗速度 | 更快，但 32 个 ID 的空间现实中不会耗尽 |

---

## 十三、面试追问补充

### Q8：公共 generator 服务 vs 按业务线分配 workerID，哪种更好？

两种思路的核心差异：

| | 公共 generator（美餐现状）| 按业务线分 bizID |
|---|---|---|
| ID 可追溯业务 | ❌ 看不出来 | ✅ bizID 嵌入 ID |
| 故障隔离 | ❌ 全公司 SPOF | ✅ 各业务独立 |
| 运维复杂度 | 低 | 中 |
| workerID 管理 | 集中，简单 | 分散，各服务自管 |

实际推荐：**在 ID 位段里加 bizID（3~4 bits）**，从业务层面隔离，generator 服务还是可以共用，改动成本最低。

### Q9：本地嵌入方案，workerID 不够用怎么办？

全公司几百个 Pod 同时在线，10 bits（1024个）在大规模场景下可能不够。两个方向：

1. **扩大 node bits**：`12 bits node + 10 bits seq` → 4096 Pod 同时在线，1024/ms 每 Pod，总吞吐够用
2. **按服务隔离 etcd 路径**：`/snowflake/{svc}/workers/`，每个服务独立 workerID 池，互不影响

### Q10：多个 Pod 同时启动，如何保证不拿到同一个 workerID？

依赖 **etcd 事务的线性一致性**：

```
IF key 不存在（Version==0）THEN PUT key（带 lease）
```

即使 10 个 Pod 同时抢 workerID=0，etcd 内部串行执行，只有一个事务成功，
其余的失败后自动跳到下一个 ID 继续尝试，直到找到空位。

---

## 十四、一句话总结（面试收尾用）
