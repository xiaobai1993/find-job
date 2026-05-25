# TCC 幂等、空回滚、悬挂处理机制

> TCC 最难的不是 Try、Confirm、Cancel 三个接口本身，而是分布式调用下的重复请求、超时重试、网络延迟、请求乱序。幂等、空回滚、悬挂，本质上都是时序问题。

---

## 一、先建立正确理解

TCC 正常时序只有两种：

```text
Try 成功 → Confirm
Try 成功 → Cancel
```

但分布式系统里会出现异常时序：

```text
Confirm 重复到达
Cancel 重复到达
Try 还没执行，Cancel 先到了
Cancel 已经执行完，Try 后到了
Confirm 和 Cancel 并发到达
```

所以 TCC 不能只靠业务代码判断，必须有一张**分支事务状态表**，用状态机控制每个分支事务的流转。

---

## 二、分支事务状态表

一般会设计一张表记录每个 TCC 分支事务的状态：

```sql
CREATE TABLE tcc_branch_transaction (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    xid VARCHAR(64) NOT NULL,
    branch_id VARCHAR(64) NOT NULL,
    biz_id VARCHAR(64) NOT NULL,
    action_name VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    try_time DATETIME NULL,
    confirm_time DATETIME NULL,
    cancel_time DATETIME NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE KEY uk_xid_branch (xid, branch_id)
);
```

核心字段：

| 字段 | 含义 |
|---|---|
| `xid` | 全局事务 ID |
| `branch_id` | 分支事务 ID |
| `biz_id` | 业务 ID，比如支付单号、冻结单号 |
| `action_name` | TCC 动作，比如冻结余额、锁库存 |
| `status` | 当前分支事务状态 |

常见状态：

```text
TRY_SUCCESS   Try 已成功
CONFIRMED     Confirm 已成功
CANCELLED     Cancel 已成功
CANCEL_ONLY   Cancel 先到，Try 尚未执行，用于防悬挂
```

其中最容易忽略的是：

```text
CANCEL_ONLY
```

它是处理空回滚和悬挂的关键。

---

## 三、幂等问题

### 1. 什么是幂等？

幂等指：

```text
同一个请求执行一次和执行多次，结果一样。
```

TCC 里 Confirm 和 Cancel 都可能重复到达。

例如：

```text
Try 成功
Confirm 执行成功
协调器没有收到 Confirm 成功响应
协调器重试 Confirm
```

如果 Confirm 不幂等，就可能重复扣款。

---

### 2. Confirm 幂等处理

Confirm 进来时，先查分支事务状态。

处理规则：

```text
状态是 CONFIRMED：说明已经确认过，直接返回成功
状态是 TRY_SUCCESS：执行确认逻辑，然后改成 CONFIRMED
状态是 CANCELLED / CANCEL_ONLY：说明已经回滚或取消，不能再确认
其他状态：异常
```

伪代码：

```go
func Confirm(xid, branchID string) error {
    tx := LoadBranchTx(xid, branchID)

    if tx.Status == "CONFIRMED" {
        return nil
    }

    if tx.Status == "CANCELLED" || tx.Status == "CANCEL_ONLY" {
        return ErrAlreadyCancelled
    }

    if tx.Status != "TRY_SUCCESS" {
        return ErrInvalidStatus
    }

    BeginTx()
    DeductFrozenAmount()
    UpdateBranchStatus("CONFIRMED")
    Commit()

    return nil
}
```

---

### 3. Cancel 幂等处理

Cancel 也必须幂等。

处理规则：

```text
状态是 CANCELLED / CANCEL_ONLY：说明已经回滚过，直接返回成功
状态是 TRY_SUCCESS：执行释放资源，然后改成 CANCELLED
状态是 CONFIRMED：说明已经确认提交，不能再回滚
```

伪代码：

