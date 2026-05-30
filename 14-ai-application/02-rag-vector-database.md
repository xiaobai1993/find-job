# RAG 架构与向量数据库 — 后端面试核心考点

来源：RAG 论文 + 生产实践 + 面试高频题整理

---

## 一、RAG 架构概览

RAG（Retrieval-Augmented Generation）的核心思想：**用检索补充 LLM 的知识，而不是把所有知识塞进模型参数里**。

### 1.1 三阶段详解

```
┌─────────────────────────────────────────────────────────────────┐
│                     RAG 完整流程                                 │
│                                                                 │
│  ┌──────────┐     ┌──────────────┐     ┌──────────────────┐    │
│  │ Indexing  │ ──> │  Retrieval   │ ──> │   Generation     │    │
│  │ 离线准备  │     │  在线检索     │     │   生成回答        │    │
│  └──────────┘     └──────────────┘     └──────────────────┘    │
│                                                                 │
│  文档加载          Query 向量化       Context + Prompt → LLM     │
│  Chunking          向量相似度搜索      生成最终回答               │
│  Embedding         结果排序/过滤                                 │
│  写入向量库                                                     │
└─────────────────────────────────────────────────────────────────┘
```

**Indexing 阶段（离线）**：

1. 文档加载：PDF/HTML/Markdown/数据库 → 纯文本
2. Chunking：长文档切分为语义完整的小片段
3. Embedding：每个 chunk 通过 Embedding 模型转为向量
4. 存储：向量 + 原文写入向量数据库，建立索引

**Retrieval 阶段（在线）**：

1. 用户 Query → 同一个 Embedding 模型向量化
2. 在向量数据库中做相似度搜索（ANN），取 Top-K
3. 可选：对召回结果做 ReRank 精排
4. 可选：混合检索（稠密 + 稀疏）

**Generation 阶段（在线）**：

1. 将检索到的 Context 拼接到 Prompt 模板
2. 发送给 LLM 生成回答
3. 可选：引用溯源（标注答案来自哪个文档片段）

### 1.2 RAG vs Fine-tuning

| 维度 | RAG | Fine-tuning |
|------|-----|-------------|
| 知识更新 | 实时，改文档即可 | 需重新训练，成本高 |
| 成本 | 向量库 + 推理费用 | 训练费用高，需 GPU |
| 幻觉控制 | 有 Context 约束，幻觉更少 | 仍可能幻觉 |
| 适用场景 | 知识密集型 QA、文档查询 | 风格适配、领域术语学习 |
| 数据隐私 | 文档可留在本地 | 训练数据需上传 |
| 可解释性 | 可溯源到原文 | 难以解释知识来源 |
| 长尾知识 | 好（直接检索） | 差（训练数据难覆盖） |

**面试答法**：

> RAG 和 Fine-tuning 不是互斥的。RAG 解决「知识注入」问题——模型不知道什么，检索补什么；Fine-tuning 解决「行为适配」问题——让模型学会某种输出格式或风格。实际生产中经常两者结合：先 Fine-tune 让模型适配业务风格，再用 RAG 注入实时知识。

---

## 二、文档处理：Chunking 策略

Chunking 是 RAG 效果的基础——**切得不好，检索再强也没用**。

### 2.1 切分策略对比

| 策略 | 原理 | 优点 | 缺点 | 适用场景 |
|------|------|------|------|----------|
| 固定大小切分 | 按 token/字符数切，带 overlap | 简单、可预测 | 可能切断语义 | 通用 baseline |
| 语义切分 | 用 Embedding 计算相邻句子相似度，低于阈值则切 | 语义完整性好 | 计算开销大，阈值难调 | 高精度要求场景 |
| 递归切分 | 按分隔符层级递归切（\n\n → \n → 句 → 词） | 兼顾结构和大小 | 依赖文档格式 | 通用推荐方案 |
| 文档结构切分 | 按标题/段落/章节等文档结构切 | 语义最完整 | 结构不规范的文档不适用 | Markdown/HTML 等结构化文档 |

### 2.2 固定大小切分 + Overlap

最常用的 baseline 方案，关键参数是 `chunk_size` 和 `chunk_overlap`：

