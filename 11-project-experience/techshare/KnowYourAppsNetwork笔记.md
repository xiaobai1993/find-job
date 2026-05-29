# Know Your App's Network 技术分享笔记

> 美餐内部技术分享，主题是 K8s 应用网络优化，覆盖 DNS、Istio、Kong 三大领域共 9 个优化点。

---

## 一、DNS in Kubernetes

### K8s 内部 DNS 地址格式

```
<service>.<namespace>.svc.cluster.local
```

**最佳实践**：集群内通信优先使用完整 FQDN（Fully Qualified Domain Name），而不是短名称。

### ndots 问题

K8s 默认 DNS 配置：`ndots: 5`

**问题**：当域名中的点数 < 5 时，resolver 会先追加 search domain 进行查询，失败后才查真正的域名。

```
查询 "redis"：
  1. redis.namespace.svc.cluster.local → 找到，返回
  查询 "google.com"（2个点）：
  1. google.com.namespace.svc.cluster.local → 失败
  2. google.com.svc.cluster.local → 失败
  3. google.com.cluster.local → 失败
  4. google.com. → 成功（第4次才查到！）
```

**优化方案**：使用 FQDN（末尾加点）或调整 ndots：

```yaml
dnsConfig:
  options:
    - name: ndots
      value: "2"
```

或者直接用完整地址：`redis.namespace.svc.cluster.local.`（末尾的点表示绝对路径，跳过 search domain 追加）

---

## 二、Istio 机制

### 核心架构

```
Pod 内部：
  App Container ←→ Envoy Sidecar（透明代理）
                         ↕
              iptables 劫持所有入出流量
```

Envoy 作为 sidecar 注入到每个 Pod，通过 iptables 规则劫持所有流量（inbound + outbound），实现：
- 流量管理（路由、负载均衡）
- 可观测性（指标、链路追踪）
- 安全（mTLS）

### VirtualService（流量路由规则）

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: my-service
spec:
  hosts:
    - my-service
  http:
    - route:
        - destination:
            host: my-service
            subset: stable
          weight: 90
        - destination:
            host: my-service
            subset: canary
          weight: 10
```

VirtualService 定义**如何路由流量**：金丝雀、A/B 测试、按 Header 路由等。

### DestinationRule（目标负载均衡策略）

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: my-service
spec:
  host: my-service
  subsets:
    - name: stable
      labels:
        version: stable
    - name: canary
      labels:
        version: canary
  trafficPolicy:
    connectionPool:
      http:
        http1MaxPendingRequests: 100
    outlierDetection:
      consecutiveErrors: 5
      ejectionSweepInterval: 10s
```

DestinationRule 定义**目标子集（subset）和连接策略**：熔断、连接池、健康检查等。

---

## 三、Kong 网关优化

美餐使用 Kong 作为 API Gateway，以下是生产中遇到的问题和优化方案。

### 背景：Kong 的流量路径

```
外部请求 → Kong (Ingress Controller) → Service (Cluster IP) → Pod
```

Kong 通过 K8s Ingress 资源配置路由规则。

---

## 四、Optimization #1~#5（早期优化）

（覆盖 DNS 配置优化、Kong 基础配置等，见上述 DNS 和基础配置章节）

---

## 五、Optimization #6：Rolling Update with Kong — service-upstream

### 问题背景

默认模式下，Kong 直接记录 Pod IP。Rolling Update 时：
1. 旧 Pod 被删除
2. Kong 的 IP 列表**更新有延迟**
3. 延迟期间请求打到已下线的 Pod → 502/503

### 解决方案：开启 service-upstream

```yaml
annotations:
  ingress.kubernetes.io/service-upstream: "true"
```

**工作原理**：

```
默认模式（Kong 直连 Pod IP）：
Kong → Pod IP（直连，Kong 自己维护 IP 列表）

service-upstream 模式：
Kong → Service Cluster IP → Node IP Tables → Pod IP
                                ↑
                         kube-proxy 维护 iptables 规则
```

IPtables 规则示例：

```bash
-s 172.24.159.105/32 -j KUBE-MARK-MASQ
-p tcp -m tcp -j DNAT --to-destination 172.24.159.105:8080
```

**为什么更好**：
- kube-proxy 是 K8s 原生组件，在每个节点上都有**专用监听器**监听 IP 变化
- 具有**最高资源优先级**
- IP 变更延迟更小，Rolling Update 期间丢包更少

### 效果对比

```
默认 IP 模式：
  Socket errors: connect 0, read 0, write 0, timeout 2
  Non-2xx or 3xx responses: 2   ← 有错误！

Enable upstream-service：
  Socket errors: connect 0, read 0, write 0, timeout 2
  Non-2xx or 3xx responses: 0   ← 无 HTTP 错误！
```

### 代价

- **性能下降约 20%**
  - 多一层 IPtables 转发开销
  - 生产集群 IPtables 规则可达 **10k+**，匹配耗时增加
  - 占用 Node 资源

> **权衡**：如果服务对可用性要求高（不允许部署期间丢包），值得牺牲 20% 性能。

---

## 六、Optimization #7：Pod Pre-stop Hook

### 问题

Pod 进入 Terminating 状态后：
1. Kong 发现 Pod Terminating → 从 IP 列表中移除该 Pod
2. 但此时 Pod 可能**仍在处理中的请求**

如果 Pod 被直接 kill，正在处理的请求直接中断 → 请求失败。

### 解决方案：Pre-stop Hook + sleep

```yaml
lifecycle:
  preStop:
    exec:
      command:
        - "/bin/sh"
        - "-c"
        - |
          sleep 15
```

