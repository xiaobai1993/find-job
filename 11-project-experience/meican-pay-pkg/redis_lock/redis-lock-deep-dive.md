# Redis 分布式锁深度解析 - 面试版

> 🎯 核心考点：Redlock 算法、自动续期、Lua 原子性、死锁预防、锁过期问题
> 源码位置：`pkg/redis_lock/`

---

## 一、这是什么？

这是**基于 Redlock 算法**的生产级分布式锁实现。

Redlock 是 Redis 官方推荐的分布式锁算法，解决了单实例 Redis 锁的单点故障问题。

**核心设计思想：**
- 向 N 个独立的 Redis 实例同时加锁
- 只要超过半数（N/2 + 1）成功，就认为加锁成功
- 即使少数节点宕机，锁仍然有效

---

## 二、核心数据结构

### Mutex 单体锁

```go
type mutex struct {
    mutex       *redsync.Mutex  // 底层 redsync 实现
    expire      time.Duration   // 锁过期时间
    pools       []redis.Pool    // 多个 Redis 连接池
    lockFlag    int32           // 原子标记，防止重复加锁
    stopWatch   chan struct{}   // 停止自动续期的信号
    autoRenewal bool            // 是否自动续期（看门狗）
}
```

### BatchLock 批量锁

```go
type BatchLock struct {
    pools  []redis.Pool    // 多个 Redis 连接池
    tries  int             // 重试次数
    factor float64         // 漂移因子
    quorum int             // 法定票数 = N/2 + 1
}
```

---

## 三、四大核心实现

### 1. Redlock 多数派加锁

```go
// 向所有 Redis 实例并行发加锁请求
n, err := l.actOnPoolsAsync(func(pool redis.Pool) (bool, error) {
    return l.acquire(ctx, pool, keys, value, expire)
})

// 超过半数成功才算加锁成功
if n >= l.quorum {
    return nil  // 加锁成功
}
```

**面试点：为什么需要多数派？**
> - 防止单点故障：1 个节点挂了，还有 N-1 个在
> - 防止脑裂：两个客户端不可能同时获得多数派
> - N 一般取奇数，推荐 3 或 5 个实例

---

### 2. Lua 脚本保证原子性

**加锁脚本：**
```lua
-- 原子执行 SET + NX + PX
for i,k in ipairs(KEYS) do
    if not redis.call("set", k, ARGV[1], "PX", ARGV[2], "NX") then
        return 0  // 有一个失败就整体失败
    end
end
return 1
```

**解锁脚本：**
```lua
-- 先校验 value，再删除，防止误删别人的锁
for i,k in ipairs(KEYS) do
    if redis.call("GET", k) == ARGV[1] then
        redis.call("DEL", k)
    else
        res = 0  // 有一个校验失败就整体失败
    end
end
return res
```

**面试必问：为什么用 Lua 不用多条命令？**
> - Redis 执行 Lua 是原子的，中间不会被其他命令插队
> - 不用 Lua 的话，「GET 校验 + DEL 删除」中间可能被人抢走锁
> - 这是分布式锁最经典的坑！

---

### 3. 自动续期（看门狗 Watch Dog）

```go
func (m *mutex) Lock(ctx context.Context) error {
    if err := m.mutex.LockContext(ctx); err != nil {
        return err
    }

    // 加锁成功后，启动看门狗自动续期
    if m.autoRenewal {
        if atomic.CompareAndSwapInt32(&m.lockFlag, 0, 1) {
            m.stopWatch = make(chan struct{})
            safego.Go(ctx, func(context.Context) { m.watch() })
        }
    }
    return nil
}

// 看门狗：每过期时间的一半，续期一次
func (m *mutex) watch() {
    watchTicker := time.NewTicker(m.expire / 2)
    defer watchTicker.Stop()

    for {
        select {
        case <-m.stopWatch:
            return  // 正常解锁，停止续期
        case <-watchTicker.C:
            if err := m.extend(); err != nil {
                return  // 续期失败，停止
            }
        }
    }
}
```