```python
from langchain.text_splitter import RecursiveCharacterTextSplitter

splitter = RecursiveCharacterTextSplitter(
    chunk_size=512,       # 每个 chunk 最大 token 数
    chunk_overlap=64,     # 相邻 chunk 重叠部分
    separators=["\n\n", "\n", "。", ".", " ", ""],  # 分隔符优先级
)
chunks = splitter.split_text(document)
```

**为什么需要 Overlap？**

> 固定切分会在任意位置断开，一个完整的语义单元可能被拆到两个 chunk。Overlap 让相邻 chunk 有重叠，检索时至少有一个 chunk 包含完整上下文。典型 overlap 是 chunk_size 的 10%~20%。

### 2.3 Chunk Size 的选择

| Chunk Size | 优势 | 劣势 |
|------------|------|------|
| 小（128-256） | 检索精度高，噪声少 | 上下文不完整，可能缺关键信息 |
| 中（512-1024） | 平衡精度和上下文 | 通用推荐 |
| 大（1024-2048） | 上下文完整 | 检索噪声多，可能超出 LLM 窗口 |

**实践经验**：

- 法律/医疗等严谨领域：小 chunk + 元数据过滤
- 通用知识库：512-1024 平衡方案
- 代码库：按函数/类切分，不用固定大小

### 2.4 元数据（Metadata）的重要性

每个 chunk 除了文本和向量，还应附加元数据：

```json
{
  "content": "退款流程需要3-5个工作日...",
  "source": "refund-policy-v2.pdf",
  "page": 12,
  "section": "退款时效",
  "doc_type": "policy",
  "created_at": "2025-01-15"
}
```

元数据用于**检索前过滤**（pre-filtering）：比如只查最近更新的文档、只查特定类型，大幅提升精度。

---

## 三、Embedding 模型

Embedding 是 RAG 的"翻译层"——把文本变成向量，质量直接决定检索效果的上限。

### 3.1 主流模型对比

| 模型 | 维度 | MTEB 排名 | 特点 | 价格 |
|------|------|-----------|------|------|
| OpenAI text-embedding-3-small | 1536 | 中上 | 稳定、易用、支持维度截断 | $0.02/1M tokens |
| OpenAI text-embedding-3-large | 3072 | 高 | 精度更高，可截断到 256/1024 | $0.13/1M tokens |
| BGE-M3 (BAAI) | 1024 | 高 | 开源、多语言、多粒度 | 自部署免费 |
| BGE-large-zh-v1.5 | 1024 | 中文优秀 | 中文场景首选开源模型 | 自部署免费 |
| Cohere embed-v3 | 1024 | 高 | 支持搜索类型参数（query/doc） | $0.10/1M tokens |
| GTE-Qwen2 (Alibaba) | 1536 | 高 | 最新开源 SOTA 之一 | 自部署免费 |

### 3.2 模型选择考量

**选型维度**：

1. **语言**：纯中文 → BGE-large-zh / GTE-Qwen2；中英混合 → BGE-M3 / text-embedding-3
2. **部署方式**：要私有化 → 开源模型（BGE/GTE）；接受 API → OpenAI / Cohere
3. **成本**：大规模数据 → 自部署开源模型更划算
4. **精度要求**：高精度 → 大模型 + 高维度；快速迭代 → API 调用

**关键细节：Query vs Document Embedding**

有些模型对 query 和 document 使用不同的前缀：

```python
# BGE 系列需要加指令前缀
query_embedding = model.encode("query: 如何退款")        # 查询加前缀
doc_embedding = model.encode("退款流程如下...")           # 文档不加前缀

# Cohere embed-v3 通过 input_type 参数区分
query_embedding = cohere.embed(texts=["如何退款"], input_type="search_query")
doc_embedding = cohere.embed(texts=["退款流程如下..."], input_type="search_document")
```

> 不加前缀会显著降低检索精度——因为 query 短、信息密度高，document 长、信息分散，模型需要在训练时区分这两种输入模式。

### 3.3 维度截断

OpenAI text-embedding-3 支持**动态维度**：3072 维的向量可截断到 256/1024 等，只需在 API 参数中指定。

```python
# 截断到 1024 维，省存储省计算
response = client.embeddings.create(
    model="text-embedding-3-large",
    input=text,
    dimensions=1024
)
```

**权衡**：维度降低 → 存储和检索成本降低，但语义信息有损。实际测试中 1024 维通常够用，256 维在简单任务上也可接受。

---

## 四、向量数据库对比

向量数据库是 RAG 的存储和检索引擎，选型直接影响系统性能和运维复杂度。

