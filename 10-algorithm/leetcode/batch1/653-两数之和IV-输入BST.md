# 653. 两数之和 IV - 输入 BST

题目链接：`https://leetcode.cn/problems/two-sum-iv-input-is-a-bst/description/`

---

## 一、题目描述

给定一棵二叉搜索树的根节点 `root` 和一个整数 `k`。

判断这棵树中是否存在两个不同节点，它们的值之和等于 `k`。

也就是判断是否存在：

```text
node1.Val + node2.Val == k
```

并且：

```text
node1 和 node2 必须是两个不同节点。
```

---

## 二、你的 Go 提交

你的代码：

```go
func findTarget(root *TreeNode, k int) bool {
    result := visit(root)
    var m = make(map[int]int)
    for idx, v := range result {
        m[v] = idx
    }
    for idx , v := range result {
        if idx2, ok := m[k-v];ok && idx != idx2{
            return true
        }
    }
    return false
}

func visit(root * TreeNode) []int {
    if root == nil{
        return []int{}
    }
    result := []int{}
    result = append(result,visit(root.Left)...)
    result = append(result,root.Val)
    result = append(result, visit(root.Right)...)
    return result
}
```

这个提交是正确的。

你的思路是：

```text
1. 对 BST 做中序遍历，得到一个升序数组
2. 用 map 记录每个值的位置
3. 遍历数组里的每个值 v
4. 判断 k - v 是否存在
5. 如果存在，并且不是同一个节点，就返回 true
```

核心判断是：

```go
if idx2, ok := m[k-v]; ok && idx != idx2 {
    return true
}
```

这句的意思是：

```text
如果 k-v 存在，并且它的位置和当前 v 的位置不同，说明找到了两个不同节点。
```

---

## 三、为什么你的写法能过？

这题本质上是：

```text
树上的 Two Sum
```

如果先不考虑 BST 性质，最直接的办法就是：

```text
遍历所有节点，用哈希表查另一个数是否存在。
```

你的代码虽然先做了中序遍历，但最后本质上还是：

```text
数组 + map 查找两数之和
```

所以它是正确的。

---

## 四、你的写法还有什么优化空间？

你已经感觉到了：

```text
总感觉没有必要遍历所有节点。
```

这个感觉是对的。

因为题目给的是：

```text
二叉搜索树 BST
```

BST 有一个很重要的性质：

```text
中序遍历是升序数组。
```

也就是说，你的 `visit(root)` 得到的 `result` 不是普通数组，而是一个已经排好序的数组。

既然数组已经有序，就可以把普通 Two Sum 的 map 写法优化成：

```text
双指针
```

---

## 五、BST 中序遍历为什么是升序？

BST 满足：

```text
左子树所有节点值 < 当前节点值 < 右子树所有节点值
```

中序遍历顺序是：

```text
左 -> 根 -> 右
```

所以访问顺序刚好是：

```text
小 -> 中 -> 大
```

因此得到的是升序数组。

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

中序遍历结果是：

```text
[1, 2, 3, 4, 5, 6]
```

---

## 六、优化一：中序数组 + 双指针

得到升序数组以后，就可以用双指针。

思路：

```text
left 指向最小值
right 指向最大值

如果 nums[left] + nums[right] == k，找到答案
如果和太小，说明需要更大的数，left++
如果和太大，说明需要更小的数，right--
```

这就是经典有序数组 Two Sum。

---

## 七、Go 解法一：你的 map 版本整理版

