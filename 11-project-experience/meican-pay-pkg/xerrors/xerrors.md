# XErrors - 错误包装与栈追踪

## 核心设计

### 问题：Go 标准库 error 没有栈信息
```go
// 只能看到错误信息，不知道哪里出错的
fmt.Println(err)  // "sql: no rows in result set"
```

### 解决方案：Wrap + StackTrace
```go
type withStack struct {
    error
    stack *stacktrace.Stack
}

// Wrap 只在第一次调用时记录栈，避免重复包装
func Wrap(err error, message string) error {
    if err == nil {
        return nil
    }

    // 已经包装过了？只追加 message
    if e, ok := err.(*withStack); ok {
        if message == "" {
            return e
        }
        return &withStack{
            fmt.Errorf("%s: %w", message, e.error),
            e.stack,  // 复用原来的栈！
        }
    }

    // 第一次包装，记录栈
    if message != "" {
        err = fmt.Errorf("%s: %w", message, err)
    }
    return &withStack{err, stacktrace.Callers()}
}
```

## 使用方式

### 基础用法
```go
import "go.planetmeican.com/meican-pay/pkg/xerrors"

func QueryOrder(ctx context.Context, id int64) (*Order, error) {
    var order Order
    err := db.First(&order, id).Error
    if err != nil {
        // 包装错误，附加上下文信息
        return nil, xerrors.Wrap(err, fmt.Sprintf("query order %d", id))
    }
    return &order, nil
}

// 调用处
order, err := QueryOrder(ctx, 123)
if err != nil {
    // %+v 打印完整栈信息
    log.Error("query order failed", log.String("err", fmt.Sprintf("%+v", err)))
    return err
}
```

### 错误判断（兼容 errors.Is）
```go
// 包装后的错误仍能正确判断
var ErrNotFound = errors.New("not found")

err := xerrors.Wrap(ErrNotFound, "query failed")

xerrors.Is(err, ErrNotFound)  // true ✓
errors.Is(err, ErrNotFound)   // false ✗（需要用 xerrors.Is）
```

## 输出示例

```
query order 123: sql: no rows in result set
goroutine 1 [running]:
go.planetmeican.com/meican-pay/service/order.QueryOrder
    /path/to/order/service.go:42
go.planetmeican.com/meican-pay/handler/order.HandleQuery
    /path/to/order/handler.go:89
net/http.HandlerFunc.ServeHTTP
    /usr/local/go/src/net/http/server.go:2084
...
```

## 设计亮点

### 1. 不重复记录栈
```go
// ✅ 好：只记录最内层的调用栈
func A() error { return xerrors.Wrap(B(), "A failed") }
func B() error { return xerrors.Wrap(C(), "B failed") }
func C() error { return xerrors.Wrap(errors.New("oops"), "C failed") }

// 输出：A failed: B failed: C failed: oops + [C 的调用栈]
// 而不是 3 层重复的栈！
```

### 2. 环境变量控制栈深度
- `ERR_STACK_DEPTH` 环境变量控制栈深度
- 默认 5 层，足够定位问题

### 3. Format 接口
- `%s` / `%q`: 只显示错误信息
- `%v`: 只显示错误信息
- `%+v`: 显示错误信息 + 完整栈

## 最佳实践

### 什么时候 Wrap？
✅ **在错误产生的最内层 Wrap**
```go
// DB 层 Wrap，上层不需要重复 Wrap
func (d *Dao) Get(ctx context.Context, id int64) (*Entity, error) {
    err := d.db.First(&e, id).Error
    if err != nil {
        return nil, xerrors.Wrap(err, "db first")
    }
    return &e, nil
}
```

❌ **不要每层都 Wrap**
```go
// Service 层不要再 Wrap，会丢失原始栈位置
func (s *Service) Get(ctx context.Context, id int64) (*Entity, error) {
    e, err := s.dao.Get(ctx, id)
    if err != nil {
        // ❌ 不需要！dao 层已经 Wrap 过了
        return nil, xerrors.Wrap(err, "service get")
    }
    return e, nil
}
```

### 什么时候不需要 Wrap？
- 返回预定义的 sentinel error 时
- 参数校验错误（不需要栈）
- 业务自定义错误（自己带了信息）

## 思考问题

1. **为什么不直接用 github.com/pkg/errors？**
   - pkg/errors 每次 Wrap 都记录新栈
   - 多层调用后栈信息爆炸
   - 这里的实现复用最内层的栈

2. **Wrap 的 message 应该写什么？**
   - 当前函数做了什么
   - 关键参数（ID 等）
   - 不要写 "error occurred" 这种废话

3. **栈深度设置多少合适？**
   - 默认 5 层通常够了
   - 深调用链可以调大到 10
   - 太多了日志会很大
