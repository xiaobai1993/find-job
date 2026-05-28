# MySQL 补充问题

---

## 1. 当前读、快照读、Read View 分别是什么？

**快照读**：普通 `SELECT`，读历史版本，不加锁。

**当前读**：读最新版本，加锁。包括：
```sql
SELECT ... FOR UPDATE
SELECT ... LOCK IN SHARE MODE
INSERT / UPDATE / DELETE
```

**Read View** 是 MVCC 的核心结构，事务做快照读时创建，记录四个字段：

```
m_ids           当前所有活跃（未提交）的事务 ID 列表
min_trx_id      m_ids 里最小的
max_trx_id      下一个将分配的事务 ID（当前最大 + 1）
creator_trx_id  创建这个 Read View 的事务 ID
```

**可见性判断规则**：每行数据有隐藏的 `trx_id`（最后修改它的事务）。读某行时：

```
trx_id < min_trx_id              → 创建 Read View 前就提交了 → 可见
trx_id >= max_trx_id             → 创建 Read View 后才开启   → 不可见
min_trx_id <= trx_id < max_trx_id:
    trx_id 在 m_ids 里           → 还没提交                  → 不可见
    trx_id 不在 m_ids 里         → 已经提交了                → 可见
```

不可见就顺着 undo log 链往前找上一个版本，直到找到可见的为止。

**RC 和 RR 的本质区别就在 Read View 的创建时机：**

| 隔离级别 | Read View 创建时机 | 效果 |
|---------|------------------|------|
| RC | 每次 SELECT 都重新创建 | 能读到别人新提交的数据 → 不可重复读 |
| RR | 整个事务只在第一次 SELECT 时创建，之后复用 | 永远看同一个快照 → 可重复读 |

---

## 2. undo log 是什么时候产生的？

**修改数据之前，先写 undo log，再改数据。** 是前置动作，不是事后记录。

```
事务执行 DML
    ↓
① 写 undo log（记录"怎么撤销"）← 先
    ↓
② 修改 Buffer Pool 里的数据页   ← 后
    ↓
③ 写 redo log
    ↓
④ 事务提交
    ↓
⑤ purge 线程异步判断是否还有 Read View 引用，不需要则删除
```

**必须先写 undo log 的原因：**

如果先改数据再写 undo log，中间崩溃会导致数据已改但没有撤销手段，破坏原子性。

不同操作写的 undo log 内容：

| 操作 | undo log 记录内容 | 回滚时执行 |
|------|-----------------|-----------|
| INSERT | 新插入行的主键 | DELETE |
| DELETE | 整行数据 | INSERT |
| UPDATE | 被修改字段的旧值 | UPDATE 回旧值 |

undo log 除了用于回滚，还是 MVCC 版本链的存储介质，每行数据有隐藏字段 `DB_ROLL_PTR` 指向上一个版本，形成链表：

```
[当前版本] → [undo v2] → [undo v1] → [原始版本]
```

MVCC 读旧版本就是顺着这条链往回找，找到第一个"可见"的版本为止。

---

## 3. RC 下 undo log 提交后立即删除吗？

**不是立即删除，但比 RR 删得快很多。**

原因在于 **Read View 的生命周期**不同：

```
RR：Read View 活到整个事务结束（可能几分钟、几小时）
RC：Read View 只活到这条 SQL 结束（通常几毫秒）
```

purge 线程的判断条件是：

```
undo log 的 trx_id < 所有活跃 Read View 里最老的 min_trx_id → 可以删
```

在 RC 下，两条 SQL 之间那段间隔里，几乎没有任何 Read View 还在引用旧版本，purge 线程可以很快清理。

```
RC 事务A：
  SQL1 执行中 → Read View 存在 → 可能引用旧 undo
  SQL1 结束   → Read View 销毁 → 不再引用，几毫秒后 undo 可清理
  ...
  SQL2 执行中 → 重建新 Read View → 看到最新状态

RR 事务A（开着不提交）：
  SQL1 执行中 → Read View 存在
  SQL1 结束   → Read View 仍然存在！（RR 复用同一个）
  ...2小时后...
  SQL2 执行中 → 还是同一个 Read View
  → 2小时内产生的所有 undo log 都不敢删
```

所以"RC 下提交了基本就能删"这个直觉大体正确，RC 的 undo 积压问题几乎不会出现。长事务危害主要在 RR 下。

---

## 4. 业务中是否需要 RR？RC 是否足够？

**结论：大多数互联网业务 RC 足够，RR 在特定场景有价值。**

### RC 满足的场景（大多数业务）

简单 CRUD 的假设成立：查一次用一次，不依赖多次查询的一致性。

```
接口查单：SELECT WHERE id=xxx → 用查到的数据返回响应     → RC 够
更新操作：UPDATE ... WHERE id=xxx → 检查影响行数          → RC 够
幂等控制：INSERT 失败唯一索引冲突 / UPDATE WHERE status=old → RC 够
```

### RR 有价值的场景

**核心：同一事务内多次查询需要看到一致的数据快照。**

**场景一：聚合报表（最典型）**

```sql
BEGIN;
SELECT SUM(amount) FROM orders WHERE date='2024-01'; -- 100000，100 笔

-- 此时另一个事务 INSERT 了一条新订单并提交

SELECT COUNT(*) FROM orders WHERE date='2024-01';
-- RC 下重建 Read View，能看到新插入行 → 返回 101 条

COMMIT;
-- 结果：总金额基于100笔，总数量101笔，平均值算错！
```

RR 下两次查询看同一快照，SUM 和 COUNT 对得上。

**场景二：对账 / 数据导出**

导出期间有其他事务写入，RC 下前后批次看到的数据不一致，对账结果出错。RR 保证整个导出过程视图一致。

### RC + 显式加锁是更实用的组合

对确实需要强一致的关键路径，用 `SELECT FOR UPDATE` 显式加锁，比开 RR 粒度更可控：

```sql
-- 支付扣款：RC 下显式锁 + 检查状态
BEGIN;
SELECT balance FROM account WHERE id=1 FOR UPDATE;  -- 加行锁
UPDATE account SET balance = balance - 100 WHERE id=1 AND balance >= 100;
COMMIT;
```

**这也是互联网公司（早期阿里等）把默认隔离级别调成 RC 的原因：**

```
RR 的额外代价：
  - undo log 生命周期更长，长事务风险更大
  - 间隙锁增加死锁概率
  - 限制 binlog 必须使用 row 格式

RC 的代价：
  - 幻读和不可重复读需要业务层自己保证（唯一索引 + 状态机 + 显式锁）
```

---

## 5. RC 隔离级别下优化器不走索引，为什么强调隔离级别是 RC？

关键在于**间隙锁（Gap Lock）只在 RR 下才有**。

同样是 UPDATE 不走索引走全表扫描，两个隔离级别的锁行为完全不同：

**RR 下：**
```
全表扫描 → 对扫过的每一行都加 Next-Key Lock（行锁 + 间隙锁）
→ 实际上把整张表的数据 + 所有间隙都锁了
→ 其他事务无法插入任何行，并发几乎为零，还容易死锁
```

**RC 下：**
```
全表扫描 → semi-consistent read 优化
→ 先读已提交的最新版本，判断是否匹配 WHERE 条件
→ 只对真正匹配的行加行锁，不匹配的行扫过就释放
→ 没有间隙锁，其他事务还能正常插入
```

结论：

- RC 下不走索引：伤害是"锁了多余的行"
- RR 下不走索引：伤害是"几乎锁全表 + 锁所有间隙"，严重得多

这也是部分互联网公司（如早期阿里）把 MySQL 默认隔离级别调成 RC 的原因——牺牲一点一致性，换取更好的并发和更小的锁范围。

---

## 3. MySQL 5.6 以前加大字段为什么锁表，后来怎么改的？

**5.6 以前：Copy 算法**

```
① 创建一张包含新字段的临时新表
② 把旧表数据全量 INSERT 到新表   ← 数据量大时耗时很长
③ 旧表加 MDL 写锁，禁止所有 DML
④ 重命名新表替换旧表，删旧表
```

数据量大时第 ② 步要几十分钟，整个过程旧表被写锁挡住，读写全阻塞。

**5.6 引入 Online DDL：Inplace 算法**

```
① 在原表上原地修改，不创建临时表
② 修改期间允许并发 DML（增量变更记录到 row log）
③ 最后有一个短暂的 MDL 写锁，回放 row log，锁时间极短
```

**8.0 引入 Instant Add Column**

加普通字段（nullable 或固定默认值）直接用 `ALGORITHM=INSTANT`：

```
只修改数据字典里的元数据，记录"这张表有个新列，默认值是 X"
一行物理数据都不动，几毫秒完成，完全不锁表
```

旧行读新列时，InnoDB 从数据字典里拿默认值返回，不碰磁盘数据页。

**为什么强调"普通字段"（无默认值或固定默认值）：**

