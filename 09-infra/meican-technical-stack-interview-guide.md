# 美餐技术栈面试指南

> 🎯 基于美餐内部 App-cli HelloWorld Sandbox 文档整理
> 📌 按照面试高频提问方向组织，包含架构设计、CI/CD、可观测性、服务治理等模块

---

## 🗺️ 一、整体技术架构全景

### 面试官问：你们公司的技术栈是什么样的？

> **标准答案：**
>
> "我们公司是典型的云原生微服务架构，全部基于 AWS EKS，基础设施即代码用 Terraform 管理。
>
> **核心技术栈分层：**
>
> **1. 语言与框架层**
> - 全部 Go 语言，自研脚手架 app-cli 一键生成 Web + gRPC 服务
> - 自研 Go 框架库：app (v0.6.x)、bazaar (v0.2.x)
>
> **2. 基础设施层**
> - 容器编排：AWS EKS，跨可用区部署
> - IaC：Terraform 管理所有云资源
> - IAM：IRSA (IAM Roles for Service Accounts) 方式鉴权，不用 AK/SK
>
> **3. CI/CD 层**
> - CI：GitLab CI，自动测试、代码质量检查、Sonar、镜像构建上传 ECR
> - CD：ArgoCD v2.7.3 + Argo Rollouts，GitOps 方式部署
> - 发布策略：支持金丝雀发布、蓝绿发布
>
> **4. 中间件层**
> - 配置中心：Nacos
> - 消息队列：Apache Pulsar
> - API 网关：Kong
> - 服务发现：K8s DNS + Route53
>
> **5. 数据层**
> - 关系型：Aurora PostgreSQL
> - KV：ElastiCache Redis (支持集群/非集群模式)
> - NoSQL：DynamoDB
> - 对象存储：S3
>
> **6. 可观测性层**
> - 链路追踪：Jaeger
> - 监控：Prometheus + Grafana（每个服务自动生成 Dashboard）
> - 日志：EFK 栈，Kibana 查询
> - Profiling：Go pprof，18888 端口暴露
>
> **7. 开发层**
> - 脚手架：app-cli 一键生成标准化项目结构
> - 沙箱环境：完整隔离的 sandbox 环境，与生产同架构
> - VPN：WireGuard 方式接入
>
> 整个架构高度标准化，新服务从创建到上线只需要遵循标准流程即可，不需要重复造轮子。"

---

## 🔄 二、CI/CD 流程详解

### 面试官问：你们的服务从代码到上线的完整流程是怎样的？

> **标准答案：**
>
> "我们是典型的 GitOps 流程，分为 CI 和 CD 两部分：
>
> **CI 阶段（GitLab CI）：**
> 1. 代码提交 MR，自动触发 CI 流水线
> 2. 运行单元测试、代码质量扫描、Sonar 质量门禁
> 3. 构建 Docker 镜像，打上版本 tag，推送到 AWS ECR
> 4. 代码合并 master 后，自动生成 v0.x.x 版本 tag
>
> **CD 阶段（ArgoCD + Argo Rollouts）：**
> 1. Git 仓库维护所有服务的 K8s YAML 配置（argocd-sandbox 仓库）
> 2. ArgoCD 监听 Git 仓库变更，自动同步到 K8s 集群
> 3. 部署策略：默认用 Argo Rollouts 做金丝雀发布
>    - 先发布 5% 流量，观察指标
>    - 逐步放量到 100%
>    - 指标异常自动回滚
> 4. 发布过程：ArgoUI 可视化查看进度，支持手工暂停、回滚
>
> **新服务上线完整流程（约 1 天）：**
> 1. app-cli 生成项目脚手架
> 2. Terraform 创建 ECR 仓库、RDS/Redis 等资源
> 3. Nacos 配置初始化
> 4. ArgoCD 创建 Application
> 5. Kong Ingress + Route53 配置域名
> 6. 自动生成 Grafana Dashboard、Jaeger 链路、Kibana 日志索引
>
> 整个流程高度自动化，基础设施层面不需要开发操心太多。"

---

## 🔐 三、AWS 权限管理设计

