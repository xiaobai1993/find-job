# MCP (Model Context Protocol) 深度解析

> 面向 Go 后端开发者的 MCP 协议面试深度指南，侧重 "为什么" 和 "怎么做"。

---

## 1. MCP 是什么

### 1.1 定义与定位

Model Context Protocol (MCP) 是由 Anthropic 于 2024 年 11 月发布的**开放标准协议**，旨在标准化 LLM 应用与外部数据源、工具之间的连接方式。2025 年 12 月，MCP 被捐赠给 Linux 基金会下的 Agentic AI Foundation (AAIF)，由 Anthropic、OpenAI、Google、Microsoft、AWS 等联合治理。

用一个类比：**MCP 之于 AI 应用，如同 USB-C 之于硬件** — 一个通用连接器，让任意 AI 应用接入任意工具。

### 1.2 解决什么问题：N×M 集成噩梦

MCP 出现之前，每接入一个新工具，每个 AI 应用都需要单独写集成代码：

```
N 个工具 × M 个 AI 客户端 = N×M 个集成点
```

5 个工具 + 5 个客户端 = 25 个集成。MCP 将其转为 N+M：

```
N 个工具各自实现 1 个 MCP Server
M 个客户端各自实现 1 个 MCP Client
→ 总共 N+M 个实现
```

**核心价值**：
- **标准化发现**：Client 通过 `tools/list` 自动发现可用工具，无需硬编码
- **可复用**：一个 MCP Server 可被任意 MCP 兼容的 Host 使用
- **上下文保持**：模型在不同工具间切换时保持上下文连续性
- **安全模型**：OAuth 2.0、TLS、沙箱、用户审批流

### 1.3 MCP 与 Function Calling 的关系和区别

这是面试高频问题。两者不是竞争关系，而是**互补的层次**：

| 维度 | Function Calling | MCP |
|------|-----------------|-----|
| 本质 | LLM 的内置能力：模型输出结构化 JSON 请求调用函数 | 开放协议：标准化工具的发现、托管和消费方式 |
| 类比 | "我现在需要打一辆 Uber" | "所有网约车服务如何可靠地接入同一个平台" |
| 架构 | 紧耦合：工具 schema 嵌入 API 请求，执行在应用内 | 松耦合：Client-Server 架构，工具逻辑独立于 AI 应用 |
| 模型可移植性 | 绑定特定 LLM 提供商（如 OpenAI） | 同一 Server 可对接 Claude、GPT、Gemini、本地模型 |
| 工具定义格式 | JSON Schema 嵌入 `tools` 数组 | MCP 原生 schema，有 Tools、Resources、Prompts 三种原语 |
| 延迟 | 低：单次 API 调用内完成 | 略高：需 Client→Server 一跳（本地通常 <10ms） |
| 安全 | 应用层控制，凭证在同一进程 | 凭证隔离在 Server 端，最小权限原则 |
| 扩展性 | 添加工具需改应用代码 | 运行时动态发现和加载工具 |
| 适用场景 | 快速原型、少量工具、单提供商 | 多工具系统、跨提供商、生产级 Agent |

**面试要点**：Function Calling 解决的是"模型何时调用哪个函数"的决策问题；MCP 解决的是"工具如何被标准化地发现、托管和消费"的基础设施问题。实际生产中常两者结合使用 — Function Calling 处理应用特定工具，MCP 处理共享基础设施工具。

---

## 2. 核心架构：Host → Client → Server

### 2.1 三层模型

```
┌─────────────────────────────────────────────┐
│  Host（宿主应用）                              │
│  例如：Claude Desktop、Cursor、VS Code Copilot  │
│                                               │
│  ┌─────────────┐  ┌─────────────┐            │
│  │  MCP Client  │  │  MCP Client  │           │
│  │  (连接器 A)   │  │  (连接器 B)   │           │
│  └──────┬───────┘  └──────┬───────┘           │
└─────────┼──────────────────┼──────────────────┘
          │                  │
    JSON-RPC 2.0      JSON-RPC 2.0
    (stdio/HTTP)      (stdio/HTTP)
          │                  │
   ┌──────▼───────┐  ┌──────▼───────┐
   │  MCP Server  │  │  MCP Server  │
   │  (GitHub)    │  │  (Postgres)  │
   └──────────────┘  └──────────────┘
```

