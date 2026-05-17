# Payment 核心流程深度解析：支付、回滚、退款与状态机

## 一、先看一个完整的支付例子（带金额）

### 场景设定
- **订单金额**：45 元
- **可用账户**：
  - 补贴账户：30 元
  - 个人余额：10 元
  - 微信支付：绑定可用

### 支付流程图

```
订单金额 45 元
    ↓
【第一步：按优先级扣款】
    ├─ 补贴账户扣 30 元（全部扣完）
    ├─ 个人余额扣 10 元（全部扣完）
    └─ 剩余 5 元需要微信支付
    ↓
【第二步：状态机流转】
    ├─ New → StartAutoPay → FullPay
    └─ 开始自动支付流程
    ↓
【第三步：并行执行】
    ├─ 补贴 30 元（同步，立即成功）
    ├─ 个人余额 10 元（同步，立即成功）
    └─ 微信 5 元（异步，唤起前端 SDK）
    ↓
【第四步：用户支付确认】
    ├─ 用户点击支付 → 微信回调成功
    └─ 回调 Payment 服务
    ↓
【第五步：最终状态】
    └─ FullPay → AutoPaySuccess → Success
```

---

## 二、支付渠道优先级与金额分配

### 2.1 扣款优先级排序（从高到低）

```go
// 优先级从高到低：
// 1. 餐点（MealPoint）
// 2. 补贴（Subsidy）
// 3. 折扣（Discount）
// 4. 个人余额（PersonalBalance）
// 5. 微信支付（Wechat）
// 6. 支付宝（Alipay）
```

### 2.2 金额分配逻辑详解

**核心代码位置**：`auto_pay.go` 中的 `runExecutePlanner()` → `payPlanner.Generate()`

```go
// 伪代码：金额分配器
func (p *autoPayExecutePlanner) Generate(needPayAmount uint64, isDeductToZero bool) bool {
    for _, account := range p.PayableAccounts {
        // 1. 判断是否为内部渠道（补贴/余额/餐点）
        if account.AccountType.IsInternalChannel() {
            // 内部渠道：能扣多少扣多少
            if account.Balance >= needPayAmount {
                deduct = needPayAmount
                needPayAmount = 0
            } else {
                deduct = account.Balance
                needPayAmount -= account.Balance
            }
        } else {
            // 外部渠道（微信/支付宝）：要么全扣，要么不扣
            if needPayAmount > 0 {
                deduct = needPayAmount
                needPayAmount = 0
            }
        }

        // 记录本次要扣的金额
        p.PayAccounts = append(p.PayAccounts, &PayAccount{
            AccountType: account.AccountType,
            Amount:     deduct,
        })
    }

    return needPayAmount == 0
}
```

### 2.3 关键设计点

| 渠道类型 | 扣款策略 | 原因 |
|---------|---------|------|
| **内部渠道**（补贴/余额/餐点） | 能扣多少扣多少，扣完为止 | 内部系统，实时到账，支持部分扣 |
| **外部渠道**（微信/支付宝） | 要么全扣，要么不扣 | 支付体验，用户不想付两次 |

> 💡 **为什么外部渠道不能部分扣？**
>
> 假设订单 45 元，微信只有 3 元余额：
> - 微信扣 3 元，还差 2 元 → 用户还要再用支付宝付 2 元
> - 体验极差！用户会说："我明明点了一次支付，为什么扣了两次钱？"

---

## 三、有/无手动支付方式的分支处理

### 3.1 状态机关键判断逻辑

**核心代码位置**：`fsm.go` 中 `AutoPayInsufficient` 事件处理