### 面试官问：Pod 怎么访问 AWS 资源？用 AK/SK 吗？

> **标准答案：**
>
> "我们不用 AK/SK 硬编码，用的是 K8s 标准的 **IRSA (IAM Roles for Service Accounts)** 方案：
>
> **核心原理：**
> 1. Terraform 创建 IAM Role，定义权限策略（如 S3 读写、DDB 读写）
> 2. Terraform 创建 K8s ServiceAccount，并与 IAM Role 关联
> 3. Pod 指定使用这个 ServiceAccount
> 4. K8s 自动把 AWS Web Identity Token 注入到 Pod
>    - 路径：`/var/run/secrets/eks.amazonaws.com/serviceaccount/token`
> 5. AWS SDK 自动发现这个 token，调用 STS 获取临时凭证
>
> **优势：**
> - ✅ 没有 AK/SK 泄露风险，不需要存在配置里
> - ✅ 凭证自动轮转，都是临时的
> - ✅ 权限粒度精确到单个服务
> - ✅ 完全符合 AWS 安全最佳实践
>
> **代码层面不需要任何改动**，AWS SDK 会自动处理。配置只需要在 Rollout YAML 里指定 serviceAccountName 就行。
>
> 这是 EKS 访问 AWS 资源的标准做法，非常成熟。"

---

## 📡 四、服务治理与中间件

### 1. 配置中心 - Nacos

> **面试官问：你们用什么做配置中心？怎么保证配置安全？**
>
> "我们用 Nacos 做配置中心：
> - 按 Namespace 隔离环境（sandbox/production）
> - 按 Group 隔离不同服务（如 payment 组）
> - 配置文件格式：TOML
> - 权限控制：基于角色的细粒度权限（r/rw）
> - 支持配置热更新，不需要重启服务
>
> 服务启动时自动从 Nacos 拉取配置，本地有缓存兜底。"

### 2. 消息队列 - Pulsar

> **面试官问：你们用什么 MQ？和 Kafka 比有什么优势？**
>
> "我们用 Apache Pulsar 做消息系统：
>
> **优势：**
> - 存算分离架构，Broker 无状态方便扩容
> - Bookie 存储层，数据多副本
> - 原生支持多租户、多 Namespace，适合公司级复用
> - 支持消息持久化、ack 机制
>
> **我们的用法：**
> - Topic 权限在 Argo YAML 里声明
> - Producer/Consumer 代码封装在内部 bazaar 库
> - 业务代码调用很简单，不需要关心底层细节
> - Backlog 有监控报警"

### 3. API 网关 - Kong

> **面试官问：你们的 gRPC/HTTP 服务怎么对外暴露？**
>
> "用 Kong 做 API 网关，统一入口：
>
> **HTTP 服务：**
> - Kong Ingress 配置路由
> - Route53 域名解析 → Kong ELB → Kong → Service → Pod
> - Host: `service-web.app-group.ntrnl-eks-fan.meican`
>
> **gRPC 服务：**
> - Kong 支持 gRPC 代理，用 ServerName 做路由
> - 客户端 TLS 配置 ServerName
> - Host: `service-grpc.app-group.ntrnl-eks-fan.meican`
>
> **本地测试方式：**
> - 公司内网/VPN 直接访问域名
> - 外部：WireGuard VPN 接入后访问"

---

## 👁️ 五、可观测性体系

### 面试官问：你们的可观测性是怎么做的？线上出问题怎么排查？

