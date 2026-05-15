# 2022 面试题整理 — MySQL 篇（3/5）

---

## Q26：mysql 的引擎有哪些

| 引擎 | 特点 | 适用场景 |
|------|------|---------|
| **InnoDB** | 事务、行锁、MVCC、外键、崩溃恢复 | 绝大多数业务（默认引擎） |
| **MyISAM** | 表锁、无事务、FULLTEXT 索引、COUNT(*) 快 | 只读、日志表、全文检索 |
| **Memory** | 数据在内存、表锁、哈希索引 | 临时表、缓存 |
| **Archive** | 压缩存储、只支持 INSERT/SELECT | 归档日志 |
| **NDB** | 分布式、行锁、高可用 | MySQL Cluster |
| **CSV** | CSV 文件存储 | 数据导入导出 |

**面试重点：InnoDB vs MyISAM**

| | InnoDB | MyISAM |
|---|---|---|
| 事务 | ✅ ACID | ❌ |
| 锁粒度 | 行锁 | 表锁 |
| MVCC | ✅ | ❌ |
| 崩溃恢复 | ✅ redo log | ❌ |
| 外键 | ✅ | ❌ |
| 全文索引 | ✅（5.6+） | ✅ |
| COUNT(*) | 需要遍历 | 直接存储行数 |

---

## Q27：mysql 索引用了哪些数据结构，场景是什么

| 数据结构 | 索引类型 | 场景 |
|---------|---------|------|
| **B+ Tree** | 主键索引、二级索引、联合索引 | 绝大多数场景（默认） |
| **Hash** | MEMORY 引擎、自适应哈希索引(AHI) | 等值查询，不支持范围 |
| **FULLTEXT** | 全文索引 | 文本搜索（5.6+ InnoDB 支持） |
| **R-Tree** | 空间索引 | GIS 地理数据 |

**为什么用 B+ Tree 不用 B Tree？**
- B+ Tree 非叶子节点只存 key，不存 data → 每个节点能存更多 key → 树更矮 → IO 更少
- 所有数据在叶子节点 → 查询性能稳定（都是 O(log n) 到叶子）
- 叶子节点链表连接 → 范围查询高效（遍历链表）

**为什么不用红黑树？**
- 红黑树是二叉树，高度远大于 B+ Tree → IO 次数多
- 红黑树不适合磁盘存储（一个节点一个页，浪费空间）

---

## Q28：select * from table where id in (1,2,3) 是怎么走的索引

**id 是主键时：**

```
id IN (1,2,3) 等价于 id=1 OR id=2 OR id=3
→ 主键索引范围扫描（range）
→ 优化器会排序为 id IN (1,2,3)，按顺序在 B+ Tree 上定位
→ 比三次等值查询更高效，因为可以利用 B+ Tree 的有序性
```

**id 是二级索引时：**

```
二级索引范围扫描 → 找到主键值 → 回表查聚簇索引
```

**IN 列表优化：**
- MySQL 将 IN 列表排序后，转为范围扫描
- IN 列表太长时（超过 `eq_range_index_dive_limit`，默认 200），优化器不再精确计算成本，可能走全表扫描
- IN 列表中的值会去重

---

## Q29：mysql 的优化手段

| 方向 | 具体手段 |
|------|---------|
| **索引优化** | 覆盖索引、最左前缀、避免索引失效（函数/隐式转换/OR/!=） |
| **查询优化** | 避免 SELECT *、限制返回行数、分页优化（游标分页） |
| **表结构优化** | 合适的字段类型、垂直拆分、反范式化冗余 |
| **架构优化** | 读写分离、分库分表、缓存（Redis） |
| **参数优化** | innodb_buffer_pool_size、连接池、慢查询日志 |
| **事务优化** | 减小事务粒度、避免长事务、降低隔离级别 |
| **SQL 改写** | 子查询改 JOIN、EXISTS 替代 IN、UNION ALL 替代 UNION |

---

## Q30：mysql 的 binlog 有几种模式，优缺点

