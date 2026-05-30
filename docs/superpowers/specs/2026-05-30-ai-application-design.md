# AI Application Development Content Design

## Overview

Add a new directory `14-ai-application/` to the interview preparation repository, covering LLM application development full-stack topics with deep Markdown notes following the existing `XX-topic.md` convention.

## Directory Structure

```
14-ai-application/
  01-prompt-engineering.md
  02-rag-vector-database.md
  03-agent-framework.md
  04-mcp-protocol.md
  05-llm-api-framework.md
  06-agent-ecosystem.md
```

## File Content

### 01-prompt-engineering.md
- Prompt design principles: clarity, specificity, role assignment
- Basic patterns: Zero-shot / Few-shot / Chain-of-Thought / Tree-of-Thought
- Advanced patterns: Self-Consistency, ReAct Prompting, Directional Stimulus
- Structured output: JSON Mode, Function Calling, Structured Output
- Prompt optimization: iterative process, A/B testing, auto-optimization (DSPy)
- Prompt caching and cost optimization
- Interview high-frequency questions

### 02-rag-vector-database.md
- RAG architecture: Indexing -> Retrieval -> Generation
- Document processing: Chunking strategies (fixed-size / semantic / recursive)
- Embedding models: OpenAI / BGE / Cohere
- Vector database comparison: Milvus / Pinecone / Weaviate / Qdrant / Chroma
- Retrieval strategies: dense, sparse (BM25), hybrid, multi-route recall
- ReRank: Cross-Encoder / Cohere Rerank / BGE-Reranker
- Advanced RAG: Query Rewrite, HyDE, Self-RAG, CRAG, GraphRAG
- Evaluation: RAGAS framework
- Interview high-frequency questions

### 03-agent-framework.md
- Agent core concepts: Perception -> Reasoning -> Action -> Memory
- ReAct pattern: Reasoning + Acting interleaved
- Tool Use / Function Calling mechanism
- Planning strategies: Plan-and-Execute, Tree of Planning, Reflexion
- Memory mechanisms: short-term/long-term, vector memory, conversation summary
- Multi-Agent architecture: collaboration patterns, communication, task assignment
- Agent framework comparison: LangGraph / CrewAI / AutoGen / OpenAI Agents SDK
- Agent reliability: hallucination, tool call error handling, guardrails
- Interview high-frequency questions

### 04-mcp-protocol.md
- What is MCP: Model Context Protocol positioning and goals
- Core architecture: Host -> Client -> Server three-layer model
- Primitives: Tools / Resources / Prompts
- Transport layer: stdio / SSE / Streamable HTTP
- Tool registration and discovery
- Practical integration: Claude Desktop / Cursor / custom MCP Server
- MCP ecosystem: existing server inventory
- Interview high-frequency questions

### 05-llm-api-framework.md
- OpenAI API: Chat Completions, Assistants, Streaming, Function Calling
- Claude API: Messages API, Tool Use, Streaming, Prompt Caching
- Framework comparison: LangChain vs LlamaIndex vs Haystack vs DSPy
- LangChain core: Chain / Agent / Tool / Memory / Callback
- LlamaIndex core: Index / Query Engine / Node / Retriever
- Streaming output: SSE / WebSocket, implementation patterns
- Token billing and cost control
- Model gateway: LiteLLM / One API / OpenRouter
- Interview high-frequency questions

### 06-agent-ecosystem.md
- Development trends: Chatbot -> Copilot -> Agent -> Multi-Agent
- Enterprise projects: Cursor / Devin / Sierra / ByteDance Coze / Baidu AgentBuilder
- Open-source projects deep analysis:
  - AutoGPT: autonomous agent pioneer
  - CrewAI: multi-agent collaboration framework
  - OpenHands: AI software development agent
  - MetaGPT: multi-agent simulating a software company
  - Dify: LLM application development platform
  - Agno: lightweight agent framework
- Agent development ideas:
  - Design process from requirements to agent architecture
  - Tool design principles
  - Reliability and observability
  - Production deployment considerations
- Interview high-frequency questions

## Format

- Each file is a deep Markdown note following existing `XX-topic.md` convention
- Content includes theory, architecture diagrams (text-based), comparisons, and interview Q&A
- No demo code files needed for this directory (content is conceptual/architectural)
- Each file ends with interview high-frequency questions section

## Design Decisions

- Single directory `14-ai-application/` follows existing 00-13 numbering scheme
- 6 files cover all requested topics without over-splitting
- Agent-related content gets the most coverage (files 03, 04, 06) per user emphasis on Agent ecosystem
- MCP gets its own file due to growing importance and depth of content