```go
func Cancel(xid, branchID string) error {
    tx := LoadBranchTx(xid, branchID)

    if tx.Status == "CANCELLED" || tx.Status == "CANCEL_ONLY" {
        return nil
    }

    if tx.Status == "CONFIRMED" {
        return ErrAlreadyConfirmed
    }

    BeginTx()
    ReleaseFrozenAmount()
    UpdateBranchStatus("CANCELLED")
    Commit()

    return nil
}
```

---

## 四、空回滚问题

### 1. 什么是空回滚？

空回滚是：

```text
Try 还没有执行成功，Cancel 先到了。
```

典型时序：

```text
1. 协调器调用 Try
2. Try 请求网络超时
3. 协调器以为 Try 失败，发起 Cancel
4. 业务系统先收到了 Cancel
5. 此时本地并没有 Try 成功记录
```

这时不能释放资源，因为根本没有冻结过资源。

---

### 2. 空回滚怎么处理？

Cancel 进来时，如果查不到 Try 记录：

```text
不能报错
不能释放资源
必须插入一条 CANCEL_ONLY 记录
然后返回成功
```

伪代码：

```go
func Cancel(xid, branchID string) error {
    tx := LoadBranchTx(xid, branchID)

    if tx == nil {
        BeginTx()
        InsertBranchTx(xid, branchID, "CANCEL_ONLY")
        Commit()
        return nil
    }

    if tx.Status == "CANCELLED" || tx.Status == "CANCEL_ONLY" {
        return nil
    }

    if tx.Status == "CONFIRMED" {
        return ErrAlreadyConfirmed
    }

    BeginTx()
    ReleaseFrozenAmount()
    UpdateBranchStatus("CANCELLED")
    Commit()

    return nil
}
```

关键点：

```text
空回滚不是简单地什么都不做。
必须留下 CANCEL_ONLY 记录。
```

这条记录是为了处理后面的悬挂问题。

---

## 五、悬挂问题

### 1. 什么是悬挂？

悬挂是：

```text
Cancel 已经执行完成，Try 后到了。
```

典型时序：

```text
1. 协调器发送 Try
2. Try 请求因为网络延迟迟迟没到
3. 协调器超时，认为 Try 失败
4. 协调器发送 Cancel
5. Cancel 先到，业务系统写入 CANCEL_ONLY
6. 延迟的 Try 终于到达
```

如果这时 Try 继续执行成功，就会出现：

```text
资源被冻结了
但全局事务已经取消
后续不会再有 Confirm 或 Cancel
资源一直挂住
```

这就是悬挂。

---

### 2. 悬挂怎么处理？

Try 执行前必须先查分支事务状态。

处理规则：

```text
如果发现 CANCEL_ONLY：说明 Cancel 先到过，拒绝 Try
如果发现 CANCELLED：说明已经取消，拒绝 Try
如果发现 TRY_SUCCESS：说明 Try 已经成功过，直接返回成功
如果没有记录：正常执行 Try
```

伪代码：

```go
func Try(xid, branchID string, amount int64) error {
    tx := LoadBranchTx(xid, branchID)

    if tx != nil {
        if tx.Status == "CANCEL_ONLY" || tx.Status == "CANCELLED" {
            return ErrTransactionCancelled
        }

        if tx.Status == "TRY_SUCCESS" {
            return nil
        }

        if tx.Status == "CONFIRMED" {
            return nil
        }
    }

    BeginTx()
    FreezeAmount(amount)
    InsertBranchTx(xid, branchID, "TRY_SUCCESS")
    Commit()

    return nil
}
```

核心点：

```text
Try 不是上来就冻结资源。
Try 必须先检查这个 xid + branch_id 是否已经被 Cancel 过。
```

---

## 六、状态机视角

正常状态流转：

```text
TRY_SUCCESS → CONFIRMED
TRY_SUCCESS → CANCELLED
```

异常状态流转：

```text
初始状态 → CANCEL_ONLY       // 空回滚
CANCEL_ONLY → Try 拒绝执行    // 防悬挂
CONFIRMED → 重复 Confirm 返回成功
CANCELLED → 重复 Cancel 返回成功
```