| 模式 | 记录内容 | 优点 | 缺点 |
|------|---------|------|------|
| **STATEMENT** | 记录 SQL 语句 | 日志量小 | 主从不一致（NOW()、UUID()、自增列） |
| **ROW** | 记录行变更（修改前后的值） | 数据一致性最好 | 日志量大（大批量 UPDATE/DELETE） |
| **MIXED** | 默认 STATEMENT，不确定的用 ROW | 折中 | 仍可能有边界情况不一致 |

**推荐：ROW 模式。** 虽然日志量大，但数据一致性有保障，磁盘便宜，数据无价。

---

## Q31：mysql 的 RR 隔离级别是怎么实现的

**RR（Repeatable Read）= MVCC + 间隙锁。**

### MVCC 实现可重复读

每行数据有两个隐藏列：
- `DB_TRX_ID`：最后修改该行的事务 ID
- `DB_ROLL_PTR`：指向 undo log 中的上一个版本

**Read View（读视图）：** 事务第一次 SELECT 时创建，记录当前活跃事务列表。

**可见性判断规则：**

```
对于 undo log 版本链中的某个版本：
  │
  ├── DB_TRX_ID == 自己的事务ID → 可见（自己改的）
  │
  ├── DB_TRX_ID < ReadView 最小活跃事务ID → 可见（事务已提交）
  │
  ├── DB_TRX_ID > ReadView 最大事务ID → 不可见（事务在 ReadView 后才开启）
  │
  └── DB_TRX_ID 在活跃列表中 → 不可见（事务未提交）
      └── 沿 DB_ROLL_PTR 找上一个版本，重新判断
```

**RC vs RR 的区别：**
- **RC**：每次 SELECT 都创建新的 Read View → 能看到其他已提交事务的修改
- **RR**：只在第一次 SELECT 创建 Read View → 后续 SELECT 复用 → 保证可重复读

### 间隙锁防止幻读

MVCC 只解决了"快照读"的幻读，"当前读"（SELECT ... FOR UPDATE）需要间隙锁：

```sql
-- RR 下，当前读会加间隙锁
SELECT * FROM t WHERE id > 5 FOR UPDATE;
-- 锁住 (5, +∞) 的间隙，防止其他事务插入 id > 5 的新行
```

---

## Q32：mysql 的间隙锁是什么

**间隙锁（Gap Lock）：** 锁住索引记录之间的间隙，防止其他事务在间隙中插入新记录。

```
索引: 1, 5, 10, 15, 20

间隙锁锁定的范围:
(-∞, 1), (1, 5), (5, 10), (10, 15), (15, 20), (20, +∞)
```

**临键锁（Next-Key Lock）：** = 行锁 + 间隙锁，锁住记录本身 + 前面的间隙。

```
Next-Key Lock: (-∞, 1], (1, 5], (5, 10], (10, 15], (15, 20], (20, +∞)
```

**RR 下 InnoDB 默认加 Next-Key Lock，退化为：**
- 等值查询唯一索引且存在 → 退化为行锁
- 等值查询，最后一个不满足 → 退化为间隙锁
- 范围查询 → Next-Key Lock

**间隙锁之间不冲突**，间隙锁和插入操作冲突。

**RC 下不使用间隙锁**，所以 RC 可能幻读。

---

## Q33：mysql 有哪些锁

### 按粒度

| 锁 | 粒度 | 说明 |
|----|------|------|
| **全局锁** | 整个实例 | `FLUSH TABLES WITH READ LOCK`，备份用 |
| **表级锁** | 整张表 | 表锁、元数据锁(MDL)、意向锁 |
| **行级锁** | 行 | Record Lock、Gap Lock、Next-Key Lock |

### 行锁类型

| 锁 | 说明 |
|----|------|
| **Record Lock** | 锁住索引记录（行锁） |
| **Gap Lock** | 锁住间隙，不锁记录本身 |
| **Next-Key Lock** | Record + Gap，左开右闭区间 |
| **Insert Intention Lock** | 插入意向锁，间隙锁的特殊形式 |

### 意向锁（表级）

| 锁 | 说明 |
|----|------|
| **意向共享锁(IS)** | 事务打算加行共享锁前，先加表级 IS |
| **意向排他锁(IX)** | 事务打算加行排他锁前，先加表级 IX |

意向锁之间兼容，意向锁和表锁冲突（快速判断表里是否有行锁）。

### MDL（元数据锁）

