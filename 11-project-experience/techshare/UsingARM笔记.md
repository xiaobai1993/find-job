# Using ARM CPU Architecture in Production 技术分享笔记

> 美餐内部技术分享，背景是 FinOps（降低云成本）驱动，把生产服务从 x86（amd64）迁移到 ARM（arm64）。

---

## 一、ARM 是什么 —— CISC vs RISC

**ARM = Advanced RISC Machine**（精简指令集机器）

- x86 = **CISC**（Complex Instruction Set Computers，复杂指令集）
- ARM = **RISC**（Reduced Instruction Set Computers，精简指令集）

### 以"两数相乘并存回内存"为例

```
CISC（x86）：1 条指令
MULT 2:3, 5:2       ← 直接内存×内存，硬件内部完成所有步骤

RISC（ARM）：4 条指令
LOAD A, 2:3         ← 从内存加载到寄存器 A
LOAD B, 5:2         ← 从内存加载到寄存器 B
PROD A, B           ← 寄存器相乘
STORE 2:3, A        ← 结果写回内存
```

### CISC vs RISC 对比

| | CISC | RISC |
|--|--|--|
| 设计重点 | **硬件** | **软件** |
| 指令复杂度 | 多时钟周期复杂指令 | 单时钟周期简单指令 |
| 内存访问 | LOAD/STORE 集成在指令里 | LOAD/STORE 是独立指令 |
| 晶体管用途 | 存储复杂指令逻辑 | 更多通用寄存器 |
| 代码大小 | 小（指令数少） | 大（指令数多） |

### RISC 的优势（代码行数多≠性能差）

1. **流水线（Pipelining）**：每条指令恰好一个时钟周期，流水线可以满负荷工作
2. **更多通用寄存器**：省出的晶体管用来做寄存器，减少内存读写频率
3. **寄存器数据不自动清除**：CISC 执行完 MULT 后寄存器自动清空；RISC 里操作数一直留在寄存器，可复用

---

## 二、为什么服务器要用 ARM —— AWS Graviton

### 背景：摩尔定律放缓

> "集成电路上的晶体管数量每两年翻一番"

工艺节点 5nm → 3nm 越来越难，x86 提升空间有限。AWS 自己做 ARM 芯片（Graviton）走了另一条赛道。

### AWS Graviton 规格亮点

- 7nm 工艺，300 亿晶体管
- **每个 vCPU = 物理核心，无超线程（Hyper-Threading）**
- 大缓存：L1 **64KB** / L2 **1MB** per vCPU
  - 对比 Intel i7 6700：L1 32KB / L2 256KB（Graviton 是 2~4 倍）

### AWS Graviton 官方宣称优势

- 性能 **+20%**
- 实例费用 **-20%**
- 能耗效率更高（lower power consumption）

---

## 三、Benchmarking —— 内部实测数据

### Go WebServer 高负载

**Server 1 vCPU，Client 100 并发连接：**

| 架构 | Total Requests | RPS | Avg Latency | P50 | P99 |
|--|--|--|--|--|--|
| amd64 | 1,761,056 | 14.75k | 15.70ms | 1.15ms | 71.54ms |
| arm64 | 2,018,862 **(+14.6%)** | 16.89k **(+14.5%)** | 12.92ms **(-17.7%)** | 1.22ms (+5.7%) | 63.55ms **(-12.5%)** |

**Server 2 vCPU，Client 200 并发连接：**

| 架构 | Total Requests | RPS | Avg Latency | P50 | P99 |
|--|--|--|--|--|--|
| amd64 | 3,521,897 | 29.48k | 8.06ms | 1.51ms | 45.26ms |
| arm64 | 4,245,043 **(+20.5%)** | 35.54k **(+20.5%)** | 6.04ms **(-25.1%)** | 1.09ms **(-38.5%)** | 37.42ms **(-20.9%)** |

### Go WebServer 低负载（最惊人的数据）

**Server 2 vCPU，Client 10 并发连接：**

| 架构 | Total Requests | RPS | Avg Latency | P50 | P99 |
|--|--|--|--|--|--|
| amd64 | 2,772,679 | 23.19k | 562.29us | 179.00us | **9.44ms** |
| arm64 | 3,059,087 (+10.3%) | 25.62k (+10.4%) | 205.77us (-63.4%) | 174.00us (-2.8%) | **803.00us (-91.5%)** ← 圈出重点 |

