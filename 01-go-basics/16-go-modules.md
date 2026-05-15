# Go Modules 完全指南：从 GOPATH 到 vgo 再到 Module

---

## 一、先搞懂：Go 依赖管理的进化史

### 1. Go 1.0 ~ 1.4：GOPATH 时代

```
GOPATH/
├── src/
│   ├── github.com/xxx/aaa
│   └── github.com/yyy/bbb
├── bin/
└── pkg/
```

**问题：**
- 所有项目共享同一个 GOPATH
- 没有版本概念！一个库只能有一个版本！
- 项目 A 要 v1，项目 B 要 v2，直接 GG
- 没有依赖锁定，go get 永远拉最新
- 无法复现构建，今天能编，明天可能就编不过

**一句话形容：原始社会，全靠手动管理，苦不堪言。**

---

### 2. Go 1.5 ~ 1.10：Vendor 时代

Go 1.5 引入 vendor 目录：
```
你的项目/
├── vendor/
│   ├── github.com/xxx/aaa
│   └── github.com/yyy/bbb
└── main.go
```

编译器优先找 vendor 下的依赖，每个项目可以有自己的一份依赖副本。

**社区工具：**
- godep
- glide
- dep（官方钦定的"官方实验品"）

**进步了，但问题依然很多：**
- vendor 把所有依赖源码都提交到 git，仓库巨肿
- 没有语义化版本概念
- 依赖冲突还是很难解
- 每个工具格式不一样，互不兼容

---

### 3. Go 1.11+：Modules 时代（现在的方案）

2018 年 Go 1.11 正式引入 Modules，Russ Cox 亲自操刀设计，解决了之前所有问题：

- ✅ 语义化版本支持
- ✅ go.mod 声明依赖，go.sum 校验哈希
- ✅ 最小版本选择（MVS）算法解冲突
- ✅ replace 指令解决各种奇奇怪怪的问题
- ✅ vendor 可选，想用就用不想用就不用
- ✅  GOPATH 终于可以退出历史舞台了

---

## 二、go.mod 文件详解

一个典型的 go.mod：

```go
module github.com/yourname/project  // 模块名，也是 import 路径

go 1.21  // Go 版本要求

require (
    github.com/gin-gonic/gin v1.9.1   // 直接依赖
    github.com/go-redis/redis/v8 v8.11.5
    golang.org/x/sync v0.3.0  // indirect 是间接依赖
)

exclude github.com/xxx/yyy v1.0.0  // 排除某个版本

replace github.com/xxx/yyy v1.0.0 => github.com/yourfork/yyy v1.0.1  // 替换
replace golang.org/x/net => ./local/net  // 替换成本地目录
```

---

### 1. module 指令：模块名

module 名就是你的项目的唯一标识，也是别人 import 你的路径。

**命名最佳实践：**
```go
// ✅ 好：用你的代码仓库路径，别人能直接 go get
module github.com/yourname/project

// ❌ 坏：随便起个名字，别人 import 不到
module myproject
```

---

### 2. go 指令：Go 版本要求

指定你的项目要求的最低 Go 版本，也会影响一些语言特性的开关。

```go
go 1.21  // 要求 Go 1.21+
```

Go 1.21 开始还可以写补丁版本：
```go
go 1.21.3
```

---

### 3. require 指令：依赖声明

两种依赖：
- **直接依赖**：你代码里直接 import 的
- **间接依赖**：直接依赖的依赖，标注 `// indirect`

```go
require (
    github.com/gin-gonic/gin v1.9.1          // 直接依赖
    golang.org/x/sync v0.3.0 // indirect      // 间接依赖
)
```

---

### 4. exclude 指令：排除某个版本

某个版本有 bug，不想让任何人用到：

```go
exclude github.com/xxx/yyy v1.2.0  // v1.2.0 有严重 bug，排除
```

---

### 5. replace 指令：神器级别，必看！

Go Modules 最好用的功能，没有之一！解决 90% 的依赖问题。

#### 场景 1：fork 了一个库，想先用自己的 fork

```go
// 把官方的替换成你的 fork
replace github.com/xxx/yyy => github.com/yourname/yyy v1.0.0-fix
```

#### 场景 2：本地调试依赖，改完立即生效

```go
// 替换成本地目录，改什么立即生效，不用提交不用打 tag
replace github.com/xxx/yyy => ../yyy  // 相对路径
```

开发的时候太好用了！改完依赖的代码，主项目立即生效，不用每次都 go get。

#### 场景 3：解决 import path 变了的问题

```go
// 库改名了，旧的 import 路径还在代码里
replace github.com/oldname/yyy => github.com/newname/yyy v2.0.0
```

#### 场景 4：解决依赖冲突，强制用某个版本

```go
// 强制所有地方都用 v1.5.0，不管别人要求什么
replace github.com/xxx/yyy => github.com/xxx/yyy v1.5.0
```

---

## 三、最小版本选择（MVS）算法

Go Modules 的依赖冲突解决算法，和 npm/yarn 完全不一样。

### 1. 算法逻辑

**规则：所有依赖声明的版本中，选最大的那个。**

举个例子：
- 你的项目要求 lib >= v1.1.0
- 依赖 A 要求 lib >= v1.2.0
- 依赖 B 要求 lib >= v1.3.0

**MVS 选择：v1.3.0**

就是这么简单！没有版本范围，没有 caret，没有 tilde，没有任何花活，直接选最大的。

---

### 2. 和 npm 的区别