- DML 操作自动加 MDL 读锁
- DDL 操作自动加 MDL 写锁
- 防止 DDL 和 DML 冲突

---

## Q34：乐观锁和悲观锁

| | 悲观锁 | 乐观锁 |
|---|---|---|
| **思想** | 假定一定冲突，先加锁 | 假定不冲突，提交时检查 |
| **实现** | `SELECT ... FOR UPDATE` | 版本号/CAS |
| **适用** | 写多、冲突频繁 | 读多、冲突少 |
| **开销** | 锁开销大 | 无锁开销，冲突时重试开销 |

**乐观锁实现：**

```sql
-- 版本号方式
UPDATE account SET balance = balance - 100, version = version + 1
WHERE id = 1 AND version = 5;

-- 影响行数 = 0 → 被其他事务修改了，需要重试
```

**悲观锁实现：**

```sql
BEGIN;
SELECT balance FROM account WHERE id = 1 FOR UPDATE;  -- 加锁
UPDATE account SET balance = balance - 100 WHERE id = 1;
COMMIT;  -- 释放锁
```

**选型考量：** 冲突率 < 10% 用乐观锁，冲突率高用悲观锁。支付系统扣余额通常用悲观锁（资金安全优先）。

---

## Q35：mysql 的 binlog 复制涉及几个线程

**三个线程：**

| 线程 | 位置 | 作用 |
|------|------|------|
| **Binlog Dump** | 主库 | 读取 binlog 发送给从库 |
| **I/O Thread** | 从库 | 接收主库 binlog，写入 relay log |
| **SQL Thread** | 从库 | 读取 relay log，重放 SQL |

```
主库                     从库
┌──────────┐         ┌──────────────┐
│ binlog   │ ──────→ │ relay log    │
│          │  Dump   │              │
└──────────┘ Thread  └──────┬───────┘
                            │ SQL Thread
                            ↓
                     ┌──────────────┐
                     │ 从库数据      │
                     └──────────────┘
```

**Go 5.6+ 并行复制：**
- **库级并行**：不同库的事务可以并行重放
- **Group Commit**：同一组提交的事务可以并行
- **WriteSet**（5.7.22+）：不同行的事务可以并行

---

## Q36：undo_log 和 redo_log 的作用

| | undo log | redo log |
|---|---|---|
| **作用** | 回滚、MVCC | 崩溃恢复（持久性） |
| **记录内容** | 修改前的数据（反向操作） | 修改后的数据（物理变更） |
| **存储位置** | 系统表空间 / undo 表空间 | ib_logfile0/1 |
| **写入时机** | 修改数据前先写 | 修改数据时写 |
| **用途** | ROLLBACK、Read View 版本链 | 宕机恢复已提交事务 |

**undo log 两大作用：**
1. **事务回滚**：保存修改前的数据，ROLLBACK 时恢复
2. **MVCC**：通过 DB_ROLL_PTR 形成版本链，实现非锁定读

**redo log 两大作用：**
1. **崩溃恢复**：宕机后用 redo log 重放已提交但未刷盘的数据
2. **性能优化**：WAL（Write-Ahead Logging），随机写变顺序写，无需每次 fsync 数据页

---

## Q37：mysql 崩溃重启后加载顺序

**崩溃恢复流程：**

```
1. 扫描 redo log
   │
   ├── 找到 checkpoint 位置
   │
   ├── 重做（REDO）：从 checkpoint 开始，重放所有已提交事务的修改
   │   → 保证已提交事务的数据不丢失
   │
   └── 撤销（UNDO）：回滚所有未提交事务的修改
       → 保证未提交事务的数据不生效

2. 最终状态 = 所有已提交事务生效 + 所有未提交事务回滚
```

**undo log 和 redo log 的加载关系：**
- redo log 先用（重做已提交事务）
- undo log 后用（回滚未提交事务）
- binlog 不参与崩溃恢复（用于主从复制和时间点恢复）

---

## Q38：redolog 和 binlog 的写入顺序，如何保证一致性

**两阶段提交（2PC）：**

```
事务提交过程：
  │
  ├── 阶段 1：写 redo log（prepare 状态）
  │
  ├── 阶段 2：写 binlog
  │
  └── 阶段 3：写 redo log（commit 状态）
```

