# Payment 服务状态机 FSM 深度分析

## 一、整体架构概览

### 设计哲学

这不是教科书式的"纯"状态机，而是**业务驱动的工程化实现**。核心特点：
- **无 entry/exit 动作**：状态变更的副作用由调用方处理
- **动态次态计算**：次态不是固定的，由业务闭包实时计算
- **线性查找匹配**：30+ 转移规则，逐个匹配，简单可靠

---

## 二、核心数据结构分析

```go
// 单个状态转移规则
type paymentFsmModel struct {
    currentStatus     types.PaymentSlipStatus  // 现态
    payEvent          types.PaymentSlipEvent   // 触发事件
    calNextStatusFun  func(...)                // 次态计算函数（重点！）
}

// 状态机容器
type paymentFsm struct {
    models []*paymentFsmModel  // 所有转移规则的切片
}
```

### 关键设计洞察

> **为什么 `calNextStatusFun` 的签名这么复杂？**
>
> ```go
> func(ctx context.Context, ps *entity.PaymentSlip, scene commonv2pb.Scene, paymentCore PaymentCore) (types.PaymentSlipStatus, error)
> ```
>
> 因为次态不是固定的，需要：
> 1. **查数据库/缓存**（需要 `context`）
> 2. **看支付单业务属性**（`ps.IsSourceQuick()`、`ps.OnlyAutoPay()`...）
> 3. **调用外部服务**（`paymentCore.GetPaymentMethods()` 查可用支付方式）
> 4. **看业务场景**（`scene`：食堂/堂食/外卖...）
>
> 这就是为什么简单的 `map[现态][事件]次态` 满足不了业务需求。

---

## 三、流式 API 设计：链式调用的优雅

### 定义转移规则的 DSL

```go
// 这三个方法是精髓！每次返回自身，形成链式调用
func (m *paymentFsmModel) event(payEvent types.PaymentSlipEvent) *paymentFsmModel {
    m.payEvent = payEvent
    return m
}

func (m *paymentFsmModel) from(currentStatus types.PaymentSlipStatus) *paymentFsmModel {
    m.currentStatus = currentStatus
    return m
}

func (m *paymentFsmModel) cal(calNextStatusFun func(...)) *paymentFsmModel {
    m.calNextStatusFun = calNextStatusFun
    return m
}
```

### 实际使用效果

```go
// 像在读业务文档一样清晰
fsm.Register(
    (&paymentFsmModel{}).
        event(types.AutoPayInsufficient).
        from(types.PaymentSlipStatusFullPay).
        cal(func(...) {
            // 业务逻辑...
        })
)
```

### Go 语言技巧应用

| 技巧 | 价值 |
|------|------|
| **方法值接收者返回自身** | 实现流式 API，代码可读性提升 10 倍 |
| **函数作为参数** | 业务逻辑与状态机框架解耦 |
| **闭包捕获外部变量** | `cal` 函数内可以访问 `ps`、`paymentCore` 等 |

---

## 四、状态转移注册：30+ 规则的完整图景

### 举一个最复杂的业务规则

看 **自动支付余额不足** 这个场景：

```go
fsm.Register((&paymentFsmModel{}).
    event(types.AutoPayInsufficient).    // 事件：自动支付钱不够
    from(types.PaymentSlipStatusFullPay). // 现态：正在全额自动支付
    cal(func(ctx context.Context, ps *entity.PaymentSlip, scene commonv2pb.Scene, paymentCore PaymentCore) (types.PaymentSlipStatus, error) {

        // 分支 1：仅自动支付 或 Quick 场景 → 直接失败
        if ps.OnlyAutoPay() || ps.IsSourceQuick() {
            return types.PaymentSlipStatusFailed, nil
        }

        // 分支 2：后付场景 → 全额支付失败（可以手动补）
        if ps.IsPayAfter() {
            return types.PaymentSlipStatusFullPayFailed, nil
        }

        // 分支 3：纯餐点支付 → 直接失败
        if ps.IsMealPointPay() {
            return types.PaymentSlipStatusFailed, nil
        }

        // 分支 4：其他场景 → 查有没有手动支付方式
        payMethodSet, err := paymentCore.GetPaymentMethods(ctx, ps, scene, false, pscommonpb.PayType_MANUAL_PAY)
        if err != nil {
            return 0, err
        }

        // 有手动支付方式 → 全额支付失败（用户可以手动付）
        if payMethodSet.HasMealplanManualPayMethods() {
            return types.PaymentSlipStatusFullPayFailed, nil
        }

        // 啥都没有 → 直接失败
        return types.PaymentSlipStatusFailed, nil
    }))
```

