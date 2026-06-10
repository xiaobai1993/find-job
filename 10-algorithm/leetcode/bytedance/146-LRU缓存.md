# 146. LRU 缓存

题目链接：`https://leetcode.cn/problems/lru-cache/description/`

---

## 一、题目描述

设计一个数据结构，支持两个操作：

| 操作 | 说明 |
|------|------|
| `get(key)` | key 存在则返回值，不存在返回 -1 |
| `put(key, value)` | 插入或更新键值对 |

**容量限制**：当键值对数量超过 `capacity` 时，淘汰**最久没被使用**的那个。

**"最近最少使用"**的意思：`get` 和 `put` 都算"使用"，最久没碰的就是要淘汰的。

**要求**：`get` 和 `put` 都是 O(1)

---

## 二、需要什么数据结构

O(1) 查找 → 哈希表
O(1) 淘汰最旧 → 有序队列，最旧的在头部，最新的在尾部

但普通队列不够，因为 `get` 一个中间元素后，要把它挪到尾部（标记为最近使用），普通队列挪中间元素是 O(n)。

**答案：哈希表 + 双向链表**

```
哈希表：key → 链表节点指针     → O(1) 查找
双向链表：按访问顺序排列        → O(1) 移动/删除节点

链表头部 = 最久没用的（淘汰对象）
链表尾部 = 最近刚用的
```

图示（capacity = 3）：

```
put(1,10) put(2,20) put(3,30) 后：

哈希表: {1 → node1, 2 → node2, 3 → node3}

链表（从旧到新）：
  head ↔ [1,10] ↔ [2,20] ↔ [3,30] ↔ tail
          ↑最旧              ↑最新

get(1) 后，1 被挪到尾部：

  head ↔ [2,20] ↔ [3,30] ↔ [1,10] ↔ tail
          ↑最旧              ↑最新

put(4,40)，容量满了，淘汰最旧的 2：

  head ↔ [3,30] ↔ [1,10] ↔ [4,40] ↔ tail
          ↑最旧              ↑最新
```

---

## 三、双向链表节点

```go
type node struct {
    key  int        // 删除时需要知道 key 来删哈希表
    val  int
    prev *node
    next *node
}
```

为什么要存 `key`？淘汰头节点时，要用 `key` 去删哈希表里的记录。

---

## 四、为什么用哨兵头尾

双向链表操作要处理 `nil` 判断，很烦。加两个哨兵节点（dummy head / dummy tail），所有真实节点都在它们中间，永远不会空。

```
head(哨兵) ↔ [真实节点...] ↔ tail(哨兵)

插入时：往 tail 前面插
删除时：删 head 的下一个
永远不用判 nil
```

---

## 五、三个链表操作

### 1. 删除节点 — O(1)

```go
func (c *LRUCache) remove(n *node) {
    n.prev.next = n.next
    n.next.prev = n.prev
}
```

```
删除 [2,20]：

  [1,10] ↔ [2,20] ↔ [3,30]
            ↓
  [1,10] ↔ [3,30]
```

### 2. 插入到尾部（标记为最近使用）— O(1)

```go
func (c *LRUCache) pushBack(n *node) {
    n.prev = c.tail.prev
    n.next = c.tail
    c.tail.prev.next = n
    c.tail.prev = n
}
```

```
插入 [4,40] 到尾部：

  ... ↔ [3,30] ↔ tail
        ↓
  ... ↔ [3,30] ↔ [4,40] ↔ tail
```

### 3. 移到尾部 = 删了再插

```go
func (c *LRUCache) moveToBack(n *node) {
    c.remove(n)
    c.pushBack(n)
}
```

---

## 六、Go 完整代码

