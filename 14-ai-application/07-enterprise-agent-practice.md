# 企业级 Agent 开发实践 — 权限/审核/案例/架构

> 面向后端岗位（含 AI 方向）的企业级 Agent 设计与落地指南

---

## 一、企业 Agent 与玩具 Agent 的核心差异

| 维度 | Demo/玩具 | 企业生产 |
|------|-----------|----------|
| 权限控制 | 无 | RBAC + 工具白名单 + 操作范围限制 |
| 审核机制 | 无 | 高风险操作必须人工审批 |
| 审计日志 | 无 | 所有操作留痕，满足合规 |
| 多租户隔离 | 无 | 数据和工具级隔离 |
| 成本治理 | 无 | Token 预算 + 限流 + 成本分摊 |
| 可观测性 | print | Trace + Metrics + Alert |
| 测试 | 手动 | Golden Set + 自动化评估 |
| 幂等性 | 不考虑 | 关键操作防重放 |
| 灾备 | 无 | 多模型 fallback + 降级策略 |

> **面试要点**：企业面试中，不只考察你会不会调 LLM API，更关注你是否理解权限、审计、稳定性这些工程问题。

---

## 二、企业级权限控制架构

### 2.1 三层权限模型

```
用户/角色层（Who）
    ↓
    RBAC 权限映射
    ↓
Agent 能力层（What）
    ↓
    工具白名单 + Scope 限制
    ↓
工具执行层（How）
    ↓
    操作范围 + 参数约束 + 资源配额
```

### 2.2 RBAC for Agent Tools

```go
// 工具权限定义
type ToolPermission struct {
    ToolName   string   // 工具名
    Scopes     []string // 允许的操作范围
    MaxAmount  *float64 // 金额上限（支付类工具）
    Allowed    []string // 允许调用的角色
}

// 角色-工具权限表
var roleToolPermissions = map[string][]ToolPermission{
    "customer_service": {
        {ToolName: "query_order",   Scopes: []string{"read"},        Allowed: []string{"customer_service"}},
        {ToolName: "query_payment", Scopes: []string{"read"},        Allowed: []string{"customer_service"}},
        {ToolName: "create_refund", Scopes: []string{"write"},
            MaxAmount: pointer(500.0),                               // 客服最多退 500 元，超过需主管审批
            Allowed: []string{"customer_service"}},
    },
    "supervisor": {
        {ToolName: "create_refund", Scopes: []string{"write"},
            MaxAmount: pointer(10000.0),
            Allowed: []string{"supervisor"}},
        {ToolName: "manual_reconcile", Scopes: []string{"write"},    Allowed: []string{"supervisor"}},
    },
    "risk_analyst": {
        {ToolName: "query_transaction", Scopes: []string{"read", "export"}},
        {ToolName: "freeze_account",    Scopes: []string{"write"},   Allowed: []string{"risk_analyst"}},
        {ToolName: "unfreeze_account",  Scopes: []string{"write"},   Allowed: []string{"supervisor", "compliance"}},
    },
}
```

### 2.3 工具调用前的权限检查

```go
func (s *AgentService) ExecuteTool(ctx context.Context, req ToolCallRequest) (*ToolResult, error) {
    // 1. 提取调用者身份
    caller := auth.FromContext(ctx)

    // 2. 检查工具是否在白名单
    perms, ok := s.getPermissions(caller.Role, req.ToolName)
    if !ok {
        return nil, ErrToolNotAllowed{Tool: req.ToolName, Role: caller.Role}
    }

    // 3. 检查 Scope
    if !containsScope(perms.Scopes, req.RequiredScope) {
        return nil, ErrScopeNotAllowed{...}
    }

    // 4. 检查金额上限（支付类工具）
    if perms.MaxAmount != nil {
        amount := req.Params.GetFloat("amount")
        if amount > *perms.MaxAmount {
            // 超额 → 转人工审批流程
            return s.createApprovalRequest(ctx, req, caller, amount)
        }
    }

    // 5. 记录审计日志（执行前）
    s.auditLog.Record(ctx, AuditEvent{
        Action:    "tool_call",
        Tool:      req.ToolName,
        Params:    req.Params,
        Caller:    caller,
        Timestamp: time.Now(),
    })

    // 6. 执行工具
    result, err := s.toolRegistry.Execute(ctx, req)

    // 7. 记录审计日志（执行后）
    s.auditLog.Record(ctx, AuditEvent{
        Action: "tool_result", Result: result, Err: err,
    })

    return result, err
}
```

