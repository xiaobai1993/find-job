# K8s 集群网络排障实战指南：从抓包到 Istio 流量劫持

> 🎯 覆盖 K8s 网络全链路：DNS、iptables、Istio Sidecar、连接池、长连接
> ⭐ 面试官问：服务间调用慢怎么排查？你会怎么一步步分析？

---

## 📋 故事背景：服务间调用为什么这么慢？

### 面试官问：线上服务调用变慢了，你怎么排查？

> **标准答案：**
>
> "这是一个非常经典的问题，我会按照「从外到内、从易到难」的步骤来排查：
>
> **第一步：先看监控**
> - QPS、延迟、错误率有没有异常？
> - CPU、内存、网络带宽有没有打满？
>
> **第二步：网络层面排查**
> - 是集群内调用还是跨集群？
> - DNS 解析慢不慢？
> - TCP 建连有没有问题？
> - 有没有丢包、重传？
>
> **第三步：中间件排查**
> - 有没有经过网关？
> - 有没有经过 Service Mesh（Istio）？
> - Sidecar 的 CPU、内存有没有问题？
>
> **第四步：应用层面排查**
> - 连接池配置对不对？
> - 长连接有没有复用？
> - 有没有线程/协程阻塞？
>
> **第五步：抓包兜底**
> - tcpdump/Wireshark 抓包，看看到底慢在哪个环节
>
> 我在美餐做过一次非常经典的 K8s 网络排障，最终发现是 Istio 的连接池配置有问题，
> 调完之后 QPS 直接翻倍！"

---

## 🔴 实验一：Istio 连接池的巨大影响

### 一个参数让 QPS 差了一倍！

> **我们做了一个对照实验：**
>
> **场景：** Client Pod → Server Pod，都注入了 Istio Sidecar
> **压测工具：** wrk -t2 -c50 -d30s
>
> ---
>
> **实验组 A：连接池限制很严**
> ```yaml
> trafficPolicy:
>   connectionPool:
>     tcp:
>       maxConnections: 1       # 最多 1 条连接！
>     http:
>       maxRequestsPerConnection: 1  # 每条连接最多发 1 个请求！
> ```
>
> **结果：**
> ```
> Latency    104.76ms   # 延迟 100ms+
> Requests/sec:  478.67  # QPS 只有 478
> ```
>
> ---
>
> **实验组 B：不加限制**
> ```yaml
> trafficPool: {}  # 空的，用默认值
> ```
>
> **结果：**
> ```
> Latency    55.07ms   # 延迟降了一半！
> Requests/sec:  944.55  # QPS 几乎翻倍！
> ```
>
> ---
>
> **为什么差别这么大？**
>
> 当 `maxRequestsPerConnection = 1` 的时候：
> 1. 发一个请求
> 2. 等响应回来
> 3. 关闭连接
> 4. 重新建连
> 5. 发下一个请求
>
> 相当于完全变成了短连接！TCP 三次握手、四次挥手的开销全部都要算进去！
>
> 👉 **这是 Istio 最容易踩的坑之一！很多人为了「安全」把连接池设得很小，结果性能雪崩！**

---

## 🟠 实验二：长连接 vs 短连接

### KeepAlive 到底有多重要？

> **集群内调用对比：**
>
> **开 KeepAlive（默认）：**
> ```
> Latency    11-13ms
> Requests/sec:  780-940
> ```
>
> **关 KeepAlive（每次新建连接）：**
> ```
> Latency    15.33ms    # 延迟涨了 30%！
> Requests/sec:  660.45 # QPS 降了 20%！
> Socket errors: timeout 3  # 还出现了超时！
> ```
>
> ---
>
> **跨集群调用对比：**
>
> **开 KeepAlive：**
> ```
> Latency    11-12ms
> Requests/sec:  860-919
> ```
>
> **关 KeepAlive：**
> ```
> Latency    15.33ms → 延迟涨了 30%！
> Requests/sec:  660 → QPS 降了 28%！
> ```
>
> **结论：**
> 不管是集群内还是跨集群，长连接（KeepAlive）的性能优势巨大！
> 尤其是高 QPS 场景，一定要开长连接！

---

## 🟡 Istio Sidecar 流量劫持原理

### 包到底是怎么跑到 Envoy 里的？

> **很多人用了很久 Istio，都不知道流量是怎么被劫持的。**
>
> **真相就是：iptables！**
>
> 看这个真实的 iptables 规则：
> ```iptables
> *nat
> :PREROUTING ACCEPT [0:0]
> :OUTPUT ACCEPT [0:0]
> :POSTROUTING ACCEPT [0:0]
> :ISTIO_INBOUND - [0:0]
> :ISTIO_IN_REDIRECT - [0:0]
> :ISTIO_OUTPUT - [0:0]
> :ISTIO_REDIRECT - [0:0]
>
> # 入站流量：全部转到 ISTIO_INBOUND 链
> -A PREROUTING -p tcp -j ISTIO_INBOUND
>
> # 出站流量：全部转到 ISTIO_OUTPUT 链
> -A OUTPUT -p tcp -j ISTIO_OUTPUT
>
> # 入站重定向：把所有流量转到 15006 端口（Envoy 监听的端口）
> -A ISTIO_IN_REDIRECT -p tcp -j REDIRECT --to-ports 15006
>
> # 出站重定向：把所有流量转到 15001 端口（Envoy 出站端口）
> -A ISTIO_REDIRECT -p tcp -j REDIRECT --to-ports 15001
> ```
>
> **完整的流量路径：**
> ```
> 你的应用发请求
>     ↓
>   iptables 拦截（你完全感知不到！）
>     ↓
>   重定向到本地 15001 端口
>     ↓
>   Envoy（Sidecar）收到请求
>     ↓
>   做路由、鉴权、限流、监控、mTLS 加密...
>     ↓
>   Envoy 把请求发给目标服务的 Sidecar
>     ↓
>   目标 Sidecar 解密、验证、记录监控...
>     ↓
>   重定向给目标应用
> ```
>
> 👉 **重点：整个过程应用层完全感知不到！**
> 你的代码以为是直接连对方，实际上中间经过了两次 Sidecar 转发！
>
> 这就是 Service Mesh 的「透明」代理的真谛！