**面试点：自动续期解决了什么问题？**
> **锁过期问题：** 业务执行时间超过锁的过期时间，锁自动释放了，但是业务还在跑！
>
> 后果：两个线程同时持有锁，并发安全被破坏。
>
> 解决方案：看门狗定期给锁续期，只要业务还在跑，锁就不会过期。

**设计细节：**
- 用 `atomic.CompareAndSwapInt32` 防止重复启动看门狗
- 续期间隔是过期时间的一半（留有余量）
- 续期失败自动退出（说明锁已经丢了）

---

### 4. 批量加锁 BatchLock

```go
// 一次性给多个 key 加锁
func (l *BatchLock) Lock(ctx context.Context, keys []string, value string, expire time.Duration) error {
    // 向所有 Redis 实例发请求，每个实例一次性加所有 key
    n, err := l.actOnPoolsAsync(func(pool redis.Pool) (bool, error) {
        return l.acquire(ctx, pool, keys, value, expire)
    })

    if n >= l.quorum {
        return nil  // 超过半数实例加锁成功
    }

    // 失败则回滚：把已经加上的锁全部释放
    _, _ = l.actOnPoolsAsync(func(pool redis.Pool) (bool, error) {
        return l.release(ctx, pool, keys, value)
    })

    return redsync.ErrFailed
}
```

**设计亮点：**
- 一次性 Lua 脚本加 N 个锁，原子性
- 失败自动回滚，不会留下半吊子锁
- 适用于「需要同时锁多个资源」的场景

---

## 四、经典问题 & 解决方案

### ❌ 问题 1：锁过期了业务还没跑完

**场景：**
- 锁过期时间设 30 秒
- 业务因为 GC 卡了 40 秒
- 锁自动释放了，但是业务还在写数据库
- 另一个线程也拿到了锁，也在写
- 数据乱了！

**美餐的解决方案：**
```go
autoRenewal: true  // 开启自动续期

// 看门狗每 15 秒续期一次，只要业务还在跑
```

---

### ❌ 问题 2：误删了别人的锁

**场景：**
- 线程 A 加锁，value = "A"
- 线程 A 卡了，锁过期自动释放
- 线程 B 加锁，value = "B"
- 线程 A 醒了，执行 DEL，把线程 B 的锁删了！

**美餐的解决方案：**
```lua
-- 解锁必须先校验 value！
if redis.call("GET", k) == ARGV[1] then
    redis.call("DEL", k)
end
```

> 💡 这就是为什么锁的 value 必须是随机字符串！不能是固定值。

---

### ❌ 问题 3：Redis 主从切换丢锁

**场景：**
- 客户端给 master 加锁成功
- master 还没同步给 slave 就宕机了
- slave 升级成新 master
- 另一个客户端也能加锁成功！
- 两个客户端同时持有锁

**美餐的解决方案：**
```go
// Redlock 算法：不用主从，直接用多个独立的 Redis 实例
pools: []redis.Pool{redis1, redis2, redis3, redis4, redis5}

// 超过半数成功才算加锁成功
quorum: len(pools)/2 + 1
```

> 只要不超过半数节点同时宕机，锁就是安全的。

---

### ❌ 问题 4：时钟漂移

**场景：**
- Redis 节点之间时钟不同步
- 有的节点时间快，有的慢
- 导致锁的实际过期时间不准

**美餐的解决方案：**
```go
// 校验加锁耗时是否超过了漂移因子
if n >= l.quorum && expire > time.Since(start) + time.Duration(int64(float64(expire)*l.factor)) {
    return nil  // 加锁成功，剩余时间足够
}
```

> factor 通常取 0.5，预留 50% 的时间缓冲时钟漂移。

---

## 五、常见坑 & 最佳实践

### ❌ 坑 1：忘了解锁

```go
// 错误写法
mutex.Lock(ctx)
doSomething()  // 这里 panic 了，锁永远不会释放！
mutex.Unlock()

// ✅ 正确写法
mutex.Lock(ctx)
defer mutex.Unlock()  // 一定要 defer！
doSomething()
```

---

### ❌ 坑 2：可重入问题

redsync **默认不支持可重入**！

```go
mutex.Lock(ctx)
mutex.Lock(ctx)  // 死锁！自己把自己挡住了
```

