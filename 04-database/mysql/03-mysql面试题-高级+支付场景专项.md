# MySQL 面试题 - 高级 + 支付场景专项

> 结合美餐支付业务，直击面试官常问的业务 + 技术结合题

---

## 一、高级原理题

### 1. Buffer Pool 是什么？为什么要有？

**答：**
Buffer Pool 是 InnoDB 的内存缓冲池，用来缓存磁盘的数据页，避免每次查询都读磁盘，提高性能。

**核心机制：**
- 大小一般配置为物理内存的 50%~70%
- 用 **LRU 链表** 管理缓存页，但是优化了的 LRU，分成 young 区和 old 区，防止全表扫描把热数据全部挤掉（扫描的页只放 old 区）
- 读数据：先查 Buffer Pool，有就直接返回，没有就从磁盘读进去缓存
- 写数据：先改 Buffer Pool 里的页（脏页），后台线程不定时刷回磁盘，不是每次写都刷磁盘

> **为什么不用操作系统缓存？** 因为 InnoDB 要自己控制刷盘时机、保证事务持久性，而且可以预读、优化缓存淘汰策略，比通用的 OS Cache 更适合数据库场景。

---

### 2. 什么是脏页？什么时候刷脏？

**答：**
Buffer Pool 里修改了还没刷到磁盘的页叫脏页。

**刷脏的触发时机：**
1. redo log 写满了，必须刷脏，不然没空间写新的 redo log（会阻塞用户写入）
2. Buffer Pool 不够用了，要淘汰一些内存页，脏页就先刷回磁盘
3. MySQL 空闲的时候后台刷
4. 正常关闭 MySQL 的时候全量刷

> **性能关键点**：尽量不要让 redo log 写满了才刷脏，那时候会阻塞所有写入，性能抖动厉害。要合理设置 redo log 大小，控制刷脏速度。

---

### 3. 什么是 Change Buffer？作用是什么？

**答：**
Change Buffer（以前叫 Insert Buffer）是 Buffer Pool 的一部分，用来缓存二级索引的 DML 操作（INSERT、UPDATE、DELETE）。

**作用原理：**
- 修改二级索引的时候，如果对应的索引页不在 Buffer Pool 里，就先把修改存在 Change Buffer 里，不用立刻读磁盘
- 后面这个索引页被其他查询读到的时候，再把 Change Buffer 里的变更合并进去（merge）
- 后台线程也会定期 merge

**为什么二级索引才需要？**
- 聚簇索引是主键，一般是自增的，插入是顺序的，不需要太多随机读
- 二级索引插入更新是随机的，比如 out_order_no 索引，插入是乱序的，随机 IO 多，Change Buffer 可以把多次修改合并成一次 IO，大幅提升性能

> **适用场景**：写多读少的业务，变更后不会立刻查的场景。不适用：写完立刻就查的场景，反而多了 merge 的开销。支付业务的二级索引变更多，很适用。

---

### 4. MySQL 怎么排查慢 SQL？

**答：** 面试必问的排查流程

**排查步骤：**
1. **开启慢查询日志**，设置 `long_query_time = 1`（超过1秒的都记录）
2. **用 mysqldumpslow 或 pt-query-digest 分析慢日志**，找出 TOP N 的慢 SQL
3. **用 EXPLAIN 分析执行计划**，看有没有全表扫描、有没有用对索引、rows 大不大、有没有 filesort
4. **show profile 看 SQL 执行各阶段耗时**（CPU、IO 等）
5. **查看 OPTIMIZER_TRACE**，看优化器是怎么选索引的，为什么选了你不想要的
6. **show processlist 看当前正在执行的 SQL**，有没有锁等待

---

## 二、支付场景专项题（重点！面试官百分百会结合你业务问）

### 5. 支付单表数据量大了怎么处理？（分库分表必考题）

**答：** 结合你们的业务实际说

**第一步：先优化，不急着分**
1. 索引优化，该加的索引加上，避免慢查询
2. 冷热数据分离：超过半年的历史支付单归档到历史库，只保留近3个月的热数据在主库
3. 读写分离：查询走从库，写入走主库

**第二步：确实太大了才分库分表**
- **分表维度选什么？** 选 `user_id` 或者 `mch_id` 都可以，支付场景大部分查询都是按用户或者按商户查
- **分多少张表？** 预估未来5年的数据量，一张表建议不超过2000万行，比如5年有10亿数据，就分64张或者128张
- **分片算法：** 用 `Hash(user_id) mod 64` 就行，简单高效
- **非分片键查询怎么解决？** 比如按 `out_order_no` 查，可以在基因法（把分片号编码到 out_order_no 里），或者做映射表，或者走ES查询