### 2.4 数据隔离（多租户）

```go
// 工具调用时注入租户 Scope，防止跨租户数据泄露
type TenantScopedTool struct {
    inner  Tool
    tenant string
}

func (t *TenantScopedTool) Call(ctx context.Context, params map[string]any) (any, error) {
    // 强制注入租户过滤条件
    params["_tenant_id"] = t.tenant   // 所有查询自动带租户过滤
    params["_data_scope"] = t.tenant  // 禁止跨租户访问

    return t.inner.Call(ctx, params)
}

// Agent 初始化时绑定租户
func NewAgentForTenant(tenantID string) *Agent {
    tools := []Tool{
        &TenantScopedTool{inner: NewOrderQueryTool(), tenant: tenantID},
        &TenantScopedTool{inner: NewPaymentTool(), tenant: tenantID},
    }
    return &Agent{tools: tools, tenantID: tenantID}
}
```

---

## 三、人工审批（Human-in-the-Loop）工作流

### 3.1 哪些操作必须走人工审批

| 操作类型 | 阈值 | 审批人 |
|---------|------|--------|
| 退款 | > $500 | 主管 |
| 账号冻结/解冻 | 任何时候 | 合规 + 风控 |
| 批量操作 | 影响 > 100 个用户 | 技术负责人 |
| 数据导出 | 包含 PII 数据 | 数据合规官 |
| 配置变更 | 影响生产环境 | 技术负责人 |
| 大额转账 | > $10000 | 财务 |

### 3.2 审批流实现（LangGraph 风格）

```python
from langgraph.graph import StateGraph, END
from langgraph.checkpoint.sqlite import SqliteSaver

class ApprovalState(TypedDict):
    task_id: str
    action: str
    params: dict
    requester: str
    approver: Optional[str]
    approval_status: Optional[str]  # pending/approved/rejected
    reason: Optional[str]

def need_approval(state: ApprovalState) -> str:
    """判断是否需要审批"""
    if state["action"] == "create_refund" and state["params"]["amount"] > 500:
        return "require_approval"
    if state["action"] in ["freeze_account", "unfreeze_account"]:
        return "require_approval"
    return "execute_directly"

def request_approval(state: ApprovalState):
    """创建审批请求，发送通知"""
    approval_id = create_approval_ticket(
        task_id=state["task_id"],
        action=state["action"],
        params=state["params"],
        requester=state["requester"],
    )
    notify_approver(approval_id)  # Slack/Email/飞书通知审批人
    return {"approval_status": "pending", "approval_id": approval_id}

def wait_for_approval(state: ApprovalState):
    """等待审批结果（LangGraph interrupt_before 实现）"""
    # 这里图会暂停，等待外部 resume
    pass

def execute_action(state: ApprovalState):
    """获得批准后执行"""
    if state["approval_status"] == "rejected":
        return {"result": "Action rejected by approver", "done": True}
    result = execute_tool(state["action"], state["params"])
    return {"result": result, "done": True}

# 构建审批图
graph = StateGraph(ApprovalState)
graph.add_node("check_approval", lambda s: s)
graph.add_node("request_approval", request_approval)
graph.add_node("wait_for_approval", wait_for_approval)
graph.add_node("execute", execute_action)

graph.add_conditional_edges("check_approval", need_approval, {
    "require_approval": "request_approval",
    "execute_directly": "execute",
})
graph.add_edge("request_approval", "wait_for_approval")
graph.add_edge("wait_for_approval", "execute")

# interrupt_before=["wait_for_approval"] 让图在此暂停
app = graph.compile(
    checkpointer=SqliteSaver(conn),
    interrupt_before=["wait_for_approval"]
)

# 审批人批准后 resume
def approve_action(task_id: str, approver: str, decision: str):
    app.update_state(
        {"configurable": {"thread_id": task_id}},
        {"approval_status": decision, "approver": approver}
    )
    app.invoke(None, {"configurable": {"thread_id": task_id}})
```

