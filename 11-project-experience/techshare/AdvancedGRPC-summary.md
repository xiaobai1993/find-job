# Advanced gRPC — 技术分享笔记

> 来源：美餐内部技术分享 PDF（67页）
> 主题：gRPC 高级特性，重点是**负载均衡**和**连接池**在 Kubernetes + Istio 环境下的实战踩坑

---

## 这个分享讲了什么

用一句话概括：**gRPC 在 Kubernetes 里看起来是"自动负载均衡"的，但其实没有——你需要手动做一些事情，否则流量只打到一个 Pod 上。** 这个分享就是讲怎么发现这个问题、怎么解决、以及踩了哪些坑。

---

## 一、背景：gRPC 负载均衡为什么是个问题？

gRPC 底层用的是 HTTP/2，HTTP/2 的特性是**一个 TCP 连接可以复用**（多路复用），同时发多个请求。

这个设计本来是优化点，但带来了一个副作用：

- HTTP/1.x 每次请求都新建 TCP 连接 → 正好命中不同 Pod，自然负载均衡
- gRPC/HTTP/2 复用同一个连接 → **所有请求都走同一个 Pod**，负载完全不均衡

所以在 Kubernetes 里，用 ClusterIP Service 做 gRPC 后端，实际上**只有一个 Pod 在工作**，其他 Pod 是空转的。

---

## 二、gRPC 内部架构（Channel / Subchannel / Resolver）

分享花了不少篇幅讲 gRPC 客户端的内部结构，这是理解后续一切的基础：

```
Client Request
     ↓
Channel（ClientConn）  ←── 你 grpc.Dial() 得到的那个连接对象
     ↓
LoadBalancer           ←── 选哪个 Subchannel 发请求
     ↓
Subchannel             ←── 真正的 TCP 连接，对应一个后端 IP:Port
```

**Resolver**：负责把你给的地址（比如 `my-service:8023`）解析成具体的 IP 列表，交给 LoadBalancer 建 Subchannel。

**关键：如果 Resolver 只解析出一个 IP，就只有一个 Subchannel，就只有一个 Pod 在干活。**

---

## 三、调试工具：channelz

gRPC 内置了一个诊断工具叫 **channelz**，可以看当前有几个 Channel、几个 Subchannel、每个 Subchannel 成功/失败多少次。

示例输出：
```
Name                                          State   Channel  SubChannel  Calls  Success  Fail
my-service.cluster.local:8023                 READY   0        3           1      1        0
127.0.0.1:44325                               READY   0        1           1      0        0
```

如果 SubChannel 只有 1，说明只连了一个 Pod，有问题。有 3 个说明连了 3 个 Pod，正常。

---

## 四、解决方案一：DNS Resolver + Headless Service（简单方式）

**原理：** 把 Kubernetes Service 改成 Headless（`clusterIP: None`），这样 DNS 解析会返回**所有 Pod 的 IP 列表**，而不是 ClusterIP 的单一 IP。

同时，把 gRPC endpoint 改成 `dns:///my-service:8023` 格式，让 gRPC 使用 DNS Resolver。

效果：gRPC 会为每个 Pod IP 建一个 Subchannel，用 round_robin 策略分发请求，实现真正的负载均衡。

日志里会看到：
```
Channel switches to new LB policy "round_robin"
Subchannel(id:4) created
Subchannel(id:5) created
Subchannel(id:6) created
```

**局限：** 依赖 DNS 刷新，Pod 新增/删除有感知延迟。如果你不用 Istio，这个方案很干净。

---

## 五、解决方案二：Manual Resolver（连接池，硬核方式）

这是分享的重点。团队自己写了一个 **Pool Resolver**，实现了真正的连接池。

**核心思路：**

> 一个目标地址 → 告诉 gRPC "有 N 个 Subchannel" → gRPC 建 N 个 TCP 连接 → N 路并发打到后端

代码层面就是实现 gRPC 的 `resolver.Builder` 接口，在 `ResolveNow` 里把同一个地址复制 N 份返回给 ClientConn，gRPC 就会建 N 个 Subchannel。

使用方式：
```go
endpoint, rb, err = poolresolver.GetPoolResolver(endpoint, poolSize)
dopts = append(dopts, grpc.WithResolvers(rb))
return grpc.DialContext(context.TODO(), endpoint, dopts...)
```

用了 Pool Resolver 后，channelz 显示：
```
Name                                          State   Channel  SubChannel
passthrough+pool://my-service:8023            READY   1        10
```

1 个 Channel，10 个 Subchannel。scheme 从 `dns:///` 变成了 `passthrough+pool://`。

---

## 六、连接池性能测试

测试环境：2500 并发，5 个后端 Pod，1 个客户端（1.5 vCPU，500Mi 内存）

| 方案 | QPS |
|------|-----|
| Istio（不加连接池） | 7k |
| Manual Resolver（连接池） | 8k |

QPS 提升约 14%。但资源消耗（内存）也更高，需要权衡。

---

