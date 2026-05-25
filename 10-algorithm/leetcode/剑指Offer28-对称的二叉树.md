# 剑指 Offer 28. 对称的二叉树

题目链接：`https://leetcode.cn/problems/dui-cheng-de-er-cha-shu-lcof/`

---

## 一、题目描述

给定一棵二叉树，判断它是不是对称的。

如果一棵二叉树和它的镜像一样，那么它是对称的。

示例：

```text
        1
       / \
      2   2
     / \ / \
    3  4 4  3
```

这棵树是对称的。

因为左子树和右子树互为镜像。

---

再看一个不对称的例子：

```text
        1
       / \
      2   2
       \   \
        3   3
```

这棵树不是对称的。

虽然根节点左右都是 `2`，但是结构不是镜像。

---

## 二、你的思路为什么已经接近了？

你写的代码：

```go
func checkSymmetricTree(root *TreeNode) bool {
    if root == nil {
        return true
    }
    if root.Left == nil && root.Right != nil {
        return false
    }
    if root.Left != nil && root.Right == nil {
        return false
    }
    if root.Left == nil && root.Right == nil {
        return true
    }
}
```

这个思路已经判断了第一层结构：

```text
左为空、右不为空：不对称
左不为空、右为空：不对称
左右都为空：对称
```

这些判断本身没问题。

但问题是：

```text
它只判断了 root 的左右孩子，没有继续判断更深层是否镜像。
```

比如：

```text
        1
       / \
      2   2
       \   \
        3   3
```

根节点的左右孩子都存在，看起来第一层没问题。

但它不是对称的，因为镜像关系应该是：

```text
左子树的右边，要对应右子树的左边
```

这里右子树的左边是空，所以不对称。

---

## 三、这题关键：递归比较两个节点

这题不是单独判断一棵子树是否对称。

真正要判断的是：

```text
左子树和右子树是否互为镜像。
```

所以通常要新写一个递归函数：

```go
func check(left, right *TreeNode) bool
```

它的含义是：

```text
判断 left 这棵子树 和 right 这棵子树 是否互为镜像。
```

这就是为什么这题需要一个新的 helper 函数。

---

## 四、为什么不能直接递归调用原函数？

有些人会想写：

```go
return checkSymmetricTree(root.Left) && checkSymmetricTree(root.Right)
```

这其实不对。

因为这句话判断的是：

```text
左子树自己是否对称
右子树自己是否对称
```

但题目真正要求的是：

```text
左子树和右子树是否互为镜像
```

举个例子：

```text
        1
       / \
      2   2
     /     \
    3       3
```

整棵树是对称的。

但是左子树：

```text
  2
 /
3
```

它自己不是对称的。

右子树：

```text
2
 \
  3
```

它自己也不是对称的。

所以如果递归调用原函数，会误判。

---

## 五、递归函数怎么定义？

定义：

```go
func check(left, right *TreeNode) bool
```

含义：

```text
判断 left 和 right 是否互为镜像。
```

然后分情况讨论。

---

### 情况一：两个节点都为空

```go
if left == nil && right == nil {
    return true
}
```

两边都没有节点，说明这部分是对称的。

---

### 情况二：一个为空，一个不为空

```go
if left == nil || right == nil {
    return false
}
```

结构不一样，不对称。

---

### 情况三：两个都不为空，但值不同

```go
if left.Val != right.Val {
    return false
}
```

节点值不一样，不对称。

---

### 情况四：当前节点值一样，继续比较下一层

这是本题最关键的地方。

镜像比较不是普通的左右比较，而是交叉比较：

```text
left.Left  对 right.Right
left.Right 对 right.Left
```

代码：

```go
return check(left.Left, right.Right) &&
       check(left.Right, right.Left)
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
func checkSymmetricTree(root *TreeNode) bool {
    if root == nil {
        return true
    }

    return check(root.Left, root.Right)
}

func check(left, right *TreeNode) bool {
    if left == nil && right == nil {
        return true
    }

    if left == nil || right == nil {
        return false
    }

    if left.Val != right.Val {
        return false
    }

    return check(left.Left, right.Right) &&
        check(left.Right, right.Left)
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
    def checkSymmetricTree(self, root: TreeNode) -> bool:
        if not root:
            return True

        def check(left, right):
            if not left and not right:
                return True

            if not left or not right:
                return False

            if left.val != right.val:
                return False

            return check(left.left, right.right) and check(left.right, right.left)

        return check(root.left, root.right)
```

---

## 八、例子走一遍

树：

```text
        1
       / \
      2   2
     / \ / \
    3  4 4  3
```

一开始：

```text
check(root.Left, root.Right)
```

也就是比较两个 `2`。

它们值相等，继续比较：

```text
check(左边 2 的 left=3, 右边 2 的 right=3)
check(左边 2 的 right=4, 右边 2 的 left=4)
```

两组都相等，并且再往下都是空节点。

所以整棵树是对称的。

---

再看不对称的树：

```text
        1
       / \
      2   2
       \   \
        3   3
```

一开始比较两个 `2`，值相等。

继续交叉比较：

```text
check(左边 2 的 left=nil, 右边 2 的 right=3)
```

一个为空，一个不为空，返回 `false`。

所以整棵树不是对称的。

---

## 九、复杂度分析

时间复杂度：

```text
O(n)
```

每个节点最多被访问一次。

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

## 十、容易错的点

### 1. 不要只判断第一层

只判断：

```go
root.Left == nil
root.Right == nil
```

是不够的。

还要继续判断更深层是否镜像。

---

### 2. 不要递归调用原函数判断左右子树自己是否对称

错误：

```go
return checkSymmetricTree(root.Left) && checkSymmetricTree(root.Right)
```

这判断的是左右子树自己是否对称。

正确目标是：

```text
左子树和右子树是否互为镜像。
```

所以要用双参数递归函数。

---

### 3. 递归比较要交叉

正确：

```go
return check(left.Left, right.Right) &&
    check(left.Right, right.Left)
```

错误：

```go
return check(left.Left, right.Left) &&
    check(left.Right, right.Right)
```

那是普通同方向比较，不是镜像比较。

---

### 4. 先判断 nil，再取 Val

必须先判断：

```go
if left == nil || right == nil {
    return false
}
```

再访问：

```go
left.Val
right.Val
```

否则会出现空指针问题。

---

## 十一、什么时候要新写递归函数？

可以这样判断。

如果递归时只处理一棵树：

```text
求树高
遍历整棵树
判断平衡二叉树
```

通常可以用一个参数：

```go
func dfs(root *TreeNode)
```

如果递归时需要同时比较两棵树或两个节点：

```text
判断对称二叉树
判断两棵树是否相同
判断一棵树是不是另一棵树的子结构
```

通常需要写双参数 helper：

```go
func check(a, b *TreeNode) bool
```

---

## 十二、面试表达

可以这样说：

> 这题的关键是对称二叉树的本质是左右子树互为镜像，所以不能只递归判断左子树自己是否对称、右子树自己是否对称，而是要同时比较两个节点。我会写一个辅助函数 `check(left, right)`，表示判断 `left` 和 `right` 是否互为镜像。如果两个节点都为空，返回 true；如果只有一个为空，返回 false；如果两个节点值不同，也返回 false；否则继续交叉比较 `left.Left` 和 `right.Right`，以及 `left.Right` 和 `right.Left`。最后从 `check(root.Left, root.Right)` 开始即可。

---

## 十三、一句话记忆

```text
对称二叉树：比较两个子树是否镜像，左左对右右，左右对右左。
```

核心代码：

```go
return check(left.Left, right.Right) &&
    check(left.Right, right.Left)
```