| 角色 | 职责 | 示例 |
|------|------|------|
| **Host** | 宿主应用，管理多个 Client，协调 LLM 与工具交互 | Claude Desktop、Cursor |
| **Client** | 协议连接器，与 Server 建立 1:1 连接，处理初始化、能力协商、消息路由 | Claude Desktop 内置的 Client 实例 |
| **Server** | 提供上下文和工具，暴露 Tools/Resources/Prompts，执行具体操作 | GitHub MCP Server、Postgres MCP Server |

### 2.2 通信流程

完整的 MCP 连接生命周期：

```
1. 初始化（Initialize）
   Client ──→ Server: initialize { protocolVersion, capabilities, clientInfo }
   Client ←── Server: { protocolVersion, capabilities, serverInfo }
   Client ──→ Server: notifications/initialized

2. 能力协商
   - 双方声明支持哪些原语（tools/resources/prompts）
   - 协商协议版本（如 "2025-06-18"）
   - 如版本不兼容则终止连接

3. 正常通信
   Client ──→ Server: tools/list
   Client ←── Server: [tool1, tool2, ...]
   Client ──→ Server: tools/call { name: "get_user", arguments: {...} }
   Client ←── Server: { content: [...], isError: false }

4. 动态更新
   Server ──→ Client: notifications/tools/list_changed
   Client ──→ Server: tools/list  (重新获取列表)

5. 关闭连接
   任一方关闭传输通道
```

### 2.3 两层设计

MCP 在概念上分为两层：

| 层 | 职责 |
|----|------|
| **数据层（Data Layer）** | JSON-RPC 2.0 消息格式、生命周期管理、原语定义（Tools/Resources/Prompts）、通知机制 |
| **传输层（Transport Layer）** | 消息的实际传输通道，包括连接建立、消息帧化、授权 |

---

## 3. 原语（Primitives）

MCP 定义了三种核心原语，每种有不同的控制方和交互模式：

| 原语 | 控制方 | 用途 | 发现方法 | 执行方法 |
|------|--------|------|----------|----------|
| **Tools** | 模型控制（Model-controlled） | 模型可调用的函数/操作 | `tools/list` | `tools/call` |
| **Resources** | 应用控制（Application-controlled） | 模型可读取的数据/上下文 | `resources/list` | `resources/read` |
| **Prompts** | 用户控制（User-controlled） | 预定义的 Prompt 模板 | `prompts/list` | `prompts/get` |

### 3.1 Tools：模型可调用的函数

Tool 是模型决定调用并执行的操作。模型根据 Tool 的名称和描述来判断何时调用。

**工具定义 Schema**：

```json
{
  "name": "get_payment_status",
  "title": "查询支付状态",
  "description": "根据订单号查询支付状态，返回支付结果和详细信息",
  "inputSchema": {
    "type": "object",
    "properties": {
      "order_id": {
        "type": "string",
        "description": "订单号"
      },
      "include_details": {
        "type": "boolean",
        "description": "是否包含详细信息",
        "default": false
      }
    },
    "required": ["order_id"]
  },
  "annotations": {
    "readOnlyHint": true,
    "idempotentHint": true
  }
}
```

**工具调用请求/响应**：

```json
// 请求
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "get_payment_status",
    "arguments": {
      "order_id": "PAY20250618001",
      "include_details": true
    }
  }
}

// 响应
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"status\": \"SUCCESS\", \"amount\": 99.9, \"method\": \"WeChat Pay\"}"
      }
    ],
    "isError": false
  }
}
```

**工具设计最佳实践**：
- 工具名称要明确无歧义：`get_user_by_email` 而非 `get_user`
- 将读操作和写操作分开定义（模型对写操作更谨慎）
- 在 `inputSchema` 中使用 `enum` 约束减少幻觉
- 描述中明确说明工具何时用、何时不该用
- 利用 `annotations` 声明 `readOnlyHint`、`idempotentHint`、`destructiveHint`

