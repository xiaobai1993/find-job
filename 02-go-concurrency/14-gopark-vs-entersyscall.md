# gopark vs entersyscall：Go 两种阻塞机制彻底搞懂

---

## 一、核心概念：gopark() = G 被其他数据结构"收养"

G 调用 `gopark()` 后，会**离开运行队列**，进入 `_Gwaiting` 状态。此时 G 必须有一个数据结构负责"收养"它——把它挂到自己的等待队列上，并在条件满足时调用 `goready()` 把 G 放回运行队列。

**如果没有数据结构收养，G 就永远丢失了！**

```
gopark() 之前：G 在 P 的运行队列里 → _Grunning
gopark() 之后：G 从运行队列消失 → _Gwaiting → 被某个数据结构收养
goready() 之后：G 回到 P 的运行队列头部 → _Grunnable → 下次优先执行
```

---

## 二、所有 gopark 场景一览

### 1. Channel 阻塞

```go
ch := make(chan int, 1)
ch <- 1           // 缓冲区满了
ch <- 2           // ← G 调用 gopark()，被 channel.sendq 收养

ch2 := make(chan int)
<-ch2             // 没人发送，G 调用 gopark()，被 channel.recvq 收养
```

**收养结构：** channel 的 `sendq` 或 `recvq`（sudog 链表）

```
         channel
  ┌─────────────────────┐
  │ buf: [1]            │  ← 环形缓冲区（已满）
  │ sendq: sudog → sudog│  ← 等待发送的 G 链表
  │   ↓G2      ↓G5     │
  └─────────────────────┘

另一个 goroutine: <- ch
  → 从 buf 取出 1
  → 发现 sendq 有等待者 G2
  → 把 G2 的数据放入 buf
  → 调用 goready(G2) → G2 放回运行队列头部
```

---

### 2. Mutex 竞争

```go
var mu sync.Mutex
mu.Lock()         // G1 拿到锁
// 另一个 goroutine
mu.Lock()         // G2 拿不到锁 → gopark()，被 Mutex.waiters 收养
```

**收养结构：** Mutex 的 `waiters`（sudog 链表，实际上是个队列）

```
         Mutex
  ┌─────────────────────┐
  │ state: locked       │
  │ waiters: sudog → sudog│ ← 等待锁的 G 链表
  │   ↓G2      ↓G3     │
  └─────────────────────┘

G1 调用 Unlock():
  → 发现 waiters 有等待者 G2
  → 唤醒 G2: goready(G2)
  → G2 被放到运行队列头部
```

---

### 3. WaitGroup

```go
var wg sync.WaitGroup
wg.Add(1)
go func() { defer wg.Done() }()
wg.Wait()         // 计数器不为0 → gopark()，被 WaitGroup.waiters 收养
```

**收养结构：** WaitGroup 的 `waiters` 列表

**唤醒：** 最后一个 `Done()` 使计数器归零，调用 `goready()` 唤醒所有等待者

---

### 4. time.Sleep

```go
time.Sleep(5 * time.Second)
// G 调用 gopark()，被 timer 堆收养
```

**收养结构：** timer 堆（四叉堆，按触发时间排序）

```
         Timer Heap（四叉堆）
              t1(2s)
           /   |   \   \
        t2(3s) t3(4s) t4(5s) t5(6s)
          ↑
        G1 的 timer

sysmon / 其他 M 检查 timer：
  → 2s 到了 → 触发 t1 → goready(G1) → G1 回到运行队列
```

---

### 5. Cond（条件变量）

```go
cond.Wait()       // gopark()，被 Cond.notifyList 收养
cond.Signal()     // 唤醒一个等待者
cond.Broadcast()  // 唤醒所有等待者
```

**收养结构：** Cond 的 `notifyList`

---

### 6. 网络 IO（netpoll）

```go
conn.Read(buf)    // socket 没数据 → 底层调 gopark()，被 pollDesc 收养
conn.Write(data)  // 写缓冲区满 → gopark()，被 pollDesc 收养
```

**收养结构：** `pollDesc`（底层由 epoll/io_uring 管理）

**唤醒：** epoll_wait 检测到 IO 就绪 → 调用 `goready()` 唤醒等待的 G

**注意：** 这是 Go 网络编程高性能的关键——网络 IO 不走 entersyscall，而是通过 netpoll 在用户态管理！

---

### 7. 其他场景

| 场景 | 收养结构 | 唤醒方 |
|------|---------|--------|
| `sync.Once`（等待首次执行） | Once 内部等待者 | 首次执行完成 |
| GC STW | GC 等待机制 | GC 完成 |
| `runtime.Gosched()` | **无**（主动让出，G 直接放回运行队列尾部，不走 gopark） | 不需要唤醒，G 一直是 `_Grunnable` |

---

## 三、gopark vs entersyscall：两种阻塞的本质区别

### 1. 一张图看懂

