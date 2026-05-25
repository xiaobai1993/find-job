# K8s 服务发现从零理解 + Go 代码 + 美餐实践

> 服务发现不要只理解成“注册中心”。在 K8s 体系里，服务发现更多是由基础设施完成的：Pod 变化由 Service 屏蔽，服务名由 CoreDNS 解析，流量由 kube-proxy / CNI / Service Mesh 转发。

---

## 一、先建立整体认知

### 1. 服务发现到底解决什么问题？

微服务之间要互相调用，比如：

```text
订单服务 -> 支付服务
支付服务 -> 账户服务
支付服务 -> 风控服务
```

但是服务实例不是固定的：

```text
Pod 会重启
Pod 会扩容
Pod 会缩容
Pod 会重新调度到其他节点
Pod IP 会变化
```

如果调用方把 IP 写死：

```text
http://10.2.3.15:8080/pay
```

那一旦这个 Pod 被销毁，调用就失败了。

所以服务发现要解决的核心问题是：

```text
调用方只关心“我要调用哪个服务”，不关心“这个服务当前有哪些实例、IP 是多少、谁还活着”。
```

---

### 2. 两种典型服务发现模式

#### 模式一：应用层注册中心

这是 Nacos、Eureka、Consul 这类模式。

```text
服务启动 -> 注册到注册中心
调用方 -> 从注册中心拉取实例列表
调用方 -> 本地负载均衡选择一个实例调用
```

典型流程：

```text
payment-service 启动
    -> 注册 IP、端口、服务名到 Nacos
order-service 调用 payment-service
    -> 从 Nacos 获取 payment-service 实例列表
    -> 选择一个实例发起请求
```

这种模式的特点是：

```text
服务自己注册
客户端自己发现
客户端自己负载均衡
```

---

#### 模式二：K8s 原生服务发现

K8s 里通常不需要业务服务自己注册 IP，而是由 K8s 控制面自动维护。

```text
Pod 启动 -> Deployment 管理 Pod
Service -> 根据 label selector 选择 Pod
EndpointSlice -> 自动维护后端 Pod IP 列表
CoreDNS -> 把服务名解析成 Service ClusterIP
kube-proxy / CNI -> 把请求转发到真实 Pod
```

调用方只需要写：

```text
http://payment-service.app-payment.svc.cluster.local:8080
```

或者同 namespace 下直接写：

```text
http://payment-service:8080
```

这种模式的特点是：

```text
服务不需要主动注册
调用方不需要感知实例列表
K8s Service 提供稳定入口
CoreDNS 提供域名解析
kube-proxy / CNI 提供转发和负载均衡
```

---

## 二、K8s 服务发现完整链路

### 1. 为什么不能直接调 Pod IP？

Pod 是临时资源。

比如支付服务有 3 个 Pod：

```text
payment-service-abc   10.1.1.11
payment-service-def   10.1.1.12
payment-service-ghi   10.1.1.13
```

发布、重启、扩缩容后可能变成：

```text
payment-service-jkl   10.1.2.21
payment-service-mno   10.1.2.22
payment-service-pqr   10.1.2.23
```

所以 Pod IP 不能作为稳定访问入口。

K8s 用 Service 给一组 Pod 提供稳定入口：

```text
payment-service -> 一组 payment Pod
```

调用方只访问 Service，不直接访问 Pod。

---

### 2. Service：稳定访问入口

一个典型 Service：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: payment-service
  namespace: app-payment
spec:
  type: ClusterIP
  selector:
    app: payment-service
  ports:
    - name: http
      port: 8080
      targetPort: 8080
```

关键点：

```text
metadata.name = 服务名
namespace = 命名空间
type = ClusterIP，集群内部访问
selector = 选择哪些 Pod 作为后端
port = Service 暴露的端口
targetPort = Pod 容器真正监听的端口
```

Service 会得到一个稳定的虚拟 IP，也就是 ClusterIP：

```text
payment-service.app-payment.svc.cluster.local -> 172.20.10.88
```

这个 ClusterIP 是稳定的，不会因为后端 Pod 重启而变化。

---

### 3. Endpoint / EndpointSlice：真实后端列表

Service 本身只是稳定入口，它背后真正有哪些 Pod，由 EndpointSlice 维护。

假设 Service selector 是：

```yaml
selector:
  app: payment-service