INSTANT 能工作的前提是不需要回填数据：
- `NULL` 或固定值（`DEFAULT 0`）→ 字典里记一下就行 → INSTANT
- `DEFAULT NOW()` 这种每行不同的值 → 必须逐行写入 → 不能 INSTANT，退回 Inplace

---

## 4. MySQL 8.0 INSTANT ADD COLUMN 什么时候真正把字段写入数据？加完字段多久可以做 UPDATE 或加索引？

**逻辑存在 vs 物理存在**

INSTANT 执行后：
- 数据字典：记录了新列的元数据 + 默认值 + instant 标记
- 磁盘上的旧数据行：一个字节都没动

InnoDB 行格式里每行有一个隐藏版本号，记录"这行写入时表有几列"。读旧行时，InnoDB 发现列数不够，从数据字典取默认值补上返回，旧行本身没有这个字段的任何字节。

```
旧行：[col1][col2]           ← 磁盘上没有 col3，读时从字典补默认值
新行：[col1][col2][col3]     ← 物理完整
```

**旧行何时才物理写入新字段（懒补齐）：**

```
① 这行被 UPDATE     → InnoDB 重写这行，以新格式写入，包含新字段
② OPTIMIZE TABLE   → 重建整张表，所有行都补齐
③ ALTER TABLE 触发重建（INPLACE/COPY）→ 所有行重写
```

**加完字段多久可以 UPDATE 或加索引：立刻可以，没有等待期。**

- UPDATE：执行那行会在本次写操作中被物理补齐
- 加索引：用 INPLACE 算法扫全表建索引，扫旧行时读逻辑值（默认值），扫新行读物理值，对索引构建没有区别，期间不锁表

---

## 5. EXPLAIN 的 type 字段各场景是什么？

假设有如下表：

```sql
CREATE TABLE payment_order (
    id          BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id     BIGINT NOT NULL,
    merchant_id BIGINT NOT NULL,
    order_no    VARCHAR(64) UNIQUE NOT NULL,
    amount      DECIMAL(10,2) NOT NULL,
    status      TINYINT NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL,
    INDEX idx_user_id (user_id),
    INDEX idx_merchant_status (merchant_id, status),
    INDEX idx_created_at (created_at)
);
```

### const — 主键或唯一索引等值查询，最多命中一行

```sql
SELECT * FROM payment_order WHERE id = 1;
SELECT * FROM payment_order WHERE order_no = 'PAY20240101001';
```

最多只匹配一行，MySQL 把这行当常量处理，最快。

### eq_ref — JOIN 时被驱动表用主键/唯一索引

```sql
SELECT o.*, u.name
FROM payment_order o
JOIN user u ON o.user_id = u.id;  -- u.id 是主键
```

每次从 `payment_order` 取一行，去 `user` 表精确命中一行，性能很好。

### ref — 普通索引等值查询，可能匹配多行

```sql
-- user_id 是普通索引，一个用户可能有多条订单
SELECT * FROM payment_order WHERE user_id = 100;

-- 联合索引 idx_merchant_status，只用了最左列
SELECT * FROM payment_order WHERE merchant_id = 50;
```

和 const 的区别：**const 确定只有一行，ref 可能有多行**。

### range — 索引范围扫描

```sql
SELECT * FROM payment_order WHERE id BETWEEN 100 AND 200;
SELECT * FROM payment_order WHERE created_at >= '2024-01-01';
SELECT * FROM payment_order WHERE user_id IN (1, 2, 3);  -- IN 也是 range
```

只扫索引的一段，是**生产环境慢查询优化的目标线，至少要到这一级**。

### index — 全索引扫描

```sql
-- 只查 user_id，覆盖索引但要扫整棵索引树
SELECT user_id FROM payment_order;

-- ORDER BY 索引列但没有 WHERE，扫整个索引
SELECT id, user_id FROM payment_order ORDER BY user_id;
```

比 ALL 好一点，因为索引比完整数据行小，但数据量大同样慢。

### ALL — 全表扫描，最差

```sql
-- amount 没有索引
SELECT * FROM payment_order WHERE amount > 100;

-- 函数导致索引失效
SELECT * FROM payment_order WHERE YEAR(created_at) = 2024;

-- 隐式类型转换导致索引失效（user_id 是 BIGINT，传了字符串）
SELECT * FROM payment_order WHERE user_id = '100abc';
```

线上大表出现 ALL 是告警级别的问题。

### 一句话记法

```
const/eq_ref  → 精确命中一行
ref           → 索引等值，多行
range         → 索引范围，一段
index         → 索引全扫
ALL           → 表全扫，要优化
```

---

## Extra 字段重要值

| Extra 值 | 出现场景 | 好/坏 |
|---------|---------|------|
| `Using index` | 覆盖索引，不需要回表 | ✅ 好 |
| `Using where` | 从索引拿到数据后还要用 WHERE 过滤 | 中性 |
| `Using filesort` | ORDER BY 没走索引，内存/磁盘排序 | ❌ 要优化 |
| `Using temporary` | 用了临时表，常见于 GROUP BY | ❌ 要优化 |
| `Using index condition` | 索引下推（ICP），在索引层过滤减少回表次数 | ✅ 好 |

---

## 6. 哪些场景适合加索引，哪些场景不适合？

### 适合加索引

**1. WHERE 条件里频繁出现的列**
```sql
-- user_id 每次查用户订单都会用，适合加
SELECT * FROM payment_order WHERE user_id = 100;
```

**2. JOIN 的关联列**
```sql
-- ON 后面的列如果没有索引，被驱动表每次都要全表扫
SELECT o.*, r.refund_amount
FROM payment_order o
JOIN refund r ON o.order_no = r.order_no;  -- order_no 要有索引
```

**3. ORDER BY / GROUP BY 的列**
```sql
-- 没有索引就要 Using filesort，数据量大时很慢
SELECT * FROM payment_order WHERE merchant_id = 50 ORDER BY created_at DESC;
-- idx_merchant_created (merchant_id, created_at) 能同时支持 WHERE + ORDER BY
```

**4. 高选择性的列**

选择性 = 不重复值的数量 / 总行数，越接近 1 越适合加索引。

```sql
-- order_no 每行唯一，选择性 = 1，非常适合
-- user_id 几百万用户，选择性高，适合
-- status 只有 0/1/2/3，选择性极低，不适合单独加
```

**5. 覆盖索引场景**
```sql
-- 查询只需要 merchant_id 和 amount，建联合索引避免回表
SELECT merchant_id, SUM(amount) FROM payment_order
WHERE merchant_id = 50
GROUP BY merchant_id;
```

---

### 不适合加索引

**1. 低选择性的列**
```sql
-- status 只有几个值，加了索引 MySQL 大概率不走
-- 一个 status=1 可能占全表 60%，还不如全表扫
ALTER TABLE payment_order ADD INDEX idx_status (status);  -- 意义不大
```

**2. 频繁写入、很少查询的列**

索引不是免费的，每次 INSERT/UPDATE/DELETE 都要同步维护索引树。写多读少的列加索引，写性能白白损耗。

**3. 小表**
```sql
-- 几百行的配置表，全表扫一次很快，索引反而多一次查找开销
-- MySQL 优化器也会自己判断放弃走索引
```

**4. 会走函数/表达式操作的列**
```sql
-- 即使 created_at 有索引，加了函数也会失效
SELECT * FROM payment_order WHERE YEAR(created_at) = 2024;  -- 索引失效
-- 改成范围查询才能用上
SELECT * FROM payment_order WHERE created_at BETWEEN '2024-01-01' AND '2024-12-31';
```

**5. 区分度低的列单独加索引**
```sql
-- is_deleted、gender、type 这类字段单独加索引没意义
-- 如果业务真的需要，放到联合索引的后面
INDEX idx_merchant_deleted (merchant_id, is_deleted)  -- 可以
INDEX idx_is_deleted (is_deleted)                     -- 没意义
```

**6. 索引已经很多了**

单表索引不要超过 5~6 个。索引多了：
- 写操作要维护多棵 B+ 树，写性能下降
- 占用磁盘空间
- 优化器要从更多索引里选，决策成本上升

---

### 支付场景的典型例子

```sql
-- payment_order 合理的索引设计
PRIMARY KEY (id)                                          -- 主键
UNIQUE INDEX idx_order_no (order_no)                      -- 幂等查询
INDEX idx_user_created (user_id, created_at)              -- 用户账单列表
INDEX idx_merchant_status (merchant_id, status, created_at) -- 商户对账

-- 不需要单独加的
INDEX idx_status (status)         -- ❌ 选择性太低
INDEX idx_is_deleted (is_deleted) -- ❌ 选择性太低
INDEX idx_amount (amount)         -- ❌ 很少单独按金额查
```

### 一句话记忆

```
查得多、选择性高、不常变 → 加
写得多、选择性低、用函数操作 → 不加
```

