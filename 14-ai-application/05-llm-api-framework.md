# LLM API 与框架集成深度解析

> 面向 Go 后端开发者的 LLM 应用开发核心知识点
> 重点关注：API 调用、框架选型、流式输出、成本控制、模型网关

---

## 一、OpenAI API 深度解析

### 1. Chat Completions API 核心参数

Chat Completions 是 OpenAI 最核心的 API，几乎所有 LLM 应用的入口。

```python
import openai

client = openai.OpenAI(api_key="sk-xxx")

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[
        {"role": "system", "content": "你是一个支付系统风控专家"},
        {"role": "user", "content": "分析这笔交易的风险等级"}
    ],
    temperature=0.7,          # 随机性控制：0 确定性输出，2 高随机性
    top_p=0.9,               # 核采样：与 temperature 二选一
    max_tokens=4096,         # 最大生成 token 数
    stop=["\n\n"],           # 停止序列
    presence_penalty=0.0,    # 存在惩罚：鼓励新话题
    frequency_penalty=0.0,   # 频率惩罚：降低重复
    n=1,                     # 生成候选数
    stream=False,            # 是否流式输出
    response_format={"type": "json_object"},  # 强制 JSON 输出
    seed=42,                 # 可复现性种子（尽量确定性）
)
```

**关键参数深度理解：**

| 参数 | 作用 | 实战建议 |
|------|------|----------|
| `temperature` | 控制采样概率分布的平滑度 | 风控/对账场景用 0-0.3；创意场景用 0.7-1.0 |
| `top_p` | 只从概率累积前 p 的 token 中采样 | 与 temperature 互斥，OpenAI 建议改 temperature 而非 top_p |
| `max_tokens` | 限制输出长度（不含输入） | 注意：截断不会报错，需在业务层校验完整性 |
| `response_format` | 强制结构化输出 | 需搭配 system prompt 说明 JSON schema，否则可能输出空 JSON |
| `seed` | 使输出尽量可复现 | 并非 100% 确定，OpenAI 只保证"大部分一致" |

**messages 角色体系：**

| 角色 | 作用 | 典型用法 |
|------|------|----------|
| `system` | 设定模型行为边界和人格 | "你是支付对账助手，只回答对账相关问题" |
| `user` | 用户输入 | 用户的提问或指令 |
| `assistant` | 模型历史回复 | 多轮对话上下文 |
| `tool` | 工具调用结果 | Function Calling 返回值 |

> **面试要点**：system 消息不是"最高优先级"，模型可能被 user 消息覆盖。生产环境中，应将关键约束同时在 system 和 user 中强调，或在输出后做校验。

### 2. Assistants API

Assistants API 封装了完整的 Agent 能力，适合构建持久化、有状态的 AI 助手。

```
核心能力：
+------------------+     +------------------+     +------------------+
|    Code          |     |   File Search    |     |  Function        |
|  Interpreter     |     |   (RAG)          |     |  Calling         |
+------------------+     +------------------+     +------------------+
        |                        |                        |
        v                        v                        v
  代码沙箱执行            向量检索 + 文件解析           工具调用
  (Python only)           (自动分块+嵌入)             (自定义逻辑)
```

```python
# 创建 Assistant
assistant = client.beta.assistants.create(
    name="Payment Risk Analyzer",
    instructions="你是支付风控分析师，分析交易风险并给出建议",
    model="gpt-4o",
    tools=[
        {"type": "code_interpreter"},        # 代码解释器
        {"type": "file_search"},             # 文件检索（RAG）
        {"type": "function", "function": {   # 自定义函数
            "name": "query_transaction",
            "description": "查询交易详情",
            "parameters": {
                "type": "object",
                "properties": {
                    "txn_id": {"type": "string", "description": "交易ID"}
                },
                "required": ["txn_id"]
            }
        }}
    ]
)

# 创建 Thread（对话线程，持久化上下文）
thread = client.beta.threads.create()

# 添加消息
client.beta.threads.messages.create(
    thread_id=thread.id,
    role="user",
    content="分析交易 TXN-20240101-001 的风险"
)

# 运行（自动处理工具调用循环）
run = client.beta.threads.runs.create_and_poll(
    thread_id=thread.id,
    assistant_id=assistant.id
)
```

**Assistants API vs Chat Completions 选择：**

| 维度 | Chat Completions | Assistants API |
|------|-----------------|----------------|
| 状态管理 | 无状态，需自己维护对话历史 | 有状态，Thread 自动管理 |
| 工具调用 | 单轮 request-response | 自动循环直到完成 |
| RAG | 需自己搭建 | 内置 File Search |
| 控制粒度 | 精细控制每一步 | 抽象度高，控制力弱 |
| 适用场景 | 生产系统、需要精细控制 | 快速原型、内部工具 |

> **面试要点**：Assistants API 本质上是 OpenAI 帮你跑了一个 Agent Loop（工具调用 → 执行 → 再调用，直到完成）。生产环境中，更推荐用 Chat Completions + 自建 Agent Loop，因为可以精确控制每一步、方便 debug、降低延迟。

### 3. Streaming（流式输出）

