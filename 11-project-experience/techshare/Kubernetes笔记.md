# Kubernetes 技术分享笔记

> 来源：Meican 内部技术分享 - Container Orchestration: Kubernetes in MEICAN

---

## 目录

1. [Kubernetes 概览](#1-kubernetes-概览)
2. [Control Plane 控制面](#2-control-plane-控制面)
3. [Workload 工作负载](#3-workload-工作负载)
4. [Service Discovery 服务发现](#4-service-discovery-服务发现)
5. [Meican EKS 架构](#5-meican-eks-架构)
6. [Kong Ingress](#6-kong-ingress)
7. [Service Mesh - Istio](#7-service-mesh---istio)
8. [Pod 详解](#8-pod-详解)
9. [ArgoCD - 部署控制器](#9-argocd---部署控制器)
10. [可观测性体系](#10-可观测性体系)
11. [AWS IAM 集成](#11-aws-iam-集成)
12. [Tower - 内部平台](#12-tower---内部平台)

---

## 1. Kubernetes 概览

**Kubernetes** = 希腊语 "κυβερνήτης"（舵手/领航员）

### 核心组件总览

```
Kubernetes Cluster
├── Control Plane（控制面）
│   ├── kube-apiserver   # 对外暴露的 REST API 入口
│   ├── etcd             # 持久化存储（键值数据库）
│   ├── kube-scheduler   # 调度决策
│   ├── controller-manager # Watch & control
│   └── cloud-controller-manager # 与云厂商交互（可选）
└── Worker Nodes
    ├── kubelet          # 节点 agent，负责 Pod 生命周期
    ├── kube-proxy       # 网络代理（proxy 或 no-proxy 模式）
    └── CRI              # 容器运行时接口（如 Docker、containerd）
```

---

## 2. Control Plane 控制面

### 组件职责

| 组件 | 职责 |
|------|------|
| `kube-apiserver` | 所有操作的统一入口（Frontend） |
| `etcd` | 集群状态持久化存储 |
| `kube-scheduler` | 决定 Pod 调度到哪个 Node |
| `kube-controller-manager` | Watch 资源变化并执行控制逻辑 |
| `cloud-controller-manager` | 与 AWS/GCP 等云厂商 API 交互 |
| `kubelet` | Worker Node 上的 dispatcher，执行 Pod 创建 |
| `kube-proxy` | 维护 iptables/ipvs 规则，实现 Service 流量转发 |

### Pod 创建流程（关键！面试高频）

```
1. kubectl create resource
         ↓
2. apiserver 写入 etcd
         ↓
3. controller-manager watch 到事件，创建 ReplicaSet/Pod 对象
         ↓
4. scheduler watch 到未调度的 Pod，选择合适 Node，写回 apiserver
         ↓
5. 目标 Node 的 kubelet watch 到调度结果
         ↓
6. kubelet 调用 CRI（Docker/containerd）创建容器
         ↓
7. 容器创建完成，状态回写 apiserver
```

> **核心思想**：所有组件只与 apiserver 通信，通过 Watch 机制驱动，etcd 是唯一持久化存储。

---

## 3. Workload 工作负载

| 类型 | 说明 |
|------|------|
| **Pod** | 最小调度单元，包含一个或多个容器 |
| **Deployment** | 管理无状态多副本 Pod |
| **ReplicaSet** | 维护指定数量的 Pod 副本（通常由 Deployment 管理） |
| **StatefulSet** | 有状态服务，类似 Deployment 但绑定 Volume，Pod 名有序 |
| **DaemonSet** | 每个 Node 上运行一个 Pod（如日志采集、监控 agent） |
| **Job / CronJob** | 一次性或周期性任务 |

---

## 4. Service Discovery 服务发现

### Service（集群内部）

- 类型：`ClusterIP`（默认）、`NodePort`、`LoadBalancer`
- DNS 格式：`{name}.{namespace}.svc.cluster.local`
- **ClusterIP 流量路径**：
  1. kube-proxy 转发到 NodePort Service
  2. NodePort → ClusterIP Service（自动创建）
  3. ClusterIP 做负载均衡到各 Pod

### Ingress（集群外部访问）

- Ingress 定义路由规则，需要 **Ingress Controller** 实现
- Meican 使用 **Kong Ingress Controller**
- 其他方式：NodePort、LoadBalancer

---

## 5. Meican EKS 架构

### 集群环境

| 环境 | 集群名 | 说明 |
|------|--------|------|
| Sandbox | - | 开发测试 |
| Prod1 | eks-fan | 仅 minor bug fix |
| Prod2 | eks-fan2 | 主力生产，Terraform 管理，可快速扩展 |

> fan2 相比 fan 的优势：**Portability with Terraform**，专为快速扩展（expedition）设计。

### AWS 基础设施

```
Public Subnet × 3
    └── Nginx EC2
           ↓
        Route53
           ↓
       LoadBalancer
    ┌──────┼──────┐
   AZ1    AZ2    AZ3（3个可用区，10.120.128/144/160.0/20）
    │
    ├── EC2 (C5.xlarge 4vCPU, On-Demand) → Kong Ingress Pods
    └── EC2 (On-Demand + Spot Mix) → Application Pods
```

### Worker Node 内部结构

```
EC2 Instance
├── DaemonSet（每节点运行）
│   ├── Logging Fluent-bit       # 日志采集
│   ├── Node Exporter            # 节点指标
│   ├── Node Termination Handler # Spot 实例回收处理
│   └── Jaeger Agent             # 链路追踪 agent
├── Mandatory Daemon
│   ├── Kube Proxy
│   └── Kubelet
└── Pods（应用 Pod）
    ├── Kong / Kong backup → NLB
    └── 业务服务 Pods（含 Istio Sidecar）
```

### Ingress Controller 高可用

```
Route53 (ntrnl-eks-fan2.meican)
    ├── LoadBalancer → EC2 Running Kong Pods（主）
    └── LoadBalancer → EC2 Running Kong Backup Pods（备）
```

---

## 6. Kong Ingress

**Kong** = Lua + Nginx，作为 API Gateway 兼 Ingress Controller。

### Kong 功能（Plugin Based）

- `set-upstream`：路由到上游服务
- Rate Limiter：限流（支持 Redis 策略）
- Circuit Breaker：熔断
- Authentication：认证
- Inbound / Outbound Control：流量控制
- Transformation：请求/响应转换
- Logging / Monitoring：可观测性

### Kong Ingress YAML 示例

```yaml
# ⚠️ K8s 1.18 之后 apiVersion 改为 networking.k8s.io/v1
apiVersion: extensions/v1beta1  # 旧版
kind: Ingress
metadata:
  name: apple-kong
  namespace: app1
  annotations:
    kubernetes.io/ingress.class: "kong1"
    konghq.com/plugins: app1-apple-rate-limit
spec:
  rules:
    - host: apple.app1.ntrnl-eks-fan.meican
      http:
        paths:
          - path: /
            backend:
              serviceName: apple
              servicePort: 5678

---
# Kong 限流插件配置
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: app1-apple-rate-limit
  namespace: app1
config:
  minute: 10
  policy: redis
  redis_host: kong-redis.xxx.cache.amazonaws.com
plugin: rate-limiting
```

### 服务命名规范

```
{app}.app-{group}.ntrnl-eks-fan{clusterID}.meican
例：order20.app-mutants.ntrnl-eks-fan2.meican
```

### 调用方式

| 协议 | 端口 | 说明 |
|------|------|------|
| HTTP | 80 | 普通 HTTP 调用 |
| gRPC | 443 | 需 `InsecureSkipVerify: true`（Kong 做 TLS 终止） |

```go
// gRPC 通过 Kong Ingress 调用示例
tlsOption = grpc.WithTransportCredentials(
    credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}),
)
cc, _ = grpc.DialContext(ctx,
    "order20.app-mutants.ntrnl-eks-fan2.meican:443",
    tlsOption,
)
```

### 集群内 Service 调用（不经过 Kong）

```go
// 直接调用 ClusterIP Service，使用 grpc.WithInsecure()
// port: 由 service.yaml 中的 spec.ports 定义
```

Service YAML 要点：
- `selector.app` 匹配目标 Pod 的 label
- port name `grpc` 需与 `konghq.com/protocol: "grpc"` 注解对应
- port name `telemetry` 为 Prometheus 指标端口（18888）

---

## 7. Service Mesh - Istio

### 架构

```
Control Plane: istiod
    ├── Pilot    # 服务发现、流量管理配置下发
    ├── Citadel  # 证书管理（mTLS）
    └── Galley   # 配置验证

Data Plane: Envoy Sidecar（每个 Pod 注入）
    ├── 拦截所有 Ingress/Egress 流量
    └── 执行流量管理规则
```

### Sidecar 注入原理 - IPTable Hijacking

```bash
# istio-init 容器设置 iptables 规则，劫持所有流量到 Envoy
istio-iptables -p 15001 -z 15006 -u 1337 -m REDIRECT -i '*' -x "" -b '*' -d 15090,15020
```

- 端口 `15001`：Envoy Outbound 监听
- 端口 `15006`：Envoy Inbound 监听
- 端口 `1337`：Envoy 进程用户 UID（流量不再劫持，避免死循环）

### VirtualService & DestinationRule

**VirtualService**（流量路由规则）：

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: order20-vsvc
  namespace: app-mutants
spec:
  hosts:
    - order20.app-mutants.svc.cluster.local
  http:
    - name: primary
      route:
        - destination:
            host: order20-stable    # 指向 DestinationRule
            subset: v1
          weight: 100               # 100% 流量到稳定版
        - destination:
            host: order20-canary
            subset: v1
          weight: 0                 # 0% 到金丝雀版（灰度准备）
```

**DestinationRule**（负载均衡策略 + subset 定义）：

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: order20-stable
  namespace: app-mutants
spec:
  host: order20-stable.app-mutants.svc.cluster.local
  trafficPolicy:
    loadBalancer:
      simple: ROUND_ROBIN
  subsets:
    - name: v1
      labels:
        app: order20              # 匹配 Pod label
```

**VirtualService 能力**：
- 路由规则（按 header、weight 等）
- 负载均衡策略
- Timeout / Retry
- Circuit Breaker
- Fault Injection（故障注入，用于混沌测试）

---

## 8. Pod 详解

### 定义

- K8s **最小部署单元**
- 包含一个或多个容器
- Pod 内容器共享网络（同一 IP）和存储（Volume）
- Co-located & Co-scheduled（同节点调度）

### 基础 YAML

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp-pod
  labels:
    app: myapp
spec:
  containers:
    - name: myapp-container
      image: busybox
      command: ['sh', '-c', 'echo Hello Kubernetes! && sleep 3600']
```

### Sidecar 模式（Istio）

```
Pod: distro-b666f5d8d-d9hnw
├── istio-proxy（Envoy sidecar）
│   └── Ports: http-envoy-prom: 15090/TCP
└── distro-image（业务容器）
    └── Ports: http-web: 8022/TCP, metrics: 18888/TCP
```

流量：Client → istio-proxy（拦截） → distro-image（业务逻辑）

---

## 9. ArgoCD - 部署控制器

### GitOps 模式

```
Config Repo（Git） ←→ ArgoCD ←→ Kubernetes
```

- 声明式配置存储在 Git
- ArgoCD 持续 reconcile，确保集群状态与 Git 一致

### Rollout 部署策略

```yaml
strategy:
  canary:
    stableService: order20-stable
    canaryService: order20-canary
    trafficRouting:
      istio:
        virtualService:
          name: order20-vsvc    # 与 VirtualService 联动
          routes:
            - primary
    steps:
      - setWeight: 80           # 先把 80% 流量切到新版
      - pause: {duration: 3s}   # 观察 3 秒
```

**支持策略**：
- **Blue/Green**：两套完整环境，瞬时切换
- **Canary**：按权重逐步灰度，与 Istio Service Mesh 协作

---

## 10. 可观测性体系

### 10.1 日志 - Fluent-bit + ELK

```
Container
    ↓ (stdout/stderr)
Docker Log
    ↓
Fluent-bit（DaemonSet）
    ├── → Pulsar（消息队列）
    │         ↓
    │   Pulsar ES Sync（Rust 编写）
    │         ↓
    └──→ ElasticSearch → Kibana
```

### 10.2 监控 - Prometheus + Grafana

```
1. Prometheus PULL Kubernetes API → 自动发现 Target
2. Prometheus PULL Kube State Metrics / Node Exporters / Kube Components
3. Prometheus PUSH Metric Alerts → Alertmanager
4. Alertmanager → Webhooks / Slack / Email
5. Grafana PULL Prometheus Data → 可视化 Dashboard
```

**应用接入方式（推荐）**：使用 `ServiceMonitor` CRD
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    prometheus: k8s
  name: order20-metrics-telemetry-monitor
  namespace: monitoring
spec:
  endpoints:
    - port: telemetry           # 对应 Service port name
  namespaceSelector:
    matchNames:
      - app-mutants
  selector:
    matchLabels:
      app: order20
```

**旧方式**：在 Pod annotations 中声明
```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "2020"
  prometheus.io/path: /api/v1/metrics/prometheus
```

### 10.3 链路追踪 - Jaeger（OpenTracing）

- **Agent**：以 DaemonSet 形式运行在每个节点
- **接入**：应用通过环境变量连接本节点 Jaeger Agent
```
JAEGER_AGENT_HOST: fieldRef(v1:status.hostIP)   # 获取本节点 IP
JAEGER_SERVICE_NAME: distro.app-mutants
JAEGER_SAMPLER_MANAGER_HOST_PORT: http://$(JAEGER_AGENT_HOST):5778
```
- Tower 自动注入这些环境变量，业务无感知

### 10.4 Service Mesh 观测 - Kiali

- 可视化服务间调用拓扑
- 实时展示 RPS、Success Rate、Error Rate
- 支持按 namespace 过滤

---

## 11. AWS IAM 集成

### 认证流程（EKS + IRSA）

```
Client
  ↓ IAM Identity Token + K8s Action
EKS API Server
  ↓ Webhook
aws-iam-authenticator
  ↓ Verify
IAM → 返回 IAM Identity
  ↓
ConfigMap aws-auth → 映射为 Kubernetes User
  ↓
RBAC（Role-Based Access Control）
  ↓ Allow / Deny
```

### Pod 获取 AWS 权限（IRSA）

通过 **ServiceAccount Annotation** 绑定 IAM Role：

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: fluentd
  namespace: logging
  annotations:
    eks.amazonaws.com/role-arn: arn:aws-cn:iam::651844176281:role/eksctl-fan-...
```

EKS Webhook 自动向 Pod 注入环境变量：
```
AWS_DEFAULT_REGION: cn-northwest-1
AWS_ROLE_ARN: arn:aws-cn:iam::...
AWS_WEB_IDENTITY_TOKEN_FILE: /var/run/secrets/eks.amazonaws.com/serviceaccount/token
```

> AWS SDK 会自动读取这些环境变量获取临时凭证，**无需硬编码 AK/SK**。

---

## 12. Tower - 内部平台

### 定位

Meican 自研的 **Secret Management + Deployment Controller**。

### 功能

1. **Watch annotations**（`nerds.k8s.meican.com/inject=true`）
2. **自动注入**环境变量和配置文件到 Pod
3. **与 nerds/app 配合**，自动生成 K8s YAML

### 注入的内容（示例）

```
APP_CREDENTIALS: /var/run/secrets/tower/tower_resource_schema.toml
APP_GROUP: mutants
APP_PROJECT: order20
APP_SECRET: <自动生成>
JAEGER_AGENT_HOST: fieldRef(v1:status.hostIP)
JAEGER_SERVICE_NAME: order20.app-mutants
LOG_FORMAT: json
LOG_HANDLER: console
```

### Scaffolding（脚手架体系）

```
nerds/app-cli   → 初始化项目
nerds/bazaar    → 提供公共 toolset（telemetry、tracing 等）
nerds/app       → 应用框架基础
nerds/tower.v2  → 构建 K8s 云环境配置（Secret + Deployment）
```

### WebApp CRD 示例

```yaml
apiVersion: nerds.k8s.meican.com/v1alpha1
kind: WebApp
metadata:
  name: helloworld
  namespace: app-meican-cd
spec:
  group: meican-cd
  project: helloworld
  mq:                         # 自动创建 Pulsar Topic
    namespace: meican-cd
    tenant: public
    topics:
      - name: helloworld-t1
        partitions: 1
```

---

## 面试要点总结

### 高频问题

**Q: K8s 如何创建一个 Pod？（控制流）**
> kubectl → apiserver → etcd → controller-manager watch → scheduler 选节点 → kubelet watch → CRI 创建容器

**Q: Service 和 Ingress 的区别？**
> Service 是集群内部访问（L4），Ingress 是集群外部访问的 L7 路由规则，需要 Ingress Controller 实现（如 Kong、Nginx）

**Q: Istio Sidecar 如何拦截流量？**
> initContainer 修改 iptables 规则，将 Pod 所有入出流量重定向到 Envoy 代理端口（15001/15006），Envoy 用户 UID 1337 的流量不做拦截以避免死循环

**Q: 金丝雀发布如何实现？**
> ArgoCD Rollout + Istio VirtualService：通过 DestinationRule 定义 stable/canary subset，VirtualService 控制流量权重，逐步将流量从 stable 切换到 canary

**Q: K8s 中 Pod 如何访问 AWS S3/SQS 等服务？**
> IRSA（IAM Role for Service Account）：ServiceAccount 绑定 IAM Role ARN，EKS Webhook 自动注入 WebIdentityToken，AWS SDK 自动获取临时凭证

**Q: DaemonSet 有哪些典型用途？**
> 日志采集（Fluent-bit）、节点指标收集（Node Exporter）、链路追踪 Agent（Jaeger）、kube-proxy、kubelet

**Q: StatefulSet 和 Deployment 的区别？**
> StatefulSet Pod 名称有序（pod-0, pod-1），绑定固定 PVC，重启后数据保留，适合数据库、消息队列等有状态服务；Deployment Pod 名称随机，无状态

---

## 参考架构图关键词

- `cn-northwest-1` = AWS 宁夏区
- `eks-fan2` = Meican 主生产集群
- 3 AZ 部署，On-Demand + Spot 混用节省成本
- Kong + Istio 双层流量管控（外部 L7 网关 + 内部 Service Mesh）
