# 美餐Payment生产ArgoCD配置 - 面试专版

> 基于 `/Users/cat/Desktop/meican-project/argocd-prod/meican-pay/payment/` 真实生产配置整理。面试时不要只讲 ArgoCD 概念，要把“支付系统怎么在生产 K8s 上做发布、灰度、扩缩容、限流、监控、权限和网络治理”讲出来，并且带具体数据。

---

## 一、项目背景怎么介绍？

### Q1：你们 Payment 服务在生产环境是怎么通过 ArgoCD 管理的？

**答：**

我们美餐 Payment 服务的生产部署是通过 **GitOps + ArgoCD** 管理的，生产配置单独放在 `argocd-prod` 仓库下，路径是：

```text
meican-pay/payment
```

这个目录里不是只有一个 Deployment，而是一整套支付服务生产运行配置，包括：

| 配置类型 | 文件 | 作用 |
|---|---|---|
| Argo Rollouts | `rollout.yaml` | 支付服务主工作负载，负责金丝雀发布 |
| HPA | `hpa.yaml` | 基于 CPU 自动扩缩容 |
| Cron HPA | `cron-hpa.yaml` | 午高峰前后定时扩缩容 |
| Service | `service.yaml` | 普通、stable、canary、metrics 四类 Service |
| Istio VirtualService | `istio-virtual-service.yaml` | stable/canary 流量比例控制 |
| Istio DestinationRule | `istio-destination-rule.yaml` | 子集和负载均衡策略 |
| Kong Ingress | `ingress-kong1.yaml` / `ingress-kong2.yaml` | 双 Kong 入口暴露 gRPC 服务 |
| KongPlugin | `kong-rate-limiting.yaml` | 网关层限流，300 QPS |
| ServiceMonitor | `service-monitor.yaml` | Prometheus 指标采集 |
| PrometheusRule | `alert.yaml` | QPS 和 DLQ 告警 |
| PagerDuty | `pagerduty.yaml` | OnCall 通知规则 |
| WebApp CRD | `webapp.yaml` | 美餐内部 CRD，声明 MQ Topic、权限和项目元信息 |
| ServiceEntry/Sidecar | `serviceentry.yaml` / `sidecar.yaml` | Istio 出站访问白名单 |
| ServiceAccount | `service-account.yaml` | EKS IAM Role 绑定 |
| TieredApplication | `tiered-application.yaml` | 服务分级、Spot 策略、拓扑分散策略 |

这个配置体现的是一个生产级支付服务的完整治理体系：

```text
GitLab 配置仓库
  ↓
ArgoCD Application: payment
  ↓
Argo Rollouts 负责灰度发布
  ↓
Istio 控制 stable/canary 流量
  ↓
Kong 负责入口和限流
  ↓
Prometheus + PagerDuty 负责监控告警
  ↓
Sidecar + ServiceEntry 控制外部访问
```

---

## 二、生产部署数据怎么讲？

### Q2：Payment 生产服务的核心运行参数是什么？

**答：**

从 `rollout.yaml` 看，Payment 生产服务的核心配置是：

| 参数 | 配置值 | 说明 |
|---|---:|---|
| 工作负载类型 | `Rollout` | 使用 Argo Rollouts，不是普通 Deployment |
| 默认副本数 | `4` | 基础容量 4 个 Pod |
| 历史版本保留 | `2` | `revisionHistoryLimit: 2`，保留最近两个版本便于回滚 |
| 容器端口 | `8023` | gRPC 服务端口 |
| 指标端口 | `18888` | Prometheus telemetry 指标端口 |
| CPU request | `500m` | 每个 Pod 保底 0.5 核 |
| CPU limit | `1000m` | 每个 Pod 最多 1 核 |
| Memory request | `512Mi` | 每个 Pod 保底 512Mi |
| Memory limit | `896Mi` | 每个 Pod 最多 896Mi |
| readiness 初始延迟 | `8s` | 启动 8 秒后开始探测 |
| readiness 周期 | `5s` | 每 5 秒探测一次 |
| readiness 失败阈值 | `5` | 连续失败 5 次才摘流量 |
| terminationGracePeriod | `10s` | 优雅退出窗口 10 秒 |
| preStop | `sleep 5` | Pod 下线前先等 5 秒，让网关和服务发现摘流量 |
| 节点架构 | `arm64` | 调度到 ARM 节点 |
| 节点池 | `intend: app1` | 调度到应用节点池 |
| ServiceAccount | `payment-sa` | 绑定 AWS IAM Role |

