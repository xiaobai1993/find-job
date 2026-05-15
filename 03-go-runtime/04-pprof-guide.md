# pprof 性能分析实战指南

---

## 一、先搞懂：pprof 是什么？

pprof 是 Go 内置的性能分析工具，可以分析：

| 分析类型 | 说明 | 解决什么问题 |
|---------|------|-------------|
| **CPU profile** | 统计每个函数占用的 CPU 时间 | 哪里 CPU 高，性能瓶颈 |
| **Heap profile** | 统计内存分配情况 | 哪里内存泄漏，哪里分配多 |
| **Goroutine profile** | 所有 goroutine 的栈信息 | goroutine 泄漏，死锁 |
| **Mutex profile** | 锁竞争统计 | 哪个锁抢的人多，锁瓶颈 |
| **Block profile** | 阻塞时间统计 | 哪里卡了很久，IO 还是锁 |
| **Thread create profile** | 线程创建统计 | 为什么创建了这么多线程 |

**面试一句话总结：** pprof 就是 Go 程序的"CT 机"，程序慢了、卡了、内存涨了，用 pprof 一照就知道哪里有问题。

---

## 二、集成 pprof 到你的项目

### 1. 最简单的集成方式：一行代码

```go
import _ "net/http/pprof"

func main() {
    // 启动 pprof 服务，端口 6060，不影响业务
    go func() {
        _ = http.ListenAndServe(":6060", nil)
    }()

    // 你的业务代码...
}
```

就这么简单！import 一下就好了，`net/http/pprof` 会自动注册路由。

---

### 2. 可用的 pprof 端点

启动后，浏览器访问 `http://localhost:6060/debug/pprof/` 就能看到：

```
/debug/pprof/          → 主页，列出所有 profile
/debug/pprof/goroutine → goroutine profile
/debug/pprof/heap      → 内存 heap profile
/debug/pprof/profile   → CPU profile，默认 30s
/debug/pprof/threadcreate → 线程创建
/debug/pprof/block     → 阻塞 profile
/debug/pprof/mutex     → 锁竞争 profile

?debug=1 → 打印人类可读的文本
?debug=2 → goroutine 打印完整栈
?seconds=10 → 采集 10 秒
```

---

## 三、go tool pprof 交互式使用

### 1. 基本使用方式

```bash
# 方式 1：直接从 http 拉取
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10

# 方式 2：先把 profile 存成文件，再分析
curl -o cpu.pprof http://localhost:6060/debug/pprof/profile?seconds=10
go tool pprof cpu.pprof

# 方式 3：分析已经跑完的程序的 profile
go test -cpuprofile cpu.pprof -bench .
go tool pprof cpu.pprof
```

进入交互式界面后，常用命令：

| 命令 | 说明 |
|------|------|
| `top` | 显示占用最高的函数，默认前 10 个 |
| `top 20` | 显示前 20 个 |
| `top -cum` | 按累计时间排序（包括子函数调用） |
| `list 函数名` | 看这个函数的源码，每一行的开销 |
| `web` | 生成 svg 调用图，用浏览器打开（需要 graphviz） |
| `flamegraph` | 生成火焰图！Go 1.19+ 内置支持 |
| `peek 函数名` | 看这个函数的调用者和被调用者 |
| `focus 正则` | 只看匹配的函数 |
| `quit` / `exit` | 退出 |

---

### 2. top 输出各列含义

```
  flat  flat%   sum%        cum   cum%
  520ms 42.28% 42.28%     520ms 42.28%  runtime.kevent
  180ms 14.63% 56.91%     180ms 14.63%  runtime.pthread_cond_wait
   80ms  6.50% 63.41%      80ms  6.50%  runtime.memclrNoHeapPointers
```

| 列 | 含义 |
|----|------|
| **flat** | 函数自身执行的时间，不包括调用子函数的时间 |
| **flat%** | flat 占总时间的百分比 |
| **sum%** | 上面所有 flat% 加起来的总和 |
| **cum** | 累计时间，函数自身 + 所有子函数调用的总时间 |
| **cum%** | cum 占总时间的百分比 |

**怎么看：**
- flat 高 → 这个函数本身计算量大，是热点
- cum 高但 flat 低 → 这个函数调用链很深，瓶颈在子函数里

---

### 3. list 命令：定位到具体哪一行

```
(pprof) list json.Marshal
Total: 1.23s
ROUTINE ======================== encoding/json.Marshal in /usr/local/go/src/encoding/json/encode.go
     10ms      120ms (flat, cum)  9.76% of Total
      .          .    148: func Marshal(v interface{}) ([]byte, error) {
      .          .    149:     e := newEncodeState()
     10ms       10ms    150:     err := e.marshal(v, encOpts{escapeHTML: true})
      .          .    151:     if err != nil {
      .          .    152:         return nil, err
      .          .    153:     }
      .        110ms    154:     buf := append([]byte(nil), e.Bytes()...)
      .          .    155:     e.encoderPool.Put(e)
      .          .    156:     return buf, nil
      .          .    157: }
```