### 这个闭包里的业务深度

这一个 `cal` 函数里包含了：
- **4 种业务场景判断**（Quick/后付/餐点/普通）
- **1 次 RPC 调用**（查可用支付方式）
- **5 种可能的次态结果**

> **思考**：如果不用状态机，这堆逻辑会散落在 `if/else` 的海洋里...

---

## 五、核心执行逻辑：`Call` 方法

```go
func (f *paymentFsm) Call(
    ctx context.Context,
    event types.PaymentSlipEvent,
    ps *entity.PaymentSlip,
    scene commonv2pb.Scene,
    paymentCore PaymentCore
) (types.PaymentSlipStatus, error) {

    // 线性遍历所有规则，O(n) 查找
    for _, m := range f.models {
        // 匹配条件：事件 + 现态
        if m.payEvent == event && m.currentStatus == ps.Status {
            // 执行次态计算函数
            status, err := m.calNextStatusFun(ctx, ps, scene, paymentCore)
            if err != nil {
                log.For(ctx).Error("paymentFsm Call guardFun err", log.ErrField(err))
                return 0, err
            }

            log.For(ctx).Info("paymentFsm cal status",
                log.Any("event", event),
                log.Any("currentStatus", ps.Status),
                log.Any("nextStatus", status))

            return status, nil
        }
    }

    // 没有匹配的规则 → 报错（防御式编程）
    log.For(ctx).Error("paymentFsm can not find fsmModel err",
        log.Any("event", event),
        log.Any("status", ps.Status))

    return 0, fmt.Errorf("paymentFsm can not find fsmModel err, event: %d, status: %d", event, ps.Status)
}
```

### 执行流程可视化

```
触发事件 (AutoPayInsufficient)
        ↓
遍历 30+ 规则
        ↓
找到匹配的（现态 FullPay + 事件 AutoPayInsufficient）
        ↓
执行 calNextStatusFun 闭包
    ├─ 检查业务属性（IsSourceQuick? OnlyAutoPay?）
    ├─ 可能调用外部服务（GetPaymentMethods）
    └─ 返回计算出的次态
        ↓
返回次态给调用方
        ↓
调用方自己更新 ps.Status = status  ← 重点：状态机不做副作用！
```

---

## 六、实际业务调用示例

### 场景：手动支付成功后的状态变更

```go
// manual_pay.go:181
func (m *manualPayService) HandlePaySuccess(ctx context.Context, ps *entity.PaymentSlip, ss *entity.PaymentSubSlip) error {
    // 1. 先更新子单状态
    ss.Status = types.SubSlipStatusSuccess
    ps.PaidAmount += ss.EffectiveAmount
    ps.SubSlips = append(ps.SubSlips, ss)

    // 2. 调用状态机计算主单新状态
    status, err := m.PaymentFsm.Call(
        ctx,
        types.ManualPaySuccess,  // 事件：手动支付成功
        ps,                       // 支付单（包含现态）
        ss.TpwPayScene.ToScene(), // 业务场景
        m.PaymentCore             // 依赖服务
    )
    if err != nil {
        return err
    }

    // 3. 调用方自己更新状态（状态机不做副作用）
    ps.Status = status

    // 4. 后续业务逻辑...
    if ps.Status == types.PaymentSlipStatusSuccess {
        // 触发支付完成事件、发 MQ、更新账单...
    }
}
```

### 调用模式的统一

所有业务代码调用状态机的模式完全一致：
```go
// 统一模式
status, err := fsm.Call(ctx, 事件, 支付单, 场景, 核心服务)
if err != nil { return err }
ps.Status = status  // 调用方负责副作用
```