> **低负载下 P99 延迟下降 91.5%！**
>
> 原因：ARM 的每个 vCPU 都是物理核心，没有超线程争用，CPU 调度抖动极小，长尾延迟大幅改善。

### Benchmark 结论

- ARM 在**高并发、多 vCPU** 场景下优势最明显
- 物理核心无 throttling（x86 超线程会在负载高时抢占物理核，产生延迟毛刺）
- **Web 服务场景特别适合迁 ARM**

### 踩坑：Graviton 2 的 RSA 性能问题

- **RSA 签名（Sign）**：正常
- **RSA 验签（Check）**：性能下降 **50%**！
- 原因：Graviton 2 没有专用 RSA 加速核心
- **Graviton 3 已修复**（额外增加了专用核心）

> 如果服务有大量 RSA 验签（如 JWT 验证、TLS 证书校验），务必用 Graviton 3 实例。

---

## 四、如何构建 ARM 容器镜像

### 前提条件

- Go >= 1.9（官方支持 Linux ARM 64bit）

### 本地构建（docker buildx）

```bash
docker buildx build \
  --platform linux/arm64,linux/amd64 \
  -t misc:test \
  -f Dockerfile .
```

> nerdctl 在 MacOS 上难用，不推荐。

### 生产 CI 构建流程（GitlabCI + Kaniko）

```
Build AMD64 Job (tags: go)           Build ARM64 Job (tags: go-arm64)
         ↓                                    ↓
   v0.1-amd64 (ECR)               v0.1-arm64 (ECR)
              ↘                  ↙
            Combine Image Job (Auto 自动触发)
              使用 manifest-tool 合并
                      ↓
              v0.1 (multi-arch manifest)
```

**三个关键 CI Job：**

| Job | 运行机器 | 工具 | 作用 |
|--|--|--|--|
| `compile_upload_ecr_..._amd64` | amd64 (tags: go) | Kaniko | 编译+推送 amd64 镜像 |
| `compile_upload_ecr_..._arm64` | arm64 (tags: go-arm64) | Kaniko | 编译+推送 arm64 镜像 |
| `combine_image_for_...` | arm64 (auto) | manifest-tool | 合并为 multi-arch manifest |

### combine_image 核心命令

```bash
manifest-tool --docker-cfg /.docker/config.json push from-args \
  --platforms linux/amd64,linux/arm64 \
  --template $ECR_PATH/$PROJECT_NAME:$RELEASE_VERSION-ARCH \
  # ↑ ARCH 是模板变量，manifest-tool 会分别替换为 arm64 / amd64
  --tags $RELEASE_VERSION \
  --target $ECR_PATH/$PROJECT_NAME:$RELEASE_VERSION
```

**结果：** `$PROJECT_NAME:v0.1` 这个 tag 背后是一个 manifest list，拉取时 Docker 自动选择对应架构。

### Dockerfile 要点

```dockerfile
FROM 651844176281.dkr.ecr.cn-northwest-1.amazonaws.com.cn/titan/base-images/alpine-tz8:v3.18.2
# ↑ 基础镜像必须支持 multi-arch！

COPY --from=build /executor /executor
COPY --from=build /go/src/public /public

RUN addgroup go \
    && adduser -D -G go go \
    && chown -R go:go /executor

EXPOSE 18888   # metrics port
EXPOSE 8023
EXPOSE 8024

ENTRYPOINT ["/executor"]
```

### 验证基础镜像是否支持 multi-arch

```bash
docker manifest inspect <base-image>
```

输出的 `manifests` 数组里如果同时有 `amd64` 和 `arm64` 的 platform，才支持 multi-arch。

---

## 五、GitlabCI 最佳实践：使用 include 模板

```yaml
# .gitlab-ci.yml（项目自身，只有 20+ 行）
variables:
  PROJECT_NAME: titan/alertmanager-bridge
  REPO_NAME: go.planetmeican.com/titan/alertmanager-bridge

include:
  - project: 'meican-cd/ci-template'
    ref: main
    file: all-in-one-go1.20.yml    # 引用共享模板，包含 600+ 行的 job 定义

stages:
  - init
  - lint
  - analysis
  - report
  - build_branch
  - combine_image_for_branch
  - release
  - build_release
  - combine_image_for_release
```

