# Service Mesh 服务网格 基础 + Istio（Go + 真实生产配置）

> 基于真实支付服务（checkout）的 Istio + Argo Rollout + Kong 生产配置讲解

---

## 一、基础概念

### 1. 什么是服务网格？解决了什么问题？

服务网格是处理服务间通信的基础设施层。核心是 **Sidecar 边车模式**：每个服务 Pod 旁边注入一个代理容器（Envoy），所有入站和出站流量先经过 Sidecar，限流、重试、超时、鉴权、监控这些公共逻辑全部下沉到 Sidecar，业务代码不用关心。

**解决的核心痛点：**

1. **零侵入**：业务代码不引任何网络治理 SDK，不用因为框架升级而重新发版
2. **多语言统一治理**：Go、Java、Python 各自的服务，都由同一套 Istio 统一管理
3. **统一可观测性**：不需要业务埋点，Sidecar 自动上报所有服务的调用指标、链路追踪、访问日志
4. **流量安全**：服务间 mTLS 双向加密，不需要业务代码处理

---

### 2. Istio 的架构是什么？

Istio 分两层：

```text
控制平面（Control Plane）：istiod
  - 接收你写的 VirtualService / DestinationRule / Sidecar 等配置
  - 翻译成 Envoy 能识别的 xDS 配置，实时推送给所有 Sidecar
  - 不处理业务流量，只管配置下发

数据平面（Data Plane）：每个 Pod 里的 Envoy Sidecar
  - 拦截所有入站（inbound）和出站（egress）流量
  - 执行路由、重试、限流、超时、追踪等具体操作
  - 上报指标和日志给监控系统
```

改一条路由规则，只需要 `kubectl apply` 一下 VirtualService，istiod 立刻把新配置推给所有 Sidecar，不用重启任何业务服务。

---

### 3. 我们实际用了哪些 Istio 功能？

checkout 服务在生产上实际使用的 Istio 功能：

| 功能 | 对应资源 | 用途 |
|------|---------|------|
| 灰度发布（金丝雀）| VirtualService + DestinationRule | Argo Rollout 按比例切量 |
| 重试 + 超时 | VirtualService | 下游不可用时自动重试 |
| 出口流量白名单 | Sidecar（REGISTRY_ONLY）| 限制服务只能访问白名单域名 |
| 外部服务注册 | ServiceEntry | 把 RDS、Redis、外部 API 注册进网格 |
| 负载均衡 | DestinationRule | 稳定版和金丝雀版各自的 ROUND_ROBIN |

---

## 二、灰度发布（金丝雀）

### 4. checkout 的灰度发布是怎么实现的？

用 **Argo Rollout + Istio VirtualService** 联动，实现按流量比例的金丝雀发布。

**三个 Service 的分工：**

```yaml
# service.yaml
checkout         # 全量 Service，选所有 Pod（selector: app: checkout）
checkout-stable  # 稳定版 Service，Argo Rollout 维护，始终指向老版本 Pod
checkout-canary  # 金丝雀 Service，Argo Rollout 维护，指向新版本 Pod
```

**VirtualService 控制流量比例：**

```yaml
# istio-virtual-service.yaml
spec:
  hosts:
    - checkout.app-meican-pay.svc.cluster.local
  http:
    - name: primary
      timeout: 10s
      retries:
        attempts: 3
        retryOn: unavailable   # 下游不可用时重试
      route:
        - destination:
            host: checkout-stable  # 老版本
            subset: v1
          weight: 100             # 平时 stable 拿全部流量
        - destination:
            host: checkout-canary  # 新版本
            subset: v1
          weight: 0               # 发布时由 Argo Rollout 逐渐调高
```

**DestinationRule 定义子集：**

```yaml
# istio-destination-rule.yaml
# stable 和 canary 各自一个 DestinationRule
spec:
  host: checkout-stable
  trafficPolicy:
    loadBalancer:
      simple: ROUND_ROBIN   # Pod 间轮询
  subsets:
    - name: v1
      labels:
        app: checkout
```

**Argo Rollout 定义发布节奏：**