```go
/**
 * Definition for a binary tree node.
 * type TreeNode struct {
 *     Val int
 *     Left *TreeNode
 *     Right *TreeNode
 * }
 */
func findTarget(root *TreeNode, k int) bool {
    nums := inorder(root)

    indexMap := make(map[int]int)
    for i, v := range nums {
        indexMap[v] = i
    }

    for i, v := range nums {
        if j, ok := indexMap[k-v]; ok && i != j {
            return true
        }
    }

    return false
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

这个版本和你的提交最接近。

它的优点是：

```text
思路直观，容易想到。
```

缺点是：

```text
没有充分利用中序数组已经有序这个性质。
```

---

## 八、Go 解法二：中序数组 + 双指针

```go
func findTarget(root *TreeNode, k int) bool {
    nums := inorder(root)

    left, right := 0, len(nums)-1

    for left < right {
        sum := nums[left] + nums[right]

        if sum == k {
            return true
        }

        if sum < k {
            left++
        } else {
            right--
        }
    }

    return false
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

这个版本更推荐先掌握。

核心链路是：

```text
BST -> 中序遍历 -> 升序数组 -> 双指针找两数之和
```

---

## 九、双指针为什么对？

因为数组是升序的。

假设当前：

```text
sum = nums[left] + nums[right]
```

### 1. 如果 `sum == k`

说明找到两个数，直接返回：

```go
return true
```

---

### 2. 如果 `sum < k`

说明当前和太小。

因为 `right` 已经是当前能选的最大值了。

想让和变大，只能让左边变大：

```go
left++
```

---

### 3. 如果 `sum > k`

说明当前和太大。

因为 `left` 已经是当前能选的最小值了。

想让和变小，只能让右边变小：

```go
right--
```

---

## 十、为什么循环条件是 `left < right`？

因为题目要求：

```text
必须是两个不同节点。
```

如果写成：

```go
left <= right
```

就可能出现：

```text
同一个元素和自己相加
```

比如数组：

```text
[2, 3, 4]
```

`k = 6`。

如果允许 `left == right`，就可能用中间的 `3 + 3`，但树里只有一个 `3` 节点，这不合法。

所以双指针必须写：

```go
for left < right {
```

---

## 十一、例子走一遍

树：

```text
      5
     / \
    3   6
   / \   \
  2   4   7
```

`k = 9`

中序遍历得到：

```text
[2, 3, 4, 5, 6, 7]
```

双指针：

```text
left = 0, right = 5
nums[left] = 2, nums[right] = 7
sum = 9
```

找到了：

```text
2 + 7 = 9
```

返回：

```go
true
```

---

再看一个：

```text
nums = [1, 2, 3, 4, 5]
k = 10
```

过程：

```text
1 + 5 = 6，小了，left++
2 + 5 = 7，小了，left++
3 + 5 = 8，小了，left++
4 + 5 = 9，小了，left++
left == right，停止
```

没有找到，返回 `false`。

---

## 十二、Python3 解法一：中序数组 + set

```python
# Definition for a binary tree node.
# class TreeNode:
#     def __init__(self, val=0, left=None, right=None):
#         self.val = val
#         self.left = left
#         self.right = right

class Solution:
    def findTarget(self, root: Optional[TreeNode], k: int) -> bool:
        nums = []

        def inorder(node):
            if not node:
                return
            inorder(node.left)
            nums.append(node.val)
            inorder(node.right)

        inorder(root)

        seen = set()
        for v in nums:
            if k - v in seen:
                return True
            seen.add(v)

        return False
```

---

## 十三、Python3 解法二：中序数组 + 双指针

```python
class Solution:
    def findTarget(self, root: Optional[TreeNode], k: int) -> bool:
        nums = []

        def inorder(node):
            if not node:
                return
            inorder(node.left)
            nums.append(node.val)
            inorder(node.right)

        inorder(root)

        left, right = 0, len(nums) - 1

        while left < right:
            total = nums[left] + nums[right]

            if total == k:
                return True
            elif total < k:
                left += 1
            else:
                right -= 1

        return False
```

---

## 十四、更高级优化：双栈模拟 BST 双指针

上面的双指针版本还有一个问题：

```text
它需要先把整棵树中序遍历成数组。
```

空间复杂度是：

```text
O(n)
```

如果想进一步优化空间，可以不用提前保存完整数组。

可以用两个栈：

```text
leftStack：模拟从小到大的中序迭代器
rightStack：模拟从大到小的反向中序迭代器
```

它们分别相当于：

```text
left 指针：不断取当前最小值
right 指针：不断取当前最大值
```

这样可以把额外空间从 `O(n)` 优化到：

```text
O(h)
```

其中 `h` 是树高。

---

## 十五、Go 解法三：双栈优化版

```go
func findTarget(root *TreeNode, k int) bool {
    if root == nil {
        return false
    }

    leftStack := []*TreeNode{}
    rightStack := []*TreeNode{}

    pushLeft := func(node *TreeNode) {
        for node != nil {
            leftStack = append(leftStack, node)
            node = node.Left
        }
    }

    pushRight := func(node *TreeNode) {
        for node != nil {
            rightStack = append(rightStack, node)
            node = node.Right
        }
    }

    pushLeft(root)
    pushRight(root)

    for len(leftStack) > 0 && len(rightStack) > 0 {
        leftNode := leftStack[len(leftStack)-1]
        rightNode := rightStack[len(rightStack)-1]

        if leftNode == rightNode {
            break
        }

        sum := leftNode.Val + rightNode.Val

        if sum == k {
            return true
        }

        if sum < k {
            node := leftStack[len(leftStack)-1]
            leftStack = leftStack[:len(leftStack)-1]
            pushLeft(node.Right)
        } else {
            node := rightStack[len(rightStack)-1]
            rightStack = rightStack[:len(rightStack)-1]
            pushRight(node.Left)
        }
    }

    return false
}
```

这个版本更难一些。

刚开始刷 BST 题，不需要优先背这个版本。

更推荐先掌握：

```text
中序数组 + 双指针
```

---

## 十六、双栈版怎么理解？

### 1. `pushLeft(root)`

```go
pushLeft(root)
```

会把从根节点一路往左的节点都压栈。

栈顶就是当前最小节点。

例如：

```text
      5
     /
    3
   /
  2
```

压栈顺序是：

```text
5, 3, 2
```

栈顶是：

```text
2
```

也就是最小值。

---

### 2. `pushRight(root)`

```go
pushRight(root)
```

会把从根节点一路往右的节点都压栈。

栈顶就是当前最大节点。

例如：

```text
5
 \
  6
   \
    7
```

压栈顺序是：

```text
5, 6, 7
```

栈顶是：

```text
7
```

也就是最大值。

---

### 3. 如果当前和太小

```go
if sum < k {
    node := leftStack[len(leftStack)-1]
    leftStack = leftStack[:len(leftStack)-1]
    pushLeft(node.Right)
}
```

当前和太小，就要让左边变大。

在 BST 中，比当前节点大的下一个节点，可能在：

```text
当前节点的右子树里最左边的位置
```

所以弹出当前左节点以后，要执行：

```go
pushLeft(node.Right)
```

---

### 4. 如果当前和太大

```go
if sum > k {
    node := rightStack[len(rightStack)-1]
    rightStack = rightStack[:len(rightStack)-1]
    pushRight(node.Left)
}
```

当前和太大，就要让右边变小。

在 BST 中，比当前节点小的上一个节点，可能在：

```text
当前节点的左子树里最右边的位置
```

所以弹出当前右节点以后，要执行：

```go
pushRight(node.Left)
```

---

## 十七、三种写法对比

| 写法 | 思路 | 时间复杂度 | 空间复杂度 | 推荐程度 |
|---|---|---|---|---|
| 中序数组 + map | 中序得到数组，再用 map 查 `k-v` | O(n) | O(n) | 容易想到 |
| 中序数组 + 双指针 | 利用中序数组升序，两头夹 | O(n) | O(n) | 最推荐先掌握 |
| 双栈 BST 迭代器 | 不保存完整数组，两个栈模拟最小/最大指针 | O(n) | O(h) | 面试拔高 |

注意：

```text
双栈版本虽然可能提前结束，但最坏情况下仍然可能访问很多节点。
```

所以时间复杂度最坏仍然是：

```text
O(n)
```

---

## 十八、复杂度分析

### 中序数组 + map

时间复杂度：

```text
O(n)
```

因为需要遍历所有节点，并遍历数组。

空间复杂度：

```text
O(n)
```

数组和 map 都会保存节点信息。

---

### 中序数组 + 双指针

时间复杂度：

```text
O(n)
```

中序遍历需要访问所有节点，双指针最多再扫一遍数组。

空间复杂度：

```text
O(n)
```

需要保存中序数组。

递归调用栈额外是：

```text
O(h)
```

---

### 双栈优化版

时间复杂度：

```text
O(n)
```

最坏情况下可能访问所有节点。

空间复杂度：

```text
O(h)
```

两个栈最多保存树高相关的节点。

如果树平衡：

```text
O(log n)
```

如果树退化成链表：

```text
O(n)
```

---

## 十九、容易错的点

### 1. 忘记 BST 中序是升序

这题的优化点来自：

```text
BST 中序遍历 = 升序数组
```

如果忘了这个性质，就只能想到普通 map。

---

### 2. 双指针必须是 `left < right`

正确：

```go
for left < right {
```

不要写成：

```go
for left <= right {
```

因为不能用同一个节点两次。

---

### 3. `sum < k` 时移动左指针

```go
if sum < k {
    left++
}
```

因为和太小，需要更大的数。

---

### 4. `sum > k` 时移动右指针

```go
if sum > k {
    right--
}
```

因为和太大，需要更小的数。

---

### 5. map 版本要避免同一个节点被使用两次

如果用 map，需要判断索引不同：

```go
if j, ok := indexMap[k-v]; ok && i != j {
    return true
}
```

否则当：

```text
v * 2 == k
```

时，可能错误地把同一个节点用两次。

---

### 6. 双栈版要判断两个指针不能指向同一个节点

```go
if leftNode == rightNode {
    break
}
```

如果两个栈顶是同一个节点，说明左右迭代器相遇了，不能继续用同一个节点相加。

---

## 二十、面试表达

可以这样说：

> 这题是 BST 上的两数之和。最直观的做法是遍历整棵树，用哈希表记录已经见过的值，判断 `k - val` 是否存在。但因为题目给的是二叉搜索树，BST 的中序遍历是升序数组，所以可以先中序遍历得到有序数组，再用双指针从两端向中间夹。如果两数之和等于 k，返回 true；如果和小于 k，移动左指针；如果和大于 k，移动右指针。这个写法时间复杂度 O(n)，空间复杂度 O(n)。如果要进一步优化空间，可以用两个栈分别模拟从小到大和从大到小的 BST 迭代器，把空间优化到 O(h)。

---

## 二十一、一句话记忆

```text
BST 两数之和：中序变升序，双指针两头夹。
```

核心代码：

```go
nums := inorder(root)
left, right := 0, len(nums)-1

for left < right {
    sum := nums[left] + nums[right]
    if sum == k {
        return true
    }
    if sum < k {
        left++
    } else {
        right--
    }
}

return false
```
