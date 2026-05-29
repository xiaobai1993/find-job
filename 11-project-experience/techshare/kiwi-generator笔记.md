# Meican kiwi/generator ID 生成服务笔记

> 美餐内部雪花 ID 生成服务，供全公司业务线统一调用。
> 源码在 `gitlab.planetmeican.com/kiwi/generator`，此处基于 ArgoCD 配置和 SDK vendor 代码整理。

---

## 一、整体架构

```
业务服务（checkout / payment / order / ...）
    └── go.planetmeican.com/meican-pay/pkg/idmachine
          ├── [test 环境] localid（本地生成，非标准雪花）
          └── [其他环境] generator-sdk
                └── gRPC 连接池（2~5 个连接）
                      └── generator 服务（app-kiwi namespace，3~6 Pod）
                            ├── etcd  → workerID 注册 / 租约管理
                            └── PostgreSQL → 辅助持久化
```

### 请求路径（集群内）

```
业务 Pod
  └── generator-sdk.Generate(ctx)
        └── 本地 channel(4096) 有数据 → 直接返回，零网络开销
            本地 channel 空了 → gRPC BatchGenerate(100个)
                  └── K8s ClusterIP: generator.app-kiwi.svc.cluster.local:9123
                        └── Istio VirtualService → generator-stable（正常）
                                                 → generator-canary（发布期）
                              └── generator Pod 本地生成雪花 ID 返回
```

---

## 二、雪花 ID 位段结构

```
shiftBits = 19
epoch     = 2017-08-03 18:48:03 +0800（美餐自定义纪元，毫秒）

64-bit 结构（从 SDK localid 注释推断）：
┌──────────────────────────────────┬────────┬─────────┬────────────┐
│     timestamp（约 45 bits）       │ dc(4b) │ node(5b)│  seq(10b)  │
│  (t.UnixMilli - epoch) << 19     │   2    │  0~31   │  0~1023/ms │
└──────────────────────────────────┴────────┴─────────┴────────────┘

生成示例（Go）：
  ts  = uint64(now.UnixMilli()) - defaultEpoch
  id  = (ts << 19) | (dc << 15) | (node << 10) | sequence

反解时间：
  ts  = id >> 19
  t   = time.UnixMilli(int64(ts + defaultEpoch))
```

**JSON 序列化为字符串**：雪花 ID 是 uint64，最大 ~1.8×10¹⁹，超过 JS `Number.MAX_SAFE_INTEGER`（2⁵³），前端直接用数字会精度截断，所以 MarshalJSON 输出带引号的字符串。

---

## 三、服务配置

### 3.1 应用配置（ConfigMap）

```yaml
# argocd-prod/kiwi/generator/configmap.yaml
dc: 2               # datacenter ID，写死，prod 和 sandbox 都是 2
httpPort: 9122      # HTTP（健康检查 /health-check、HTTP Gateway）
rpcPort: 9123       # gRPC（对业务提供 ID 生成接口）

database:
  dsn: "postgres://:@127.0.0.1:5432/generator?sslmode=disable&Timezone=Asia/Shanghai"
  maxConn: 20
  maxIdle: 10

etcd:
  addrs: ["etcd-endpoint.etcd.svc.cluster.local:2379"]
  rawPath: "/kiwi/generator/dc/%d/node/seq"   # %d = dc 值(2)
  dialTimeout: "3s"
```

**etcd rawPath 含义**：路径末尾 `/seq` 表示用 etcd 存储顺序计数器来分配 nodeID。每个 Pod 启动时原子递增该计数器，取到的值作为自己的 nodeID。

### 3.2 Rollout（ArgoCD 发布策略）

```yaml
# prod: replicas=3，sandbox: replicas=1
replicas: 3
strategy:
  canary:
    stableService: generator-stable
    canaryService: generator-canary
    trafficRouting:
      istio:
        virtualService:
          name: generator-vsvc
          routes: [primary]
    steps:
      - setWeight: 0
      - pause: {}          # 手动放行
      - setWeight: 15
      - pause: {}          # 手动放行
      - setWeight: 40
      - pause: {duration: 40s}
      - setWeight: 60
      - pause: {duration: 20s}
      - setWeight: 80
      - pause: {duration: 20s}
      # 最后 20% → 100% 由 ArgoCD 自动完成
```

### 3.3 HPA 自动扩缩容