---

## 7. MySQL 体系结构是什么样的？

### 整体四层结构

```
┌─────────────────────────────────────┐
│           客户端                     │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│           连接层                     │
│  连接管理 / 认证 / 线程池             │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│           Server 层                  │
│  解析器 → 预处理器 → 优化器 → 执行器  │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│         存储引擎层                   │
│    InnoDB / MyISAM / Memory ...      │
└─────────────┬───────────────────────┘
              │
┌─────────────▼───────────────────────┐
│           文件系统                   │
│  数据文件 / redo log / binlog ...    │
└─────────────────────────────────────┘
```

### 第一层：连接层

客户端连上来之后，这层负责：

- **身份认证**：校验用户名、密码、host
- **线程分配**：每个连接分配一个线程处理请求
- **连接池**：复用空闲线程，避免频繁创建销毁

```sql
SHOW PROCESSLIST;                          -- 查看当前连接情况
SHOW VARIABLES LIKE 'max_connections';     -- 最大连接数，默认 151
```

### 第二层：Server 层

一条 SQL 进来依次经过：

**① 解析器（Parser）**

词法分析 + 语法分析，把 SQL 字符串解析成语法树。语法有问题在这里报错。

**② 预处理器（Preprocessor）**

语义检查：表存不存在、列存不存在、用户有没有权限。

**③ 优化器（Optimizer）**

生成执行计划，决定走哪个索引、JOIN 的顺序、是否用覆盖索引。`EXPLAIN` 展示的就是这里的结果。

**④ 执行器（Executor）**

按照执行计划调用存储引擎接口逐行获取数据，做过滤、排序、聚合，最终返回结果。

### 第三层：存储引擎层

插件式架构，可以替换：

| 引擎 | 特点 | 场景 |
|------|------|------|
| InnoDB | 事务、行锁、外键、崩溃恢复 | 默认，几乎所有场景 |
| MyISAM | 不支持事务、表锁 | 老系统，现在基本不用 |
| Memory | 数据在内存，重启丢失 | 临时表、缓存 |

InnoDB 内部核心结构：

```
Buffer Pool   → 数据页缓存，减少磁盘 IO
Change Buffer → 缓存二级索引的写操作
Redo Log      → 崩溃恢复，保证持久性
Undo Log      → 事务回滚 + MVCC
```

### 第四层：文件系统

```
.ibd 文件      每张表的数据 + 索引（InnoDB）
redo log       崩溃恢复
binlog         主从复制 / 数据恢复
undo log       回滚段
slow query log 慢查询日志
```

### 一条 SELECT 的完整链路

```
客户端发 SQL
    ↓
连接层：认证 + 分配线程
    ↓
解析器：SQL → 语法树
    ↓
预处理器：表/列/权限校验
    ↓
优化器：生成执行计划（选索引）
    ↓
执行器：按计划调用 InnoDB 接口取数据
    ↓
InnoDB：先查 Buffer Pool，没有再读磁盘
    ↓
返回结果给客户端
```

---

## 8. MySQL 服务端线程和 GORM 连接池是同一个概念么？

不是同一个，分别在不同位置，但互相关联。

### MySQL 服务端线程

MySQL 默认模型是**一个连接对应一个线程**。客户端每建立一个 TCP 连接，MySQL Server 就分配一个线程专门服务这个连接。

```
Go 应用                    MySQL Server
  连接1  ─────────────→   线程 A
  连接2  ─────────────→   线程 B
  连接3  ─────────────→   线程 C
```

### GORM 连接池（客户端侧）

GORM 底层用 Go 标准库 `database/sql`，维护一组**已建立好的 TCP 连接**，用完不关放回池子复用。解决的是：建立 TCP 连接 + MySQL 认证握手有开销，每次查询都新建连接代价太大。

```go
sqlDB, _ := db.DB()
sqlDB.SetMaxOpenConns(100)          // 最多同时 100 个连接
sqlDB.SetMaxIdleConns(10)           // 空闲时保留 10 个
sqlDB.SetConnMaxLifetime(time.Hour) // 连接最长存活 1 小时
```

### 两者的关系

```
GORM 连接池里的每一个连接，对应 MySQL 服务端的一个线程

GORM 连接池  → 解决客户端复用 TCP 连接，避免反复握手
MySQL 线程   → 解决服务端用线程处理每个连接的请求
```

### 实际配置注意

```
GORM MaxOpenConns  ≤  MySQL max_connections

多个服务实例部署时，所有实例的连接数加起来不能超过 MySQL 的 max_connections
否则新连接会报 "Too many connections"
```

---

## 10. B+ 树是什么？为什么索引选择用 B+ 树？

### B+ 树结构

B+ 树是一种多叉平衡搜索树，两个核心特征：
1. **非叶子节点只存 key，不存数据**，只用于导航
2. **所有数据都在叶子节点**，叶子节点之间用双向链表连接

```
                [30 | 60]
               /    |    \
      [10|20]    [40|50]    [70|80]
      /  |  \    /  |  \    /  |  \
    [10][20][30][40][50][60][70][80][90]
     ↓    ↓   ↓   ↓   ↓   ↓   ↓   ↓   ↓
    叶子节点之间用链表串起来 →→→→→→→→→→→
```

### 为什么不用普通二叉树 / 红黑树

树高太高。100 万行数据，红黑树高度约 20 层，查一条数据要 20 次磁盘 IO。

B+ 树是多叉的，每个节点可以放几百个 key，100 万行数据树高只有 3 层，最多 3 次磁盘 IO。

```
二叉树：每个节点 2 个分支，100 万行 → 约 20 层
B+ 树：每个节点几百个分支，100 万行 → 3 层
```

### 为什么不用 B 树

B 树的非叶子节点也存数据，导致两个问题：

```
B 树：非叶子节点存 key + data
     → 每个节点能放的 key 数量变少
     → 树更高，IO 次数更多

B+ 树：非叶子节点只存 key
      → 同样大小的节点能放更多 key
      → 树更矮，IO 次数更少
```

B 树的叶子节点之间没有链表，范围查询要反复回到根节点，效率很差。B+ 树叶子节点串成链表，范围查询直接顺序扫。

### 为什么不用哈希表

哈希表等值查询是 O(1)，但：

```sql
-- 完全不支持范围查询
WHERE created_at >= '2024-01-01'

-- 不支持排序
ORDER BY created_at

-- 不支持最左前缀
WHERE merchant_id = 50  -- 联合索引 (merchant_id, status) 只用左边
```

支付场景里范围查询、排序、联合索引极其常见，哈希表完全满足不了。

### 契合磁盘 IO 的原理

操作系统读磁盘的最小单位是页，InnoDB 页默认 16KB。B+ 树每个节点设计成刚好一页大小，一次磁盘 IO 读一页就把整个节点的所有 key 都读进来，然后在内存里二分查找。

```
磁盘 IO 次数 = 树的高度
树高 3 → 最多 3 次磁盘 IO → 查到任意一行
```

### 一句话记忆

```
B+ 树矮（多叉）→ 少 IO
非叶子只存 key → 节点放更多 key → 更矮
叶子串链表    → 范围查询直接顺序扫
```

---

## 11. B+ 树高度和索引大小怎么计算？

### 基础参数

```
InnoDB 页大小：16KB（默认）
每个节点 = 一个页 = 16KB
一次磁盘 IO 读一个节点
```

### 非叶子节点能放多少个 key

非叶子节点只存 key + 指针：

```
每条记录 = key(BIGINT 8字节) + 子节点指针(6字节) = 14字节

一个节点能放：16KB / 14B = 16384 / 14 ≈ 1170 个 key
```

**一个非叶子节点大约能放 1170 个分支。**

### 叶子节点能放多少行

叶子节点存完整行数据（聚簇索引），假设平均每行 1KB：

```
一个叶子节点能放：16KB / 1KB = 16 行
```

### 树高 → 能存多少行

```
树高 2：1170 × 16 = 18,720 行          → 约 2 万行
树高 3：1170 × 1170 × 16 = 21,902,400  → 约 2000 万行
树高 4：1170 × 1170 × 1170 × 16        → 约 256 亿行
```

**面试标准答案：树高 3 可以存约 2000 万行，查找任意一行最多 3 次磁盘 IO。**

根节点常驻 Buffer Pool，实际通常只需要 **2 次真正的磁盘 IO**。

### 索引大小估算

以二级索引为例（只存 索引列 + 主键，不存完整行）：

```
场景：payment_order 表 5000 万行
索引：idx_user_id (user_id BIGINT)

叶子节点每条记录：
  user_id(8字节) + id主键(8字节) = 16字节

一个叶子节点能放：16KB / 16B = 1024 条

需要多少个叶子节点：
  50,000,000 / 1024 ≈ 48,828 个

叶子层大小：48,828 × 16KB ≈ 762MB
非叶子层（可忽略）：48,828 / 1170 ≈ 42 个节点 ≈ 0.67MB

总索引大小：约 763MB
```

