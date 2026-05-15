# Go 单元测试与 Mock 完全指南

> 面试高频：Go 单元测试怎么写？有哪些 Mock 方式？依赖怎么注入？

---

## 一、Go 单元测试基础

### 1.1 最简单的测试

Go 测试文件命名规则：`*_test.go`

```go
// math.go
func Add(a, b int) int {
    return a + b
}

// math_test.go
func TestAdd(t *testing.T) {
    result := Add(2, 3)
    expected := 5

    if result != expected {
        t.Errorf("Add(2, 3) = %d; want %d", result, expected)
    }
}
```

运行测试：
```bash
go test              # 运行当前目录测试
go test -v           # 详细输出
go test -run TestAdd # 只跑指定测试
go test ./...        # 递归跑所有包
```

---

### 1.2 表驱动测试（Table Driven Test）

Go 社区最推崇的写法，**面试一定要说这个**！

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string  // 测试用例名称
        a        int     // 输入参数 1
        b        int     // 输入参数 2
        expected int     // 期望结果
    }{
        {"正数相加", 2, 3, 5},
        {"负数相加", -1, -1, -2},
        {"零相加", 0, 0, 0},
        {"正负相加", 5, -3, 2},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {  // 子测试！关键！
            result := Add(tt.a, tt.b)
            if result != tt.expected {
                t.Errorf("got %d, want %d", result, tt.expected)
            }
        })
    }
}
```

**为什么这是最佳实践**：
- 新增测试用例只需要加一行数据
- 子测试可以单独运行：`go test -run "TestAdd/正数相加"`
- 失败时会显示具体是哪个用例失败

---

### 1.3 测试覆盖率

```bash
go test -cover                  # 查看覆盖率
go test -coverprofile=cover.out # 输出覆盖率文件
go tool cover -html=cover.out   # 浏览器查看覆盖率详情
```

---

## 二、为什么需要 Mock？

真实项目中，代码不可能是纯函数，一定会有各种依赖：

```go
// OrderService 依赖了三个外部服务
type OrderService struct {
    userService  UserService   // 用户服务 RPC
    inventorySvc InventorySvc  // 库存服务 RPC
    payment      Payment       // 支付接口
    db           *gorm.DB      // 数据库
}

func (s *OrderService) CreateOrder(req *CreateOrderReq) (*Order, error) {
    // 1. 调用用户服务查用户
    user, err := s.userService.GetUser(req.UserID)
    if err != nil {
        return nil, err
    }

    // 2. 调用库存服务扣库存
    if err := s.inventorySvc.Deduct(req.GoodsID, req.Quantity); err != nil {
        return nil, err
    }

    // 3. 调用支付服务扣款
    if err := s.payment.Pay(req.Amount); err != nil {
        return nil, err
    }

    // 4. 写数据库
    order := &Order{...}
    if err := s.db.Create(order).Error; err != nil {
        return nil, err
    }

    return order, nil
}
```

**问题来了**：单元测试这个函数，难道要真的启动 RPC、真的连数据库吗？

❌ 绝对不行！
- 单元测试要快（毫秒级）
- 单元测试要稳定（不能网络断了就失败）
- 单元测试要可控（想让依赖返回什么就返回什么）

✅ 解决方案：**Mock 所有外部依赖**

---

## 三、Mock 的四种方式（面试按这个顺序说）

| 方式 | 适用场景 | 优点 | 缺点 |
|------|---------|------|------|
| **手写 Mock 实现** | 接口方法少，简单场景 | 无依赖，最灵活 | 方法多了写起来累 |
| **gomock** | 接口方法多，团队统一规范 | 自动生成，功能全 | 需要学习工具 |
| **testify/mock** | 喜欢断言风格的团队 | API 优雅，社区活跃 | 也是第三方库 |
| **Monkey Patch** | 不得不 Mock 函数/私有方法 | 啥都能 Mock | **黑魔法，慎用** |

---

### 方式 1：手写 Mock（面试一定要会，最基础！）

**核心思想**：面向接口编程，手写一个实现接口的假对象。

#### Step 1：依赖用接口定义

```go
// 1. 定义接口（核心！不定义接口没法 Mock）
type UserService interface {
    GetUser(ctx context.Context, id int64) (*User, error)
}

type InventorySvc interface {
    Deduct(ctx context.Context, goodsID int64, quantity int) error
}