**面试表达：**

> 我们 Payment 生产不是简单 Deployment，而是 Argo Rollouts。基础副本是 4 个，HPA 可以扩到 15 个。每个 Pod request 500m CPU、512Mi 内存，limit 是 1 核、896Mi。服务暴露 gRPC 8023，指标暴露 18888。下线的时候通过 preStop sleep 5 秒 + 10 秒 terminationGracePeriod 做优雅退出，避免正在处理支付请求时被强杀。

---

### Q3：为什么支付服务要用 Argo Rollouts，而不是普通 Deployment？

**答：**

因为支付系统是资金核心链路，不能接受“一次性全量发布”。普通 Deployment 虽然支持滚动发布，但是它的流量控制比较粗，不能精确控制 stable 和 canary 的流量比例。

我们用 **Argo Rollouts + Istio** 做金丝雀发布，配置里有：

```yaml
strategy:
  canary:
    stableService: payment-stable
    canaryService: payment-canary
    maxSurge: 25%
    maxUnavailable: 0
```

关键点：

| 配置 | 生产值 | 作用 |
|---|---:|---|
| `maxUnavailable` | `0` | 发布过程中不允许减少可用 Pod，保障支付服务不断流 |
| `maxSurge` | `25%` | 允许额外启动 25% 新 Pod，提高发布安全性 |
| `stableService` | `payment-stable` | 老版本流量入口 |
| `canaryService` | `payment-canary` | 新版本流量入口 |
| `VirtualService` | `payment-vsvc` | 由 Istio 控制 stable/canary 权重 |
| `route` | `primary` | Rollouts 控制的主路由 |

灰度步骤是：

```text
0% → 暂停 8s → 15% → 人工暂停 → 40% → 暂停 40s → 60% → 暂停 20s → 80% → 暂停 20s → 100%
```

**面试重点：**

> 这里最关键的是 15% 后面有一个人工 pause，这说明核心支付服务不会自动一路放量到 100%。新版本先接 15% 真实生产流量，观察支付成功率、错误率、P99、DLQ 有没有异常，确认没问题后再继续放到 40%、60%、80%，最后全量。这个比普通滚动发布安全很多。

---

## 三、HPA 和容量治理

### Q4：Payment 生产服务是怎么自动扩缩容的？

**答：**

Payment 用的是 K8s HPA v2，扩缩容目标是 Argo Rollout，不是 Deployment：

```yaml
scaleTargetRef:
  apiVersion: argoproj.io/v1alpha1
  kind: Rollout
  name: payment
```

核心 HPA 数据：

| 参数 | 配置值 | 说明 |
|---|---:|---|
| 最小副本数 | `4` | 平峰最低 4 个 Pod |
| 最大副本数 | `15` | 峰值最多扩到 15 个 Pod |
| 指标类型 | `ContainerResource cpu` | 按 payment-image 容器 CPU 扩容 |
| 扩容阈值 | `500m` | 单容器平均 CPU 达到 500m 触发扩容 |
| 扩容稳定窗口 | `0s` | 高峰来时立即扩容 |
| 扩容策略 1 | `100% / 5s` | 5 秒内最多翻倍 |
| 扩容策略 2 | `2 Pods / 5s` | 5 秒内最多加 2 个 Pod |
| 扩容选择 | `Min` | 两个策略取更保守的，防止扩太猛 |
| 缩容稳定窗口 | `300s` | 5 分钟稳定后才缩容 |
| 缩容策略 | `10% / 60s` | 每分钟最多缩 10%，避免抖动 |

**为什么这么设计？**

- 支付流量高峰来得快，所以 **扩容要快**，`stabilizationWindowSeconds: 0`
- 但支付服务依赖数据库、Redis、Pulsar，扩容太快会给下游造成瞬时压力，所以扩容选择 `Min`，最多每 5 秒增加 2 个 Pod
- 缩容要慢，避免午高峰流量抖动时频繁上下线，所以缩容稳定窗口是 300 秒，每分钟最多缩 10%