### 面试推导模板

```
① 非叶子节点容量
   (key大小 + 6字节指针) = 每条大小
   16KB / 每条大小 ≈ 每节点分支数 N

② 叶子节点容量
   聚簇索引：16KB / 平均行大小 = 每叶行数 M
   二级索引：16KB / (索引列大小 + 主键大小) = 每叶行数 M

③ 树高能存多少行
   高度 3：N × N × M

④ 索引大小
   总行数 / M = 叶子节点数
   叶子节点数 × 16KB ≈ 索引大小（非叶子层忽略）
```

### 一句话记忆

```
非叶子约 1170 分支，叶子约 16 行（1KB/行）
树高 3 → 2000 万行 → 3 次 IO（根在内存实际 2 次）
```

---

## 12. MySQL 5.6 以后各版本更新了哪些重要特性？

### MySQL 5.6（2013）

**索引优化**
- **ICP（Index Condition Pushdown）**：过滤条件下推到存储引擎层，减少回表次数，`EXPLAIN` 里显示 `Using index condition`
- **MRR（Multi-Range Read）**：回表前先对主键排序，把随机 IO 变成顺序 IO
- **BKA（Batched Key Access）**：JOIN 时批量获取数据，减少随机 IO

**复制**
- **GTID 复制**：每个事务有全局唯一 ID，主从切换不再需要手动指定 binlog 文件名和位置
- **多线程并行复制**：从库回放 binlog 支持按库并行，降低主从延迟

**DDL**
- **Online DDL**：`ALTER TABLE` 支持 `ALGORITHM=INPLACE`，加字段不再全程锁表

---

### MySQL 5.7（2015）

**JSON 支持**
- 新增 JSON 数据类型，支持 `JSON_EXTRACT`、`JSON_SET` 等函数

**生成列（Generated Column）**
```sql
-- 虚拟列，基于其他列计算，可以加索引
ALTER TABLE payment_order
ADD COLUMN amount_fen INT AS (amount * 100) VIRTUAL,
ADD INDEX idx_amount_fen (amount_fen);
```

**性能**
- **在线调整 Buffer Pool 大小**：不重启直接改 `innodb_buffer_pool_size`
- **sys schema**：内置诊断库，方便查慢查询、锁等待、IO 热点

**复制**
- **多源复制**：一个从库同时从多个主库复制
- **组复制（Group Replication）**：5.7.17 引入，多主强一致

---

### MySQL 8.0（2018）

**SQL 能力**

窗口函数：
```sql
SELECT user_id, order_no, amount,
       ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at) AS rn
FROM payment_order;
```

CTE 公共表表达式：
```sql
WITH monthly AS (
    SELECT DATE_FORMAT(created_at, '%Y-%m') AS month, SUM(amount) AS total
    FROM payment_order GROUP BY month
)
SELECT * FROM monthly WHERE total > 100000;
```

**索引增强**
- **Instant Add Column**：加普通字段只改元数据，秒级完成
- **降序索引**：`INDEX idx (created_at DESC)`，ORDER BY DESC 不再需要 filesort
- **隐藏索引**：加 `INVISIBLE`，优化器不用但结构保留，灰度验证删索引是否安全
- **函数索引**：直接对表达式建索引

**优化器**
- **直方图统计**：对数据分布不均匀的列收集统计，优化器选择更准确
- **Hash Join**：等值 JOIN 大表性能大幅提升
- **跳跃扫描（Skip Scan）**：联合索引 `(a, b)`，只有 b 条件时部分场景也能用索引

**其他**
- 默认字符集改为 `utf8mb4`，新建库表不用再手动指定
- 移除 Query Cache（见下题）
- DDL 操作支持原子性，要么成功要么回滚
- 角色（Role）权限管理

### 一张表总结

| 版本 | 核心关键词 |
|------|-----------|
| 5.6 | Online DDL、GTID、ICP、MRR |
| 5.7 | JSON 类型、生成列、组复制、sys schema |
| 8.0 | 窗口函数、CTE、Instant Add Column、降序/隐藏索引、Hash Join、utf8mb4 默认 |

---

## 13. Query Cache 为什么被移除？

**原理**：缓存 SELECT 语句的完整结果集，相同 SQL 再来直接返回，跳过解析器、优化器、存储引擎。

**问题一：缓存失效太激进**

只要一张表有任何 DML，这张表相关的**所有缓存全部失效**：

```
payment_order 表有 1000 条缓存的查询结果
只要有一条 INSERT/UPDATE/DELETE 进来
→ 1000 条缓存全部清掉
→ 支付系统写操作频繁，缓存基本永远是空的
```

**问题二：全局锁竞争**

Query Cache 用一把全局互斥锁保护：

```
读缓存        → 加共享锁
缓存失效（写）→ 加排他锁

高并发下所有线程都在抢这把锁
写操作一来，所有读全部等待
本来想提速，反而成了串行瓶颈
```

**问题三：命中率极低**

SQL 必须字节级完全一致才能命中：

```sql
SELECT * FROM payment_order WHERE user_id = 100;   -- 缓存了
select * from payment_order where user_id = 100;   -- 未命中（大小写不同）
SELECT * FROM payment_order WHERE user_id = 100 ;  -- 未命中（多了空格）
SELECT * FROM payment_order WHERE user_id = 101;   -- 未命中（参数不同）
```

**问题四：内存碎片**

不同查询结果集大小不一，长时间运行后内存碎片严重，有额外维护成本。

**正确替代方案是 Redis：**

```
Query Cache：表一有写 → 全表缓存失效 → 粒度太粗，不可控
Redis：       由业务代码控制失效 → 只清该清的 → 灵活可控
```

MySQL 8.0 正式将其移除。

---

## 14. MySQL 索引下推（ICP）是什么？

**Index Condition Pushdown**，MySQL 5.6 引入，将部分 WHERE 条件的过滤从 Server 层下推到存储引擎层执行，减少回表次数。

### 没有 ICP 时

```
存储引擎：用索引前缀找到匹配行 → 回表读完整行数据
Server 层：用剩余 WHERE 条件过滤
→ 每次都要回表，哪怕回来后发现不满足条件，这次 IO 白费了
```

### 有 ICP 时

```
存储引擎：用索引前缀找到匹配行 → 直接在索引上检查剩余条件
→ 只有通过检查的行才回表
→ 减少不必要的回表 IO
```

### 具体例子

联合索引 `(name, age)`，执行：

```sql
SELECT * FROM users WHERE name LIKE '张%' AND age = 25;
```

**无 ICP**：找到所有姓张的索引行 → 每条都回表 → Server 层再过滤 age
**有 ICP**：找到所有姓张的索引行 → 在索引上直接检查 age = 25 → 只有满足的才回表

### 触发条件

| 条件 | 说明 |
|------|------|
| 存储引擎 | 仅 InnoDB 和 MyISAM |
| 访问类型 | range、ref、eq_ref、ref_or_null |
| ICP 字段 | 必须在索引中，InnoDB 主键不用（聚簇索引无需回表） |

### 验证方式

```sql
EXPLAIN SELECT * FROM users WHERE name LIKE '张%' AND age = 25;
-- Extra: Using index condition → 使用了 ICP
-- Extra: Using where          → 在 Server 层过滤，没用 ICP
```

### 一句话记忆

```
减少回表次数 = 减少随机 IO = 提升查询性能
ICP 不改变结果，只把过滤时机提前到存储引擎层
```

---

## 15. MySQL 8.0 新增的 SKIP LOCKED / NOWAIT 是什么？

MySQL 8.0 在 `SELECT ... FOR UPDATE` 后新增了两个选项，专门用于非阻塞锁控制。

### 语法

```sql
SELECT ... FOR UPDATE SKIP LOCKED;  -- 跳过已被锁定的行，不等待
SELECT ... FOR UPDATE NOWAIT;        -- 遇到锁直接报错，不等待
```

### 用 SKIP LOCKED 实现任务队列

```sql
CREATE TABLE job_queue (
  id      INT PRIMARY KEY AUTO_INCREMENT,
  status  ENUM('pending', 'processing', 'done') DEFAULT 'pending',
  payload JSON
);
```

多个 Worker 并发执行：

```sql
BEGIN;

-- 抢一条 pending 任务，跳过已被其他 Worker 锁住的行
SELECT id, payload
FROM job_queue
WHERE status = 'pending'
ORDER BY id
LIMIT 1
FOR UPDATE SKIP LOCKED;

UPDATE job_queue SET status = 'processing' WHERE id = ?;

COMMIT;
```

### 没有 SKIP LOCKED 的问题

```
Worker1 锁住 id=1
Worker2 执行同样 SQL → 阻塞等待 Worker1 释放
Worker3 也在等...
→ 所有 Worker 串行化，队列退化成单线程消费
```