```

K8s 会自动找到所有带这个 label 的 Pod：

```text
payment-service-abc   10.1.1.11:8080
payment-service-def   10.1.1.12:8080
payment-service-ghi   10.1.1.13:8080
```

然后写入 EndpointSlice。

可以理解成：

```text
Service 是门牌号
EndpointSlice 是门牌号背后真实住户列表
```

当 Pod 扩容、缩容、重启时，EndpointSlice 会自动更新。

---

### 4. CoreDNS：服务名解析

K8s 集群里会部署 CoreDNS。

当创建 Service 后，CoreDNS 会支持这种域名：

```text
payment-service.app-payment.svc.cluster.local
```

DNS 名称格式是：

```text
服务名.命名空间.svc.cluster.local
```

如果调用方和被调用方在同一个 namespace，可以简写成：

```text
payment-service
```

如果跨 namespace，建议写完整：

```text
payment-service.app-payment.svc.cluster.local
```

解析链路：

```text
Go 代码访问 payment-service.app-payment.svc.cluster.local
    -> Pod 内 DNS 配置指向 CoreDNS
    -> CoreDNS 查询 K8s Service
    -> 返回 Service 的 ClusterIP
```

---

### 5. kube-proxy：把 Service 流量转发到 Pod

DNS 只负责把服务名解析成 ClusterIP。

真正把请求从 ClusterIP 转发到后端 Pod 的，是 kube-proxy 或 CNI 的 Service 实现。

典型链路：

```text
order-service Pod
    -> 请求 payment-service.app-payment.svc.cluster.local:8080
    -> CoreDNS 返回 ClusterIP 172.20.10.88
    -> 请求发到 172.20.10.88:8080
    -> kube-proxy 根据 iptables/ipvs 规则
    -> 转发到某一个后端 Pod 10.1.1.12:8080
```

所以 K8s 服务发现可以拆成两件事：

```text
服务名怎么找到？CoreDNS
流量怎么转过去？kube-proxy / iptables / ipvs / CNI
```

---

## 三、一次完整调用链路

以订单服务调用支付服务为例：

```text
1. payment-service 部署 3 个 Pod
2. K8s 创建 payment-service 这个 ClusterIP Service
3. EndpointSlice 自动记录 3 个 payment Pod 的 IP
4. order-service 代码里访问 http://payment-service.app-payment.svc.cluster.local:8080/pay
5. Pod 内 DNS 请求 CoreDNS
6. CoreDNS 返回 payment-service 的 ClusterIP
7. 请求打到 ClusterIP
8. kube-proxy / CNI 把请求转发到某个 payment Pod
9. payment Pod 处理请求并返回结果
```

可以画成：

```text
order-service Pod
      |
      | DNS 查询
      v
   CoreDNS
      |
      | 返回 ClusterIP
      v
payment-service ClusterIP
      |
      | kube-proxy / iptables / ipvs
      v
payment Pod A / payment Pod B / payment Pod C
```

---

## 四、Service 四种类型

### 1. ClusterIP

默认类型，只能集群内部访问。

适合：

```text
订单服务调用支付服务
支付服务调用账户服务
支付服务调用风控服务
对账服务调用支付流水服务
```

支付系统内部服务之间调用，优先用 ClusterIP。

---

### 2. NodePort

在每个 Node 上开放一个端口：

```text
NodeIP:NodePort -> Service -> Pod
```

一般不建议生产直接暴露 NodePort，因为：

```text
端口管理麻烦
安全边界弱
不适合大规模服务治理
```

更多用于临时调试或某些特殊场景。

---

### 3. LoadBalancer

对接云厂商负载均衡器。

在 AWS EKS 里，Service 类型是 LoadBalancer 时，通常会创建 AWS ELB / NLB。

适合：

```text
需要从集群外部访问某个服务
需要云厂商负载均衡器承接入口流量
```

但在公司生产环境里，一般不会每个业务服务都创建一个 LoadBalancer。更常见是：

```text
外部流量 -> 统一网关 / Ingress / Kong -> 内部 Service -> Pod
```

---

### 4. ExternalName

ExternalName 相当于给外部域名做一个 K8s 内部别名。

例如：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: channel-api
  namespace: app-payment
spec:
  type: ExternalName
  externalName: api.channel.example.com
```