```python
# 流式调用
stream = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "解释支付网关的工作原理"}],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content is not None:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

**流式数据格式（SSE）：**

```
data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":"支"},"index":0}]}\n\n
data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":"付"},"index":0}]}\n\n
data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":"网"},"index":0}]}\n\n
data: {"id":"chatcmpl-xxx","choices":[{"delta":{},"finish_reason":"stop"}]}\n\n
data: [DONE]\n\n
```

**关键流式参数：**
- `stream_options.include_usage`：流式结束时返回 token 用量统计
- `chunk.choices[0].finish_reason`：`stop`（正常结束）、`length`（截断）、`tool_calls`（工具调用）

### 4. Function Calling

Function Calling 是 LLM 与外部系统交互的核心机制。

```python
tools = [
    {
        "type": "function",
        "function": {
            "name": "query_payment_status",
            "description": "查询支付订单状态",
            "parameters": {
                "type": "object",
                "properties": {
                    "order_id": {
                        "type": "string",
                        "description": "支付订单号，格式：PAY-YYYYMMDD-XXXXX"
                    },
                    "merchant_id": {
                        "type": "string",
                        "description": "商户ID"
                    }
                },
                "required": ["order_id"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "refund_order",
            "description": "对支付订单发起退款",
            "parameters": {
                "type": "object",
                "properties": {
                    "order_id": {"type": "string", "description": "订单号"},
                    "amount": {"type": "number", "description": "退款金额（单位：分）"},
                    "reason": {"type": "string", "description": "退款原因"}
                },
                "required": ["order_id", "amount"]
            }
        }
    }
]

# 第一轮：模型决定调用哪个函数
response = client.chat.completions.create(
    model="gpt-4o",
    messages=messages,
    tools=tools,
    tool_choice="auto"    # auto: 模型决定 | none: 禁止 | required: 强制
)

message = response.choices[0].message

# 检查是否有工具调用
if message.tool_calls:
    for tool_call in message.tool_calls:
        function_name = tool_call.function.name
        function_args = json.loads(tool_call.function.arguments)

        # 执行实际函数
        result = execute_function(function_name, function_args)

        # 将结果返回给模型
        messages.append(message)
        messages.append({
            "role": "tool",
            "tool_call_id": tool_call.id,
            "content": json.dumps(result)
        })

    # 第二轮：模型根据工具结果生成最终回复
    final_response = client.chat.completions.create(
        model="gpt-4o",
        messages=messages,
        tools=tools
    )
```

**Function Calling 核心要点：**

| 要点 | 说明 |
|------|------|
| 模型不执行函数 | 模型只生成函数名和参数，执行由应用层完成 |
| `tool_choice` | `auto` 让模型判断；`required` 强制调用；`none` 禁止调用；也可指定具体函数 |
| 并行调用 | 模型可能一次返回多个 tool_calls，需并行执行 |
| description 很重要 | 函数描述质量直接决定模型能否正确选择和传参 |
| 参数验证必须做 | 模型生成的参数可能格式错误，必须做校验和容错 |
| 幂等性 | 工具调用可能重复触发，下游操作必须幂等 |

### 5. Structured Output

Structured Output 解决了 LLM 输出不可靠的问题，是 2024 年最重要的 API 更新之一。

```python
# 方式一：JSON Mode（宽松，只保证合法 JSON）
response = client.chat.completions.create(
    model="gpt-4o",
    messages=messages,
    response_format={"type": "json_object"}
)

# 方式二：Structured Output with Schema（严格，保证遵循 Schema）
from pydantic import BaseModel

class RiskAssessment(BaseModel):
    risk_level: str       # "high" | "medium" | "low"
    confidence: float     # 0.0 - 1.0
    factors: list[str]    # 风险因素列表
    recommendation: str   # 建议

response = client.beta.chat.completions.parse(
    model="gpt-4o-2024-08-06",
    messages=messages,
    response_format=RiskAssessment
)

result = response.choices[0].message.parsed
# result 是类型安全的 RiskAssessment 对象
```

**JSON Mode vs Structured Output：**

| 维度 | JSON Mode | Structured Output |
|------|-----------|-------------------|
| 保证级别 | 只保证合法 JSON | 保证遵循指定 JSON Schema |
| Schema 约束 | 无，需要 prompt 约束 | 强类型约束 |
| 首个 token | 可能是 `{` 之外的字符 | 一定是 `{` |
| 可靠性 | 中等，可能字段缺失 | 极高，OpenAI 保证符合 |
| 支持模型 | gpt-3.5-turbo+ | gpt-4o-2024-08-06+ |

> **面试要点**：Structured Output 的实现原理是"约束解码"（Constrained Decoding）——在每一步 token 采样时，根据 JSON Schema 的合法后续 token 集合来限制采样范围，确保输出一定符合 Schema。这是在推理引擎层面做的，不是 prompt 层面。

---

## 二、Claude API 深度解析

### 1. Messages API 核心参数

```python
import anthropic

client = anthropic.Anthropic(api_key="sk-ant-xxx")

message = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=4096,                # 必填，Claude 要求显式指定
    system="你是一个支付系统风控专家",   # system 是顶层参数，不在 messages 里
    messages=[
        {"role": "user", "content": "分析这笔交易的风险等级"}
    ],
    temperature=0.7,
    top_p=0.9,
    stop_sequences=["\n\n"],        # 停止序列
    stream=False,
    metadata={"user_id": "u-123"}   # 请求级元数据，用于追踪
)
```

**Claude vs OpenAI API 设计差异：**

| 差异点 | OpenAI | Claude |
|--------|--------|--------|
| system 消息位置 | 在 messages 数组内 | 顶层参数，独立于 messages |
| max_tokens | 可选，有默认值 | **必填**，不填会报错 |
| 消息角色 | system/user/assistant/tool | user/assistant（system 顶层） |
| 停止序列 | `stop` | `stop_sequences` |
| 元数据 | 无 | `metadata` 字段 |

> **面试要点**：Claude 的 system 放在顶层参数是一个更好的设计——它语义上不属于对话历史，放在顶层可以避免被对话上下文污染，也让 API 更清晰地区分"系统指令"和"对话内容"。

### 2. Tool Use（工具调用）

```python
response = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=4096,
    tools=[
        {
            "name": "query_transaction",
            "description": "查询交易详情",
            "input_schema": {          # 注意：Claude 用 input_schema，OpenAI 用 parameters
                "type": "object",
                "properties": {
                    "txn_id": {
                        "type": "string",
                        "description": "交易ID"
                    }
                },
                "required": ["txn_id"]
            }
        }
    ],
    tool_choice={"type": "auto"},     # auto | any(强制) | tool(指定)
    messages=[{"role": "user", "content": "查一下交易 TXN-001 的详情"}]
)

# 处理工具调用
if response.stop_reason == "tool_use":
    for block in response.content:
        if block.type == "tool_use":
            # block.id: 工具调用ID
            # block.name: 函数名
            # block.input: 参数（已是 dict，无需 json.loads）
            result = execute_function(block.name, block.input)

            # 将结果返回
            response = client.messages.create(
                model="claude-sonnet-4-20250514",
                max_tokens=4096,
                tools=tools,
                messages=[
                    {"role": "user", "content": "查一下交易 TXN-001 的详情"},
                    {"role": "assistant", "content": response.content},
                    {
                        "role": "user",
                        "content": [{
                            "type": "tool_result",
                            "tool_use_id": block.id,
                            "content": json.dumps(result)
                        }]
                    }
                ]
            )
```

**Claude Tool Use vs OpenAI Function Calling：**

| 差异 | OpenAI | Claude |
|------|--------|--------|
| 参数名 | `parameters` | `input_schema` |
| 参数解析 | 返回 JSON 字符串，需 `json.loads` | 返回 dict，直接可用 |
| 返回方式 | `role: "tool"` + `tool_call_id` | `role: "user"` + `type: "tool_result"` |
| 强制指定函数 | `tool_choice: {type: "function", function: {name}}` | `tool_choice: {type: "tool", name: "xxx"}` |
| 禁止调用 | `tool_choice: "none"` | 不传 tools 参数即可 |
| 并行调用 | 支持 | 支持 |

> **关键差异**：Claude 将工具返回结果放在 `user` 消息中，而非单独的 `tool` 角色。这设计更简洁——对模型来说，工具返回就是"用户提供了新信息"，语义更统一。

### 3. Streaming

```python
with client.messages.stream(
    model="claude-sonnet-4-20250514",
    max_tokens=4096,
    messages=[{"role": "user", "content": "解释支付网关"}]
) as stream:
    for text in stream.text_stream:
        print(text, end="", flush=True)

    # 流结束后获取完整消息
    message = stream.get_final_message()
    usage = message.usage  # token 用量
```

**Claude 流式事件类型：**

```
event: message_start       # 消息开始，包含 metadata
event: content_block_start # 内容块开始
event: content_block_delta # 内容增量（文本/工具参数）
event: content_block_stop  # 内容块结束
event: message_delta       # 消息级更新（stop_reason, usage）
event: message_stop        # 消息结束
```

Claude 的流式事件粒度比 OpenAI 更细，支持按 content block 区分文本和工具调用。

### 4. Prompt Caching

Prompt Caching 是 Claude 的差异化优势，能显著降低长上下文的成本和延迟。

```python
# 标记可缓存的上下文
response = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=4096,
    system=[
        {
            "type": "text",
            "text": "你是一个支付风控专家...",     # 不会被缓存
            "cache_control": {"type": "ephemeral"}  # 标记为可缓存
        }
    ],
    tools=[
        {
            "name": "query_transaction",
            "input_schema": large_schema,           # 大型 schema
            "cache_control": {"type": "ephemeral"}  # 缓存工具定义
        }
    ],
    messages=[
        {"role": "user", "content": long_context}   # 也可以缓存
    ]
)
```

**缓存命中规则：**

| 条件 | 说明 |
|------|------|
| 前缀匹配 | 缓存从请求的开头开始匹配，必须前缀完全一致 |
| 5 分钟 TTL | 缓存创建后 5 分钟内有效，最后一次命中后 5 分钟过期 |
| 最小 token 数 | Sonnet/Opus 至少 1024 tokens，Haiku 至少 2048 tokens |

**缓存价格（以 Sonnet 为例）：**

| 类型 | 正常价格 | 缓存写入 | 缓存读取 |
|------|---------|---------|---------|
| 输入 | $3/MTok | $3.75/MTok（+25%） | $0.30/MTok（-90%） |

**最佳实践：**
1. 将 system prompt 和工具定义放在消息开头，标记 `cache_control`
2. 对话历史放在后面，随对话推进自然失效
3. 大型知识文档（如支付规则手册）标记为缓存，多轮复用
4. 短 prompt 不要用缓存（写入成本 > 读取节省）

### 5. Extended Thinking

Extended Thinking 是 Claude 的推理增强能力，模型在回答前进行内部"思考"。

```python
response = client.messages.create(
    model="claude-sonnet-4-20250514",
    max_tokens=16000,
    thinking={
        "type": "enabled",
        "budget_tokens": 10000     # 思考 token 预算
    },
    messages=[{"role": "user", "content": "设计一个高并发支付系统的架构"}]
)

# thinking 内容在 content block 中
for block in response.content:
    if block.type == "thinking":
        print(f"思考过程: {block.thinking}")
    elif block.type == "text":
        print(f"最终回答: {block.text}")
```

**关键特性：**

| 特性 | 说明 |
|------|------|
| thinking block | 模型的内部推理过程，不暴露给用户 |
| budget_tokens | 控制思考的最大 token 数，影响推理深度 |
| 必须与 max_tokens 配合 | max_tokens 需要包含 thinking + 输出的总量 |
| 不影响工具调用 | 思考后仍可调用工具 |
| 适用场景 | 复杂推理、数学计算、架构设计、代码生成 |

> **面试要点**：Extended Thinking 类似 OpenAI 的 o1/o3 系列的"推理链"，但 Claude 的实现更透明——你可以看到 thinking block。思考 token 也要计费，需要根据任务复杂度合理设置 budget。

---

## 三、OpenAI vs Claude API 全面对比

| 维度 | OpenAI | Claude |
|------|--------|--------|
| **旗舰模型** | GPT-4o / o3 | Claude Opus 4 / Sonnet 4 |
| **上下文窗口** | 128K (GPT-4o) | 200K (Sonnet/Opus) |
| **API 设计** | messages 内含 system | system 顶层参数 |
| **工具调用** | Function Calling | Tool Use |
| **工具参数格式** | `parameters` (JSON string) | `input_schema` (dict) |
| **工具返回** | `role: "tool"` | `role: "user"` + `tool_result` |
| **结构化输出** | JSON Mode + Structured Output | Tool Use 实现类 JSON Schema |
| **流式粒度** | chunk 级 | content block 级（更细） |
| **Prompt Caching** | 自动缓存（无 API 控制） | 手动标记 + 显式缓存控制 |
| **推理增强** | o1/o3 内置推理链 | Extended Thinking |
| **输入价格** | $2.5/MTok (GPT-4o) | $3/MTok (Sonnet 4) |
| **输出价格** | $10/MTok (GPT-4o) | $15/MTok (Sonnet 4) |
| **缓存节省** | 自动，无显式折扣 | 缓存读取 -90% |
| **Assistants API** | 有（完整 Agent 抽象） | 无，需自建 |
| **多模态** | 文本+图像+音频 | 文本+图像 |
| **安全性** | 标准安全过滤 | Constitution AI，更谨慎 |
| **SDK 语言** | Python, Node.js, Go(社区) | Python, Node.js |

**选型建议：**

- **长上下文 + 成本敏感**：Claude（200K 窗口 + Prompt Caching）
- **快速原型 + 需要内置 Agent**：OpenAI（Assistants API）
- **结构化输出严格性**：OpenAI（Structured Output 有 Schema 保证）
- **推理任务**：根据具体任务测试 o3 vs Claude Extended Thinking
- **生产系统**：两者都应通过模型网关接入，方便切换

---

## 四、LLM 应用框架对比

### 框架全景

| 框架 | 定位 | 核心抽象 | 适用场景 | 学习曲线 | 语言 |
|------|------|----------|----------|----------|------|
| **LangChain** | 通用 LLM 应用开发框架 | Chain / Agent / Tool / Memory | Agent、工具编排、对话系统 | 陡峭，概念多 | Python/JS |
| **LlamaIndex** | LLM 数据框架（RAG 优先） | Index / Query Engine / Node / Retriever | RAG、文档问答、知识库 | 中等 | Python/TS |
| **Haystack** | 生产级 NLP 管道 | Pipeline / Component / Node | 搜索增强、NLP 管道、生产部署 | 中等偏低 | Python |
| **DSPy** | LLM 编程框架（声明式） | Signature / Module / Optimizer | Prompt 优化、自动调参、学术研究 | 陡峭（新范式） | Python |

**如何选择？**

```
你的需求是什么？
│
├── 构建 Agent / 工具编排 ──→ LangChain
│
├── 构建 RAG / 文档问答 ──→ LlamaIndex
│
├── 生产级 NLP 管道 ──→ Haystack
│
├── Prompt 自动优化 / 研究 ──→ DSPy
│
└── 简单 API 调用 ──→ 直接用 SDK，不需要框架
```

> **面试要点**：框架不是必须的。对于简单的 LLM 调用场景（如 API 代理、结构化提取），直接用 SDK 更轻量、更可控。框架的价值在于：复杂工具编排、RAG 管道、可复用组件。Go 后端开发者尤其要注意：主流框架都是 Python 生态，Go 侧更多是做 API 网关和后端服务。

---

## 五、LangChain 核心概念

### 架构概览

```
用户输入
    │
    ▼
┌─────────────────────────────────────────────┐
│              LCEL (LangChain Expression)      │
│  ┌─────────┐  ┌──────────┐  ┌────────────┐  │
│  │ Prompt  │──│   LLM    │──│  Output     │  │
│  │Template │  │          │  │  Parser     │  │
│  └─────────┘  └──────────┘  └────────────┘  │
│       │              │            │          │
│  ┌─────────┐  ┌──────────┐  ┌────────────┐  │
│  │ Retriever│  │   Tool   │  │   Memory   │  │
│  └─────────┘  └──────────┘  └────────────┘  │
│                     │                        │
│              ┌──────────────┐                 │
│              │    Agent     │                 │
│              │ (决策+编排)   │                 │
│              └──────────────┘                 │
│                     │                        │
│              ┌──────────────┐                 │
│              │   Callback   │                 │
│              │ (观测+日志)   │                 │
│              └──────────────┘                 │
└─────────────────────────────────────────────┘
    │
    ▼
  输出
```

### 核心概念详解

| 概念 | 作用 | 类比 |
|------|------|------|
| **Chain** | 将多个组件串联成管道 | Unix pipe |
| **Agent** | 根据输入动态选择工具和决策 | 智能路由器 |
| **Tool** | 封装外部能力供 Agent 调用 | 微服务 API |
| **Memory** | 管理对话历史和状态 | Redis Session |
| **Callback** | 钩子机制，用于日志、追踪、调试 | Middleware |
| **LCEL** | 声明式链式调用语法 | Pipeline DSL |

### LCEL（LangChain Expression Language）

LCEL 是 LangChain 的核心编排语法，通过 `|` 管道符连接组件。

```python
from langchain_openai import ChatOpenAI
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.output_parsers import StrOutputParser, JsonOutputParser
from langchain_core.runnables import RunnablePassthrough

# 基础链式调用
chain = ChatPromptTemplate.from_template(
    "分析以下交易的风险等级，输出JSON：{transaction}"
) | ChatOpenAI(model="gpt-4o") | JsonOutputParser()

result = chain.invoke({"transaction": "金额50000元，新设备，异地登录"})
```

```python
# LCEL 高级用法：RAG + 工具 + 并行
from langchain_core.runnables import RunnableParallel

# 并行执行两个分支
parallel_chain = RunnableParallel({
    "context": retriever | format_docs,     # RAG 检索
    "question": RunnablePassthrough()       # 原样传递
})

# 组装完整 RAG 链
rag_chain = parallel_chain | ChatPromptTemplate.from_template(
    "基于以下上下文回答问题：\n{context}\n\n问题：{question}"
) | ChatOpenAI(model="gpt-4o") | StrOutputParser()

# 流式调用
for chunk in rag_chain.stream("支付失败的原因有哪些？"):
    print(chunk, end="", flush=True)

# 批量调用
results = rag_chain.batch([
    "什么是 T+1 结算？",
    "如何处理重复支付？"
])

# 异步调用
async for chunk in rag_chain.astream("解释风控规则"):
    print(chunk, end="", flush=True)
```

**LCEL 核心接口（Runnable）：**

| 方法 | 作用 | 说明 |
|------|------|------|
| `invoke()` | 单次同步调用 | 最基本的方法 |
| `stream()` | 流式输出 | 逐 token 返回 |
| `batch()` | 批量调用 | 并发处理多个输入 |
| `ainvoke()` | 异步单次调用 | 非阻塞 |
| `astream()` | 异步流式 | 适合 Web 服务 |
| `abatch()` | 异步批量 | 高并发场景 |

> **面试要点**：LCEL 的本质是实现了 `Runnable` 接口的组件可以自由组合。每个组件都有 `invoke/stream/batch` 等统一方法，所以任何子链都可以当作独立组件使用。这比旧的 `LLMChain` 类更灵活、更可组合。

### Agent 与工具

```python
from langchain.agents import create_tool_calling_agent, AgentExecutor
from langchain_core.tools import tool

@tool
def query_payment(order_id: str) -> str:
    """查询支付订单状态"""
    # 实际调用支付系统 API
    return f"订单 {order_id} 状态：已支付，金额 99.00 元"

@tool
def check_risk(order_id: str) -> str:
    """查询订单风控评估结果"""
    return f"订单 {order_id} 风控等级：低风险"

tools = [query_payment, check_risk]

agent = create_tool_calling_agent(
    llm=ChatOpenAI(model="gpt-4o"),
    tools=tools,
    prompt=ChatPromptTemplate.from_messages([
        ("system", "你是支付系统客服助手"),
        ("human", "{input}"),
        ("placeholder", "{agent_scratchpad}"),  # 工具调用历史
    ])
)

executor = AgentExecutor(agent=agent, tools=tools, verbose=True)
result = executor.invoke({"input": "查一下订单 PAY-001 的状态和风控"})
```

---

## 六、LlamaIndex 核心概念

### 架构概览

```
文档 (PDF/HTML/DB/...)
    │
    ▼
┌──────────┐    ┌──────────┐    ┌──────────────┐
│  Loader   │───▶│  Node    │───▶│    Index     │
│ (数据加载) │    │ (分块节点)│    │   (索引结构)  │
└──────────┘    └──────────┘    └──────────────┘
                                       │
                                       ▼
                                ┌──────────────┐
                 ┌─────────────│   Retriever   │──────────────┐
                 │              │  (检索器)      │              │
                 │              └──────────────┘              │
                 │                     │                      │
                 ▼                     ▼                      ▼
          ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
          │   Response    │    │   Response   │    │   Response   │
          │  Synthesizer  │◀───│   Synthesizer│◀───│  Synthesizer │
          │  (响应合成)    │    │              │    │              │
          └──────────────┘    └──────────────┘    └──────────────┘
                 │
                 ▼
            最终回答
```

### 核心概念详解

| 概念 | 作用 | 关键实现 |
|------|------|----------|
| **Document** | 原始文档的抽象 | 包含 text + metadata |
| **Node** | 文档的分块单元 | LlamaIndex 最小索引单位，继承 Document 的 metadata |
| **Index** | 数据索引结构 | Vector Index / Summary Index / Keyword Index / 知识图谱 |
| **Retriever** | 从 Index 中检索相关 Node | 向量检索 / 关键词检索 / 混合检索 |
| **Response Synthesizer** | 将检索结果合成为最终回答 | refine / compact / tree_summarize |
| **Query Engine** | 端到端查询引擎 = Retriever + Response Synthesizer | 高级封装，开箱即用 |

### 基础用法

```python
from llama_index.core import VectorStoreIndex, SimpleDirectoryReader, Settings
from llama_index.core.node_parser import SentenceSplitter

# 1. 加载文档
documents = SimpleDirectoryReader("./payment_docs").load_data()

# 2. 文档分块
splitter = SentenceSplitter(chunk_size=1024, chunk_overlap=200)
nodes = splitter.get_nodes_from_documents(documents)

# 3. 构建索引
index = VectorStoreIndex(nodes)

# 4. 创建查询引擎
query_engine = index.as_query_engine(
    similarity_top_k=5,                      # 检索 top-5 相关节点
    response_mode="compact",                  # 响应合成模式
    streaming=True                           # 支持流式
)

# 5. 查询
response = query_engine.query("T+1 结算规则是什么？")
print(response)

# 流式查询
streaming_response = query_engine.query("退款流程是什么？")
for text in streaming_response.response_gen:
    print(text, end="", flush=True)
```

### 高级 RAG 模式

```python
# 混合检索（向量 + 关键词）
from llama_index.core.retrievers import QueryFusionRetriever

vector_retriever = index.as_retriever(similarity_top_k=5)
bm25_retriever = BM25Retriever.from_defaults(nodes=nodes, similarity_top_k=5)

# Reciprocal Rank Fusion 融合
hybrid_retriever = QueryFusionRetriever(
    retrievers=[vector_retriever, bm25_retriever],
    num_queries=1,      # 不改写查询
    similarity_cutoff=0.5
)

# 子问题分解（复杂查询自动拆解）
from llama_index.core.query_engine import SubQuestionQueryEngine

query_engine = SubQuestionQueryEngine.from_defaults(
    query_engine_tools=[
        QueryEngineTool(
            query_engine=index.as_query_engine(),
            metadata=ToolMetadata(
                name="payment_rules",
                description="支付规则和流程文档"
            )
        )
    ]
)

# 自动将"对比 T+1 和 D+0 结算的优劣"拆解为子问题
response = query_engine.query("对比 T+1 和 D+0 结算的优劣")
```

---

## 七、流式输出详解

### SSE（Server-Sent Events）原理

SSE 是 LLM 流式输出的主流传输协议。

```
客户端                              服务端
  │                                   │
  │  ── GET /stream ─────────────────▶ │  建立连接
  │                                   │
  │  ◀── data: {"token": "支"} ────── │  推送 chunk 1
  │  ◀── data: {"token": "付"} ────── │  推送 chunk 2
  │  ◀── data: {"token": "网"} ────── │  推送 chunk 3
  │  ◀── data: {"token": "关"} ────── │  推送 chunk 4
  │  ◀── data: [DONE] ────────────── │  结束
  │                                   │
```

**SSE 协议规范：**

```
# 响应头
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

# 数据格式（每条消息）
field: value\n\n

# 示例
data: {"id":"1","content":"支付"}\n\n
data: {"id":"2","content":"网关"}\n\n
data: [DONE]\n\n

# 支持事件类型
event: message\n
data: {"content": "hello"}\n\n

# 支持重连
retry: 3000\n\n    # 客户端重连间隔（ms）
id: 12345\n\n      # 事件ID，用于断点续传
```

### SSE vs WebSocket

| 维度 | SSE | WebSocket |
|------|-----|-----------|
| 方向 | 服务端 → 客户端（单向） | 双向 |
| 协议 | HTTP/1.1+ | 独立协议（ws://） |
| 重连 | 浏览器自动重连 + Last-Event-ID | 需自己实现 |
| 代理友好 | 天然兼容 HTTP 代理/CDN | 部分代理不支持 |
| 适用场景 | LLM 流式输出、实时通知 | 聊天应用、游戏、双向通信 |
| 复杂度 | 低 | 中 |
| 连接开销 | 轻量 | 需握手升级 |

> **面试要点**：LLM 流式输出选 SSE 而非 WebSocket，核心原因是 LLM 输出是单向的（服务端→客户端），SSE 够用且更简单。WebSocket 适合双向交互场景（如实时聊天），但对 LLM 流式输出来说过于重量级。

### Go 服务端实现模式

```go
// 模式一：直接转发 LLM SSE
func handleStreamChat(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // 调用 OpenAI Streaming API
    stream, err := openaiClient.CreateChatCompletionStream(r.Context(),
        openai.ChatCompletionRequest{
            Model:    "gpt-4o",
            Messages: buildMessages(r),
            Stream:   true,
        })
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer stream.Close()

    for {
        response, err := stream.Recv()
        if errors.Is(err, io.EOF) {
            fmt.Fprintf(w, "data: [DONE]\n\n")
            flusher.Flush()
            return
        }
        if err != nil {
            fmt.Fprintf(w, "data: {\"error\": \"%s\"}\n\n", err.Error())
            flusher.Flush()
            return
        }

        content := response.Choices[0].Delta.Content
        if content != "" {
            data, _ := json.Marshal(map[string]string{"content": content})
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        }
    }
}
```

```go
// 模式二：Channel + Goroutine（更灵活的生产-消费模式）
func streamWithChannel(ctx context.Context, req ChatRequest) <-chan StreamChunk {
    ch := make(chan StreamChunk, 100)

    go func() {
        defer close(ch)
        stream, err := openaiClient.CreateChatCompletionStream(ctx, req.toOpenAI())
        if err != nil {
            ch <- StreamChunk{Err: err}
            return
        }
        defer stream.Close()

        for {
            resp, err := stream.Recv()
            if errors.Is(err, io.EOF) {
                ch <- StreamChunk{Done: true}
                return
            }
            if err != nil {
                ch <- StreamChunk{Err: err}
                return
            }
            ch <- StreamChunk{Content: resp.Choices[0].Delta.Content}
        }
    }()

    return ch
}

// HTTP handler 消费 channel
func handleStream(w http.ResponseWriter, r *http.Request) {
    flusher := w.(http.Flusher)
    w.Header().Set("Content-Type", "text/event-stream")

    ch := streamWithChannel(r.Context(), parseRequest(r))

    for chunk := range ch {
        if chunk.Err != nil {
            fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", chunk.Err)
            flusher.Flush()
            return
        }
        if chunk.Done {
            fmt.Fprintf(w, "data: [DONE]\n\n")
            flusher.Flush()
            return
        }
        data, _ := json.Marshal(map[string]string{"content": chunk.Content})
        fmt.Fprintf(w, "data: %s\n\n", data)
        flusher.Flush()
    }
}
```

### 客户端消费模式

```javascript
// 浏览器端：EventSource（仅 GET，不支持自定义 header）
const source = new EventSource('/api/chat/stream');
source.onmessage = (event) => {
    if (event.data === '[DONE]') { source.close(); return; }
    const chunk = JSON.parse(event.data);
    appendToUI(chunk.content);
};

// 浏览器端：fetch + ReadableStream（支持 POST + 自定义 header）
async function streamChat(message) {
    const response = await fetch('/api/chat/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer xxx' },
        body: JSON.stringify({ message })
    });

    const reader = response.body.getReader();
    const decoder = new TextDecoder();

    while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const text = decoder.decode(value);
        const lines = text.split('\n').filter(line => line.startsWith('data: '));

        for (const line of lines) {
            const data = line.slice(6);
            if (data === '[DONE]') return;
            const chunk = JSON.parse(data);
            appendToUI(chunk.content);
        }
    }
}
```

**流式输出常见问题：**

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| 前端收到数据不逐字显示 | Nginx 缓冲 | `proxy_buffering off; X-Accel-Buffering: no` |
| 连接意外断开 | 超时/代理断开 | 设置合理超时 + 客户端自动重连 |
| 跨域问题 | SSE 是 HTTP 请求 | 正常 CORS 配置即可 |
| 顺序错乱 | 多 goroutine 并发写 | 单 goroutine 负责写 response |

---

## 八、Token 计费与成本控制

### 计费模型

```
总费用 = 输入 token 费 + 输出 token 费 + 缓存费（如有）