### 有 SKIP LOCKED 之后

```
Worker1 锁住 id=1 → 处理中
Worker2 自动跳过 id=1，抢到 id=2 → 并行处理
Worker3 自动跳过 id=1/2，抢到 id=3 → 并行处理
→ 多 Worker 真正并发消费，互不干扰
```

### SKIP LOCKED vs NOWAIT

| | 行为 | 适用场景 |
|---|---|---|
| `SKIP LOCKED` | 跳过被锁行，继续找下一条 | 任务队列、多 Worker 并发 |
| `NOWAIT` | 遇锁立即抛 `ER_LOCK_NOWAIT` | 乐观并发控制，让上层快速重试 |
| 默认 | 阻塞等待锁释放 | 普通事务场景 |

### 注意事项

- MySQL **8.0+** 才支持，5.7 没有
- 仅 InnoDB 有效
- `SKIP LOCKED` 读到的不是一致性快照，只适合队列消费，不适合统计查询

---

## 16. 除了 MySQL，还有哪些方式实现无锁队列？

MySQL 的 `SKIP LOCKED` 本质是数据库帮你做了"原子抢占"，在其他场景实现同样效果，核心思路是用原子操作替代锁——**CAS（Compare-And-Swap）**。

### 核心原理：CAS

```
普通加锁：lock → 读 → 改 → 写 → unlock    （悲观，排他）
CAS：     读旧值 → 计算新值 → 原子地"如果还是旧值就写入，否则重试"  （乐观，无锁）
```

CPU 原生支持 CAS 指令（x86 的 `CMPXCHG`），硬件级原子操作，不需要 OS 介入。

### 方案一：CAS 链表队列（Michael-Scott Queue）

1996 年提出的经典算法，Go/Java 标准库底层参考了它。

```go
type node struct {
    val  int
    next unsafe.Pointer
}

type LockFreeQueue struct {
    head unsafe.Pointer
    tail unsafe.Pointer
}

func (q *LockFreeQueue) Enqueue(val int) {
    newNode := &node{val: val}
    for {
        tail := atomic.LoadPointer(&q.tail)
        tailNode := (*node)(tail)
        next := atomic.LoadPointer(&tailNode.next)
        if next == nil {
            if atomic.CompareAndSwapPointer(&tailNode.next, nil, unsafe.Pointer(newNode)) {
                atomic.CompareAndSwapPointer(&q.tail, tail, unsafe.Pointer(newNode))
                return
            }
        } else {
            atomic.CompareAndSwapPointer(&q.tail, tail, next)
        }
    }
}
```

没有任何 `sync.Mutex`，失败就重试（spin），不阻塞挂起。

### 方案二：无锁环形队列（Ring Buffer）

固定大小，用原子整数做读写游标，比链表更快（缓存友好）：

```go
type RingQueue struct {
    buf  []int
    mask uint64
    head uint64  // 原子读游标
    tail uint64  // 原子写游标
}

func (q *RingQueue) Enqueue(val int) bool {
    for {
        tail := atomic.LoadUint64(&q.tail)
        head := atomic.LoadUint64(&q.head)
        if tail-head >= uint64(len(q.buf)) {
            return false // 满了
        }
        if atomic.CompareAndSwapUint64(&q.tail, tail, tail+1) {
            q.buf[tail&q.mask] = val
            return true
        }
    }
}
```

这是 **LMAX Disruptor** 的核心思想，金融交易系统用它实现每秒千万级消息吞吐。

### 方案三：Redis（分布式无锁队列）

Redis 单线程模型保证原子性，用 Lua 脚本模拟 SKIP LOCKED：

```lua
local job = redis.call('LPOP', 'queue:pending')
if job then
    redis.call('HSET', 'queue:processing', job, 1)
    return job
end
return nil
```

多 Worker 并发调用，Redis 单线程保证只有一个人拿到同一条任务。

### 横向对比

| 方案 | 原子原语 | 适用场景 | 缺点 |
|------|----------|----------|------|
| MySQL SKIP LOCKED | 数据库行锁 + 跳过 | 任务持久化、需要事务 | 依赖数据库，性能有限 |
| CAS 链表 | CPU CAS 指令 | 进程内高并发 | ABA 问题，GC 压力 |
| Ring Buffer | 原子整数 | 极高吞吐，固定大小 | 不能动态扩容 |
| Redis Lua | Redis 单线程 | 分布式、跨进程 | 网络开销，单点 |
| Go channel | 内部 mutex + ring | 日常 goroutine 通信 | 有锁，但封装好 |

### 本质都一样

```
MySQL SKIP LOCKED  → 读到锁了就跳过，找下一个
CAS               → 写失败了就重试，直到成功
都是：不阻塞等待，遇到竞争就绕开或重试
```

区别在于 CAS 在 CPU 层面完成，无线程切换，适合进程内纳秒级竞争；MySQL/Redis 适合分布式场景下的任务调度。

---

## 17. 所有队列都适合做成无锁的吗？Go channel 多消费者和 CAS 队列是一回事么？

### 无锁队列不是万能的

CAS 失败时会**自旋重试**，持续占用 CPU：

```
goroutine1: CAS 失败 → 重试 → 重试 ...（一直占着 CPU）
goroutine2: CAS 失败 → 重试 → 重试 ...
→ 竞争越激烈，大家都在空转，CPU 浪费越严重
```

| 场景 | 适合无锁？ | 原因 |
|------|-----------|------|
| 低竞争、高吞吐（1 写多读） | ✓ 适合 | CAS 几乎不失败，无自旋浪费 |
| 高竞争（几十个 goroutine 同时抢） | ✗ 不适合 | 大量自旋，还不如 mutex 让出 CPU |
| 需要阻塞等待（空队列时挂起） | ✗ 不适合 | 无锁本身没有挂起机制 |
| 优先级队列 | ✗ 不适合 | 维护有序性 + CAS，复杂度极高 |
| 需要严格公平性 | ✗ 不适合 | CAS 不保证顺序，可能某个 goroutine 一直重试 |

**结论**：无锁队列适合竞争不激烈、不需要阻塞语义的场景。高竞争下 mutex 让被阻塞的 goroutine 真正睡眠，反而比自旋更省 CPU。

### Go channel 多消费者：行为一样，机制不同

```go
jobs := make(chan int, 100)

for i := 0; i < 5; i++ {
    go func() {
        for job := range jobs {
            process(job) // 每个 job 只会被一个 Worker 拿到
        }
    }()
}
```

行为上和 SKIP LOCKED、CAS 队列效果完全一样——多消费者互不干扰地消费同一个队列，每条任务只被一个 Worker 处理。

但机制上不同：

```
CAS 无锁队列：atomic.CompareAndSwap → 失败就自旋重试，全程无锁

Go channel：  内部是 mutex + ring buffer
              拿到 mutex → 从 ring buffer 取数据 → 释放 mutex
              抢不到就 gopark() 真正挂起，不占 CPU
```

Go channel **不是无锁的**，但是细粒度锁，临界区极短，配合调度器挂起机制，实际性能很好。

### 对比

| | Go channel | CAS 无锁队列 |
|---|---|---|
| 底层机制 | mutex + ring buffer | atomic CAS |
| 队列为空时 | goroutine 挂起，不占 CPU | 需要自己处理（忙等或加信号量） |
| 高竞争表现 | 好（挂起不浪费 CPU） | 差（自旋浪费 CPU） |
| 代码复杂度 | 极简 | 高 |
| 适用场景 | Go 日常并发，99% 的场景 | 对延迟极度敏感（如金融撮合引擎） |

### 一句话记忆

```
Go channel 多消费者：有锁但高效，是 Go 的正确默认选择
CAS 无锁队列：无锁但有自旋风险，只在对 μs 级抖动敏感时才值得用
两者解决同一个问题，机制不同
```

---

## 18. 隔离级别和锁的关系是什么？不同隔离级别下分别用了哪些锁？

### 先建立基础：两种读机制

理解隔离级别和锁，必须先区分两种读：

```
快照读（普通 SELECT）：读 MVCC 历史版本，完全不加锁
当前读（加锁的读/写）：读最新版本，必须加锁

当前读包括：
  SELECT ... FOR UPDATE        加排他锁 X
  SELECT ... LOCK IN SHARE MODE 加共享锁 S
  INSERT / UPDATE / DELETE      加排他锁 X
```

**隔离级别同时控制两件事：**
1. 快照读能看到哪个版本的数据（MVCC 版本可见性）
2. 当前读加什么样的锁（锁的粒度和类型）

---

### 锁类型速查