**异常情况分析：**

| 异常时刻 | redo log | binlog | 恢复策略 |
|---------|----------|--------|---------|
| 阶段 1 前崩溃 | 无 | 无 | 无影响 |
| 阶段 1 后、阶段 2 前崩溃 | prepare | 无 | 回滚（事务未提交） |
| 阶段 2 后、阶段 3 前崩溃 | prepare | 有 | **提交**（binlog 有记录，从库需要重放） |
| 阶段 3 后崩溃 | commit | 有 | 已提交，无影响 |

**关键：** 崩溃恢复时，如果 redo log 是 prepare 状态，就检查 binlog 是否完整：
- binlog 完整 → 提交（保证主从一致）
- binlog 不完整 → 回滚

---

## Q39：redolog 里面放的是什么，写入磁盘规则

**内容：** 物理日志，记录"在某个数据页的某个偏移量做了什么修改"。

```
redo log 记录格式（简化）：
[表空间ID, 页号, 偏移量, 修改后的数据]

示例：
"表空间 67, 页 5, 偏移 100, 写入 0x12345678"
```

**写入规则：**

| 时机 | 说明 |
|------|------|
| **写入 buffer** | 修改数据时，先写 redo log buffer（内存） |
| **刷到磁盘** | 根据 `innodb_flush_log_at_trx_commit` 控制 |

**`innodb_flush_log_at_trx_commit` 值：**

| 值 | 行为 | 安全性 | 性能 |
|----|------|--------|------|
| **0** | 每秒刷一次 | 可能丢 1 秒数据 | 最快 |
| **1** | 每次事务提交都 fsync | 不丢数据 | 最慢 |
| **2** | 每次提交写 OS cache，每秒 fsync | OS 崩溃丢 1 秒 | 中等 |

**金融场景必须设为 1。**

---

## Q40：redolog 除了保证持久性以外还有什么作用

1. **崩溃恢复（持久性）**：宕机后重放已提交事务
2. **性能优化（WAL）**：随机写变顺序写，避免每次修改都刷数据页
3. **Group Commit**：多个事务的 redo log 合并一次 fsync，减少 IO

**WAL 原理：**
- 数据页的修改是随机 IO（B+ Tree 分散在各处）
- redo log 是顺序 IO（追加写入）
- 顺序 IO 比随机 IO 快 1-2 个数量级
- 所以先写 redo log（保证不丢），数据页可以延迟刷盘（靠 redo log 恢复）

---

## Q41：MVCC 和 undo log 的关系

**MVCC 的实现依赖 undo log 版本链。**

### 版本链

```
当前行: {id:1, name:"Alice", DB_TRX_ID:103, DB_ROLL_PTR:→}

undo log 版本链:
  TRX_ID=103: name="Alice" ──→
  TRX_ID=101: name="Bob"   ──→
  TRX_ID=98:  name="Carol" ──→ NULL (最老版本)
```

### Read View 可见性规则

```go
// 伪代码
func isVisible(trxID, readView) bool {
    if trxID == readView.creatorID {
        return true  // 自己改的，可见
    }
    if trxID < readView.minActiveID {
        return true  // 在 Read View 创建前已提交
    }
    if trxID > readView.maxID {
        return false  // 在 Read View 创建后才开始
    }
    if trxID in readView.activeList {
        return false  // 事务还没提交
    }
    return true  // 在活跃列表外，已提交
}
```

### RC vs RR

| 隔离级别 | Read View 创建时机 | 效果 |
|---------|-------------------|------|
| **RC** | 每次 SELECT 都创建新的 | 能看到其他已提交事务的修改（不可重复读） |
| **RR** | 只在第一次 SELECT 创建，后续复用 | 同一事务内多次 SELECT 结果一致（可重复读） |

### 当前读 vs 快照读

| | 快照读 | 当前读 |
|---|---|---|
| **SQL** | 普通 SELECT | SELECT FOR UPDATE / LOCK IN SHARE MODE / INSERT / UPDATE / DELETE |
| **读取** | MVCC Read View | 读取最新已提交数据 + 加锁 |
| **幻读** | MVCC 解决 | 间隙锁解决 |