token ≈ 0.75 个英文单词 ≈ 1-2 个中文字符
```

| 模型 | 输入价格 ($/MTok) | 输出价格 ($/MTok) | 缓存输入 | 上下文窗口 |
|------|-------------------|-------------------|----------|-----------|
| GPT-4o | 2.50 | 10.00 | 1.25（自动） | 128K |
| GPT-4o-mini | 0.15 | 0.60 | 0.075 | 128K |
| Claude Sonnet 4 | 3.00 | 15.00 | 0.30（手动） | 200K |
| Claude Haiku 3.5 | 0.80 | 4.00 | 0.08 | 200K |
| Claude Opus 4 | 15.00 | 75.00 | 1.50 | 200K |
| DeepSeek V3 | 0.27 | 1.10 | 0.07 | 128K |

> **关键发现**：输出 token 价格通常是输入的 3-5 倍。控制输出长度是降本最直接的手段。

### 上下文窗口管理

```go
// 策略一：滑动窗口 - 只保留最近 N 轮对话
func trimMessages(messages []Message, maxRounds int) []Message {
    systemMsg := messages[0] // 保留 system prompt
    chatMsgs := messages[1:]

    if len(chatMsgs) > maxRounds*2 { // 每轮 user + assistant
        chatMsgs = chatMsgs[len(chatMsgs)-maxRounds*2:]
    }

    return append([]Message{systemMsg}, chatMsgs...)
}
```

```go
// 策略二：Token 预算控制 - 按 token 数截断
func trimByTokenBudget(messages []Message, budget int) []Message {
    systemMsg := messages[0]
    chatMsgs := messages[1:]

    // 从最新消息开始，倒序累加 token
    var selected []Message
    used := countTokens(systemMsg.Content)

    for i := len(chatMsgs) - 1; i >= 0; i-- {
        msgTokens := countTokens(chatMsgs[i].Content)
        if used+msgTokens > budget {
            break
        }
        used += msgTokens
        selected = append([]Message{chatMsgs[i]}, selected...)
    }

    return append([]Message{systemMsg}, selected...)
}
```

```go
// 策略三：摘要压缩 - 用小模型压缩历史对话
func summarizeHistory(messages []Message, llm LLM) []Message {
    systemMsg := messages[0]
    oldMsgs := messages[1 : len(messages)-4] // 保留最近 2 轮
    recentMsgs := messages[len(messages)-4:]

    // 用便宜小模型生成摘要
    summary, _ := llm.Compress(fmt.Sprintf(
        "将以下对话历史压缩为关键信息摘要：\n%s", formatMessages(oldMsgs)))

    return append([]Message{systemMsg},
        Message{Role: "system", Content: "对话历史摘要：" + summary},
    )
    // 然后拼接 recentMsgs
}
```

### Prompt Caching 策略

| 策略 | 说明 | 适用模型 |
|------|------|----------|
| System 前置 | 将不变的 system prompt 放在最前面 | 通用 |
| 工具定义缓存 | 大型 tool schema 标记缓存 | Claude |
| 文档前缀缓存 | 将知识库文档放在消息前缀 | Claude（显式）/ OpenAI（自动） |
| 多用户共享前缀 | 不同用户共享相同的 system + tools | Claude |
| 对话轮次设计 | 把缓存部分和动态部分分离 | 通用 |

**成本估算示例：**

```
场景：支付风控助手，日均 10K 次调用
- System prompt: ~500 tokens（风控规则描述）
- 工具定义: ~1000 tokens（3个工具）
- 用户输入: 平均 ~200 tokens
- 模型输出: 平均 ~500 tokens