| 锁 | 英文 | 作用 |
|----|------|------|
| 共享锁 | S Lock | 读锁，多个事务可同时持有 |
| 排他锁 | X Lock | 写锁，独占，和 S/X 均互斥 |
| 意向共享锁 | IS | 表级，表示事务打算对某行加 S 锁 |
| 意向排他锁 | IX | 表级，表示事务打算对某行加 X 锁 |
| 行锁 | Record Lock | 锁定索引上的一条具体记录 |
| 间隙锁 | Gap Lock | 锁定两条索引记录之间的间隙，不锁记录本身 |
| 临键锁 | Next-Key Lock | 行锁 + 前面的间隙锁，InnoDB RR 的默认锁 |
| 插入意向锁 | Insert Intention Lock | INSERT 时设置，表示要插入间隙 |

**示例表（贯穿全文）：**

```sql
CREATE TABLE orders (
  id      INT PRIMARY KEY,
  user_id INT,
  amount  DECIMAL(10,2),
  INDEX idx_user (user_id)
);

-- 数据
-- id=1,  user_id=100, amount=100
-- id=5,  user_id=101, amount=200
-- id=10, user_id=101, amount=300
-- id=15, user_id=102, amount=150
-- id=20, user_id=103, amount=400
```

索引上的间隙划分（id 主键）：
```
(-∞,1]  (1,5]  (5,10]  (10,15]  (15,20]  (20,+∞)
```

---

### READ UNCOMMITTED（RU）

**快照读：** 直接读最新数据，包括未提交的，不走 MVCC，不加锁。

**当前读：** 只加行锁（Record Lock），无间隙锁。

```sql
-- 事务 A
UPDATE orders SET amount = 999 WHERE id = 5;  -- 加 X 行锁，未提交

-- 事务 B（RU 隔离级别）
SELECT amount FROM orders WHERE id = 5;
-- 直接读到 999，脏读！
```

**问题：** 脏读、不可重复读、幻读都存在。实际业务几乎不用。

---

### READ COMMITTED（RC）

**快照读：** 每条 SELECT 语句执行时重新创建 Read View，读已提交的最新版本。

**当前读：** 只加行锁（Record Lock），不加间隙锁。有 semi-consistent read 优化。

#### 例子一：不可重复读

```sql
-- 事务 A（RC）
BEGIN;
SELECT amount FROM orders WHERE id = 5;  -- 读到 200

-- 事务 B 此时提交
UPDATE orders SET amount = 999 WHERE id = 5;
COMMIT;

-- 事务 A 再次读
SELECT amount FROM orders WHERE id = 5;  -- 读到 999，不可重复读！
```

原因：RC 每次 SELECT 重建 Read View，能看到事务 B 提交后的新值。

#### 例子二：UPDATE 不走索引时的锁行为（RC 的 semi-consistent read）

```sql
-- RC 下，全表扫描 UPDATE
UPDATE orders SET amount = 0 WHERE amount > 100;
```

RC 的 semi-consistent read 优化：
```
扫到 id=1 → 读已提交版本，不匹配 → 直接跳过，不加锁
扫到 id=5 → 匹配 → 加 X 行锁
扫到 id=10 → 匹配 → 加 X 行锁
...
```

只对真正匹配的行加行锁，不匹配的行扫过就放走，没有间隙锁。

#### RC 的锁范围总结

```sql
-- 主键等值：只锁 id=5 这一行
SELECT * FROM orders WHERE id = 5 FOR UPDATE;
-- 加锁：Record Lock(id=5)

-- 普通索引等值：锁 user_id=101 的所有行，无间隙锁
SELECT * FROM orders WHERE user_id = 101 FOR UPDATE;
-- 加锁：Record Lock(id=5), Record Lock(id=10)（两条 user_id=101 的行）
-- 无 Gap Lock，其他事务可以 INSERT user_id=101 的行
```

---

### REPEATABLE READ（RR）— MySQL 默认

**快照读：** 整个事务只在第一次 SELECT 时创建 Read View，之后复用，永远看同一个快照。

**当前读：** 默认加 **Next-Key Lock**（行锁 + 前面的间隙锁），防止幻读。

#### 例子一：可重复读

```sql
-- 事务 A（RR）
BEGIN;
SELECT amount FROM orders WHERE id = 5;  -- 读到 200

-- 事务 B 此时提交
UPDATE orders SET amount = 999 WHERE id = 5;
COMMIT;

-- 事务 A 再次读
SELECT amount FROM orders WHERE id = 5;  -- 仍读到 200，可重复读！
```

原因：RR 复用同一个 Read View，看不到事务 B 的修改。

#### 例子二：主键等值查询——只加行锁，不加间隙锁

唯一索引（主键）等值命中时，InnoDB 知道只有一条记录，无需间隙锁：

```sql
SELECT * FROM orders WHERE id = 5 FOR UPDATE;
-- 加锁：Record Lock(id=5)，仅此一个
-- 间隙 (1,5) 和 (5,10) 不加锁
```

#### 例子三：普通索引等值查询——加 Next-Key Lock + Gap Lock

非唯一索引（user_id），同一个值可能有多条记录，需要锁住间隙防止幻读：

```sql
SELECT * FROM orders WHERE user_id = 101 FOR UPDATE;
```

加锁过程（在 idx_user 索引上）：

```
user_id 索引的顺序：100(id=1) → 101(id=5) → 101(id=10) → 102(id=15) → 103(id=20)

锁住：
  Next-Key Lock (100, 101(id=5)]    ← 锁住 user_id=101 第一条记录及其前面的间隙
  Next-Key Lock (101(id=5), 101(id=10)]   ← 锁住第二条
  Gap Lock (101(id=10), 102(id=15))       ← 锁住最后一条记录后面的间隙

同时对聚簇索引：
  Record Lock(id=5), Record Lock(id=10)  ← 回表加行锁
```

效果：其他事务无法再插入 user_id=101 的新行，幻读被阻止。

#### 例子四：范围查询——Next-Key Lock 覆盖整个范围

```sql
SELECT * FROM orders WHERE id > 5 AND id < 15 FOR UPDATE;
```

加锁（id 主键，范围查找）：
```
扫到 id=10 → Next-Key Lock (5, 10]
扫到 id=15 → 不满足 id < 15，但扫到了这条记录 → Gap Lock (10, 15)

最终加锁：Next-Key Lock(5,10] + Gap Lock(10,15)
→ 间隙 (5,15) 内其他事务无法插入 id=6,7,...,14 的行
```

#### 例子五：幻读场景下 RR 的表现

快照读不会幻读，但当前读仍需小心：

```sql
-- 事务 A（RR）
BEGIN;
SELECT * FROM orders WHERE user_id = 101;       -- 快照读，读到 2 条

-- 事务 B
INSERT INTO orders VALUES (7, 101, 500);
COMMIT;

-- 事务 A 再次快照读
SELECT * FROM orders WHERE user_id = 101;       -- 仍 2 条，MVCC 屏蔽了新行 ✓

-- 但如果事务 A 用当前读
SELECT * FROM orders WHERE user_id = 101 FOR UPDATE;  -- 读到 3 条，幻读！
```

RR 通过 MVCC 解决了快照读的幻读，通过 Gap Lock 解决了当前读的幻读（只要事务 A 先执行了加锁查询，间隙被锁住，事务 B 的 INSERT 就会阻塞）。

---

### SERIALIZABLE（串行化）

**快照读：** 自动升级为当前读，普通 SELECT 也加 S 锁（共享锁）。

**当前读：** 和 RR 一样加 Next-Key Lock，但范围更广。

```sql
-- SERIALIZABLE 下
SELECT * FROM orders WHERE user_id = 101;
-- 等价于 SELECT ... LOCK IN SHARE MODE
-- 加 S Next-Key Lock，其他事务不能在这个范围内写
```

完全串行化，彻底解决所有并发问题，但吞吐量大幅下降，支付系统中除非极个别强一致性场景，否则不用。

---

### 四个隔离级别对比

| 隔离级别 | 快照读版本 | 当前读加锁 | 脏读 | 不可重复读 | 幻读 |
|---------|-----------|-----------|------|-----------|------|
| RU | 最新（含未提交）| Record Lock | ✗ 有 | ✗ 有 | ✗ 有 |
| RC | 最新已提交（每条 SQL 重建）| Record Lock，无 Gap Lock | ✓ 无 | ✗ 有 | ✗ 有 |
| RR | 事务开始时的快照（复用）| Next-Key Lock（行锁+间隙锁）| ✓ 无 | ✓ 无 | 基本无* |
| SERIALIZABLE | 不存在快照读，全部升为当前读 | S Next-Key Lock | ✓ 无 | ✓ 无 | ✓ 无 |

*RR 下：快照读完全没有幻读；当前读需要 Gap Lock 配合，但两次快照读之间插入加锁读可能幻读。

---

### RC vs RR：锁的核心差异

同样执行 `SELECT * FROM orders WHERE user_id = 101 FOR UPDATE`：

