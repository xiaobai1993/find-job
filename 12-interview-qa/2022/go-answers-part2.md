# 2022 面试题整理 — Go 进阶篇（2/5）

---

## Q20：谈谈 GC 和各个 Go 版本对 GC 的优化

### Go GC 演进史

| 版本 | GC 算法 | STW 时间 | 关键改进 |
|------|---------|---------|---------|
| Go 1.0 | STW 标记清除 | 百ms级 | 全程 STW，延迟极高 |
| Go 1.1 | STW 标记清除 | 百ms级 | 精确 GC（不再保守扫描） |
| Go 1.3 | 并行标记清除 | 十ms级 | 标记和清除与用户代码并行 |
| Go 1.5 | **三色并发标记清除** | **< 1ms** | 三色标记 + 混合写屏障，几乎无 STW |
| Go 1.8 | 三色并发 | **< 100μs** | 混合写屏障取代 Dijkstra/YY |
| Go 1.12 | 三色并发 | < 100μs | 更积极的 mark termination 并行 |
| Go 1.19 | 三色并发 | < 100μs | 软内存上限 GOGC + GOMEMLIMIT |
| Go 1.24 | 三色并发 | < 100μs | Swiss Table map 减少 GC 扫描开销 |

### 三色标记法

**三种颜色：**
- **白色**：未访问，GC 结束后白色对象被回收
- **灰色**：已访问但未扫描其引用，待处理
- **黑色**：已访问且已扫描其引用，不会被回收

**流程：**
1. 初始所有对象为白色
2. 根对象标记为灰色
3. 取灰色对象，扫描其引用的对象标灰，自身标黑
4. 重复步骤 3 直到没有灰色对象
5. 剩余白色对象即为垃圾，回收

### 写屏障（Write Barrier）

并发标记期间，用户代码可能修改引用关系，导致漏标。写屏障保证正确性。

| 写屏障 | 版本 | 特点 |
|--------|------|------|
| Dijkstra 插入写屏障 | Go 1.5 | 栈无屏障，需要 STW 重新扫描栈 |
| Yuasa 删除写屏障 | - | 保守，不漏标但可能多标 |
| **混合写屏障** | **Go 1.8+** | 结合两者优点，**栈不需要 STW 重新扫描** |

**混合写屏障规则：**
1. GC 开始时，栈上对象全部标黑（无需 STW 扫描栈）
2. 堆上被删除的引用，旧对象标灰（Yuasa）
3. 堆上新增加的引用，新对象标灰（Dijkstra）
4. 栈上操作无屏障

### GC 四阶段

```
1. Sweep Termination（清扫终止）
   └── 等待上一个 GC 的清扫完成

2. Mark Phase（标记阶段）— 并发，用户代码运行
   └── 启用写屏障
   └── 从根对象开始三色标记
   └── 后台 mark worker 并发标记

3. Mark Termination（标记终止）— STW
   └── 停止用户代码
   └── 完成剩余标记工作
   └── 关闭写屏障
   └── 这个阶段通常 < 100μs

4. Sweep Phase（清扫阶段）— 并发，用户代码运行
   └── 回收白色对象
   └── 通过 mcentral/mheap 回收内存
```

### GOGC 和 GOMEMLIMIT

**GOGC**（Go 1.5+）：控制 GC 触发频率
- `GOGC=100`（默认）：堆增长 100% 时触发 GC
- `GOGC=off`：关闭 GC
- `GOGC=200`：堆增长 200% 时触发，更少 GC 但更多内存

**GOMEMLIMIT**（Go 1.19+）：设置内存上限
- `GOMEMLIMIT=1GiB`：堆 + 栈总量不超过 1GB
- 解决 GOGC 在内存充足时 GC 太频繁的问题
- 比 GOGC 更直观，推荐使用

---

## Q21：监控线程（sysmon）是怎么避免单个协程执行时间过长的

**sysmon** 是 Go 运行时的后台监控线程，不需要 P 绑定，独立运行。

**抢占机制三阶段：**

### 1. Go 1.0-1.13：基于函数调用的栈检查（协作式）

```go
// 每个函数入口编译器插入检查
func foo() {
    // 编译器插入：如果 stackguard0 被标记，调用 morestack → 抢占
    // ...
}
```

**问题：** 如果 goroutine 执行纯计算循环（无函数调用），永远无法被抢占。

### 2. Go 1.14：基于信号的异步抢占

```
sysmon 检测到 G 运行超过 10ms
  │
  ├── 向 M 发送 SIGURG 信号
  │
  └── M 的信号处理器（sighandler）
        │
        ├── 修改 G 的 PC 寄存器
        │   让 G 跳转到抢占处理函数
        │
        └── G 被挂起，M 执行其他 G
```

**关键：** 不再依赖函数调用，即使死循环也能被抢占。

### 3. sysmon 的完整职责

| 职责 | 间隔 | 说明 |
|------|------|------|
| 抢占长时间运行的 G | 10ms | 发信号强制抢占 |
| 释放闲置 5ms 的 P | 5ms | 让 P 给其他 M 使用 |
| 触发 GC | - | 检查是否需要 GC |
| 回收闲置的 M | - | 释放操作系统线程 |
| 网络轮询 | 20μs~10ms | 检查 netpoller |

---

## Q22：Go 的 map 读取和写入是怎么实现的

### Go 1.24 之前：Bucket + 溢出链

```go
type hmap struct {
    count     int            // 元素个数
    B         uint8          // 桶数量 = 2^B
    hash0     uint32         // 哈希种子
    buckets   unsafe.Pointer // 桶数组
    oldbuckets unsafe.Pointer // 扩容时的旧桶
    // ...
}

type bmap struct {
    tophash [8]uint8  // 每个桶 8 个槽，存储哈希高 8 位
    // 后面紧跟 8 个 key、8 个 value、overflow 指针
    // 编译时动态生成完整结构
}
```

