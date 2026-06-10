# 47. 全排列 II

> LeetCode 链接：https://leetcode.cn/problems/permutations-ii/

## 题目
给定**可能含有重复数字**的整数数组 `nums`，返回其所有不重复的全排列。

## 最易理解的方法：回溯 + 排序去重

**核心思路：**
先排序，使相同数字相邻。回溯时，若当前数字与前一个相同**且前一个未被使用**（说明在同一层重复了），则跳过（剪枝）。

```
nums = [1, 1, 2]  (排序后)
used = [F, F, F]

选nums[0]=1: used=[T,F,F]
  选nums[1]=1: [1,1,2] ✓
  选nums[2]=2: [1,2,1] ✓（再选完剩余1）
选nums[1]=1: 与nums[0]相同且nums[0]未使用 → 跳过！
选nums[2]=2: used=[F,F,T]
  选nums[0]=1: [2,1,1] ✓（再选完剩余1）
```

## Go 实现

```go
import "sort"

func permuteUnique(nums []int) [][]int {
    sort.Ints(nums)
    var res [][]int
    used := make([]bool, len(nums))
    var dfs func(path []int)
    dfs = func(path []int) {
        if len(path) == len(nums) {
            tmp := make([]int, len(path))
            copy(tmp, path)
            res = append(res, tmp)
            return
        }
        for i := 0; i < len(nums); i++ {
            if used[i] {
                continue
            }
            // 同层去重：相同数字且前一个未使用（同层已用过）
            if i > 0 && nums[i] == nums[i-1] && !used[i-1] {
                continue
            }
            used[i] = true
            dfs(append(path, nums[i]))
            used[i] = false
        }
    }
    dfs([]int{})
    return res
}
```

## 复杂度
- 时间：O(n × n!)
- 空间：O(n)

## 记忆口诀
> 排序后回溯；去重条件：nums[i]==nums[i-1] && !used[i-1]（同层已经用过相同的了）。
