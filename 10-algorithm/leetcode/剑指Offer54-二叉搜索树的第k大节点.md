# 剑指 Offer 54. 二叉搜索树的第 k 大节点

题目链接：`https://leetcode.cn/problems/er-cha-sou-suo-shu-de-di-kda-jie-dian-lcof/description/`

---

## 一、题目描述

给定一棵二叉搜索树，请找出其中第 `k` 大的节点值。

例如：

```text
      5
     / \
    3   6
   / \
  2   4
 /
1
```

如果：

```text
k = 3
```

第 3 大的节点值是：

```text
4
```

---

## 二、这题的关键：BST 中序遍历有序

二叉搜索树 BST 满足：

```text
左子树所有节点值 < 当前节点值 < 右子树所有节点值
```

所以 BST 的中序遍历：

```text
左 -> 根 -> 右
```

会得到一个升序数组。

例如上面的树，中序遍历结果是：

```text
[1, 2, 3, 4, 5, 6]
```

那么：

```text
第 k 大 = 升序数组里的倒数第 k 个
```

---

## 三、你的 Go 提交

你的代码：

```go
func findTargetNode(root *TreeNode, cnt int) int {
    result := visit(root)
    return result[len(result)-cnt]
}

func visit(root * TreeNode)[]int {
    if root == nil {
        return []int{}
    }
    return append(append(visit(root.Left),[]int{root.Val}...),visit(root.Right)...)
}
```

这个思路是正确的。

`visit(root)` 做的是中序遍历：

```text
左 -> 根 -> 右
```

所以得到的是升序数组。

然后：

```go
result[len(result)-cnt]
```

就是取倒数第 `cnt` 个，也就是第 `cnt` 大。

---

## 四、把你的中序遍历写拆开看

你写的是：

```go
return append(append(visit(root.Left), []int{root.Val}...), visit(root.Right)...)
```

这句是对的，但是阅读起来比较绕。

可以拆成：

```go
result := []int{}
result = append(result, visit(root.Left)...)
result = append(result, root.Val)
result = append(result, visit(root.Right)...)
return result
```

这样一眼就能看出顺序是：

```text
左 -> 根 -> 右
```

---

## 五、Go 写法一：中序数组版

```go
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func findTargetNode(root *TreeNode, cnt int) int {
    nums := inorder(root)
    return nums[len(nums)-cnt]
}

func inorder(root *TreeNode) []int {
    if root == nil {
        return nil
    }

    result := []int{}
    result = append(result, inorder(root.Left)...)
    result = append(result, root.Val)
    result = append(result, inorder(root.Right)...)

    return result
}
```

这个版本最直观。

核心链路是：

```text
BST -> 中序遍历 -> 升序数组 -> 倒数第 k 个
```

---

## 六、为什么还有优化版？

题目要的是第 `k` 大。

中序遍历是升序：

```text
左 -> 根 -> 右
```

那么反向中序遍历就是降序：

```text
右 -> 根 -> 左
```

也就是说，如果按照：

```text
右子树 -> 当前节点 -> 左子树
```

访问 BST，访问顺序就是从大到小。

所以我们可以：

```text
每访问一个节点，cnt--
当 cnt == 0 时，当前节点就是第 k 大
```

这样不需要遍历完整棵树，也不需要保存整个数组。

---

## 七、优化版递归怎么拿到答案？

这是你卡住的地方。

关键是用两个外部变量：

```go
ans := 0
cnt := k
```

递归函数里每访问一个节点，就让：

```go
cnt--
```

当：

```go
cnt == 0
```

说明当前节点就是第 k 大，于是：

```go
ans = node.Val
return
```

为了提前停止后面的递归，在函数开头加：

```go
if node == nil || cnt == 0 {
    return
}
```

这句的意思是：

```text
如果节点为空，直接返回。
如果已经找到答案了，也直接返回，不再继续遍历。
```

---

## 八、Go 写法二：反向中序优化版

```go
func findTargetNode(root *TreeNode, cnt int) int {
    ans := 0

    var dfs func(node *TreeNode)
    dfs = func(node *TreeNode) {
        if node == nil || cnt == 0 {
            return
        }

        dfs(node.Right)

        cnt--
        if cnt == 0 {
            ans = node.Val
            return
        }

        dfs(node.Left)
    }

    dfs(root)
    return ans
}
```

核心顺序是：

```go
dfs(node.Right)
cnt--
dfs(node.Left)
```

也就是：

```text
右 -> 根 -> 左
```

这就是从大到小遍历。

---

## 九、为什么开头要判断 cnt == 0？

代码：

```go
if node == nil || cnt == 0 {
    return
}
```

`cnt == 0` 表示：

```text
已经找到第 k 大节点了。
```