```go
// 自动支付余额不足时的判断
fsm.Register((&paymentFsmModel{}).
    event(types.AutoPayInsufficient).
    from(types.PaymentSlipStatusFullPay).
    cal(func(ctx context.Context, ps *entity.PaymentSlip, scene commonv2pb.Scene, paymentCore PaymentCore) (types.PaymentSlipStatus, error) {

        // 分支 1：仅自动支付场景 / Quick场景 → 直接失败
        if ps.OnlyAutoPay() || ps.IsSourceQuick() {
            return types.PaymentSlipStatusFailed, nil
        }

        // 分支 2：后付场景 → 全额支付失败（可手动补）
        if ps.IsPayAfter() {
            return types.PaymentSlipStatusFullPayFailed, nil
        }

        // 分支 3：纯餐点支付 → 直接失败
        if ps.IsMealPointPay() {
            return types.PaymentSlipStatusFailed, nil
        }

        // 分支 4：普通场景 → 查询有没有手动支付方式
        payMethodSet, err := paymentCore.GetPaymentMethods(ctx, ps, scene, false, pscommonpb.PayType_MANUAL_PAY)
        if err != nil {
            return 0, err
        }

        // 有手动支付方式 → FullPayFailed（提示用户手动付）
        if payMethodSet.HasMealplanManualPayMethods() {
            return types.PaymentSlipStatusFullPayFailed, nil
        }

        // 啥都没有 → 直接失败
        return types.PaymentSlipStatusFailed, nil
    }))
```

### 3.2 完整状态流转全景图

```
初始状态：New
    │
    ├─ 【StartAutoPay】 → FullPay（自动支付中）
    │       │
    │       ├─ 全部成功 ── AutoPaySuccess ──→ Success ✅
    │       │
    │       └─ 余额不足 ── AutoPayInsufficient ──┐
    │                                                │
    │                    ┌──────────────────────────┴──────────────────────────┐
    │                    │                        │                             │
    │              仅自动支付/Quick         有手动支付方式                  无手动支付方式
    │                    │                        │                             │
    │                    ↓                        ↓                             ↓
    │                 Failed               FullPayFailed                    Failed
    │                                             │
    │                                             └─ 用户点击手动支付 ── ManualPay
    │                                                              │
    │                                                              ├─ 支付成功 ── ManualPaySuccess ──→ Success ✅
    │                                                              └─ 支付失败 ── ManualPayFailed ──→ ManualPayFailed
    │
    └─ 【StartManualPay】→ ManualPay（手动支付中）
            │
            ├─ 支付成功 ── ManualPaySuccess ──→ Success ✅
            └─ 支付失败 ── ManualPayFailed ──→ ManualPayFailed
```

### 3.3 两种失败状态的区别

| 状态 | 含义 | 用户可操作 |
|------|------|-----------|
| **Failed** | 终态失败 | 不能再支付，需要重新下单 |
| **FullPayFailed / ManualPayFailed** | 中间态失败 | 用户可以选择其他支付方式重试 |

> 💡 **产品设计思考**：
>
> - 自动支付失败但有手动方式 → 给用户一次"补救"机会，降低弃单率
> - 自动支付失败且无手动方式 → 直接失败，避免用户等待

---

## 四、回滚机制详解（核心！）

### 4.1 什么时候会触发回滚？

| 触发场景 | 原因 | 回滚范围 |
|---------|------|---------|
| 自动支付余额不足 | 内部渠道扣了部分钱，外部渠道扣不了 | 已扣款的子单全部回滚 |
| TPW 回调失败 | 微信/支付宝支付失败 | 失败的那个子单 |
| 超时未支付 | 支付单过期自动关闭 | 所有已扣款的子单 |
| 退款 | 用户主动申请退款 | 按退款金额回滚对应子单 |

### 4.2 回滚执行流程（以余额不足为例）

**代码位置**：`rollback_pay.go` → `RollbackPaymentSubSlip()`

```go
// 伪代码：回滚流程
func (r *rollbackPayService) RollbackPaymentSubSlip(ctx context.Context, subSlips ...*entity.PaymentSubSlip) error {
    // 场景：补贴扣了 30 元，余额扣了 10 元，微信扣不了
    // 需要回滚补贴 30 元 + 余额 10 元

    // 步骤 1：分类处理
    for _, subSlip := range subSlips {
        if subSlip.Status.IsFailed() {
            // 失败的直接删除即可，不用真回滚
            deletePaymentSubSlipIds = append(deletePaymentSubSlipIds, subSlip.Id)
            continue
        }

        if subSlip.Status.IsSucceeded() {
            // 成功的需要创建回滚单 + 发回滚消息
            rollbackSlips = append(rollbackSlips, buildRollbackSlip(subSlip))
            slipDecrAmountMap[subSlip.SlipId] += subSlip.EffectiveAmount
        }
    }

    // 步骤 2：数据库事务（原子性保证！）
    return xormv2.Transaction(ctx, func(ctx context.Context) error {
        // 2.1 批量创建回滚单（RollbackSlip）
        if err := r.PaymentSlipRepo.BatchCreateRollbackSlip(ctx, rollbackSlips); err != nil {
            return err
        }

        // 2.2 减少支付单已支付金额
        for slipId, decrAmount := range slipDecrAmountMap {
            if err := r.PaymentSlipRepo.DecrPaymentSlipPaidAmount(ctx, slipId, decrAmount); err != nil {
                return err
            }
        }

        // 2.3 发送各渠道回滚消息（最终一致性）
        if _, err := r.MQProducer.SendWithTx(ctx, buildRollbackMsg(rollbackSlips)...); err != nil {
            return err
        }

        // 2.4 删除子支付单（物理删除？软删除？看具体实现）
        return r.PaymentSlipRepo.BatchDeletePaymentSubSlip(ctx, deletePaymentSubSlipIds)
    })
}
```

