# XORM 封装面试全解

> ⚠️ 重要澄清：这个 `xorm` 包**不是 GitHub 上的 xorm ORM 框架**，是美餐在 GORM 基础上封装的事务层。面试时一定要先说明这个！

---

## 一、核心设计

### 解决的问题
原生 GORM 事务没有回调机制，事务提交后想做事只能在事务外做，可能失败。

### 核心架构
```
      DB 接口（统一对外）
          │
    ┌─────┴─────┐
    │           │
   db          tx
  普通DB       事务
               额外能力：
               - Commit/Rollback
               - Parent() 父事务
               - Put/Get 存数据
               - afterCommit 回调
               - afterSavePoint 回调
```

### 关键代码
```go
type DB interface {
    Transaction(ctx context.Context, fc func(ctx context.Context) error) error
    InTransaction() bool
    DB() *gorm.DB
}

type TX interface {
    DB
    Commit(ctx context.Context) error
    Rollback()
    Parent() TX
    Put(k, v interface{})
    Get(k interface{}) (interface{}, bool)
}
```

---

## 二、面试回答：这个封装做了什么？

> **面试标准答案：**
>
> "这个 xorm 包是我们在 GORM 基础上封装的事务层，核心是加了事务回调机制。
>
> 主要解决三个问题：
> 1. 原生 GORM 事务没有回调，事务消息不好做
> 2. 通过 SavePoint 实现真正的嵌套事务，支持部分回滚
> 3. 把 DB 藏在 context 里，业务代码不用到处传 DB 对象
>
> 典型应用就是本地消息表模式的事务消息实现，保证事务提交后才发 MQ。"

---

## 三、SavePoint 详解

### SavePoint 是什么？
**是数据库本身的特性，不是代码层面模拟的！**

```sql
-- SQL 标准语法
BEGIN;
    INSERT INTO orders ...;

    SAVEPOINT sp_deduct_stock;  -- 打个标记

    UPDATE stock SET count = count - 1 WHERE id = 123;

    -- 失败了就回滚到标记点
    ROLLBACK TO SAVEPOINT sp_deduct_stock;

    -- 成功了继续
COMMIT;
```

GORM 的 `SavePoint()` 和 `RollbackTo()` 就是帮你生成这两句 SQL。

---

## 四、面试回答：有 SavePoint 和没有的区别？

> **面试标准答案：**
>
> "区别很大。没有 SavePoint 的嵌套事务是假嵌套，内层失败会导致整个事务回滚。
>
> 有 SavePoint 才是真嵌套，可以做到部分回滚。比如支付场景，扣余额成功了，扣库存失败了，我们希望只回滚扣库存，然后走降级逻辑排队扣，而不是连扣成功的余额也回滚。
>
> 美餐这个封装的聪明之处是把 SavePoint 藏在了 Transaction 方法里，业务代码不需要关心现在是不是已经在事务中，想嵌套就嵌套，对调用者完全透明。"

---

## 五、假嵌套 vs 真嵌套

| 能力 | 假嵌套（只用 IsTXOpen 判断） | 真嵌套（IsTXOpen + SavePoint） |
|------|---------------------------|-------------------------------|
| 代码可嵌套 | ✅ 可以 | ✅ 可以 |
| 内层报错回滚 | ❌ 回滚整个事务 | ✅ 只回滚内层 |
| 外层能继续执行 | ❌ 不能 | ✅ 可以 |
| 降级逻辑 | ❌ 没法做 | ✅ 可以做 |

---

## 六、面试回答：只用 IsTXOpen 判断行不行？

> **面试标准答案：**
>
> "可以，但只是假嵌套。
>
> 只用 IsTXOpen 判断能解决代码不用到处判断的问题，业务代码可以无脑写 Transaction，但内层失败了还是会导致整个事务回滚，做不到部分回滚。
>
> 真嵌套还需要配合 SavePoint，才能实现内层失败不影响外层，才能做降级逻辑。
>
> 所以完整的嵌套事务 = 事务传播判断 + SavePoint 部分回滚。"

---

## 七、v1 vs v2 区别

| 对比维度 | v1 | v2 |
|---------|-----|-----|
| context 设计 | 两个 key：`rwDBKey` + `roDBKey` | 一个 `ctxKeyDB` 搞定 |
| 事务数据存储 | 自己维护 `tx.data map` | 用 GORM 内置 `InstanceSet/InstanceGet` |
| 读写库控制 | 两个方法 `MustGetRWDB` / `MustGetRODB` | 模式切换 `WithDBReadMode(ctx)` |
| 回调设置方式 | NewDB 时必须传全 | Builder 模式 `db.WithAfterCommitCallbacks(...)` |

**核心逻辑（回调 + SavePoint）完全没变，v2 是简化重构版。**

---

## 八、面试回答：为什么 GORM 官方不支持 AfterCommit？

> **面试标准答案：**
>
> "我觉得主要是三个原因：
>
> 第一是定位问题，GORM 的定位是 ORM 不是事务框架，设计哲学是只做数据库层的事，不多做架构层面的东西。
>
> 第二是没有标准答案，回调什么时候执行、失败了怎么处理、要不要重试，每个团队的策略都不一样，官方不敢定标准，怕限制用户。
>
> 第三是依赖问题，回调往往涉及业务依赖，比如发 MQ 需要 Producer，清缓存需要 Redis，GORM 作为底层库不好集成这些。
>
> 这也是为什么很多公司都会在 GORM 上面再封一层的原因。"

---

## 九、事务消息完整链路

```
业务调用 producer.SendWithTx(ctx, msg)
   ↓
1. 消息不直接发 MQ
   ↓
2. tx.Put(txMessageKey, messages)
   ↓ 把消息存在 tx 里！
3. 等着...
   ↓
内层事务成功 → afterSavePoint 回调
   ↓
4. 从事务 context 拿出消息
5. 批量 INSERT 到本地消息表
   ↓
最外层 Commit 成功 → afterCommit 回调
   ↓
6. 遍历 parent 链找所有消息
7. 真正发送到 MQ！
```

---

## 十、面试高频问题汇总

| 问题 | 核心回答点 |
|------|-----------|
| 这个 xorm 是干什么的？ | GORM 事务封装 + 回调机制 |
| 为什么不用原生 GORM？ | 没有回调，事务消息难做 |
| SavePoint 是数据库特性吗？ | 是，SQL 标准，所有关系型数据库都支持 |
| 假嵌套和真嵌套的区别？ | 能不能部分回滚 |
| 为什么要搞 v2？ | 简化设计，用 GORM 内置能力 |
| GORM 为什么不做 AfterCommit？ | 定位、没有标准答案、依赖问题 |
| 这个封装是必须的吗？ | 技术上不是必须，但支付业务强烈推荐 |

---

## 十一、一句话总结

> "这个封装的本质是：把事务对象藏在 context 里，通过统一的 DB 接口对外，再加上 SavePoint + 回调机制，完美实现了本地消息表模式。"
