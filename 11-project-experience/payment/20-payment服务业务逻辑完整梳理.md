# Payment 服务业务逻辑完整梳理

## 一、业务架构总览

Payment 是美餐支付中台的核心服务，负责所有支付场景的统一调度与处理。

### 1.1 核心职责

| 职责分类 | 具体内容 |
|---------|---------|
| **支付调度** | 统一管理多种支付渠道的扣款顺序、重试、回滚 |
| **状态管理** | 通过状态机管理支付单全生命周期状态流转 |
| **渠道对接** | 对接补贴、个人余额、微信/支付宝、餐点等支付渠道 |
| **交易对账** | 生成账单数据，对接对账系统 |
| **退款处理** | 统一退款入口，支持原路退回、换单退款等 |
| **异步处理** | 通过消息队列处理回调、超时检查、回滚等 |

### 1.2 三大支付产品

| 产品类型 | 枚举值 | 适用场景 | 典型业务 |
|---------|-------|---------|---------|
| **Meican 支付** | `PaymentProductTypeMeican` | 美餐 App 下单支付 | 堂食、外卖、团餐等 |
| **Web 支付** | `PaymentProductTypeWeb` | 网页端支付 | Web 收银台、后台代付 |
| **Quick 支付** | `PaymentProductTypeQuick` | 快速支付场景 | 扫码付、付款码支付 |

### 1.3 业务场景全景图

```
美餐支付中台 (Payment 服务)
    │
    ├── 🔹 正向交易
    │       ├── 普通支付（补贴 → 余额 → 微信/支付宝）
    │       ├── 付款码支付（扫码枪）
    │       ├── 后付支付（先消费后结算）
    │       ├── 充值支付（个人余额充值）
    │       └── 餐点支付（纯餐点抵扣）
    │
    ├── 🔹 逆向交易
    │       ├── 全额退款
    │       ├── 部分退款
    │       └── 换单（旧单退 + 新单付）
    │
    ├── 🔹 异步处理
    │       ├── TPW 回调处理（微信/支付宝）
    │       ├── 支付单超时检查
    │       ├── 支付结果通知下游
    │       └── 回滚任务处理
    │
    └── 🔹 辅助能力
            ├── 风控（重复支付检查）
            ├── 幂等性保证
            └── 支付方式路由
```

---

## 二、核心业务实体模型

### 2.1 支付单 (PaymentSlip)

支付领域的核心聚合根，记录一次完整的支付请求。

```go
type PaymentSlip struct {
    // 基础标识
    Id              int64                   // 支付单ID（雪花ID）
    MchId           int64                   // 商户ID
    OutOrderNo      string                  // 业务订单号
    OutPaymentNo    string                  // 外部支付单号

    // 金额相关
    Amount          uint64                  // 支付总金额（分）
    PaidAmount      uint64                  // 已支付金额
    MealPointCount  uint64                  // 餐点数量
    Currency        types.CurrencyType      // 币种

    // 业务属性
    BizType         types.BizType           // 业务类型（堂食/外卖/团餐...）
    UserId          int64                   // 用户ID
    ClientId        int64                   // 客户ID
    CafeteriaId     int64                   // 食堂ID
    MealplanIds     []int64                 // 餐别ID列表
    StoreIds        []int64                 // 门店ID列表
    TargetTime      time.Time               // 使用时间

    // 状态控制
    Status          types.PaymentSlipStatus // 当前状态
    OptionBits      types.PaymentSlipOptions// 位运算选项（支付产品、特性等）
    PayRole         types.PayRole           // 支付人身份
    Deadline        time.Time               // 支付超时时间

    // 扩展信息
    PaymentProductInfo *vo.PaymentProductInfo // 支付产品信息
    PaymentSlipExtra   *vo.PaymentSlipExtra   // 扩展信息
    PaymentSnapshot    *vo.PaymentSnapshot    // 支付快照

    // 子单（聚合）
    SubSlips        []*PaymentSubSlip       // 支付子单列表
}
```

### 2.2 支付子单 (PaymentSubSlip)

一个支付单可能由多个支付渠道共同完成，每个渠道对应一个子单。