---

## 🟢 实验三：加了 20ms 延迟之后

### 为什么一条连接 QPS 上不去？

> **我们做了一个极端实验：**
>
> **配置：**
> - maxConnections: 1（只有一条连接）
> - 服务端故意加 20ms 延迟
>
> **压测结果：**
> ```
> Latency   564.21ms    # 延迟暴涨到 500ms+！
> Requests/sec:  87.76  # QPS 直接降到不到 100！
> ```
>
> **为什么会这样？**
>
> 一条 TCP 连接，HTTP/1.1 是串行的！
> ```
> 发请求 → 等 20ms → 收响应 → 发下一个请求 → 等 20ms → ...
> ```
>
> 理论极限 QPS = 1000ms / 20ms = 50 QPS
> 实际测出来 87 QPS，因为 HTTP/2 可以多路复用，稍微好一点，但还是远远不够。
>
> **解决方法：**
> 1. 多开几条连接（maxConnections 调大）
> 2. 用 HTTP/2 多路复用
> 3. 连接池配置要和 QPS 匹配
>
> 👉 **这个是面试常考题：「单条 TCP 连接 QPS 上限是多少？」**
> 答案就是：1000ms / 平均延迟！

---

## 🔵 DNS 排查：被忽略的性能杀手

### CoreDNS 的 ndots 坑

> **这是 K8s 最坑的配置之一，90% 的人都不知道！**
>
> 看你的 Pod 里的 `/etc/resolv.conf`：
> ```
> nameserver 10.96.0.10
> search default.svc.cluster.local svc.cluster.local cluster.local
> options ndots:5
> ```
>
> **`ndots:5` 是什么意思？**
>
> 域名里的点少于 5 个，就会走 search 域尝试！
>
> 比如你访问 `baidu.com`，只有 1 个点，小于 5：
> 1. 先试 `baidu.com.default.svc.cluster.local` → 失败
> 2. 再试 `baidu.com.svc.cluster.local` → 失败
> 3. 再试 `baidu.com.cluster.local` → 失败
> 4. 最后才试 `baidu.com` → 成功
>
> **白白多做了 3 次 DNS 查询！延迟直接涨几倍！**
>
> **解决方法：**
> 1. 访问外部域名加末尾的点：`baidu.com.`（注意最后那个点！）
> 2. 把 ndots 改成 2 或者 1
> 3. 外部服务用 ServiceEntry 注册到集群内，走内部域名
>
> 👉 **很多服务「莫名其妙延迟高」，根源就是这个 ndots 配置！**

---

## 🎯 面试总结：K8s 网络排障方法论

### 面试官问：做了这么多网络优化，你总结出什么经验？

> **答：**
>
> "我总结了四条经验：
>
> **第一：永远不要小看连接池配置**
> Istio 的连接池、Golang 的 http.Transport、数据库连接池...
> 这些参数配置错了，性能直接差一个数量级。
> 不要凭感觉配，要压测，要算理论值。
>
> **第二：长连接是性能的朋友**
> 能复用连接就复用，不要每次都建连。
> TCP 三次握手、TLS 握手的开销在高 QPS 下是致命的。
>
> **第三：Sidecar 不是免费的**
> Istio 给了你 mTLS、可观测性、流量治理...
> 但这些都是有代价的：每个请求多了两次转发，延迟涨 1-5ms。
> 高并发场景要评估 Sidecar 的开销，核心路径甚至要考虑跳过 Sidecar。
>
> **第四：DNS 是最容易被忽略的瓶颈**
> ndots、CoreDNS  replicas、缓存策略...
> 这些小东西没配置好，整个集群的性能都上不去。
>
> 我们当时就是把这几个点都优化了一遍，
> 整个支付链路的 P99 延迟从 200ms 降到了 80ms，
> 用户体验提升非常明显！"

---

## 💡 给面试官的最后一句话

> **K8s 网络就像一个黑盒子：**
> 包发出去了，你不知道它经过了多少道关卡，
> iptables、Sidecar、kube-proxy、路由器、防火墙...
> 任何一个环节出问题，都会体现为「服务慢了」。
>
> 好的工程师，就是能把这个黑盒子拆开，
> 一层一层剥开，找到那个真正的瓶颈。
>
> 🔥 **网络排障的本质，就是把「不知道慢在哪」变成「知道慢在哪」。**
