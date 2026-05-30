# 计算机网络基础 — DNS、CDN、负载均衡等面试考点

来源：计算机网络教材 + 面试高频题整理

---

## 一、网络分层模型

### OSI 七层 vs TCP/IP 四层

| OSI 七层 | TCP/IP 四层 | 协议/设备 | 面试重点 |
|----------|------------|-----------|----------|
| 应用层 | 应用层 | HTTP、DNS、FTP、SMTP | 最多考察 |
| 表示层 | 应用层 | TLS/SSL、JPEG、ASCII | 和 HTTPS 结合考 |
| 会话层 | 应用层 | RPC、NetBIOS | 很少单独考 |
| 传输层 | 传输层 | TCP、UDP | 高频考点 |
| 网络层 | 网络层 | IP、ICMP、路由器 | IP 分片、路由 |
| 数据链路层 | 网络接口层 | 以太网、ARP、交换机 | ARP 欺骗 |
| 物理层 | 网络接口层 | 网线、光纤、集线器 | 基本不考 |

**面试答法**：「七层模型背不下来没关系，记住 TCP/IP 四层：应用层→传输层→网络层→网络接口层。数据从应用层往下每层加头部，到对端再逐层解头。」

### 数据封装过程

```
应用层数据
→ [TCP 头] + 数据              （传输层段 Segment）
→ [IP 头] + [TCP 头] + 数据     （网络层包 Packet）
→ [帧头] + [IP 头] + [TCP 头] + 数据 + [帧尾]  （链路层帧 Frame）
```

---

## 二、DNS 解析

### 解析流程

**面试常问**：「输入 URL 后发生了什么？」的第一步。

```
浏览器 DNS 缓存 → OS DNS 缓存 → 本地 DNS 服务器（递归查询）
                                    → 根 DNS 服务器（.com 在哪？）
                                    → .com 顶级域 DNS（example.com 在哪？）
                                    → 权威 DNS 服务器（example.com 的 IP 是？）
                                    → 返回 IP，逐级缓存
```

### 递归查询 vs 迭代查询

| 类型 | 谁负责追问 | 适用场景 |
|------|-----------|----------|
| 递归查询 | 本地 DNS 服务器替客户端问到底 | 客户端→本地 DNS |
| 迭代查询 | 本地 DNS 服务器自己去一级一级问 | 本地 DNS→根/顶级域/权威 DNS |

### DNS 记录类型

| 类型 | 含义 | 例子 |
|------|------|------|
| A | 域名→IPv4 | `example.com → 93.184.216.34` |
| AAAA | 域名→IPv6 | `example.com → 2606:2800:220:1:...` |
| CNAME | 域名别名 | `www.example.com → example.com` |
| MX | 邮件服务器 | `example.com → mail.example.com` |
| NS | DNS 服务器 | `example.com → ns1.dnsprovider.com` |
| TXT | 文本记录（SPF、DKIM 等） | 验证域名所有权 |

### DNS 面试追问

**Q：「DNS 用 TCP 还是 UDP？」**

> 默认用 UDP（53 端口），因为 DNS 查询通常很小（<512 字节），UDP 快。但当响应超过 512 字节（如区域传输 AXFR）时切换为 TCP。DNSSEC 引入后响应变大，TCP 使用增加。

**Q：「DNS 劫持和 DNS 污染的区别？」**

> DNS 劫持：中间人篡改 DNS 响应（如运营商广告注入），可以通过 DNSSEC 防御。DNS 污染：向 DNS 查询注入伪造响应，比劫持更难防御（攻击者不修改正常响应，而是抢先返回伪造响应）。

---

## 三、CDN（Content Delivery Network）

### 核心思想

> 把内容缓存到离用户最近的边缘节点，用户访问时从最近的节点取数据，减少延迟和源站压力。

### 工作流程

```
用户请求 → DNS 解析（CNAME 指向 CDN）→ CDN 全局负载均衡（GSLB）
→ 选择最近的边缘节点 → 命中缓存直接返回 → 未命中则回源
```

### CDN 的两种推送方式

| 方式 | 原理 | 适用场景 |
|------|------|----------|
| Pull（拉取） | 用户请求时边缘节点没有→回源取→缓存 | 大多数场景 |
| Push（推送） | 源站主动推到边缘节点 | 大文件预热、直播流 |

### CDN 面试考点

**Q：「CDN 能加速动态接口吗？」**

> 传统 CDN 只缓存静态资源。但现在的 CDN 提供动态加速（DCDN）：通过优化路由（选最快的网络路径）、TCP 优化、压缩等技术加速动态请求，但不缓存响应。