```go
type PaymentSubSlip struct {
    Id              int64
    PaymentSlipId   int64
    AccountType     types.AccountType      // 支付渠道类型
    AccountSubType  types.AccountSubType   // 子类型
    Amount          uint64                 // 子单金额
    PaidAmount      uint64                 // 实际支付金额
    Status          types.SubSlipStatus    // 子单状态
    TpwPayScene     types.TpwPayScene      // TPW支付场景
    TpwType         types.TpwType          // TPW类型（微信/支付宝）
    TradeNo         string                 // 第三方交易号
    CompletedAt     *time.Time             // 完成时间
}
```

### 2.3 退款单 (RefundSlip)

逆向交易核心实体。

```go
type RefundSlip struct {
    Id              int64
    PaymentSlipId   int64                   // 原支付单ID
    OutRefundNo     string                  // 外部退款单号
    Amount          uint64                  // 退款总金额
    RefundType      types.BalanceRefundType // 退款类型（全额/部分）
    Status          types.RefundSlipStatus  // 退款状态
    Reason          string                  // 退款原因
    SubSlips        []*RefundSubSlip        // 退款子单
}
```

### 2.4 换单 (ChangeSlip)

特殊业务场景：旧单退款 + 新单支付 = 换单操作。

```go
type ChangeSlip struct {
    Id              int64
    OldPaymentSlipId  int64                // 旧支付单ID
    NewPaymentSlipId  int64                // 新支付单ID
    ChangeAmount      uint64               // 换单差额
    ChangeSlipStatus  types.ChangeSlipStatus // 换单状态
}
```

### 2.5 后付单 (PayAfterSlip)

先消费后结算的特殊支付模式。

```go
type PayAfterSlip struct {
    Id              int64
    PaymentSlipId   int64
    PayerType       types.PayAfterPayerType // 付款方类型
    TotalAmount     uint64                  // 总金额
    PaidAmount      uint64                  // 已支付金额
    Status          types.PayAfterSlipStatus
    SubSlips        []*PayAfterSubSlip
}
```

---

## 三、支付模式分类

### 3.1 按触发方式分类

| 支付模式 | 触发时机 | 典型场景 | 关键特性 |
|---------|---------|---------|---------|
| **自动支付** | 创建支付单时立即触发 | 用户余额充足，自动扣款 | 异步重试、余额不足降级 |
| **手动支付** | 用户手动选择支付方式 | 余额不足时用户选微信支付 | SDK唤起、前端轮询 |
| **同步支付** | 接口调用时同步完成 | 付款码、QuickPay | 接口阻塞等待结果 |
| **异步支付** | 后台任务触发 | 后付结算、周期性扣款 | 消息驱动、最终一致 |

### 3.2 按支付渠道组合分类

| 组合模式 | 扣款顺序 | 业务场景 |
|---------|---------|---------|
| **纯内部渠道** | 补贴 → 个人余额 → 餐点 | 企业支付、员工福利 |
| **混合支付** | 内部渠道 + 微信/支付宝 | 补贴不够，用户补差额 |
| **纯三方支付** | 微信 / 支付宝 | 无福利用户 |
| **纯餐点支付** | 仅餐点抵扣 | 餐别福利 |
| **后付模式** | 先记账后结算 | 大客户月结 |

### 3.3 关键支付选项（位运算设计）

通过 `PaymentSlipOptions` 位标记控制支付行为：

```go
const (
    PaymentSlipOnlyAutoPay       = 1 << iota  // 仅自动支付（失败则终态）
    PaymentSlipNeedReturnBalance               // 需要返回支付后余额
    PaymentSlipCreateWithPayCode               // 付款码下单
    PaymentSlipSourceWeb                       // 来源 Web 端
    PaymentSlipSourceQuick                     // 来源 QuickPay
    PaymentSlipSourceQuickSync                 // 来源 QuickPay 同步模式
    PaymentSlipPayAfter                        // 后付标识
    PaymentSlipSourceChange                    // 来源换单
)
```

> **设计亮点**：用一个 uint64 的每一位表示不同的布尔选项，既节省空间，又方便组合判断。

