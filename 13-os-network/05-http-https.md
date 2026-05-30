# HTTP/HTTPS — 后端面试核心考点

来源：HTTP 权威指南 + RFC 文档 + 面试高频题整理

---

## 一、HTTP 版本对比

| 特性 | HTTP/1.0 | HTTP/1.1 | HTTP/2 | HTTP/3 |
|------|----------|----------|--------|--------|
| 连接方式 | 短连接 | 长连接（Keep-Alive） | 多路复用 | 多路复用 |
| 请求方式 | 串行（队头阻塞） | 串行（管道化可选但基本不用） | 并行（帧+流） | 并行 |
| 头部压缩 | 无 | 无 | HPACK | QPACK |
| 服务器推送 | 无 | 无 | 有 | 有 |
| 传输层 | TCP | TCP | TCP | **QUIC（UDP）** |
| 队头阻塞 | 应用层+传输层 | 应用层+传输层 | 传输层（TCP） | 无（QUIC 独立流） |
| 连接建立 | 1 RTT | 1 RTT | 1 RTT（TCP）+1 RTT（TLS 1.2） | 0-1 RTT |

**面试答法**：从演进动力讲——

> HTTP/1.0 每个请求一个 TCP 连接太浪费→1.1 长连接复用但串行有队头阻塞→HTTP/2 多路复用解决应用层队头阻塞但 TCP 层还有→HTTP/3 用 QUIC 替 TCP 彻底解决队头阻塞 + 减少握手延迟。

---

## 二、HTTP/1.1 核心特性

### 长连接（Keep-Alive）

- 默认开启，`Connection: keep-alive`
- 一个 TCP 连接可以发多个请求
- 关闭：`Connection: close`

### 队头阻塞（Head-of-Line Blocking）

> HTTP/1.1 虽然长连接，但请求-响应是串行的。前面的请求慢了，后面全部排队等。这是 HTTP/2 要解决的核心问题。

### 管道化（Pipelining）

- 允许不等响应就发下一个请求
- 但响应必须按请求顺序返回→还是队头阻塞
- 浏览器基本不支持，实际没人用

### Host 头

- HTTP/1.1 必须带 Host 头→同一 IP 可以部署多个网站（虚拟主机）

---

## 三、HTTP/2 核心特性

### 1. 二进制帧

HTTP/1.1 是文本协议，HTTP/2 改成二进制帧：

```
Frame: | Length (3B) | Type (1B) | Flags (1B) | Stream ID (4B) | Payload |
```

帧类型：HEADERS、DATA、SETTINGS、PING、GOAWAY 等。

### 2. 多路复用

- 每个请求-响应是一个**流（Stream）**，有唯一 Stream ID
- 一个 TCP 连接上可以同时有多个流
- 每个流的帧可以交错发送，不再队头阻塞（应用层）
- 客户端发起的流 ID 为奇数，服务端为偶数

### 3. 头部压缩（HPACK）

- HTTP/1.1 每次请求都带完整头部（User-Agent、Cookie 等重复数据）
- HPACK：静态表（61 个常见头部）+ 动态表（记录之前出现过的头部）+ 哈夫曼编码
- 相同头部只传索引号，大幅减少重复数据

### 4. 服务器推送

- 服务端可以主动推资源给客户端（如 HTML 里引用的 CSS/JS）
- 客户端可以拒绝（RST_STREAM）

### HTTP/2 的队头阻塞问题

> HTTP/2 解决了应用层队头阻塞，但 TCP 层还有：一个 TCP 包丢了→所有流的所有帧都要等重传，因为 TCP 保证有序。这是 HTTP/3 用 QUIC 的核心原因。

---

## 四、HTTP/3 与 QUIC

### QUIC 核心改进

| 改进 | 说明 |
|------|------|
| 基于 UDP | 绕过内核 TCP 栈，用户态实现可靠传输 |
| 0-RTT 连接 | 首次 1-RTT，后续复用之前协商的密钥 0-RTT |
| 无 TCP 队头阻塞 | 每个流独立，一个流丢包不影响其他流 |
| 连接迁移 | 用 Connection ID 而非四元组标识连接，换网络不断连 |
| 内置 TLS 1.3 | 握手和加密合二为一 |

### 连接迁移

**面试常问**：「WiFi 切 4G 为什么 TCP 连接断？QUIC 不断？」

> TCP 用四元组（源IP、源端口、目的IP、目的端口）标识连接。换网络 IP 变了→四元组变了→内核认为新连接。QUIC 用 Connection ID 标识，ID 不变→连接不断。这对移动端特别重要。

---

## 五、HTTPS 与 TLS