```yaml
# rollout.yaml
strategy:
  canary:
    canaryService: checkout-canary
    stableService: checkout-stable
    trafficRouting:
      istio:
        virtualService:
          name: checkout-vsvc
          routes:
            - primary
    steps:
      - setWeight: 0      # 先部署金丝雀 Pod，但不切量
      - pause:
          duration: 8s    # 等 Pod Ready
      - setWeight: 15     # 切 15% 流量给新版本
      - pause: {}         # ← 手动卡点！需要人工确认没问题再继续
      - setWeight: 40     # 切 40%
      - pause:
          duration: 40s
      - setWeight: 60
      - pause:
          duration: 20s
      - setWeight: 80
      - pause:
          duration: 20s
      # 之后 Argo 自动升到 100%，把金丝雀 Pod 全部变成 stable
```

**关键设计点：**

- 第二步 `pause: {}` 是**无限等待**，必须人工执行 `kubectl argo rollouts promote checkout` 才继续，保证有人确认 15% 流量下没有异常才放量
- `maxUnavailable: 0`，发布过程中始终保证所有旧 Pod 可用，不会有流量报错
- 出问题随时执行 `kubectl argo rollouts abort checkout`，Argo 把金丝雀 Service 的流量立刻切回 stable

---

### 5. 灰度发布整个流程说一遍

```text
1. 推送新镜像 → Argo CD 检测到 rollout.yaml 里镜像变更，触发 Rollout

2. Argo Rollout 启动金丝雀 Pod（新版本）
   Pod 通过 readinessProbe（TCP:8023）才会接流量

3. 切 15% 流量（Argo 修改 VirtualService 的 weight）
   Istio Sidecar 实时生效，无需重启任何服务

4. 手动卡点：观察 Grafana 监控、错误率、P99 延迟
   → 没问题：promote，继续放量 40% → 60% → 80% → 100%
   → 有问题：abort，VirtualService weight 立刻回到 100:0，金丝雀 Pod 销毁

5. 全量后，stable Pod 滚动更新为新版本
```

---

## 三、出口流量控制

### 6. Sidecar 出口白名单是什么？为什么要做？

checkout 的 Sidecar 配置了 `REGISTRY_ONLY` 模式，出口流量只允许访问白名单里的域名：

```yaml
# sidecar.yaml
spec:
  workloadSelector:
    labels:
      app: checkout
      egress: limit        # 只对有这个 label 的 Pod 生效
  outboundTrafficPolicy:
    mode: REGISTRY_ONLY   # 访问未注册的 host 直接被 Sidecar 拦截返回 502
  egress:
    - hosts:
        # 内部服务
        - "app-meican-pay/*.app-meican-pay.svc.cluster.local"
        - "app-meican-pay/tpw.app-meican-pay.svc.cluster.local"
        - "app-meican-pay/payment.app-meican-pay.svc.cluster.local"
        - "pulsar/pulsar-broker.pulsar.svc.cluster.local"
        # 外部服务（需要配 ServiceEntry 才能访问）
        - "app-meican-pay/production-payment-redis-non-cluster.*.amazonaws.com.cn"
        - "app-meican-pay/*.rds.cn-northwest-1.amazonaws.com.cn"
        ...
```

**为什么这么做：**

1. **安全**：防止因为代码 bug 或供应链攻击，服务向未知外部地址发请求，数据不会被意外外传
2. **可观测**：所有允许的出口流量都在白名单里，一眼知道这个服务能访问哪些依赖
3. **防止"隐式依赖"**：新加一个外部依赖必须显式配置 ServiceEntry，强制走 review 流程，不会有人悄悄加依赖

Pod 上要打 `egress: limit` 这个 label，Sidecar workloadSelector 才会选到这个 Pod 生效。

---

### 7. ServiceEntry 是什么？为什么要配？

Istio 默认只知道 K8s 集群内部的服务。访问外部服务（AWS RDS、Redis、跨集群服务、外部 API）时，需要用 ServiceEntry 把它们注册进网格，才能在 REGISTRY_ONLY 模式下访问，也才能有流量监控和追踪。

**checkout 的 ServiceEntry 分了几类：**