### 3.2 Resources：模型可读取的数据

Resource 是 Server 暴露给 Client 的只读数据，由应用（而非模型）决定何时读取。

**资源定义**：

```json
{
  "uri": "db://payments/schema",
  "name": "payments_schema",
  "title": "支付表结构",
  "mimeType": "application/json",
  "annotations": {
    "audience": ["user", "assistant"],
    "priority": 0.8,
    "lastModified": "2025-06-18T10:00:00Z"
  }
}
```

**资源 URI 机制**：

MCP 支持标准 URI scheme，也支持自定义 scheme：

| URI Scheme | 用途 |
|-----------|------|
| `file:///` | 本地文件 |
| `https://` | 远程 HTTP 资源 |
| `db://` | 数据库资源（自定义） |
| `greeting://{name}` | 资源模板，支持参数化 |

**资源模板**允许动态 URI：

```json
{
  "uriTemplate": "db://payments/{order_id}",
  "name": "payment_detail",
  "description": "获取指定订单的支付详情"
}
```

**Resource vs Tool 的选择**：Resource 是被动的数据（应用决定读取），Tool 是主动的操作（模型决定调用）。例如，数据库 schema 是 Resource，执行 SQL 查询是 Tool。

### 3.3 Prompts：预定义的 Prompt 模板

Prompt 是 Server 暴露的可复用提示词模板，由用户选择触发。

**Prompt 定义**：

```json
{
  "name": "code_review",
  "title": "代码审查",
  "description": "对代码进行质量审查和改进建议",
  "arguments": [
    {
      "name": "code",
      "description": "待审查的代码",
      "required": true
    },
    {
      "name": "language",
      "description": "编程语言",
      "required": false
    }
  ]
}
```

**获取 Prompt**：

```json
// 请求
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "prompts/get",
  "params": {
    "name": "code_review",
    "arguments": {
      "code": "func Add(a, b int) int { return a + b }",
      "language": "Go"
    }
  }
}

// 响应
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "description": "Code review prompt",
    "messages": [
      {
        "role": "user",
        "content": {
          "type": "text",
          "text": "Please review this Go code:\nfunc Add(a, b int) int { return a + b }"
        }
      }
    ]
  }
}
```

### 3.4 扩展能力（Client 端原语）

MCP 还定义了 Client 端暴露给 Server 的能力：

| 原语 | 说明 |
|------|------|
| **Sampling** | Server 请求 Client 让 LLM 完成推理（如 Server 需要模型判断下一步） |
| **Roots** | Client 向 Server 暴露可操作的文件系统根目录 |
| **Elicitation** (2025-06-18 新增) | Server 向用户请求额外信息（人机交互确认） |

---

## 4. 传输层

### 4.1 三种传输方式

| 传输方式 | 版本 | 通信方式 | 适用场景 |
|---------|------|---------|---------|
| **stdio** | 2024-11-05 起支持 | 标准输入/输出，Client 将 Server 作为子进程启动 | 本地开发、Claude Desktop 桌面集成 |
| **SSE (HTTP+SSE)** | 2024-11-05 引入，2025-03-26 起废弃 | Client POST 请求 + Server SSE 推送 | **已废弃**，仅作向后兼容 |
| **Streamable HTTP** | 2025-03-26 引入 | 单 HTTP 端点，POST 发请求，可选 SSE 流式推送 | 远程/云端部署、生产环境 |

### 4.2 stdio 传输

```
Client                    Server (子进程)
  │                          │
  ├── 启动 Server 子进程 ──→ │
  │                          │
  ├── stdin ──→ JSON-RPC ──→ │  (Client 写，Server 读)
  │                          │
  │ ←── JSON-RPC ←── stdout ─┤  (Server 写，Client 读)
  │                          │
  │ ←── 日志 ←── stderr ────┤  (Server 写日志)
```

**特点**：
- Client 启动 Server 作为子进程
- 消息以换行符分隔，必须是单行 JSON-RPC
- Server 的 stderr 可用于日志输出
- 同一台机器，零网络开销
- **每个 Client 独占一个 Server 进程**