---

### Q5：为什么除了 HPA，还要做 Cron HPA？

**答：**

支付系统的流量不是完全随机的，美餐业务有明显的 **工作日午餐高峰**。如果只靠 HPA，流量已经打上来之后再扩容，Pod 启动、readiness、缓存预热都需要时间，容易在峰值开始时出现延迟升高。

所以我们额外做了 **定时扩容 CronJob**：

| 时间 | 动作 | 配置 |
|---|---|---|
| 周一到周五 11:20 | 高峰前扩容 | 把 HPA minReplicas 调到 `8`，maxReplicas 维持 `15` |
| 周一到周五 13:00 | 高峰后缩容 | 把 HPA minReplicas 调回 `4`，maxReplicas 维持 `15` |
| 时区 | `Asia/Shanghai` | 按中国业务时间执行 |
| Job 失败历史 | `2` | 保留最近 2 个失败记录便于排查 |
| 执行镜像 | `titan/k8s-hpa:v0.2.2` | 内部 HPA 调整工具 |

**面试表达：**

> 这块是我们比较实际的容量治理经验。Payment 平时最小 4 个副本，但工作日午高峰前 11:20 会提前把最小副本数提高到 8 个，13:00 再降回 4 个。这样不是等流量来了才被动扩，而是根据业务规律主动预扩容，避免支付高峰开始的一波流量把 P99 打高。

---

## 四、流量入口、灰度和限流

### Q6：Payment 服务的入口流量是怎么设计的？

**答：**

Payment 是 gRPC 服务，对外通过 Kong Ingress 暴露。配置了两个 Ingress：

| Ingress | ingress.class | 协议 | 后端 Service | 端口 |
|---|---|---|---|---:|
| `payment-grpc-kong1` | `kong1` | `https` | `payment` | `grpc:8023` |
| `payment-grpc-kong2` | `kong2` | `https` | `payment` | `grpc:8023` |

两个入口都挂了同一个 Kong 限流插件：

```yaml
konghq.com/plugins: payment-rate-limiting-kong
```

**为什么要两个 Kong 入口？**

可以理解成入口层冗余。支付是核心服务，不能把所有入口流量压在单一 Kong 控制面/入口上，两个 Ingress class 可以提升入口层可用性，也方便入口迁移和切流。

---

### Q7：Payment 网关限流是怎么配置的？限流阈值是多少？

**答：**

通过 KongPlugin 做入口限流：

| 参数 | 配置值 | 说明 |
|---|---:|---|
| 插件 | `rate-limiting` | Kong 官方限流插件 |
| 每秒限流 | `300` | `second: 300` |
| 限流维度 | `consumer` | 按消费者维度限流 |
| 存储策略 | `redis` | 分布式限流，多个 Kong 实例共享计数 |
| Redis 超时 | `1000ms` | 限流 Redis 超时时间 |
| fault_tolerant | `true` | Redis 异常时允许请求通过，避免限流组件故障拖垮支付 |

**面试重点：**

> 支付系统限流不能只考虑“挡住流量”，还要考虑限流组件本身的可用性。这里 `fault_tolerant: true` 很关键，说明如果 Kong 限流 Redis 短暂不可用，不会直接把支付流量全拒掉，而是让请求先过，避免限流系统成为单点故障。当然下游服务内部还有 HPA、熔断、业务限流兜底。

---

### Q8：Istio 在这个配置里起什么作用？

**答：**

Istio 主要做三件事：

1. **灰度流量控制**
   - `VirtualService` 初始配置 stable 100%，canary 0%
   - Argo Rollouts 根据步骤修改权重，实现 15%、40%、60%、80% 放量

2. **请求超时和重试**
   - `timeout: 10s`
   - `retries.attempts: 3`
   - `retryOn: unavailable`

3. **负载均衡策略**
   - stable 和 canary 的 `DestinationRule` 都配置了 `ROUND_ROBIN`

**面试表达：**