**优势：**
- 项目从 600+ 行 → 20+ 行
- 模板统一维护，所有服务自动获得更新

**参数化模板：** 通过变量覆盖默认值

```yaml
variables:
  ANALYSIS_TIMEOUT: 1m      # 覆盖模板默认值
  ANALYSIS_GOFLAGS: -mod=vendor
```

**Override 特定 Job：** 需要启动额外服务（如 Redis）时

```yaml
# 覆盖模板里的 test:go_test job
test:go_test:
  services:
    - 651844176281.dkr.ecr.../titan/base-images/redis:6.2
```

### GitlabCI Workflow 全览

**Branch 构建（分支推送触发）：**
```
init → lint → analysis → report → build_branch(手动) → combine_image_for_branch(自动)
```

**Release 构建（tag 触发）：**
```
init → build_release(手动，6个并行job: prod/sandbox/staging × amd64/arm64)
     → combine_image_for_release(自动，3个job: meican1/meican2/sandbox)
```

---

## 六、K8s 部署配置

```yaml
nodeSelector:
  intend: app1
  kubernetes.io/arch: arm64   # Optional，可以不写，靠 toleration 就够

tolerations:
- key: kubernetes.io/arch
  value: arm64
  operator: Equal
  effect: NoExecute           # ARM64 节点有此 taint，必须配 toleration 才能调度
```

**机制说明：**
- ARM64 节点打了 `kubernetes.io/arch=arm64:NoExecute` taint
- 没有 toleration 的 Pod 不会被调度到 ARM64 节点
- 加了 toleration = 明确表示"我的镜像支持 ARM64，允许调度过去"
- `nodeSelector` 可选，主要用于强制指定到 ARM64 节点（不写则 amd64/arm64 混合调度）

---

## 七、内部基础镜像（支持 multi-arch）

| 用途 | 镜像 |
|--|--|
| Go lint | `titan/base-images/golangci-lint:v1.52.2-alpine-mod-token` |
| CI 编译（Kaniko） | `gcr.io/kaniko-project/executor:v1.9.2-debug` |
| 合并 manifest | `titan/base-images/manifest-tool:alpine-v2.0.8` |
| 应用基础镜像 | `titan/base-images/alpine-tz8:v3.18.2` |

> 2023-07-21 统一升级为 multi-arch 版本，更新记录在 RELEASE.md / README.md。

---

## 八、真实成本收益

**2023-07 账单**（EC2 + ElasticCache + ElasticSearch + RDS，单位：CNY）：

| 环境 | 费用 |
|--|--|
| Sandbox | 88,975.75 |
| Staging | 18,666.42 |
| Meican1 | 671,440.96 |
| Meican2 | 110,939.22 |
| **合计** | **890,022.35** |
| **节省** | **178,004.47 CNY** |

一个月节省约 **18 万人民币**，约 **20%** 的云成本。

---

## 九、迁移背景与时间节点

- 背景：FinOps 战略，目标减少云支出、建立支出模型
- 最终警告时间：**2023-12-01**
- 迁移截止：**2023-12-31**（公司强制要求）

---

## 十、总结与面试可说的点

1. **ARM vs x86 本质**：RISC vs CISC，指令简单但流水线效率高，物理核心无超线程
2. **AWS Graviton 亮点**：物理核心无 throttling，L1/L2 缓存更大，成本低 20%
3. **Benchmark 关键数据**：Go WebServer 高并发场景 RPS +20%、延迟 -20%；低负载 P99 延迟降 91.5%
4. **RSA 踩坑**：Graviton 2 验签性能 -50%，Graviton 3 修复
5. **构建方案**：`docker buildx --platform` 本地构建；CI 用 Kaniko 分别编译两个架构，再用 manifest-tool 合并为 multi-arch manifest
6. **K8s 部署**：ARM64 节点有 taint，Pod 必须配 toleration 才能调度；nodeSelector 可选
7. **CI 模板化**：用 GitlabCI include 引用公司级模板，600 行 → 20 行，统一维护