### 4.3 SSE (已废弃)

原始远程传输方案：Client 通过 HTTP POST 发送请求，Server 通过 SSE (Server-Sent Events) 推送响应和通知。

**被废弃的原因**：
- 要求 Server 维护长连接，与 CDN/负载均衡器不兼容
- 无法支持无状态 Server
- 扩展性差

### 4.4 Streamable HTTP (当前推荐)

```
Client                          Server
  │                               │
  ├── POST /mcp ────────────────→ │  (发送 JSON-RPC 请求)
  │                               │
  │ ←── 200 OK + SSE stream ────┤  (可选：流式响应)
  │     data: {"jsonrpc":"2.0",...}
  │     data: {"jsonrpc":"2.0",...}
  │                               │
  │ ←── 200 OK + JSON ──────────┤  (简单响应，非流式)
  │                               │
  ├── GET /mcp ─────────────────→ │  (打开 SSE 监听通道)
  │ ←── SSE stream ─────────────┤  (Server 主动推送通知)
  │                               │
  ├── DELETE /mcp ──────────────→ │  (关闭会话)
```

**核心设计**：
- **单一端点**：Server 只需暴露一个 MCP 端点（如 `https://api.example.com/mcp`）
- **POST**：Client 发送请求，Server 可返回 JSON 或升级为 SSE 流
- **GET**（可选）：Client 打开 SSE 通道接收 Server 主动推送
- **DELETE**：Client 终止会话
- **会话管理**：通过 `Mcp-Session-Id` header 标识会话
- **断线重连**：通过 `Last-Event-ID` header 支持消息重传

**为什么选 Streamable HTTP 而非 WebSocket**：
- 与现有 HTTP 基础设施（CDN、API Gateway、负载均衡）天然兼容
- 支持无状态 Server 部署
- 流式能力可选，简单场景仅用请求-响应即可
- 安全性更好（TLS、CORS、OAuth 2.0）

### 4.5 传输方式选择决策

```
是否需要远程访问？
  ├── 否 → stdio（最简单）
  └── 是 → Streamable HTTP
            ├── 是否需向后兼容旧 Client？同时支持 SSE
            └── 否 → 仅 Streamable HTTP
```

| 场景 | 推荐传输 |
|------|---------|
| Claude Desktop 本地工具 | stdio |
| Cursor 本地开发 | stdio |
| 企业级远程 MCP Server | Streamable HTTP + OAuth 2.0 |
| Serverless 部署 | Streamable HTTP |
| 原型验证 | stdio（先跑通逻辑，再迁移到 HTTP） |

---

## 5. 工具注册与发现

### 5.1 服务端暴露工具

Server 在启动时注册工具，通过 `tools/list` 响应暴露给 Client：

```python
# Python SDK 示例
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("Payment Service")

@mcp.tool()
def get_payment_status(order_id: str, include_details: bool = False) -> str:
    """根据订单号查询支付状态"""
    # 业务逻辑
    return f"Order {order_id}: PAID"

@mcp.tool()
def create_refund(order_id: str, amount: float, reason: str) -> str:
    """创建退款申请"""
    # 业务逻辑
    return f"Refund created for {order_id}"
```

```typescript
// TypeScript SDK 示例
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";

const server = new McpServer({ name: "Payment Service", version: "1.0.0" });

server.tool("get_payment_status",
  { order_id: z.string().describe("订单号") },
  async ({ order_id }) => ({
    content: [{ type: "text", text: `Order ${order_id}: PAID` }]
  })
);
```

### 5.2 客户端发现和调用

Client 通过标准协议流程发现和调用工具：

```
1. 连接建立 → initialize 握手
2. tools/list → 获取所有可用工具及其 schema
3. 模型根据用户意图选择工具
4. tools/call → 执行工具
5. 将结果返回给模型继续推理
```

**动态更新**：Server 可发送 `notifications/tools/list_changed` 通知 Client 重新获取工具列表，实现运行时热更新。

### 5.3 能力协商

初始化时双方声明能力：