无缓存每日成本（GPT-4o）：
  输入: 10K × (500 + 1000 + 200) × $2.5/MTok = $42.5
  输出: 10K × 500 × $10/MTok = $50.0
  合计: $92.5/天

Claude 缓存后：
  缓存写入: 10K × (500 + 1000) × $3.75/MTok = $56.25（首次）
  缓存读取: 10K × (500 + 1000) × $0.30/MTok = $4.5（后续）
  非缓存输入: 10K × 200 × $3.00/MTok = $6.0
  输出: 10K × 500 × $15/MTok = $75.0
  稳态合计: ~$85.5/天（如果缓存命中率高，可进一步降低）

优化方向：
1. 缩短 system prompt（-30% 输入成本）
2. 用 GPT-4o-mini 处理简单请求（-90% 成本）
3. 限制 max_tokens 控制输出（-20% 输出成本）
4. 批量处理（Batch API 50% 折扣）
```

### OpenAI Batch API

```python
# 批量请求：50% 折扣，24 小时内返回
batch = client.batches.create(
    input_file_id=file.id,
    endpoint="/v1/chat/completions",
    completion_window="24h"     # 24 小时内完成
)

# 适用场景：离线分析、日志处理、批量分类
# 不适用：实时交互、流式输出
```

---

## 九、模型网关

### 为什么需要模型网关？

```
没有网关的问题：
┌──────────┐     ┌───────────────┐
│  业务代码  │────▶│  OpenAI SDK   │  硬编码依赖
│           │────▶│  Claude SDK   │  切换要改代码
│           │────▶│  Azure SDK    │  各家 API 不统一
└──────────┘     └───────────────┘
                       │
                  供应商故障 → 全站挂
                  限流/配额 → 无统一管理
                  成本追踪 → 无统一视图