type Payment interface {
    Pay(ctx context.Context, amount int64) error
}

// 2. Service 依赖接口，不依赖具体实现
type OrderService struct {
    userService  UserService   // 接口类型！
    inventorySvc InventorySvc  // 接口类型！
    payment      Payment       // 接口类型！
}
```

> **面试关键**：代码要面向接口写，否则根本没法 Mock！这也是为什么 Go 一直强调面向接口编程的原因。

#### Step 2：手写 Mock 实现

```go
// MockUserService 实现 UserService 接口
type MockUserService struct {
    // 用字段控制返回值
    MockGetUser func(ctx context.Context, id int64) (*User, error)

    // 还可以记录调用参数，做断言
    CalledGetUser bool
    LastUserID    int64
}

func (m *MockUserService) GetUser(ctx context.Context, id int64) (*User, error) {
    m.CalledGetUser = true
    m.LastUserID = id
    return m.MockGetUser(ctx, id)  // 调用我们传入的假逻辑
}

// MockInventorySvc 同理
type MockInventorySvc struct {
    MockDeduct func(ctx context.Context, goodsID int64, quantity int) error
}

func (m *MockInventorySvc) Deduct(ctx context.Context, goodsID int64, quantity int) error {
    return m.MockDeduct(ctx, goodsID, quantity)
}

// MockPayment 同理
type MockPayment struct {
    MockPay func(ctx context.Context, amount int64) error
}

func (m *MockPayment) Pay(ctx context.Context, amount int64) error {
    return m.MockPay(ctx, amount)
}
```

#### Step 3：写测试用例

```go
func TestOrderService_CreateOrder_Success(t *testing.T) {
    // 1. 创建 Mock 对象，配置行为
    mockUser := &MockUserService{
        MockGetUser: func(ctx context.Context, id int64) (*User, error) {
            return &User{ID: id, Name: "测试用户"}, nil
        },
    }

    mockInventory := &MockInventorySvc{
        MockDeduct: func(ctx context.Context, goodsID int64, quantity int) error {
            return nil  // 扣库存成功
        },
    }

    mockPayment := &MockPayment{
        MockPay: func(ctx context.Context, amount int64) error {
            return nil  // 支付成功
        },
    }

    // 2. 把 Mock 注入到 Service
    svc := NewOrderService(mockUser, mockInventory, mockPayment)

    // 3. 执行测试
    order, err := svc.CreateOrder(&CreateOrderReq{
        UserID:   123,
        GoodsID:  456,
        Quantity: 2,
        Amount:   10000,
    })

    // 4. 断言结果
    assert.NoError(t, err)
    assert.NotNil(t, order)

    // 5. 断言 Mock 确实被调用了（可选但很重要）
    assert.True(t, mockUser.CalledGetUser)
    assert.Equal(t, int64(123), mockUser.LastUserID)
}
```

#### 测试异常场景

```go
func TestOrderService_CreateOrder_UserNotFound(t *testing.T) {
    // 让用户服务返回"用户不存在"错误
    mockUser := &MockUserService{
        MockGetUser: func(ctx context.Context, id int64) (*User, error) {
            return nil, errors.New("用户不存在")
        },
    }

    mockInventory := &MockInventorySvc{
        MockDeduct: func(ctx context.Context, goodsID int64, quantity int) error {
            t.Fatal("扣库存不应该被调用！")  // 用户不存在的话，扣库存应该不会执行
            return nil
        },
    }

    mockPayment := &MockPayment{...}

    svc := NewOrderService(mockUser, mockInventory, mockPayment)
    _, err := svc.CreateOrder(&CreateOrderReq{...})

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "用户不存在")
}
```

**手写 Mock 总结**：
- ✅ 不需要任何第三方库
- ✅ 完全可控，想怎么模拟就怎么模拟
- ✅ 可以断言调用次数、调用参数
- ❌ 接口方法多了写起来有点繁琐
- **面试必问**：这是最基础的方式，一定要掌握原理

---

### 方式 2：gomock（官方推荐，自动生成）

gomock 是 Google 官方的 Mock 工具，不需要手写 Mock 实现，自动生成。

#### 安装
```bash
go install go.uber.org/mock/mockgen@latest  # 注意：原 google/gomock 已经移到 uber 维护了
```

#### Step 1：go:generate 生成 Mock

```go
// user_service.go