### 4.1 主流方案对比

| 维度 | Milvus | Pinecone | Weaviate | Qdrant | Chroma |
|------|--------|----------|----------|--------|--------|
| 开源 | 是（Apache 2.0） | 否 | 是（BSD） | 是（Apache 2.0） | 是（Apache 2.0） |
| 托管服务 | Zilliz Cloud | 原生 SaaS | Weaviate Cloud | Qdrant Cloud | 无 |
| 索引类型 | IVF_FLAT/IVF_PQ/HNSW/SCANN | 闭源（自动选择） | HNSW | HNSW | HNSW |
| 元数据过滤 | 支持，丰富 | 支持 | 支持，GraphQL 风格 | 支持，Payload 过滤 | 有限支持 |
| 分布式 | 原生分布式 | 自动扩缩容 | 支持集群 | 支持分片 | 单机为主 |
| 规模 | 十亿级 | 十亿级 | 亿级 | 亿级 | 百万级 |
| 语言 SDK | Go/Python/Java/Node | Python/Node/Go | Python/Go/Java | Python/Go/Rust | Python/JS |
| 适用场景 | 大规模生产、Go 技术栈 | 快速上线、不想运维 | 语义搜索+结构化过滤 | 高性能、Rust 底层 | 原型验证、小规模 |

### 4.2 核心概念

```
Collection (类似数据库表)
  ├── Vector Field: 存储向量 (dim=1024)
  ├── Scalar Field: 存储元数据 (source, page, doc_type...)
  └── Index: 索引类型 (HNSW / IVF_FLAT / IVF_PQ)
```

**索引类型选择**：

| 索引 | 原理 | 召回率 | 速度 | 内存 | 适用规模 |
|------|------|--------|------|------|----------|
| FLAT | 暴力搜索 | 100% | 慢 | 高 | <10万，需精确结果 |
| IVF_FLAT | 聚类后搜部分 | 95%+ | 中 | 中 | 百万级 |
| IVF_PQ | 聚类+乘积量化压缩 | 90%+ | 快 | 低 | 亿级，内存敏感 |
| HNSW | 图索引，近似最近邻 | 98%+ | 很快 | 高 | 百万~亿级，实时性要求高 |

### 4.3 Go 后端开发者的选型建议

**面试答法**：

> 选向量数据库要看四个维度：规模、运维成本、过滤需求、技术栈匹配。如果是 Go 技术栈 + 大规模生产，Milvus 是首选——原生分布式、Go SDK 成熟、Zilliz 有托管。快速验证用 Chroma，不想运维用 Pinecone。如果需要复杂的结构化+向量混合查询，Weaviate 的 GraphQL 接口比较方便。

### 4.4 Milvus Go SDK 示例

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/milvus-io/milvus-sdk-go/v2/client"
    "github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func main() {
    ctx := context.Background()

    // 连接 Milvus
    c, err := client.NewClient(ctx, client.Config{
        Address: "localhost:19530",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    // 创建 Collection
    schema := entity.NewSchema().WithDynamicFieldEnabled(true).
        WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
        WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(1024)).
        WithField(entity.NewField().WithName("content").WithDataType(entity.FieldTypeVarChar).WithMaxLength(2048)).
        WithField(entity.NewField().WithName("source").WithDataType(entity.FieldTypeVarChar).WithMaxLength(256))

    err = c.CreateCollection(ctx, entity.NewCollectionSchema(schema),
        entity.DefaultConsistencyLevel)
    if err != nil {
        log.Fatal(err)
    }

    // 创建 HNSW 索引
    idx := entity.NewIndexHNSW(entity.L2, 16, 256) // M=16, efConstruction=256
    err = c.CreateIndex(ctx, "rag_docs", "vector", idx, false)
    if err != nil {
        log.Fatal(err)
    }

    // 搜索
    sp := entity.NewIndexHNSWSearchParam(64) // ef=64
    results, err := c.Search(ctx, "rag_docs", []string{}, "source == 'policy'",
        []string{"content", "source"}, []entity.Vector{queryVec},
        "vector", entity.L2, 5, sp)
    if err != nil {
        log.Fatal(err)
    }

    for _, result := range results {
        for j := 0; j < result.ResultCount; j++ {
            fmt.Printf("Score: %f, Content: %s\n",
                result.Scores[j], result.Fields.GetColumn("content").(*entity.ColumnVarChar).Data()[j])
        }
    }
}
```

---

## 五、检索策略

检索是 RAG 效果的核心——**检索不到，生成再好也没用**。

### 5.1 稠密检索（Dense Retrieval）

原理：Query 和 Document 都通过 Embedding 模型转为向量，用余弦相似度 / L2 距离计算相似性。

```
Query → Embedding → Vector → ─┐
                               ├→ Cosine Similarity → Top-K