可以这样记：

```text
幂等：重复请求不能重复执行。
空回滚：Cancel 先到，写 CANCEL_ONLY。
悬挂：Try 后到，看到 CANCEL_ONLY 后拒绝执行。
```

---

## 七、并发安全：必须用唯一索引和条件更新

只靠“先查再更新”不安全，因为 Confirm 和 Cancel 可能并发到达。

所以要用数据库唯一索引和条件更新。

Confirm：

```sql
UPDATE tcc_branch_transaction
SET status = 'CONFIRMED', confirm_time = NOW(), updated_at = NOW()
WHERE xid = ?
  AND branch_id = ?
  AND status = 'TRY_SUCCESS';
```

Cancel：

```sql
UPDATE tcc_branch_transaction
SET status = 'CANCELLED', cancel_time = NOW(), updated_at = NOW()
WHERE xid = ?
  AND branch_id = ?
  AND status = 'TRY_SUCCESS';
```

如果影响行数为 1，说明状态变更成功。

如果影响行数为 0，再查当前状态判断：

```text
已经 CONFIRMED：Confirm 幂等成功
已经 CANCELLED：Cancel 幂等成功
已经 CANCEL_ONLY：空回滚已经处理过
其他状态：异常或并发冲突
```

核心原则：

```text
状态变更必须带旧状态条件。
不能无条件 update。
```

---

## 八、账户冻结例子

假设用户支付 100 元，TCC 用于余额冻结。

### Try

```text
检查可用余额 >= 100
可用余额 -100
冻结余额 +100
写分支事务状态 TRY_SUCCESS
```

### Confirm

```text
如果状态是 CONFIRMED：直接返回成功
如果状态是 TRY_SUCCESS：
    冻结余额 -100
    写扣款流水
    状态改 CONFIRMED
```

### Cancel

```text
如果查不到 Try 记录：
    写 CANCEL_ONLY
    返回成功

如果状态是 TRY_SUCCESS：
    冻结余额 -100
    可用余额 +100
    状态改 CANCELLED

如果状态是 CANCELLED / CANCEL_ONLY：
    直接返回成功
```

### Try 后到

```text
Try 发现已有 CANCEL_ONLY
拒绝冻结余额
```

---

## 九、面试标准回答

### Q：TCC 的幂等、空回滚、悬挂怎么处理？

**答：**

TCC 的幂等、空回滚、悬挂本质上都是分布式调用下的重复请求和乱序请求问题，所以一般会给每个分支事务维护一张分支事务状态表，用 `xid + branch_id` 做唯一键，通过状态机控制 Try、Confirm、Cancel 的执行。

幂等是指 Confirm 或 Cancel 可能重复到达。如果发现状态已经是 `CONFIRMED` 或 `CANCELLED`，就直接返回成功，不能重复扣款或重复释放资源。

空回滚是 Cancel 比 Try 先到。这个时候查不到 Try 成功记录，不能真的释放资源，也不能报错，而是插入一条 `CANCEL_ONLY` 记录，表示这个分支事务已经被取消。

悬挂是 Cancel 已经执行后，之前延迟的 Try 又到了。Try 执行前必须先查事务状态，如果发现已经存在 `CANCEL_ONLY` 或 `CANCELLED`，就拒绝执行 Try，避免资源被冻结后没人处理。

同时，状态变更不能只靠先查再更新，必须用数据库唯一索引和带状态条件的 update，比如只有 `TRY_SUCCESS` 才能变成 `CONFIRMED` 或 `CANCELLED`，这样才能处理 Confirm 和 Cancel 并发到达的问题。

---

## 十、一句话记忆

```text
TCC 的坑，本质是时序问题。
重复到达靠幂等。
Cancel 先到靠 CANCEL_ONLY。
Try 后到靠检查 CANCEL_ONLY 拒绝执行。
并发到达靠唯一索引 + 状态条件更新。
```