> Payment 的灰度不是靠改 Service Selector，而是 Argo Rollouts 控制 Istio VirtualService 的 stable/canary 权重。这样可以在服务网格层精确控制流量比例，并且配合 10 秒超时、最多 3 次 unavailable 重试，降低短暂实例异常对支付请求的影响。

---

## 五、MQ 和异步任务配置

### Q9：Payment 服务配置了多少 MQ Topic？说明了什么？

**答：**

从 `webapp.yaml` 看，Payment 在生产环境通过内部 WebApp CRD 声明了 **29 个 Pulsar Topic**，总分区数 **62 个 partition**。

这些 Topic 覆盖了支付系统的核心异步场景：

| 场景 | Topic 示例 | 说明 |
|---|---|---|
| 死信和重试 | `meican-pay.payment.prod-DLQ`、`meican-pay.payment.prod-RETRY` | 失败消息兜底，保证可追溯和重试 |
| 支付事件 | `payment-trade-event`、`payment-trade-event-v2` | 支付交易事件广播给 BI、开发者平台、Kafka 源等 |
| 余额事件 | `personal-balance-event` | 和个人余额系统联动 |
| 自动支付 | `auto-pay`、`auto-partial-pay` | 自动支付、部分自动支付异步处理 |
| 支付超时 | `payment-slip-timeout-check`、`payment-slip-timeout-check-v2` | 支付单超时检查，v2 配置了 10 个分区 |
| 退款 | `refund-by-order`、`refund-by-transaction` | 按订单、按交易维度退款 |
| 结果通知 | `web-result-notify`、`quick-result-notify`、`notify-to-checkout` | 通知业务方和结算系统 |
| 回滚补偿 | `payment-tpw-rollback`、`payment-msg-subsidy-rollback`、`payment-msg-meal-point-rollback` | TPW、补贴、餐点账户回滚补偿 |
| 先享后付 | `payment-pay-after-cancel`、`payment-pay-after-complete` | 后付取消和完结 |
| 财务结算 | `payment-to-finance-event` | 5 个分区，给财务结算抽取数据 |

**几个关键数据：**

- 总 Topic 数：**29 个**
- 总分区数：**62 个****
- 最大分区 Topic：`payment-slip-timeout-check-v2`，**10 个分区**
- 财务结算事件：`payment-to-finance-event`，**5 个分区**
- 大多数业务 Topic：**2 个分区**
- DLQ 和 RETRY：各 **1 个分区**

**面试表达：**

> 这个配置能看出 Payment 不是一个简单同步 RPC 服务，而是典型的事件驱动支付系统。支付成功、退款、超时检查、结果通知、账户回滚、财务结算都通过 Pulsar 异步化。生产配置里声明了 29 个 Topic、62 个 partition，其中支付超时检查 v2 单独给了 10 个分区，说明这个场景吞吐压力最大，需要更高并发消费能力。

---

### Q10：为什么支付系统必须有 DLQ 和 RETRY Topic？

**答：**

支付异步任务不能“失败就丢”。比如支付成功通知订单系统失败、退款回滚消息消费失败，如果消息丢了就会导致订单状态不一致或者资损。

所以配置里有：

```text
meican-pay.payment.prod-DLQ
meican-pay.payment.prod-RETRY
```

- **RETRY**：短暂失败进入重试队列，比如下游系统短暂不可用
- **DLQ**：超过最大重试次数仍然失败，进入死信队列，人工或补偿任务兜底处理

并且 `alert.yaml` 里对 DLQ 做了 critical 告警：

```promql
pulsar_rate_in{topic="persistent://public/meican-pay/meican-pay.payment.prod-DLQ-partition-0"} > 0
```

只要有消息进入 DLQ，持续 1 分钟就触发 critical 告警。

**面试表达：**

> 支付系统的 MQ 设计不能只讲削峰填谷，更要讲失败兜底。我们 DLQ 是强告警，只要 payment DLQ 有消息流入，1 分钟就 critical，因为这可能意味着支付通知、退款补偿或账户回滚失败，需要马上处理。

---

## 六、监控告警体系

### Q11：Payment 生产监控是怎么配置的？

**答：**

Payment 暴露了独立的 telemetry Service：