---

## 七、设计优缺点深度评估

### ✅ 优点

| 优点 | 具体表现 |
|------|---------|
| **业务逻辑集中** | 30+ 状态转移都在 `InitPaymentFsm()` 一个函数里，改规则不用到处找 |
| **可测试性强** | `calNextStatusFun` 是纯函数（依赖通过参数传入），单元测试方便 |
| **扩展性好** | 新增状态/事件只需要新增 `Register`，不影响现有代码 |
| **开闭原则** | 新增规则不需要改 `Call` 方法，对扩展开放，对修改关闭 |
| **调试友好** | 每次状态变更都打日志，排查问题时可以看到完整的状态流转链 |

### ⚠️ 潜在问题

| 问题 | 风险 |
|------|------|
| **线性查找性能** | 30+ 规则还好，但如果到 1000+ 就需要优化成 `map[现态][事件]规则` |
| **无重复注册校验** | 两个人加了相同的 `(现态, 事件)` 组合，后注册的永远走不到，前注册的被覆盖了还不知道 |
| **类型安全依赖** | `PaymentCore` 是个胖接口，传给 `cal` 函数权限太大了 |
| **无 guard 条件** | 只有次态计算，没有单独的 guard（前置校验）层，逻辑混杂 |

---

## 八、Go 语言特性的巧妙应用

这个状态机几乎用到了所有核心特性：

| Go 特性 | 应用位置 | 价值 |
|---------|---------|------|
| **函数作为一等公民** | `calNextStatusFun` 字段 | 业务逻辑注入 |
| **闭包** | 每个 `Register` 里的匿名函数 | 捕获上下文变量 |
| **方法链式调用** | `.event().from().cal()` | DSL 可读性 |
| **接口** | `PaymentCore` 参数 | 依赖倒置，方便 mock |
| **错误处理** | `Call` 方法的多返回值 | 明确的错误传播 |
| **`context` 传递** | 第一个参数永远是 `ctx` | 链路追踪、超时取消 |
| **结构化日志** | `log.For(ctx)` | 可观测性 |

---

## 九、完整流程分析：自动支付 + 余额不足场景

### 流程图总览

```
用户下单
    ↓
[应用层] transaction_meican.go: CreatePaymentSlip()
    ↓  调用领域服务
[领域层] meican_payment.go: OnlyAutoPay()
    ↓  调用支付核心
[领域层] payment_core.go: AutoPay()
    ├─ 🔴 【状态机第1次调用】 StartAutoPay
    │      现态：New → 次态：FullPay
    │
    ↓  调用自动支付服务
[领域层] auto_pay.go: SyncAutoPay()
    ├─ 按优先级扣减：补贴 → 个人余额 → 微信/支付宝
    │
    ├─ 发现补贴账户余额不足 ← 余额不足触发点！
    │
    ├─ 🔴 【状态机第2次调用】 AutoPayInsufficient
    │      现态：FullPay → 次态：FullPayFailed 或 Failed
    │
    └─ 执行后续业务：回滚已支付子单、发 MQ 通知...
```

---

### 详细代码逐行拆解

#### 第 1 步：应用层入口 - 创建支付单

**文件**：`internal/application/svc/internal/transaction_meican.go`

```go
func (t *TransactionMeican) CreatePaymentSlip(ctx context.Context, param *appdto.CreatePaymentSlipMeican) (int64, bizerr.Error) {
    // 1. 风控检查（重复支付）
    if riskErr := t.RiskControl.RepeatPayCheck(ctx, ...); riskErr != nil {
        return 0, bizerr.NewWithErr(...)
    }

    // 2. 餐点库存校验（带分布式锁！）
    unlock, bizErr := t.MealPoint.CreatePaymentVerifyMealPoint(ctx, param)
    defer unlock()  // defer 保证锁一定会释放
    if bizErr != nil {
        return 0, bizErr
    }

    // 3. 生成雪花 ID（分布式唯一）
    id, err := t.IDGenerator.Generate(ctx)
    if err != nil {
        return 0, bizerr.NewWithErr(...)
    }

    // DTO → Entity 转换（ assembler 模式）
    ps := assembler.MeicanToPaymentSlipEntity(param, int64(id))

    // 4. 【零元支付快速路径】直接走特殊流程
    if param.Amount == 0 {
        return t.PaymentSlip.ZeroPay(ctx, ps)
    }

    // 5. 【仅自动支付路径】重点看这个！
    if param.WithoutSDK {  // 美餐后台发起的支付，不需要用户点 SDK
        return t.PaymentSlip.OnlyAutoPay(ctx, ps)
    }

    return t.PaymentSlip.CreateAndPay(ctx, ps)
}
```

