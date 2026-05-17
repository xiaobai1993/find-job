# Redis 分布式锁

## 核心设计

### 基于 redsync 库的分布式锁实现
- Redlock 算法（多 Redis 实例共识）
- 自动续期（看门狗）
- 支持批量锁

## 核心接口

```go
type Mutex interface {
    // Lock 加锁，加锁失败返回 redsync.ErrFailed
    Lock(ctx context.Context) error

    // LockAndReturnValue 加锁失败时返回当前持有者的 value
    LockAndReturnValue(ctx context.Context) (string, error)

    // Unlock 解锁
    Unlock() error
}
```

## 实现原理

### 1. 加锁流程（Redlock）
```
对 N 个 Redis 实例依次执行 SETNX:
  SET key random_value EX expire_time NX

成功计数:
  成功 >= N/2 + 1  → 加锁成功
  否则 → 释放已加的锁，返回失败
```

### 2. 自动续期（看门狗）
```go
func (m *mutex) watch() {
    ticker := time.NewTicker(m.expire / 2)
    for {
        select {
        case <-ticker.C:
            m.extend()  // 续期，延长过期时间
        case <-m.stopWatch:
            return
        }
    }
}
```

### 3. 解锁
```go
func (m *mutex) Unlock() error {
    // 停止看门狗
    if m.autoRenewal {
        if atomic.CompareAndSwapInt32(&m.lockFlag, 1, 0) {
            close(m.stopWatch)
        }
    }

    // Lua 脚本原子解锁：校验 value 匹配才删除
    return m.mutex.Unlock()
}
```

## 关键设计点

### 1. random_value 的作用
- 防止 A 释放 B 的锁（锁过期后误删）
- 只有持有相同 value 的才能解锁
- Lua 脚本保证原子性：`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`

### 2. 自动续期的边界
- 业务执行时间超过锁过期时间时自动续期
- 续期间隔 = 过期时间 / 2
- 续期失败时停止（锁可能已失效）

### 3. 错误兼容
```go
// 兼容旧代码，将 ErrTaken 转为 ErrFailed
var errTaken *redsync.ErrTaken
if errors.As(err, &errTaken) {
    err = redsync.ErrFailed
}
```

## 使用场景

### 场景1：防止重复支付
```go
lock := redislock.NewMutex(
    "pay:order:"+orderID,
    30*time.Second,   // 30秒过期
    true,             // 自动续期
    pools,
)

err := lock.Lock(ctx)
if err == redsync.ErrFailed {
    return errors.New("订单正在处理中")
}
defer lock.Unlock()

// 执行支付逻辑
```

### 场景2：批量锁（活动批量发放优惠券）
```go
batchLock := redislock.NewBatchMutex(
    []string{"coupon:1", "coupon:2", "coupon:3"},
    10*time.Second,
    pools,
)

results := batchLock.LockBatch(ctx)
for _, r := range results {
    if r.Err != nil {
        // 处理加锁失败的 key
    }
}
```

## 思考问题

1. **Redlock 真的安全吗？**
   - 网络分区、时钟漂移问题
   - 支付场景通常用单实例+主从即可
   - 红锁适合极端高可用场景

2. **锁续期失败怎么办？**
   - 业务逻辑要考虑锁可能失效
   - 数据库层面最终兜底（唯一键）

3. **如何避免死锁？**
   - 总是设置过期时间
   - defer Unlock()
   - panic 也要能解锁

4. **锁粒度如何选择？**
   - 太粗 → 并发低
   - 太细 → 死锁风险（按顺序加锁）
   - 支付场景通常锁订单 ID

## TODO

- [ ] 分析 LockAndReturnValue 方法的 FIXME 问题（未超过半数时释放已加的锁）