```
RC：  Record Lock(id=5) + Record Lock(id=10)
      → 只锁已有行，其他事务可以 INSERT user_id=101 的新行
      → 并发更好，但有幻读风险

RR：  Next-Key Lock(?, 101(id=5)] + Next-Key Lock(101(id=5), 101(id=10)] + Gap Lock(101(id=10), ?)
      → 锁住了间隙，其他事务无法 INSERT user_id=101 的新行
      → 防幻读，但间隙锁会增加死锁概率，并发略差
```

这也是互联网公司（如早期阿里）把默认隔离级别改为 RC 的原因：
- 牺牲幻读保护换取更小的锁范围和更好的并发
- 用业务层幂等代替数据库层防幻读

---

### Gap Lock 引发死锁的经典场景

```sql
-- 初始数据：id=1, id=5, id=10，没有 id=3 和 id=7

-- 事务 A（RR）
SELECT * FROM orders WHERE id = 3 FOR UPDATE;
-- id=3 不存在，加 Gap Lock (1, 5)

-- 事务 B（RR）
SELECT * FROM orders WHERE id = 7 FOR UPDATE;
-- id=7 不存在，加 Gap Lock (5, 10)

-- 事务 A 尝试 INSERT id=7
INSERT INTO orders VALUES (7, ...);
-- 需要进入 gap (5,10)，被事务 B 的 Gap Lock 阻塞，等待 B

-- 事务 B 尝试 INSERT id=3
INSERT INTO orders VALUES (3, ...);
-- 需要进入 gap (1,5)，被事务 A 的 Gap Lock 阻塞，等待 A

-- 死锁！MySQL 选一个事务回滚
```

Gap Lock 之间不互斥（两个 Gap Lock 可以共存），但 Gap Lock 会阻塞 Insert Intention Lock，这是间隙锁死锁的根源。

---

### 一句话总结

```
RU：读不加锁，写行锁，脏读
RC：快照读按语句级别，当前读只加行锁，无间隙锁，不可重复读
RR：快照读按事务级别，当前读加临键锁（行锁+间隙锁），防幻读，默认级别
SERIALIZABLE：读也加锁，完全串行，彻底无并发问题

隔离级别越高，锁越重，并发越差，一致性越强
```

---

## 19. 意向锁是什么，什么时候用？

### 意向锁解决的问题

InnoDB 同时支持行锁和表锁。当事务 A 锁住了某一行，事务 B 想对整张表加表锁时，B 如何判断能不能加？

如果没有意向锁，就得**逐行扫描**检查每一行有没有被锁，数据量大时代价极高。

意向锁的作用：**在加行锁之前，先在表上打一个标记**，让后来想加表锁的人一眼就能看到冲突，O(1) 完成判断。

### 两种意向锁

| 锁 | 含义 | 自动加的时机 |
|----|------|------------|
| IS（意向共享锁）| "我打算对某行加 S 锁" | `SELECT ... LOCK IN SHARE MODE` |
| IX（意向排他锁）| "我打算对某行加 X 锁" | `SELECT ... FOR UPDATE` / `INSERT` / `UPDATE` / `DELETE` |

**全部由 InnoDB 自动加，开发者无需手动操作。**

### 具体例子

```sql
-- 事务 A
SELECT * FROM orders WHERE id = 5 FOR UPDATE;
-- InnoDB 自动：① orders 表加 IX 锁  ② id=5 行加 X 锁

-- 事务 B 想加表锁
LOCK TABLE orders WRITE;  -- 想加表级 X 锁
-- 直接检查表上有无 IX → 发现有 → 冲突 → 阻塞
-- 不需要扫描每一行！
```

### 兼容性矩阵

意向锁之间互相兼容，只和表级 S/X 锁冲突：

```
          IS    IX    表级S   表级X
IS        ✓     ✓     ✓      ✗
IX        ✓     ✓     ✗      ✗
表级S      ✓     ✗     ✓      ✗
表级X      ✗     ✗     ✗      ✗
```

- **IX 和 IX 兼容** → 多个事务各自锁不同行，互不干扰
- **IX 和表级 X 冲突** → 有人持有行锁时，无法加写表锁
- **IX 和表级 S 冲突** → 有人持有写行锁时，无法加读表锁

### 一句话记忆

```
意向锁 = 行锁存在的"公告牌"
加行锁前先在表上挂公告 → 想加表锁的人看公告就知道有没有冲突
意向锁之间不互斥，不影响行级并发
```

---

## 20. IS NULL / IS NOT NULL 不走索引吗？

这是常见误解。**不是绝对不走索引，取决于选择性（数据分布）。**

### InnoDB 能对 NULL 建索引

B+ 树索引可以存储 NULL 值，NULL 被当作最小值排在索引最左端：

```
idx_phone 叶子节点：[NULL,id=3] → [NULL,id=7] → ['138...',id=1] → ['139...',id=5]
                      ↑ NULL 排最前面
```

`IS NULL` 本质是从索引最左端开始的**范围扫描**，和普通范围查询没有本质区别。

### 走不走索引，看匹配行占比

```sql
CREATE TABLE users (
  id    INT PRIMARY KEY,
  phone VARCHAR(20),
  INDEX idx_phone (phone)
);
```

| 场景 | 数据分布 | 走索引？ |
|------|---------|---------|
| `WHERE phone IS NULL` | 100 万行中只有 100 行是 NULL | ✓ 走，选择性高 |
| `WHERE phone IS NULL` | 100 万行中 80 万行是 NULL | ✗ 不走，匹配行太多 |
| `WHERE phone IS NOT NULL` | 100 万行中只有 100 行是 NULL（其余有值）| ✗ 不走，99.99% 行匹配，全表扫更快 |
| `WHERE phone IS NOT NULL` | 100 万行中只有 1000 行非 NULL | ✓ 走，选择性高 |

**核心规律和普通索引一模一样：匹配行超过全表 20~30%，优化器倾向放弃索引走全表扫。**

### 验证方法

```sql
EXPLAIN SELECT * FROM users WHERE phone IS NULL;
EXPLAIN SELECT * FROM users WHERE phone IS NOT NULL;
-- type: ref/range → 走了索引
-- type: ALL       → 全表扫
```

### 设计建议

如果字段经常按 NULL 查询且希望稳定走索引，可以用哨兵值代替 NULL：

```sql
phone VARCHAR(20) NOT NULL DEFAULT ''

-- 查询改为等值查询，稳定走索引
WHERE phone = ''
```

代价是 `COUNT(phone)` 和 `COUNT(*)` 语义统一了，需根据业务权衡。

### 一句话记忆

```
IS NULL / IS NOT NULL 不是索引杀手
走不走和普通查询一样，看匹配行占比
匹配行少 → 走索引；匹配行多 → 全表扫
用 EXPLAIN 验证，别靠经验猜
```

---

## 21. InnoDB 引擎里有哪些后台线程？

InnoDB 是多线程架构，核心有 4 类后台线程：

### Master Thread（主线程）

最核心的后台线程，统筹调度所有后台任务，分两个频率循环执行：

**每 1 秒：**
```
① 将 redo log buffer 刷新到磁盘（即使事务未提交）
② 合并 Insert Buffer（根据 IO 繁忙程度决定）
③ 刷新脏页到磁盘（根据脏页比例决定刷多少）
④ 如果空闲，切换到 background loop
```

**每 10 秒：**
```
① 刷新 100 个脏页到磁盘
② 合并 Insert Buffer
③ 强制刷新 redo log buffer 到磁盘
④ full purge，删除无用的 undo 页
```

随着版本迭代，Master Thread 不断瘦身：把脏页刷新交给了 Page Cleaner，把 undo 回收交给了 Purge Thread。

### IO Thread（IO 线程）

InnoDB 大量使用 AIO（异步 IO），IO Thread 负责处理 AIO 请求完成后的回调：

| 线程类型 | 默认数量 | 负责的 IO | 配置参数 |
|---------|---------|---------|---------|
| read thread | 4 | 数据页读请求 | `innodb_read_io_threads` |
| write thread | 4 | 数据页写请求 | `innodb_write_io_threads` |
| insert buffer thread | 1 | Insert Buffer 写 IO | — |
| log thread | 1 | redo log 写 IO | — |

### Purge Thread（清除线程）

负责回收已经不再需要的 undo log 页。事务提交后，undo log 不能立刻删除（可能还有其他事务的 Read View 在引用历史版本），等所有活跃事务都不再需要时，Purge Thread 负责回收。

```sql
SHOW VARIABLES LIKE 'innodb_purge_threads'; -- 默认 4（MySQL 8.0）
```

**为什么重要：** 如果有超长事务一直持有老 Read View，Purge Thread 无法推进，undo log 持续堆积，磁盘会暴涨。这是长事务危害的根本原因之一。

### Page Cleaner Thread（页清理线程）

MySQL 5.6 从 Master Thread 拆分出来，专门负责将 Buffer Pool 中的脏页刷新到磁盘。