```yaml
# serviceentry.yaml

# 1. 外部 HTTP/HTTPS 服务（跨集群接口、第三方 API）
kind: ServiceEntry
spec:
  hosts:
    - "payment-gateway.meican.com"
    - "*.app-meican-pay.ntrnl-eks-fan2.meican"
  ports:
    - number: 443
      protocol: TCP
  location: MESH_EXTERNAL

# 2. AWS RDS PostgreSQL（端口 5432）
kind: ServiceEntry
spec:
  hosts:
    - "monopoly-cluster.cluster-xxx.rds.cn-northwest-1.amazonaws.com.cn"
    - "monopoly-cluster.cluster-ro-xxx.rds.cn-northwest-1.amazonaws.com.cn"  # 只读副本
  ports:
    - number: 5432
      protocol: TCP
  location: MESH_EXTERNAL

# 3. AWS ElastiCache Redis（端口 6379）
kind: ServiceEntry
spec:
  hosts:
    - "production-payment-redis-non-cluster.*.amazonaws.com.cn"
    - "production-payment-redis-non-cluster-ro.*.amazonaws.com.cn"  # 只读副本
  ports:
    - number: 6379
      protocol: TCP
  location: MESH_EXTERNAL
```

没有 ServiceEntry 的域名，在 REGISTRY_ONLY 模式下访问直接报 502，所以每次新增外部依赖都要同步更新 Sidecar 白名单和 ServiceEntry。

---

## 四、高可用配置

### 8. checkout 怎么保证高可用？

**多维度保障，每一层都有兜底：**

**Pod 分散部署（防单点）：**

```yaml
# rollout.yaml
topologySpreadConstraints:
  - topologyKey: kubernetes.io/hostname     # 不同节点
    maxSkew: 1
    whenUnsatisfiable: ScheduleAnyway
  - topologyKey: topology.kubernetes.io/zone # 不同可用区（AZ）
    maxSkew: 1
    whenUnsatisfiable: ScheduleAnyway
```

多 AZ 打散，一个 AZ 挂了，其他 AZ 的 Pod 还在跑。

**最少可用副本（防发布时全挂）：**

```yaml
# pdb.yaml
spec:
  minAvailable: 2   # 任何时候至少保证 2 个 Pod 可用
```

K8s 滚动更新或节点维护时，如果删一个 Pod 会让可用数低于 2，K8s 会等待，不会强制驱逐。

**优雅退出（防流量丢失）：**

```yaml
# rollout.yaml
lifecycle:
  preStop:
    exec:
      command: ["sh", "-c", "sleep 5"]  # 先睡 5s 再退出
terminationGracePeriodSeconds: 10
```

K8s 先发 SIGTERM，Pod 执行 preStop sleep 5 等待 Istio Sidecar 把路由从这个 Pod 上摘掉，再开始实际停止，防止停止瞬间还有新请求进来。

**请求重试（防瞬时抖动）：**

```yaml
# VirtualService
retries:
  attempts: 3
  retryOn: unavailable   # 下游返回 unavailable 时自动重试，最多 3 次
timeout: 10s             # 10s 超时，防止慢请求拖垮线程
```

---

### 9. 扩缩容策略是怎么设计的？

**两层扩缩容：HPA + CronHPA**

**HPA（自动扩缩容）：**

```yaml
# hpa.yaml
spec:
  minReplicas: 4
  maxReplicas: 15
  metrics:
    - type: ContainerResource
      containerResource:
        name: cpu
        container: checkout-image
        target:
          averageUtilization: 50   # CPU 超过 50% 就扩容
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0   # 立刻扩，不等稳定窗口
      policies:
        - type: Percent
          value: 100      # 每 5s 最多翻倍
        - type: Pods
          value: 2        # 或者每 5s 最多加 2 个
      selectPolicy: Min   # 取两个策略中的较小值（更保守地扩容）
    scaleDown:
      stabilizationWindowSeconds: 300  # 缩容等 5 分钟稳定窗口
      policies:
        - type: Percent
          value: 10       # 每分钟最多缩 10%
```

