# Redis Cluster 实施与一致性哈希指南

> 🎯 包含：Redis Cluster 实施步骤、客户端感知机制、一致性哈希原理、Go语言客户端调用示例

---

## 一、Redis Cluster 实施步骤

### 📋 部署步骤总览

#### 1. 规划阶段
```
┌─────────────────────────────────────────┐
│ 节点数量：至少 3 主 3 从（共6节点）     │
│ 确定每个节点的 IP 和端口                 │
│ 哈希槽分配：16384 个槽平均分配          │
└─────────────────────────────────────────┘
```

#### 2. 配置每个节点的 redis.conf
```conf
# 开启集群模式
cluster-enabled yes

# 集群配置文件（自动生成）
cluster-config-file nodes.conf

# 节点超时时间（毫秒）
cluster-node-timeout 15000

# 开启 AOF 持久化
appendonly yes

# 端口配置
port 6379
```

#### 3. 启动所有节点
```bash
# 分别启动 6 个 Redis 实例
redis-server redis-6379.conf
redis-server redis-6380.conf
redis-server redis-6381.conf
redis-server redis-6382.conf
redis-server redis-6383.conf
redis-server redis-6384.conf
```

#### 4. 创建集群
```bash
# --cluster-replicas 1 表示每个主节点有 1 个从节点
redis-cli --cluster create \
  192.168.1.10:6379 \
  192.168.1.10:6380 \
  192.168.1.10:6381 \
  192.168.1.10:6382 \
  192.168.1.10:6383 \
  192.168.1.10:6384 \
  --cluster-replicas 1
```

#### 5. 验证集群
```bash
# 查看集群状态
redis-cli -c cluster info

# 查看节点和槽分配
redis-cli -c cluster nodes

# 测试操作（-c 表示集群模式，会自动重定向）
redis-cli -c set key1 value1
```

---

## 二、客户端感知机制

### ✅ Redis Cluster 客户端会感知

这是 Redis Cluster 和代理分片（Codis/Twemproxy）的本质区别：

#### 客户端行为流程：
1. **启动时拉取集群拓扑**：客户端连接任意一个节点，获取所有节点信息和 16384 个槽的分配表
2. **本地维护槽映射**：客户端在内存中保存「槽 → 节点地址」的映射表
3. **客户端自己计算路由**：`CRC16(key) % 16384` 算出槽，直接连接对应节点
4. **处理重定向**：如果节点返回 `MOVED` 或 `ASK`，客户端更新本地槽映射表

> ⚠️ **重要**：老的单机版 Redis 客户端 **不能直接用**，必须使用支持 Redis Cluster 协议的客户端。

---

## 三、一致性哈希环方案

### 🔄 原理图解

```
                0
               / \
              /   \
       节点A ○     ○ 节点B
            /       \
           /         \
   key1 → ●           ○ 节点C
           \         /
            \       /
             ○     ○
              \   /
               \ /
             2^32-1
```

#### 核心思想：
1. **环形空间**：范围是 `0 ~ 2^32 - 1`
2. **节点映射**：每个 Redis 节点计算哈希值，放到环的某个位置
3. **Key 映射**：每个 key 计算哈希值，也放到环上
4. **顺时针路由**：key 顺时针走，遇到的第一个节点就是存储节点

#### 解决的问题：
- **普通取模问题**：`hash(key) % N`，节点数变化时几乎所有 key 都要迁移
- **一致性哈希优势**：增加/删除节点时，**只影响相邻节点的 key**，迁移成本极低

#### 虚拟节点优化：
```
物理节点A → 虚拟节点A-1, A-2, ..., A-100
物理节点B → 虚拟节点B-1, B-2, ..., B-100
物理节点C → 虚拟节点C-1, C-2, ..., C-100

所有虚拟节点均匀分布在环上，解决数据倾斜问题
```

---

## 四、方案对比总表