```json
// Client 声明
{
  "capabilities": {
    "tools": { "listChanged": true },
    "resources": { "subscribe": true },
    "prompts": { "listChanged": true },
    "sampling": {}
  }
}

// Server 声明
{
  "capabilities": {
    "tools": { "listChanged": true },
    "resources": { "subscribe": true, "listChanged": true },
    "prompts": { "listChanged": true }
  }
}
```

---

## 6. 实际接入

### 6.1 Claude Desktop 集成 MCP Server

**配置文件路径**：
- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

**配置方式**：

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/Users/me/projects"],
      "transport": "stdio"
    },
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_xxxxxxxxxxxx"
      }
    },
    "remote-postgres": {
      "type": "streamable-http",
      "url": "https://mcp.example.com/postgres"
    }
  }
}
```

**Claude Desktop 也支持 UI 方式**：Settings → Connectors → "+" 添加远程 MCP Server。

### 6.2 Cursor 集成 MCP

**项目级配置**：在项目根目录创建 `.cursor/mcp.json`：

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/project"]
    },
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_xxxxxxxxxxxx"
      }
    }
  }
}
```

**验证**：Settings → MCP tab，检查绿色状态指示器。

### 6.3 自建 MCP Server

#### Python SDK

```bash
# 安装
pip install mcp

# 或使用 uv 运行
uv run mcp-server
```

```python
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("My Payment Service")

@mcp.tool()
def query_order(order_id: str) -> str:
    """查询订单支付状态"""
    # 调用内部支付系统 API
    return f"Order {order_id}: SUCCESS, amount: 99.9 CNY"

@mcp.resource("config://payment/methods")
def get_payment_methods() -> str:
    """获取支持的支付方式列表"""
    return "WeChat Pay, Alipay, UnionPay"

@mcp.prompt()
def review_payment(code: str) -> str:
    """审查支付相关代码"""
    return f"请审查以下支付逻辑代码的安全性和正确性:\n{code}"

if __name__ == "__main__":
    mcp.run()  # 默认 stdio 传输
```

#### TypeScript SDK

```bash
npm install @modelcontextprotocol/sdk
```

```typescript
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

const server = new McpServer({
  name: "Payment Service",
  version: "1.0.0"
});

server.tool(
  "query_order",
  { order_id: z.string().describe("订单号") },
  async ({ order_id }) => ({
    content: [{
      type: "text",
      text: `Order ${order_id}: SUCCESS, amount: 99.9 CNY`
    }]
  })
);

const transport = new StdioServerTransport();
await server.connect(transport);
```

### 6.4 Go 语言实现 MCP Server

Go 生态目前有两个主要 SDK：

| SDK | 类型 | 仓库 | 特点 |
|-----|------|------|------|
| **go-sdk** (官方) | 官方 | `github.com/modelcontextprotocol/go-sdk` | 官方维护，与 Google 协作开发，完整实现 MCP 规范 |
| **mcp-go** (社区) | 社区 | `github.com/mark3labs/mcp-go` | 最流行的社区 SDK，API 更高层，先于官方发布 |

#### 使用官方 Go SDK

```go
package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// 创建 Server
	server := mcp.NewServer("Payment Service", "1.0.0", nil)

	// 注册 Tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_order",
		Description: "查询订单支付状态",
	}, func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
		orderID := params.Arguments["order_id"].(string)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Order %s: SUCCESS, amount: 99.9 CNY", orderID)},
			},
		}, nil
	})

	// 启动 stdio 传输
	server.Run(context.Background(), mcp.NewStdioTransport())
}
```

#### 使用 mcp-go (社区 SDK)

```go
package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	s := server.NewMCPServer(
		"Payment Service",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// 注册 Tool
	tool := mcp.NewTool("query_order",
		mcp.WithDescription("查询订单支付状态"),
		mcp.WithString("order_id",
			mcp.Required(),
			mcp.Description("订单号"),
		),
		mcp.WithBoolean("include_details",
			mcp.Description("是否包含详细信息"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		orderID := request.Params.Arguments["order_id"].(string)
		return mcp.NewToolResultText(fmt.Sprintf("Order %s: SUCCESS", orderID)), nil
	})

	// 启动 stdio 传输
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}
```