**扩容很激进，缩容很保守**：流量突然来了要快速响应，但缩容太快会在下一波流量来时措手不及。

**CronHPA（定时预扩容）：**

```yaml
# cron-hpa.yaml
# 周一至周五 11:20 午高峰前扩容
schedule: "20 11 * * 1-5"
args:
  - "--min-replicas=7"   # 最小副本从 4 提升到 7

# 周一至周五 13:00 过了高峰期缩回来
schedule: "00 13 * * 1-5"
args:
  - "--min-replicas=4"   # 最小副本恢复到 4
```

**为什么要 CronHPA：**

HPA 是响应式的，等 CPU 上去了才扩，扩的过程本身需要时间（拉镜像 + Pod Ready + Istio Sidecar 注入）。午餐高峰（11:30-12:30）流量涨的很快，等 HPA 反应过来可能已经影响用户了。提前在 11:20 把最低副本调高，高峰前就 warm up 好，不会有扩容延迟。

---

## 五、踩坑经验

### 10. 用 Istio 踩过什么坑？

**坑 1：新加外部依赖忘了更新 Sidecar 白名单和 ServiceEntry**

新接入一个下游服务，代码写好了，测试环境没问题（测试环境 Sidecar 没开 REGISTRY_ONLY），发到生产环境直接 502。排查半天才发现 Sidecar 的 egress 列表里没有这个域名。

- 解决：写了 checklist，新增外部依赖必须同时提交 sidecar.yaml 和 serviceentry.yaml 的变更，代码 review 时一并检查

**坑 2：preStop 时间不够，请求在 Pod 停止期间丢失**

早期没有 preStop，K8s 发 SIGTERM 给 Pod 之后，Istio Sidecar 的路由摘除有延迟，这个时间窗口里还会有新请求路由到这个 Pod，但 Pod 已经开始停止，导致请求失败。

- 解决：加了 `preStop: sleep 5`，给 Sidecar 足够时间把这个 Pod 从路由中摘除，再开始实际停止流程。5s 来自 Istio 控制平面下发配置的 P99 延迟，实际场景下够用

**坑 3（更隐蔽）：nativeSidecar 未开启，Envoy 和 app 同时退出，优雅退出期间网络断了**

现象：加了 `preStop: sleep 5`，Go 应用也正确实现了 SIGTERM 处理和 5s 优雅退出，但 Pulsar 消息仍然会出现消费失败进死信队列，或者被其他 Pod 重复消费的情况。查日志发现，app 在优雅退出阶段调用 `pulsarConsumer.Close()` 时连接直接断了，ack 发不出去。

**根本原因：Sidecar 退出顺序问题。**

没有开启 `nativeSidecar` 时，Istio Sidecar 是一个普通容器。K8s 在 Pod 终止时，对所有普通容器同时发送 SIGTERM，app 和 Envoy 同时开始退出：

```text
t=0   SIGTERM 同时发给 app 容器 和 Envoy 容器（所有普通容器是平等的）

t=0   app 的 preStop: sleep 5 开始
t=0   Envoy 也开始 drain（关闭入向连接，逐步关闭出向连接）

t=5   app 收到 SIGTERM，开始优雅退出
      → pulsarConsumer.Close() 尝试 ack 消息
      → 但 app 的出站流量全部走 Envoy，Envoy 正在 drain
      → Envoy 已经把 Pulsar 连接关掉了
      → ack 失败，消息重投 / 进死信队列
```

app 的出站流量（包括对 Pulsar broker 的连接）全部被 Envoy 劫持。Envoy 在 drain 时会关闭出向连接，导致 app 在优雅退出阶段**有时间但没有网络**。

**解决：开启 `nativeSidecar: "true"`。**

这个特性（K8s 1.28+ 引入）将 Istio Sidecar 变为 Native Sidecar Container（init container + `restartPolicy: Always`），K8s 对这类容器有明确的退出顺序保证：

> **所有普通业务容器退出之后，sidecar 才收到 SIGTERM。**