---

## 四、三大支付产品详解

### 4.1 Meican 支付（核心产品）

**适用场景**：美餐 App 内各种下单支付

**核心特点**：
1. 优先自动支付，失败可降级手动支付
2. 支持多渠道混合支付（补贴 + 余额 + 微信）
3. 支持餐点抵扣
4. 完整的风控和幂等保证

**入口方法**：`TransactionMeican.CreatePaymentSlip()`

**支付流程分支**：

```
创建支付单
    │
    ├─ 零元支付 → 直接成功（金额=0）
    │
    ├─ 付款码支付 → 调用 TPW 扫码支付
    │   ├─ 食堂异步模式 → MQ 异步处理
    │   └─ 普通同步模式 → 接口等待
    │
    └─ 普通支付
        ├─ 仅自动支付（WithoutSDK=true）
        │   └─ 余额充足→成功，余额不足→失败（无手动选项）
        │
        └─ 自动 + 手动（WithoutSDK=false）
            └─ 先尝试自动支付
                ├─ 成功→返回
                └─ 失败→返回手动支付方式列表
```

---

### 4.2 Web 支付

**适用场景**：
- 网页版收银台
- 企业后台代员工支付
- 财务系统对接

**核心特点**：
1. 一般为手动支付模式
2. 支持帮点（代付）场景
3. 支持更灵活的支付方式选择

**入口方法**：`TransactionWeb.CreatePaymentSlip()`

---

### 4.3 Quick 支付（快速支付）

**适用场景**：
- 付款码支付（用户出示付款码，收银员扫码）
- 一码付
- 快速收银台

**核心特点**：
1. 高性能、低延迟
2. 一般为同步支付
3. 失败不重试，直接返回终态

**入口方法**：`TransactionQuick.CreatePaymentSlip()`

---

## 五、核心业务流程详解

### 5.1 普通支付完整流程

```
[入口] gRPC Handler → Application Service
    │
    ▼
1. 前置校验
   ├─ 美餐码是否已使用（防重复支付）
   ├─ 风控检查（同用户同金额短时间重复支付）
   └─ 餐点库存校验（带分布式锁）
    │
    ▼
2. 生成支付单
   ├─ 雪花算法生成 ID
   ├─ 组装 PaymentSlip 聚合根
   └─ 验证 ClientMemberPaymentRequired
    │
    ▼
3. 领域服务处理 (OnlyAutoPay / CreateAndPay)
   ├─ 检查可用支付账户余额
   └─ 调用状态机设置初始状态
    │
    ▼
4. 数据库事务提交
   ├─ 支付单落库
   └─ 发送异步任务消息
    │
    ▼
5. 异步自动支付执行（AutoPayService）
   ├─ 按优先级遍历支付渠道
   ├─ 调用各渠道 Pay 方法
   ├─ 记录子单状态
   └─ 全部成功 → 更新主单为成功
    │
    ▼
6. 支付完成后处理
   ├─ 生成账单数据
   ├─ 扣减餐点库存
   ├─ 发送支付成功 MQ 通知
   └─ 调用下游业务回调
```

---

### 5.2 余额不足处理流程

```
自动支付中 → 某渠道余额不足
    │
    ▼
1. 状态机触发事件 AutoPayInsufficient
   ├─ Quick 场景 → 直接失败
   ├─ 后付场景 → FullPayFailed（可手动补）
   ├─ 纯餐点场景 → 直接失败
   └─ 普通场景 → 查询是否有手动支付方式
       ├─ 有 → FullPayFailed（提示用户手动付）
       └─ 无 → 失败
    │
    ▼
2. 回滚已支付子单
   ├─ 按支付逆序回滚
   ├─ 同步调用各渠道 Refund 接口
   └─ 更新子单状态
    │
    ▼
3. 清理上下文
   ├─ 删除 ManualPayContext
   └─ 更新主单状态
    │
    ▼
4. 终态处理
   └─ 如果是失败 → 发 MQ 通知下游
```

---

### 5.3 手动支付流程