```go
type node struct {
    key  int
    val  int
    prev *node
    next *node
}

type LRUCache struct {
    cap     int
    m       map[int]*node
    head    *node // 哨兵头，next 指向最旧
    tail    *node // 哨兵尾，prev 指向最新
}

func Constructor(capacity int) LRUCache {
    c := LRUCache{
        cap:  capacity,
        m:    make(map[int]*node),
        head: &node{},
        tail: &node{},
    }
    c.head.next = c.tail
    c.tail.prev = c.head
    return c
}

func (c *LRUCache) Get(key int) int {
    n, ok := c.m[key]
    if !ok {
        return -1
    }
    c.moveToBack(n) // 访问了，挪到尾部
    return n.val
}

func (c *LRUCache) Put(key int, value int) {
    // key 已存在：更新值，挪到尾部
    if n, ok := c.m[key]; ok {
        n.val = value
        c.moveToBack(n)
        return
    }

    // key 不存在：新建节点
    n := &node{key: key, val: value}
    c.m[key] = n
    c.pushBack(n)

    // 超容量：淘汰最旧的（head.next）
    if len(c.m) > c.cap {
        oldest := c.head.next
        c.remove(oldest)
        delete(c.m, oldest.key)
    }
}

func (c *LRUCache) remove(n *node) {
    n.prev.next = n.next
    n.next.prev = n.prev
}

func (c *LRUCache) pushBack(n *node) {
    n.prev = c.tail.prev
    n.next = c.tail
    c.tail.prev.next = n
    c.tail.prev = n
}

func (c *LRUCache) moveToBack(n *node) {
    c.remove(n)
    c.pushBack(n)
}
```

---

## 七、例子完整走一遍

`capacity = 2`

```
操作              链表（旧→新）           哈希表              返回值
------------------------------------------------------------------------
put(1,1)         [1,1]                  {1:n1}              -
put(2,2)         [1,1] ↔ [2,2]          {1:n1, 2:n2}        -
get(1)           [2,2] ↔ [1,1]          不变                 1
put(3,3)         淘汰1 → [2,2] ↔ [3,3]  {2:n2, 3:n3}        -
get(2)           [3,3] ↔ [2,2]          不变                 2
put(4,4)         淘汰3 → [2,2] ↔ [4,4]  {2:n2, 4:n4}        -
get(1)           -                      不含1                -1
get(3)           -                      不含3                -1
get(4)           [2,2] ↔ [4,4]          不变                 4
```

---

## 八、复杂度分析

| 操作 | 时间 | 空间 |
|------|------|------|
| Get  | O(1) | - |
| Put  | O(1) | - |
| 整体 | -    | O(capacity) |

---

## 九、容易错的点

### 1. 节点必须存 key

淘汰时 `delete(c.m, oldest.key)`，如果节点不存 key，就没法删哈希表记录。

### 2. 哨兵节点要互相指向

```go
c.head.next = c.tail
c.tail.prev = c.head
```

忘了这步，空链表时 `pushBack` 会 nil 指针 panic。

### 3. Put 时 key 已存在，要更新值

不是只挪位置，`n.val = value` 也要更新。

### 4. remove 的顺序

```go
// ✓ 正确：先改别人的指针，再改自己的（顺序无所谓因为不依赖自己）
n.prev.next = n.next
n.next.prev = n.prev

// ✗ 如果先断开 n 自己的指针，后面的赋值就 nil 了
```

### 5. 先删后加，不是先加后删

超容量时：先删最旧节点，再插入新节点。顺序反了会导致多一个节点。

---

## 十、面试表达

> LRU 缓存需要 O(1) 的 get 和 put。用哈希表实现 O(1) 查找，双向链表维护访问顺序。链表头部是最久没用的，尾部是最近刚用的。get 时把节点移到链表尾部；put 时如果 key 存在就更新值并移到尾部，如果不存在就新建节点插到尾部，超容量就删链表头部的节点。用哨兵头尾节点简化边界处理。

---

## 十一、一句话记忆

```text
哈希表查，双链表排；头旧尾新，get/put 都往尾挪，满了删头。
```

再短一点：

```text
哈希+双链表，访问了挪尾，满了删头。
```