Doc Chunks → Embedding → Vectors ─┘
```

**优点**：能捕捉语义相似性（"如何退款" 和 "退货流程" 语义接近但字面不同）
**缺点**：对精确关键词匹配不敏感（搜 "HNSW" 可能召回 "近似最近邻" 但遗漏含 "HNSW" 的文档）

### 5.2 稀疏检索（Sparse Retrieval / BM25）

原理：基于词频统计的检索，BM25 是经典算法。

```python
# BM25 打分公式
score(D, Q) = Σ IDF(qi) · (f(qi, D) · (k1 + 1)) / (f(qi, D) + k1 · (1 - b + b · |D| / avgdl))
```

- `f(qi, D)`：词 qi 在文档 D 中的频率
- `IDF(qi)`：逆文档频率，越稀有的词权重越高
- `b`：文档长度归一化参数（通常 0.75）
- `k1`：词频饱和参数（通常 1.2-2.0）

**优点**：精确关键词匹配好（搜 "HNSW" 一定召回含 "HNSW" 的文档），不需要 GPU
**缺点**：无法理解语义（"退款" 和 "退货" 被视为无关词）

### 5.3 混合检索（Hybrid Search）

核心思想：**稠密检索捕捉语义，稀疏检索捕捉精确匹配，两者互补**。

```
          ┌─ Dense Retrieval → Top-K₁ ─┐
Query ────┤                             ├→ Merge (RRF / 加权) → Final Top-K
          └─ Sparse Retrieval → Top-K₂ ─┘
```

**结果融合方法：Reciprocal Rank Fusion (RRF)**

```python
# RRF 公式：将排序位置转为分数，避免不同检索方法的分数不可比
def rrf_merge(ranks_list, k=60):
    """
    ranks_list: 多路检索结果，每路是一个按相似度排序的 doc_id 列表
    k: 平滑常数，通常取 60
    """
    scores = {}
    for ranks in ranks_list:
        for position, doc_id in enumerate(ranks):
            scores[doc_id] = scores.get(doc_id, 0) + 1.0 / (k + position + 1)
    return sorted(scores.items(), key=lambda x: -x[1])
```

**为什么 RRF 而不是直接加权分数？**

> 稠密检索返回余弦相似度（-1~1），稀疏检索返回 BM25 分数（0~无穷），两种分数不可直接比较。RRF 用排序位置而非原始分数，天然解决了分数不可比的问题。

### 5.4 多路召回

在混合检索基础上，还可以增加更多召回通道：

| 召回通道 | 方法 | 解决的问题 |
|----------|------|-----------|
| 向量召回 | Dense Embedding | 语义相似 |
| 关键词召回 | BM25 / SPLADE | 精确匹配 |
| 知识图谱召回 | Graph Traversal | 关系推理 |
| 全文检索 | Elasticsearch | 长文本匹配 |
| 规则召回 | 业务规则 | 强制匹配（如 FAQ 精确命中） |

**面试答法**：

> 单一检索方法总有力所不及的地方。稠密检索擅长语义理解但不精确，BM25 精确但不懂语义。混合检索用 RRF 融合两者，是当前生产环境的标准做法。更进一步，对特定业务可以加知识图谱召回做关系推理，或者加规则召回保证 FAQ 必中。

---

## 六、ReRank：检索结果的二次排序

### 6.1 为什么需要 ReRank？

**问题**：Embedding 模型（Bi-Encoder）为了效率，Query 和 Document **独立编码**，无法捕捉 Query-Document 之间的深层交互。

```
Bi-Encoder (检索阶段):
  Query → Encoder → Vec_Q ─┐
                            ├→ cosine(Vec_Q, Vec_D) → 粗排分数
  Doc   → Encoder → Vec_D ─┘
  特点：快（向量可预计算），但交互浅

Cross-Encoder (ReRank 阶段):
  [Query; Doc] → Encoder → score
  特点：慢（每对都要过模型），但交互深