**逻辑**：
1. Pod 收到 SIGTERM（进入 Terminating）
2. **先执行 preStop hook**（sleep 15s）
3. 在这 15 秒内：Kong 把 Pod 从路由中摘除，不再发新请求
4. 15 秒后 Pod 开始真正关闭，此时已无新请求进来
5. 等待 gracefulShutdown 处理完存量请求后退出

> pre-stop hook 和 `terminationGracePeriodSeconds` 配合使用，保证零丢包下线。

---

## 七、Optimization #7：Kong Retry

### 问题

Rolling Update 过程中，即使有 pre-stop hook，也可能存在极少量请求打到刚下线的 Pod。

### 解决方案：Kong 重试

在 Service 上加 annotation：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: server-service
  annotations:
    konghq.com/retries: "5"
spec:
  selector:
    app: test-server
  ports:
    - port: 8080
      targetPort: 8080
      name: http
```

**Kong 默认重试条件**：
- Connect timeout（连接超时）
- 5xx 响应

设置 `retries: 5` 后，Kong 最多重试 5 次，大幅降低因 Pod 重启导致的用户侧错误。

### 效果

Grafana 图表对比：
- 优化前：5xx 错误在部署期间出现明显峰值（300+ 次）
- 优化后：5xx 错误显著下降（<90 次）

---

## 八、Optimization #9：Kong Canary Deployment

### 问题：错误的金丝雀配置

最初配置：Ingress → Service（`stage: all`）

这意味着 Service 的 selector 会同时选中 stable pod 和 canary pod，**流量随机分发**，无法精确控制。

### 正确方案

```
Kong
 ├── Ingress (普通流量) → Stable Service → Stable Pod
 └── Ingress w/ header  → Canary Service  → Canary Pod
```

**两个 Ingress 资源**：

```yaml
# Ingress 1：正常流量 → stable 服务
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: server-service1-kong1
  annotations:
    kubernetes.io/ingress.class: "kong1"
    konghq.com/protocols: "http"
spec:
  rules:
    - host: server-service.testing.ntrnl-eks-fan.meican
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: server-service1   # ← stable 服务
                port:
                  name: http
```

```yaml
# Ingress 2：带特定 Header → canary 服务
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: server-service2-kong1
  annotations:
    kubernetes.io/ingress.class: "kong1"
    konghq.com/protocols: "http"
    konghq.com/headers.version: v2   # ← 关键：header 匹配
spec:
  rules:
    - host: server-service.testing.ntrnl-eks-fan.meican
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: server-service2   # ← canary 服务
                port:
                  name: http
```

**使用方式**：

```bash
# 正常流量 → stable
curl http://server-service.testing.ntrnl-eks-fan.meican:80/call

# 测试金丝雀
curl -H 'version: v2' http://server-service.testing.ntrnl-eks-fan.meican:80/call
```

### 局限性

- 需要手动操作（维护两套 Ingress）
- **与 ArgoCD Rollout 不兼容**（ArgoCD Rollout 有自己的金丝雀管理机制）
- 当前方案是临时 workaround，**等待 GatewayAPI 实现**后会有更优雅的方案

---

## 九、整体 Takaways

| 领域 | 优化点 | 核心要点 |
|------|--------|---------|
| DNS | 使用 FQDN | 优先 `.svc.cluster.local`，避免短名称 |
| DNS | ndots | 注意 ndots=5 导致多余 DNS 查询，酌情降低 |
| Istio | Envoy 劫持流量 | iptables 拦截，VirtualService + DestinationRule |
| Kong | service-upstream | 开启后走 kube-proxy IPtables，减少 Rolling Update 丢包；代价 -20% 性能 |
| Kong | Pre-stop hook | `sleep 15` 给 Kong 时间摘除 Pod，再处理存量请求 |
| Kong | Retry | `konghq.com/retries: "5"`，自动重试 connect timeout 和 5xx |
| Kong | Canary | 双 Ingress + header 路由，手动金丝雀；等待 GatewayAPI |

---

## 十、面试可说的点

1. **ndots 坑**：K8s 默认 `ndots=5`，查 `google.com` 这种短域名会先查 5 次 search domain 失败后才命中真实域名，导致延迟和 DNS 压力。优化方式：FQDN 或降低 ndots。

2. **service-upstream 原理**：默认 Kong 直连 Pod IP，IP 变更时有同步延迟。开启 `service-upstream: "true"` 后，Kong 通过 kube-proxy 维护的 IPtables 转发，kube-proxy 有专用监听器最高优先级，IP 变更更及时，Rolling Update 丢包减少。

3. **Pre-stop hook 实战**：Pod Terminating 后 Kong 立刻摘除 Pod，但 Pod 可能还有存量连接。通过 `preStop: sleep 15` 让 Pod 在停止前等待 15 秒，让 Kong 完成摘除再关闭，实现零丢包滚动更新。

4. **Kong Retry 场景**：配置 `konghq.com/retries: "5"`，Kong 在 connect timeout 或 5xx 时自动重试，掩盖 Rolling Update 期间极少量的连接失败。

5. **Istio vs Kong**：两者都是流量治理，Istio 通过 sidecar 在 Pod 内做，适合东西向（服务间）流量；Kong 是 API Gateway，适合南北向（外部进入集群）流量。金丝雀部署在 Istio 用 VirtualService weight 实现，在 Kong 用双 Ingress + header 实现。

6. **GatewayAPI**：K8s 新一代网络 API，统一替代 Ingress，Kong/Istio 等都在迁移，能更好地支持金丝雀等高级路由。