```yaml
scaleTargetRef: generator Rollout
minReplicas: 3
maxReplicas: 6
metrics:
  - type: ContainerResource
    containerResource:
      name: cpu
      container: generator-image
      target:
        type: Utilization
        averageUtilization: 70    # CPU 使用率超 70% 触发扩容
```

正常 3 个 Pod，CPU 压力大时最多扩到 6 个。

### 3.4 资源配置

```yaml
# prod
resources:
  requests: {cpu: 100m, memory: 128Mi}
  limits:   {cpu: 500m, memory: 512Mi}
qos: Burstable    # TieredApplication 定义

# sandbox
resources:
  requests: {cpu: 128m, memory: 128Mi}
  limits:   {cpu: 128m, memory: 128Mi}
```

### 3.5 节点调度

```yaml
nodeSelector:
  intend: app1
  kubernetes.io/arch: arm64    # 跑在 ARM 节点上
tolerations:
  - key: kubernetes.io/arch
    value: arm64
    effect: NoExecute

# TieredApplication
tier: tier1       # 核心服务，高优先级调度
spotPreference: NoSpot   # 不使用 Spot 实例，保证稳定性
terminationGracePeriodSeconds: 10
```

---

## 四、网络配置

### 4.1 K8s Service（三个）

| Service 名 | 用途 | 端口 |
|---|---|---|
| `generator` | 全量流量，普通调用方使用 | 80→9122(http), 9123→9123(grpc) |
| `generator-stable` | Istio 路由到稳定版 Pod | 同上 |
| `generator-canary` | Istio 路由到金丝雀 Pod | 同上 |

所有 Service 均标注 `konghq.com/protocol: grpc`。

### 4.2 访问域名（Kong Ingress）

| 集群 | HTTP 域名 | gRPC 域名 |
|---|---|---|
| eks-fan（老集群）| `generator-web.app-kiwi.ntrnl-eks-fan.meican` | `generator-grpc.app-kiwi.ntrnl-eks-fan.meican` |
| eks-fan2（新集群）| `generator-web.app-kiwi.ntrnl-eks-fan2.meican` | `generator-grpc.app-kiwi.ntrnl-eks-fan2.meican` |

每个集群各有 kong1 / kong2 双活各一份 Ingress，共 4 组 8 个 Ingress 规则。
gRPC 入口使用 HTTPS（`konghq.com/protocols: "https"`）。

### 4.3 Istio 流量路由

```
正常状态：
  generator.app-kiwi.svc.cluster.local
    └── VirtualService → stable 100% / canary 0%

发布期间（canary 步骤）：
    └── VirtualService → stable 60% / canary 40%（由 ArgoCD 控制权重）

DestinationRule：stable 和 canary 均使用 ROUND_ROBIN
```

### 4.4 Istio Sidecar Egress（出站白名单）

generator Pod 的出站流量受 `REGISTRY_ONLY` 限制，只允许访问以下目标：

```
集群内部（必须）：
  - istio-system/*
  - kube-system/*
  - default/kubernetes.default.svc.cluster.local
  - nacos/nacos-hs.nacos.svc.cluster.local
  - nacos/nacos-cs.nacos.svc.cluster.local

外部 AWS 服务：
  - app-kiwi/*.cn-northwest-1.amazonaws.com.cn
  - app-kiwi/*.cn-north-1.amazonaws.com.cn

PostgreSQL（外部数据库）：
  - app-kiwi/5432.customize.serviceentry.k8s.com  (port 5432)

etcd（通过 NLB 访问）：
  - app-kiwi/ntrnl-kong1-eks-fan-*.elb.cn-northwest-1.amazonaws.com.cn (port 2379)
```

---

## 五、workerID 分配机制

### 5.1 分配流程

```
Pod 启动
  └── 连接 etcd: etcd-endpoint.etcd.svc.cluster.local:2379
      └── 读取 /kiwi/generator/dc/2/node/seq（原子递增）
          └── 拿到值 N → 本 Pod 的 nodeID = N
              └── 开始生成 ID：(timestamp << 19) | (dc=2 << 15) | (N << 10) | seq
```

### 5.2 已知限制

- **nodeID 不回收**：rawPath 末尾是 `/seq`（计数器），Pod 死掉后该 nodeID 不归还，下次新 Pod 拿到的是 N+1。在 3~6 个 Pod 规模下，5 bits 的 node 空间（最多 32 个）短期内不会耗尽，但长期频繁发布会消耗。
- **dc 写死为 2**：prod 和 sandbox 配置相同，未来多地域部署需注意区分。
- **无鉴权**：SDK 调用时 clientID / clientSecret 均传空串，依赖集群内网隔离。