```

**类比**：Bi-Encoder 像是"看照片选人"——快但粗略；Cross-Encoder 像是"面试详谈"——慢但精准。

### 6.2 ReRank 模型对比

| 模型 | 特点 | 性能 | 延迟 |
|------|------|------|------|
| BGE-Reranker-v2-m3 | 开源、多语言、通用性强 | 高 | ~50ms/pair |
| Cohere Rerank | API 调用、效果顶尖 | 很高 | ~100ms/batch |
| bce-reranker-base_v1 | 中文优化、轻量 | 中上 | ~30ms/pair |
| Cross-Encoder ms-marco-MiniLM | 经典 baseline | 中 | ~20ms/pair |

### 6.3 生产实践

```python
# 典型 RAG 检索 + ReRank 流程
def retrieve_and_rerank(query, top_k=20, rerank_top_n=5):
    # Step 1: 混合检索，多召回一些候选
    dense_results = vector_search(query, top_k=top_k)
    sparse_results = bm25_search(query, top_k=top_k)
    candidates = rrf_merge([dense_results, sparse_results])

    # Step 2: ReRank 精排
    reranked = reranker.rank(
        query=query,
        documents=[c.content for c in candidates],
        top_n=rerank_top_n
    )

    return reranked
```

**关键参数**：检索阶段 Top-K 取 20-50，ReRank 后取 Top-5 送入 LLM。多召回少送入——让 ReRank 做精选。

---

## 七、高级 RAG 技术

Naive RAG（基础三阶段）面临的问题：**检索不相关、生成幻觉、无法判断知识边界**。高级 RAG 从各个阶段优化。

### 7.1 Query Rewrite（查询改写）

**问题**：用户 Query 经常模糊、口语化、或包含代词指代。

```python
# 多轮对话中的 Query Rewrite
User: "退款怎么操作？"
LLM: "退款流程是..."
User: "那个时限是多久？"  # "那个" 指代不明

# → Rewrite: "退款的时限是多久？"
```

**方法**：
- LLM-based Rewrite：让 LLM 把模糊 Query 改写为清晰 Query
- Multi-Query：生成多个改写版本，分别检索后合并
- Step-back Prompting：让 LLM 退一步生成更宏观的 Query

```
原始 Query: "2024年GPT-4的API价格是多少？"
Step-back:  "OpenAI GPT系列模型的定价历史是什么？"
```

> Step-back Query 更宏观，召回的上下文更完整，不容易因为具体日期变化而检索失败。

### 7.2 HyDE（Hypothetical Document Embedding）

**核心思想**：先让 LLM 生成一个"假设性答案"，再用这个答案去做检索。

```
Query: "如何配置 Kubernetes HPA？"
  ↓ LLM 生成假设答案
HyDE: "Kubernetes HPA 可以通过 kubectl autoscale 命令或 YAML 配置，
       需要设置 minReplicas、maxReplicas 和 metrics..."
  ↓ 用 HyDE 文本做 Embedding 检索
  → 检索到真实文档（比直接用 Query 检索更准确）
```

**为什么有效？**

> Query 通常短且信息不足，假设答案和真实文档在向量空间中更接近（都是完整陈述）。但风险是 LLM 幻觉的假设答案可能偏离方向，所以 HyDE 适合 LLM 有一定领域知识的场景。

### 7.3 Self-RAG

**核心思想**：让 LLM 自己判断"需不需要检索"和"检索结果有没有用"。

```
输入 Query
  ↓
[1] LLM 判断：需要检索吗？
  ├── 不需要 → 直接生成
  └── 需要 → 检索
        ↓
[2] LLM 判断：检索结果相关吗？
  ├── 相关 → 用检索结果生成
  └── 不相关 → 重新检索 或 放弃
        ↓
[3] LLM 判断：生成结果有支持吗？
  ├── 有支持 → 输出
  └── 无支持 → 标记为"无依据"
```

**三个关键 token**：

| Token | 含义 | 作用 |
|-------|------|------|
| `Retrieve` | 是否需要检索 | 避免不必要的检索开销 |
| `ISREL` | 检索结果是否相关 | 过滤无关上下文 |
| `ISSUP` | 生成内容是否有检索支撑 | 减少幻觉 |

**改进点**：Naive RAG 无论什么问题都检索，无论检索什么都塞进 prompt。Self-RAG 引入自我反思机制，按需检索、按需使用。

### 7.4 CRAG（Corrective RAG）

**核心思想**：对检索结果做质量评估，不靠谱时走纠正路径。

```
检索结果 → 评估器（可以是 LLM 或小模型）
  ├── 相关度高 → 正常使用
  ├── 相关度低 → Web Search 补充
  └── 模棱两可 → 两者都保留，让 LLM 判断