### 3.3 审批流程图

```
用户发起请求
    ↓
Agent 分析意图 → 选择工具
    ↓
权限检查 → 是否超阈值？
    ↓ 是                    ↓ 否
创建审批工单           直接执行工具
    ↓                       ↓
通知审批人              返回结果
    ↓
等待审批（可暂停数小时/天）
    ↓ 批准                  ↓ 拒绝
执行工具              返回"已拒绝"
    ↓
返回结果 + 审计日志
```

---

## 四、审计日志与合规

### 4.1 审计日志要求

支付/金融场景下，Agent 的每个操作必须留痕，满足：
- **PCI-DSS**：支付卡行业数据安全标准
- **SOX**：萨班斯-奥克斯利法案（财务审计）
- **GDPR**：数据处理留痕要求

### 4.2 审计日志结构

```go
type AgentAuditLog struct {
    // 基础信息
    TraceID    string    `json:"trace_id"`    // 全链路追踪 ID
    SpanID     string    `json:"span_id"`
    Timestamp  time.Time `json:"timestamp"`

    // 身份信息
    AgentID    string `json:"agent_id"`   // Agent 实例 ID
    UserID     string `json:"user_id"`    // 发起用户
    TenantID   string `json:"tenant_id"`  // 所属租户
    Role       string `json:"role"`       // 用户角色

    // 操作信息
    Action     string         `json:"action"`     // 操作类型
    Tool       string         `json:"tool"`       // 调用的工具
    Params     map[string]any `json:"params"`     // 参数（脱敏后）
    Result     *ToolResult    `json:"result"`     // 执行结果
    Error      *string        `json:"error"`      // 错误信息

    // 审批信息
    RequiredApproval bool    `json:"required_approval"`
    ApprovalID       string  `json:"approval_id,omitempty"`
    ApproverID       string  `json:"approver_id,omitempty"`
    ApprovalDecision string  `json:"approval_decision,omitempty"`

    // LLM 调用信息
    ModelID    string `json:"model_id"`    // 使用的模型
    InputTokens  int  `json:"input_tokens"`
    OutputTokens int  `json:"output_tokens"`
    LatencyMs  int64  `json:"latency_ms"`

    // 数据访问信息（用于 GDPR）
    DataAccessed []string `json:"data_accessed"` // 访问了哪些数据类型
    PIIAccessed  bool     `json:"pii_accessed"`   // 是否访问了 PII
}
```

### 4.3 敏感信息脱敏

```go
// 在写审计日志前脱敏
func sanitizeParams(params map[string]any) map[string]any {
    sanitized := make(map[string]any)
    for k, v := range params {
        switch k {
        case "card_number", "cvv", "bank_account":
            sanitized[k] = maskString(v.(string)) // 只显示后4位
        case "id_card", "passport":
            sanitized[k] = "[REDACTED]"
        case "amount", "order_id", "user_id":
            sanitized[k] = v // 保留业务关键字段
        default:
            sanitized[k] = v
        }
    }
    return sanitized
}
```

---

## 五、企业级案例：支付客服 Agent

### 5.1 系统架构

```
客服工作台 (Web/APP)
         ↓ WebSocket/SSE
    Agent Gateway (Go)
    ├── 身份认证 (JWT)
    ├── 权限检查 (RBAC)
    ├── 限流 (100 req/min/user)
    └── 路由 (会话管理)
         ↓
    Agent Runtime (Python/LangGraph)
    ├── LLM: Claude 3.5 Sonnet (主) / GPT-4o-mini (分类)
    ├── 工具集 (10个核心工具)
    ├── 记忆: Redis (短期) + PostgreSQL (长期)
    └── 护栏: 输入过滤 + 输出审查
         ↓
    Tool Services (Go 微服务)
    ├── order-service    (查单、改单)
    ├── payment-service  (查支付、退款)
    ├── account-service  (查账户、余额)
    └── notification-svc (发通知)
         ↓
    Audit & Observability
    ├── 审计日志 → Elasticsearch
    ├── Trace → Jaeger / OpenTelemetry
    ├── Metrics → Prometheus
    └── Alert → PagerDuty
```