> **面试加分项**：你们现在支付单应该还没到分库分表的量级吧？其实归档 + 读写分离就够了，别上来就分库分表，增加复杂度。

---

### 6. 支付场景怎么保证幂等？重复回调怎么办？

**答：** 这是支付面试必问！

**三个层级的幂等保证：**

1. **数据库唯一索引层**：`out_order_no`（业务单号）建唯一索引，重复插入直接报错，从底层拦住
   ```sql
   UNIQUE KEY uk_out_order_no (out_order_no)
   ```

2. **业务逻辑层**：状态机判断
   - 支付单终态（成功、失败、关闭）就直接返回，不重复处理
   - 比如 TPW 回调过来，先查支付单状态，如果已经是 Success 了就直接返回成功，不用再处理

3. **分布式锁层**：对同一个 `out_order_no` 加分布式锁，防止并发回调
   ```go
   // 伪代码
   lock := redis.SetNX("pay:lock:" + outOrderNo, 1, 10*time.Second)
   if !lock {
       return "处理中"
   }
   defer lock.Release()
   // 处理回调逻辑
   ```

> **你可以结合你们的代码说**：我们支付服务就是这么做的，唯一索引兜底 + 状态机判断，基本上不会有重复支付的问题。

---

### 7. 余额扣款怎么保证不超扣？并发下怎么不会扣成负数？

**答：** 余额账户是支付系统最核心的，这个必问！

**两种方案：**

**方案1：悲观锁（推荐，余额场景冲突概率高）**
```sql
-- 事务里先锁行
SELECT balance FROM user_balance WHERE user_id = 123 FOR UPDATE;
-- 判断余额够不够
-- 够就扣
UPDATE user_balance SET balance = balance - 100 WHERE user_id = 123;
```
- 优点：简单，逻辑清晰，不会有重试风暴
- 缺点：锁的时间稍长，并发太高会有性能瓶颈

**方案2：乐观锁 + 数据库条件兜底（最常用）**
```sql
-- 直接用条件兜底，不用先查，性能更好
UPDATE user_balance
SET balance = balance - 100
WHERE user_id = 123 AND balance >= 100;
-- 看影响行数，>0 就是成功，=0 就是余额不足或者被别人改了
```
> 这个写法其实就够了，加了 `balance >= 100` 的条件，数据库层面就保证了不会扣成负数，乐观锁 version 都可以不用。

**方案3：行锁 + 异步兜底**
- 高并发的余额场景（比如红包、秒杀）可以把请求丢到 MQ 里串行消费，一个用户的请求都丢到同一个分区，串行处理，完全不用锁，并发更高

> **面试加分项**：其实最核心的就是 **所有扣款都用 UPDATE ... WHERE ... 条件判断**，不要先查出来再判断再更新，那样就有并发问题，必须把判断逻辑放在 SQL 里一起执行，数据库行锁保证原子性。

---

### 8. 支付的转账怎么保证原子性？A扣钱和B加钱怎么保证都成功？

**答：** 经典的分布式事务问题，支付场景必问！

**几种方案：**

1. **同库事务**：如果两个账户在同一个库同一个实例，直接用本地事务就行，最简单
   ```sql
   BEGIN;
   UPDATE account SET balance = balance - 100 WHERE user_id = 'A' AND balance >= 100;
   UPDATE account SET balance = balance + 100 WHERE user_id = 'B';
   COMMIT;
   ```

2. **分布式场景：可靠消息 + 最终一致性（推荐支付场景用）**
   - A 账户扣款成功 → 发送 MQ 消息 → B 账户消费消息加钱
   - 失败了就重试，达到最终一致
   - 用本地消息表保证消息一定发出去
   - 后台定时对账，发现不一致就人工介入或者自动补单

3. **TCC 方案**：Try - Confirm - Cancel，强一致性，但是侵入性强，开发成本高，核心转账才用

4. **Seata AT 模式**：自动生成反向 undo log，两阶段提交，业务侵入性小，适合对性能要求不是特别高的场景

> **你可以这么说**：我们支付内部的转账其实大部分都是同库的，直接本地事务保证。跨系统的就用可靠消息 + 最终一致性，然后每天对账检查，很少有不一致的情况，比 TCC 简单多了。

---

### 9. 每天的支付对账怎么实现？用 SQL 怎么算日成交额？

**答：** 结合你们的账单业务说

