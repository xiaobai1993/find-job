# Go 面试高频题

---

## 一、Go 基础

### Q1：for 循环 + goroutine 输出什么？

```go
for i := 0; i < 10; i++ {
    go func() {
        fmt.Println(i)
    }()
}
```

**Go 1.22 之前：** 大概率输出 10 个 `10`。闭包捕获的是变量 `i` 的引用，循环共享同一个 `i`，goroutine 执行时循环多半已结束，`i` 已经是 10。

**Go 1.22+：** 输出 0~9 的随机排列。Go 1.22 改了语义，每次迭代创建一个新的 `i`，闭包捕获的是当前迭代的副本。

**旧版本修复：**

```go
// 方法 1：参数传递
go func(n int) {
    fmt.Println(n)
}(i)

// 方法 2：局部拷贝
i := i
go func() {
    fmt.Println(i)
}()
```

**考查点：** 闭包捕获机制、Go 1.22 loop variable fix

---
