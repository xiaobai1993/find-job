# Linux I/O 模型与多路复用 — 后端面试核心考点

来源：操作系统教材 + Go runtime 源码 + 面试高频题整理

---

## 一、五种 I/O 模型

这是 Stevens 的经典分类，面试必考。用一个比喻讲清楚：

> 你去餐厅点餐，等厨师做好。

| 模型 | 比喻 | 系统调用 | 特点 |
|------|------|----------|------|
| 阻塞 I/O | 站在窗口干等，啥也不干 | recvfrom | 最简单，全程阻塞 |
| 非阻塞 I/O | 不断回来问「好了没？」，没好就干别的 | recvfrom（返回 EAGAIN） | 轮询浪费 CPU |
| I/O 多路复用 | 雇一个服务员同时等好几桌 | select/poll/epoll | 一个线程监控多个 fd |
| 信号驱动 I/O | 厨师做好了给你打电话 | sigaction + recvfrom | 异步通知，但数据拷贝还是同步 |
| 异步 I/O | 直接外卖送到家，你完全不用管 | aio_read | 真异步，Linux 上 aio 实现不完善 |

**面试关键区分**：

- 前四种模型的**数据拷贝阶段都是同步的**（recvfrom 要自己把数据从内核拷到用户态）
- 只有异步 I/O 的数据拷贝也是内核完成，完成后通知用户
- Linux 的 AIO 不完善，实际生产主要用 epoll（I/O 多路复用）

---

## 二、I/O 多路复用详解

### select

```c
int select(int nfds, fd_set *readfds, fd_set *writefds, fd_set *exceptfds, struct timeval *timeout);
```

| 限制 | 原因 |
|------|------|
| 最多监控 1024 个 fd | `FD_SETSIZE` 宏定义，改大需要重编译内核 |
| 每次调用要拷贝整个 fd_set | 从用户态拷到内核态，O(n) 开销 |
| 返回后要线性扫描所有 fd | 不知道哪个 fd 就绪了，O(n) 遍历 |
| 每次调用都要重新设置 fd_set | 内核会修改 fd_set，不能复用 |

### poll

```c
int poll(struct pollfd *fds, nfds_t nfds, int timeout);
```

- 用 `pollfd` 数组替代 fd_set，没有 1024 限制
- 但每次调用仍然：拷贝整个数组到内核→返回后线性扫描→O(n)
- fd 很多但活跃少时（如 C10K），效率很低

### epoll

```c
// 创建 epoll 实例
int epoll_create1(int flags);

// 注册/修改/删除 fd
int epoll_ctl(int epfd, int op, int fd, struct epoll_event *event);

// 等待就绪事件
int epoll_wait(int epfd, struct epoll_event *events, int maxevents, int timeout);
```

| 优势 | 原因 |
|------|------|
| 没有 fd 数量上限 | 内核维护红黑树，不依赖固定大小数组 |
| 只返回就绪的 fd | 不需要遍历所有 fd，O(活跃 fd 数) |
| 不需要每次重新设置 | `epoll_ctl` 增删改一次注册，`epoll_wait` 直接用 |
| 零拷贝（LT 模式） | 就绪 fd 列表在内核维护，不需要从用户态拷贝 fd 集合 |

### epoll 的两种触发模式

| 模式 | 行为 | 优缺点 |
|------|------|--------|
| LT（Level Triggered，水平触发） | fd 就绪时持续通知，直到数据读完 | 简单安全，默认模式 |
| ET（Edge Triggered，边缘触发） | fd 从不就绪→就绪只通知一次，之后不再通知 | 效率更高但必须一次性读完数据，否则漏事件 |

**ET 模式的坑**：必须用非阻塞 I/O + 循环读直到 EAGAIN，否则：
- 只读了一部分数据→内核认为你处理完了→不再通知→剩余数据丢失

**面试追问**：「为什么 nginx 用 ET 而 Redis 用 LT？」

> nginx 是高性能 Web 服务器，追求极致效率，ET 减少 epoll_wait 调用次数。Redis 是单线程模型，LT 更简单安全，不需要担心漏读。

---

## 三、select vs poll vs epoll 对比

| 维度 | select | poll | epoll |
|------|--------|------|-------|
| fd 上限 | 1024（FD_SETSIZE） | 无上限 | 无上限 |
| 内核数据结构 | bitmap | 数组 | 红黑树 + 就绪链表 |
| 返回就绪 fd | 全量返回，需遍历 | 全量返回，需遍历 | 只返回就绪的，O(1) 拷贝 |
| 每次调用开销 | O(n) 拷贝+扫描 | O(n) 拷贝+扫描 | O(活跃数) |
| fd 变更 | 每次重新设置 | 每次重新设置 | epoll_ctl 增删改 |
| 适用场景 | fd 少且活跃多 | fd 少 | fd 多但活跃少（C10K+） |

