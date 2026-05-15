# Go 泛型 vs Interface：真实业务场景对比

> **面试高频题**：Go 1.18 支持泛型了，开发中到底什么时候用泛型，什么时候用 Interface？结合具体业务场景说明。

---

## 一、先上结论：决策速查表

| 场景类型 | 优先用泛型 | 优先用 Interface |
|---------|-----------|------------------|
| **数据容器操作** | slice/map 过滤、转换、去重、合并 | 不需要 |
| **算法逻辑复用** | 排序、搜索、聚合、数值计算 | 不需要 |
| **多态行为** | 不需要 | 不同类型有不同实现（策略模式） |
| **依赖注入** | 不需要 | 面向接口编程，解耦依赖 |
| **插件/扩展点** | 不需要 | 定义扩展契约 |
| **返回值类型安全** | 需要类型匹配，不想断言 | 返回 interface{} 需要断言 |
| **需要运算符支持** | `+` `-` `*` `<` `>` 等 | 不支持运算符 |
| **性能敏感场景** | 编译期单态化，无运行时开销 | 动态派发，有少量开销 |

**面试一句话总结**：
> 对「**数据**」做通用操作（转换、过滤、算法）用泛型；对「**行为**」做抽象、需要多态用 Interface。

---

## 二、10 个真实业务场景详解

### 场景 1：API 通用响应封装 ❌ Interface ✅ 泛型

**业务背景**：所有 HTTP 接口返回统一格式：
```json
{
    "code": 0,
    "msg": "success",
    "data": { ... }  // 不同接口 data 结构不同
}
```

**Go 1.18 之前（烂代码）**：
```go
type Response struct {
    Code int         `json:"code"`
    Msg  string      `json:"msg"`
    Data interface{} `json:"data"`  // ❌ 失去类型！
}

// 使用时每次都要断言，非常容易出 bug
resp := GetUser()
user, ok := resp.Data.(*User)  // ❌ 漏写 ok 直接 panic
if !ok {
    return errors.New("类型错误")
}
```

**Go 1.18+ 泛型写法（优雅）**：
```go
type Response[T any] struct {
    Code int    `json:"code"`
    Msg  string `json:"msg"`
    Data T      `json:"data"`  // ✅ 类型安全！
}

// 使用时不需要断言
resp := GetUser()  // 返回 Response[*User]
user := resp.Data   // ✅ 直接用，类型就是 *User
```

**为什么泛型更好**：
- 编译期类型检查，不会有运行时类型错误
- 不需要类型断言，代码更干净
- IDE 有代码提示，开发体验更好

---

### 场景 2：通用工具函数 ❌ Interface ✅ 泛型

**业务背景**：项目中大量 slice 操作：过滤、去重、分组、转换

**Go 1.18 之前（重复代码地狱）**：
```go
func FilterInt(s []int, f func(int) bool) []int { ... }
func FilterString(s []string, f func(string) bool) []string { ... }
func FilterUser(s []*User, f func(*User) bool) []*User { ... }
func FilterOrder(s []*Order, f func(*Order) bool) []*Order { ... }
// 每个类型写一遍，N 个类型就是 N 份几乎一样的代码...
```

**或者用 any + 反射（性能烂+不安全）**：
```go
func Filter(s any, f func(any) bool) []any {
    // 大量反射代码，慢 10~100 倍，还容易 panic
}
```

**Go 1.18+ 泛型写法（一份代码搞定所有）**：
```go
func Filter[T any](s []T, f func(T) bool) []T {
    result := make([]T, 0, len(s))
    for _, v := range s {
        if f(v) {
            result = append(result, v)
        }
    }
    return result
}

// ✅ 所有类型通用，类型安全，性能和手写一样
adults := Filter(users, func(u *User) bool { return u.Age >= 18 })
paid := Filter(orders, func(o *Order) bool { return o.Paid })
```

**为什么泛型更好**：
- 一份代码，所有类型通用
- 编译期生成，性能=手写版本，没有反射开销
- 类型安全，IDE 提示完整