```text
t=0   preStop: sleep 5 开始
      Envoy（native sidecar）继续正常运行，不参与这一阶段

t=5   app 收到 SIGTERM，开始优雅退出
      ✓ pulsarConsumer.Close() 成功 ack（Envoy 还活着，网络正常）
      ✓ 所有收尾工作在有网络的情况下完成

t=10  app 容器退出
      Envoy 收到 SIGTERM，开始退出（最后退出）
```

配置就加一行 annotation：

```yaml
# rollout.yaml
annotations:
  sidecar.istio.io/inject: "true"
  sidecar.istio.io/nativeSidecar: "true"   # ← 这一行保证 Envoy 最后退出
```

加了之后 Pulsar 消费失败的情况彻底消失。本质原因是：Sidecar 模式下所有网络都经过 Envoy，优雅退出期间必须保证 Envoy 比业务进程活得更长，否则优雅退出是假的。

**坑 3：金丝雀发布时忘记 promote，线上长期有两个版本**

Rollout 的第 2 步是无限 pause，等人工确认再 promote。有一次发布后工程师忘了 promote，结果线上 15% 流量跑新版本、85% 跑老版本，持续了一天，造成部分接口行为不一致。

- 解决：在 CI/CD 流水线里加告警，Rollout 处于 Paused 状态超过 2 小时就发钉钉通知，提醒工程师确认

**坑 4：CronHPA 和 HPA 冲突**

CronHPA 把 minReplicas 改成 7，但 HPA 在没有压力时会按照配置文件里的 minReplicas=4 把副本缩回 4。

- 解决：CronHPA 是通过 K8s API 直接修改 HPA 资源的 minReplicas 字段，不是修改 hpa.yaml 文件。所以 CronHPA scale down Job 在 13:00 要把 HPA 的 minReplicas 改回 4，两个 Job 配合才能正确运行

---

## 六、扩容和缩容能做到无损么？

### 11. 扩容能做到无损么？

**扩容基本无损，但有两个隐患。**

扩容天然比缩容安全：老 Pod 一直在跑，新 Pod 只有通过 readinessProbe 才会被加入 Endpoint 列表，才会接收流量，所以不存在"流量打到还没准备好的 Pod"的问题。

**隐患一：readinessProbe 用的是 TCP 端口探测，不等于应用真正就绪**

checkout 的 readinessProbe 配置：

```yaml
readinessProbe:
  initialDelaySeconds: 8   # 启动后等 8s 再开始探测
  periodSeconds: 5         # 每 5s 探测一次
  failureThreshold: 5      # 连续失败 5 次才标记为 NotReady
  tcpSocket:
    port: grpc             # 只探测 gRPC 端口（8023）是否可以建立 TCP 连接
```

checkout 的启动顺序（来自 `main.go`）：

```text
connectDB()               → 建立 DB 连接
InitProvider()            → 初始化所有 gRPC 连接池（tpw/payment/subsidy 等）
client.InitFromDBData()   → 从 DB 加载客户端配置到内存
mch.InitFromDBData()      → 从 DB 加载商户配置到内存
buildServers()            → 构建 gRPC / HTTP Server
application.Run()         → 启动监听，端口打开  ← readinessProbe 这时才能通过
```

看起来没问题，因为端口打开时初始化已经全部完成。

**但真正的隐患在连接池**：InitProvider 里建了 gRPC 连接池（`InitConn` 个初始连接），DB 连接池也是惰性建立的（GORM 默认第一次请求才真正建连）。readinessProbe 通过时连接池可能还没有热身完，头几个打进来的请求需要等连接建立，P99 延迟会出现一个短暂的尖峰。

这不是"丢请求"，但会有延迟毛刺。流量越大越容易触发（突然涌入新 Pod 的请求都要抢着建连）。

**隐患二：Istio Sidecar 的就绪时机**

Pod 有 Istio Sidecar 注入时（`sidecar.istio.io/inject: "true"`），Sidecar 需要连上 istiod 拿到完整的 xDS 配置才算就绪。如果 Sidecar 还没 ready 就开始接流量，入站请求会失败。

新版本的 Istio（包括 checkout 用的 nativeSidecar 模式）已经处理了这个问题——Sidecar 作为 init container 先于业务容器启动，但老版本要靠 `holdApplicationUntilProxyStarts` 配置。