#### Streamable HTTP 传输 (Go)

```go
// 使用 mcp-go 的 Streamable HTTP 传输
handler := server.NewStreamableHTTPServer(s)
http.Handle("/mcp", handler)
log.Fatal(http.ListenAndServe(":8080", nil))
```

---

## 7. MCP 生态

### 7.1 热门 MCP Server 清单

截至 2026 年 5 月，MCP 生态已有 14,000+ 索引 Server。以下按类别列出最常用的：

| 类别 | Server | 维护方 | 功能 |
|------|--------|--------|------|
| **文件系统** | `@modelcontextprotocol/server-filesystem` | Anthropic | 读写本地文件，限制在指定目录 |
| **Git** | `@modelcontextprotocol/server-git` | Anthropic | Git 操作：log、diff、blame 等 |
| **GitHub** | `github/github-mcp-server` | GitHub | Issues、PR、代码搜索 |
| **数据库** | `postgres-mcp` | Crystal DBA | PostgreSQL 查询和 schema 读取 |
| **数据库** | `mcp-redis` | Redis | Redis 操作 |
| **数据库** | `mcp-clickhouse` | ClickHouse | ClickHouse 查询 |
| **搜索** | `@modelcontextprotocol/server-brave-search` | Anthropic | Brave 网页搜索 |
| **浏览器** | `playwright-mcp` | Microsoft | 浏览器自动化、E2E 测试 |
| **通信** | `@modelcontextprotocol/server-slack` | Anthropic | Slack 消息读写 |
| **知识** | `context7` | Context7 | 实时库文档查询 |
| **DevOps** | `mcp-server-kubernetes` | Community | K8s 集群管理 |
| **DevOps** | `mcp-server-terraform` | HashiCorp | Terraform 基础设施管理 |
| **可观测** | `mcp-server-sentry` | Sentry | 错误追踪和问题分诊 |
| **项目管理** | `mcp-server-linear` | Linear | 项目和任务管理 |
| **邮件** | `mcp-server-gmail` | Community | Gmail 邮件操作 |

### 7.2 生态查找渠道

| 渠道 | 地址 |
|------|------|
| 官方参考实现 | `modelcontextprotocol.io/examples` |
| 社区精选列表 | `github.com/punkpeye/awesome-mcp-servers` |
| 可搜索市场 | `glama.ai/mcp/servers` |
| Docker MCP 目录 | Docker 官方 MCP Catalog |

---

## 8. MCP 与其他协议对比

### 8.1 MCP vs OpenAPI

| 维度 | MCP | OpenAPI |
|------|-----|---------|
| 定位 | AI 应用与工具的通信协议 | REST API 的描述规范 |
| 核心关注 | 上下文提供 + 工具调用 + 动态发现 | API 接口文档 + 代码生成 |
| 消息格式 | JSON-RPC 2.0 | HTTP REST (通常 JSON) |
| 发现机制 | 运行时 `tools/list` 动态发现 | 静态 spec 文件，设计时定义 |
| 上下文类型 | Tools + Resources + Prompts 三种原语 | 仅 HTTP 端点描述 |
| 状态管理 | 有状态连接，支持会话和通知 | 无状态请求-响应 |
| 安全模型 | OAuth 2.0 + 用户审批流 | OAuth 2.0 / API Key |

**关系**：MCP 可将 OpenAPI spec 作为 Resource 暴露给模型，让模型了解现有 API。OpenAPI 定义"API 长什么样"，MCP 定义"AI 如何发现和使用工具"。

### 8.2 MCP vs LSP (Language Server Protocol)

| 维度 | MCP | LSP |
|------|-----|-----|
| 定位 | AI 与工具的通信协议 | 编辑器与语言服务的通信协议 |
| 相似点 | JSON-RPC 消息格式、初始化握手、能力协商、请求/响应/通知模式 | 同左 |
| 核心差异 | 3 种原语（Tools/Resources/Prompts）、支持 Sampling/Elicitation | 专注于代码智能（补全、跳转、诊断） |
| 传输 | stdio / Streamable HTTP | stdio / WebSocket / socket |
| 典型场景 | AI Agent 调用任意外部工具 | IDE 获取代码补全和跳转 |