//go:generate mockgen -source=user_service.go -destination=mock_user_service.go -package=main UserService
type UserService interface {
    GetUser(ctx context.Context, id int64) (*User, error)
    CreateUser(ctx context.Context, user *User) error
    UpdateUser(ctx context.Context, user *User) error
    DeleteUser(ctx context.Context, id int64) error
    // 10 个方法也不怕，自动生成
}
```

运行生成：
```bash
go generate ./...
```

#### Step 2：写测试用例

```go
func TestOrderService_CreateOrder(t *testing.T) {
    // 1. 创建 gomock 控制器
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()  // 自动验证所有期望的调用都被执行了

    // 2. 创建 Mock 对象
    mockUser := NewMockUserService(ctrl)
    mockInventory := NewMockInventorySvc(ctrl)
    mockPayment := NewMockPayment(ctrl)

    // 3. 设置期望行为（这是 gomock 最强大的地方）
    mockUser.EXPECT().
        GetUser(gomock.Any(), int64(123)).  // 参数匹配：ctx 任意，id 必须是 123
        Return(&User{ID: 123, Name: "测试用户"}, nil).
        Times(1)  // 必须调用 1 次

    mockInventory.EXPECT().
        Deduct(gomock.Any(), int64(456), 2).
        Return(nil).
        Times(1)

    mockPayment.EXPECT().
        Pay(gomock.Any(), int64(10000)).
        Return(nil).
        Times(1)

    // 4. 注入并测试
    svc := NewOrderService(mockUser, mockInventory, mockPayment)
    order, err := svc.CreateOrder(&CreateOrderReq{
        UserID:   123,
        GoodsID:  456,
        Quantity: 2,
        Amount:   10000,
    })

    // 5. 断言
    assert.NoError(t, err)
    assert.NotNil(t, order)

    // Finish() 会自动验证：
    // - 所有 EXPECT 的调用都真的被执行了
    // - 调用次数符合要求
    // - 参数匹配正确
}
```

#### gomock 高级匹配

```go
// 参数匹配
mockUser.EXPECT().GetUser(gomock.Any(), gomock.Any())  // 任意参数
mockUser.EXPECT().GetUser(gomock.Any(), gomock.Eq(int64(123)))  // 等于
mockUser.EXPECT().GetUser(gomock.Any(), gomock.Not(int64(0)))   // 不等于
mockUser.EXPECT().GetUser(gomock.Any(), gomock.GreaterThan(int64(0)))  // 大于

// 调用顺序
gomock.InOrder(
    mockUser.EXPECT().GetUser(...),  // 必须先调用 GetUser
    mockInventory.EXPECT().Deduct(...),  // 然后调用 Deduct
    mockPayment.EXPECT().Pay(...),   // 最后调用 Pay
)

// 动态返回值
mockUser.EXPECT().GetUser(gomock.Any(), gomock.Any()).DoAndReturn(
    func(ctx context.Context, id int64) (*User, error) {
        if id < 0 {
            return nil, errors.New("invalid id")
        }
        return &User{ID: id}, nil
    },
)
```

**gomock 总结**：
- ✅ 官方推荐，自动生成 Mock 代码
- ✅ 强大的参数匹配、调用顺序、调用次数验证
- ✅ 接口方法再多也不怕
- ❌ 需要学习 mockgen 工具
- ❌ 只能 Mock 接口（不能 Mock 函数、结构体方法）

---

### 方式 3：testify/mock（社区最流行）

testify 是 Go 社区最流行的测试库，mock 是其中一个模块。

#### 安装
```bash
go get github.com/stretchr/testify/mock
```

#### 手写 Mock 实现（比纯手写简单）

```go
// MockUserService 继承 testify 的 mock.Mock
type MockUserService struct {
    mock.Mock
}