---

## 六、SDK 行为（调用方视角）

### 6.1 初始化

```go
// 伪代码
sdk.New("", "", "generator-grpc.app-kiwi.ntrnl-eks-fan2.meican:443")
  └── NewGRpcPool(addr, initConn=2, maxConn=5, idleTimeout=30s)
      └── fulfill()：预填 buffer
            └── 循环调用 batch(100)，共约 40 次 RPC，填满 4096 个 ID
                └── 失败则 log.Panic，服务无法启动
      └── go produce()  // 后台无限循环持续补货
```

### 6.2 运行时

```
produce() 无限循环：
  batch(100) → gRPC BatchGenerate → 写入 chan(4096)
  → chan 满时阻塞（背压），不会无限打 server
  → 失败时直接 log.Error + 重试（无退避，可能热循环打日志）

Generate(ctx)：
  select {
    case id := <-cache.Chan: return id   // 本地 channel 取，无网络
    case <-ctx.Done(): return timeout
  }
```

### 6.3 关键参数

| 参数 | 值 | 说明 |
|---|---|---|
| BufferSize | 4096 | 本地 channel 容量 |
| BatchSize | 100 | 每次 RPC 取多少个 |
| InitConn | 2 | gRPC 连接池初始连接数 |
| MaxConn | 5 | gRPC 连接池最大连接数 |
| IdleTimeout | 30s | 空闲连接超时 |
| PingInterval | 1min | gRPC keepalive 心跳间隔 |
| PingTimeout | 10s | keepalive 超时 |

---

## 七、本地/测试环境（localid）

```go
// pkg/idmachine/localid/localid.go
// test 环境（cfg.Env == "test"）使用，不走 gRPC

func (s client) Generate(ctx context.Context) (sdk.SnowFlakeID, error) {
    return sdk.SnowFlakeID(GenIDWithTime(xtime.Now()) + atomic.AddUint64(&randomNum, 1)), nil
}

// GenIDWithTime: 时间戳部分对齐真实雪花格式（左移 19 位，相对 epoch）
// 低 19 位：atomic 自增计数器（不区分 dc/node/seq）
// 结论：时间部分格式正确，低位不是标准雪花，单进程唯一，多实例有冲突风险
```

附带工具函数（用于按 ID 范围查询，避免存 created_at 字段）：

| 函数 | 用途 |
|---|---|
| `GenIDWithTime(t)` | 给定时间 → 生成对应时间戳的雪花 ID（低位全 0），作范围查询下界 |
| `GetTimeFromID(id)` | 从 ID 反解生成时间，用于日志排查 |
| `GetIDRangeForMonth(t)` | 返回某月 ±1 个月的 ID 范围，用于分表/分区查询 |
| `GetExtendedIDRange(s,e)` | 自定义时间范围转 ID 范围 |

---

## 八、已知问题与改进点

| 问题 | 现状 | 影响 | 建议 |
|---|---|---|---|
| 全公司 SPOF | 所有业务共用 3~6 Pod | generator 故障 → 全公司 buffer 耗尽后 ID 生成失败 | 各业务线嵌入本地生成（etcd 只做 workerID 注册） |
| 无鉴权 | clientID/Secret 传空 | 内网任意服务可调用，无 quota 控制 | 补充 JWT 或 mTLS 鉴权 |
| produce() 无退避 | RPC 失败立即重试 | server 故障时热循环打日志，CPU 空转 | 加指数退避 + jitter |
| nodeID 不回收 | etcd 纯计数器 | 频繁发布消耗 nodeID，5bits 上限 32 | 改为 etcd lease，Pod 死后自动归还 |
| dc 硬编码 | prod/sandbox 同为 2 | 多地域部署时 ID 可能冲突 | dc 从环境变量注入，不同 region 用不同值 |
| terminationGracePeriodSeconds 短 | 仅 10s | 强杀时 etcd nodeID 需等 30s lease 过期才释放 | 改为 >= 30s，或补充优雅关闭逻辑主动 revoke |
| ID 无业务标识 | nodeID 绑定 Pod，看不出业务来源 | 排查问题无法从 ID 知道是哪个服务生成 | 位段里加 bizID（3~4 bits）区分业务线 |