```
用户点击支付方式 → 调用 Pay 接口
    │
    ▼
1. 前置校验
   ├─ 支付单状态检查（必须是 New / FullPayFailed / ManualPayFailed）
   └─ 支付方式有效性检查
    │
    ▼
2. 状态机触发事件 StartManualPay
   └─ 状态变更为 ManualPay（支付中）
    │
    ▼
3. 调用 TPW 统一下单
   ├─ 微信统一下单 / 支付宝支付
   └─ 返回预支付 ID 或 SDK 参数
    │
    ▼
4. 返回给前端
   └─ 前端唤起微信/支付宝 SDK
    │
    ▼
5. 用户支付 → TPW 回调 Payment 服务
   └─ HandlePayCallback()
       ├─ 验签
       ├─ 更新子单状态
       ├─ 检查是否所有子单都成功
       └─ 是 → 更新主单为 Success
```

---

### 5.4 退款流程

```
发起退款申请
    │
    ▼
1. 创建退款单
   ├─ 生成 RefundSlip + RefundSubSlip
   └─ 退款单状态：Pending
    │
    ▼
2. 按渠道退款（逆序）
   ├─ 先退微信/支付宝（异步，等回调）
   ├─ 再退个人余额（同步）
   └─ 最后退补贴（同步）
    │
    ▼
3. 全部渠道退款成功
   ├─ 更新退款单状态为 Success
   └─ 发送退款成功 MQ
    │
    ▼
4. 支付单状态更新
   └─ 全额退款 → PaymentSlipStatusFullRefundClosed
```

---

### 5.5 换单流程（特殊业务）

**场景**：用户下单后修改订单，需要撤销原支付单，重新创建新支付单。

```
发起换单请求
    │
    ▼
1. 创建 ChangeSlip 换单记录
   ├─ OldPaymentSlipId（原单）
   └─ NewPaymentSlipId（新单）
    │
    ▼
2. 原子化处理（分布式事务）
   ├─ 第一步：旧单全额退款（不走真实渠道，仅记账）
   ├─ 第二步：创建新支付单并支付
   └─ 两步都成功才提交
    │
    ▼
3. 特殊处理
   ├─ 不重复扣减餐点库存
   ├─ 不重复发送交易事件
   └─ 只发送换单专用事件
```

---

### 5.6 后付流程

```
创建后付支付单 → PayAfter 标记
    │
    ▼
1. 下单阶段
   └─ 不扣真实钱，只记账 → 状态变为 FullPay → Success
    │
    ▼
2. 结算阶段（定时任务）
   ├─ 按结算周期聚合后付单
   ├─ 调用企业账户或个人账户扣款
   └─ 生成结算账单
    │
    ▼
3. 扣款异常处理
   └─ 企业余额不足 → 挂起，人工介入
```

---

### 5.7 充值流程

```go
// 入口：RechargeService.CreateRechargeSlip()
// 核心特点：
// - 仅支持微信/支付宝渠道
// - 充值成功后增加个人余额
// - 有独立的充值单状态机
```

---

## 六、状态机模型详解

### 6.1 支付单状态全集

| 状态 | 枚举值 | 说明 | 是否终态 |
|------|-------|------|---------|
| **New** | 0 | 初始创建 | ❌ |
| **FullPay** | 1 | 全额自动支付中 | ❌ |
| **FullPayFailed** | 2 | 自动支付余额不足（可手动付） | ❌ |
| **ManualPay** | 3 | 手动支付中 | ❌ |
| **ManualPayFailed** | 4 | 手动支付失败（可重试） | ❌ |
| **TimeoutClosed** | 5 | 超时关闭 | ✅ |
| **Success** | 6 | 支付成功 | ✅ |
| **Failed** | 7 | 支付失败（终态） | ✅ |
| **FullRefundClosed** | 8 | 全额退款关闭 | ✅ |

### 6.2 核心触发事件

| 事件 | 触发时机 |
|------|---------|
| `StartAutoPay` | 开始自动支付 |
| `StartManualPay` | 开始手动支付 |
| `AutoPayInsufficient` | 自动支付余额不足 |
| `ManualPaySuccess` | 手动支付成功 |
| `ManualPayFailed` | 手动支付失败 |
| `AutoPaySuccess` | 自动支付成功 |