这时没必要继续递归其它分支。

比如：

```text
k = 1
```

只要找到最大节点，就可以停止。

如果没有这句，虽然答案已经拿到了，递归还可能继续往左子树走，做多余工作。

---

## 十、反向中序例子走一遍

树：

```text
      5
     / \
    3   6
   / \
  2   4
 /
1
```

反向中序遍历顺序是：

```text
6, 5, 4, 3, 2, 1
```

如果：

```text
cnt = 3
```

递归过程：

```text
访问 6，cnt 从 3 变成 2
访问 5，cnt 从 2 变成 1
访问 4，cnt 从 1 变成 0
```

此时：

```text
ans = 4
```

然后后续递归因为 `cnt == 0` 会直接返回。

---

## 十一、Python3 写法

### 写法一：中序数组版

```python
# Definition for a binary tree node.
# class TreeNode:
#     def __init__(self, x):
#         self.val = x
#         self.left = None
#         self.right = None

class Solution:
    def findTargetNode(self, root: TreeNode, cnt: int) -> int:
        nums = []

        def inorder(node):
            if not node:
                return
            inorder(node.left)
            nums.append(node.val)
            inorder(node.right)

        inorder(root)
        return nums[len(nums) - cnt]
```

---

### 写法二：反向中序优化版

```python
class Solution:
    def findTargetNode(self, root: TreeNode, cnt: int) -> int:
        ans = 0

        def dfs(node):
            nonlocal cnt, ans
            if not node or cnt == 0:
                return

            dfs(node.right)

            cnt -= 1
            if cnt == 0:
                ans = node.val
                return

            dfs(node.left)

        dfs(root)
        return ans
```

---

## 十二、中序数组版 vs 反向中序版

| 写法 | 思路 | 优点 | 缺点 |
|---|---|---|---|
| 中序数组版 | 中序得到升序数组，取倒数第 k 个 | 最直观，容易写对 | 需要保存所有节点 |
| 反向中序版 | 按右根左从大到小遍历，数到第 k 个 | 不用数组，可以提前停止 | 需要理解外部变量和提前返回 |

刚开始刷题时，可以先写中序数组版。

面试时可以补充优化：

```text
因为题目要第 k 大，所以可以反向中序遍历，访问到第 k 个节点时停止。
```

---

## 十三、复杂度分析

### 中序数组版

时间复杂度：

```text
O(n)
```

需要遍历所有节点。

空间复杂度：

```text
O(n)
```

需要保存所有节点值。

递归栈额外是：

```text
O(h)
```

---

### 反向中序版

时间复杂度：

```text
平均 O(h + k)，最坏 O(n)
```

因为可能只访问到第 `k` 大节点就停止。

如果 `k` 很小，可以少访问很多节点。

空间复杂度：

```text
O(h)
```

只需要递归调用栈。

---

## 十四、容易错的点

### 1. BST 中序是升序

```text
左 -> 根 -> 右 = 从小到大
```

---

### 2. 第 k 大是倒数第 k 个

正确：

```go
nums[len(nums)-cnt]
```

不要写成：

```go
nums[cnt]
```

也不要写成：

```go
nums[cnt-1]
```

---

### 3. 反向中序是右根左

```text
右 -> 根 -> 左 = 从大到小
```

不要写成普通中序：

```text
左 -> 根 -> 右
```

---

### 4. `cnt--` 的位置要在访问当前节点时

反向中序里，访问顺序是：

```go
dfs(node.Right)
cnt--
dfs(node.Left)
```

`cnt--` 对应：

```text
当前节点被访问了一次。
```

不能在递归右子树之前就减。

---

### 5. 找到答案后要提前停止

函数开头写：

```go
if node == nil || cnt == 0 {
    return
}
```

找到答案后：

```go
ans = node.Val
return
```

这样后面的递归会因为 `cnt == 0` 停止。

---

## 十五、面试表达

可以这样说：

> 这题利用 BST 的性质。BST 的中序遍历是升序，所以最直观的做法是先中序遍历得到数组，然后返回倒数第 k 个元素，也就是 `nums[len(nums)-k]`。如果优化空间和遍历次数，可以用反向中序遍历，也就是按照右、根、左的顺序访问节点，这样访问顺序就是从大到小。递归过程中维护一个计数器 `k`，每访问一个节点就 `k--`，当 `k == 0` 时当前节点就是第 k 大，把它保存到答案里，并通过递归开头的 `k == 0` 判断提前停止后续遍历。

---

## 十六、一句话记忆

```text
BST 第 k 大：中序升序取倒数，右根左可提前数到第 k 个。
```

优化版核心代码：

```go
dfs(node.Right)
cnt--
if cnt == 0 {
    ans = node.Val
    return
}
dfs(node.Left)
```