有网关后：
┌──────────┐     ┌───────────────┐     ┌───────────────┐
│  业务代码  │────▶│   模型网关     │────▶│  OpenAI       │
│           │     │              │────▶│  Claude       │
│           │     │  统一 API     │────▶│  Azure        │
│           │     │  路由/降级    │────▶│  本地模型      │
│           │     │  限流/计费    │     └───────────────┘
└──────────┘     └───────────────┘
  只需对接一个 API    统一管理所有供应商
```

**核心价值：**

1. **统一接口**：一套 API 对接所有模型供应商
2. **灵活路由**：按成本、延迟、质量自动选择模型
3. **故障降级**：主模型不可用时自动切换备选
4. **限流控制**：统一管理调用频率，防止超额
5. **成本追踪**：统一计费视图，按业务/团队拆分
6. **合规审计**：统一的请求日志和内容审核

### 主流模型网关对比

| 网关 | 语言 | 核心特点 | 部署方式 | 适用场景 |
|------|------|----------|----------|----------|
| **LiteLLM** | Python | OpenAI 格式代理 100+ 模型 | 自部署 / Docker | Python 项目、灵活路由 |
| **One API** | Go+React | 中文生态、多租户、额度管理 | 自部署 / Docker | 团队共享、国内模型 |
| **OpenRouter** | SaaS | 无需部署、即开即用 | 云服务 | 快速验证、无运维 |

### LiteLLM

```python
# 安装
pip install litellm