> **注意**：应用层只做**编排**，不做核心业务逻辑。所有真正的支付逻辑都在领域层。

---

#### 第 2 步：领域层 - 发起自动支付

**文件**：`internal/domain/transaction/svc/internal/meican_payment.go`

```go
func (p *PaymentSlipMeican) OnlyAutoPay(ctx context.Context, ps *entity.PaymentSlip) (int64, bizerr.Error) {
    // 1. 先检查账户余额（前置校验，减少失败概率）
    accounts, totalBalance, bizErr := p.PaymentCore.CheckBalance(ctx, ps, scene, pscommonpb.PayType_AUTO_PAY)
    if bizErr != nil {
        return 0, bizErr
    }

    var payFinishTransactionFunc PayFinishTransactionFunc

    // 2. 🔴 【状态机第1次调用】开始自动支付
    payFinishTransactionFunc, bizErr = p.PaymentCore.AutoPay(ctx, ps, accounts, scene)
    if bizErr != nil {
        return 0, bizErr.Wrap("PaymentCore AutoPay err")
    }

    // 3. 数据库事务：落库 + 发消息
    err := xormv2.Transaction(ctx, func(ctx context.Context) error {
        // 创建支付单
        if err := p.PaymentSlipRepo.CreatePaymentSlip(ctx, ps); err != nil {
            return err
        }

        // 自动支付完成后的事务回调
        if err := payFinishTransactionFunc(ctx); err != nil {
            return err
        }
        return nil
    })

    return ps.Id, nil
}
```

---

#### 第 3 步：支付核心 - 状态机第一次调用

**文件**：`internal/domain/transaction/svc/internal/payment_core.go:840`

```go
func (p *paymentService) AutoPay(ctx context.Context, ps *entity.PaymentSlip, accounts []*msg.PayableAccount, scene commonv2pb.Scene) (PayFinishTransactionFunc, bizerr.Error) {

    // =================================================================
    // 🔴 状态机第 1 次调用：开始自动支付
    // =================================================================
    // 事件：types.StartAutoPay
    // 现态：ps.Status = PaymentSlipStatusNew
    // 输入：ps（支付单实体）、scene（业务场景）、p（核心服务）
    // 输出：次态 = PaymentSlipStatusFullPay
    // =================================================================
    status, err := p.PaymentFsm.Call(
        ctx,
        types.StartAutoPay,  // 触发事件
        ps,                   // 支付单（包含现态 ps.Status）
        scene,                // 场景：食堂/堂食/外卖
        p                     // 支付核心服务（供闭包调用）
    )
    if err != nil {
        return nil, bizerr.NewWithErr(paymentv2pb.ResultCode_RESULT_CODE_INTERNAL_ERROR, err)
    }

    // 状态机只负责计算次态，不负责更新实体！
    ps.Status = status  // 现在 ps.Status = FullPay 了

    // =================================================================
    // 返回事务回调函数（闭包捕获了上下文）
    // 这个函数会在外部事务提交时执行
    // =================================================================
    return func(ctx context.Context) error {
        // 真正执行扣款逻辑（注意：这是在支付单落库之后！）
        autoPayResult, err := p.AutoPayService.SyncAutoPay(ctx, ps, accounts)
        if err != nil {
            return err
        }

        // 根据自动支付结果处理...
        if autoPayResult.InsufficientBalance {
            // 余额不足处理 ← 进入下一个关键节点
            return autoPayResult.InsufficientBalanceFunc(ctx, ps, scene, p.AutoPayService)
        }

        return nil
    }, nil
}
```