| 配置 | 值 |
|---|---|
| Service | `payment-telemetry` |
| 指标端口 | `18888` |
| 端口名 | `http-telemetry` |
| ServiceMonitor namespace | `monitoring` |
| 被采集 namespace | `app-meican-pay` |
| Prometheus label | `k8s-thanos-2` |
| selector | `app=payment, stage=telemetry` |

Prometheus 通过 ServiceMonitor 抓取 Payment 的 `18888` 指标端口，然后通过 PrometheusRule 配置告警。

---

### Q12：Payment 配了哪些告警？阈值是多少？

**答：**

当前生产配置里有两个核心告警：

#### 1. QPS 高告警

```promql
sum(rate(grpc_server_handling_seconds_count{pod=~"^payment.*", service="payment-telemetry"}[1m])) > 80
```

| 参数 | 值 |
|---|---:|
| 指标 | `grpc_server_handling_seconds_count` |
| 统计窗口 | `1m` |
| 阈值 | `> 80 QPS` |
| 持续时间 | `1m` |
| 级别 | `warning` |

说明 Payment 正常情况下 QPS 超过 80 持续 1 分钟就会 warning，用来识别流量高峰或异常流量。

#### 2. DLQ 消息告警

```promql
pulsar_rate_in{topic="persistent://public/meican-pay/meican-pay.payment.prod-DLQ-partition-0"} > 0
```

| 参数 | 值 |
|---|---:|
| 指标 | `pulsar_rate_in` |
| Topic | Payment DLQ partition-0 |
| 阈值 | `> 0` |
| 持续时间 | `1m` |
| 级别 | `critical` |

说明只要死信队列有消息进入，就是 critical，因为支付异步任务失败不能忽略。

---

### Q13：OnCall 是怎么接入的？

**答：**

生产配置里有 `PagerDuty` CRD：

| 参数 | 值 |
|---|---|
| group | `meican-pay` |
| project | `payment` |
| app label | `payment` |
| Argo 链接 | `payment` Application |
| 通知级别 | `info`、`warn`、`critical` |
| OnCall 规则 | 覆盖周日到周六 |
| 联系人 | `limingming` |

**面试表达：**

> 我们不是只把 Prometheus 告警打到页面上，而是通过内部 PagerDuty CRD 接入 OnCall。Payment 这种资金核心服务，critical 告警要能直接打到值班人，尤其是 DLQ、支付成功率、错误率这类告警，不能只靠人去看大盘。

---

## 七、网络安全和出站治理

### Q14：Payment 为什么要配置 Sidecar 和 ServiceEntry？

**答：**

因为生产环境启用了 Istio，并且 Payment 的 Sidecar 设置了：

```yaml
outboundTrafficPolicy:
  mode: REGISTRY_ONLY
```

这意味着 Payment Pod 不能随便访问外部地址，只能访问 Sidecar 和 ServiceEntry 显式声明过的服务。

**好处：**

1. **最小权限网络访问**：支付服务只能访问白名单里的数据库、Redis、Pulsar、内部服务、AWS 服务
2. **防止误访问**：代码里如果写错外部地址，不会偷偷打出去，会被 mesh 拦掉
3. **便于审计**：Payment 依赖哪些外部资源，配置里一眼能看到
4. **降低安全风险**：即使服务被攻击，也不能随便访问外网

---

### Q15：Payment 配置了哪些外部依赖？

**答：**

从 `ServiceEntry` 看，Payment 至少声明了以下外部依赖端口和类型：

| 端口 | 类型 | 说明 |
|---:|---|---|
| 80/443 | HTTP/HTTPS | AWS 服务、外部 HTTP 服务、内部 HTTP 入口 |
| 5432 | PostgreSQL | `monopoly` 系列 RDS 主库/只读库、新旧集群 |
| 6379 | Redis | Payment Redis、生产 Redis 主从/只读实例 |
| 9123 | 内部服务 | kiwi generator |
| 3306 | MySQL | divide-bill RDS |
| 6650 | Pulsar | Pulsar broker |
| 18889 | keep-alive | 内部 keep-alive ELB |

Sidecar 里同时允许访问：