# 配置（litellm_config.yaml）
model_list:
  - model_name: gpt-4o            # 对外暴露名
    litellm_params:
      model: openai/gpt-4o
      api_key: os.environ/OPENAI_KEY
  - model_name: gpt-4o            # 同名 = 自动负载均衡
    litellm_params:
      model: azure/gpt-4o
      api_base: https://xxx.openai.azure.com
      api_key: os.environ/AZURE_KEY
  - model_name: gpt-4o-fallback   # 降级模型
    litellm_params:
      model: anthropic/claude-sonnet-4-20250514
      api_key: os.environ/ANTHROPIC_KEY

router_settings:
  routing_strategy: "latency-based-routing"  # 延迟优先
  num_retries: 2
  fallbacks:
    - {"gpt-4o": ["gpt-4o-fallback"]}       # 降级链

# 启动代理服务
litellm --config litellm_config.yaml --port 4000
```

```python
# 业务代码：统一使用 OpenAI 格式
import openai

client = openai.OpenAI(
    api_key="any",               # 网关管理 key
    base_url="http://localhost:4000"
)

# 调用方式完全一样，网关负责路由
response = client.chat.completions.create(
    model="gpt-4o",              # 网关自动路由到最优供应商
    messages=[{"role": "user", "content": "你好"}]
)
```

### One API

```
核心特性：
├── 多租户管理
│   ├── 用户/分组/令牌 三级权限
│   ├── 额度控制（按 token 计费）
│   └── 使用量统计
├── 模型渠道
│   ├── 支持所有主流模型供应商
│   ├── 自动重试 + 降级
│   └── 负载均衡
├── 中文生态
│   ├── 国内模型优先支持（通义、文心、智谱）
│   ├── 微信/钉钉通知
│   └── 中文文档完善
└── Go 实现
    ├── 单二进制部署
    ├── SQLite / MySQL / PostgreSQL
    └── 性能好，资源占用低