**Q：「CDN 缓存和浏览器缓存的关系？」**

> 浏览器缓存是第一层（本地），CDN 缓存是第二层（边缘节点）。请求链路：浏览器缓存→CDN 缓存→源站。Cache-Control 头对两层都生效。

---

## 四、负载均衡

### 四层 vs 七层负载均衡

| 维度 | 四层（L4） | 七层（L7） |
|------|-----------|-----------|
| 工作层 | 传输层 | 应用层 |
| 分发依据 | IP + 端口 | URL、Header、Cookie 等 |
| 速度 | 快（只看网络层信息） | 慢（要解析应用层协议） |
| 功能 | 简单转发 | 内容路由、限流、认证 |
| 例子 | LVS、AWS NLB | Nginx、HAProxy、AWS ALB |

**面试答法**：

> 四层基于 IP+端口做转发，不解析应用层，快但功能少。七层解析 HTTP 头部，可以按 URL 路径、Cookie 做路由，功能多但慢。实际架构通常两者结合：四层在最前面扛流量，七层在后面做精细路由。

### 负载均衡算法

| 算法 | 原理 | 适用场景 |
|------|------|----------|
| 轮询（Round Robin） | 依次分配 | 服务器性能相近 |
| 加权轮询 | 按权重分配 | 服务器性能不同 |
| 最少连接 | 分给当前连接最少的 | 长连接场景 |
| IP Hash | 同一 IP 固定分配到同一后端 | 需要会话保持 |
| 一致性 Hash | Hash 环，增删节点只影响相邻节点 | 缓存场景 |
| 随机 | 随机分配 | 简单场景 |

### 一致性 Hash 详解

**面试常问**：「一致性 Hash 是什么？解决什么问题？」

**普通 Hash 的问题**：

```
hash(key) % N → server_index
```

节点数 N 变化时（扩容/缩容），几乎所有 key 的映射都变了→缓存雪崩。

**一致性 Hash**：

1. 把 0~2^32-1 的整数空间连成环
2. 每个服务器节点 Hash 到环上
3. 每个 key 顺时针找到第一个节点就是它的归属节点
4. 增删节点只影响相邻节点的 key

**虚拟节点**：解决节点少时分布不均匀的问题——每个真实节点映射多个虚拟节点到环上。

---

## 五、ARP 协议

### 工作原理

```
主机 A 要和主机 B 通信（知道 B 的 IP）：
1. A 查 ARP 缓存，没有 B 的 MAC
2. A 广播 ARP 请求：「谁有 192.168.1.2？告诉我你的 MAC」
3. B 单播 ARP 应答：「192.168.1.2 的 MAC 是 xx:xx:xx:xx:xx:xx」
4. A 缓存 B 的 MAC，开始通信
```

**面试追问**：「ARP 欺骗怎么防御？」

> ARP 欺骗：攻击者发伪造的 ARP 应答，把网关 IP 映射到攻击者 MAC→流量被中间人截获。防御：静态 ARP 绑定、ARP 防护工具、交换机端口安全（DHCP Snooping + Dynamic ARP Inspection）。

---

## 六、ICMP 协议

- Internet Control Message Protocol，网络层协议
- 用于传递网络控制和错误信息
- **ping**：发送 ICMP Echo Request，接收 Echo Reply
- **traceroute**：利用 TTL 递增，每一跳返回 ICMP Time Exceeded

**面试注意**：ICMP 不是传输层协议，是网络层协议，和 IP 同层。

---

## 七、从输入 URL 到页面显示的完整过程

面试经典大题，完整链路：

```
1. URL 解析（提取协议、域名、端口、路径）
2. DNS 解析（域名→IP，递归+迭代查询）
3. TCP 三次握手（建立连接）
4. TLS 握手（如果是 HTTPS，TLS 1.3 只需 1-RTT）
5. 发送 HTTP 请求（请求行 + 请求头 + 请求体）
6. 服务器处理请求，返回响应
7. 浏览器接收响应，解析 HTML
8. 构建 DOM 树 + CSSOM 树 → 渲染树 → 布局 → 绘制
9. 遇到 JS/CSS/图片等资源，并行下载（HTTP/2 多路复用）
10. TCP 四次挥手（如果 Connection: close）
```

**面试加分点**：

- 提到 DNS 缓存层级（浏览器→OS→本地 DNS）
- 提到 CDN 命中场景
- 提到 HTTP/2 的多路复用减少连接数
- 提到浏览器渲染的关键路径