- `nacos`：配置中心
- `pulsar`：消息队列
- `otel`：链路追踪/可观测性
- `app-meican-pay` 命名空间内服务
- `app-nation-client`、`app-keepalive`、`app-planet`、`app-kiwi` 等内部服务
- AWS 中国区域域名：`cn-northwest-1`、`cn-north-1`

**面试表达：**

> 这个配置能看出 Payment 的外部依赖非常多，包括 PostgreSQL、MySQL、Redis、Pulsar、Nacos、OTel、AWS 服务、内部 RPC 服务。我们通过 Istio REGISTRY_ONLY 把所有出站访问收敛到 ServiceEntry 白名单里，这对支付系统非常重要，因为支付系统不能允许任意外联。

---

### Q16：`sidecar.istio.io/nativeSidecar: "true"` 是什么？为什么 Payment 要开？

**答：**

`sidecar.istio.io/nativeSidecar: "true"` 是 Istio 的注解，用来启用 Kubernetes 原生 Sidecar Container 能力。传统 Istio sidecar 是作为普通 `container` 注入的，和业务容器并列启动、并列退出，启动顺序和退出顺序不够严格。

启用 native sidecar 后，Istio 会把 `istio-proxy` 作为带 `restartPolicy: Always` 的 `initContainer` 注入，生命周期语义变成：

```text
启动：istio-proxy 先启动并 ready → 再启动 payment 业务容器
退出：payment 业务容器先退出 → istio-proxy 最后退出
```

**它主要解决两个问题：**

1. **启动阶段更稳定**
   - Payment 启动时要访问 Nacos、Redis、Pulsar、PostgreSQL、内部 RPC 服务
   - 如果业务容器先于 Envoy ready，就可能出现启动阶段出站连接失败
   - native sidecar 保证 Envoy 先 ready，再启动业务容器

2. **下线阶段更安全**
   - Pod 删除时，如果 Envoy 先退出，业务容器还在处理支付请求，就可能导致 RPC、MQ、数据库访问失败
   - native sidecar 保证业务容器退出后 Envoy 再退出，减少发布、扩缩容、Pod 重启过程中的瞬时失败

**面试表达：**

> Payment 是核心支付服务，出入站流量都经过 Istio，出站访问还配置了 `REGISTRY_ONLY` 白名单。如果 Envoy 没 ready，业务容器提前启动，访问 Nacos、Redis、Pulsar、DB 可能失败；如果下线时 Envoy 先退出，正在处理的支付请求可能失败。所以我们通过 `sidecar.istio.io/nativeSidecar: "true"` 开启 Kubernetes 原生 sidecar，保证 sidecar 先启动、后退出，提高发布、扩缩容、Pod 重启过程中的稳定性。

**补充说明：** 这个能力依赖 Kubernetes 和 Istio 版本支持，生产上一般会先在核心服务或灰度服务验证启动、下线、Rollout 发布流程都没问题，再逐步推广。

---

## 八、服务分级和资源策略

### Q17：Payment 在平台里是怎么做服务分级的？

**答：**

`TieredApplication` 里定义了 Payment 的服务等级和资源策略：

| 参数 | 配置值 | 含义 |
|---|---|---|
| tier | `tier1` | 一级核心服务 |
| spotPreference | `NoSpot` | 不使用 Spot，避免节点被回收影响支付 |
| qos | `Burstable` | K8s QoS 等级 |
| enableInPlaceResize | `true` | 支持原地资源调整 |
| enableTopologySpreadConstraint | `true` | 开启拓扑分散约束 |
| workloadSelector | `Rollout/payment` | 绑定 Payment Rollout |
| git path | `meican-pay/payment/rollout.yaml` | 关键工作负载配置路径 |

**面试表达：**

> Payment 是 tier1 核心服务，不跑 Spot 节点，因为 Spot 可能被云厂商回收，核心支付服务不能为了省成本牺牲稳定性。配置里还启用了 topology spread constraint，Pod 会尽量分散到不同 Node 和不同 Zone，避免单点故障。

---

### Q18：Pod 是怎么做高可用调度的？

**答：**

`rollout.yaml` 里配置了两个 `topologySpreadConstraints`：