---

### 12. 缩容能做到无损么？

**缩容远比扩容复杂，是最容易丢请求的环节。**

先看不加任何保护措施，K8s 缩容时会发生什么：

```text
t=0ms  K8s 决定终止这个 Pod
t=0ms  Pod 被从 Endpoints 列表里移除（EndpointSlice controller 更新）
t=0ms  同时：向容器发送 SIGTERM

t=Xms  kube-proxy 在每个节点上更新 iptables 规则（异步，可能延迟 1~5s）
t=Yms  Istio：istiod 感知到 Endpoint 变化，把新配置推给所有 Envoy Sidecar（异步，可能延迟 1~3s）

---在 t=0 到 t=max(X,Y) 这个窗口里：---
  - kube-proxy/Istio 还没更新完，新请求仍然可能被路由到这个 Pod
  - 但 Pod 收到 SIGTERM，应用可能已经开始停止监听
  → 请求发过来，连接被拒绝或直接 reset → 报错
```

**核心矛盾：Endpoint 移除和 iptables/Envoy 更新是异步的，中间有一段时间差，新请求还会打进来，但应用已经开始停止。**

---

### 13. preStop sleep 是怎么解决这个问题的？

`preStop` hook 的关键作用是**在应用收到 SIGTERM 之前插入一段等待**，让路由更新先完成。

```text
t=0ms  K8s 决定终止 Pod
t=0ms  Pod 从 Endpoints 移除（立刻）
t=0ms  preStop hook 开始执行：sleep 5

t=0 ~ t=5s 这 5 秒里：
  ✓ kube-proxy 在各节点更新 iptables（通常 < 1s）
  ✓ istiod 把新 Endpoint 配置推给所有 Sidecar（通常 1~3s）
  ✓ 应用还在正常运行，在途请求正常处理，新进来的少量请求也正常响应

t=5s   preStop 完成，SIGTERM 才真正交给应用进程

t=5s ~ t=10s  应用优雅退出：
  - gRPC Server 停止接收新连接，等待在途 RPC 完成
  - HTTP Server 类似处理
  - Pulsar consumer 停止消费（pulsarConsumer.Close()）
  - DB/Redis 连接关闭（cleanupFunc()）

t=10s  terminationGracePeriodSeconds 到达，Pod 被强制 kill
```

`terminationGracePeriodSeconds: 10`，其中 preStop 占 5s，留给应用的优雅退出时间只有 5s。如果在途请求处理时间超过 5s，依然会被强制 kill。

---

### 14. 有没有情况 preStop sleep 5 也不够？

**有，三种情况下仍然可能有损：**

**情况一：Istio 控制平面压力大，xDS 推送延迟超过 5s**

istiod 需要向集群里所有 Sidecar 推送配置，如果集群规模大或 istiod 本身压力大，推送延迟可能超过 5s，这段超出时间里新请求仍然会路由到正在停止的 Pod。

5s 是根据我们集群的实际 xDS P99 延迟估算的，正常情况够用。

**情况二：gRPC 长连接的 GOAWAY 处理**

HTTP/1.1 是短连接，请求完就断，没什么问题。gRPC 是长连接（HTTP/2 多路复用），一条连接上可能有多个并发 RPC。

停止流程：
- Envoy Sidecar 感知到 Pod 要退出，向客户端（其他服务的 Envoy）发 `HTTP/2 GOAWAY` 帧
- 客户端收到 GOAWAY 后，不在这条连接上发新的 RPC，已有的 RPC 继续跑完
- 客户端重新建连到其他 Pod

这个过程不会丢请求，但如果客户端没有正确处理 GOAWAY（比如某些老版本客户端直接报错），就会有问题。checkout 的客户端都是走 Envoy Sidecar，Envoy 会正确处理，这块没问题。

**情况三：应用优雅退出时间不足**

checkout 留给应用退出的时间只有 5s（10s - preStop 5s）。如果某个请求的处理时间本身就超过 5s（比如某个慢查询），这个请求会被强制 kill。

VirtualService 设了 `timeout: 10s`，正常请求不会跑 5s 以上，这块一般没问题。