| 特性 | Go MVS | npm/yarn |
|------|--------|----------|
| 算法 | 选所有要求里最大的版本 | 语义化版本范围内选最新的 |
| 版本范围 | 不支持，就是最小版本 | 支持 ^ ~ > < 各种 |
| 同一库多版本 | 不允许，全局只有一个版本 | 允许，每个依赖可以有自己的版本 |
| 锁定文件 | go.sum 是校验哈希，不是锁定版本 | package-lock.json/yarn.lock 精确锁定 |

**Go 的设计哲学：**
> 新版本应该向后兼容，所以直接用最大的版本就好了。一个程序里同一个库不应该有两个版本。

---

## 四、go mod 常用命令

| 命令 | 作用 |
|------|------|
| `go mod init` | 初始化一个新模块，创建 go.mod |
| `go mod tidy` | ✅ 最常用！自动加需要的依赖，删不用的依赖，整理 go.mod |
| `go mod download` | 下载所有依赖到本地缓存 |
| `go mod vendor` | 把所有依赖复制到 vendor 目录 |
| `go mod verify` | 校验依赖哈希有没有被篡改 |
| `go mod graph` | 打印依赖图 |
| `go mod why` | 告诉你为什么需要某个依赖 |

**最佳实践：每次 pull 代码之后先跑一遍 `go mod tidy`！**

---

## 五、Go Modules 常见坑与最佳实践

### 坑 1：v2+ 版本的 import 路径要加 /v2

**这是最经典的坑，90% 的人第一次用都会踩！**

语义化版本 v2 及以上，import 路径最后必须加 /v2！

```go
// ❌ 错误！v8 版本不加 /v8 找不到
import "github.com/go-redis/redis"  // 只能拿到 v6 及以下

// ✅ 正确！v8 必须加 /v8
import "github.com/go-redis/redis/v8"
```

**为什么要这么设计？**
因为 Go 的 import path 就是唯一标识，不同版本就是不同的包！v1 和 v2 可以同时存在，互不影响。

```go
// 可以同时 import v7 和 v8
import (
    "github.com/go-redis/redis/v7"
    "github.com/go-redis/redis/v8"
)
```

---

### 坑 2：replace 只在当前模块生效！

**超级重要！replace 不会传递！**

你的模块 go.mod 里写的 replace，只有你自己 build 的时候生效，别人 import 你的模块的时候你的 replace 对他们完全无效！

**不要依赖 replace 来修复问题！replace 只是给你自己本地开发用的，最终还是要把 fix 推到上游！**

---

### 坑 3：伪版本（pseudo-version）是什么？

你可能会看到这样的版本号：
```
v0.0.0-20230515213812-123456789abc
```

这叫伪版本，三种情况会出现：
1. 你 go get 了一个没有打 tag 的 commit
2. 这个 commit 在某个 tag 之后
3. 主分支还没发 v1.0.0

Go 自动生成的，格式是：`版本号-时间-commit hash`

---

### 最佳实践总结

1. ✅ **永远用 Go Modules**，不要再用 GOPATH 了
2. ✅ **每次改代码 pull 之后跑 go mod tidy**
3. ✅ **v2+ 版本 import 路径加 /vN**
4. ✅ **本地开发用 replace 指向本地目录，爽得飞起**
5. ✅ **replace 不要提交！或者提交了也要记得最终要上游修复**
6. ✅ **不要把 vendor 提交到 git**，现在 Go 已经不需要了
7. ❌ **不要手动改 go.mod**，让 go mod tidy 帮你管
8. ❌ **不要搞什么 vendor 提交 CI**，现在 Go 缓存已经很好了

---

## 六、面试高频问答

| 问题 | 答案 |
|------|------|
| Go 依赖管理经历了哪几个阶段？ | GOPATH 时代 → Vendor 时代 → Modules 时代。 |
| go.mod 里有哪几个指令？ | module、go、require、exclude、replace 五个。 |
| replace 指令有什么用？ | 替换依赖源，可以换成别的 fork，也可以换成本地目录，本地调试神器。 |
| replace 会传递吗？ | 绝对不会！replace 只在当前模块 build 生效，别人 import 你时无效。 |
| MVS 最小版本选择算法怎么工作的？ | 所有依赖声明的版本中，选最大的那个。简单直接，没有花活。 |
| v2+ 版本的 import 路径有什么要求？ | 必须加 /v2 后缀，比如 github.com/go-redis/redis/v8。 |
| go mod tidy 是干什么的？ | 自动分析代码，加需要的依赖，删不用的依赖，整理 go.mod。最常用的命令。 |
| 伪版本是什么？ | go get 没有 tag 的 commit 时自动生成的版本号，格式 v0.0.0-时间-commit hash。 |
| go.sum 文件是干什么的？ | 记录所有依赖的哈希值，校验依赖有没有被篡改，保证构建可复现。 |
| vendor 目录还要用吗？ | 大部分场景不需要了，除非你要离线构建，或者公司内网环境。 |
| Go Modules 之前有什么工具？ | godep、glide、dep，全都是历史了，别用了。 |
| 同一个库可以有多个版本在一个项目里吗？ | v1 和 v2+ 可以，因为 import path 不一样。同一个大版本下不行，全局只有一个版本。 |

---

## 七、一句话总结

> Go Modules 是现在唯一的官方依赖管理方案，记住 go mod tidy 万能，v2+ 加 /v2，replace 本地开发爽到飞起，MVS 选最大版本，基本就够用了。