| 维度 | 配置 | 作用 |
|---|---|---|
| 主机维度 | `kubernetes.io/hostname` | Pod 尽量分散到不同 Node |
| 可用区维度 | `topology.kubernetes.io/zone` | Pod 尽量分散到不同 Zone |
| maxSkew | `1` | 不同拓扑域之间 Pod 数量最多差 1 |
| whenUnsatisfiable | `ScheduleAnyway` | 资源不足时仍允许调度，优先保证可用性 |

**面试表达：**

> 支付服务至少 4 个副本，如果都调度到同一个节点或者同一个 AZ，节点故障就会影响支付。所以我们配置了 hostname 和 zone 双维度拓扑分散，maxSkew 是 1，尽量让 Pod 分散部署，提升高可用。

---

## 九、面试官高频追问

### Q19：如果 Payment 新版本发布后发现错误率升高，你怎么处理？

**答：**

我会按四步处理：

1. **先看 Rollout 阶段**
   - 如果还在 15%/40% 灰度阶段，先暂停继续放量
   - 如果是人工 pause 阶段，直接不继续推进

2. **切回 stable**
   - 通过 Argo Rollouts abort 或回滚，让 Istio VirtualService 权重回到 stable 100%、canary 0%
   - 这样流量马上回到老版本

3. **确认核心指标恢复**
   - 看 `payment-telemetry` 指标：QPS、错误率、P99
   - 看 DLQ 是否继续进消息
   - 看 Pulsar、Redis、DB 是否被新版本打异常

4. **GitOps 回滚**
   - 如果确认是版本问题，Git revert 镜像 tag 的提交
   - ArgoCD 同步回旧版本，保证 Git 和集群状态一致

**核心原则：先止血，后定位。支付系统出问题不要在生产上边观察边放量，必须先把流量切回稳定版本。**

---

### Q20：为什么 maxUnavailable 要设置成 0？

**答：**

Payment 是核心支付链路，发布时不能因为替换 Pod 导致可用实例减少。`maxUnavailable: 0` 表示滚动过程中不允许减少可用 Pod，必须先启动新 Pod，readiness 通过之后才逐步切流量。

这能保证发布期间容量不下降，尤其配合基础 4 副本和 HPA，发布时不会突然少一个实例，避免支付请求延迟升高。

---

### Q21：为什么需要 preStop sleep 5 秒？

**答：**

K8s 删除 Pod 时，Service Endpoint、Kong、Istio、客户端连接池都需要一点时间感知这个 Pod 不再可用。如果容器收到 SIGTERM 后马上退出，可能还有请求被转发过来，导致请求失败。

所以配置了：

```yaml
preStop:
  exec:
    command: ["sh", "-c", "sleep 5"]
terminationGracePeriodSeconds: 10
```

意思是下线前先等 5 秒，让流量入口和服务发现完成摘流，然后再退出；总的优雅退出窗口是 10 秒。对支付这种 gRPC 服务非常重要，可以降低发布和扩缩容过程中的请求失败。

---

### Q22：HPA 为什么用 averageValue 500m，而不是 CPU utilization 百分比？

**答：**

因为 Payment 容器的 CPU request 是 500m，使用 `averageValue: 500m` 表示当每个容器平均 CPU 使用达到 0.5 核时开始扩容，这个阈值更直观，也不容易因为 request 改动导致 utilization 百分比语义变化。

对核心支付服务来说，我们更关心实际 CPU 消耗达到多少，而不是百分比看起来是多少。

---

### Q23：为什么 Cron HPA 高峰扩到 minReplicas=8，但 maxReplicas 还是 15？

**答：**

`minReplicas=8` 是提前准备基础容量，保证午高峰来之前已经有足够 Pod 在运行；`maxReplicas=15` 是给异常峰值和突发流量留弹性空间。

也就是说：

```text
平峰：4 ~ 15
工作日午高峰：8 ~ 15
```

高峰前不是固定 8 个，而是至少 8 个，如果 CPU 继续高，HPA 仍然能扩到 15 个。这样既避免流量上来时冷启动，又保留自动扩容能力。

---

### Q24：为什么 Payment 的 Service 要拆成 payment、payment-stable、payment-canary、payment-telemetry？

