# 剑指 Offer 55-II. 平衡二叉树

题目链接：`https://leetcode.cn/problems/ping-heng-er-cha-shu-lcof/`

---

## 一、题目描述

输入一棵二叉树的根节点，判断该树是不是平衡二叉树。

平衡二叉树的定义是：

```text
任意一个节点的左右子树高度差不超过 1。
```

注意这里是：

```text
任意一个节点
```

不是只判断根节点。

---

## 二、什么是平衡二叉树？

比如这棵树是平衡的：

```text
      3
     / \
    9  20
      /  \
     15   7
```

原因是每个节点的左右子树高度差都不超过 `1`。

这棵树不是平衡的：

```text
      1
     /
    2
   /
  3
 /
4
```

因为节点 `2` 的左子树高度是 `2`，右子树高度是 `0`，高度差是 `2`，超过了 `1`。

所以它不是平衡二叉树。

---

## 三、最容易误解的点

很多人会以为：

```text
只要根节点的左右子树高度差不超过 1，就是平衡二叉树。
```

这是错的。

正确的是：

```text
树里每一个节点的左右子树高度差都不能超过 1。
```

所以这题不是只看根节点，而是要递归检查整棵树。

---

## 四、普通思路：每个节点都算高度

可以先写一个函数：

```text
height(root)
```

用来计算一棵树的高度。

然后对每个节点判断：

```text
abs(height(root.Left) - height(root.Right)) <= 1
```

并且左右子树也必须是平衡的。

伪代码：

```text
isBalanced(root):
    if root == nil:
        return true

    leftHeight = height(root.left)
    rightHeight = height(root.right)

    return abs(leftHeight - rightHeight) <= 1
           && isBalanced(root.left)
           && isBalanced(root.right)
```

这个思路很好理解，但是会重复计算高度。

比如根节点算高度时，会遍历一遍子树；递归到子节点时，又会重新遍历同一批节点。

最坏情况下时间复杂度可能是：

```text
O(n^2)
```

---

## 五、推荐思路：后序递归，一边算高度一边判断

更好的做法是：

```text
递归函数返回当前子树高度。
如果发现当前子树不平衡，就返回 -1。
```

为什么用 `-1`？

因为正常树高不会是负数，所以可以用 `-1` 表示：

```text
这棵子树已经不平衡。
```

---

## 六、递归函数怎么定义？

定义一个函数：

```go
func checkHeight(root *TreeNode) int
```

它的含义是：

```text
如果 root 这棵树是平衡的，返回它的高度。
如果 root 这棵树不是平衡的，返回 -1。
```

这个定义非常关键。

只要这个函数定义想清楚，代码就很自然。

---

## 七、为什么是后序遍历？

判断当前节点是否平衡，需要先知道：

```text
左子树高度
右子树高度
```

所以必须先递归左右子树，再判断当前节点。

顺序是：

```text
左子树 -> 右子树 -> 当前节点
```

这就是后序遍历。

---

## 八、递归过程

对每个节点来说：

```text
1. 如果 root == nil，返回高度 0
2. 递归计算左子树高度 leftHeight
3. 如果 leftHeight == -1，说明左子树已经不平衡，直接返回 -1
4. 递归计算右子树高度 rightHeight
5. 如果 rightHeight == -1，说明右子树已经不平衡，直接返回 -1
6. 判断左右高度差是否大于 1
7. 如果大于 1，返回 -1
8. 否则返回当前树高度 max(leftHeight, rightHeight) + 1
```

---

## 九、Go 解法

```go
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func isBalanced(root *TreeNode) bool {
    return checkHeight(root) != -1
}

func checkHeight(root *TreeNode) int {
    if root == nil {
        return 0
    }

    leftHeight := checkHeight(root.Left)
    if leftHeight == -1 {
        return -1
    }

    rightHeight := checkHeight(root.Right)
    if rightHeight == -1 {
        return -1
    }

    if abs(leftHeight-rightHeight) > 1 {
        return -1
    }

    if leftHeight > rightHeight {
        return leftHeight + 1
    }
    return rightHeight + 1
}

func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}
```

---

## 十、Python3 解法

```python
# Definition for a binary tree node.
# class TreeNode:
#     def __init__(self, x):
#         self.val = x
#         self.left = None
#         self.right = None

class Solution:
    def isBalanced(self, root: TreeNode) -> bool:
        def check_height(node):
            if not node:
                return 0

            left_height = check_height(node.left)
            if left_height == -1:
                return -1

            right_height = check_height(node.right)
            if right_height == -1:
                return -1

            if abs(left_height - right_height) > 1:
                return -1

            return max(left_height, right_height) + 1

        return check_height(root) != -1
```

---

## 十一、例子走一遍

树：

```text
      3
     / \
    9  20
      /  \
     15   7
```

从底部开始算：

```text
9 是叶子节点，高度 = 1
15 是叶子节点，高度 = 1
7 是叶子节点，高度 = 1
20 的左高 = 1，右高 = 1，差值 = 0，所以高度 = 2
3 的左高 = 1，右高 = 2，差值 = 1，所以高度 = 3
```

所有节点左右高度差都不超过 `1`。

所以返回：

```text
true
```

---

再看不平衡的树：

```text
      1
     /
    2
   /
  3
 /
4
```

从底部开始：

```text
4 高度 = 1
3 左高 = 1，右高 = 0，差值 = 1，高度 = 2
2 左高 = 2，右高 = 0，差值 = 2，不平衡，返回 -1
1 发现左子树返回 -1，直接返回 -1
```

最终：

```text
checkHeight(root) == -1
```

所以：

```text
false
```

---

## 十二、复杂度分析

时间复杂度：

```text
O(n)
```

每个节点最多访问一次。

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

## 十三、容易错的点

### 1. 不能只判断根节点

错误理解：

```text
根节点左右高度差 <= 1 就是平衡树
```

正确理解：

```text
每一个节点的左右高度差都要 <= 1
```

---

### 2. 递归函数返回的是高度，不是 bool

推荐写法里：

```go
checkHeight(root) int
```

返回的是高度。

但是如果发现不平衡，就返回：

```text
-1
```

所以它同时表达两个信息：

```text
正常数字：当前子树高度
-1：当前子树不平衡
```

---

### 3. 空树高度是 0

```go
if root == nil {
    return 0
}
```

空节点没有高度，记作 `0`。

所以叶子节点的高度是：

```text
max(0, 0) + 1 = 1
```

---

### 4. 为什么要后序遍历？

因为当前节点是否平衡，依赖左右子树高度。

所以必须先拿到左右子树结果，再判断当前节点。

```text
先左，再右，最后当前节点。
```

---

## 十四、面试表达

可以这样说：

> 这题要判断的是每一个节点的左右子树高度差是否都不超过 1。普通做法是对每个节点都单独计算左右子树高度，但会有重复计算。更好的做法是后序递归：递归函数返回当前子树高度，如果发现某棵子树已经不平衡，就返回 -1 作为标记。当前节点拿到左右子树高度后，如果任意一边是 -1，或者左右高度差大于 1，就继续向上返回 -1；否则返回当前树的高度 `max(left, right) + 1`。最后判断根节点返回值是否为 -1 即可。

---

## 十五、一句话记忆

```text
平衡二叉树：后序递归算高度，左右差超过 1 就返回 -1。
```

再记这个函数定义：

```text
checkHeight(root)：平衡就返回高度，不平衡就返回 -1。
```