---

### 15. 完整时序图

```text
缩容触发
   │
   ├─ [t=0]  Endpoint 从 EndpointSlice 移除
   │
   ├─ [t=0]  preStop 开始：sleep 5
   │         │
   │         ├─ [t≈0~1s]  kube-proxy 更新各节点 iptables
   │         │             → 普通 HTTP 流量不再路由到此 Pod
   │         │
   │         └─ [t≈1~3s]  istiod → Envoy xDS 推送完成
   │                       → gRPC 流量不再路由到此 Pod
   │                       → Envoy 对旧连接发 GOAWAY
   │
   ├─ [t=5]  preStop 完成，SIGTERM → 应用进程
   │         │
   │         ├─ gRPC Server：停止 Accept 新连接，等待在途 RPC 完成
   │         ├─ HTTP Server：停止 Accept，等待在途请求完成
   │         ├─ Pulsar Consumer：停止拉消息（pulsarConsumer.Close()）
   │         └─ 关闭 DB/Redis 连接（cleanupFunc()）
   │
   └─ [t=10] terminationGracePeriodSeconds：强制 kill（兜底）
```

**结论：不能做到 100% 无损，但可以做到 99.99% 无损。**

preStop sleep 覆盖了绝大部分情况，剩余的风险点（控制平面压力过大、应用退出超时）需要通过监控来发现，实际出现的概率很低。

---

### 16. PDB 是怎么配合保证缩容安全的？

```yaml
# pdb.yaml
spec:
  minAvailable: 2
```

PDB 是另一个维度的保护：**防止 K8s 一次性缩掉太多 Pod**。

checkout 正常运行 4 个 Pod，PDB 要求最少 2 个可用。如果 K8s 要做节点维护（`kubectl drain`），同时驱逐多个 checkout Pod，PDB 会阻止它——只允许一次驱逐一个，等被驱逐的 Pod 的流量被其他 Pod 接管了，再继续驱逐下一个。

没有 PDB 的情况下，节点维护可能同时驱逐所有 4 个 Pod（都在一个节点上），服务瞬间完全不可用。

结合 topologySpreadConstraints（多 AZ 打散），一个节点上最多 1~2 个 Pod，PDB 再保证最少 2 个可用，两者叠加基本能保证节点维护时服务不中断。

---

## 七、面试答题技巧

这块面试核心考三个点：

1. **Istio 核心概念**：VirtualService（路由规则）、DestinationRule（负载均衡和熔断）、Sidecar（出口控制）、ServiceEntry（外部服务注册），说清楚每个是干什么的就行

2. **灰度发布整体流程**：Argo Rollout + Istio VirtualService weight 联动，手动 pause 卡点，出问题 abort 回滚，这套能完整说出来是很大加分项

3. **结合实际说踩坑**：外部依赖忘更新白名单、preStop 防流量丢失、CronHPA 和 HPA 的关系，这些细节说明你真的在生产环境用过，不是背的理论

---

## 附：资源清单速查

| 文件 | 资源类型 | 作用 |
|------|---------|------|
| `rollout.yaml` | Argo Rollout | 定义金丝雀发布步骤和 Pod spec |
| `istio-virtual-service.yaml` | VirtualService | 路由规则（超时/重试/流量权重） |
| `istio-destination-rule.yaml` | DestinationRule | stable/canary 子集定义和负载均衡策略 |
| `sidecar.yaml` | Sidecar | 出口白名单（REGISTRY_ONLY） |
| `serviceentry.yaml` | ServiceEntry | 注册外部服务（RDS/Redis/外部 API） |
| `hpa.yaml` | HPA | 基于 CPU 的自动扩缩容 |
| `cron-hpa.yaml` | CronJob | 定时调整 HPA 副本数（午高峰预扩容） |
| `pdb.yaml` | PodDisruptionBudget | 最少保证 2 个 Pod 可用 |
| `service.yaml` | Service | checkout / checkout-stable / checkout-canary 三个 Service |
| `kong-rate-limiting.yaml` | KongPlugin | 入口限流 200 req/s per consumer |