```

**与 Self-RAG 的区别**：

> Self-RAG 侧重"自我反思"（LLM 自己判断），CRAG 侧重"纠正行动"（不靠谱就换路子，比如搜互联网）。CRAG 更实用——检索不到就补搜，而不是让 LLM 硬答。

### 7.5 GraphRAG

**核心思想**：用知识图谱组织文档，支持跨文档的推理和全局摘要。

**传统 RAG 的局限**：

- 只能做局部检索——找到相关片段，拼起来
- 无法回答"全局性"问题（如"这些文档的主要主题有哪些？"）
- 跨文档的推理能力弱

**GraphRAG 流程**：

```
文档集合
  ↓ LLM 抽取实体和关系
知识图谱（Entity → Relation → Entity）
  ↓ 社区检测算法
社区层级摘要
  ↓ 检索时
Local Search: 从实体出发，遍历关联子图
Global Search: 从社区摘要出发，回答全局问题
```

**GraphRAG 适用场景**：

| 场景 | 传统 RAG | GraphRAG |
|------|----------|----------|
| "退款政策是什么？" | 好（局部检索够用） | 好 |
| "文档中所有涉及合规的条款有哪些？" | 差（需要全局理解） | 好 |
| "A 公司和 B 公司的业务关系是什么？" | 差（跨文档推理） | 好 |
| 简单 FAQ | 好（轻量快速） | 过重 |

**面试答法**：

> GraphRAG 用知识图谱解决传统 RAG 的两个短板：全局性问题和跨文档推理。但代价是建图成本高（LLM 要逐文档抽取实体关系），更新图也不方便。适合文档集合相对稳定、需要深度推理的场景（法律、研究），不适合频繁更新的 FAQ 类场景。

---

## 八、评估：RAGAS 框架

没有量化评估，就无法知道优化是否有效。RAGAS 是当前最主流的 RAG 评估框架。

### 8.1 四大核心指标

| 指标 | 评估什么 | 计算方式 | 好坏判断 |
|------|----------|----------|----------|
| Faithfulness (忠实度) | 生成答案是否忠于 Context | 答案中的每个 Claim 是否能被 Context 支持 | 越高越好（1.0 最好） |
| Answer Relevancy (答案相关性) | 生成答案是否切题 | 答案与 Query 的语义相关度 | 越高越好 |
| Context Precision (上下文精确度) | 检索到的 Context 中有用信息排不排前面 | 有用 chunk 在排序中的位置 | 越高越好 |
| Context Recall (上下文召回率) | Ground Truth 需要的信息有没有被检索到 | Ground Truth 中的信息是否被 Context 覆盖 | 越高越好 |

### 8.2 各指标的深入理解

**Faithfulness（忠实度）**：

```
Query: "退款需要多久？"
Context: "退款审核需要1-2个工作日，到账需要3-5个工作日。"
Answer A: "退款需要1-2个工作日。" → Faithfulness = 1.0（每个 claim 有支撑）
Answer B: "退款需要1-2个月。"  → Faithfulness = 0.0（与 Context 矛盾）
```

> Faithfulness 高不代表答案正确——如果 Context 本身就是错的，Faithfulness 仍然是 1.0。它只衡量"答案是否忠于检索到的上下文"。

**Context Precision vs Recall**：

```
Query: "退款政策是什么？"
Ground Truth 需要: chunk_1, chunk_5