func (m *MockUserService) GetUser(ctx context.Context, id int64) (*User, error) {
    args := m.Called(ctx, id)  // 调用 testify 的 Called 方法
    return args.Get(0).(*User), args.Error(1)  // 按索引取返回值
}
```

#### 测试用例

```go
func TestOrderService_CreateOrder(t *testing.T) {
    // 1. 创建 Mock
    mockUser := new(MockUserService)
    mockInventory := new(MockInventorySvc)
    mockPayment := new(MockPayment)

    // 2. 设置期望
    mockUser.On("GetUser", mock.Anything, int64(123)).
        Return(&User{ID: 123, Name: "测试用户"}, nil).
        Once()  // 调用一次

    mockInventory.On("Deduct", mock.Anything, int64(456), 2).
        Return(nil).
        Once()

    mockPayment.On("Pay", mock.Anything, int64(10000)).
        Return(nil).
        Once()

    // 3. 注入并测试
    svc := NewOrderService(mockUser, mockInventory, mockPayment)
    order, err := svc.CreateOrder(&CreateOrderReq{...})

    // 4. 断言
    assert.NoError(t, err)

    // 5. 验证所有期望都被满足
    mockUser.AssertExpectations(t)
    mockInventory.AssertExpectations(t)
    mockPayment.AssertExpectations(t)
}
```

**testify/mock 总结**：
- ✅ API 优雅，和 gomock 类似
- ✅ 社区非常流行，很多开源项目在用
- ✅ 和 testify/assert、testify/suite 配合使用体验好
- ❌ 需要手写 Mock 方法（比纯手写简单，但还是要写）
- ❌ 也是只能 Mock 接口

---

### 方式 4：Monkey Patch（黑魔法，面试可以装 X）

有时候你不得不 Mock 一些不是接口的东西：
- 包级别的函数
- 结构体的私有方法
- 第三方库的函数（你改不了它的代码）

这时候就需要 Monkey Patch（猴子补丁），直接在运行时修改函数指针。

#### 安装
```bash
go get github.com/bouk/monkey
```

#### 使用示例

```go
func TestCreateOrder(t *testing.T) {
    // Patch 掉 time.Now 函数，让它永远返回固定时间
    monkey.Patch(time.Now, func() time.Time {
        return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
    })
    defer monkey.Unpatch(time.Now)  // 测试完恢复

    // Patch 私有方法（需要用反射）
    user := &User{}
    monkey.PatchInstanceMethod(reflect.TypeOf(user), "calculateLevel",
        func(u *User) int { return 99 },
    )

    // 现在测试中所有调用 time.Now 的地方都会返回 2024-01-01
    order, err := CreateOrder(...)
    assert.Equal(t, "2024-01-01", order.CreateTime.Format("2006-01-02"))
}
```

**⚠️ 非常重要的警告**：
1. **不是线程安全**：绝对不能并行测试（`go test -parallel`）
2. **只能在测试中用**：生产代码绝对不能用
3. **能不用就不用**：用这个通常意味着你的代码设计有问题
4. **面试提一句就行**：知道有这个东西，但优先用接口 Mock

---

## 四、数据库 Mock 怎么搞？

业务代码 80% 都在和数据库打交道，数据库 Mock 是重中之重。

### 方案 1：sqlmock（最常用）

专门 Mock `database/sql` 接口的库，不需要真的连数据库。

#### 安装
```bash
go get github.com/DATA-DOG/go-sqlmock
```

#### 示例

```go
func TestUserRepo_GetByID(t *testing.T) {
    // 1. 创建 mock DB
    db, mock, err := sqlmock.New()
    assert.NoError(t, err)
    defer db.Close()

    // 2. 设置期望：期望执行一条 SELECT，返回一行数据
    rows := sqlmock.NewRows([]string{"id", "name", "age"}).
        AddRow(1, "测试用户", 25)

    mock.ExpectQuery("SELECT id, name, age FROM users WHERE id = ?").
        WithArgs(1).  // 参数必须是 1
        WillReturnRows(rows)

    // 3. 注入 mock DB 到 Repo
    repo := NewUserRepo(db)

    // 4. 测试
    user, err := repo.GetByID(context.Background(), 1)

    // 5. 断言
    assert.NoError(t, err)
    assert.Equal(t, int64(1), user.ID)
    assert.Equal(t, "测试用户", user.Name)

    // 6. 验证所有期望都被满足
    assert.NoError(t, mock.ExpectationsWereMet())
}
```

### 方案 2：用真实数据库（集成测试）

对于复杂的 SQL，Mock 太麻烦了，不如用真实的数据库：

1. CI/CD 时启动一个临时 MySQL 容器
2. 每次测试前建表，测试完删除
3. 用事务回滚代替删表（更快）

```go
func TestUserRepo_Create(t *testing.T) {
    // 用 docker 启动一个临时 MySQL
    // 或者每个测试用例开一个事务，测试完 rollback
    tx := db.Begin()
    defer tx.Rollback()  // 测试完直接回滚，不影响其他用例

    repo := NewUserRepo(tx)

    // 测试...
}
```

---

## 五、真实项目最佳实践（面试加分项）

### ✅ 实践 1：所有外部依赖都抽象成接口

```go
// 错误写法：依赖具体实现，没法 Mock
type OrderService struct {
    userRPC *userclient.Client  // 具体的 RPC 客户端
    db      *gorm.DB            // 具体的数据库
}