> **标准答案：**
>
> "我们是完整的三层可观测性体系，**新服务创建时自动全部集成好**，不需要开发额外配置：
>
> **1. 链路追踪 - Jaeger**
> - 自动注入：Pod 启动时自动设置 `JAEGER_SERVICE_NAME` 环境变量
> - 全链路打通：HTTP → gRPC → 数据库 → MQ，整条链路都能看到
> - TraceID 自动透传，用 OpenTracing 标准
> - 查询地址：tracing.planetmeican.com，按服务名、时间范围查
>
> **2. 监控指标 - Prometheus + Grafana**
> - 指标端口：18888，标准 Go Metrics + 业务自定义指标
> - **自动生成 Dashboard**：每个服务上线自动生成一个 Grafana 页面
> - Dashboard 分三类：
>   - 业务指标：QPS、成功率、耗时 P95/P99
>   - Go Runtime：GC、Goroutine、内存分配
>   - Pod 状态：重启、CPU/Mem 使用率、网络
> - 告警规则统一模板配置，异常自动告警
>
> **3. 日志 - EFK (Elasticsearch + Fluentd + Kibana)**
> - 统一日志格式：JSON 结构化日志
> - 自动采集：Namespace 为 `app-*` 的 Pod 日志自动收集
> - Kibana 按服务名、TraceID、Level 查询
>
> **4. Pprof 性能剖析**
> - 18888 端口暴露 `/debug/pprof`
> - heap、goroutine、threadcreate 都能看
> - 生产环境也能安全访问，有独立 telemetry 域名
>
> **线上问题排查标准流程：**
> 1. Grafana 看异常指标，定位出问题时间点
> 2. Jaeger 查对应时间的慢 Trace，看哪一步耗时异常
> 3. Kibana 用 TraceID 查详细错误日志
> 4. 必要时 Pprof 抓现场 heap/goroutine 分析
>
> 整个体系非常完善，大部分问题半小时内就能定位根因。"

---

## 🏗️ 六、项目脚手架与标准化

### 面试官问：你们怎么保证不同团队的项目结构、代码风格一致？

> **标准答案：**
>
> "我们有自研的 **app-cli 脚手架工具**，从源头上保证标准化：
>
> **1. 一键生成项目**
> ```bash
> app new meican-cd/helloworld --type=web --type=grpc
> ```
> 一条命令就生成标准化的项目结构，包含：
> - HTTP 服务（默认 8022 端口）
> - gRPC 服务（默认 8023 端口）
> - Makefile 标准命令（run/test/build/docker-push）
> - .gitlab-ci.yml 完整 CI 流程
> - ArgoCD 部署 YAML 模板
> - 配置文件模板、Nacos 集成
> - Jaeger、Prometheus、日志集成
>
> **2. 标准项目结构**
> ```
> helloworld/
> ├── cmd/           # 入口
> ├── config/        # 配置
> ├── internal/      # 业务代码
> ├── argo-sandbox/  # 部署模板
> ├── local-client/  # 本地测试客户端
> ├── Makefile       # 标准化命令
> └── .gitlab-ci.yml # CI 配置
> ```
>
> **3. 标准化带来的好处**
> - 新人上手快，任何项目看一眼就懂结构
> - 运维成本低，所有服务部署、监控、日志都一样
> - 基础设施升级不需要每个项目改代码，升级公共库就行
> - 代码 Review 成本低，风格和模式都统一
>
> 这个脚手架是我们基础设施团队花了好几年迭代出来的，现在非常成熟，新服务基本零配置就能跑起来。"

---

## 🌐 七、多环境管理

### 面试官问：你们有几套环境？怎么隔离？

> **标准答案：**
>
> "我们主要有 **Sandbox（沙箱）** 和 **Production（生产）** 两套环境，架构完全一致：
>
> **Sandbox 环境**
> - 用途：开发联调、测试、预发布
> - 独立 EKS 集群
> - 独立 Nacos、Pulsar、数据库
> - 独立 ArgoCD、Grafana、Jaeger、Kibana
> - 访问：公司内网或 WireGuard VPN
>
> **Production 环境**
> - 生产流量，多可用区部署
> - 资源配置更高、副本数更多
> - 权限更严格，生产 Argo 需要单独申请权限
>
> **环境隔离方式：**
> 1. K8s Namespace 按业务组隔离：`app-meican-pay`、`app-meican-cd`
> 2. Nacos Namespace 隔离配置
> 3. Pulsar 多租户隔离 Topic
> 4. AWS 账号级别隔离（Sandbox 和 Production 不同 AWS 账号）
>
> 好处是 Sandbox 和生产环境完全同构，Sandbox 验证过的代码，上生产基本不会有环境差异问题。"

---

## 📊 八、架构设计亮点总结