检索结果: [chunk_3(无关), chunk_1(相关), chunk_7(无关), chunk_5(相关)]
→ Context Precision: 低（相关的没排前面）
→ Context Recall: 高（需要的信息都召回了）
```

> Precision 衡量"检索结果中多少是有用的"，Recall 衡量"需要的信息有没有被检索到"。优化检索主要提升这两个指标——Precision 靠 ReRank，Recall 靠混合检索和多路召回。

### 8.3 评估数据集构建

```json
{
  "question": "退款审核需要多久？",
  "ground_truth": "退款审核需要1-2个工作日。",
  "contexts": ["退款审核需要1-2个工作日，到账需要3-5个工作日。"],
  "answer": "退款审核1-2个工作日即可完成。"
}
```

**关键**：Ground Truth 需要人工标注或由更强的模型（如 GPT-4）生成，质量直接影响评估可信度。

### 8.4 评估驱动的优化循环

```
建立评估数据集 → 跑 RAGAS → 定位短板 → 优化
                                    ↓
             Faithfulness 低 → 优化 Prompt，加"只基于 Context 回答"
             Answer Relevancy 低 → Query Rewrite，优化 Prompt
             Context Precision 低 → 加 ReRank，调 Top-K
             Context Recall 低 → 混合检索，增加召回通道，调 Chunk Size
```

---

## 九、面试高频题

### Q1: RAG 和 Fine-tuning 怎么选？

> RAG 解决知识注入问题，知识可实时更新，可溯源，成本可控。Fine-tuning 解决行为适配问题，让模型学会特定输出格式或风格。生产中常结合使用：先 Fine-tune 适配业务，再 RAG 注入知识。如果知识频繁变化（如政策文档），RAG 是必须的。

### Q2: Chunk Size 怎么选？为什么需要 Overlap？

> Chunk Size 需要权衡精度和上下文完整性：小 Chunk 检索精度高但上下文可能不完整，大 Chunk 上下文完整但噪声多。一般推荐 512-1024 tokens。Overlap（通常 10%-20%）是为了避免语义被切分边界截断，相邻 Chunk 有重叠部分，保证至少一个 Chunk 包含完整语义单元。

### Q3: 为什么需要混合检索？RRF 是什么？

> 稠密检索擅长语义理解但不精确（"退款"和"退货"语义接近但检索可能遗漏精确关键词），BM25 擅长精确匹配但不懂语义。混合检索两者互补。RRF（Reciprocal Rank Fusion）是融合方法，用排序位置的倒数求和作为最终分数，避免了不同检索方法的原始分数不可直接比较的问题。

### Q4: 为什么需要 ReRank？为什么不在检索阶段直接用 Cross-Encoder？

> Bi-Encoder（Embedding 检索）Query 和 Document 独立编码，速度很快（向量可预计算），但交互浅。Cross-Encoder 把 Query 和 Document 拼在一起编码，交互深精度高，但每对都要过模型，无法预计算。所以生产上用 Bi-Encoder 做粗排（快），Cross-Encoder 做精排（准），两阶段设计。

### Q5: Self-RAG 和 CRAG 的区别？

> Self-RAG 让 LLM 自己做三重反思：要不要检索、检索结果相不相关、生成结果有没有支撑。CRAG 侧重"纠正"：检索结果不行就换路子（比如 Web Search），不硬答。Self-RAG 更"自省"，CRAG 更"实用"——检索不到就补搜。两者可以结合。

### Q6: GraphRAG 解决了什么问题？什么时候用？

> 传统 RAG 只能做局部片段检索，无法回答全局性问题（如"所有文档的主题概览"）和跨文档推理问题（如"A 和 B 的关系"）。GraphRAG 用知识图谱组织实体关系，支持子图遍历和社区级摘要。但建图成本高（需 LLM 逐文档抽取），适合文档稳定、需要深度推理的场景（法律、研究），不适合频繁更新的 FAQ。

### Q7: 向量数据库怎么选？

> 四个维度：规模、运维、过滤需求、技术栈。大规模生产 + Go 栈 → Milvus（原生分布式、Go SDK 成熟）；快速上线不想运维 → Pinecone（全托管）；需要复杂过滤 → Weaviate（GraphQL 查询灵活）；高性能小规模 → Qdrant（Rust 底层快）；原型验证 → Chroma（最简单）。

### Q8: Embedding 模型选择要注意什么？

> 三个关键点：1) 语言匹配——中文场景选 BGE-large-zh 或 GTE-Qwen2，中英混合选 BGE-M3；2) Query/Document 编码差异——BGE 系列需要给 Query 加前缀，Cohere 用 input_type 参数，不加会显著降精度；3) 部署方式——要私有化选开源模型，接受 API 选 OpenAI/Cohere。大规模数据自部署更划算。

### Q9: 如何评估 RAG 系统的效果？

> 用 RAGAS 框架，四个核心指标：Faithfulness（答案是否忠于上下文）、Answer Relevancy（答案是否切题）、Context Precision（检索结果中有用信息是否排前面）、Context Recall（需要的信息是否被检索到）。评估驱动优化：Faithfulness 低 → 优化 Prompt 加约束；Precision 低 → 加 ReRank；Recall 低 → 混合检索、调 Chunk Size。

### Q10: RAG 的常见失败模式有哪些？

> 1) **检索不到**：Chunk 切分不当、Embedding 模型不适合、Query 太模糊 → 优化 Chunking + Query Rewrite；2) **检索到但不相关**：Top-K 设太大、缺少过滤 → 加 ReRank + 元数据过滤；3) **检索到但 LLM 不用**：Prompt 引导不够、Context 太长被忽略 → 优化 Prompt + 控制 Context 长度；4) **幻觉**：LLM 超出 Context 编造 → 加强 Faithfulness 约束 + Self-RAG。

### Q11: 如何处理多轮对话中的 RAG？

> 核心问题是后续 Query 含代词/省略，直接检索效果差。解决方案：1) Query Rewrite——用 LLM 结合对话历史把当前 Query 改写为独立完整的查询；2) Contextual Retrieval——在 Chunk 前拼接文档级摘要，让每个 Chunk 自带上下文；3) 对话记忆 + Query 合并——把历史 Query 的关键信息合并到当前 Query 中。

### Q12: 生产环境中 RAG 系统的工程挑战有哪些？

> 1) **延迟**：Embedding + 检索 + ReRank + LLM 串行，总延迟可能 3-10s → 异步流式输出 + 缓存热门 Query；2) **文档更新**：增量索引、过期清理 → 设计 doc_version + TTL 机制；3) **多租户**：不同业务隔离数据 → 向量库的 Collection 隔离 + 元数据过滤；4) **成本控制**：Embedding API + 向量库 + LLM 调用 → 缓存 Query Embedding + 控制 Top-K + 选择性 ReRank；5) **可观测性**：检索质量、生成质量、延迟 → 记录每次检索的分数分布和 LLM 输出，定期跑 RAGAS 评估。

### Q13: HNSW 索引的核心原理是什么？

> HNSW（Hierarchical Navigable Small World）是一种分层图索引。核心思想：上层图节点稀疏、边长，用于快速跳到目标区域；下层图节点密集、边短，用于精确定位。查询时从顶层入口出发，逐层向下贪心搜索，时间复杂度 O(log N)。关键参数：M（每层最大邻居数，通常 16-64）影响图的连通性和内存；efConstruction（建图时的搜索宽度，通常 128-256）影响建图质量和速度；ef（查询时的搜索宽度，通常 64-128）影响查询精度和延迟。M 越大图越连通召回越高但内存越大，ef 越大查询越精确但越慢。

---

## 十、总结：RAG 系统架构全景图

```
用户 Query
  │
  ├─ Query Rewrite / HyDE（查询优化）
  │
  ▼
