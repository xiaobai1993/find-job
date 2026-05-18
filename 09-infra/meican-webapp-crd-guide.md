# WebApp CRD - 声明式资源管理

> 📌 美餐自研 K8s Custom Resource Definition，一键声明式创建所有中间件资源
> 🔗 项目地址：nerds/tower.v2

---

## 一、什么是 WebApp？

### 面试官问：你们怎么管理应用的中间件资源？比如 Nacos 账号、Pulsar Topic？

> **标准答案：**
>
> "我们有自研的 **WebApp CRD**（Custom Resource Definition），完全声明式管理所有中间件资源：
>
> **它能做什么：**
> 1. ✅ 自动创建 Nacos 用户、角色、权限
> 2. ✅ 自动创建 Pulsar Topic、订阅、权限
> 3. ✅ 自动生成 Grafana Dashboard
> 4. ✅ 应用注册与元数据管理
>
> **工作原理：**
> - 开发人员只需要写一个 `webapp.yaml` 文件
> - 提交到 Git，通过 ArgoCD 同步到 K8s
> - Tower Operator（我们自研的控制器）自动监听 WebApp 资源变更
> - 自动调用各中间件 API 创建资源，不需要人工登录控制台操作
>
> **最大的好处：**
> - 基础设施即代码（IaC），所有变更可追溯、可审计
> - 不需要给开发人员中间件的管理员权限，安全
> - 新服务上线一键配置所有资源，效率高
> - 权限统一管理，避免乱开权限"

---

## 二、完整配置示例

```yaml
apiVersion: nerds.k8s.meican.com/v1alpha1
kind: WebApp
metadata:
  name: log-stream
  namespace: app-titan
  labels:
    group: "titan"
    project: "log-stream"
    subset: "main"
spec:
  group: titan
  project: log-stream
  subset: main

  # Pulsar MQ 配置
  mq:
    namespace: titan
    tenant: public
    topics:
      - name: log-stream-async
        partitions: 1
        subjects:
          - subject: titan-pagerduty
            actions: [ Consume ]  # 给其他服务消费权限
      - name: log-stream-event
        partitions: 1
        subjects:
          - subject: titan-pagerduty
            actions: [ Consume ]
```

---

## 三、自动创建的资源详解

提交上述 YAML 后，Tower Operator 会自动创建：

### 1. Nacos 资源
```
✅ 用户: titan-log-stream-main
✅ 角色: titan-log-stream-main-r
✅ 权限: titan namespace 下 log-stream group 的只读权限
```

### 2. Pulsar 资源
```
✅ Topic 1: persistent://public/titan/log-stream-async (1分区)
✅ Topic 2: persistent://public/titan/log-stream-event (1分区)
✅ Subject: titan-log-stream-main（对两个topic有生产+消费权限）
✅ 额外授权: titan-pagerduty 对两个 topic 有消费权限
```

### 3. Grafana 资源
```
✅ 在 Apps 目录下创建 titan/log-stream 文件夹
✅ 自动生成标准化 Go 服务监控模板
✅ 开发可以基于模板二次定制业务指标
```

---

## 四、核心字段说明

| 字段 | 必填 | 说明 |
|-----|------|------|
| `group` | ✅ | 业务组，对应 K8s namespace 前缀 app-{group} |
| `project` | ✅ | 项目名，应用唯一标识 |
| `subset` | ❌ | 子服务标识，同一个应用下的不同服务（如 server/agent） |
| `mq.topics[].name` | ❌ | Topic 名称 |
| `mq.topics[].partitions` | ❌ | 分区数 |
| `mq.topics[].subjects` | ❌ | 给其他服务授权消费 |

---

## 五、设计理念总结

### 1. 权限遵循"最小可用"原则
> WebApp 创建的用户默认只有自己服务的最小权限，需要跨服务消费时，在 `subjects` 里显式声明。

### 2. 谁创建谁负责原则
> 创建 Topic 的服务自动拥有 Produce + Consume 权限，其他服务需要显式授权。

### 3. 完全声明式，不可变
> 所有资源变更都走 Git MR，有 review 记录，出了问题可以回溯。

---

## 🎯 面试记忆点

```
CRD + Operator = 自动化
├── Nacos 账号权限自动创建
├── Pulsar Topic + 权限自动创建
├── Grafana Dashboard 自动生成
└── 所有变更 GitOps 化，可追溯、可审计
```