### 面试官问：你们这套架构设计有什么亮点？

> **标准答案：**
>
> "我觉得这套架构最大的几个亮点是：
>
> **1. 高度标准化，基础设施复用率极高**
> - 所有服务用同样的脚手架、同样的部署方式、同样的可观测性
> - 基础设施团队只需要维护好这一套东西，业务团队不需要关心
> - 新服务上线成本极低，一天就能从零到上线
>
> **2. GitOps 真正落地**
> - 所有配置都在 Git 里，变更可追溯、可审计
> - ArgoCD 自动同步，不用手工 kubectl apply
> - 回滚就是 Git revert，非常简单
>
> **3. 安全做得很到位**
> - IRSA 方式不用 AK/SK，从根源避免密钥泄露
> - 所有中间件都有权限控制，不是随便能连
> - 生产环境权限严格管控
>
> **4. 可观测性是一等公民**
> - 不是服务上线后再补监控，而是脚手架默认就全集成好了
> - 监控、日志、链路追踪三位一体，排查问题效率很高
>
> **5. 云原生最佳实践落地**
> - 没有历史包袱，从一开始就是 K8s 原生
> - 各种 K8s 最佳实践（Health Check、Probe、HPA、PDB）都用上了
> - 稳定性很高，很少因为基础设施问题出故障
>
> 当然也有可以改进的地方，比如现在 Argo Rollouts 的自动化程度还可以再提高，当前还是需要人工观察确认。但整体来说这是一套非常成熟、经过生产验证的架构。"

---

## 🚀 九、常见问题排查经验（加分项）

### 面试官问：你们线上遇到过什么典型的基础设施问题？怎么解决的？

> **可以举这几个例子：**
>
> **1. ArgoCD Readiness Probe 配置冲突**
> - 问题：脚手架默认生成了多个 Readiness Probe（HTTP + TCP），K8s 不允许
> - 报错：`Forbidden: may not specify more than 1 handler type`
> - 解决：选择适合服务的一种 Probe，删掉另一个
>
> **2. Nacos 权限问题**
> - 问题：新服务启动失败，拉取不到配置
> - 解决：Nacos 里给对应服务角色加读写权限
>
> **3. AWS IRSA 权限问题**
> - 问题：Pod 访问 S3/DDB 报权限错误
> - 排查步骤：
>   1. 检查 Pod 是否关联了正确的 ServiceAccount
>   2. 检查 IAM Role 的 Trust Policy 是否包含这个 SA
>   3. 检查 IAM Policy 是否有对应资源权限
>
> **4. Pulsar Backlog 堆积问题**
> - 问题：消费速度跟不上，消息堆积影响生产
> - 排查：Pulsar Manager 看 backlog 指标，看是消费慢还是有卡住的
> - 解决：扩容 Consumer、优化消费逻辑、限流 Producer
>
> **5. Spot 实例回收问题**
> - 问题：AWS Spot 实例被回收，服务短暂不可用
> - 解决：核心服务不用 Spot，配置 PDB 保证至少 N 个副本存活
>
> 这些都是我们实际踩过的坑，现在都有成熟的应对方案了。"

---

## 📝 面试速记清单

### 必背数字
- ArgoCD 版本：`v2.7.3`
- HTTP 端口：`8022`
- gRPC 端口：`8023`
- Metrics/Pprof 端口：`18888`
- AWS 区域：`cn-northwest-1`（宁夏区）

### 必背域名（Sandbox）
- ArgoCD: `argo.sandbox.planetmeican.com`
- Grafana: `grafana.sandbox.planetmeican.com`
- Jaeger: `tracing.sandbox.planetmeican.com`
- Kibana: `log.sandbox.planetmeican.com`
- Nacos: `nacos.sandbox.planetmeican.com`
- Pulsar Manager: `pulsar-manager.sandbox.planetmeican.com`

### 必背技术关键词
```
Go + EKS + Terraform + GitLab CI + ArgoCD + Argo Rollouts
Nacos + Pulsar + Kong + Jaeger + Grafana + Kibana
IRSA + GitOps + Canary Release + 云原生 + 微服务
```
