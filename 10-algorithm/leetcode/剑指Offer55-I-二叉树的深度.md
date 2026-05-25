# 剑指 Offer 55-I. 二叉树的深度

题目链接：`https://leetcode.cn/problems/er-cha-shu-de-shen-du-lcof/description/`

---

## 一、题目描述

输入一棵二叉树的根节点，求该树的深度。

二叉树的深度是指：

```text
从根节点到最远叶子节点的最长路径上的节点数。
```

示例：

```text
    3
   / \
  9  20
     / \
    15  7
```

最大深度是：

```text
3
```

因为最长路径可以是：

```text
3 -> 20 -> 15
```

或者：

```text
3 -> 20 -> 7
```

路径上一共有 `3` 个节点。

---

## 二、你的 Go 提交

你的代码：

```go
func calculateDepth(root *TreeNode) int {
    if root == nil {
        return 0
    }
    return max(calculateDepth(root.Left), calculateDepth(root.Right)) + 1
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

这个提交是正确的。

这是这题最标准、最简洁的递归写法。

---

## 三、递归函数的含义

这一句最重要：

```go
func calculateDepth(root *TreeNode) int
```

它的含义是：

```text
返回以 root 为根的这棵树的最大深度。
```

那么对于当前节点来说：

```text
当前树最大深度 = max(左子树最大深度, 右子树最大深度) + 1
```

其中 `+1` 表示：

```text
当前 root 自己这一层
```

---

## 四、为什么空节点返回 0？

代码：

```go
if root == nil {
    return 0
}
```

空树没有节点，所以深度是 `0`。

这样叶子节点就很好理解了。

叶子节点的左右孩子都是空：

```text
左子树深度 = 0
右子树深度 = 0
```

所以叶子节点深度是：

```text
max(0, 0) + 1 = 1
```

这正好符合定义：

```text
叶子节点本身深度是 1。
```

---

## 五、核心思路

二叉树递归最常见的思路是：

```text
一棵树的答案，可以由左子树答案和右子树答案推出来。
```

这题就是：

```text
root 的深度 = max(root.Left 的深度, root.Right 的深度) + 1
```

伪代码：

```text
depth(root):
    if root == nil:
        return 0

    leftDepth = depth(root.left)
    rightDepth = depth(root.right)

    return max(leftDepth, rightDepth) + 1
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
func calculateDepth(root *TreeNode) int {
    if root == nil {
        return 0
    }

    return max(calculateDepth(root.Left), calculateDepth(root.Right)) + 1
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

---

## 七、Python3 解法

```python
# Definition for a binary tree node.
# class TreeNode:
#     def __init__(self, x):
#         self.val = x
#         self.left = None
#         self.right = None

class Solution:
    def calculateDepth(self, root: TreeNode) -> int:
        if not root:
            return 0

        return max(
            self.calculateDepth(root.left),
            self.calculateDepth(root.right)
        ) + 1
```

---

## 八、例子走一遍

树：

```text
    3
   / \
  9  20
     / \
    15  7
```

从底部开始：

```text
9 的左右孩子都是 nil，所以深度 = max(0, 0) + 1 = 1
15 的左右孩子都是 nil，所以深度 = 1
7 的左右孩子都是 nil，所以深度 = 1
20 的左深度 = 1，右深度 = 1，所以深度 = max(1, 1) + 1 = 2
3 的左深度 = 1，右深度 = 2，所以深度 = max(1, 2) + 1 = 3
```

最终返回：

```text
3
```

---

## 九、为什么这题适合递归？

因为树本身就是递归结构。

一棵树可以看成：

```text
当前节点 + 左子树 + 右子树
```

要求整棵树的最大深度，就必须先知道：

```text
左子树最大深度
右子树最大深度
```

然后取更大的那个，再加上当前节点这一层。

所以递归天然适合这题。

---

## 十、复杂度分析

时间复杂度：

```text
O(n)
```

每个节点只访问一次。

空间复杂度：

```text
O(h)
```

`h` 是树高，来自递归调用栈。

如果树平衡：

```text
O(log n)
```

如果树退化成链表：

```text
O(n)
```

---

## 十一、容易错的点

### 1. 空树深度是 0

```go
if root == nil {
    return 0
}
```

不要返回 `1`。

如果空树返回 `1`，叶子节点就会变成：

```text
max(1, 1) + 1 = 2
```

明显不对。

---

### 2. 要取左右子树更大的深度

正确：

```go
return max(calculateDepth(root.Left), calculateDepth(root.Right)) + 1
```

不是左右相加。

因为深度是一条最长路径，不是所有节点数量。

---

### 3. `+1` 不能忘

`+1` 表示当前节点这一层。

如果不加 `1`，整棵树深度会少一层。

---

### 4. 函数定义要想清楚

递归题最重要的是先定义清楚函数含义：

```text
calculateDepth(root)：返回 root 这棵树的最大深度。
```

然后代码自然就是：

```text
左深度、右深度取最大，再加 1。
```

---

## 十二、面试表达

可以这样说：

> 这题用递归。递归函数 `calculateDepth(root)` 的含义是返回以 `root` 为根的树的最大深度。如果 `root` 为空，说明空树深度为 0；否则分别递归计算左右子树深度，然后取较大值再加 1，这个 1 表示当前节点这一层。这样每个节点只访问一次，时间复杂度是 O(n)，空间复杂度是 O(h)，h 是树高。

---

## 十三、一句话记忆

```text
二叉树最大深度：左深度和右深度取最大，再加当前节点这一层。
```

再记住递归函数定义：

```text
calculateDepth(root)：返回 root 这棵树的最大深度。
```