---

### 场景 3：数据库 DAO 层 ❌ Interface ✅ 泛型

**业务背景**：每个表的 DAO 都有 CRUD 操作，代码高度重复

**Go 1.18 之前（每个表写一遍）**：
```go
type UserDAO struct { db *gorm.DB }
func (d *UserDAO) GetByID(id int64) (*User, error) { ... }
func (d *UserDAO) Create(u *User) error { ... }
func (d *UserDAO) Update(u *User) error { ... }
func (d *UserDAO) Delete(id int64) error { ... }

type OrderDAO struct { db *gorm.DB }
func (d *OrderDAO) GetByID(id int64) (*Order, error) { ... }
func (d *OrderDAO) Create(o *Order) error { ... }
// 10 张表就是 10×4 = 40 个几乎一样的函数...
```

**Go 1.18+ 泛型 BaseDAO（写一次用一辈子）**：
```go
type BaseDAO[T any] struct {
    db *gorm.DB
}

func NewBaseDAO[T any](db *gorm.DB) *BaseDAO[T] {
    return &BaseDAO[T]{db: db}
}

func (d *BaseDAO[T]) GetByID(id int64) (*T, error) {
    var t T
    err := d.db.First(&t, id).Error
    return &t, err
}

func (d *BaseDAO[T]) Create(t *T) error {
    return d.db.Create(t).Error
}

func (d *BaseDAO[T]) Update(t *T) error {
    return d.db.Save(t).Error
}

func (d *BaseDAO[T]) Delete(id int64) error {
    var t T
    return d.db.Delete(&t, id).Error
}
```

**使用（零重复代码）**：
```go
// UserDAO 直接继承 BaseDAO 的所有方法
type UserDAO struct {
    *BaseDAO[User]
    // 可以在这里加 User 特有的方法
}

func NewUserDAO(db *gorm.DB) *UserDAO {
    return &UserDAO{BaseDAO: NewBaseDAO[User](db)}
}

// OrderDAO 同理
type OrderDAO struct { *BaseDAO[Order] }
```

**为什么泛型更好**：
- CRUD 通用逻辑 100% 复用
- 新增表只需要一行代码嵌入 BaseDAO
- 类型安全，不需要断言

---

### 场景 4：缓存层封装 ❌ Interface ✅ 泛型

**业务背景**：Redis 缓存封装，不同 key 对应不同 value 类型

**Go 1.18 之前（interface{} + 断言）**：
```go
func Get(key string) (interface{}, error) { ... }
func Set(key string, value interface{}, ttl time.Duration) error { ... }

// 使用时
userData, _ := cache.Get("user:1")
user, ok := userData.(*User)  // ❌ 每次都要断言，烦！
```

**Go 1.18+ 泛型写法**：
```go
type Cache[T any] struct {
    client *redis.Client
}

func (c *Cache[T]) Get(key string) (*T, error) {
    data, err := c.client.Get(key).Bytes()
    if err != nil {
        return nil, err
    }
    var t T
    if err := json.Unmarshal(data, &t); err != nil {
        return nil, err
    }
    return &t, nil
}

func (c *Cache[T]) Set(key string, t *T, ttl time.Duration) error {
    data, _ := json.Marshal(t)
    return c.client.Set(key, data, ttl).Err()
}
```

**使用（零断言）**：
```go
userCache := &Cache[User]{client: redisClient}
orderCache := &Cache[Order]{client: redisClient}

// 直接拿类型，不需要断言 ✅
user, _ := userCache.Get("user:1")  // user 类型是 *User
order, _ := orderCache.Get("order:1")  // order 类型是 *Order
```

---

### 场景 5：结果集 / Optional 类型 ❌ Interface ✅ 泛型

**业务背景**：函数需要返回「成功/失败/空」三种状态

**Go 1.18 之前（多返回值 + error）**：
```go
func GetUser(id int64) (*User, bool, error) {
    // 三个返回值，调用方很容易搞混顺序
}

user, exists, err := GetUser(1)
// 经常有人写成：if err != nil && !exists 这种错误逻辑
```