**面试答法**：从 C10K 问题开始讲——

> 早期服务器用 select/poll，fd 多时每次调用 O(n) 开销太大。epoll 用红黑树管理 fd 注册、双向链表管理就绪事件，把 O(n) 降到 O(活跃数)，解决了 C10K。Go runtime 在 Linux 上就是用 epoll 做 net poller。

---

## 四、Go runtime 的 net poller

Go 把 epoll 封装在 runtime 里，对用户完全透明。

**工作流程**：

1. goroutine 调用 `net.Read()` → 发现 fd 没数据
2. goroutine 把 fd 注册到 net poller（epoll），然后**挂起**（gopark）
3. epoll_wait 返回就绪 fd → runtime 把对应 goroutine 放回 P 的本地队列
4. goroutine 被调度执行 → 继续读数据

**面试加分点**：

- 这就是为什么 Go 的网络 I/O 不会阻塞操作系统线程
- 一个 M 上的 goroutine 做网络 I/O 时，不会卡住 M，M 可以继续跑其他 G
- `runtime/netpoll.go` 里封装了 epoll（Linux）/ kqueue（macOS）/ wepoll（Windows）
- poller 在 sysmon（系统监控线程）里定期调用，也在 schedule() 里顺手检查

---

## 五、零拷贝技术

面试常问：「如何减少 I/O 的数据拷贝次数？」

### 传统 read + write 的 4 次拷贝

```
磁盘 → 内核页缓存 → 用户缓冲区 → socket 内核缓冲区 → 网卡
      (DMA拷贝)      (CPU拷贝)     (CPU拷贝)       (DMA拷贝)
```

4 次拷贝 + 2 次 CPU 参与 + 4 次上下文切换（read 用户→内核→用户，write 用户→内核→用户）

### mmap（3 次拷贝）

```
磁盘 → 内核页缓存 → 直接映射到用户空间 → socket 内核缓冲区 → 网卡
      (DMA拷贝)      (映射，不拷贝)     (CPU拷贝)       (DMA拷贝)
```

省掉了从内核缓冲区到用户缓冲区的拷贝。

### sendfile（2 欍拷贝，Linux 2.4+）

```
磁盘 → 内核页缓存 → socket 内核缓冲区 → 网卡
      (DMA拷贝)     (CPU拷贝→或DMA scatter-gather直接到网卡)
```

完全在内核态完成，数据不经过用户态。Kafka、Nginx 都用 sendfile 做文件传输。

### splice

类似 sendfile，但两个 fd 都可以是 pipe，数据在内核管道缓冲区之间移动，完全不经过用户态。

**面试答法**：

> 零拷贝不是「不拷贝」，而是「减少 CPU 参与的拷贝」。理想状态是数据只在内核态流动（DMA 拷贝），用户态只负责下发指令。mmap 省一次、sendfile 省两次、splice 更彻底。

---

## 六、文件描述符（fd）

| 要点 | 说明 |
|------|------|
| 定义 | 内核维护的进程级打开文件表索引，非负整数 |
| 默认上限 | 1024（`ulimit -n` 查看，可调大） |
| 系统上限 | `/proc/sys/fs/file-max`，通常几十万 |
| 0/1/2 | stdin/stdout/stderr，进程启动就有 |
| fork 继承 | 子进程继承父进程所有 fd（这是管道通信的基础） |
| close-on-exec | exec 替换进程映像时自动关闭标记的 fd |

**面试追问**：「C10K 问题是什么？怎么解决？」

> 一台服务器同时服务 1 万个客户端连接。select 的 1024 fd 上限直接不行，poll O(n) 遍历 1 万 fd 太慢。epoll 的 O(活跃数) + 无上限解决了这个问题。现在 C10M 都可以实现（通过内核 bypass 如 DPDK）。

---

## 七、Buffered I/O vs Direct I/O

| 方式 | 特点 | 场景 |
|------|------|------|
| Buffered I/O（标准 I/O） | 数据经过内核页缓存 | 大多数文件操作，利用缓存加速 |
| Direct I/O | 数据绕过页缓存，直接到用户缓冲区 | 数据库（MySQL InnoDB）、自管理缓存的应用 |
| AIO（异步 I/O） | 提交 I/O 请求后立即返回，完成后通知 | Linux aio 不完善，libaio 只支持 Direct I/O |

**面试追问**：「为什么数据库喜欢用 Direct I/O？」

> 数据库有自己的缓存管理（如 InnoDB Buffer Pool），如果还走内核页缓存就是双重缓存浪费内存。Direct I/O 绕过页缓存，让数据库自己决定什么缓在内存、什么换出去。

---