业务访问：

```text
channel-api.app-payment.svc.cluster.local
```

实际 DNS CNAME 到：

```text
api.channel.example.com
```

适合把外部依赖用内部服务名统一管理。

---

## 五、Go 代码怎么调用 K8s 服务

### 1. HTTP 调用示例

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(
        ctx,
        http.MethodGet,
        "http://payment-service.app-payment.svc.cluster.local:8080/health",
        nil,
    )
    if err != nil {
        panic(err)
    }

    client := &http.Client{
        Timeout: 2 * time.Second,
    }

    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    fmt.Println(resp.StatusCode)
}
```

重点不是代码复杂，而是这里没有写 Pod IP：

```text
不用写 10.x.x.x
不用关心 payment 有几个 Pod
不用关心 Pod 是否重启
只写 Service DNS 名称
```

---

### 2. HTTP 调用时要注意连接池

Go 的 `http.Client` 默认会复用 TCP 连接。

生产里一般会显式配置：

```go
transport := &http.Transport{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 20,
    IdleConnTimeout:     90 * time.Second,
}

client := &http.Client{
    Transport: transport,
    Timeout:   2 * time.Second,
}
```

原因是：

```text
服务发现只解决“地址在哪里”
连接池还会影响连接是否复用、旧连接是否及时释放
发布期间如果后端 Pod 被删，旧连接可能失败
所以还需要超时、重试、熔断兜底
```

---

### 3. gRPC 调用示例

```go
package main

import (
    "context"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    conn, err := grpc.DialContext(
        ctx,
        "payment-grpc.app-payment.svc.cluster.local:50051",
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock(),
    )
    if err != nil {
        panic(err)
    }
    defer conn.Close()

    // client := pb.NewPaymentServiceClient(conn)
    // resp, err := client.Pay(ctx, &pb.PayRequest{...})
}
```

同样，gRPC 里写的是服务域名，不是 Pod IP。

---

### 4. 为什么 Go 里不需要接入 Nacos SDK？

如果服务都跑在 K8s 内部，且内部调用走 K8s Service，那么 Go 服务通常不需要自己接入 Nacos 做服务注册。

因为：

```text
Pod 生命周期由 K8s 管
后端实例列表由 EndpointSlice 管
服务名解析由 CoreDNS 管
流量转发由 kube-proxy / CNI 管
```

应用只需要：

```text
监听端口
暴露健康检查
配置好 Deployment / Service
用 Service DNS 调用下游
```

这就是云原生服务发现和传统注册中心最大的不同。

---

## 六、美餐生产实践怎么理解

结合当前仓库里的美餐技术栈，可以这样理解：

```text
语言：Go
容器编排：AWS EKS
服务发现：K8s DNS + Route53
配置中心：Nacos
API 网关：Kong
部署：GitLab CI + ArgoCD + Argo Rollouts
观测：Prometheus + Grafana + Jaeger + EFK
```

### 1. 集群内部服务发现：K8s DNS

服务之间内部调用，核心是：

```text
Service Name -> CoreDNS -> ClusterIP -> Pod
```

比如：

```text
order-service 调 payment-service
payment-service 调 account-service
reconciliation-service 调 payment-service
```

都可以通过 K8s Service DNS 完成。

面试可以说：

> 我们在 EKS 里主要用 K8s 原生 Service 做内部服务发现。每个服务都会有对应的 ClusterIP Service，服务之间通过 `service.namespace.svc.cluster.local` 这种 DNS 名称互相调用。Pod 扩缩容、重启、发布时，EndpointSlice 会自动更新后端实例，调用方不需要感知 Pod IP 变化。

---

### 2. 集群外部入口：Route53 + Kong

对外 HTTP / gRPC 入口不是直接暴露每个业务 Service，而是统一走网关。

典型链路：

```text
用户 / 内部系统
    -> Route53 域名解析
    -> AWS LoadBalancer
    -> Kong Gateway
    -> K8s Service
    -> Pod