### HTTPS = HTTP + TLS

```
HTTP → [TLS 加密] → TCP
```

### TLS 1.2 握手（2 RTT）

```
Client                          Server
  |--- ClientHello ------------>|
  |<-- ServerHello + Certificate-|
  |--- Key Exchange ------------>|     （RSA 或 ECDHE 密钥交换）
  |--- ChangeCipherSpec ------->|
  |--- Finished ---------------->|
  |<-- ChangeCipherSpec --------|
  |<-- Finished ----------------|
```

### TLS 1.3 握手（1 RTT）

```
Client                          Server
  |--- ClientHello + Key Share->|     （直接带上密钥交换参数）
  |<-- ServerHello + Key Share --|     （服务端也直接带参数）
  |<-- Certificate + Finished ---|     （合并发送）
  |--- Finished ---------------->|     （1 RTT 完成！）
```

**TLS 1.3 核心改进**：

| 改进 | 说明 |
|------|------|
| 1-RTT 握手 | 去掉了 RSA 密钥交换，只用 ECDHE |
| 0-RTT 恢复 | 重连时可复用之前协商的 PSK，0-RTT |
| 去掉不安全算法 | 删除了 RSA 静态密钥交换、CBC 模式、RC4、SHA-1 等 |
| 强制前向保密 | 只支持 ECDHE，每次会话密钥不同 |

### 前向保密（Forward Secrecy）

**面试常问**：「什么是前向保密？为什么重要？」

> 前向保密 = 即使长期私钥泄露，历史会话也无法解密。因为每次握手都临时生成 Diffie-Hellman 密钥对（ECDHE），会话密钥 = 双方临时私钥运算的结果，私钥用完丢弃。RSA 密钥交换做不到这一点：会话密钥用服务端长期公钥加密，私钥泄露→所有历史会话都可解密。

### 证书链验证

```
根 CA → 中间 CA → 服务端证书
```

1. 浏览器内置根 CA 公钥
2. 收到服务端证书→用中间 CA 公钥验证签名
3. 中间 CA 证书→用根 CA 公钥验证签名
4. 根 CA 在浏览器信任列表→信任链建立

---

## 六、HTTP 状态码

面试不需要全背，记住常用的和面试常问的。

### 1xx 信息

| 状态码 | 含义 |
|--------|------|
| 100 Continue | 继续发送请求体（大文件上传前确认服务端愿意接收） |

### 2xx 成功

| 状态码 | 含义 | 面试注意 |
|--------|------|----------|
| 200 OK | 请求成功 | |
| 201 Created | 创建成功 | POST 创建资源后返回 |
| 204 No Content | 成功但无返回体 | DELETE 成功 |

### 3xx 重定向

| 状态码 | 含义 | 方法是否允许变 | 典型场景 |
|--------|------|---------------|----------|
| 301 Moved Permanently | 永久重定向 | **可能变 POST→GET** | HTTP→HTTPS |
| 302 Found | 临时重定向 | **可能变 POST→GET** | 未登录跳登录页 |
| 303 See Other | 临时重定向 | **必须变 GET** | POST 后跳到结果页 |
| 307 Temporary Redirect | 临时重定向 | **不变** | 安全的 302 |
| 308 Permanent Redirect | 永久重定向 | **不变** | 安全的 301 |

**面试陷阱**：「301 和 302 的区别？」

> 301 是永久重定向，浏览器会缓存，下次直接跳不再请求原 URL。302 是临时的，每次都先请求原 URL。还有个坑：旧浏览器对 301/302 可能把 POST 变成 GET，307/308 保证方法不变。

### 4xx 客户端错误

| 状态码 | 含义 | 说明 |
|--------|------|------|
| 400 Bad Request | 请求格式错误 | 参数校验失败 |
| 401 Unauthorized | 未认证 | 缺少或无效的认证信息 |
| 403 Forbidden | 已认证但无权限 | 知道你是谁但不让你访问 |
| 404 Not Found | 资源不存在 | |
| 405 Method Not Allowed | 方法不允许 | 比如对只读资源用 DELETE |
| 408 Request Timeout | 请求超时 | |
| 429 Too Many Requests | 请求过多 | 限流返回 |

### 5xx 服务端错误

| 状态码 | 含义 | 说明 |
|--------|------|------|
| 500 Internal Server Error | 服务端内部错误 | 未捕获的 panic/异常 |
| 502 Bad Gateway | 网关错误 | 上游服务挂了或返回无效响应 |
| 503 Service Unavailable | 服务不可用 | 过载或维护 |
| 504 Gateway Timeout | 网关超时 | 上游服务超时 |

