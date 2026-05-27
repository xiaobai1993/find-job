# 剑指 Offer 32-II. 从上到下打印二叉树 II

题目链接：`https://leetcode.cn/problems/cong-shang-dao-xia-da-yin-er-cha-shu-ii-lcof/`

---

## 一、题目描述

从上到下按层打印二叉树，同一层的节点按照从左到右的顺序打印。

每一层打印到一行。

也就是说，返回结果是一个二维数组：

```text
[
  [第 1 层节点值],
  [第 2 层节点值],
  [第 3 层节点值],
  ...
]
```

示例：

```text
给定二叉树：

    3
   / \
  9  20
     / \
    15  7
```

返回：

```text
[
  [3],
  [9, 20],
  [15, 7]
]
```

---

## 二、这题是什么类型？

这题就是典型的：

```text
二叉树层序遍历
```

也叫：

```text
BFS，广度优先搜索
```

它不是递归优先的前序、中序、后序遍历，而是按层处理：

```text
第 1 层 -> 第 2 层 -> 第 3 层
```

所以最适合用：

```text
队列
```

---

## 三、为什么用队列？

队列的特点是：

```text
先进先出
```

二叉树层序遍历也是：

```text
先进入队列的节点，先被处理。
```

例如：

```text
    3
   / \
  9  20
```

先把 `3` 放进队列。

处理 `3` 时，把它的孩子 `9` 和 `20` 放进去。

因为队列先进先出，所以后面会先处理 `9`，再处理 `20`。

这就保证了从左到右的顺序。

---

## 四、这题最关键的一行：size := len(queue)

很多人记得用队列，但是不知道怎么分层。

分层的关键是：

```go
size := len(queue)
```

这表示：

```text
当前这一层有多少个节点
```

为什么要提前取 `size`？

因为处理当前层节点的时候，会把下一层节点加入队列。

如果不提前固定当前层大小，就会分不清：

```text
哪些节点属于当前层
哪些节点属于下一层
```

所以模板是：

```go
for len(queue) > 0 {
    size := len(queue)
    level := []int{}

    for i := 0; i < size; i++ {
        // 只处理当前层的 size 个节点
    }

    result = append(result, level)
}
```

---

## 五、核心思路

整体步骤：

```text
1. 如果 root 为空，返回空二维数组
2. 创建队列，把 root 放进去
3. 当队列不为空时：
   3.1 先记录当前队列长度 size，这就是当前层节点数
   3.2 创建 level 数组保存当前层节点值
   3.3 循环 size 次，每次弹出队头节点
   3.4 把节点值加入 level
   3.5 如果有左孩子，左孩子入队
   3.6 如果有右孩子，右孩子入队
   3.7 当前层处理完后，把 level 加入 result
4. 返回 result
```

伪代码：

```text
levelOrder(root):
    if root == nil:
        return []

    queue = [root]
    result = []

    while queue not empty:
        size = len(queue)
        level = []

        repeat size times:
            node = queue.pop_front()
            level.append(node.val)

            if node.left exists:
                queue.push_back(node.left)
            if node.right exists:
                queue.push_back(node.right)

        result.append(level)

    return result
```

---

## 六、Go 解法

```go
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func levelOrder(root *TreeNode) [][]int {
    if root == nil {
        return [][]int{}
    }

    queue := []*TreeNode{root}
    result := [][]int{}

    for len(queue) > 0 {
        size := len(queue)
        level := []int{}

        for i := 0; i < size; i++ {
            node := queue[0]
            queue = queue[1:]

            level = append(level, node.Val)

            if node.Left != nil {
                queue = append(queue, node.Left)
            }
            if node.Right != nil {
                queue = append(queue, node.Right)
            }
        }

        result = append(result, level)
    }

    return result
}
```

---

## 七、Go 代码拆开理解

### 1. 空树返回空二维数组

```go
if root == nil {
    return [][]int{}
}
```

因为函数返回值是：

```go
[][]int
```

所以空树返回：

```go
[][]int{}
```

---

### 2. 队列里放节点，不是放值

```go
queue := []*TreeNode{root}
```

队列里放的是：

```go
*TreeNode
```

因为后面还需要访问：

```go
node.Left
node.Right
```

如果只放 `node.Val`，就找不到孩子节点了。

---

### 3. 取队头和出队

```go
node := queue[0]
queue = queue[1:]
```

含义是：

```text
queue[0]：取队头节点
queue = queue[1:]：把队头从队列里移除
```