### 4.3 各渠道回滚方式

| 渠道 | 回滚方式 | 同步/异步 | 说明 |
|------|---------|----------|------|
| **补贴** | 发 MQ 消息给 subsidy 服务 | 异步 | subsidy 消费后加回余额 |
| **个人余额** | 发 MQ 消息给 balance 服务 | 异步 | balance 消费后加回余额 |
| **微信/支付宝** | 发 MQ 消息给 TPW 服务 | 异步 | TPW 调用微信退款接口 |
| **餐点** | 不直接回滚（特殊处理） | - | 通过改单流程处理 |

### 4.4 关键设计：为什么用 MQ 异步回滚，而不是同步 RPC？

```
方案对比：

【方案 A：同步 RPC 回滚】
Payment → 同步调用 subsidy.Rollback() → 同步调用 balance.Rollback()
问题：
  - 任何一个服务挂了 → 回滚失败 → 数据不一致
  - 事务时间长 → 数据库连接占着 → 性能差
  - 耦合严重

【方案 B：MQ 异步回滚】（实际采用）
Payment → 发 MQ 消息到各自 Topic
问题：
  - 消息可能丢失（但有重试机制 + 死信队列）
优点：
  ✅ 解耦
  ✅ 各服务独立重试
  ✅ 事务短（只发消息，不等结果）
  ✅ 最终一致性保证
```

---

## 五、退款功能详解（不只是支付成功才能退！）

### 5.1 退款的 4 种场景

| 场景 | 支付单状态 | 退款方式 | 说明 |
|------|-----------|---------|------|
| **场景 1** | Success（支付成功） | 正常退款 | 最常见，原路退回 |
| **场景 2** | FullPay（自动支付中） | 回滚 + 退款 | 用户中途取消，需要回滚已扣的内部渠道 |
| **场景 3** | ManualPay（手动支付中） | 回滚 + 退款 | 同上 |
| **场景 4** | 换单场景 | 旧单全额退 + 新单重新付 | 用户改地址/改菜品 |

### 5.2 全额退款流程（支付成功后）

**代码位置**：`refund_core.go` → `FullRefund()`

```go
// 伪代码：全额退款流程
func (t *refundService) FullRefund(ctx context.Context, param *dto.CreateRefundSlipParams, ...) (int64, bizerr.Error) {
    // 步骤 1：幂等校验（OutRefundNo 是否已用）
    if rf, err := t.refundRepo.FindSlipByMchAndOutRefundNo(ctx, param.MchId, param.OutRefundNo); err == nil {
        return rf.Id, nil  // 已创建过，直接返回（幂等）
    }

    // 步骤 2：获取所有支付单 + 退款单
    paymentSlips, err := t.paymentRepo.FindPaymentSlipsByMchIdAndOutOrderNo(ctx, param.MchId, param.OutOrderNo)
    refundSlips, err := t.refundRepo.FindByMchIdAndOutOrderNo(ctx, param.MchId, param.OutOrderNo)

    // 步骤 3：校验是否已全额退款
    if refundSlips.IsAllFullRefund() {
        return 0, bizerr.New(RESULT_CODE_UNAVAILABLE_REFUNDABLE_AMOUNT)
    }

    // 步骤 4：按支付渠道逆序退款（先退微信，再退余额，最后退补贴）
    // 原因：外部渠道退款成功率低，先处理难的
    // 如果外部渠道退款失败，内部渠道就不用退了，避免部分退款的尴尬

    // 步骤 5：创建退款单（RefundSlip）+ 退款子单（RefundSubSlip）
    // 状态：Pending（退款中）

    // 步骤 6：调用各渠道退款接口
    for _, subSlip := range refundSubSlips {
        result, err := t.payChannel.Refund(ctx, subSlip.AccountType, ...)
        if err != nil {
            // 某个渠道退款失败，标记退款单状态
            return 0, err
        }
    }

    // 步骤 7：所有渠道退款成功 → 标记退款单为 Success
    // 步骤 8：如果是全额退款 → 支付单状态变为 FullRefundClosed（终态）
}
```

