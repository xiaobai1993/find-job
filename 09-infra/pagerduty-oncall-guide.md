# PagerDuty 告警与 OnCall 值班管理

> 📌 美餐自研 K8s CRD，声明式配置告警触达与值班规则
> 🔗 CRD 类型：pagerduty.k8s.meican.com/v1alpha1

---

## 一、整体架构

### 面试官问：你们线上告警怎么触达开发？OnCall 怎么管理？

> **标准答案：**
>
> "我们用 PagerDuty 做告警收敛和值班管理，并且实现了 K8s CRD 声明式配置：
>
> **整体流程：**
> 1. Prometheus 告警 → AlertManager → 自研 PagerDuty Operator
> 2. Operator 根据 CRD 配置匹配告警级别
> 3. 根据 OnCall 规则自动路由到对应的值班人
> 4. 支持多级触达：企业微信 → 邮件 → 电话
>
> **核心特点：**
> - ✅ 所有配置都是 YAML，GitOps 管理
> - ✅ 按级别分级触达，信息不打扰开发
> - ✅ OnCall 排班声明式配置
> - ✅ 历史告警可追溯，可复盘"

---

## 二、完整配置示例

```yaml
apiVersion: pagerduty.k8s.meican.com/v1alpha1
kind: PagerDuty
metadata:
  name: pulsar-broker
  namespace: pulsar
  finalizers:
    - "pagerduty.k8s.meican.com"  # 保护，删 CRD 前先删配置
spec:
  group: "pulsar"  # 和项目 namespace 对应

  # 标签选择器：匹配集群内的事件
  labels:
    app: "pulsar"  # 匹配所有 app=pulsar 的告警

  # ArgoCD 链接（告警里带跳转地址）
  argoUrl: "https://argo.planetmeican.com/applications/pulsar"

  # 🚨 通知规则：按严重程度分级触达
  notificationRules:
    - severity: "info"
      wecomWebhook:
        - url: "https://qyapi.weixin.qq.com/xxx"  # 只发企业微信

    - severity: "warn"
      wecomWebhook:
        - url: "https://qyapi.weixin.qq.com/xxx"
      email: true  # 加邮件

    - severity: "critical"
      wecomWebhook:
        - url: "https://qyapi.weixin.qq.com/xxx"
      email: true
      phone: true  # 严重时打电话！

  # 📅 OnCall 值班规则
  onCallRules:
    - users: ["zhangsan", "lisi", "wangwu"]
      weekdays: ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday"]  # 工作日轮值
    - users: ["lisi"]
      weekdays: ["Saturday", "Sunday"]  # 周末 lisi 值班

  # ☎️ 联系人信息
  contacts:
    - name: "zhangsan"
      email: "zhangsan@meican.com"
      phone: "+8613800138000"
    - name: "lisi"
      email: "lisi@meican.com"
      phone: "+8613900139000"
    - name: "wangwu"
      email: "wangwu@meican.com"
      phone: "+8613700137000"
```

---

## 三、核心字段详解

### 1. labels - 事件匹配

采用 K8s Label Selector 设计模式：

```yaml
# 例子：Pulsar Broker 组件的标签
labels:
  app: pulsar
  release: pulsar
  cluster: fan2-pulsar0
  component: broker

# 例子：Bookie 组件的标签
labels:
  app: bookkeeper
  component: bookie

# 例子：一个 CRD 管所有 Pulsar 相关告警
labels:
  app: pulsar
```

> 💡 好处：不同组件的告警可以聚合到同一个 PagerDuty CRD 管理。

### 2. notificationRules - 分级触达

| 级别 | 触达方式 | 适用场景 |
|-----|---------|---------|
| `info` | 仅企业微信 | 通知类，比如发布成功 |
| `warn` | 企业微信 + 邮件 | 警告类，比如 P99 升高但还没影响业务 |
| `critical` | 企业微信 + 邮件 + 电话 | 紧急，服务挂了，支付不可用 |

### 3. onCallRules - 值班规则

```yaml
# 按周轮值
onCallRules:
  - users: ["zhangsan"]
    weekdays: ["Monday", "Wednesday", "Friday"]
  - users: ["lisi"]
    weekdays: ["Tuesday", "Thursday"]

# 按周轮换（需要 Operator 支持更复杂的逻辑）
# 实际我们是按月排好班的
```

---

## 四、告警最佳实践

### 1. 告警分级原则

> **告警不是越多越好，而是越准越好！**

| 级别 | 定义 | 响应时间 |
|-----|------|---------|
| P0 Critical | 核心业务不可用（支付失败、用户打不开） | 15 分钟内响应 |
| P1 Warning | 非核心业务异常，有降级方案 | 1 小时内响应 |
| P2 Info | 通知类，发布、扩缩容 | 上班看一眼就行 |

### 2. 告警收敛

> ❌ 不要一个 Pod OOM 了给所有开发打电话！
> ✅ 应该：
> - 1 分钟内同类告警超过 N 条才触发
> - 持续 N 分钟异常才发电话告警
> - 工作时间和非工作时间阈值不同

### 3. 告警必须带上下文

> 好的告警应该直接告诉开发怎么处理：
> ```
> 【P0 告警】支付服务错误率超过 50%
> 影响：用户支付失败
> Grafana: https://grafana.xxx/d/payment
> ArgoCD: https://argo.xxx/app/payment
> 排查建议：1. 看最近发布 2. 看 Pulsar 3. 看数据库
> ```

---

## 五、设计亮点

### 1. CRD + GitOps

> 所有配置都在 Git 里，变更走 MR，有 Review，有历史记录。

### 2. 配置即文档

> 看 YAML 就知道谁在值班，谁管什么业务，告警怎么发。

### 3. 统一管理

> 所有业务的告警配置方式一致，没有平台差异。

---

## 🎯 面试记忆点

```
PagerDuty CRD = 声明式告警 + OnCall 管理
├── Label Selector 匹配事件
├── 三级触达：信息→邮件→电话
├── 排班配置化，Git 管理
└── 告警带上下文，方便快速排查
```