## 七、混沌测试：在 K8s 滚动更新时请求会不会失败？

这部分是分享最有价值的地方——**现实里 gRPC + Istio 在 Pod 重启时有小概率请求失败**，原因很微妙。

### 7.1 为什么 Istio 下会有失败？

根因是 **ClientConn 的状态机**。当后端 Pod 停止时，Subchannel 会进入 `TRANSIENT_FAILURE` 状态。

- **有 retry（默认开启）**：gRPC 会自动重连，等 Subchannel 恢复后继续发请求，对上层业务无感知 → 10000/10000 成功
- **没有 retry（手动关闭）**：处于 `TRANSIENT_FAILURE` 时收到的请求直接失败，返回 `Unavailable` 错误 → 47% 失败率（4716/10000 失败）

状态流转：
```
Idle → Connecting → Ready → (Pod 重启) → IDLE → Connecting → TRANSIENT_FAILURE
                                                                    ↓
                                            DisableRetry=true → Unavailable（返回错误）
                                            DisableRetry=false → 重试 → Ready（成功）
```

### 7.2 Istio（Envoy）的 3 种连接场景

Istio 的 Envoy Sidecar 也做了连接管理，有几种边界情况：

**场景 1：正常请求**
Envoy 选连接 → OK → 建 Stream → 发请求 → 关闭 Stream，一切正常。

**场景 2：服务端在发请求时重启（GOAWAY）**
服务端发 GOAWAY 帧通知客户端 → Envoy 收到 → 销毁 Stream → 重新建连接。
如果 gRPC timeout 只有 2s，可能来不及重连就超时了 → 请求失败。
把 timeout 调到 5s → 没问题。

**场景 3：CDS/EDS 更新慢（Istio 控制面问题）**
Istiod 向 Envoy 推送 Endpoint 列表，如果推送有延迟，Envoy 的连接池里存的还是旧 IP。
连到旧 IP → connection refused → 失败。
体现为偶发性错误，重试能解决。

---

## 八、用 Istio VirtualService 配置 Retry 彻底解决

最终解法是在 Istio VirtualService 加 retry 策略：

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
spec:
  http:
    - name: primary
      timeout: 10s
      retries:
        attempts: 3
        perTryTimeout: 5s
        retryOn: unavailable
```

**注意一个坑：** Istio 的 retry 会覆盖 gRPC 客户端设置的 deadline。如果 gRPC 代码里设置了 1s timeout，但 Istio 配了 3 次重试每次 5s，实际等待时间可能远超 gRPC 的 timeout。两边要协调一致。

测试结果：滚动更新 + 随机 kill Pod + 10万请求 → 零错误。

---

## 九、附加内容：gRPC Healthz

分享末尾介绍了 gRPC 健康检查标准接入方式：

```go
import "google.golang.org/grpc/health/grpc_health_v1"

// 注册
grpc_health_v1.RegisterHealthServer(grpcServer, healthz.NewServer())
```

K8s 部署配置 readinessProbe / livenessProbe 用 `grpc_health_probe` 工具：
```yaml
readinessProbe:
  exec:
    command:
      - grpc_health_probe
      - '-addr=:8023'
  periodSeconds: 5
  failureThreshold: 5
```

返回 `{"status":"SERVING"}` 表示健康。

---

## 十、总结：什么情况用什么方案

| 方案 | 复杂度 | 适用场景 |
|------|--------|---------|
| Istio + retry（不加连接池） | 低 | 大多数场景，配置 VirtualService retry 即可 |
| DNS Resolver + Headless Service | 中 | 不用 Istio 的 K8s 环境 |
| Manual Pool Resolver | 高 | 极高并发，需要精细控制连接数，愿意承担复杂度 |

**最后结论（原文）：**
> Load balancing: round_robin
> Connection Pooling: Simple way = no pooling, use Istio; Hard way = manual resolver
> Istio pitfalls: Add retry, or embrace the error rate

---

## 值得关注的点

1. **gRPC 在 K8s 默认不均衡** — 一定要验证 channelz 里 Subchannel 数量，不然可能只有一个 Pod 在扛流量

2. **gRPC retry 默认是开的** — 不要随便关，它是 Pod 重启时保证零失败的关键

3. **Istio retry 会覆盖 gRPC timeout** — 坑点，两边超时要对齐

4. **channelz 是排查 LB 问题的利器** — 开发环境一定要会用，能直接看到连接分布

5. **连接池不是银弹** — QPS 提升约 14%，但内存消耗更高，而且代码复杂度大幅上升，普通业务场景不需要

6. **GOAWAY 场景需要足够的 timeout** — 服务端重启时会发 GOAWAY，客户端需要时间重连，timeout 太短就会报错

---

## 相关资料

- gRPC Wireshark 调试: https://grpc.io/blog/wireshark/
- HTTP/2 Frame 结构: https://halfrost.com/http2-http-frames-definitions/
- gRPC 负载均衡官方文档: https://github.com/grpc/grpc/blob/master/doc/load-balancing.md
