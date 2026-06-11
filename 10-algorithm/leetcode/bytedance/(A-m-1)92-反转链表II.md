# 92. 反转链表 II

题目链接：`https://leetcode.cn/problems/reverse-linked-list-ii/`

---

## 一、题目描述

给你单链表的头指针 `head` 和两个整数 `left` 和 `right`，其中 `left <= right`，请你反转从位置 `left` 到位置 `right` 的链表节点，返回反转后的链表。

节点位置从 **1** 开始计数。

**示例：**

```
输入: head = [1,2,3,4,5], left = 2, right = 4
输出: [1,4,3,2,5]

输入: head = [5], left = 1, right = 1
输出: [5]
```

**约束：**
- `1 <= left <= right <= n`

---

## 二、思路：虚拟头节点 + 定位 + 局部翻转

核心想法：找到反转区间的前一个节点 `pre`，然后用"头插法"把 `[left, right]` 区间翻转到 `pre` 后面。

```
原始: dummy→1→2→3→4→5   left=2, right=4

找到 pre 指向位置1的节点（即节点1）
用 cur 指向位置2的节点（即节点2，反转起点）

头插法翻转：每次把 cur.Next 摘出来，插到 pre.Next 处
第1次: dummy→1→3→2→4→5  （把3插到1后面）
第2次: dummy→1→4→3→2→5  （把4插到1后面）

结果: 1→4→3→2→5  ✓
```

---

## 三、图解头插法

以 `1→2→3→4→5`，`left=2, right=4` 为例：

```
初始状态:
dummy→[1]→[2]→[3]→[4]→5
       ↑    ↑
      pre  cur

第1步: 取出 cur.Next = 3，插到 pre.Next 处
  摘出 next = 3
  cur.Next = next.Next  → 2→4
  next.Next = pre.Next  → 3→2
  pre.Next = next       → 1→3

链表: dummy→1→3→2→4→5
             ↑  ↑
            pre cur

第2步: 取出 cur.Next = 4，插到 pre.Next 处
  摘出 next = 4
  cur.Next = next.Next  → 2→5
  next.Next = pre.Next  → 4→3
  pre.Next = next       → 1→4

链表: dummy→1→4→3→2→5
             ↑     ↑
            pre   cur

共翻转 right-left = 2 次，结束
结果: 1→4→3→2→5  ✓
```

关键：`cur` 始终是反转区间的第一个节点，每次把它的下一个插到最前面。

---

## 四、Go 代码

```go
func reverseBetween(head *ListNode, left int, right int) *ListNode {
    dummy := &ListNode{Next: head}
    pre := dummy

    // 1. pre 走到 left 前一个位置
    for i := 1; i < left; i++ {
        pre = pre.Next
    }

    // 2. cur 指向反转区间第一个节点
    cur := pre.Next

    // 3. 头插法翻转 right-left 次
    for i := 0; i < right-left; i++ {
        next := cur.Next
        cur.Next = next.Next
        next.Next = pre.Next
        pre.Next = next
    }

    return dummy.Next
}
```

---

## 五、逐行解释

```go
// 1. 虚拟头节点，处理 left=1 时 head 本身需要变的情况
dummy := &ListNode{Next: head}
pre := dummy

// 2. pre 移到 left-1 位置（即反转区间的前驱）
for i := 1; i < left; i++ {
    pre = pre.Next
}

// 3. cur = 反转区间的第一个节点，它不动，每次把它的 Next 提到前面
cur := pre.Next

// 4. 循环 right-left 次，把 cur 后面的节点一个个插到 pre 后面
for i := 0; i < right-left; i++ {
    next := cur.Next      // 摘出要插的节点
    cur.Next = next.Next  // cur 跳过 next
    next.Next = pre.Next  // next 接上原来 pre 后面的链
    pre.Next = next       // pre 后面接 next（插入最前面）
}
```

---

## 六、例子完整走一遍

`head = [1,2,3,4,5]`，`left=2`，`right=4`

```
初始化: dummy→1→2→3→4→5, pre=dummy

pre 走 left-1=1 步:
  i=1: pre=节点1

cur = pre.Next = 节点2

循环 right-left=2 次:

i=0:
  next = cur.Next = 节点3
  cur.Next = 节点3.Next = 节点4
  节点3.Next = pre.Next = 节点2
  pre.Next = 节点3
  链表: dummy→1→3→2→4→5

i=1:
  next = cur.Next = 节点4
  cur.Next = 节点4.Next = 节点5
  节点4.Next = pre.Next = 节点3
  pre.Next = 节点4
  链表: dummy→1→4→3→2→5

返回 dummy.Next = 节点1
结果: [1,4,3,2,5]  ✓
```

---

## 七、复杂度分析

| 维度 | 值 |
|------|------|
| 时间 | O(n)，pre 走 left-1 步 + 翻转 right-left 步 |
| 空间 | O(1)，只用几个指针 |

---

## 八、容易错的点

### 1. 为什么需要虚拟头节点？

```go
// 没有 dummy，当 left=1 时，head 本身要变
// 代码里 pre.Next = next 会改变 head，但 head 变量还是旧的
// 用 dummy 作为统一的前驱，最后返回 dummy.Next 就对了
dummy := &ListNode{Next: head}
```

### 2. cur 不动，动的是 cur.Next

头插法的核心：`cur` 一直是反转区间的第一个节点，不往前走。
每轮把 `cur.Next` 摘下来，插到 `pre` 后面。

```
错误理解：cur 往后走
正确理解：cur 不走，每次摘它的下一个插到最前
```

### 3. 循环次数是 right-left

反转 n 个节点需要 n-1 次操作（区间长度 = right-left+1，操作次数 = right-left）。

### 4. 三行赋值顺序不能乱

```go
next := cur.Next      // 必须先保存，否则后面修改了就找不到了
cur.Next = next.Next  // cur 跳过 next
next.Next = pre.Next  // next 接链（先接再插）
pre.Next = next       // 最后才改 pre.Next
```

如果先改 `pre.Next = next`，再做 `next.Next = pre.Next`，就会让 next 指向自己，成环。

---

## 九、面试表达

> 用虚拟头节点 dummy，先把 pre 移到 left 前一个位置，cur 指向 left 位置节点。然后用头插法循环 right-left 次：每次把 cur.Next 摘下来，插到 pre.Next 的位置。这样 [left, right] 区间就被逆序了。时间 O(n)，空间 O(1)。

---

## 十、一句话记忆

```text
虚拟头找前驱，cur 不动头插法，摘 next 插到 pre 后，循环 right-left 次。
```

再短一点：

```text
dummy + pre定位 + cur头插，翻 right-left 次。
```