```

> **Go 后端开发者注意**：One API 用 Go 实现，与你的技术栈一致。如果你需要自建模型网关，One API 是最自然的选择——可以阅读源码、二次开发、与现有 Go 微服务集成。

### 网关核心功能清单

| 功能 | 说明 | 优先级 |
|------|------|--------|
| **统一 API** | 所有供应商用 OpenAI 兼容格式 | P0 |
| **路由策略** | 轮询 / 延迟优先 / 成本优先 / 权重 | P0 |
| **故障降级** | 主模型失败自动切换备选 | P0 |
| **重试机制** | 超时/限流时自动重试 | P0 |
| **Key 管理** | 多 Key 轮询、额度管理 | P1 |
| **限流控制** | 全局/按用户/按模型 QPS 限制 | P1 |
| **成本追踪** | Token 用量统计、费用估算 | P1 |
| **流式透传** | 支持 SSE 流式转发 | P1 |
| **请求日志** | 完整请求/响应记录 | P2 |
| **内容审核** | 输入/输出内容过滤 | P2 |
| **缓存层** | 相同请求返回缓存结果 | P2 |
| **虚拟 Key** | 给业务方发放独立 Key | P2 |

---

## 十、面试高频题

### Q1：OpenAI Function Calling 的执行流程是什么？模型真的会执行函数吗？

**答**：不会。Function Calling 的流程是：
1. 应用发送消息 + 工具定义给 LLM
2. LLM 判断是否需要调用工具，如果需要，返回工具名和参数（不执行）
3. 应用解析工具名和参数，执行实际函数
4. 应用将函数返回结果作为 `role: tool` 消息追加到上下文
5. 再次调用 LLM，LLM 根据工具结果生成最终回复

关键点：LLM 只负责"决策"，不负责"执行"。这种设计是安全的——防止模型直接操作外部系统。

### Q2：SSE 和 WebSocket 在 LLM 流式输出场景下怎么选？

**答**：优先选 SSE。原因：
1. LLM 输出是单向的（服务端→客户端），SSE 足够
2. SSE 基于 HTTP，天然兼容代理、CDN、负载均衡
3. SSE 浏览器原生支持自动重连（EventSource）
4. WebSocket 需要额外握手升级，部分代理不支持
5. SSE 实现更简单，Go 一个 handler 搞定

只有在需要双向通信（如实时聊天 + 流式输出同时存在）时才考虑 WebSocket。

### Q3：如何控制 LLM 调用成本？

**答**：四个维度：
1. **模型选择**：简单任务用 mini/haiku，复杂任务才用旗舰模型
2. **上下文管理**：滑动窗口/摘要压缩/限制 max_tokens
3. **缓存策略**：Claude Prompt Caching（-90%）、OpenAI 自动缓存
4. **批量处理**：OpenAI Batch API 50% 折扣，适合离线任务

实际操作：先用 GPT-4o 跑通业务 → 统计 prompt 分布 → 将简单请求路由到 mini → 给复杂请求加缓存 → 离线任务走 Batch。

### Q4：LangChain 和 LlamaIndex 怎么选？什么时候不需要框架？

**答**：
- **LlamaIndex**：核心做 RAG，数据接入和检索优化做得好
- **LangChain**：核心做 Agent 和工具编排，通用性更强
- **不需要框架**：简单的 API 调用 + 结构化输出，直接用 SDK 更轻量

Go 后端开发者尤其注意：主流框架都是 Python 生态。如果你的 LLM 调用只是"API 代理 + 结构化提取"，直接用 Go SDK + 模型网关就够了，不需要引入 Python 框架。

### Q5：什么是 Prompt Caching？为什么 Claude 的缓存比 OpenAI 更有优势？

**答**：Prompt Caching 是将已处理的 prompt 前缀缓存在推理服务器上，避免重复计算。

Claude 的优势：
1. **显式控制**：开发者用 `cache_control` 标记哪些内容需要缓存
2. **大幅折扣**：缓存读取只需 10% 价格
3. **长上下文利好**：200K 窗口 + 缓存，长文档场景成本优势巨大
4. **可预测**：知道哪些请求会命中缓存

OpenAI 的缓存是自动的（对长 prompt 自动缓存），但开发者无法控制，且折扣不如 Claude 明确。

### Q6：模型网关解决了什么问题？生产环境必须用吗？

**答**：解决的核心问题：
1. **供应商锁定**：一套 API 切换任意模型
2. **故障降级**：主模型挂了自动切换
3. **统一管理**：Key、限流、计费一处搞定

生产环境是否必须？取决于规模：
- 小规模（<1K QPS）：可以不用，直接 SDK 调用
- 中规模（1K-10K QPS）：建议用，方便降级和成本追踪
- 大规模（>10K QPS）：必须用，否则运维成本极高

Go 项目推荐 One API，Python 项目推荐 LiteLLM。

### Q7：Structured Output 的 JSON Mode 和 Schema 约束有什么区别？

**答**：
- **JSON Mode**：只保证输出是合法 JSON，不保证字段名和类型正确。需要 prompt 约束，可能字段缺失
- **Structured Output (Schema)**：保证输出严格遵循 JSON Schema，字段名、类型、必填都保证。实现原理是约束解码——每步采样时只从 Schema 合法的 token 中选

生产环境必须用 Schema 约束，JSON Mode 不够可靠。

### Q8：如何在 Go 中实现 LLM 调用的重试和降级？

**答**：

```go
// 降级链：主模型 → 备选模型 → 最小模型
type FallbackClient struct {
    providers []Provider  // 按优先级排序
    timeout   time.Duration
    maxRetry  int
}