一眼就看出：第 154 行的 append 占了 110ms，是性能瓶颈！

---

## 四、火焰图：最直观的性能分析方式

### 1. Go 1.19+ 内置火焰图

```bash
# 进入 pprof 交互界面后直接输入
(pprof) flamegraph

# 或者直接生成
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/heap
```

浏览器打开 `http://localhost:8080/ui/flamegraph` 就能看到火焰图。

---

### 2. 火焰图怎么看？

```
┌────────────────────────────────────────────────────────────┐
│                          main                               │
├──────────────────┬─────────────────────────────────────────┤
│    foo           │              bar                        │
├────────┬─────────┼──────────────┬──────────────────────────┤
│ fooA   │  fooB   │   barA       │   barB                   │
└────────┴─────────┴──────────────┴──────────────────────────┘
```

| 特征 | 含义 |
|------|------|
| **Y 轴** | 调用栈深度，上面的是父函数，下面的是子函数 |
| **X 轴** | 宽度代表 CPU 时间占比，越宽越耗时 |
| **平顶** | 函数本身（不包括子函数）耗时大，这就是热点！ |

**找瓶颈的秘诀：找最宽的平顶！平顶越宽，这个函数本身越耗时，优化收益越大。**

---

## 五、实战：常见性能问题定位流程

### 场景 1：CPU 使用率很高，不知道哪里耗的

**Step 1：采集 CPU profile**
```bash
# 采集 10 秒 CPU
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=10
```

**Step 2：top 看热点**
```
(pprof) top
# 看哪个函数 flat 最高
```

**Step 3：list 看具体哪一行**
```
(pprof) list 热点函数名
```

**Step 4：火焰图确认调用关系**
```
(pprof) flamegraph
```

**常见 CPU 高的原因：**
1. 正则表达式写的烂，每次匹配回溯爆炸
2. JSON 序列化反序列化太频繁
3. 循环次数太多，复杂度 O(n²)
4. 字符串拼接用 + 不用 strings.Builder
5. GC 太频繁（内存分配太多）

---

### 场景 2：内存一直在涨，怀疑泄漏

**Step 1：采集 heap profile**
```bash
# 注意：heap profile 是采样的，不是 100% 准确
# 看分配空间用 -inuse_space，看分配次数用 -alloc_space
go tool pprof -inuse_space http://localhost:6060/debug/pprof/heap
```

**Step 2：对比两个时间点的 heap**
```bash
# 时间点 1 保存
curl -o heap1.pprof http://localhost:6060/debug/pprof/heap

# 过 10 分钟，时间点 2 保存
curl -o heap2.pprof http://localhost:6060/debug/pprof/heap

# 对比两个 heap，看哪里涨了
go tool pprof -base heap1.pprof heap2.pprof
(pprof) top
```

