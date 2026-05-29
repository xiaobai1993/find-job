# 剑指 Offer 51. 数组中的逆序对

题目链接：`https://leetcode.cn/problems/shu-zu-zhong-de-ni-xu-dui-lcof/`

---

## 一、题目描述

在数组中，若**前面一个数大于后面的数**，则这两个数构成一个**逆序对**。

输入一个数组，统计数组中逆序对的总数。

例如：

```text
输入：[7, 5, 6, 4]
输出：5

逆序对为：(7,5) (7,6) (7,4) (5,4) (6,4)
```

---

## 二、为什么暴力超时

暴力做法是双重循环，枚举所有 (i, j) 对：

```go
for i := 0; i < len(record)-1; i++ {
    for j := i+1; j < len(record); j++ {
        if record[i] > record[j] {
            cnt++
        }
    }
}
```

时间复杂度 O(n²)，数据量 50000 时大约要做 12.5 亿次比较，必然超时。

---

## 三、核心思路：归并排序统计逆序对

**归并排序**在合并两个有序子数组时，天然能批量统计逆序对，时间复杂度 O(n log n)。

---

## 四、归并时为什么能统计逆序对

归并排序把数组分成左右两半，分别排好序后合并。

合并时，左半 `[left..mid]` 和右半 `[mid+1..right]` 都已经**有序**。

用两个指针 i、j 分别指向左右半段，从小到大选数放入结果：

```text
左半：[5, 7]
右半：[4, 6]

i=0 指向 5，j=0 指向 4

比较：5 > 4
→ 选右半的 4，但此时左半剩余 [5, 7] 都比 4 大
→ 它们都和 4 构成逆序对，一次性加 mid-i+1 = 2
```

**关键结论**：当 `left[i] > right[j]` 时，选右边的 `right[j]`，
同时左半从 i 到 mid 的所有元素都大于 `right[j]`（因为左半有序），
所以逆序对数 `+= mid - i + 1`。

---

## 五、走一遍例子

`[7, 5, 6, 4]`：

```text
拆分：[7, 5] 和 [6, 4]

排序左半 [7, 5]：
  左=[7]，右=[5]，7>5 → 选5，cnt += 1（剩余左半[7]都 > 5）
  结果：[5, 7]，cnt=1

排序右半 [6, 4]：
  左=[6]，右=[4]，6>4 → 选4，cnt += 1
  结果：[4, 6]，cnt=1

合并 [5, 7] 和 [4, 6]：
  i=0→5，j=0→4，5>4 → 选4，cnt += mid-i+1 = 1-0+1 = 2，result=[4]
  i=0→5，j=1→6，5<6 → 选5，result=[4,5]
  i=1→7，j=1→6，7>6 → 选6，cnt += mid-i+1 = 1-1+1 = 1，result=[4,5,6]
  j 用完，把左半剩余的 7 放入，result=[4,5,6,7]
  本次 cnt = 3

总逆序对 = 1 + 1 + 3 = 5 ✓
```

---

## 六、Go 解法

```go
func reversePairs(record []int) int {
    temp := make([]int, len(record))
    return mergeSort(record, temp, 0, len(record)-1)
}

func mergeSort(arr, temp []int, left, right int) int {
    if left >= right {
        return 0
    }
    mid := left + (right-left)/2
    cnt := mergeSort(arr, temp, left, mid) + mergeSort(arr, temp, mid+1, right)
    return cnt + merge(arr, temp, left, mid, right)
}

func merge(arr, temp []int, left, mid, right int) int {
    // 备份到 temp
    for i := left; i <= right; i++ {
        temp[i] = arr[i]
    }
    i, j := left, mid+1
    cnt := 0
    for k := left; k <= right; k++ {
        if i > mid {
            arr[k] = temp[j]
            j++
        } else if j > right {
            arr[k] = temp[i]
            i++
        } else if temp[i] <= temp[j] {
            arr[k] = temp[i]
            i++
        } else {
            // 左半 temp[i] > 右半 temp[j]
            // 左半 i..mid 的所有元素都 > temp[j]，全是逆序对
            cnt += mid - i + 1
            arr[k] = temp[j]
            j++
        }
    }
    return cnt
}
```

---

## 七、代码拆开理解

### 1. 用 temp 备份

```go
for i := left; i <= right; i++ {
    temp[i] = arr[i]
}
```

```text
合并过程会覆盖 arr，必须先把当前范围备份到 temp，
从 temp 读，写回 arr。
```

---

### 2. 四种情况处理

```go
if i > mid {              // 左半用完，直接取右半
if j > right {            // 右半用完，直接取左半
if temp[i] <= temp[j] {   // 左边小，取左边，不产生逆序对
} else {                  // 右边小，取右边，产生逆序对
    cnt += mid - i + 1
```

---

### 3. 为什么是 mid - i + 1

```text
左半 temp[i..mid] 共 mid-i+1 个元素，都 >= temp[i]。
又因为 temp[i] > temp[j]，所以它们全都 > temp[j]。
它们在原始位置都在 temp[j] 的左边，全是逆序对。
```

---

### 4. 递归结构

```text
mergeSort(left, mid)    → 统计左半内部的逆序对
mergeSort(mid+1, right) → 统计右半内部的逆序对
merge(...)              → 统计跨越左右两半的逆序对
```

三部分加起来就是全部逆序对。

---

## 八、复杂度分析

时间复杂度：

```text
O(n log n)：归并排序的标准复杂度
每层合并操作是 O(n)，共 log n 层
```

空间复杂度：

```text
O(n)：temp 数组
```

---

## 九、面试表达

> 暴力双重循环是 O(n²)，会超时。正确做法是借助归并排序，在合并两个有序子数组时批量统计逆序对。合并时，如果左半的 temp[i] > 右半的 temp[j]，选右边的同时，左半从 i 到 mid 的所有元素都大于 temp[j]，一次加 mid-i+1 个逆序对。整体时间复杂度 O(n log n)，空间 O(n)。

---

## 十、一句话记忆

```text
逆序对 = 归并排序：合并时右边小，左半剩余全都算，cnt += mid-i+1。
```
