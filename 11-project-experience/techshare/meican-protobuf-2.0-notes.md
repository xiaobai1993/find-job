# Meican Protobuf 2.0 技术分享笔记

> 原始 PDF：`Intro to Meican Protobuf 2.0.pdf`
> 作者：zhengchao.deng@meican.com

整体分三个部分：**Protobuf Intro → API → Plan**

---

## 第一部分：Protobuf Intro

### protoc & 插件系统

`protoc` 编译器的工作流程：

```
.proto 文件
   ↓
protoc（编译器主程序）
   ↓ 通过不同插件（--xxx_out 参数）生成不同产物
──────────────────────────────────────
--go_out        →  *.pb.go          （Go 消息结构体）
--go-grpc_out   →  *_grpc.pb.go     （gRPC 服务代码）
--validate_out  →  *.pb.validate.go （字段校验代码）
--openapiv2_out →  *.swagger.json   （Swagger 文档）
--doc_out       →  *.html / *.md    （API 文档）
```

### protoc 和 plugin 的关系

- **protoc 本身**：只负责 parse .proto 文件，把解析结果序列化后写到 stdout
- **plugin（插件）**：
  - 通过 `--xxx_out` 参数触发，protoc 从 `PATH` 里找对应插件二进制
  - protoc 把解析结果通过 **stdin** 传给插件
  - 插件自己实现代码生成逻辑，输出文件

### 1.0 时代没解决的问题

| 问题 | 说明 |
|------|------|
| 依赖版本怎么处理 | 不同服务依赖同一 proto 的不同版本 |
| import 路径怎么找 | 编译时 proto 文件的 import 路径解析问题 |
| well-known types | Google 官方内置类型（如 Timestamp、Duration）的处理 |
| protoc + plugin 版本稳定性 | 不同机器/环境编译出的代码不一致 |

2.0 版本就是为了解决以上问题而设计的。

---

## 第二部分：API

### API 版本管理

- **Versioning is a basis for Upgrade**：版本化是升级的基础
- **Export versioning explicitly**：版本号必须显式体现在路径中（如 `/v1/`, `/v2/`），支持不破坏旧调用方的前提下升级

### API 规范

- **Naming**：字段名、方法名、包名统一规范
- **Consensus Conflicts**：多团队对同一 API 理解不一致时的解决机制

### API 多样性

公司内部存在两种不同性质的 API：

| 类型 | 特点 |
|------|------|
| **gRPC API**（内部服务间） | Ability Oriented（能力导向）、Solid Express（表达精确）、Stable & Reuse（稳定可复用） |
| **Public / Open API**（对外/BFF） | Scene Oriented（场景导向）、concrete（描述具体业务场景） |

---

## 第三部分：Plan（2.0 方案设计）

### 开发流程定义

整个 Protobuf 2.0 方案的生态圈包含：

```
                 guides（指南）
          specifications（规范）
protoc      docs（文档）     protoc plugins（插件）
            constraints（约束/校验）
```

三步开发流程：
1. **Define your proto** — 按规范编写 .proto 文件
2. **Compile（local / remote）** — 本地或远程编译
3. **Distribution** — 分发生成的代码给各个服务

### Monorepo

把所有的 .proto 文件放在**同一个仓库**管理，解决三个问题：

1. **Type Reuse** — 公共类型（如 Address、User）可被多个服务引用，避免重复定义
2. **Business Distribution** — 按业务分目录管理，结构清晰
3. **Consensus** — 所有人在同一个地方修改，统一共识

### 自动化流程

#### 本地（Local / branch）

```bash
make build  # 本地执行所有编译逻辑
```

#### CI 流程（Gitlab CI）

| 触发分支 | CI Job | 说明 |
|----------|--------|------|
| branch | `compile_protobuf_and_tag` | 编译并打一个 CI tag（如 `v1.5.0-fix-ci.20230214014150`） |
| branch | `tag_branch` | 仅打 tag |
| main | `changelog_and_tag` | 打出正式版 tag（如 `v1.5.0`） |

整个发版流程由 `marvin_bot` 自动执行，commit message 带 `[skip ci]` 避免循环触发。

#### BFF 自动化

前端 BFF 层接入自动化：
- **pull** — 自动拉取最新 proto 编译产物
- **ts-plugin code generation** — 生成 TypeScript 类型代码
- **GraphQL integration** — 集成到 GraphQL 层

---

## 总结

这套方案的核心思路：

> 用一个中央 Monorepo 管理所有 .proto 文件 → 通过 Gitlab CI 自动编译 + 打版本 tag → 各服务依赖版本化的编译产物 → 解决了版本混乱、环境不一致、依赖难找的问题

### 面试中可以重点描述的亮点

- 统一了 proto 仓库管理（Monorepo），实现了类型复用和跨团队共识
- 解决了 protoc + plugin 版本不一致导致的编译结果不稳定问题
- 通过 Gitlab CI 实现全自动化编译、lint、版本发布流程
- 支持后端（Go gRPC）和前端（TypeScript / GraphQL）双端代码自动生成