**美餐的解决方案：**
- 业务层面避免重入
- 或者自己封装可重入锁（用 goroutine ID 计数）

---

### ❌ 坑 3：解锁失败怎么办

代码注释里说得很清楚：

```go
// Unlock 解锁可能失败，但是自动续期会停止，意味着最终锁一定会失效
// 见 https://planetmeican.atlassian.net/browse/PAY-68
```

> 设计哲学：**最终一致性**。即使解锁失败，锁也会自动过期，不会永久死锁。

---

## 六、和其他方案的对比

| 方案 | 优点 | 缺点 | 适用场景 |
|-----|------|------|---------|
| **美餐 Redlock** | 高可用、防单点、自动续期 | 实现复杂 | 支付核心、高可靠要求 |
| **单 Redis SETNX** | 简单 | 单点故障 | 非核心、低要求场景 |
| **ZooKeeper 锁** | 强一致、有等待队列 | 性能差、运维重 | 已有 ZK 集群 |
| **etcd 锁** | 强一致、lease 机制 | 需要 etcd 集群 | K8s 生态 |
| **数据库 select for update** | 不用额外组件 | 性能差、死锁风险 | 小流量场景 |

---

## 七、面试标准回答模板

### 面试官问：能说说你们的分布式锁实现吗？有什么亮点？

> "我们用的是基于 Redlock 算法的分布式锁，在 redsync 基础上做了封装。
>
> 核心设计有几个亮点：
>
> 第一，**Redlock 多数派机制**，用 3-5 个独立的 Redis 实例，超过半数加锁成功才算成功，解决了单实例的单点故障问题。
>
> 第二，**自动续期（看门狗）**，业务开启 `autoRenewal` 后，会启动一个后台 goroutine，每到过期时间的一半就自动给锁续期，解决了「业务还没跑完锁就过期了」的经典问题。
>
> 第三，**Lua 脚本保证原子性**，加锁和解锁都是用 Lua 脚本一次性执行，防止了「校验 + 删除」中间被人抢走锁的问题。
>
> 第四，**批量锁支持**，可以一次性给多个 key 加锁，失败自动回滚，适用于需要同时锁多个资源的场景。
>
> 还有一些细节处理：用 `atomic.CompareAndSwap` 防止重复启动看门狗，考虑了时钟漂移问题，加锁耗时超过漂移因子就认为失败。
>
> 这个锁在我们的支付核心链路用了好几年，稳定性非常好。"

---

### 面试官追问：分布式锁有哪些经典坑？你们是怎么解决的？

> "分布式锁的坑主要有这几个：
>
> **第一个坑：锁过期了业务还没跑完。** 我们的解决方案是自动续期，也就是看门狗机制，只要业务还在跑，锁就不会过期。
>
> **第二个坑：误删别人的锁。** 我们的锁 value 是随机字符串，解锁的时候先用 GET 校验 value 是不是自己的，是才删，用 Lua 保证原子性。
>
> **第三个坑：Redis 主从切换丢锁。** 我们用 Redlock 不用主从，直接连多个独立的 Redis 实例，多数派成功才算成功。
>
> **第四个坑：时钟漂移。** 我们加了漂移因子校验，加锁耗时太长就认为失败，回滚重试。
>
> **第五个坑：忘记解锁。** 这个是业务层面的，我们 Code Review 会强校验所有加锁都必须 defer unlock。"

---

## 八、一句话总结

> **分布式锁 = 原子加锁 + 随机 value + 过期时间 + 自动续期 + 多数派冗余**
>
> 最难的不是实现加锁，而是处理各种边界情况和异常场景。

---

## 📝 面试要点复盘

1. ✅ **Redlock 算法原理** - 多数派、独立节点、防单点
2. ✅ **Lua 原子性** - 为什么解锁必须先校验 value
3. ✅ **看门狗自动续期** - 解决锁过期业务还没跑完的问题
4. ✅ **四个经典坑及解决方案** - 过期、误删、主从切换、时钟漂移
5. ✅ **批量锁的回滚机制** - 失败了要把已经加上的锁释放掉
6. ✅ **最终一致性设计** - 解锁失败也没关系，最终会自动过期