混合检索 ─────────────────────────────────┐
  │                                       │
  ├─ Dense Retrieval（向量检索）           │
  ├─ Sparse Retrieval（BM25 关键词）      │
  ├─ 知识图谱召回（可选）                  │
  │                                       │
  ▼                                       │
RRF 融合 → Top-K 候选                     │
  │                                       │
  ▼                                       │
ReRank（Cross-Encoder 精排）              │
  │                                       │
  ▼                                       │
Top-N 精选 Context ───────────────────────┘
  │
  ├─ Self-RAG / CRAG（质量校验）
  │
  ▼
Prompt 组装（Context + Query + Instruction）
  │
  ▼
LLM 生成回答
  │
  ▼
输出（含引用溯源）
```

**优化 RAG 的优先级排序**（投入产出比从高到低）：

1. **Chunking 质量** — 基础中的基础，切不好后面全白费
2. **混合检索** — 稠密 + 稀疏，立竿见影的提升
3. **ReRank** — 精排提升 Context Precision
4. **Query Rewrite** — 解决用户输入质量差的问题
5. **元数据过滤** — 缩小检索范围，减少噪声
6. **高级 RAG（Self-RAG / CRAG / GraphRAG）** — 特定场景的深度优化
7. **Embedding 模型升级** — 收益有但边际递减

**记住**：RAG 是系统工程，不要只优化一个环节。评估先行，数据驱动，找到瓶颈再针对性优化。