**Go 1.18+ 泛型 Option 类型**：
```go
type Option[T any] struct {
    value T
    ok    bool
}

func Some[T any](v T) Option[T] {
    return Option[T]{value: v, ok: true}
}

func None[T any]() Option[T] {
    return Option[T]{ok: false}
}

func (o Option[T]) IsSome() bool { return o.ok }
func (o Option[T]) Unwrap() T { return o.value }

// 使用
opt := GetUser(1)  // 返回 Option[*User]
if opt.IsSome() {
    user := opt.Unwrap()  // ✅ 有值才取，不会空指针
}
```

---

### 场景 6：支付接口多态 ✅ Interface ❌ 泛型

**业务背景**：支持多种支付方式（支付宝、微信、银联）

**用 Interface（正确姿势）**：
```go
// 定义支付接口
type Payment interface {
    Pay(amount int64) (*PayResult, error)
    Refund(orderID string, amount int64) error
    Query(orderID string) (*PayStatus, error)
}

// 不同支付方式各自实现
type Alipay struct{}
func (a *Alipay) Pay(amount int64) (*PayResult, error) {
    // 支付宝签名、调用接口...
}

type WechatPay struct{}
func (w *WechatPay) Pay(amount int64) (*PayResult, error) {
    // 微信签名、调用接口...
}

type UnionPay struct{}
func (u *UnionPay) Pay(amount int64) (*PayResult, error) {
    // 银联签名、调用接口...
}
```

**使用（多态）**：
```go
// 工厂根据配置返回不同实现
func GetPayment(channel string) Payment {
    switch channel {
    case "alipay":
        return &Alipay{}
    case "wechat":
        return &WechatPay{}
    case "unionpay":
        return &UnionPay{}
    default:
        return nil
    }
}

// 业务代码完全不知道具体实现 ✅
payment := GetPayment("alipay")
result, err := payment.Pay(10000)  // 多态调用
```

**为什么 Interface 更好**：
- 每种支付方式的实现完全不同
- 需要运行时动态选择实现
- 新增支付方式不需要修改业务代码
- 这是典型的「行为多态」，泛型不擅长

---

### 场景 7：数据库事务管理 ✅ Interface ❌ 泛型

**业务背景**：Service 层需要事务支持，但是不关心底层用什么数据库

**用 Interface（正确姿势）**：
```go
// 定义事务接口，业务层只依赖这个
type TxManager interface {
    WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// GORM 实现
type GormTxManager struct { db *gorm.DB }
func (g *GormTxManager) WithTx(ctx context.Context, fn func(context.Context) error) error {
    // GORM 事务逻辑
}

// 可以换成 XORM，业务代码完全不用改
type XormTxManager struct { engine *xorm.Engine }
func (x *XormTxManager) WithTx(ctx context.Context, fn func(context.Context) error) error {
    // XORM 事务逻辑
}
```

**业务层使用（完全解耦）**：
```go
type OrderService struct {
    txMgr TxManager  // 只依赖接口，不依赖具体实现
}

func (s *OrderService) CreateOrder(ctx context.Context, req *CreateOrderReq) error {
    return s.txMgr.WithTx(ctx, func(ctx context.Context) error {
        // 1. 创建订单
        // 2. 扣库存
        // 3. 生成流水
        // ... 任何一步失败自动回滚
    })
}
```

**为什么 Interface 更好**：
- 这是典型的「依赖倒置原则」，面向接口编程
- 可以方便地 mock 做单元测试
- 底层数据库替换不影响上层业务

---

### 场景 8：消息队列消费封装 ⚠️ 都可以，看场景

**业务背景**：不同 topic 的消息，不同的处理逻辑