---

### 4. 把下一层节点加入队列

```go
if node.Left != nil {
    queue = append(queue, node.Left)
}
if node.Right != nil {
    queue = append(queue, node.Right)
}
```

注意顺序是：

```text
先左后右
```

这样同一层才是从左到右。

---

## 八、Python3 解法

### 写法一：普通列表当队列

```python
# Definition for a binary tree node.
# class TreeNode:
#     def __init__(self, x):
#         self.val = x
#         self.left = None
#         self.right = None

class Solution:
    def levelOrder(self, root: TreeNode) -> List[List[int]]:
        if not root:
            return []

        queue = [root]
        result = []

        while queue:
            size = len(queue)
            level = []

            for _ in range(size):
                node = queue.pop(0)
                level.append(node.val)

                if node.left:
                    queue.append(node.left)
                if node.right:
                    queue.append(node.right)

            result.append(level)

        return result
```

---

### 写法二：使用 deque

Python 中更推荐用 `deque`，因为 `pop(0)` 会移动数组元素，效率不如队列结构。

```python
from collections import deque

class Solution:
    def levelOrder(self, root: TreeNode) -> List[List[int]]:
        if not root:
            return []

        queue = deque([root])
        result = []

        while queue:
            size = len(queue)
            level = []

            for _ in range(size):
                node = queue.popleft()
                level.append(node.val)

                if node.left:
                    queue.append(node.left)
                if node.right:
                    queue.append(node.right)

            result.append(level)

        return result
```

---

## 九、例子走一遍

树：

```text
    3
   / \
  9  20
     / \
    15  7
```

初始：

```text
queue = [3]
result = []
```

---

### 第一轮

当前队列：

```text
[3]
```

当前层大小：

```text
size = 1
```

处理 `3`：

```text
level = [3]
把 9、20 入队
queue = [9, 20]
```

当前层结束：

```text
result = [[3]]
```

---

### 第二轮

当前队列：

```text
[9, 20]
```

当前层大小：

```text
size = 2
```

处理 `9`：

```text
level = [9]
```

处理 `20`：

```text
level = [9, 20]
把 15、7 入队
queue = [15, 7]
```

当前层结束：

```text
result = [[3], [9, 20]]
```

---

### 第三轮

当前队列：

```text
[15, 7]
```

当前层大小：

```text
size = 2
```

处理 `15`、`7`：

```text
level = [15, 7]
queue = []
```

当前层结束：

```text
result = [[3], [9, 20], [15, 7]]
```

队列为空，结束。

---

## 十、复杂度分析

时间复杂度：

```text
O(n)
```

每个节点只会入队一次、出队一次。

空间复杂度：

```text
O(n)
```

最坏情况下，队列中可能存放一层的大量节点，结果数组也会保存所有节点值。

---

## 十一、容易错的点

### 1. 忘记用 `size` 固定当前层

错误思路：

```go
for len(queue) > 0 {
    node := queue[0]
    queue = queue[1:]
    // 直接一直处理
}
```

这样只能得到一维结果，或者分不清每一层。

正确做法：

```go
size := len(queue)
for i := 0; i < size; i++ {
    // 只处理当前层
}
```

---

### 2. 队列里要放节点指针

错误：

```go
queue := []int{root.Val}
```

这样拿不到左右孩子。

正确：

```go
queue := []*TreeNode{root}
```

---

### 3. 出队写法别忘了移动队列

取队头：

```go
node := queue[0]
```

还要出队：

```go
queue = queue[1:]
```

如果忘了 `queue = queue[1:]`，会一直处理同一个节点。

---

### 4. 先左后右入队

正确顺序：

```go
if node.Left != nil {
    queue = append(queue, node.Left)
}
if node.Right != nil {
    queue = append(queue, node.Right)
}
```

这样结果才是从左到右。

---

## 十二、面试表达

可以这样说：

> 这题是典型的二叉树层序遍历，也就是 BFS。因为要一层一层从左到右处理节点，所以使用队列。先把根节点放入队列，然后每一轮先记录当前队列长度 `size`，这个 `size` 就是当前层的节点个数。接着循环 `size` 次，每次从队头取出节点，把节点值加入当前层数组，并把它的左右孩子加入队列。当前层处理完后，把这一层数组加入最终结果。这样直到队列为空，就得到了按层分组的结果。

---

## 十三、一句话记忆

```text
层序遍历：队列放节点，每轮先取 size，循环 size 次处理当前层。
```