```
╔══════════════════════════════════════════════════════════════╗
║                    Go G 的两种阻塞方式                        ║
╠═════════════════════════════╦════════════════════════════════╣
║     gopark() _Gwaiting      ║    entersyscall() _Gsyscall    ║
╠═════════════════════════════╬════════════════════════════════╣
║                             ║                                ║
║  G 阻塞，M 释放继续跑        ║   G 和 M 一起进内核阻塞         ║
║                             ║                                ║
║  ┌───┐  ┌───┐              ║   ┌───┐  ┌───┐                ║
║  │ G1│  │ G2│ 在运行队列    ║   │ G1│  │ G2│ 在运行队列      ║
║  └───┘  └───┘              ║   └───┘  └───┘                ║
║    │                        ║     │                          ║
║  gopark()                   ║   entersyscall()               ║
║    ↓                        ║     ↓                          ║
║  G1 离开运行队列             ║   G1 留在 M 身上              ║
║  M 继续调度 G2              ║   M 和 G1 一起阻塞             ║
║                             ║   P 可能被 sysmon 抢走          ║
║  ┌──────────────┐           ║                                ║
║  │  数据结构收养  │          ║   ┌─────────────────────┐     ║
║  │  channel.sendq│          ║   │ M0(阻塞) ─ G1(阻塞)  │     ║
║  │  Mutex.waiters│          ║   │      ↑               │     ║
║  │  timer堆      │          ║   │   内核态阻塞          │     ║
║  │  pollDesc     │          ║   └─────────────────────┘     ║
║  └──────────────┘           ║                                ║
║    │                        ║     │                          ║
║  条件满足                    ║   系统调用返回                  ║
║    ↓                        ║     ↓                          ║
║  goready(G1)                ║   exitsyscall()                ║
║  G1 回到运行队列头部         ║   找空闲P或G1放全局队列          ║
║                             ║                                ║
╚═════════════════════════════╩════════════════════════════════╝
```

### 2. 详细对比表

| | gopark (_Gwaiting) | entersyscall (_Gsyscall) |
|---|---|---|
| **谁阻塞了** | 只有 G 阻塞 | G 和 M 一起阻塞 |
| **M 状态** | M 释放，切到 g0 栈继续调度 | M 跟着 G 一起进内核阻塞 |
| **P 状态** | `_Prunning`，不受影响 | `_Psyscall`，可能被 sysmon 抢走 |
| **G 在哪里** | 在某个数据结构的等待队列里 | 跟着阻塞的 M，不在任何队列 |
| **谁负责唤醒** | 收养 G 的数据结构调 `goready()` | 内核系统调用返回，M 调 `exitsyscall()` |
| **G 回到队列的位置** | **头部**（`runqput(next=true)`） | **全局队列或拿空闲P继续** |
| **典型场景** | channel、Mutex、Sleep、WaitGroup、网络IO | read()、write()、accept()、sync 文件 IO |
| **是否需要 sysmon 介入** | 不需要 | 可能需要（P 被抢走时） |

### 3. 为什么网络 IO 走 gopark 而不是 entersyscall？

这是 Go 高性能网络编程的核心设计：

```go
// Go 的网络 IO（如 net包）不走 entersyscall！
// 而是通过 netpoll + gopark 实现：

conn.Read(buf)
  → 内部调用 runtime.pollWait()
  → 如果没有数据，调用 gopark()  → G 进入 _Gwaiting
  → G 被 pollDesc 收养
  → M 释放，继续跑其他 G
  → epoll 检测到 IO 就绪 → goready() 唤醒 G

// 这就是 Go 能轻松支撑百万连接的秘诀！
// 每个连接只是一个 G（2KB），阻塞时 M 不跟着阻塞
```

**对比传统模型：**
- Java NIO：一个线程阻塞在 epoll_wait，事件来了分发给 handler
- Go netpoll：每个连接一个 G，G 阻塞时 M 不阻塞，M 继续服务其他 G

---

## 四、goready 放回队列的位置

| 场景 | 放回位置 | runqput 参数 | 原因 |
|------|---------|-------------|------|
| `goready()` 唤醒 | **头部** | `next=true` | 刚被唤醒，优先执行 |
| `go func()` 新建 | **尾部** | `next=false` | 新 G 排队等候 |
| 时间片抢占 | **尾部** | — | 用完了重新排队 |

```go
// goready 源码
func goready(gp *g, traceskip int) {
    systemstack(func() {
        ready(gp, traceskip, true)  // next=true → 头部
    })
}

func ready(gp *g, traceskip int, next bool) {
    casgstatus(gp, _Gwaiting, _Grunnable)
    runqput(_p_.ptr(), gp, next)  // next=true → 放头部
}
```

---

## 五、面试高频问答

| 问题 | 答案 |
|------|------|
| gopark 后 G 在哪里？ | 不在 P 的运行队列里，被触发阻塞的数据结构（channel/Mutex/timer 等）收养 |
| gopark 和 entersyscall 的区别？ | gopark 只阻塞 G，M 继续跑；entersyscall G 和 M 一起进内核阻塞 |
| goready 放回队列头部还是尾部？ | 头部（`next=true`），优先执行 |
| 为什么 Go 网络 IO 不走系统调用阻塞？ | 用 netpoll + gopark，G 阻塞但 M 不阻塞，一个 M 能服务大量连接 |
| time.Sleep 的 G 被谁收养？ | timer 堆（四叉堆），定时器触发时 goready |
| Mutex 竞争时 G 被谁收养？ | Mutex 的 waiters 队列，Unlock 时 goready |
| 如果 gopark 后没人收养 G 会怎样？ | G 永远丢失，goroutine 泄漏！这是 channel 泄漏的根本原因 |

---

## 六、一句话总结

> **gopark() = G 离开运行队列，被某个数据结构收养，条件满足时 goready() 放回队列头部；entersyscall() = G 跟 M 一起进内核阻塞，P 可能被 sysmon 抢走，返回时 exitsyscall() 找空闲 P 或放全局队列。**