**答：**

四个 Service 职责不同：

| Service | 作用 |
|---|---|
| `payment` | 普通业务访问入口，对外 Ingress 后端 |
| `payment-stable` | Argo Rollouts stable 流量入口 |
| `payment-canary` | Argo Rollouts canary 流量入口 |
| `payment-telemetry` | Prometheus 指标采集入口，端口 18888 |

这样业务流量、灰度流量、监控流量可以分开治理。灰度发布时，Istio 控制 stable/canary；监控采集走 telemetry，不和业务 gRPC 入口混在一起。

---

### Q25：这个配置里有哪些地方体现了支付系统的“资金安全优先”？

**答：**

至少有 8 个点：

1. **Rollout 金丝雀发布**：不是一次性全量，15% 先观察
2. **maxUnavailable=0**：发布过程中不减少可用实例
3. **HPA 4~15**：根据 CPU 自动扩容，防止高峰打垮服务
4. **Cron HPA 11:20 预扩到 8 副本**：按业务规律提前准备容量
5. **Kong 300 QPS 限流**：入口层保护核心服务
6. **DLQ critical 告警**：异步失败不沉默，死信 1 分钟就告警
7. **REGISTRY_ONLY 出站白名单**：防止支付服务任意外联
8. **NoSpot + tier1**：核心服务不跑可回收节点，稳定性优先于成本

---

## 十、可以直接背的项目总结

### Q26：你怎么总结这个 Payment ArgoCD 生产配置？

**答：**

我们 Payment 的生产配置体现的是一套完整的云原生支付服务治理体系。它不是简单地把服务部署到 K8s，而是围绕支付系统的稳定性和资金安全做了很多生产级设计。

从数据上看：

- 部署层：使用 Argo Rollouts，基础 **4 副本**，历史版本保留 **2 个**
- 灰度层：通过 Istio 做金丝雀，流量按 **0% → 15% → 40% → 60% → 80% → 100%** 推进，15% 后有人工 pause
- 容量层：HPA **4~15 副本**，CPU 平均 **500m** 触发扩容；工作日 **11点20** 提前扩到 **8~15**，**13点** 降回 **4~15**
- 网关层：Kong 双入口，gRPC HTTPS，按 consumer 做 **300 QPS** 限流，Redis 分布式计数，限流组件 fault tolerant
- MQ 层：声明 **29 个 Pulsar Topic，总 62 个 partition**，支付超时检查 v2 给了 **10 个分区**，财务事件给了 **5 个分区**
- 监控层：Prometheus 采集 **18888** 端口，QPS 超过 **80** 持续 1 分钟 warning，DLQ 有消息持续 1 分钟 critical
- 网络层：Istio Sidecar 使用 `REGISTRY_ONLY`，所有外部依赖通过 ServiceEntry 白名单声明，包括 PostgreSQL、MySQL、Redis、Pulsar、AWS 服务和内部 RPC 服务
- 稳定性层：tier1、NoSpot、hostname + zone 双维度拓扑分散，nativeSidecar 先启动后退出，preStop 5 秒，terminationGracePeriod 10 秒

> 所以我面试时会说，这套配置说明我们对 Payment 的治理不是只停留在“能跑”，而是覆盖了发布安全、流量治理、容量管理、异步可靠性、监控告警、网络安全、权限隔离和高可用调度。支付系统最重要的是稳定和资金安全，这些配置都是围绕这两个目标做的。

---

## 十一、面试答题技巧

讲这段经历时，建议按这个顺序：

1. **先讲整体架构**：GitOps + ArgoCD + Argo Rollouts + Istio + Kong + Prometheus
2. **再讲数据**：4~15 副本、11点20 扩到 8、300 QPS 限流、29 Topic/62 partition、DLQ critical
3. **再讲为什么这样设计**：支付系统不能全量发布、不能任意外联、不能消息失败沉默、不能 Spot 被回收影响
4. **最后讲故障处理**：灰度异常先 abort，流量回 stable，Git revert 回滚，监控确认恢复

只要你把这些具体数据讲出来，面试官会明显感觉这是你真实做过的生产项目，不是背 ArgoCD 概念。