---

## 八、网络安全基础

### 常见攻击

| 攻击 | 原理 | 防御 |
|------|------|------|
| XSS（跨站脚本） | 注入恶意 JS 到页面 | 转义输出、CSP、HttpOnly Cookie |
| CSRF（跨站请求伪造） | 借用户身份发恶意请求 | CSRF Token、SameSite Cookie |
| SQL 注入 | 拼接 SQL 语句 | 参数化查询（prepared statement） |
| 中间人攻击 | 截获篡改通信内容 | HTTPS、证书校验 |
| DDoS | 大量请求淹没服务 | 流量清洗、限流、CDN |
| DNS 劫持 | 篡改 DNS 响应 | DNSSEC、DoH/DoT |

### XSS vs CSRF

**面试常问**：「XSS 和 CSRF 的区别？」

> XSS 是往页面注入恶意脚本，在用户浏览器里执行，可以窃取 Cookie/Token。CSRF 是借用户已登录的身份发请求，用户不知道但请求合法（浏览器自动带 Cookie）。XSS 防输出转义，CSRF 防 Token 校验。

### HTTPS 能防什么不能防什么

**能防**：窃听（加密）、篡改（完整性校验）、冒充（证书验证）

**不能防**：XSS（应用层漏洞）、CSRF（请求本身合法）、DDoS（请求可以正常加密）、服务端漏洞

---

## 九、WebSocket

| 维度 | HTTP | WebSocket |
|------|------|-----------|
| 通信方向 | 请求-响应（单向） | 全双工（双向） |
| 连接 | 短连接/长连接 | 持久连接 |
| 开销 | 每次请求带完整头部 | 升级后帧头很小（2-10 字节） |
| 适用场景 | 请求-响应模式 | 实时推送、聊天、行情 |

**握手过程**（基于 HTTP 升级）：

```
GET /ws HTTP/1.1
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13

---

HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

---

## 十、SSE（Server-Sent Events）

### 核心概念

> SSE 是基于 HTTP 的**单向推送**机制——服务器可以持续向客户端推送数据，但客户端只能通过 HTTP 请求发送数据，不能在 SSE 连接上双向通信。

### SSE vs WebSocket vs 长轮询

这是面试高频对比题：

| 维度 | SSE | WebSocket | 长轮询 |
|------|-----|-----------|--------|
| 通信方向 | **单向**（服务器→客户端） | 双向 | 单向（每次请求一轮） |
| 协议 | 普通 HTTP | 独立协议（ws://） | 普通 HTTP |
| 数据格式 | **文本**（UTF-8） | 文本或二进制 | 任意 |
| 连接管理 | 自动重连（浏览器内置） | 需手动重连 | 每次重新请求 |
| 复杂度 | 极低 | 中等（协议握手、帧解析） | 低 |
| 浏览器支持 | 除 IE 外全支持 | 全支持 | 全支持 |
| 连接数限制 | HTTP/1.1 下受 6 连接限制 | 无限制 | 受 HTTP 连接限制 |
| 适用场景 | 通知推送、实时日志、股票行情 | 聊天、游戏、协同编辑 | 兼容性要求高的场景 |

### SSE 的协议格式

SSE 响应的 Content-Type 是 `text/event-stream`，数据用简单的文本格式：

```
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

field: value\n
field: value\n
\n          ← 两个连续换行表示一个事件结束
```

**支持的字段**：

| 字段 | 含义 | 示例 |
|------|------|------|
| `data` | 事件数据（可多行） | `data: {"price": 100}` |
| `event` | 事件类型（默认 message） | `event: update` |
| `id` | 事件 ID（断线重连用） | `id: 42` |
| `retry` | 重连间隔（毫秒） | `retry: 3000` |

**示例**：服务器推送一条消息

```
id: 1
event: price_update
data: {"symbol": "AAPL", "price": 178.5}

id: 2
event: price_update
data: {"symbol": "GOOG", "price": 141.2}

```

### SSE 的断线自动重连

**面试加分点**：这是 SSE 比 WebSocket 最独特的优势之一。

1. 浏览器内置 EventSource API，连接断开后**自动重连**
2. 服务器通过 `id` 字段标记每个事件
3. 重连时浏览器自动带上 `Last-Event-ID` 头：`Last-Event-ID: 42`
4. 服务器据此从断点继续推送，不丢数据

```javascript
// 前端极简用法
const es = new EventSource('/api/stream');

es.onmessage = (e) => {
    console.log(e.data);  // 收到 data 字段内容
};