### 5.2 工具集设计（10个）

```python
# 查询类（只读，无需审批）
tools_readonly = [
    "query_order",          # 查询订单状态
    "query_payment",        # 查询支付流水
    "query_refund",         # 查询退款进度
    "query_account",        # 查询账户信息
    "query_faq",            # 查询知识库 FAQ
]

# 写入类（低风险，可自动执行）
tools_low_risk = [
    "create_refund_small",  # 小额退款 (≤$50，自动审批)
    "send_notification",    # 发送通知/短信
    "add_note",             # 添加客服备注
]

# 写入类（高风险，需审批）
tools_high_risk = [
    "create_refund_large",  # 大额退款 (>$50，需主管审批)
    "escalate_to_human",    # 转人工客服
]
```

### 5.3 完整对话流程

```
用户: "我的订单 ORD123 退款怎么还没到？"

[Agent 内部]
Step 1: 意图识别
  - 分类器(小模型): "退款查询"
  - 提取实体: order_id=ORD123

Step 2: 权限检查
  - 用户角色: customer (普通用户)
  - 允许工具: query_refund ✓

Step 3: 工具调用
  - call: query_refund(order_id="ORD123")
  - 审计: 记录 → trace_id=xxx, action=query_refund

Step 4: 结果处理
  - 返回: {status: "processing", bank: "招商银行", eta: "1-3工作日"}

Step 5: 生成回复
  - 引用工具结果，不靠 LLM 记忆
  - 护栏检查输出无敏感信息

Agent 回复: "您好！ORD123 的退款已于 2024-01-15 提交到招商银行，
            退款正在处理中，预计 1-3 个工作日到账。
            如超过 3 个工作日未到，建议联系银行查询。"
```

### 5.4 大额退款审批流程

```
用户: "我要退款1500元，订单 ORD456"

[Agent 内部]
Step 1: 意图 → 大额退款
Step 2: 检查金额 1500 > 500 (客服权限上限)
Step 3: 创建审批工单 → 发飞书/Slack 通知主管
Step 4: 返回用户等待消息

Agent: "您好，1500元退款申请已提交，需要主管审批（通常30分钟内处理）。
       审批单号: APV-20240115-001，请关注短信通知。"

[30分钟后，主管在审批系统点击批准]

[系统通知 Agent 恢复执行]
Step 5: 调用 create_refund(order_id="ORD456", amount=1500, approver="supervisor_id")
Step 6: 记录完整审计日志（含审批链）

Agent 主动通知用户: "您的1500元退款已批准并提交到银行，
                   预计1-3工作日到账。"
```

---

## 六、企业级案例：对账 Agent

### 6.1 对账 Agent 架构

```
定时触发 (每日 T+1 02:00)
         ↓
    Reconcile Agent (自主模式)
    ├── 阶段1: 数据收集
    │   ├── tool: fetch_internal_txn(date)    获取内部流水
    │   ├── tool: fetch_channel_statement(date) 获取渠道对账单
    │   └── tool: fetch_bank_statement(date)   获取银行流水
    │
    ├── 阶段2: 数据比对
    │   ├── tool: compare_transactions()       逐条比对
    │   └── tool: identify_differences()       识别差异
    │
    ├── 阶段3: 差异分类（LLM 推理）
    │   ├── "时间差异" → 标记为待确认，次日再查
    │   ├── "金额不符" → 创建人工处理工单
    │   ├── "单边流水" → 识别是否为撤销/退款
    │   └── "疑似欺诈" → 立即告警，冻结相关账户
    │
    └── 阶段4: 结果处理
        ├── 自动平账（差异 < $0.01 的精度误差）
        ├── 创建工单（需人工处理的差异）
        └── 生成对账报告（发送给财务团队）
```

### 6.2 差异处理的风险分级