### 5.3 退款的逆序处理（关键设计！）

```
支付顺序（从左到右）：
补贴 → 个人余额 → 微信

退款顺序（从右到左）：
微信 → 个人余额 → 补贴

为什么逆序？
├─ 外部渠道（微信）退款成功率低，先处理难的
├─ 如果微信退款失败，内部渠道就不用退了
└─ 避免"微信退失败，补贴退成功了"的尴尬局面
```

### 5.4 部分退款的特殊处理

- 按比例分配到各渠道（用户付了 30 补贴 + 15 微信，退 15 元 → 按 2:1 比例退）
- 商品级退款（只退某个菜品，需要关联到具体子单）
- 部分退款后，支付单状态仍为 Success（不是终态失败）

---

## 六、兜底状态处理与一致性保证

### 6.1 状态不一致的可能场景

| 场景 | 原因 | 处理方式 |
|------|------|---------|
| 支付回调超时 | TPW 回调没收到，支付单状态停在 ManualPay | 定时任务轮询 + 主动查单 |
| 回滚消息丢失 | MQ 消息丢了，钱扣了但没加回去 | 对账系统定期核对 |
| 部分退款成功 | 微信退成功了，补贴退失败 | 人工介入 + 补偿任务 |
| 服务中途挂了 | 创建子单后，Payment 服务重启了 | 定时任务扫超时单 + 回滚 |

### 6.2 定时任务兜底

**核心任务 1：支付单超时检查**
```go
// 每分钟运行一次
// 找出所有超过 N 分钟未完成的支付单
// 状态：FullPay / ManualPay 超过 15 分钟
// 处理：自动关闭 + 回滚已扣款
```

**核心任务 2：退款单超时检查**
```go
// 找出超过 30 分钟仍在 Pending 的退款单
// 处理：主动调用渠道查单，更新状态
```

### 6.3 对账系统（最终防线）

```
T + 1 日对账流程：
    1. 拉取微信/支付宝前一天的对账单
    2. 拉取 Payment 服务前一天的支付/退款记录
    3. 两边逐笔核对：
       ├─ 两边都有且金额一致 → 正常
       ├─ 支付有，微信没有 → 支付异常，可能掉单
       ├─ 微信有，支付没有 → 长款，需要核实
       └─ 金额不一致 → 异常，人工介入
```

---

## 七、与其他系统的交互方式汇总

### 7.1 RPC 同步调用

| 调用方 | 目标服务 | 用途 |
|-------|---------|------|
| Payment | ClientSetting | 查询可用支付方式、支付配置 |
| Payment | Subsidy | 查询补贴余额、扣款（同步） |
| Payment | PersonalBalance | 查询余额、扣款（同步） |
| Payment | MealPoint | 查询餐点余额、扣餐点（同步） |
| Payment | TPW | 统一下单（获取微信参数）、查询订单状态 |

### 7.2 MQ 异步消息

| Topic | 用途 | 消费者 |
|-------|------|--------|
| `PulsarTpwRollback` | TPW 渠道回滚任务 | TPW 服务 |
| `PulsarSubsidyRollback` | 补贴回滚任务 | Subsidy 服务 |
| `PulsarMsgToPersonalBalance` | 余额回滚任务 | Balance 服务 |
| `PulsarMealPointRollback` | 餐点回滚任务 | MealPoint 服务 |
| `PulsarAsyncTaskV2` | 自动支付异步任务 | Payment 服务自身 |
| `PulsarPaymentSlipTimeoutCheckV2` | 支付单超时检查 | 定时任务触发 |
| `PulsarTradeEvent` / `PulsarTradeEventV2` | 交易事件通知下游 | 订单、对账、财务等 |
| `PulsarPaymentBilling` | 支付账单数据 | 对账系统 |