// 正确写法：依赖接口，可 Mock
type OrderService struct {
    userRPC  UserService        // 接口
    db       DBRepository       // 接口
}
```

### ✅ 实践 2：依赖注入（DI），不要在内部 new

```go
// 错误写法：内部创建依赖，耦合太紧，没法换 Mock
func NewOrderService() *OrderService {
    return &OrderService{
        userRPC: userclient.New("http://user-service"),  // 内部写死了
        db:      gorm.Open("mysql", dsn),
    }
}

// 正确写法：依赖从外面传进来（依赖注入）
func NewOrderService(userRPC UserService, db DBRepository) *OrderService {
    return &OrderService{
        userRPC: userRPC,  // 测试时传 Mock，生产传真实实现
        db:      db,
    }
}
```

### ✅ 实践 3：分层测试，不要什么都集成测

```
┌─────────────────────────────────────┐
│  Handlers (API 层)                   │  ← 用 httptest 测，Mock Service
│    └───────────────────────────────┐ │
│      Service 层 (业务逻辑)          │  ← 重点测试！Mock DAO/RPC
│        └─────────────────────────┐ │ │
│          DAO 层 (数据库)          │ │  ← 用 sqlmock 或真实测试库
│            └───────────────────┐ │ │ │
│              RPC/外部服务       │ │ │  ← 用 gomock 或手写 Mock
└─────────────────────────────────┴─┴─┘
```

**重点测试 Service 层的业务逻辑**，这是最复杂、最容易出 bug 的地方。

### ✅ 实践 4：测试用例要覆盖边界

一个好的测试套件应该覆盖：
- ✅ 正常流程（Happy Path）
- ✅ 各种错误场景（依赖返回错误、超时）
- ✅ 边界值（零值、空值、最大值）
- ✅ 幂等测试（重复调用结果一样）
- ✅ 并发测试（有锁的地方）

---

## 六、面试标准答案模板

**面试官问**：Go 单元测试怎么写？依赖怎么 Mock？

**标准回答**：

> Go 的单元测试我主要用表驱动测试（Table Driven Test）的写法，子测试可以单独运行，新增用例也方便。
>
> **Mock 主要有四种方式**：
>
> 1. **手写 Mock 实现**：这是最基础的方式，先把所有依赖都抽象成接口，然后手写一个实现接口的 Mock 结构体，里面用字段控制返回值，还可以记录调用参数做断言。不需要第三方库，最灵活。
>
> 2. **gomock**：Google 官方的工具，用 mockgen 自动生成 Mock 代码，不需要手写。支持参数匹配、调用顺序、调用次数验证，接口方法再多也不怕。
>
> 3. **testify/mock**：社区用得比较多的库，API 很优雅，和 testify/assert 配合很好。
>
> 4. **Monkey Patch**：黑魔法，可以 Mock 函数和私有方法，但不是线程安全的，一般不推荐用，知道有这个东西就行。
>
> **数据库 Mock** 主要用 sqlmock，可以模拟 SQL 的执行和返回结果。
>
> **最重要的一点**：代码一定要面向接口编程，而且依赖要从外面注入（依赖注入），否则根本没法 Mock。

---

## 七、常用工具链汇总

| 工具 | 用途 | 地址 |
|------|------|------|
| `testing` | 标准库测试框架 | 内置 |
| `testify/assert` | 断言库 | github.com/stretchr/testify/assert |
| `testify/suite` | 测试套件，支持 Setup/Teardown | github.com/stretchr/testify/suite |
| `gomock` | 官方 Mock 工具 | go.uber.org/mock |
| `sqlmock` | 数据库 Mock | github.com/DATA-DOG/go-sqlmock |
| `httptest` | HTTP 接口测试 | 内置 |
| `monkey` | 猴子补丁 | github.com/bouk/monkey |

---

**文档位置**：`01-go-basics/12-unit-test-and-mock.md`
