# 剑指 Offer 06. 从尾到头打印链表

题目链接：`https://leetcode.cn/problems/cong-wei-dao-tou-da-yin-lian-biao-lcof/`

---

## 一、题目描述

输入一个链表的头节点，从尾到头反过来返回每个节点的值。

示例：

```text
输入：head = [1,3,2]
输出：[2,3,1]
```

---

## 二、核心思路：顺序收集 + 反转数组

链表只能从头往后遍历，不能直接从尾往前遍历。

所以可以：

```text
1. 从头到尾遍历链表，把每个节点的值放进数组
2. 遍历结束后，把数组反转
3. 返回反转后的数组
```

例如：

```text
链表：1 -> 3 -> 2
先收集：[1, 3, 2]
反转后：[2, 3, 1]
```

---

## 三、你的代码为什么编译不过？

你的原始代码：

```go
func reverseBookList(head *ListNode) []int {
    var stack []int
    for t = head ;t != nil; t = t.Next {
        stack = append(stack, t.Val)
    }
    for i,j:=0,len(stack);i < j; i++,j--{
        stack[i], stack[j] = stack[j],stack[i]
    }
    return stack
}
```

主要有三个问题。

---

### 1. t 没有声明

你写的是：

```go
for t = head; t != nil; t = t.Next {
```

这里的 `t` 之前没有声明，Go 会编译报错。

应该写成：

```go
for t := head; t != nil; t = t.Next {
```

`:=` 表示在这里声明并初始化变量。

---

### 2. j := len(stack) 会数组越界

你写的是：

```go
for i, j := 0, len(stack); i < j; i++, j-- {
```

如果数组长度是 3：

```text
stack = [1, 3, 2]
```

合法下标是：

```text
0, 1, 2
```

但是：

```go
len(stack) == 3
```

如果访问：

```go
stack[3]
```

就会越界。

所以右指针应该从最后一个合法下标开始：

```go
j := len(stack) - 1
```

---

### 3. Go 里不能写 i++, j--

Go 的 `++` 和 `--` 是语句，不是表达式。

所以不能写在 `for` 第三段的逗号表达式里：

```go
i++, j--
```

要写成：

```go
i, j = i+1, j-1
```

完整写法：

```go
for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
```

---

## 四、Go 正确解法

```go
/**
 * Definition for singly-linked list.
 * type ListNode struct {
 *     Val int
 *     Next *ListNode
 * }
 */
func reverseBookList(head *ListNode) []int {
    var stack []int

    for t := head; t != nil; t = t.Next {
        stack = append(stack, t.Val)
    }

    for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
        stack[i], stack[j] = stack[j], stack[i]
    }

    return stack
}
```

---

## 五、Python3 解法

```python
# Definition for singly-linked list.
# class ListNode:
#     def __init__(self, x):
#         self.val = x
#         self.next = None

class Solution:
    def reverseBookList(self, head: Optional[ListNode]) -> List[int]:
        ans = []

        cur = head
        while cur:
            ans.append(cur.val)
            cur = cur.next

        ans.reverse()
        return ans
```

也可以用切片反转：

```python
class Solution:
    def reverseBookList(self, head: Optional[ListNode]) -> List[int]:
        ans = []

        cur = head
        while cur:
            ans.append(cur.val)
            cur = cur.next

        return ans[::-1]
```

---

## 六、你的 Python 写法为什么不对？

你的原始写法：

```python
class Solution:
    def reverseBookList(self, head: Optional[ListNode]) -> List[int]:
        stack = []
        while head:
            stack.append(head.val)
        for i,j =0, len(stack)-1;i<j;i++,j--:
            stack[i], stack[j] = stack[j],stack[i]
        return stack
```

主要有两个问题。

### 1. while 里面没有移动 head，会死循环

你写的是：

```python
while head:
    stack.append(head.val)
```

但是 `head` 一直指向同一个节点，没有往后走，所以会一直循环。

需要补上：

```python
head = head.next
```

### 2. Python 没有 Go/C 风格的 for 写法

Python 不能写：

```python
for i,j =0, len(stack)-1;i<j;i++,j--:
```

这种写法是 Go / C / Java 风格，不是 Python 语法。

如果要手动双指针反转数组，Python 应该用 `while`：

```python
i, j = 0, len(stack) - 1
while i < j:
    stack[i], stack[j] = stack[j], stack[i]
    i += 1
    j -= 1
```

完整修复版：

```python
class Solution:
    def reverseBookList(self, head: Optional[ListNode]) -> List[int]:
        stack = []

        while head:
            stack.append(head.val)
            head = head.next

        i, j = 0, len(stack) - 1
        while i < j:
            stack[i], stack[j] = stack[j], stack[i]
            i += 1
            j -= 1

        return stack
```

Python 中更常用的写法还是：

```python
stack.reverse()
```

或者：

```python
return stack[::-1]
```

---

## 七、另一种思路：递归

递归可以利用函数调用栈，先走到链表尾部，再回溯时收集值。

```go
func reverseBookList(head *ListNode) []int {
    var ans []int

    var dfs func(node *ListNode)
    dfs = func(node *ListNode) {
        if node == nil {
            return
        }
        dfs(node.Next)
        ans = append(ans, node.Val)
    }

    dfs(head)
    return ans
}
```

递归写法更贴合“从尾到头”，但是空间复杂度仍然是 `O(n)`，而且链表很长时可能有递归栈风险。

面试时优先写数组反转版本，更稳定。

---

## 八、复杂度分析

顺序收集 + 反转数组：

时间复杂度：

```text
O(n)
```

需要遍历链表一次，再反转数组一次，整体仍然是线性复杂度。

空间复杂度：

```text
O(n)
```

需要数组保存所有节点值。

---

## 九、容易错的点

### 1. Go 变量必须先声明

错误：

```go
for t = head; t != nil; t = t.Next
```

正确：

```go
for t := head; t != nil; t = t.Next
```

---

### 2. 右指针要从 len(stack)-1 开始

错误：

```go
j := len(stack)
```

正确：

```go
j := len(stack) - 1
```

因为数组最后一个元素下标是 `len(stack)-1`。

---

### 3. Go 的 for 第三段不能写 i++, j--

错误：

```go
for i, j := 0, len(stack)-1; i < j; i++, j--
```

正确：

```go
for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1
```

---

## 十、面试表达

可以这样说：

> 因为单向链表只能从头往后遍历，不能直接从尾往前走，所以我先遍历链表，把所有节点值按顺序放到数组里，然后再反转数组，最后返回。这个方法时间复杂度是 O(n)，空间复杂度是 O(n)。也可以用递归利用系统栈实现从尾到头收集，但链表很长时递归可能有栈风险，所以我更倾向于数组反转的写法。

---

## 十一、一句话记忆

```text
从尾到头打印链表：链表先顺序遍历进数组，再把数组反转。
```