```go
type DiffRiskLevel string
const (
    RiskAuto   DiffRiskLevel = "auto"   // Agent 自动处理
    RiskReview DiffRiskLevel = "review" // 人工复核
    RiskAlert  DiffRiskLevel = "alert"  // 立即告警
)

func classifyDiff(diff ReconcileDiff) DiffRiskLevel {
    // 精度误差，自动平账
    if diff.Type == "precision" && diff.Amount < 0.01 {
        return RiskAuto
    }
    // 时间差异（渠道延迟），等待 T+1 再处理
    if diff.Type == "timing" && diff.AgeDays <= 3 {
        return RiskReview
    }
    // 金额不符，必须人工
    if diff.Type == "amount_mismatch" {
        return RiskAlert
    }
    // 超大差异，立即告警
    if diff.Amount > 10000 {
        return RiskAlert
    }
    return RiskReview
}
```

---

## 七、企业级案例：风控 Agent

### 7.1 实时风控场景

```
交易请求到来 (P99 < 100ms)
         ↓
    风控 Agent (实时模式)
    ├── 快速规则引擎 (< 5ms)
    │   ├── 黑名单检查
    │   ├── 设备指纹
    │   └── IP 风险
    ├── 特征计算 (< 20ms)
    │   ├── 用户历史行为
    │   └── 关联图谱
    └── LLM 决策 (< 80ms)
        ├── 综合特征 → 评分
        ├── 决策: 放行 / 拦截 / 验证
        └── 生成风控报告（用于审计）
```

> **注意**：实时风控对延迟要求极高，LLM 只参与中高风险的精细决策，不参与全量请求的初步筛查。

### 7.2 风控 Agent 的约束设计

```python
guardrails = {
    # 决策范围限制
    "allowed_decisions": ["approve", "reject", "verify"],

    # 拒绝率限制（防止 Agent 过于保守影响业务）
    "max_rejection_rate_per_hour": 0.05,  # 每小时最多拒绝 5%

    # 关键操作审批
    "require_human_review": [
        "account_freeze",       # 冻结账户
        "bulk_reject",          # 批量拒绝 > 100 笔
        "vip_customer_reject",  # 拒绝 VIP 客户
    ],

    # 可解释性要求
    "must_provide_reason": True,  # 每个决策必须附带原因
    "reason_language": "zh-CN",   # 中文，客服可读

    # 审计
    "audit_all_decisions": True,
}
```

---

## 八、成本治理

### 8.1 Token 预算架构

```go
type TokenBudget struct {
    PerRequest  int // 单次请求最大 Token
    PerSession  int // 单个会话总 Token
    PerUserDay  int // 单用户每日 Token
    PerTenantDay int // 单租户每日 Token
}

// 不同用户等级的 Token 配额
var budgets = map[string]TokenBudget{
    "free":       {PerRequest: 2000,  PerSession: 10000,  PerUserDay: 50000},
    "pro":        {PerRequest: 8000,  PerSession: 100000, PerUserDay: 500000},
    "enterprise": {PerRequest: 32000, PerSession: 1000000, PerUserDay: -1}, // 无上限
}
```

### 8.2 模型分层策略

```
请求进来
    ↓
意图分类（小模型: GPT-4o-mini，$0.00015/1k token）
    ↓
简单查询？ → 直接调工具，不用 LLM 回答（成本 $0）
    ↓ 否
标准推理？ → 中模型: Claude 3.5 Haiku（$0.00025/1k token）
    ↓ 否
复杂推理？ → 大模型: Claude 3.5 Sonnet / GPT-4o（$0.003/1k token）
    ↓
超复杂？   → Claude 3.5 Opus（$0.015/1k token），严格限流
```

### 8.3 成本监控

```
每日成本报告维度：
- 按用户分摊（哪些用户消耗了多少）
- 按功能分摊（客服 vs 风控 vs 对账）
- 按模型分摊（大模型 vs 小模型占比）
- 按 Token 类型（输入 vs 输出，输出 Token 通常贵 4x）
- 异常告警：单用户日消耗超阈值，发告警
```

---

## 九、可观测性设计

### 9.1 OpenTelemetry 接入 Agent 链路