func (fc *FallbackClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
    var lastErr error
    for _, provider := range fc.providers {
        for retry := 0; retry < fc.maxRetry; retry++ {
            ctx, cancel := context.WithTimeout(ctx, fc.timeout)
            resp, err := provider.Chat(ctx, req)
            cancel()

            if err == nil {
                return resp, nil
            }
            if !isRetryable(err) {  // 4xx 不重试
                break
            }
            lastErr = err
            time.Sleep(backoff(retry))
        }
    }
    return ChatResponse{}, fmt.Errorf("all providers failed: %w", lastErr)
}
```

关键设计：4xx 不重试（客户端错误）、5xx 才重试、跨 provider 降级、每个 provider 内部重试。

### Q9：Assistants API 和自建 Agent Loop 怎么选？

**答**：

| 维度 | Assistants API | 自建 Agent Loop |
|------|---------------|-----------------|
| 开发速度 | 快，几行代码搞定 | 慢，需自己写循环 |
| 控制力 | 弱，不能精细控制每步 | 强，可以插日志、限流、校验 |
| 可观测性 | 差，黑盒 | 好，每步可追踪 |
| 延迟 | 高，每次 Run 有启动开销 | 低，直接 API 调用 |
| 成本 | 高，Thread 存储有费用 | 低，只付 API 调用费 |
| 适用场景 | 原型验证、内部工具 | 生产系统 |

生产环境推荐自建，核心是 Chat Completions + while 循环 + 工具执行 + 上下文管理。

### Q10：Claude 的 Extended Thinking 和 OpenAI 的 o1/o3 有什么区别？

**答**：

| 维度 | Claude Extended Thinking | OpenAI o1/o3 |
|------|------------------------|--------------|
| 可见性 | thinking block 可见 | 不可见（黑盒） |
| 可控性 | budget_tokens 可控 | reasoning_effort 可调 |
| 适用模型 | Sonnet 4, Opus 4 | o1, o3, o3-mini |
| 计费 | thinking token 计费 | reasoning token 计费 |
| 工具调用 | 支持思考后调用 | 支持 |
| 流式 | 支持 | 支持 |

两者原理类似（Chain of Thought 推理），但 Claude 更透明。实际选择时，建议针对具体任务做 A/B 测试。

### Q11：LLM 应用中如何做可观测性？

**答**：三个维度：
1. **请求追踪**：记录每次 LLM 调用的 input/output/tokens/latency/model
2. **成本监控**：按业务/团队/模型统计 token 用量和费用
3. **质量评估**：抽样评估输出质量，关注幻觉率和准确率

工具选择：LangSmith（LangChain 生态）、LangFuse（开源）、Helicone（代理层）

Go 后端建议：在模型网关层统一做日志，用 OpenTelemetry trace 串联 LLM 调用到业务链路。

### Q12：如何评估 LLM 应用的输出质量？

**答**：
- **人工评估**：抽样标注，最可靠但成本高
- **LLM-as-Judge**：用 GPT-4o 评估输出质量，适合大规模评估
- **自动化指标**：exact match / F1 / BLEU / ROUGE（适合分类、抽取任务）
- **A/B 测试**：线上对比不同模型/prompt 的效果

关键原则：先定义评估维度（准确性、完整性、格式正确性），再选评估方法。评估集要持续更新，防止过拟合。

---

## 参考资源

- [OpenAI API Reference](https://platform.openai.com/docs/api-reference)
- [Anthropic API Reference](https://docs.anthropic.com/en/api)
- [LangChain Documentation](https://python.langchain.com/docs/)
- [LlamaIndex Documentation](https://docs.llamaindex.ai/)
- [LiteLLM Documentation](https://docs.litellm.ai/)
- [One API GitHub](https://github.com/songquanpeng/one-api)