**面试追问**：「502 和 504 的区别？」

> 都是网关/代理返回的。502 是上游服务返回了无效响应（进程崩溃、端口没监听）；504 是上游服务没有在规定时间内响应（进程还在但太慢了）。

---

## 七、HTTP 缓存

### 强缓存（不发请求）

| 头部 | 说明 |
|------|------|
| `Cache-Control: max-age=3600` | 资源在 3600 秒内直接用缓存，不发请求 |
| `Expires` | HTTP/1.0 的绝对时间，优先级低于 Cache-Control |

### 协商缓存（发请求，可能 304）

| 请求头 | 响应头 | 机制 |
|--------|--------|------|
| `If-Modified-Since` | `Last-Modified` | 基于修改时间 |
| `If-None-Match` | `ETag` | 基于内容哈希，更精确 |

**流程**：

```
强缓存命中？→ 是 → 直接用缓存（200 from cache）
          → 否 → 协商缓存
                    → 发请求带 If-None-Match/If-Modified-Since
                    → 服务端判断没变 → 304 Not Modified（不传 body）
                    → 服务端判断变了 → 200 + 新资源
```

**Cache-Control 常用指令**：

| 指令 | 含义 |
|------|------|
| `no-cache` | 可以缓存，但每次用之前必须验证（不是「不缓存」！） |
| `no-store` | 真正不缓存，不存任何响应内容 |
| `private` | 只能被浏览器缓存，CDN 不能缓存 |
| `public` | 任何中间节点都可以缓存 |
| `max-age=N` | 缓存 N 秒 |

**面试陷阱**：「no-cache 是不缓存吗？」

> 不是！no-cache 是「可以缓存但每次用之前必须去服务端验证」。no-store 才是真正不缓存。这是最常搞混的。

---

## 八、Cookie、Session、Token

| 维度 | Cookie | Session | Token（JWT） |
|------|--------|---------|-------------|
| 存储位置 | 客户端（浏览器） | 服务端（内存/Redis） | 客户端（localStorage/Cookie） |
| 安全性 | 可设 HttpOnly + Secure | 服务端存，用户看不到 | 签名防篡改，但 payload 可解码 |
| 跨域 | 不行（同源策略） | 不行 | 可以（放 Header） |
| 扩展性 | 无状态 | 有状态（服务端存储） | 无状态 |
| 服务端压力 | 低 | 高（要存所有 session） | 低（不存储） |
| 注销 | 删 Cookie | 删服务端记录 | 难（只能等过期或加黑名单） |

**JWT 结构**：

```
Header.Payload.Signature
```

- Header：算法类型（`{"alg":"HS256","typ":"JWT"}`）
- Payload：声明（用户 ID、角色、过期时间等）
- Signature：`HMAC(Header + "." + Payload, secret)`

**JWT 面试考点**：

1. **JWT 的 payload 是 base64 编码不是加密**——任何人都能解码看到内容，不要放敏感信息
2. **JWT 无法主动失效**——签发后到过期之前一直有效，注销只能靠黑名单（又变有状态了）
3. **JWT 适合短期 token + 长期 refresh token** 的模式

---

## 九、CORS 跨域

**面试常问**：「为什么会有跨域问题？怎么解决？」

### 同源策略

浏览器安全策略：协议+域名+端口 三者相同才算同源。跨域请求默认被浏览器拦截（不是服务端拦截！）。

### CORS（Cross-Origin Resource Sharing）

| 请求类型 | 条件 | 例子 |
|----------|------|------|
| 简单请求 | GET/POST + 简单头部 + 特定 Content-Type | 普通 GET 请求 |
| 预检请求 | 非简单方法或自定义头部 | PUT、自定义 Header、application/json |

**简单请求流程**：

```
浏览器 → 请求带 Origin 头 → 服务端返回 Access-Control-Allow-Origin → 浏览器检查通过
```

**预检请求流程**：

```
浏览器 → OPTIONS 预检 → 服务端返回允许的方法和头部 → 浏览器检查通过 → 发实际请求
```

**服务端关键响应头**：

```
Access-Control-Allow-Origin: https://example.com
Access-Control-Allow-Methods: GET, POST, PUT
Access-Control-Allow-Headers: Content-Type, Authorization
Access-Control-Allow-Credentials: true      // 允许带 Cookie
Access-Control-Max-Age: 86400              // 预检结果缓存 24 小时
```

**面试注意**：`Access-Control-Allow-Origin` 不能设为 `*` 且同时 `Allow-Credentials: true`，浏览器会拒绝。带 Cookie 时必须指定具体域名。

---