**面试要点**：MCP 借鉴了 LSP 的设计模式（JSON-RPC、初始化握手、能力协商），但解决的是不同层面的问题。LSP 解决的是"编辑器如何获取语言智能"，MCP 解决的是"AI 如何获取上下文和调用工具"。有人称之为 "MCP 是 LLM 的 LSP"。

### 8.3 MCP vs Function Calling (补充)

```
层次关系：

┌──────────────────────────────┐
│  应用层：AI Agent/Host       │  ← 用户交互
├──────────────────────────────┤
│  协议层：MCP                 │  ← 工具发现、连接、通信
├──────────────────────────────┤
│  能力层：Function Calling    │  ← 模型决策何时调用哪个函数
├──────────────────────────────┤
│  执行层：具体工具/API        │  ← 实际操作
└──────────────────────────────┘
```

MCP 不是替代 Function Calling，而是在其之上提供标准化的基础设施层。Function Calling 是"模型能力"，MCP 是"协议标准"。

---

## 9. 面试高频题

### Q1: MCP 是什么？解决什么问题？

**答**：MCP (Model Context Protocol) 是一个开放标准协议，标准化 LLM 应用与外部数据源/工具的连接方式。它解决的是 N×M 集成问题：以前 N 个工具 × M 个客户端需要 N×M 个定制集成，MCP 将其降为 N+M — 工具方实现一个 MCP Server，客户端实现一个 MCP Client，即可互通。类比 USB-C：一个标准连接器替代所有专用接口。

### Q2: MCP 的三层架构是什么？各自职责？

**答**：Host → Client → Server 三层。Host 是宿主应用（如 Claude Desktop），管理多个 Client 实例；Client 是协议连接器，与 Server 建立 1:1 连接，负责初始化、能力协商、消息路由；Server 提供具体的上下文和工具，暴露 Tools/Resources/Prompts 给 Client。一个 Host 可管理多个 Client，一个 Client 对应一个 Server。

### Q3: MCP 的三种原语是什么？区别在哪？

**答**：Tools（模型控制）— 模型决定何时调用的操作函数，如查询支付状态；Resources（应用控制）— 应用决定何时读取的上下文数据，如数据库 schema；Prompts（用户控制）— 用户选择触发的预定义模板。核心区别在于**控制方不同**：谁决定何时使用该原语。

### Q4: MCP 和 Function Calling 有什么区别？能替代吗？

**答**：不能替代，是互补关系。Function Calling 是 LLM 的内置能力，解决"模型何时调用哪个函数"的决策问题，工具 schema 嵌入 API 请求，执行在应用内，绑定特定提供商。MCP 是开放协议，解决"工具如何被标准化发现、托管和消费"的基础设施问题，Client-Server 架构，跨提供商，运行时动态发现。实际生产中常结合使用：Function Calling 做决策，MCP 做工具基础设施。

### Q5: stdio 和 Streamable HTTP 传输怎么选？

**答**：本地场景选 stdio — Client 将 Server 作为子进程启动，零网络开销，配置最简单，Claude Desktop 本地工具默认用 stdio。远程场景选 Streamable HTTP — 支持多客户端、与 CDN/负载均衡兼容、支持 OAuth 认证、适合生产部署。SSE 传输已废弃，仅作向后兼容。建议先 stdio 跑通逻辑，再迁移到 Streamable HTTP。

### Q6: MCP 如何保证安全？

**答**：多层安全机制：(1) OAuth 2.0 + PKCE 强制用于远程 Server；(2) 最小权限 scope 授权；(3) 凭证隔离在 Server 端，Client 无法直接获取；(4) 用户审批流 — 工具调用需用户确认；(5) TLS 加密传输；(6) Server 运行在沙箱中，限制文件系统和网络访问；(7) 资源 URI 限制在允许目录内。OWASP 已发布 MCP 安全备忘单。

### Q7: 用 Go 如何实现一个 MCP Server？