| 特性 | Redis Cluster 哈希槽 | 一致性哈希环 |
|-----|-------------------|-----------|
| **分片单位** | 固定 16384 个槽 | 0 ~ 2^32-1 环形空间 |
| **路由位置** | 客户端自己计算 | 代理层计算 |
| **客户端感知** | 必须支持集群协议 | 完全透明，普通客户端即可 |
| **扩容影响** | 需要手动迁移哈希槽 | 只影响相邻节点的 key |
| **数据倾斜** | 槽均匀分配，基本无倾斜 | 需要虚拟节点解决 |
| **高可用** | 内置主从自动切换 | 需要额外实现 |
| **现在用不用** | 官方推荐，主流 | 老方案，基本被取代 |

---

## 五、Go 语言 Redis Cluster 调用代码

### 📦 使用 go-redis 客户端（v8 版本）

#### 1. 安装依赖
```bash
go get github.com/go-redis/redis/v8
```

#### 2. 基础使用示例

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

var ctx = context.Background()

func main() {
	// 创建 Redis Cluster 客户端
	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		// 只需要填几个种子节点，客户端会自动发现所有节点
		Addrs: []string{
			"192.168.1.10:6379",
			"192.168.1.10:6380",
			"192.168.1.10:6381",
		},

		// 连接池配置
		PoolSize:     100,        // 连接池大小
		MinIdleConns: 10,         // 最小空闲连接数
		MaxRetries:   3,          // 最大重试次数
		PoolTimeout:  4 * time.Second, // 获取连接超时

		// 超时配置
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,

		// 密码（如果有的话）
		// Password: "your-password",
	})

	// 测试连接
	pong, err := rdb.Ping(ctx).Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("Redis Cluster 连接成功:", pong)

	// 基础操作示例
	basicOperations(rdb)

	// 哈希操作示例
	hashOperations(rdb)

	// 管道操作示例
	pipelineOperations(rdb)

	// 事务操作示例
	transactionOperations(rdb)

	// 查看集群信息
	clusterInfo(rdb)
}

// 基础操作
func basicOperations(rdb *redis.ClusterClient) {
	// Set
	err := rdb.Set(ctx, "user:1:name", "张三", 10*time.Minute).Err()
	if err != nil {
		panic(err)
	}

	// Get
	val, err := rdb.Get(ctx, "user:1:name").Result()
	if err == redis.Nil {
		fmt.Println("key 不存在")
	} else if err != nil {
		panic(err)
	} else {
		fmt.Println("user:1:name =", val)
	}

	// Incr
	count, err := rdb.Incr(ctx, "counter").Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("counter =", count)

	// Expire
	rdb.Expire(ctx, "counter", 5*time.Minute)
}

// 哈希操作
func hashOperations(rdb *redis.ClusterClient) {
	key := "user:1:profile"

	// HSet
	err := rdb.HSet(ctx, key, "name", "张三", "age", 28, "email", "zhangsan@example.com").Err()
	if err != nil {
		panic(err)
	}

	// HGet
	name, err := rdb.HGet(ctx, key, "name").Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("HGet name =", name)

	// HGetAll
	data, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("HGetAll =", data)
}

// 管道操作（批量操作，减少网络往返）
func pipelineOperations(rdb *redis.ClusterClient) {
	pipe := rdb.Pipeline()

	// 批量发送命令
	incr := pipe.Incr(ctx, "pipeline_counter")
	pipe.Expire(ctx, "pipeline_counter", time.Hour)
	pipe.Set(ctx, "pipe:key1", "value1", 0)
	pipe.Set(ctx, "pipe:key2", "value2", 0)

	// 一次性执行
	_, err := pipe.Exec(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println("Pipeline incr result =", incr.Val())
}

// 事务操作
func transactionOperations(rdb *redis.ClusterClient) {
	// Watch + Pipeline 实现事务
	key := "tx_counter"

	err := rdb.Watch(ctx, func(tx *redis.Tx) error {
		// 获取当前值
		n, err := tx.Get(ctx, key).Int()
		if err != nil && err != redis.Nil {
			return err
		}

		// 在事务中执行多个操作
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, n+1, 0)
			return nil
		})
		return err
	}, key)

	if err != nil {
		fmt.Println("事务失败:", err)
	} else {
		fmt.Println("事务成功")
	}
}