**读取流程：**

```
Load(key)
  │
  ├── 计算哈希 h = hash(key, hash0)
  │
  ├── 用低 B 位定位桶：bucket = h & (2^B - 1)
  │
  ├── 用高 8 位快速匹配：tophash = h >> (64-8)
  │     遍历桶内 8 个槽，对比 tophash
  │     tophash 不匹配 → 跳过（快速路径）
  │     tophash 匹配 → 比较完整 key
  │
  ├── 当前桶没找到 → 沿 overflow 链找下一个桶
  │
  └── 正在扩容 → 可能还要查 oldbuckets
```

**写入流程：**

```
Store(key, value)
  │
  ├── 计算哈希，定位桶
  │
  ├── 查找 key 是否存在
  │     ├── 存在 → 覆盖值
  │     └── 不存在 → 找空槽写入
  │
  ├── 桶满了 → 挂 overflow 桶
  │
  └── count/2^B > 6.5（负载因子）→ 触发扩容
        ├── 翻倍扩容：桶数量 ×2
        └── 等量扩容：溢出桶太多，整理压缩
```

**扩容是渐进式的：** 每次写操作迁移 1-2 个旧桶，不是一次性迁移。

### Go 1.24+：Swiss Table

```go
// 每个 group：8 个 key-value + 64 位 control word
type group struct {
    ctrl    [8]byte       // 控制字，每字节对应一个槽
    keys    [8]keyType    // 8 个 key
    values  [8]valueType  // 8 个 value
}
```

**改进：**
- 用 control word 的 metadata 快速过滤不匹配的 key（SIMD 友好）
- 线性探测取代溢出链，缓存友好
- 可扩展哈希：大 map 拆成多个 table（每个最多 1024 条目），减少扩容影响
- 内存使用更优，Datadog 报告节省了数百 GB

---

## Q23：panic 和 error 的使用场景

### 何时用 error

**业务逻辑中的预期错误，调用方应该处理的。**

```go
func ReadFile(path string) ([]byte, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read file %s: %w", path, err)
    }
    return data, nil
}
```

### 何时用 panic

**不可恢复的编程错误，程序不该继续运行。**

| 场景 | 示例 |
|------|------|
| 逻辑上不可能发生的情况 | `switch` 的 default 分支 |
| 初始化失败 | 配置文件缺失、数据库连不上 |
| 类型断言确定成功 | `v := i.(string)` 确定是 string |
| API 违反使用约定 | `sync.WaitGroup` 计数器为负 |

```go
func mustParseConfig(path string) *Config {
    data, err := os.ReadFile(path)
    if err != nil {
        panic(fmt.Sprintf("无法读取配置文件 %s: %v", path, err))
    }
    // ...
}
```

### 原则

| | error | panic |
|---|---|---|
| **调用方能否处理** | 能 | 不能 |
| **是否是预期错误** | 是 | 否 |
| **是否可恢复** | 是 | 通常不可恢复 |
| **库代码** | 必须用 error | 绝不用 panic |
| **main/init** | 可以用 panic | 可以用 |

**库代码的铁律：** 绝对不能向调用方抛 panic，必须返回 error。只有 `must*` 系列函数（如 `template.Must`）例外。

---

## Q24：Go 里面有哪些协程并发控制手段

| 方式 | 用途 | 特点 |
|------|------|------|
| `sync.WaitGroup` | 等待一组 goroutine 完成 | 简单，适合批量任务 |
| `sync.Once` | 只执行一次 | 单例、初始化 |
| `sync.Mutex / RWMutex` | 保护共享资源 | 互斥/读写锁 |
| `sync.Map` | 并发安全 map | 读多写少场景 |
| `sync.Pool` | 对象复用 | 减少 GC |
| `sync.Cond` | 条件等待 | 通常用 channel 替代 |
| `sync/singleflight` | 合并并发请求 | 防缓存击穿 |
| `channel` | 通信 + 同步 | CSP 模型 |
| `context` | 超时/取消传播 | 请求级生命周期 |
| `atomic` | 原子操作 | 无锁，保护单个变量 |
| `semaphore` (x/sync) | 限制并发数 | 信号量 |
| `errgroup` (x/sync) | 带错误处理的 WaitGroup | 收集错误 + 取消 |

---

## Q25：谈谈 channel 在工作中的使用场景

| 场景 | 模式 | 示例 |
|------|------|------|
| **信号通知** | `chan struct{}` | goroutine 完成通知 |
| **扇出/扇入** | 多 channel → 1 channel | 多个 worker 并行处理，结果合并 |
| **生产者消费者** | 缓冲 channel | 任务队列 |
| **限流** | 缓冲 channel 当信号量 | `make(chan struct{}, maxConcurrency)` |
| **超时控制** | channel + time.After | select 超时退出 |
| **优雅退出** | done channel | 收到信号后 close(done) 通知所有 goroutine |
| **pipeline** | 串行 channel | 数据流经多个处理阶段 |

**限流示例：**

```go
sem := make(chan struct{}, 10) // 最多 10 个并发
for _, task := range tasks {
    sem <- struct{}{} // 获取令牌
    go func(t Task) {
        defer func() { <-sem }() // 释放令牌
        process(t)
    }(task)
}
```

**优雅退出示例：**

```go
done := make(chan struct{})
go func() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    close(done) // 通知所有 goroutine 退出
}()

// 多个 goroutine 检查 done
go func() {
    for {
        select {
        case <-done:
            return
        default:
            doWork()
        }
    }
}()
```