es.addEventListener('price_update', (e) => {
    console.log(JSON.parse(e.data));
});

es.onerror = () => {
    // 连接断开，浏览器会自动重连，不需要手动处理
    // 如果服务器返回非 200，浏览器会停止重连
};
```

### Go 实现 SSE 服务端

```go
func handleSSE(w http.ResponseWriter, r *http.Request) {
    // 设置 SSE 必需的头部
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    // 支持跨域
    w.Header().Set("Access-Control-Allow-Origin", "*")

    // 检查客户端是否断连
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    // 从 Last-Event-ID 恢复（断线重连）
    lastID := r.Header.Get("Last-Event-ID")

    // 持续推送
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    eventID := 0
    if lastID != "" {
        eventID = parseID(lastID)  // 从断点恢复
    }

    for {
        select {
        case <-ticker.C:
            eventID++
            fmt.Fprintf(w, "id: %d\n", eventID)
            fmt.Fprintf(w, "event: price_update\n")
            fmt.Fprintf(w, "data: %s\n\n", getPriceJSON())
            flusher.Flush()  // 重要！必须 Flush 才能立即发送

        case <-r.Context().Done():
            // 客户端断连
            log.Println("client disconnected")
            return
        }
    }
}
```

**关键点**：
- `http.Flusher` 必须调用，否则数据攒在缓冲区里不会发送
- 用 `r.Context().Done()` 检测客户端断连（Go 1.20+ net/http 自动取消 context）
- `id` 字段实现断线重连，客户端重连带 `Last-Event-ID`

### SSE 的实际应用场景

| 场景 | 为什么用 SSE 而不是 WebSocket |
|------|------|
| 通知推送（订单状态变更） | 只需要服务端→客户端单向推送 |
| 实时日志/监控面板 | 数据流式展示，不需要客户端回传 |
| 股票行情 | 频繁推送，浏览器自动重连保证不丢数据 |
| AI 对话流式输出（ChatGPT 式） | 流式返回文本，SSE 比 WebSocket 更简单 |
| GitOps 状态更新 | 事件驱动，单向推送即可 |

**面试追问**：「为什么 ChatGPT 用 SSE 而不是 WebSocket？」

> ChatGPT 的流式输出是**单向推送**场景——服务端持续吐 token，客户端只接收展示。用 SSE：1）更简单，不需要 WebSocket 的握手和帧协议；2）基于 HTTP，天然兼容各种代理和 CDN；3）浏览器自动重连保证对话不中断；4）每个请求是一个独立的 SSE 流，比 WebSocket 的长连接更易于扩缩容。

### SSE 的限制与注意事项

1. **HTTP/1.1 连接数限制**：浏览器对同一域名最多 6 个 HTTP 连接，SSE 占一个。如果开 6 个 SSE 就无法再发普通 HTTP 请求。HTTP/2 没有此限制（多路复用）。
2. **只能推文本**：不能推二进制数据（如文件、图片），需要 Base64 编码。
3. **单向通信**：客户端要发数据只能另起 HTTP 请求。
4. **IE 不支持**：老项目需要用 polyfill 或改用长轮询。

### SSE 和 HTTP/2 的关系

**面试加分**：HTTP/2 下 SSE 更好用——

> HTTP/1.1 每个 SSE 连接占用一个 TCP 连接（浏览器限制 6 个），HTTP/2 多路复用让所有 SSE 流共享一个 TCP 连接，不再有连接数瓶颈。这也是为什么实际项目中 SSE + HTTP/2 组合非常流行。

---

## 十一、GET vs POST

面试送分题但很多人答不精确：

| 维度 | GET | POST |
|------|-----|------|
| 语义 | 获取资源 | 创建/提交资源 |
| 参数位置 | URL 查询字符串 | 请求体 |
| 幂等性 | 幂等 | 非幂等 |
| 缓存 | 可缓存 | 不缓存 |
| 浏览器历史 | 保留参数 | 不保留 |
| 长度限制 | 浏览器和服务器有限制 | 无限制（理论上） |
| 安全性 | 参数暴露在 URL | 在请求体中（但 HTTPS 下都不裸露） |

**面试注意**：

- GET 和 POST 本质都是 HTTP 请求，技术上 GET 也能有 body、POST 也能有查询参数
- 「GET 不安全 POST 安全」是不准确的——HTTPS 下都加密，HTTP 下都裸露
- 真正的区别是**语义**：GET 是幂等的读操作，POST 是非幂等的写操作

---