```go
// Agent 调用工具时注入 Span
func (a *Agent) callTool(ctx context.Context, toolName string, params map[string]any) (any, error) {
    ctx, span := tracer.Start(ctx, "agent.tool_call",
        trace.WithAttributes(
            attribute.String("tool.name", toolName),
            attribute.String("agent.id", a.id),
            attribute.String("session.id", a.sessionID),
            attribute.String("tenant.id", a.tenantID),
        ),
    )
    defer span.End()

    result, err := a.toolRegistry.Execute(ctx, toolName, params)

    // 记录关键 Span 属性
    span.SetAttributes(
        attribute.Bool("tool.success", err == nil),
        attribute.Int("tool.latency_ms", latencyMs),
    )
    if err != nil {
        span.RecordError(err)
    }
    return result, err
}
```

### 9.2 关键监控指标

```
Agent 层面：
- agent_task_completion_rate    任务完成率（最核心）
- agent_tool_call_count         工具调用次数/任务
- agent_session_duration_p99    会话时长 P99
- agent_llm_error_rate          LLM 调用失败率
- agent_guardrail_trigger_rate  护栏触发率

成本层面：
- agent_token_usage_total       总 Token 消耗
- agent_cost_per_task           每任务成本
- agent_model_distribution      模型使用分布

质量层面：
- agent_human_escalation_rate   转人工率（过高说明 Agent 能力不足）
- agent_approval_rejection_rate 审批拒绝率
- agent_hallucination_rate      幻觉检测率（需要评估集）
```

---

## 十、测试与评估

### 10.1 测试体系

```
单元测试：
  - 工具函数的输入输出
  - 权限检查逻辑
  - 护栏规则

集成测试：
  - Mock LLM 输出，测试工具调用链路
  - 测试审批流程的状态机

端到端测试（Golden Set）：
  - 维护 100+ 真实场景的测试案例
  - 输入：用户消息
  - 预期：调用了哪些工具、最终回复是否正确
  - 自动化评估：LLM-as-Judge 打分
```

### 10.2 LLM-as-Judge 评估

```python
def evaluate_agent_response(
    user_message: str,
    agent_response: str,
    ground_truth: str,
    rubric: str = "请判断 Agent 回复是否准确完整（1-5分）"
) -> EvalResult:
    prompt = f"""
    用户问题：{user_message}
    Agent 回复：{agent_response}
    标准答案：{ground_truth}
    评估标准：{rubric}

    请给出：
    1. 评分 (1-5)
    2. 评分理由
    3. 改进建议
    """
    return eval_model.invoke(prompt)
```

### 10.3 上线质量门禁

```
PR 合并前必须通过：
1. 单元测试覆盖率 > 80%
2. Golden Set 准确率 > 90%（不能比上一版下降）
3. 护栏误触发率 < 2%
4. P99 延迟 < 3s

灰度发布：
1. 5% 流量 → 观察 24h → 关键指标无恶化
2. 20% 流量 → 观察 48h
3. 100% 流量
```

---

## 十一、面试高频题（企业向）

### Q1：如何设计支付 Agent 的权限控制？

**答**：三层设计——
1. **RBAC 角色映射**：按角色（客服/主管/风控）分配工具白名单
2. **操作范围限制**：每个工具绑定 Scope（read/write）+ 金额上限
3. **运行时检查**：每次工具调用前验证权限，超阈值走审批流

支付 Agent 的核心原则：**最小权限**。客服只能查询+小额退款，大额退款必须主管审批，账户冻结必须合规+风控双人审批。

### Q2：Agent 的审计日志需要记录什么？

**答**：要满足 PCI-DSS 的话需要记录：
1. **谁操作**：user_id + role + tenant_id
2. **操作什么**：tool + params（脱敏）
3. **何时操作**：timestamp + trace_id
4. **结果如何**：success/error + 返回内容摘要
5. **是否审批**：审批单号 + 审批人 + 决策
6. **LLM 信息**：model + token_usage（用于成本审计）

关键：敏感信息（卡号、银行账号）必须脱敏后才能写日志。

### Q3：Human-in-the-Loop 如何实现不阻塞系统？

**答**：用异步审批 + 状态持久化：
1. Agent 遇到需审批操作 → 创建审批工单 → 状态存 DB → 立即返回"等待审批"
2. 系统通过 Webhook/消息队列 通知审批人（飞书/Slack）
3. 审批人在审批系统点击批准/拒绝 → 触发 Webhook → 恢复 Agent 执行
4. 用 LangGraph 的 Checkpointer 持久化 Agent 状态，支持数小时/天后恢复