```
刷脏时机：
① 脏页比例超过 innodb_max_dirty_pages_pct（默认 90%）
② redo log 快写满，必须先把对应脏页落盘才能覆盖 log
③ 数据库正常关闭时
```

```sql
SHOW VARIABLES LIKE 'innodb_page_cleaners'; -- 默认 4，建议与 Buffer Pool 实例数一致
```

### 版本演进

| 版本 | 变化 |
|------|------|
| 5.5 以前 | 脏页刷新、undo 回收全在 Master Thread，单线程瓶颈 |
| 5.5 | Purge Thread 独立 |
| 5.6 | Page Cleaner Thread 独立 |
| 5.7+ | Purge / Page Cleaner 支持多线程并行 |

### 一句话记忆

```
Master Thread   → 调度员，定时协调所有后台任务
IO Thread       → 搬运工，处理异步磁盘读写回调（读4+写4）
Purge Thread    → 清洁工，回收无用 undo log，防磁盘堆积
Page Cleaner    → 落盘工，把 Buffer Pool 脏页及时写到磁盘
```

---

## 22. ACID 在 InnoDB 里是怎么实现的？

### 总览

```
A 原子性  →  undo log
D 持久性  →  redo log（WAL）+ doublewrite buffer
I 隔离性  →  MVCC + Lock
C 一致性  →  由 A + I + D 共同保证，加上数据库约束
```

### D 持久性 — redo log

**先讲 D，因为 undo log 本身也依赖它。**

直接写数据页到磁盘是随机 IO，代价高。InnoDB 用 WAL（Write-Ahead Logging）：

```
修改数据时：
  ① 先把变更写入 redo log（顺序 IO，极快）
  ② 修改 Buffer Pool 里的内存页（脏页）
  ③ 脏页由 Page Cleaner 异步刷盘

事务 COMMIT 时：
  只需保证 redo log 落盘即可返回成功
  崩溃后用 redo log 重放恢复数据页
```

redo log 是**物理日志**，记录："第 X 页、偏移 Y 处，值从 A 变为 B"。

```sql
-- 控制刷盘时机
-- 1（默认）：每次 COMMIT 强制 fsync，最安全
-- 2：COMMIT 写 OS buffer，每秒 fsync，宕机丢 1 秒
-- 0：每秒写 + fsync，性能最高，风险最大
innodb_flush_log_at_trx_commit = 1
```

**doublewrite buffer** 防止 partial write（16KB 页写了一半宕机）：

```
① 脏页先写入 doublewrite buffer（系统表空间连续区域）
② 再写到真正的磁盘位置
③ 若 ② 时崩溃，从 doublewrite buffer 恢复完整页，再应用 redo log
```

### A 原子性 — undo log

修改数据前，先把"怎么撤销"写入 undo log：

```
INSERT id=5       → undo 记录：DELETE id=5
DELETE id=5       → undo 记录：INSERT id=5 原来的值
UPDATE 100 → 200  → undo 记录：UPDATE 200 → 100
```

需要回滚时，按 undo log 链从后往前依次执行逆操作，所有修改被撤销。

undo log 同时也是 MVCC 的版本链，每行有隐藏字段 `DB_ROLL_PTR` 指向历史版本：

```
[当前版本] → [上一版本] → [更早版本] → ... → [原始版本]
```

### I 隔离性 — MVCC + Lock

**快照读（普通 SELECT）→ MVCC**

每行有隐藏字段 `DB_TRX_ID`（最后修改的事务 ID）。事务读数据时创建 Read View，根据可见性规则沿 undo log 版本链找到能看到的版本：

```
RC：每条 SELECT 重建 Read View → 能看到最新提交 → 不可重复读
RR：事务内复用同一个 Read View  → 永远看同一快照 → 可重复读
```

**当前读（FOR UPDATE / 写操作）→ Lock**

```
RC  → 只加行锁（Record Lock），无间隙锁
RR  → 加临键锁（Next-Key Lock = 行锁 + 间隙锁），防幻读
```

### C 一致性 — 由其他三者共同保证

一致性是**目标**，AID 是**手段**：

```
原子性：事务要么全做要么全不做，不出现半截状态
隔离性：并发事务互不干扰，不读到中间状态
持久性：提交的数据不因崩溃丢失
```

数据库约束也参与保证一致性：PRIMARY KEY、UNIQUE、FOREIGN KEY、NOT NULL、CHECK。

### undo log 和 redo log 的协作

undo log 本身也是对数据页的修改，也会产生 redo log，从而受 redo log 保护。

**崩溃恢复的完整流程：**

```
① 用 redo log 重放所有已提交事务的变更（包括 undo 段的恢复）
② 用 undo log 回滚所有崩溃时未提交的事务
③ 数据库恢复到一致状态
```

### 一句话记忆

```
A 原子性：undo log 记逆操作，出错就回滚
D 持久性：redo log WAL 先写日志崩溃后重放；doublewrite 防部分写
I 隔离性：快照读用 MVCC 版本链，当前读用锁控制并发
C 一致性：A + I + D 共同保证，加数据库约束
```

---

## 23. MySQL JOIN 操作背后是怎么实现的？

MySQL 的 JOIN 不是简单的双重 for 循环，优化器会根据是否有索引、数据量大小选择不同的算法。

### 三种核心算法

#### Simple Nested Loop Join（SNLJ）

最朴素的双重循环，几乎不单独使用：

```
for each row r in 驱动表:
    for each row s in 被驱动表:
        if r.key == s.key:
            output(r, s)
```

时间复杂度 O(n×m)，被驱动表每行都要全扫，代价极高。MySQL 不直接使用这种形式。

#### Index Nested Loop Join（INLJ）— 最优

被驱动表的 JOIN 列上有索引时使用：

```
for each row r in 驱动表:
    index_lookup(被驱动表, r.join_col)   ← B+ 树查找，O(log m)
    output matched rows
```

时间复杂度 O(n × log m)，被驱动表不需要全扫，是 JOIN 性能最好的情况。

**这就是为什么 JOIN 列必须建索引。**

#### Block Nested Loop Join（BNL）— 无索引时的兜底

被驱动表没有索引时，MySQL 不会逐行嵌套，而是把驱动表的一批行放进 `join_buffer`，然后扫一次被驱动表，在内存里完成匹配：

```
for each block of rows from 驱动表 → 放入 join_buffer:
    for each row s in 被驱动表:
        match s against all rows in join_buffer
        output matched rows
```

被驱动表扫描次数 = `驱动表总行数 / join_buffer 能装的行数`。`join_buffer_size` 越大，被驱动表扫描次数越少。

```sql
-- 查看 join_buffer_size（默认 256KB）
SHOW VARIABLES LIKE 'join_buffer_size';

-- 调大可以减少被驱动表扫描次数
SET join_buffer_size = 4 * 1024 * 1024;  -- 4MB
```

**EXPLAIN 中 Extra 列出现 `Using join buffer (Block Nested Loop)` 就说明在用 BNL，需要给被驱动表加索引。**

#### Hash Join（MySQL 8.0.18+）— 取代 BNL

MySQL 8.0.18 引入，无索引时自动替代 BNL，分两个阶段：

```
Build 阶段：把较小的表（通常是驱动表）的 JOIN 列构建成内存哈希表
Probe 阶段：逐行扫描较大的表，用 JOIN 列去哈希表里探测

时间复杂度 O(n + m)，远好于 BNL 的 O(n × m / buffer_size)
```

内存不够时会 spill 到磁盘（临时文件），但总体仍优于 BNL。

**EXPLAIN 中 Extra 列出现 `Using join buffer (hash join)` 说明在用 Hash Join。**

### 算法选择逻辑

```
被驱动表 JOIN 列有索引？
    是 → INLJ（Index Nested Loop Join）
    否 → MySQL 8.0.18+：Hash Join
         MySQL < 8.0.18：BNL（Block Nested Loop Join）
```

### 驱动表选择原则

优化器会自动选择**小表驱动大表**，但"小"指的是满足 WHERE 条件后的结果集大小，不一定是原表行数。

```sql
-- 用 STRAIGHT_JOIN 强制指定驱动顺序（调试用，一般不用）
SELECT STRAIGHT_JOIN a.*, b.name
FROM orders a
STRAIGHT_JOIN users b ON a.user_id = b.id;
```

### EXPLAIN 快速识别

| Extra 内容 | 含义 | 处理方式 |
|------------|------|----------|
| `Using index` | INLJ，最优 | 无需处理 |
| `Using join buffer (hash join)` | Hash Join，8.0.18+ | 可接受 |
| `Using join buffer (Block Nested Loop)` | BNL，无索引 | 给被驱动表加索引 |

### 一句话记忆

```
有索引 → INLJ（O(n log m)，最优）
无索引 → 8.0.18+ Hash Join（O(n+m)）/ 旧版 BNL（批量匹配减少全扫次数）
驱动表选小表，被驱动表 JOIN 列必须建索引
```
