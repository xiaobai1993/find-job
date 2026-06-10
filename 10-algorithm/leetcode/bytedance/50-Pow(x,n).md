# 50. Pow(x, n)

> LeetCode 链接：https://leetcode.cn/problems/powx-n/

## 题目
实现 `pow(x, n)`，即 x 的 n 次幂。n 可以为负数。

## 最易理解的方法：快速幂（递归）

**核心思路：**
- 若 n 为偶数：x^n = (x^(n/2))^2
- 若 n 为奇数：x^n = x * x^(n-1)
- 若 n 为负数：x^n = (1/x)^(-n)

每次将问题规模减半，时间复杂度 O(log n)。

```
x=2, n=10

myPow(2, 10)
  = myPow(2, 5) * myPow(2, 5)
  myPow(2, 5)
    = 2 * myPow(2, 4)
    myPow(2, 4)
      = myPow(2, 2) * myPow(2, 2)
      myPow(2, 2)
        = myPow(2, 1) * myPow(2, 1)
        = 2 * 2 = 4
      = 4 * 4 = 16
    = 2 * 16 = 32
  = 32 * 32 = 1024
```

## Go 实现

```go
func myPow(x float64, n int) float64 {
    if n < 0 {
        x = 1 / x
        n = -n
    }
    return fastPow(x, n)
}

func fastPow(x float64, n int) float64 {
    if n == 0 {
        return 1
    }
    half := fastPow(x, n/2)
    if n%2 == 0 {
        return half * half
    }
    return half * half * x
}
```

## 迭代版本（更推荐）

```go
func myPow(x float64, n int) float64 {
    if n < 0 {
        x = 1 / x
        n = -n
    }
    res := 1.0
    for n > 0 {
        if n%2 == 1 {
            res *= x
        }
        x *= x
        n /= 2
    }
    return res
}
```

## 复杂度
- 时间：O(log n)
- 空间：O(1)（迭代版）

## 记忆口诀
> 快速幂：n负则x取倒数；奇数多乘一个x；每轮x平方，n减半；结果累积在res。