**日统计 SQL 例子：**
```sql
-- 按天统计每个商户的支付笔数、金额、退款金额
SELECT
    DATE(created_at) AS stat_date,
    mch_id,
    COUNT(*) AS pay_count,
    SUM(amount) AS total_amount,
    SUM(CASE WHEN status = 8 THEN amount ELSE 0 END) AS refund_amount
FROM payment_slip
WHERE created_at >= '2024-05-01 00:00:00'
  AND created_at < '2024-05-02 00:00:00'
GROUP BY stat_date, mch_id;
```

**优化点：**
- 这种统计不要跑主库，跑从库或者专门的分析库
- 数据量大的话就离线计算，用 Spark 或者 Flink，不要跑 MySQL
- 可以提前做预聚合，小时级或者天级的统计结果提前算好存起来，查询直接查结果表

---

### 10. 支付单状态更新怎么不丢更新？两个人同时改同一个支付单怎么办？

**答：** 经典的 ABA 问题 + 丢失更新问题

**方案1：乐观锁版本号（最通用）**
```sql
-- 每次更新都带版本号
UPDATE payment_slip
SET status = 6, version = version + 1
WHERE id = 12345 AND version = 旧版本号;
```
- 影响行数为0说明被别人改过了，重试或者返回失败

**方案2：状态机条件判断（支付场景专用）**
支付单的状态流转是有方向的，比如只能从 `New → Paying → Success`，不能跳，所以更新的时候带上前置状态条件就行：
```sql
-- 只有当前状态是 Paying 才能改成 Success
UPDATE payment_slip
SET status = 6
WHERE id = 12345 AND status = 3;
```
> **这个就够用了！** 支付场景有严格的状态机流转，不需要版本号，带上原状态条件就不会丢更新，也不会出现状态倒流。

---

### 11. 支付场景为什么用分表而不是分区表？

**答：**
- **分区表**：是逻辑上一张表，物理上分成多个文件，对应用层透明，还是同一个 MySQL 实例，单机容量和性能有上限
- **分库分表**：可以分到多个实例多个库，水平扩展能力强，支持更大的数据量和更高的并发
- **分区表缺点**：DDL 还是会锁，加分区删分区虽然快，但是改结构还是影响大；实例挂了所有分区都不能用；并发能力还是单机的

**支付场景用分表的原因：**
1. 支付数据量太大，单实例放不下
2. 并发高，单实例扛不住
3. 高可用要求高，分库可以故障隔离，一个库挂了不影响其他库

---

## 三、经典场景设计题

### 12. 怎么设计一个账户余额表？（必考设计题）

**答：** 我会这么设计：

```sql
CREATE TABLE `user_balance` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键',
  `user_id` bigint NOT NULL COMMENT '用户ID',
  `balance` bigint NOT NULL DEFAULT 0 COMMENT '可用余额，单位分',
  `frozen_amount` bigint NOT NULL DEFAULT 0 COMMENT '冻结金额',
  `total_recharge` bigint NOT NULL DEFAULT 0 COMMENT '累计充值',
  `total_consume` bigint NOT NULL DEFAULT 0 COMMENT '累计消费',
  `version` int NOT NULL DEFAULT 0 COMMENT '乐观锁版本号',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_id` (`user_id`)
) ENGINE=InnoDB COMMENT='用户余额表';
```

**设计要点：**
1. **金额全部用分**，不用小数，避免浮点数精度问题
2. **用户ID唯一索引**，每个用户只有一条记录
3. **一定有流水表**：每一笔余额变动都要有流水记录，方便对账和排查问题
   ```sql
   CREATE TABLE `balance_log` (
     `id` bigint NOT NULL AUTO_INCREMENT,
     `user_id` bigint NOT NULL,
     `change_type` tinyint NOT NULL COMMENT '变动类型：1充值 2消费 3退款',
     `amount` bigint NOT NULL COMMENT '变动金额，正负数',
     `before_balance` bigint NOT NULL COMMENT '变动前余额',
     `after_balance` bigint NOT NULL COMMENT '变动后余额',
     `biz_no` varchar(64) NOT NULL COMMENT '业务单号',
     `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
     PRIMARY KEY (`id`),
     UNIQUE KEY `uk_biz_no_type` (`biz_no`, `change_type`),
     KEY `idx_user_id_created` (`user_id`, `created_at`)
   ) ENGINE=InnoDB COMMENT='余额变动流水表';
   ```
4. **流水和余额在同一个事务里提交**：保证每一笔变动都有记录，出了问题可以对账回溯
5. **冻结金额字段**：支持预授权冻结场景，比如下单先冻结，成功再扣

> **面试加分项**：余额表 + 流水表是支付系统的标配，任何时候都可以用流水重算余额，是最后的兜底保障。
