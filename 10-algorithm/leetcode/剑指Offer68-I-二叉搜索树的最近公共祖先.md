# 剑指 Offer 68-I. 二叉搜索树的最近公共祖先

题目链接：`https://leetcode.cn/problems/er-cha-sou-suo-shu-de-zui-jin-gong-gong-zu-xian-lcof/description/`

---

## 一、题目描述

给定一个二叉搜索树，找到该树中两个指定节点的最近公共祖先。

最近公共祖先的定义：

```text
对于有根树 T 的两个节点 p、q，最近公共祖先表示为一个节点 x，满足：
x 是 p、q 的祖先，并且 x 的深度尽可能大。
```

一个节点也可以是它自己的祖先。

---

## 二、先抓住关键条件：二叉搜索树

这题最重要的条件是：

```text
二叉搜索树 BST
```

二叉搜索树满足：

```text
左子树所有节点值 < 当前节点值 < 右子树所有节点值
```

例如：

```text
        6
       / \
      2   8
     / \ / \
    0  4 7  9
      / \
     3   5
```

对于根节点 `6`：

```text
左边都小于 6
右边都大于 6
```

因为 BST 有大小关系，所以可以通过比较节点值决定往左走还是往右走。

---

## 三、什么是最近公共祖先？

### 情况一：p 和 q 分别在当前节点两边

比如：

```text
p = 2, q = 8
```

从根节点 `6` 看：

```text
2 < 6
8 > 6
```

一个在左边，一个在右边，说明它们在 `6` 这里分叉。

所以最近公共祖先是：

```text
6
```

---

### 情况二：当前节点本身就是 p 或 q

比如：

```text
p = 2, q = 4
```

从 `6` 开始，它们都小于 `6`，所以去左边。

来到 `2`：

```text
p 就是当前节点 2
q = 4 在 2 的右子树
```

因为一个节点可以是自己的祖先，所以最近公共祖先就是：

```text
2
```

---

## 四、核心思路

从 `root` 开始往下走。

对于当前节点 `root`，有三种情况。

---

### 情况一：p 和 q 都比 root 小

```text
p.Val < root.Val
q.Val < root.Val
```

说明 `p` 和 `q` 都在左子树，最近公共祖先一定在左边。

所以：

```text
root = root.Left
```

---

### 情况二：p 和 q 都比 root 大

```text
p.Val > root.Val
q.Val > root.Val
```

说明 `p` 和 `q` 都在右子树，最近公共祖先一定在右边。

所以：

```text
root = root.Right
```

---

### 情况三：p 和 q 分叉，或者 root 就是 p / q

只要不满足“都在左边”或者“都在右边”，说明当前节点就是答案。

可能是：

```text
p 在左边，q 在右边
q 在左边，p 在右边
root 本身就是 p
root 本身就是 q
```

这时直接返回：

```text
root
```

---

## 五、一句话思路

```text
在 BST 里，从 root 往下走：
p、q 都小，往左走；
p、q 都大，往右走；
一旦分叉，当前节点就是最近公共祖先。
```

---

## 六、Go 递归写法

```go
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func lowestCommonAncestor(root, p, q *TreeNode) *TreeNode {
    if p.Val < root.Val && q.Val < root.Val {
        return lowestCommonAncestor(root.Left, p, q)
    }

    if p.Val > root.Val && q.Val > root.Val {
        return lowestCommonAncestor(root.Right, p, q)
    }

    return root
}
```

---

## 七、Go 迭代写法

这题的迭代写法也很自然，因为每次只需要根据大小关系决定往哪边走。

```go
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func lowestCommonAncestor(root, p, q *TreeNode) *TreeNode {
    for root != nil {
        if p.Val < root.Val && q.Val < root.Val {
            root = root.Left
        } else if p.Val > root.Val && q.Val > root.Val {
            root = root.Right
        } else {
            return root
        }
    }

    return nil
}
```

---

## 八、Python3 写法

```python
# Definition for a binary tree node.
# class TreeNode:
#     def __init__(self, x):
#         self.val = x
#         self.left = None
#         self.right = None

class Solution:
    def lowestCommonAncestor(
        self,
        root: 'TreeNode',
        p: 'TreeNode',
        q: 'TreeNode'
    ) -> 'TreeNode':
        while root:
            if p.val < root.val and q.val < root.val:
                root = root.left
            elif p.val > root.val and q.val > root.val:
                root = root.right
            else:
                return root
```

---

## 九、例子走一遍

树：

```text
        6
       / \
      2   8
     / \ / \
    0  4 7  9
      / \
     3   5
```

### 例子一：p = 2, q = 8

从 `6` 开始：

```text
2 < 6
8 > 6
```

一个在左，一个在右，分叉了。

所以答案是：

```text
6
```

---

### 例子二：p = 2, q = 4

从 `6` 开始：

```text
2 < 6
4 < 6
```

都在左边，去左子树。

来到 `2`：

```text
p 就是 2
q = 4 在 2 的右边
```

一个是当前节点，一个在当前节点下面，所以答案是：

```text
2
```

---

## 十、复杂度分析

时间复杂度：

```text
O(h)
```

`h` 是树高。

如果树平衡：

```text
O(log n)
```

如果树退化成链表：

```text
O(n)
```

空间复杂度：

递归写法：

```text
O(h)
```

迭代写法：

```text
O(1)
```

---

## 十一、和普通二叉树最近公共祖先的区别

普通二叉树没有大小关系，所以不能根据值判断方向。

普通二叉树通常要递归左右子树分别找 `p` 和 `q`。

但 BST 有大小关系，所以可以直接判断：

```text
都小：去左
都大：去右
分叉：当前节点
```

所以这题比普通二叉树的最近公共祖先简单。

---

## 十二、容易错的点

### 1. 不要忽略 BST 条件

这题不是普通二叉树，而是二叉搜索树。

如果忽略 BST 条件，就会写成更复杂的普通二叉树 LCA 解法。

---

### 2. root 本身可以是答案

如果当前节点就是 `p` 或 `q`，它也可以是最近公共祖先。

例如：

```text
p = 2, q = 4
```

答案是：

```text
2
```

---

### 3. 不需要保证 p 小于 q

代码不需要先排序 `p` 和 `q`。

只要判断：

```text
p 和 q 是否都小于 root
p 和 q 是否都大于 root
```

剩下情况直接返回 `root`。

---

## 十三、面试表达

可以这样说：

> 这题的关键是它给的是二叉搜索树。BST 的性质是左子树节点值都小于当前节点，右子树节点值都大于当前节点。所以从 root 开始，如果 p 和 q 都小于 root，说明最近公共祖先一定在左子树；如果 p 和 q 都大于 root，说明一定在右子树；否则说明 p 和 q 在当前节点两侧分叉，或者当前节点本身就是 p 或 q，这时当前节点就是最近公共祖先。时间复杂度是 O(h)，h 是树高，迭代写法空间复杂度是 O(1)。

---

## 十四、一句话记忆

```text
BST 最近公共祖先：都小往左，都大往右，一分叉当前就是答案。
```