#### 方案 A：Interface（适合不同消息逻辑差异大）
```go
type MessageHandler interface {
    Handle(ctx context.Context, msg []byte) error
    Topic() string
}

// 订单消息处理
type OrderHandler struct{}
func (h *OrderHandler) Handle(ctx context.Context, msg []byte) error {
    var order Order
    json.Unmarshal(msg, &order)
    // 订单逻辑...
}

// 注册所有 handler
consumers := []MessageHandler{&OrderHandler{}, &UserHandler{}, ...}
```

#### 方案 B：泛型（适合消息处理流程相似，只有解析不同）
```go
type Consumer[T any] struct {
    topic    string
    handler  func(ctx context.Context, msg *T) error
}

func NewConsumer[T any](topic string, h func(context.Context, *T) error) *Consumer[T] {
    return &Consumer[T]{topic: topic, handler: h}
}

func (c *Consumer[T]) Run(ctx context.Context) {
    // 通用的消费逻辑：拉取消息、重试、幂等、日志
    for msg := range mq.Consume(c.topic) {
        var t T
        json.Unmarshal(msg.Body, &t)
        c.handler(ctx, &t)  // 调用业务 handler
        msg.Ack()
    }
}
```

**使用**：
```go
// 只需要写业务逻辑，通用逻辑都封装了
NewConsumer[Order]("order_created", func(ctx context.Context, o *Order) error {
    // 处理订单
}).Run(ctx)

NewConsumer[User]("user_registered", func(ctx context.Context, u *User) error {
    // 处理用户
}).Run(ctx)
```

**这种场景泛型更好**：
- 消息拉取、重试、幂等、日志都是通用的
- 只有解析和业务 handler 不同
- 不需要每个 topic 写一个完整的 handler 结构体

---

### 场景 9：RPC 客户端封装 ✅ Interface + 泛型 组合

**业务背景**：微服务之间 RPC 调用，需要超时、重试、熔断

**最佳实践：两者结合**：
```go
// 1. 用 Interface 定义服务契约（多态）
type UserService interface {
    GetUser(ctx context.Context, id int64) (*User, error)
    ListUsers(ctx context.Context, page, size int) ([]*User, int64, error)
}

// 2. 用泛型封装通用 RPC 逻辑（复用）
type RPCClient[T any] struct {
    target    string
    timeout   time.Duration
    retryMax  int
}

func (c *RPCClient[T]) Call(ctx context.Context, method string, req, resp any) error {
    // 通用 RPC 逻辑：序列化、超时、重试、熔断、埋点
    for i := 0; i < c.retryMax; i++ {
        err := c.doCall(ctx, method, req, resp)
        if err == nil {
            return nil
        }
        time.Sleep(time.Second * time.Duration(i))
    }
    return errors.New("重试失败")
}

// 3. 具体实现组合两者
type UserServiceClient struct {
    *RPCClient[UserService]  // 泛型通用逻辑
}

func (u *UserServiceClient) GetUser(ctx context.Context, id int64) (*User, error) {
    var resp GetUserResponse
    err := u.Call(ctx, "UserService.GetUser", &GetUserRequest{ID: id}, &resp)
    return resp.User, err
}
```

**为什么两者结合最好**：
- Interface 定义服务契约，方便 mock 和多态
- 泛型封装通用的 RPC 重试、超时、熔断逻辑
- 每个具体服务只需要写自己的方法调用

---

### 场景 10：Mock 单元测试 ✅ Interface ❌ 泛型

**业务背景**：单元测试需要 mock 外部依赖

**Interface 是唯一正确解**：
```go
// 业务层只依赖接口
type OrderService struct {
    userService  UserService   // 接口
    payment      Payment       // 接口
    inventory    InventorySvc  // 接口
}

// 测试时可以随便 mock
type MockUserService struct { mock.Mock }
func (m *MockUserService) GetUser(ctx context.Context, id int64) (*User, error) {
    args := m.Called(ctx, id)
    return args.Get(0).(*User), args.Error(1)
}

func TestCreateOrder(t *testing.T) {
    mockUser := &MockUserService{}
    mockUser.On("GetUser", 1).Return(&User{ID: 1, Name: "test"}, nil)

    // 注入 mock，不需要真实的 RPC 客户端
    svc := NewOrderService(mockUser, ...)
    // 测试...
}
```