这样不阻塞主线程，也不会因系统重启丢失状态。

### Q4：Agent 的幂等性如何保证？

**答**：
1. **工具层幂等**：关键操作（退款/支付）绑定幂等键（idempotency_key = task_id + action + target），相同 key 只执行一次
2. **状态机保护**：执行前检查当前状态，已完成的不重复执行
3. **LLM 重试保护**：Agent 可能因 LLM 返回相似指令重复调用工具，工具层用幂等键过滤

### Q5：如何控制 Agent 的成本？

**答**：四层策略：
1. **模型分层**：简单分类用 GPT-4o-mini，复杂推理用 Claude 3.5 Sonnet
2. **Token 预算**：每用户/会话/租户设置 Token 上限，超限降级或拦截
3. **结果缓存**：重复查询（如 FAQ、汇率）缓存 LLM 响应，命中率 30%-50%
4. **工具优先**：能直接调工具返回的不过 LLM（成本最省）

### Q6：如何防止 Agent 被 Prompt 注入？

**答**：多层防御：
1. **输入清理**：检测"忽略之前的指令"等注入模式，正则+分类器双重检测
2. **指令分离**：系统 Prompt 和用户输入严格隔离，不在同一 XML/JSON 层级
3. **工具白名单**：即使注入成功，Agent 也只能调用预授权工具
4. **输出审查**：Agent 输出过内容安全过滤器，检测异常行为（如要求用户提供密码）
5. **最小权限**：Agent 没有执行任意代码的权限，即使被注入也危害有限

### Q7：Go 后端如何与 Python Agent 框架集成？

**答**：常见架构是 Go 作 API Gateway + Python 作 Agent Runtime：
1. **接口设计**：Go 侧暴露工具服务 gRPC 接口，Python Agent 通过 gRPC 调用
2. **通信**：Agent 任务通过消息队列（Pulsar/Kafka）异步分发给 Python Worker
3. **状态管理**：Agent 状态持久化在 Redis/PostgreSQL，Go 和 Python 共享
4. **可观测性**：两侧都接入 OpenTelemetry，共享 trace_id，链路打通
5. **Go 的职责**：认证、限流、审计、路由；Python 的职责：LLM 编排、工具调用

### Q8：如何在面试中回答"你们是否用了 AI/Agent"？

**答**：如果有真实经历就讲真实的；如果没有，可以从技术角度阐述设计思路：

> "目前我们正在探索将 Agent 应用于对账和客服场景。对账方面，核心挑战是可靠性——Agent 每一步都要基于真实工具返回的数据，不能依赖 LLM 记忆，同时每个差异处理都要记录审计日志满足合规要求。客服方面，关键是权限控制，客服 Agent 只能查询和处理小额退款，大额退款走人工审批流。这两个场景都用了 LangGraph 的 Checkpoint 机制来支持 Human-in-the-Loop，状态持久化在 PostgreSQL。"

---

## 十二、知识点速查卡

| 概念 | 一句话解释 | 面试考察方向 |
|------|-----------|-------------|
| RBAC for Tools | 按角色分配工具白名单 + 操作范围 | 权限设计 |
| Human-in-the-Loop | 高风险操作暂停等待人工批准 | 审批流/可靠性 |
| LangGraph Checkpointer | Agent 状态持久化，支持暂停/恢复 | 长时任务/HITL |
| Tool Idempotency | 工具用幂等键防重复执行 | 支付安全 |
| Guardrails | 输入/过程/输出三层安全检查 | 安全/稳定性 |
| Token Budget | 每用户/会话的 Token 配额上限 | 成本控制 |
| Model Tiering | 简单任务小模型，复杂任务大模型 | 成本优化 |
| Prompt Injection | 用户输入覆盖系统指令的攻击 | 安全防御 |
| PCI-DSS | 支付卡行业数据安全标准，Agent 必须合规 | 合规意识 |
| LLM-as-Judge | 用 LLM 评估另一个 LLM 的输出质量 | 测试/评估 |
| Golden Set | 标注好的测试案例集，用于回归测试 | 质量保证 |
| Multi-tenant Isolation | 工具调用强制注入租户 Scope 防越权 | 多租户设计 |