// 查看集群信息
func clusterInfo(rdb *redis.ClusterClient) {
	// 获取所有节点信息
	nodes, err := rdb.ClusterNodes(ctx).Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("\n=== 集群节点信息 ===")
	fmt.Println(nodes)

	// 查看集群状态
	info, err := rdb.ClusterInfo(ctx).Result()
	if err != nil {
		panic(err)
	}
	fmt.Println("\n=== 集群状态 ===")
	fmt.Println(info)
}
```

---

### 📦 使用 go-redis v9 版本（最新）

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func main() {
	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs: []string{
			"192.168.1.10:6379",
			"192.168.1.10:6380",
		},
		PoolSize:    100,
		MaxRetries:  3,
		DialTimeout: 5 * time.Second,
		ReadTimeout: 3 * time.Second,
	})

	// 同样的操作方式...
	pong, err := rdb.Ping(ctx).Result()
	fmt.Println(pong, err)
}
```

---

### 📦 使用 redigo 客户端（较轻量）

```go
package main

import (
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/gomodule/redigo/redisx"
)

func main() {
	// redigo 需要配合 cluster 包使用
	import "github.com/chasex/redis-go-cluster"

	cluster, err := redis.NewCluster(
		&redis.Options{
			StartNodes: []string{
				"192.168.1.10:6379",
				"192.168.1.10:6380",
			},
			ConnTimeout:  5000 * time.Millisecond,
			ReadTimeout:  5000 * time.Millisecond,
			WriteTimeout: 5000 * time.Millisecond,
			KeepAlive:    16,
			AliveTime:    60 * time.Second,
		},
	)
	if err != nil {
		panic(err)
	}
	defer cluster.Close()

	// 操作
	_, err = cluster.Do("SET", "key", "value")
	if err != nil {
		panic(err)
	}

	val, err := redis.String(cluster.Do("GET", "key"))
	if err != nil {
		panic(err)
	}
	fmt.Println("key =", val)
}
```

---

## 六、生产环境最佳实践

### 🔧 连接池配置建议

```go
&redis.ClusterOptions{
	// 根据实际业务调整
	PoolSize:     50,   // 每个节点的连接池大小
	MinIdleConns: 10,   // 保持的最小空闲连接
	MaxRetries:   3,    // 失败重试次数

	// 超时设置要合理，不要太长也不要太短
	DialTimeout:  5 * time.Second,
	ReadTimeout:  3 * time.Second,
	WriteTimeout: 3 * time.Second,
	PoolTimeout:  4 * time.Second,

	// 路由刷新频率
	RouteByLatency: false,  // 按延迟路由，适合读多写少
	RouteRandomly:  false,  // 随机路由
}
```

### ⚠️ 注意事项

1. **Key 设计要考虑槽分布**：避免热点 key 集中在同一个槽
2. **避免跨槽操作**：不要对多个 key 做事务或批量操作，可能跨节点失败
3. **使用 Hash Tag 控制槽分配**：`{user:100}:name`、`{user:100}:age` 会落在同一个槽
4. **监控槽分布**：定期检查数据是否倾斜
5. **客户端版本**：使用支持 Cluster 协议的客户端版本

---

## 📝 面试要点总结

1. ✅ **Redis Cluster 有多少个哈希槽？** 16384 个
2. ✅ **客户端会感知吗？** 会，客户端自己维护槽映射，自己计算路由
3. ✅ **一致性哈希解决了什么问题？** 节点增减时只迁移部分数据，不是全量迁移
4. ✅ **虚拟节点解决了什么问题？** 数据倾斜，让数据分布更均匀
5. ✅ **Redis Cluster vs 一致性哈希？** 前者是客户端分片，后者是代理分片
