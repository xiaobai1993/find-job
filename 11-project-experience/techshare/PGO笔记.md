# Golang Profile Guided Optimization (PGO)

> 技术分享：美餐内部 Go PGO 实践 — HUANG Yanshuo, DevOps

## 目录

1. [什么是 PGO](#什么是-pgo)
2. [PGO 如何工作](#pgo-如何工作)
   - [函数内联（Function Inlining）](#函数内联function-inlining)
   - [接口调用去虚化（Devirtualization of Interface Call）](#接口调用去虚化devirtualization-of-interface-call)
3. [PGO 常见问题](#pgo-常见问题)
4. [真实项目中的收益](#真实项目中的收益)
5. [如何接入 PGO](#如何接入-pgo)
6. [AutoFDO：自动化持续优化](#autofdo自动化持续优化)
7. [总结](#总结)

---

## 什么是 PGO

**Profile Guided Optimization**，也叫 **FDO（Feedback-Directed Optimization）**。

核心思想：
- 采集线上真实 CPU Profile
- 将 Profile 反馈给编译器
- 编译器据此做出更优的优化决策

关键特点：
- 性能提升 **2~14%**
- 接入成本极低，改动小
- Go 1.20+ 官方支持（`-pgo` 编译标志）

### pprof 热点示例

```
flat    flat%   sum%     cum     cum%
116.65s 65.14%  65.14%  135.72s  75.79%   main.processWorkLoad
 21.35s 11.92%  77.06%  157.09s  87.72%   main.main
 ...

# 热循环代码（占 99.70% 累计时间）
59: for _, d := range w.data {
60:     sum += w.processor.Process(d)   ← 接口调用热点
61: }
```

---

## PGO 如何工作

两大核心优化手段：
1. **Function Inlining**（函数内联）
2. **Devirtualization of Interface Call**（接口调用去虚化）

---

### 函数内联（Function Inlining）

#### 原理

将被调用函数的函数体直接展开到调用处，消除函数调用开销（参数传递、栈帧建立、返回跳转）。

```go
// 内联前
func add(x, y int) int { return x + y }
func sum(n int) int {
    v := 0
    for i := 0; i < n; i++ {
        v = add(v, i)   // 函数调用
    }
    return v
}

// 内联后（编译器展开）
func sum(n int) int {
    v := 0
    for i := 0; i < n; i++ {
        v = v + i        // 直接计算，无调用
    }
    return v
}
```

#### Benchmark 数据

| 场景 | 耗时 |
|------|------|
| Add（内联） | 0.2694 ns ± 8% |
| AddNoInline（禁止内联） | 0.8388 ns ± 5% |
| SumWithInline | 275.5 ns ± 4% |
| SumWithoutInline | 829.0 ns ± 2% |

**结论：内联带来约 3x 性能提升**

#### 汇编对比（-gcflags -S）

```
# w/o inline（蓝框：多余的函数调用序列）
MOVD  R1, main.i-8(SP)
MOVD  R2, R0
CALL  main.addNoInline(SB)   ← 函数调用指令
MOVD  main.i-8(SP), R2
ADD   $1, R2, R1
...

# w/ inline（绿框：直接计算，指令更少）
JMP   28
ADD   $1, R1, R3
HINT  $0
ADD   R1, R2, R2
MOVD  R3, R1
CMP   R1, R0
...
```

#### PGO 对内联的作用

Go 编译器有**内联预算（Budget）**限制，函数太大会被跳过。

以官方 Markdown HTTP Server 为例：
- `ParserBlock.Parse` 函数体太大，正常情况**不会被内联**
- CPU Profile 显示它是热路径，PGO 判断值得**突破预算**强制内联
- 结果：整体性能提升

**为什么不把所有函数都内联？**

| 负面影响 | 说明 |
|---------|------|
| 代码膨胀 | 函数体被复制到每个调用处 |
| 调试困难 | Stack trace 变得不直观 |
| CPU 指令缓存失效 | 代码过长 → I-Cache 失效 |
| 编译耗时增加 | 需要额外分析和展开 |

#### Markdown Server 实测结果

```bash
$ go tool pprof -diff_base cpu.nopgo.pprof cpu.pgo.pprof
# CPU 优化
Showing nodes accounting for -9.33s, 8.42% of 110.75s total

# Heap 优化（alloc_objects）
Showing nodes accounting for -2839721, 1.66% of 170878373 total
```

**PGO 带来 8.42% CPU 节省，1.66% 堆分配减少**

pprof peek 对比：
```
# w/o PGO
| gitlab.com/golang-commonmark/markdown.ParserBlock.Parse

# w/ PGO
| gitlab.com/golang-commonmark/markdown.ParserBlock.Parse (inline)  ← 被内联了
```

---

### 接口调用去虚化（Devirtualization of Interface Call）

#### 原理

Go 接口调用是**间接调用**（通过 iface/iTab 查找方法地址），有额外开销。PGO 通过 Profile 推断接口的**实际运行时类型**，将间接调用转为直接调用。

```go
type Processor interface {
    Process(x int) int
}

type FastProc struct{}
func (p FastProc) Process(x int) int { return x * 2 }

// 接口调用（虚调用）
func callInterface(proc Processor, data []int) int {
    sum := 0
    for i := range data {
        sum += proc.Process(data[i])   // 间接调用
    }
    return sum
}
```

**去虚化后**，编译器生成类型守卫代码：

```go
// After devirtualization（逻辑等价）
if fp, ok := iproc.(FastProc); ok {
    fp.Process(x: 10)    // 直接调用，可进一步内联
} else {
    proc.Process(x: 10)  // fallback 间接调用
}
```

#### Benchmark 对比

| 场景 | 禁用优化 | 启用优化 |
|------|---------|---------|
| Interface 调用 | 1462 ns/op | 273.3 ns/op |
| Direct 调用 | 812.2 ns/op | 280.9 ns/op |

**结论：启用去虚化后，接口调用性能与直接调用几乎持平（273 vs 281 ns/op）**

#### Go 接口底层机制（iface & iTab）

```go
// 每个非空接口值的内部表示
type iface struct {
    tab  *itab          // 指向 iTab（接口类型+具体类型信息）
    data unsafe.Pointer // 指向实际数据
}

type ITab struct {
    Inter *InterfaceType
    Type  *Type
    Hash  uint32        // Type.Hash 的副本，用于 type switch
    Fun   [1]uintptr    // 方法指针数组
}
```

**汇编层面对比：**

```
# w/o 去虚化（间接调用，多一次内存查表）
MOVD  24(R0), R3    // Load method address from interface table
MOVD  (R2)(R4<<3), R5  // Load argument
MOVD  R1, R0        // Setup receiver
MOVD  R5, R1        // Setup argument
PCDATA $1, $0       // Register map for GC
CALL  (R3)          // Indirect call through interface table

# w/ 去虚化（直接调用）
MOVD  (R0)(R2<<3), R1  // Load argument
MOVD  R1, R0           // Setup receiver
PCDATA $1, $0          // Register map for GC
CALL  main.FastProc.Process(SB)  // Direct call（可进一步内联）
```

**函数大小对比：**
```
main.callInterface STEXT size=192  ← 接口调用版本更大
main.callDirect    STEXT size=160
```

#### 去虚化的限制

与函数内联一样，也受**调用频率预算**约束。

Duck Typing 问题：如果接口有太多实现（如 `io.Closer.Close()` 有 **331 种实现**），Profile 中单一类型的调用比例不够高，则无法去虚化。

---

## PGO 常见问题

| 问题 | 答案 |
|------|------|
| PGO 会优化标准库吗？ | 会，标准库同样受益 |
| PGO 会让程序变慢吗？ | 不会，对非热路径的内联不会负面影响 |
| PGO 对构建时间和产物大小的影响？ | 构建时间和 binary 大小略有增加 |
| 不同 workload 类型怎么处理？ | 为不同 workload 维护不同 build |

---

## 真实项目中的收益

### titan/pagerduty

```
pprof diff: -120ms, 12.00% of 1000ms total
```

CPU 使用率图表呈现明显下降趋势。

### titan/k-watch

```
pprof diff: -160ms, 16.00% of 1000ms total
```

主要热点：`runtime/internal/syscall.Syscall6`、`runtime.memmove`、JSON 相关函数。

### titan/cmdb-agent

```
pprof diff: -120ms, 18.75% of 640ms total
```

主要热点：`reflect.(*rtype).Kind`、`runtime.memmove`、`encoding/json` 相关。

---

## 如何接入 PGO

三步接入，极其简单：

```bash
# Step 1: 正常编译
go build -o app .

# Step 2: 在负载下运行，采集 CPU Profile（30s）
curl -o default.pgo "http://localhost:8080/debug/pprof/profile?seconds=30"

# Step 3: 带 PGO 重新编译
# go build 默认自动查找 default.pgo 文件
go build -o app .

# 或显式指定
go build -pgo default.pgo -o app .
```

> `default.pgo` 文件建议**提交到代码仓库**，CI 构建时自动使用。

---

## AutoFDO：自动化持续优化

### 背景

随着业务演进，profile 会逐渐**过时（stale）**：
- 新功能引入新的热函数
- 依赖库升级改变调用路径

### AutoFDO（Auto Feedback Decision Optimization）

来源：Google Research Paper，解决 profile 持续更新问题。

**核心思路：定期合并新 Profile**

```bash
# 合并新采集的 profile 到现有 default.pgo
go tool pprof -proto default.pgo new.pprof > default.pgo
```

### AutoFDO 工作流

```
1. 正常构建并发布（无 PGO）
        ↓
2. 从生产环境持续采集 Profile
        ↓
3. 发布新版本时，用最新 source + 生产 Profile 重新构建
        ↓
4. GOTO 2（循环迭代，profile 持续更新）
```

### GitLab CI 集成示例

```yaml
variables:
  PROJECT_NAME: titan/pagerduty
  REPO_NAME: go.planetmeican.com/titan/pagerduty
  TELEMETRY_ENDPOINT: http://pagerduty-telemetry.app-titan.svc.cluster.local:18888

include:
  - project: 'meican-cd/ci-template'
    ref: main
    file: jobs/pgo.yml

stages:
  - pgo
```

CI 自动执行：
1. 连接 telemetry endpoint 采集 CPU Profile
2. 保存为 `pgo/cpu-sandbox-{timestamp}.pprof`
3. 更新 `default.pgo` 软链
4. 创建 MR（`perf-pgo` 分支）等待合并触发构建

---

## 总结

| 要点 | 说明 |
|------|------|
| 最大化收益 | Profile 在**生产环境高峰期**采集，代表真实热路径 |
| AutoFDO | 通过 GitLab CI 定期更新 profile，保持优化不过时 |
| 实际效果 | 内部服务 CPU 节省 **10%+**，涉及 850+ EC2 CPU Cores |
| ROI | 接入成本极低，收益显著，性价比高 |

### PGO 两大优化机制对比

| 机制 | 原理 | 收益 | 限制 |
|------|------|------|------|
| 函数内联 | 将热函数体展开到调用处 | 消除调用开销，x3 提速 | 受内联预算限制，代码膨胀 |
| 接口去虚化 | 将热接口间接调用转为直接调用 | 消除 iTab 查表开销 | 需要单一类型主导，Duck Typing 实现多时无效 |

---

## 参考资料

- Go 官方 PGO 文档：https://go.dev/blog/pgo
- Google AutoFDO 论文：https://research.google/pubs/autofdo-automatic-feedback-directed-optimization-for-warehouse-scale-applications/
- Go inline 实现：https://pkg.go.dev/golang.org/x/tools/internal/refactor/inline