### 6.3 状态流转全景图

```
           ┌─────────────────────────────────────────────┐
           │                     New (0)                 │
           └────┬──────────────────┬─────────────────────┘
                │                  │
     StartAutoPay              StartManualPay
                │                  │
                ▼                  ▼
   ┌─────────────────────┐  ┌─────────────────────┐
   │     FullPay (1)     │  │    ManualPay (3)    │
   └┬────────┬──────────┘  └┬──────────┬──────────┘
    │        │               │          │
    │        │               │          └─ ManualPaySuccess ──→ Success
    │        │               │
    │ AutoPayInsufficient    └─ ManualPayFailed ──→ ManualPayFailed
    │        │
    │        ▼
    │  ┌─────────────────────┐
    │  │ FullPayFailed (2)   │
    │  └┬────────────────────┘
    │   │
    │   └─ StartManualPay ────────────┐
    │                                  ▼
    │  AutoPaySuccess              ┌─────────────────┐
    └────────────────────────────→│  Success (6)    │
                                   └─────────────────┘

  【终态集合】 Success / Failed / TimeoutClosed / FullRefundClosed
```

> 详细状态机实现分析参见：`19-payment-fsm-深度分析.md`

---

## 七、支付渠道策略

### 7.1 支持的支付渠道

| 渠道类型 | 枚举 | 同步/异步 | 说明 |
|---------|------|----------|------|
| **补贴** | `AccountTypeSubsidy` | 同步 | 企业福利账户 |
| **个人余额** | `AccountTypePersonalBalance` | 同步 | 用户个人账户 |
| **微信支付** | `AccountTypeTpwWechat` | 异步 | TPW 对接 |
| **支付宝** | `AccountTypeTpwAlipay` | 异步 | TPW 对接 |
| **餐点** | `AccountTypeMealPoint` | 同步 | 餐别福利 |
| **折扣** | `AccountTypeDiscount` | 同步 | 营销折扣 |
| **后付** | `AccountTypePayAfter` | 异步 | 先消费后付 |

### 7.2 支付优先级排序

```go
// 扣款顺序（从左到右）
// 补贴 → 餐点 → 折扣 → 个人余额 → 微信/支付宝
var payPriority = []types.AccountType{
    types.AccountTypeSubsidy,
    types.AccountTypeMealPoint,
    types.AccountTypeDiscount,
    types.AccountTypePersonalBalance,
    types.AccountTypeTpwWechat,
    types.AccountTypeTpwAlipay,
}
```

### 7.3 渠道接口设计（策略模式）

```go
type IPayChannel interface {
    // 支付
    Pay(ctx context.Context, param *PayParam) (*PayResult, error)
    // 退款
    Refund(ctx context.Context, param *RefundParam) (*RefundResult, error)
    // 查询
    Query(ctx context.Context, param *QueryParam) (*QueryResult, error)
}
```

各渠道实现各自的 Pay/Refund/Query 方法，核心服务不关心具体实现。

---

## 八、异步消息处理机制

### 8.1 消息主题全集

| 消息类型 | 用途 | 消费者 |
|---------|------|--------|
| `AsyncTaskV2` | 异步支付任务 | `AutoPayConsumer` |
| `TpwResultNotify` | TPW 支付结果回调 | `TpwCallbackConsumer` |
| `PaymentSlipTimeoutCheckV2` | 支付单超时检查 | `TimeoutCheckConsumer` |
| `TpwRollback` | TPW 回滚任务 | `RollbackConsumer` |
| `SubsidyRollback` | 补贴回滚 | `RollbackConsumer` |
| `TradeEvent` / `TradeEventV2` | 交易事件通知下游 | 订单、对账等 |
| `ResultNotify` | 支付结果通知 | 调用方 |
| `PaymentBilling` | 支付账单 | 对账系统 |

### 8.2 超时检查机制

```
定时轮询 + 延迟消息双重保证
    │
    ├─ 轮询任务：每分钟扫描超时未支付的订单
    │   └─ 超过 Deadline 未完成 → 状态置为 TimeoutClosed
    │
    └─ 延迟消息：创建支付单时发送 N 分钟后的延迟消息
        └─ 作为兜底，防止轮询遗漏
```