**泛型做不了这个**：
- 泛型是编译期的，测试需要运行时替换实现
- 这是典型的「行为抽象」，Interface 的主场

---

## 三、终极判断公式

遇到一个场景，按顺序问自己三个问题：

### 问题 1：逻辑是对「数据」还是对「行为」？

```
对「数据」操作
    ↓
数据类型可以是任意的，但操作逻辑完全相同？
    ↓
→ 用 泛型 （Filter、Map、CRUD、缓存、Response 封装）

对「行为」抽象
    ↓
不同类型有不同的实现方式，但对外接口统一？
    ↓
→ 用 Interface （支付、短信、邮件、存储、RPC）
```

### 问题 2：需要支持运算符吗？

```
需要 + - * / > < == 等运算符？
    ↓
→ 必须用 泛型 （Interface 完全不支持运算符）
```

### 问题 3：需要运行时动态替换实现吗？

```
需要运行时根据配置/条件选择不同的实现？
    ↓
需要 mock 做单元测试？
    ↓
→ 必须用 Interface
```

---

## 四、常见反模式（面试踩坑点）

### ❌ 反模式 1：为了泛型而泛型
```go
// 完全没必要用泛型，直接用 Interface 更简单
type Handler[T Request] interface {
    Handle(req T) error
}

// 改成这样更好
type Handler interface {
    Handle(req Request) error
}
```

### ❌ 反模式 2：泛型参数满天飞
```go
// 超过 3 个泛型参数，代码可读性急剧下降
type Service[A, B, C, D any] struct { ... }

// 这种情况应该拆分成多个小类型，或者用 Interface
```

### ❌ 反模式 3：用 Interface 做容器
```go
// 这是 Go 1.18 之前的无奈之举，现在请用泛型
type List interface {
    Get(i int) interface{}
    Add(v interface{})
}

// 用泛型，类型安全，性能更好
type List[T any] struct { items []T }
func (l *List[T]) Get(i int) T { return l.items[i] }
func (l *List[T]) Add(v T) { l.items = append(l.items, v) }
```

---

## 五、面试标准答案模板

面试官问：**Go 1.18 有了泛型，什么时候用泛型，什么时候用 Interface？**

**标准回答**：

> 我总结下来主要分三种场景：
>
> **第一，对数据做通用操作，用泛型**。比如 slice 的过滤、转换、去重，DAO 的 CRUD，API 响应封装，缓存层封装。这些场景的特点是：操作逻辑完全相同，只是数据类型不同。泛型可以做到 100% 代码复用，编译期类型安全，没有反射开销。
>
> **第二，对行为做抽象，需要多态，用 Interface**。比如支付接口、短信接口、存储接口、RPC 服务契约。这些场景的特点是：不同类型有不同的实现方式，需要运行时动态选择实现，还要支持 mock 做单元测试。这是 Interface 的主场。
>
> **第三，复杂场景两者结合**。比如 RPC 客户端，用 Interface 定义服务契约，用泛型封装通用的重试、超时、熔断逻辑。
>
> **简单记就是：对数据用泛型，对行为用 Interface。**

---

## 六、补充：泛型性能开销

很多面试官会追问：泛型有性能开销吗？

**答案：几乎没有**：
- Go 采用**单态化**（monomorphization）编译策略
- 相同大小的类型共享一份代码（所有指针类型共享）
- 不同值类型生成不同代码，但和手写版本性能完全一致
- Interface 的动态派发反而有 ~50% 的性能开销（但一般可以忽略）

| 方式 | 相对性能 |
|------|---------|
| 直接调用 | 1.0x |
| 泛型调用 | 1.0x |
| Interface 调用 | 1.5x |
| 反射调用 | 10~100x |

---

**文档位置**：`12-interview-qa/generic-vs-interface-business-scenarios.md`