**答**：两种选择：(1) 官方 SDK `github.com/modelcontextprotocol/go-sdk/mcp` — 官方维护，完整规范支持；(2) 社区 SDK `github.com/mark3labs/mcp-go` — 更流行的社区方案，高层 API，支持 stdio/SSE/Streamable HTTP。核心步骤：创建 Server 实例 → 注册 Tool/Resource/Prompt → 选择 Transport 启动。Go 适合构建高性能、低内存的 MCP Server，特别是对接内部系统时。

### Q8: MCP 的能力协商机制是怎样的？

**答**：在 initialize 握手阶段，Client 和 Server 各自声明 capabilities 对象，明确支持哪些原语（tools/resources/prompts）和功能（listChanged/subscribe/sampling）。协议版本也在此协商（如 "2025-06-18"）。如双方版本不兼容则终止连接。这确保了通信高效 — 双方不会发起对方不支持的操作。

### Q9: MCP Server 如何动态更新工具列表？

**答**：Server 在工具列表变更时，向 Client 发送 `notifications/tools/list_changed` 通知。Client 收到后重新调用 `tools/list` 获取最新列表。这需要在 initialize 时声明 `"tools": { "listChanged": true }` 能力。同理，Resources 和 Prompts 也有对应的 `list_changed` 通知。

### Q10: Streamable HTTP 传输相比旧的 SSE 传输有哪些改进？

**答**：(1) 支持无状态 Server — 不需要维护长连接；(2) 与 CDN/API Gateway/负载均衡兼容；(3) 单一端点设计（POST/GET/DELETE），SSE 需要两个端点；(4) 通过 `Mcp-Session-Id` 和 `Last-Event-ID` 支持会话管理和断线重连；(5) SSE 是可选升级而非必需，简单场景仅用请求-响应；(6) 安全性更好，标准 HTTP 安全模式适用。

### Q11: MCP 与 LSP 的关系是什么？

**答**：MCP 借鉴了 LSP 的设计模式 — 都用 JSON-RPC 消息格式、都有初始化握手和能力协商、都有请求/响应/通知三种消息类型。但解决不同问题：LSP 解决"编辑器如何获取语言智能（补全/跳转/诊断）"，MCP 解决"AI 如何获取上下文和调用工具"。MCP 的原语更通用（Tools/Resources/Prompts vs LSP 的 textDocument/completion 等）。有人说 "MCP 是 LLM 的 LSP"。

### Q12: 在支付业务中，MCP 可以怎么用？

**答**：典型场景：(1) 构建内部支付系统 MCP Server，暴露查询订单、发起退款、查对账结果等 Tool，让 AI Agent 通过自然语言操作支付系统；(2) 将支付系统数据库 schema 作为 Resource 暴露，让模型理解数据结构；(3) 将常见风控审查流程定义为 Prompt 模板；(4) 构建 AI 运维助手，通过 MCP 连接支付系统、监控系统（Sentry/Prometheus）、通知系统（Slack），实现异常自动分诊和响应。

---

## 附录：MCP 规范版本演进

| 版本 | 日期 | 关键变化 |
|------|------|---------|
| 2024-11-05 | 2024.11 | 初始版本，定义 stdio + SSE 传输，三种原语 |
| 2025-03-26 | 2025.03 | 引入 Streamable HTTP 替代 SSE；SSE 标记废弃 |
| 2025-06-18 | 2025.06 | 结构化 Tool 输出、Elicitation 原语、Resource Links、OAuth 2.1 资源服务器分类、Progress message 字段 |
| 2025-11-25 | 2025.11 | OIDC 发现、Task 实验性支持、Tool 图标、Sampling 中支持 Tool 调用、JSON Schema 2020-12 |

## 参考资源

- [MCP 官方文档](https://modelcontextprotocol.io/)
- [MCP 规范 (2025-06-18)](https://modelcontextprotocol.io/specification/2025-06-18)
- [官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [社区 Go SDK (mcp-go)](https://github.com/mark3labs/mcp-go)
- [官方 Python SDK](https://github.com/modelcontextprotocol/python-sdk)
- [官方 TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- [OWASP MCP Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/MCP_Security_Cheat_Sheet.html)