---

## 九、核心设计模式与技术亮点

### 9.1 设计模式应用

| 模式 | 应用场景 |
|------|---------|
| **状态机模式** | 支付单/退款单状态流转 |
| **策略模式** | 支付渠道实现、不同产品的支付完成处理 |
| **责任链模式** | 支付渠道按优先级依次扣款 |
| **聚合根模式** | PaymentSlip 作为聚合根，操作都通过它 |
| **工厂模式** | 不同支付产品的 service 创建 |
| **观察者模式** | 支付成功后通知多个下游系统 |

### 9.2 技术亮点

1. **位运算选项设计**：一个字段表示 N 个布尔属性，节省空间，组合灵活
2. **事务回调函数**：`func(ctx context.Context) error` 在事务提交后执行，保证最终一致性
3. **闭包捕获上下文**：状态机的次态计算函数通过闭包访问外部依赖
4. **流式 API 设计**：`.event().from().cal()` 链式调用定义状态转移
5. **丰富的状态断言方法**：`IsPaying()`、`CanCallPay()`、`NeedManualPay()` 等，业务语义清晰

---

## 十、代码导航索引

### 10.1 核心入口

| 功能 | 文件路径 |
|------|---------|
| Meican 支付入口 | `application/svc/transaction_meican.go` |
| Web 支付入口 | `application/svc/transaction_web.go` |
| Quick 支付入口 | `application/svc/transaction_quick.go` |
| 退款入口 | `domain/transaction/svc/internal/refund_core.go` |
| 换单入口 | `domain/transaction/svc/internal/change.go` |

### 10.2 核心领域服务

| 功能 | 文件路径 |
|------|---------|
| 自动支付服务 | `domain/transaction/svc/internal/auto_pay.go` |
| 手动支付服务 | `domain/transaction/svc/internal/manual_pay.go` |
| 支付核心服务 | `domain/transaction/svc/internal/payment_core.go` |
| 状态机定义 | `domain/transaction/svc/internal/fsm.go` |
| 支付渠道接口 | `domain/transaction/svc/internal/pay_channel/pay_channel.go` |

### 10.3 实体定义

| 实体 | 文件路径 |
|------|---------|
| 支付单 | `domain/transaction/entity/payment_slip.go` |
| 退款单 | `domain/transaction/entity/refund_slip.go` |
| 换单 | `domain/transaction/entity/change_slip.go` |
| 后付单 | `domain/transaction/entity/pay_after_slip.go` |

### 10.4 消息消费者

| 消费者 | 文件路径 |
|--------|---------|
| 自动支付消费者 | `application/consumer/consumer.go` |
| TPW 回调消费者 | `infra/mq/consumer/tpw_callback.go` |
| 超时检查消费者 | `infra/mq/consumer/timeout_check.go` |

---

## 十一、关键业务判断速查表

### 11.1 支付状态判断

```go
// 是否正在支付中？
ps.Status.IsPaying()  // FullPay 或 ManualPay

// 是否可以发起支付？
ps.Status.CanCallPay()  // New / FullPayFailed / ManualPayFailed

// 是否终态？
ps.Status.IsCompleted()  // Success / Failed / TimeoutClosed / FullRefundClosed

// 是否需要用户手动支付？
ps.Status.NeedManualPay()  // FullPayFailed / ManualPayFailed
```

### 11.2 支付产品判断

```go
// 支付产品类型
ps.OptionBits.GetPaymentProductType()  // Meican / Web / Quick

// 来源判断
ps.OptionBits.IsSourceWeb()
ps.OptionBits.IsSourceQuick()
ps.OptionBits.IsSourceQuickSync()

// 支付模式
ps.OptionBits.OnlyAutoPay()      // 仅自动支付
ps.OptionBits.IsPayAfter()       // 后付
ps.OptionBits.CreateWithPayCode() // 付款码
```

---

> 文档生成时间：2025-05-16
> 基于代码版本：Payment Service 最新主分支