> **关键设计洞察**：注意这里的 **事务回调模式**
> - `AutoPay` 函数先执行状态机，**然后返回一个函数**
> - 这个返回的函数会在 `CreatePaymentSlip` 事务提交时才执行
> - 这保证了：支付单一定先落库，再执行真正的扣款 RPC

---

#### 第 4 步：自动支付执行 - 发现余额不足

**文件**：`internal/domain/transaction/svc/internal/auto_pay.go`

```go
func (p *autoPayService) SyncAutoPay(ctx context.Context, ps *entity.PaymentSlip, accounts []*msg.PayableAccount) (*AutoPayResult, error) {
    result := &AutoPayResult{}

    // 按支付渠道优先级循环扣款：补贴 → 个人余额 → 微信/支付宝
    for _, account := range accounts {
        payResult, err := p.PayChannel.Pay(ctx, ps, account)
        if err != nil {
            return nil, err
        }

        // =================================================================
        // 💥 余额不足！触发点在这里
        // =================================================================
        if payResult.InsufficientBalance {
            result.InsufficientBalance = true

            // 注意这里！返回两个不同的处理函数
            // 策略模式：不同业务类型用不同的余额不足处理函数
            if ps.IsPayAfter() {
                result.InsufficientBalanceFunc = insufficientBalanceOfPayAfter
            } else {
                result.InsufficientBalanceFunc = insufficientBalanceOfNormal
            }
            return result, nil
        }

        result.PaidAmount += payResult.PaidAmount
        result.SubSlips = append(result.SubSlips, payResult.SubSlip)
    }

    // 如果走到这里，说明所有账户都扣款成功了
    return result, nil
}
```

> **Go 语言技巧**：`InsufficientBalanceFunc` 是一个函数类型变量
> ```go
> type InsufficientBalanceFunc func(ctx context.Context, ps *entity.PaymentSlip, scene commonv2pb.Scene, p *autoPayService) error
> ```
> 用函数作为返回值，实现了**策略模式**的轻量化版本。

---

#### 第 5 步：余额不足处理 - 状态机第二次调用

**文件**：`internal/domain/transaction/svc/internal/auto_pay.go:410`

```go
// 普通支付单的余额不足处理
func insufficientBalanceOfNormal(ctx context.Context, ps *entity.PaymentSlip, scene commonv2pb.Scene, p *autoPayService) error {

    // =================================================================
    // 🔴 状态机第 2 次调用：自动支付余额不足
    // =================================================================
    // 事件：types.AutoPayInsufficient
    // 现态：ps.Status = PaymentSlipStatusFullPay (正在全额支付)
    // 输出：次态 = FullPayFailed 或 Failed（由闭包动态计算）
    // =================================================================
    status, err := p.PaymentFsm.Call(
        ctx,
        types.AutoPayInsufficient,  // 事件变了！
        ps,
        scene,
        p.PaymentCore
    )
    if err != nil {
        return err
    }

    // 更新状态
    ps.Status = status

    // =================================================================
    // 状态机只告诉我们"变成什么状态"，但真正的业务副作用还得自己做
    // =================================================================

    // 如果是终态（Failed），设置完成时间
    if ps.Status.IsCompleted() {
        completedAt := xtime.Now()
        ps.CompletedAt = &completedAt
    }

    // 1. 查询所有已支付的子单
    subSlipsAll, err := p.PaymentSlipRepo.FindPaymentSubSlipsBySlipID(ctx, ps.Id)
    if err != nil {
        return err
    }

    // 2. 筛选需要回滚的子单
    var subSlips entity.PaymentSubSlips
    for _, slip := range subSlipsAll {
        if !slip.TpwPayScene.IsManualPay() || (status.IsFailed() && slip.Status.IsSucceeded()) {
            subSlips = append(subSlips, slip)
        }
    }

    // 3. 数据库事务：更新状态 + 回转子单
    var afterTransactionFunc PayFinishAfterTransactionFunc
    if err := xormv2.Transaction(ctx, func(ctx context.Context) error {

        // 删除手动支付上下文
        if err := p.PaymentSlipRepo.DeleteManualPayContext(ctx, ps.Id); err != nil {
            return err
        }

        // 更新支付单主状态（状态机算出的次态落库）
        if err := p.PaymentSlipRepo.UpdatePaymentSlipStatus(ctx, ps.Id, ps.Status, ps.CompletedAt); err != nil {
            return err
        }

        // 回滚所有已支付子单（分布式一致性！）
        if err := p.RollbackPayService.RollbackPaymentSubSlip(ctx, subSlips...); err != nil {
            return err
        }

        // 如果终态是失败，触发支付失败回调（发 MQ 通知下游）
        if ps.Status.IsFailed() {
            afterTransactionFunc, err = p.HandlePaymentCompletedMap[ps.GetPaymentProductType()].FinishFailedPaymentSlip(ctx, ps)
            if err != nil {
                return err
            }
        }

        return nil
    }); err != nil {
        return err
    }

    // 事务提交后执行异步回调（发通知、清缓存...）
    if afterTransactionFunc != nil {
        afterTransactionFunc(ctx, ps)
    }

    return nil
}
```