```

例如仓库中技术栈文档提到：

```text
HTTP：service-web.app-group.ntrnl-eks-fan.meican
GRPC：service-grpc.app-group.ntrnl-eks-fan.meican
```

这类域名由 Route53 管理，流量进入 Kong，再转到内部 Service。

面试可以说：

> 内部服务发现主要靠 K8s DNS；对外访问或者跨网络访问会通过 Route53 域名进入 Kong 网关，再由 Kong 转发到集群内对应的 Service。这样入口统一，鉴权、限流、路由、灰度都可以在网关层控制。

---

### 3. Nacos 在美餐更偏配置中心

仓库里已有文档提到 Nacos，但结合当前美餐技术栈，更准确的说法是：

```text
K8s DNS + Route53：负责服务发现
Nacos：主要负责配置中心
```

Nacos 管理：

```text
应用配置
环境配置
限流阈值
功能开关
第三方渠道配置
```

服务发现不一定再依赖 Nacos，因为 K8s 已经提供了原生服务发现能力。

面试时不要把两套体系说混。

可以这样表达：

> 传统微服务可能用 Nacos 同时做注册中心和配置中心。但我们现在跑在 EKS 上，服务实例的生命周期已经由 K8s 管理，所以内部服务发现优先使用 K8s Service + CoreDNS。Nacos 更多承担配置中心角色，比如配置热更新、环境隔离、限流开关等。

---

### 4. ArgoCD / Rollout 发布时 Service 为什么稳定？

发布时 Pod 会换，但 Service 不变。

```text
旧 Pod 下线
新 Pod 上线
EndpointSlice 更新
Service DNS 不变
ClusterIP 不变
调用方地址不变
```

所以调用方始终访问：

```text
payment-service.app-payment.svc.cluster.local
```

不会因为版本发布改调用地址。

如果用了 Argo Rollouts 做金丝雀，一般会配合 stable / canary Service：

```text
payment-stable -> 稳定版本 Pod
payment-canary -> 金丝雀版本 Pod
```

再通过网关、Service Mesh 或 Rollout 控制流量比例。

---

## 七、K8s Service 和 Nacos 有什么区别？

| 对比维度 | K8s Service | Nacos |
|---|---|---|
| 所在层次 | 基础设施层 | 应用层中间件 |
| 注册方式 | K8s 根据 Pod label 自动维护 | 应用启动后主动注册 |
| 发现方式 | DNS 解析 Service 名称 | SDK / HTTP API 拉取实例列表 |
| 负载均衡 | kube-proxy / CNI 做四层转发 | 客户端负载均衡 |
| 健康来源 | Pod readiness / EndpointSlice | 心跳 / 服务端探活 |
| 适合场景 | 云原生 K8s 内部服务互调 | 非 K8s、混合部署、配置中心、客户端治理 |
| 调用方复杂度 | 低，只写域名 | 较高，需要 SDK 或框架集成 |

一句话总结：

```text
K8s Service 是平台帮你发现服务。
Nacos 是应用自己参与服务注册和发现。
```

---

## 八、服务发现不等于服务治理

服务发现只解决：

```text
我要调用的服务在哪里？
```

但生产系统还要解决：

```text
调用慢怎么办？
调用失败怎么办？
下游挂了怎么办？
发布时流量怎么切？
故障怎么观测？
```

所以完整服务治理还需要：

| 能力 | 作用 |
|---|---|
| 超时 | 防止请求无限等待 |
| 重试 | 处理短暂网络抖动 |
| 熔断 | 下游异常时快速失败，避免拖垮调用方 |
| 限流 | 防止流量打爆服务 |
| 负载均衡 | 多实例分摊流量 |
| 健康检查 | 不把流量打到未就绪实例 |
| 灰度发布 | 新版本小流量验证 |
| 可观测性 | 通过日志、指标、链路追踪定位问题 |

支付系统尤其要注意：

```text
不能只依赖“服务能发现”。
还要保证失败可控、重试幂等、状态一致、问题可追踪。
```

---

## 九、常见坑和排查思路

### 1. DNS 解析失败

现象：

```text
lookup payment-service: no such host
```

排查：

```text
Service 名字是否写错
namespace 是否写错
跨 namespace 是否用了完整域名
CoreDNS Pod 是否正常
Pod 的 /etc/resolv.conf 是否正确
```

---

### 2. Service 有，但没有后端 Pod

现象：

```text
连接超时
connection refused
Service 能解析，但请求不通
```

排查：

```text
Service selector 是否和 Pod label 匹配
EndpointSlice 是否有地址
Pod readinessProbe 是否通过
Pod containerPort / targetPort 是否一致
```

核心判断：

```text
Service 存在不代表后端一定有可用 Pod。
要看 EndpointSlice。
```

---

### 3. 发布期间偶发失败

可能原因：

```text
旧 Pod 正在退出，但连接还没完全摘除
新 Pod readiness 还没通过
客户端连接池复用了旧连接
超时时间太短或没有重试
```

解决思路：

```text
配置 readinessProbe
配置 preStop 优雅退出
给应用留 terminationGracePeriodSeconds
客户端设置合理 timeout
幂等接口可以做有限重试
```

---

### 4. 跨 namespace 调用失败

错误写法：

```text
http://payment-service:8080
```

如果调用方不在 `app-payment` namespace，就可能解析不到目标服务。

推荐写完整：

```text
http://payment-service.app-payment.svc.cluster.local:8080
```

---

### 5. 把服务发现和配置中心混为一谈

服务发现解决：

```text
服务地址在哪里
```

配置中心解决：

```text
应用配置是什么
```

在美餐这套技术栈里，可以这样区分：

```text
K8s DNS + Route53：服务发现
Nacos：配置中心
Kong：统一入口和网关治理
```

---

## 十、支付系统里的设计原则

### 1. 内部服务优先 ClusterIP

支付、账户、对账、风控这些内部服务互调，优先使用：

```text
ClusterIP Service + K8s DNS
```

不要直接暴露公网，也不要直接依赖 Pod IP。

---

### 2. 对外入口统一走网关

对外服务不要每个服务单独暴露 LoadBalancer。

推荐：

```text
Route53 -> LoadBalancer -> Kong / Ingress -> Service -> Pod
```

好处：

```text
入口统一
鉴权统一
限流统一
审计统一
灰度统一
证书统一
```

---

### 3. 健康检查必须认真做

Service 是否把流量打到某个 Pod，取决于 Pod 是否 ready。

所以 readinessProbe 很关键。

支付服务启动时，不能进程刚起来就接流量。至少要确认：

```text
配置加载完成
数据库连接正常
Redis / MQ 等关键依赖初始化完成
HTTP / gRPC server 已经监听
```

否则会出现：

```text
Pod Running 了，但业务还没准备好，请求进来直接失败。
```

---

### 4. 调用下游必须有超时

服务发现只保证你能找到服务，不保证下游一定快。

Go 调用必须带：

```text
context timeout
HTTP client timeout
gRPC deadline
```

支付链路里尤其不能无限等待，否则容易线程、goroutine、连接池全部被拖垮。

---

### 5. 重试必须结合幂等

服务发现异常、网络抖动、Pod 发布期间都可能导致短暂失败。

可以重试，但支付场景必须注意：

```text
扣款类接口不能盲目重试
必须有幂等号
必须有状态机
必须能防重复扣款
```

所以面试时可以强调：

> 我们会对查询类、幂等类接口做有限重试，但对扣款、退款这类资金操作，重试必须建立在幂等表和状态机之上，不能简单粗暴重试。

---

## 十一、面试标准回答

### Q1：K8s 服务发现是怎么实现的？

**答：**

K8s 服务发现主要依赖 Service、EndpointSlice、CoreDNS 和 kube-proxy。

Pod 本身是临时的，IP 会随着重启、扩缩容变化，所以不能直接用 Pod IP 调用。K8s 会给一组 Pod 创建一个 Service，Service 有稳定的 ClusterIP，并通过 label selector 选择后端 Pod。

后端 Pod 的真实地址列表由 EndpointSlice 自动维护。Pod 扩容、缩容、重启时，EndpointSlice 会自动更新。

调用方通过 `service.namespace.svc.cluster.local` 访问服务，CoreDNS 会把这个域名解析成 Service 的 ClusterIP。请求到达 ClusterIP 后，kube-proxy 通过 iptables 或 ipvs 规则把流量转发到某个真实 Pod。

所以完整链路是：

```text
服务名 -> CoreDNS -> ClusterIP -> kube-proxy -> Pod
```

---

### Q2：CoreDNS 和 kube-proxy 分别做什么？

**答：**

CoreDNS 负责服务名解析，把 K8s Service 的 DNS 名称解析成 ClusterIP。

例如：

```text
payment-service.app-payment.svc.cluster.local -> 172.20.10.88
```

kube-proxy 负责流量转发和负载均衡。请求访问 ClusterIP 后，kube-proxy 根据 iptables 或 ipvs 规则，把请求转发到 Service 后面的某个 Pod。

一句话：

```text
CoreDNS 解决“名字到 IP”。
kube-proxy 解决“ClusterIP 到 Pod”。
```

---

### Q3：你们公司服务发现怎么做？

**答：**

我们公司是云原生微服务架构，服务跑在 AWS EKS 上，内部服务发现主要用 K8s 原生 Service + CoreDNS。

每个服务都会有对应的 ClusterIP Service，服务之间通过 `service.namespace.svc.cluster.local` 这种域名调用，不直接写 Pod IP。Pod 扩缩容、重启、发布时，EndpointSlice 会自动更新后端 Pod 列表，调用方不需要感知。

对外入口这块，我们会通过 Route53 做域名解析，流量进入 AWS LoadBalancer 和 Kong 网关，再转发到集群内部对应的 Service。这样外部入口统一由 Kong 管理，可以做路由、鉴权、限流和灰度。

Nacos 在我们这里更多承担配置中心角色，比如环境配置、应用配置、限流阈值、功能开关等。服务发现本身主要交给 K8s DNS 和 Route53。

---

### Q4：K8s Service 和 Nacos 服务发现有什么区别？

**答：**

K8s Service 是基础设施层的服务发现。服务实例由 K8s 管理，Pod 列表由 EndpointSlice 自动维护，调用方通过 DNS 名称访问 Service，不需要自己接入注册中心 SDK。

Nacos 是应用层注册中心。服务启动后需要主动把自己的 IP、端口、服务名注册到 Nacos，调用方通过 SDK 或 API 获取实例列表，再做客户端负载均衡。

所以区别是：

```text
K8s 是平台自动发现，适合云原生集群内部调用。
Nacos 是应用主动注册，适合传统微服务、混合部署，也常用作配置中心。
```

在 EKS 场景下，内部服务调用优先用 K8s Service；Nacos 更多用于配置管理。

---

### Q5：服务发现出问题怎么排查？

**答：**

我会按链路分层排查。

第一层看 DNS：服务名是否写对、namespace 是否正确、CoreDNS 是否正常。

第二层看 Service：Service 是否存在，port 和 targetPort 是否配置正确。

第三层看 EndpointSlice：Service selector 是否匹配 Pod label，EndpointSlice 里是否有后端 Pod 地址。

第四层看 Pod：Pod 是否 Running，readinessProbe 是否通过，容器端口是否真的监听。

第五层看网络：NetworkPolicy、安全组、CNI、kube-proxy 规则是否正常。

如果是发布期间偶发失败，还会看连接池、优雅退出、preStop、terminationGracePeriodSeconds、客户端超时和重试配置。

---

## 十二、一句话记忆

```text
K8s 服务发现不是服务自己注册 IP，
而是 K8s 用 Service 屏蔽 Pod 变化，
用 EndpointSlice 维护后端实例，
用 CoreDNS 解析服务名，
用 kube-proxy / CNI 把流量转到真实 Pod。
```

美餐这套可以记成：

```text
内部调用：K8s DNS + ClusterIP Service
外部入口：Route53 + Kong + Service
配置管理：Nacos
发布治理：ArgoCD + Rollouts
问题定位：Prometheus + Grafana + Jaeger + EFK
```