### 7.3 回调通知（Webhook）

| 方向 | 触发时机 | 说明 |
|------|---------|------|
| TPW → Payment | 微信/支付宝支付成功/失败 | 用户支付完成后，微信回调 TPW，TPW 再回调 Payment |
| Payment → 业务方 | 支付成功/失败/退款 | 通过 MQ 或 HTTP 回调通知订单系统等 |

---

## 八、完整的支付生命周期时间线

以用户下单支付为例，按时间顺序：

```
T0    用户点击下单 → 创建支付单 → 状态 New

T0+1  调用 StartAutoPay → 状态 New → FullPay
      ↓
      批量创建子单：
      ├─ 补贴 30 元 → Pending
      ├─ 余额 10 元 → Pending
      └─ 微信 5 元 → Pending

T0+2  并行调用各渠道支付
      ├─ 补贴 30 元 → 同步成功（RPC 返回）
      ├─ 余额 10 元 → 同步成功（RPC 返回）
      └─ 微信 5 元 → 返回 SDK 参数（异步，等用户支付）

T0+3  前端唤起微信支付 SDK，用户输入密码

T0+N  用户支付完成 → 微信回调 TPW → TPW 回调 Payment
      ↓
      更新微信子单状态为 Success
      检查所有子单是否都成功 → 是
      更新支付单主状态为 Success
      ↓
      发送支付成功 MQ → 下游系统处理
      ↓
      返回前端：支付成功

T0+N+M 用户申请退款 → 创建退款单 → 按逆序退款
      ↓
      微信 5 元 → 调用微信退款 API（异步）
      余额 10 元 → 发 MQ 回滚
      补贴 30 元 → 发 MQ 回滚
      ↓
      全部成功 → 退款单状态 Success
```

---

## 九、关键设计模式总结

| 模式 | 应用场景 | 优点 |
|------|---------|------|
| **状态机模式** | 支付单/退款单状态流转 | 避免 `if/else` 地狱，状态变更集中管理 |
| **策略模式** | 不同支付渠道的 Pay/Refund 实现 | 新增渠道只需加实现，不改核心 |
| **责任链模式** | 按优先级依次扣款 | 渠道间解耦，顺序可配置 |
| **最终一致性** | 跨服务扣款、回滚 | 不追求强一致，保证最终一致，性能高 |
| **幂等设计** | 所有外部接口（创建支付/退款/回调） | 重复调用不产生副作用 |
| **补偿机制** | 定时任务+对账 | 异常场景兜底，99.99% 一致性保证 |

---

## 十、你可能忽略的 5 个细节

### 1. 为什么回滚时要物理删除子单，而不是标记状态？
> 因为一个支付单可能会有多次支付尝试（自动失败 → 手动再付），每次尝试都生成新的子单，如果不删除，子单会越来越多，查询越来越慢。

### 2. 为什么有 RollbackSlip 表？和 RefundSlip 有什么区别？
> - RollbackSlip：支付过程中（非终态）的回滚（如余额不足）
> - RefundSlip：支付成功后的主动退款
> 两个表分开，业务语义更清晰，对账也方便。

### 3. 为什么支付渠道调用用并发？
> 看 `BatchPay()` 里的 `safego.Go()` + `sync.WaitGroup`
> 5 个渠道串行调用要 500ms × 5 = 2500ms
> 5 个渠道并发调用只要 Max(500, 300, 200...) = 500ms
> 用户体验提升 5 倍！

### 4. 为什么余额不足时不直接失败，还要先扣了再回滚？
> 因为**余额是并发安全的**，但查询和扣款之间有时间差（查询时余额够，扣款时可能被另一个订单扣走了）
> 所以只能先尝试扣，扣成功了继续，不够再回滚。这是分布式系统的典型"乐观锁"思路。

### 5. 为什么退款状态更新不依赖回调，而是有定时任务查？
> 因为回调可能丢！微信回调 3 次都失败（比如 Payment 服务当时挂了），就再也收不到了。
> 所以必须有定时任务主动去渠道查单，这是最终一致性的兜底。

---

> 文档生成时间：2025-05-16
> 基于代码版本：Payment Service v3
> 核心文件：`auto_pay.go`、`rollback_pay.go`、`refund_core.go`、`fsm.go`