---

### 完整时间线总结

| 时间点 | 事件 | ps.Status 变化 | 关键操作 |
|--------|------|----------------|---------|
| T0 | 用户下单创建支付单 | `New` | 生成 ID、风控、库存校验 |
| T1 | 调用 `PaymentCore.AutoPay()` | `New` → `FullPay` | 状态机第1次调用：`StartAutoPay` |
| T2 | 执行自动支付扣款 | `FullPay` | 按优先级扣减：补贴 → 余额 → 三方 |
| T3 | 发现补贴余额不足 | `FullPay` | 中断扣款流程，设置 `InsufficientBalance` 标记 |
| T4 | 调用余额不足处理函数 | `FullPay` | 准备回滚逻辑 |
| T5 | 状态机第2次调用 | `FullPay` → `FullPayFailed` | `AutoPayInsufficient` 事件，动态计算次态 |
| T6 | 数据库事务 | `FullPayFailed` 落库 | 回转子单、删除支付上下文、更新主状态 |
| T7 | 事务提交后回调 | - | 发 MQ 通知下游、触发支付失败事件 |

---

### 这个流程里的设计智慧

#### 1. **状态机的单一职责**
> 状态机**只负责计算次态**，不做任何副作用！
> - ✅ 不更新数据库
> - ✅ 不发 MQ
> - ✅ 不调用 RPC
> - ✅ 只做纯计算（虽然依赖外部输入，但输出只由输入决定）

这样状态机的可测试性极强，输入什么就输出什么。

#### 2. **副作用与计算分离**

| 角色 | 职责 |
|------|------|
| 状态机 FSM | 次态计算（What） |
| 业务代码 | 副作用执行（How） |

#### 3. **事务边界的优雅设计**
- 状态机调用 **在事务外** 先算好次态
- 真正的落库 **在事务内** 一次性提交
- 事务后回调处理异步操作

避免了长事务，也避免了状态机执行过程中持有数据库连接。

---

## 十、优化思路（思考题）

基于学的 Go 知识，可以考虑的优化方向：

### 1. 线性查找 → Map 查找优化

```go
type paymentFsm struct {
    // 旧方案：models []*paymentFsmModel
    // 新方案：两级 map，O(1) 查找
    transitionMap map[types.PaymentSlipStatus]map[types.PaymentSlipEvent]*paymentFsmModel
}
```

### 2. 重复注册校验

```go
func (f *paymentFsm) Register(model *paymentFsmModel) {
    key := fmt.Sprintf("%d-%d", model.currentStatus, model.payEvent)
    if _, exists := f.transitionMap[key]; exists {
        panic(fmt.Sprintf("duplicate transition registered: %s", key))
    }
    // ...
}
```

### 3. 你的想法？

- Guard 条件与次态计算分离？
- 支持中间状态？
- 状态流转可视化？

---

> 文档生成时间：2025-05-16
> 分析对象：美餐 payment 服务内部状态机实现