**Step 3：看 goroutine 有没有泄漏**
```bash
# 看 goroutine 数量是不是一直在涨
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

**常见内存泄漏的原因：**
1. goroutine 泄漏（最常见！channel 卡住、http 没超时）
2. 全局 map 只加不删
3. time.Ticker 没 Stop
4. context 没 cancel
5. defer 太多，资源没释放

---

### 场景 3：接口响应慢，不知道卡在哪

**Step 1：开 block profile**
```go
// 默认 block profile 是关的，要手动开采样率
runtime.SetBlockProfileRate(1)  // 1 = 全部采样，生产环境可以设大一点比如 10000
```

**Step 2：采集 block profile**
```bash
go tool pprof http://localhost:6060/debug/pprof/block
```

**Step 3：top 看哪里阻塞最久**
```
(pprof) top
# 看 cum 最高的，就是卡最久的
```

**常见阻塞原因：**
1. 锁竞争太激烈（mutex profile 配合看）
2. 数据库查询慢
3. http 调用没超时，等很久
4. channel 读写等待
5. IO 阻塞

---

### 场景 4：锁竞争严重，吞吐量上不去

**Step 1：开 mutex profile**
```go
// 默认 mutex profile 也是关的
runtime.SetMutexProfileFraction(1)  // 1 = 全部采样
```

**Step 2：采集 mutex profile**
```bash
go tool pprof http://localhost:6060/debug/pprof/mutex
```

**Step 3：看哪个锁抢的最凶**
```
(pprof) top
(pprof) list 锁所在的函数
```

**优化方向：**
1. 减小锁粒度，把大锁拆成几个小锁
2. 用 sync.RWMutex，读写分离
3. 用无锁数据结构，atomic 代替锁
4. 用 channel 代替共享内存锁

---

## 六、生产环境使用 pprof 注意事项

### 1. pprof 对性能影响有多大？

| profile 类型 | 性能开销 | 生产环境能不能用？ |
|-------------|---------|-------------------|
| CPU profile | ~5% | 可以，尽量采短时间（10~30s） |
| Heap profile | <1% | 完全没问题，默认就开的 |
| Goroutine profile | <1% | 完全没问题 |
| Block / Mutex | 看采样率 | 默认关，需要时开，采样率设高一点 |

**结论：** pprof 开销很小，生产环境完全可以放心用，不用怕。

---

### 2. 生产环境安全建议

1. **不要把 pprof 端口暴露到公网！** 用内网访问，或者加认证
2. **CPU profile 不要采太久**，10~30 秒足够了
3. **生产环境 block/mutex 采样率不要设太低**，设 1000 或更高
4. **用完就关**，需要的时候临时开，不用一直开着

---

### 3. 长期监控性能指标

不用每次都手敲 pprof，可以集成到监控里：

```go
// 定期采集 profile，上传到监控系统
go func() {
    for range time.Tick(5 * time.Minute) {
        // 采集 heap profile，上传
        // ...
    }
}()
```

---

## 七、常见坑与最佳实践

### 坑 1：heap profile 看不到 goroutine 栈上的内存

**注意：** heap profile 只统计堆上分配的内存，栈上的不统计。

**为什么？** 栈内存函数返回就自动释放了，GC 不用管，所以 heap profile 不统计栈。

---

### 坑 2：CPU profile 看不到内核态时间

**注意：** Go 的 CPU profile 默认只统计用户态时间，不包括内核态（系统调用、IO 等待）。

**如果内核态 CPU 高怎么办？** 用 `perf` 配合看：
```bash
perf top -p <pid>
```

---

### 坑 3：profile 采样有误差，不是 100% 准确

pprof 是采样的，不是每一条指令都统计，有统计误差。

**注意：**
- 小于 1% 的热点不用太在意，误差范围内
- 多采几次，看是不是每次都是同一个热点
- 样本时间越长（10s 以上），结果越准确

---

### 最佳实践总结

1. ✅ **每个服务都集成 pprof**，哪怕平时不用，出问题的时候能救命
2. ✅ **先 top 找热点，再 list 看具体行，最后火焰图看调用关系**
3. ✅ **内存泄漏先看 goroutine 数量**，90% 都是 goroutine 泄漏
4. ✅ **对比两个时间点的 profile**，看增量，比看绝对值更准
5. ✅ **CPU 高先看 GC 占比**，GC 高说明分配太多，去 heap profile 看哪里分配的
6. ❌ **不要把 pprof 端口暴露公网**
7. ❌ **不要过度优化小于 1% 的热点**，投入产出比太低

---

## 八、面试高频问答

| 问题 | 答案 |
|------|------|
| pprof 可以分析哪几种 profile？ | CPU、Heap、Goroutine、Mutex、Block、ThreadCreate 六种。 |
| flat 和 cum 的区别？ | flat 是函数自身执行时间，不包括子函数；cum 是累计时间，包括所有子函数调用。flat 高说明函数本身是热点，cum 高但 flat 低说明瓶颈在子函数里。 |
| pprof 对性能影响大吗？生产环境能用吗？ | 影响很小，CPU profile ~5% 开销，Heap <1%。生产环境完全可以放心用，采 10~30 秒足够了。 |
| 怎么定位内存泄漏？ | 1. 看 goroutine 数量是不是一直在涨（90% 都是 goroutine 泄漏）；2. 隔一段时间采两次 heap profile，对比增量；3. 用 debug=2 看所有 goroutine 的栈，找大量卡在同一个地方的。 |
| 怎么定位锁竞争？ | 开 mutex profile，SetMutexProfileFraction，采集后看哪个锁的 cum 时间最高。 |
| heap profile 统计栈上的内存吗？ | 不统计，只统计堆上分配的。栈上的函数返回就释放了，不用 GC 管。 |
| 火焰图怎么看？ | Y 轴是调用栈深度，X 轴宽度代表时间占比。找最宽的平顶，平顶越宽，这个函数本身越耗时，优化收益越大。 |
| pprof 默认是采样的还是精确统计的？ | 采样的，CPU profile 每秒采 100 次，heap profile 每分配 512KB 采一次样，有统计误差，但足够定位问题了。 |
| block profile 默认是开的吗？ | 默认关的，要手动调用 runtime.SetBlockProfileRate 开启。 |
| mutex profile 默认是开的吗？ | 默认关的，要手动调用 runtime.SetMutexProfileFraction 开启。 |
| 为什么 RSS 很高但 heap profile 显示用的不多？ | 可能是栈内存、或者 Go 缓存了内存没还给 OS、或者 CGO 分配的内存（pprof 不统计 CGO 分配的）。 |
| 怎么对比两个 profile？ | 用 `-base` 参数：`go tool pprof -base old.pprof new.pprof`，看增量。 |

---

## 九、一句话总结

> pprof 是 Go 性能分析的瑞士军刀，每个服务都要集成；CPU 高看 top 找 flat 热点，内存泄漏先看 goroutine 数量再对比两个时间点的 heap，锁卡了看 mutex profile，响应慢看 block profile；记住 flat 是自身 cum 是累计，火焰图找最宽的平顶，90% 的性能问题都能快速定位。
