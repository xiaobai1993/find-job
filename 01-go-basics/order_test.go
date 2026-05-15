package main

import (
	"context"
	"errors"
	"testing"
)

// ==========================================
// Go 单元测试 + Mock 完整示例
// 运行：go test -v order_test.go
// ==========================================

// ------------------------------
// 业务代码（正常应该在单独的文件里）
// ------------------------------

type User struct {
	ID   int64
	Name string
	Age  int
}

// UserService 用户服务接口（核心！面向接口编程才能 Mock）
type UserService interface {
	GetUser(ctx context.Context, id int64) (*User, error)
	CreateUser(ctx context.Context, user *User) error
}

// OrderService 订单服务，依赖 UserService
type OrderService struct {
	userService UserService // 依赖接口！不是具体实现
}

func NewOrderService(us UserService) *OrderService {
	return &OrderService{userService: us}
}

// CreateOrder 创建订单：先查用户，再创建订单
func (s *OrderService) CreateOrder(userID int64, goods string) (string, error) {
	user, err := s.userService.GetUser(context.Background(), userID)
	if err != nil {
		return "", errors.New("查询用户失败")
	}

	if user.Age < 18 {
		return "", errors.New("未成年人不能下单")
	}

	return "order_ok", nil
}

// ------------------------------
// Mock 实现（手写方式）
// ------------------------------

// MockUserService 实现 UserService 接口
type MockUserService struct {
	// 控制返回值
	MockGetUser    func(ctx context.Context, id int64) (*User, error)
	MockCreateUser func(ctx context.Context, user *User) error

	// 记录调用信息，用于断言
	GetUserCalled    bool
	CreateUserCalled bool
	LastUserID       int64
	CallCount        int
}

func (m *MockUserService) GetUser(ctx context.Context, id int64) (*User, error) {
	m.GetUserCalled = true
	m.LastUserID = id
	m.CallCount++
	return m.MockGetUser(ctx, id)
}

func (m *MockUserService) CreateUser(ctx context.Context, user *User) error {
	m.CreateUserCalled = true
	return m.MockCreateUser(ctx, user)
}

// ------------------------------
// 测试用例 1：正常用户下单成功
// ------------------------------

func TestCreateOrder_Success(t *testing.T) {
	// 1. 创建 Mock 并设置行为
	mockUser := &MockUserService{
		MockGetUser: func(ctx context.Context, id int64) (*User, error) {
			return &User{ID: id, Name: "测试用户", Age: 25}, nil
		},
	}

	// 2. 注入 Mock
	svc := NewOrderService(mockUser)

	// 3. 执行业务逻辑
	orderID, err := svc.CreateOrder(123, "iPhone")

	// 4. 断言结果
	if err != nil {
		t.Fatalf("期望成功，实际失败: %v", err)
	}
	if orderID != "order_ok" {
		t.Fatalf("订单号不匹配: %s", orderID)
	}

	// 5. 断言 Mock 确实被正确调用了
	if !mockUser.GetUserCalled {
		t.Fatal("GetUser 应该被调用，但是没有")
	}
	if mockUser.LastUserID != 123 {
		t.Fatalf("应该调用用户 123，实际调用了: %d", mockUser.LastUserID)
	}
	if mockUser.CallCount != 1 {
		t.Fatalf("应该调用 1 次，实际调用了 %d 次", mockUser.CallCount)
	}

	t.Log("✅ 测试通过：正常用户下单成功")
}

// ------------------------------
// 测试用例 2：未成年人，拒绝下单
// ------------------------------

func TestCreateOrder_Underage(t *testing.T) {
	mockUser := &MockUserService{
		MockGetUser: func(ctx context.Context, id int64) (*User, error) {
			return &User{ID: id, Name: "小朋友", Age: 15}, nil
		},
	}

	svc := NewOrderService(mockUser)
	_, err := svc.CreateOrder(456, "游戏皮肤")

	if err == nil {
		t.Fatal("未成年人应该不能下单，但是成功了")
	}
	if err.Error() != "未成年人不能下单" {
		t.Fatalf("错误信息不匹配: %v", err)
	}

	t.Log("✅ 测试通过：未成年人下单被拒绝")
}

// ------------------------------
// 测试用例 3：用户不存在
// ------------------------------

func TestCreateOrder_UserNotFound(t *testing.T) {
	mockUser := &MockUserService{
		MockGetUser: func(ctx context.Context, id int64) (*User, error) {
			return nil, errors.New("db error: user not found")
		},
	}

	svc := NewOrderService(mockUser)
	_, err := svc.CreateOrder(999, "iPhone")

	if err == nil {
		t.Fatal("用户不存在应该返回错误")
	}
	if err.Error() != "查询用户失败" {
		t.Fatalf("错误信息不匹配: %v", err)
	}

	t.Log("✅ 测试通过：用户不存在")
}

// ------------------------------
// 测试用例 4：表驱动测试（Go 社区推荐写法）
// ------------------------------

func TestCreateOrder_TableDriven(t *testing.T) {
	tests := []struct {
		name    string // 用例名称
		userID  int64  // 输入
		mockAge int    // Mock 用户年龄
		mockErr error  // Mock 返回的错误
		wantErr bool   // 期望是否出错
	}{
		{
			name:    "正常用户，下单成功",
			userID:  123,
			mockAge: 25,
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "刚满 18 岁，可以下单",
			userID:  789,
			mockAge: 18,
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "差一天 18 岁，拒绝下单",
			userID:  456,
			mockAge: 17,
			mockErr: nil,
			wantErr: true,
		},
		{
			name:    "用户不存在",
			userID:  999,
			mockAge: 0,
			mockErr: errors.New("user not found"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		// 使用 t.Run 运行子测试，可以单独运行某个用例
		t.Run(tt.name, func(t *testing.T) {
			// 每个子测试有独立的 Mock，互不影响
			mockUser := &MockUserService{
				MockGetUser: func(ctx context.Context, id int64) (*User, error) {
					if tt.mockErr != nil {
						return nil, tt.mockErr
					}
					return &User{ID: id, Name: "测试用户", Age: tt.mockAge}, nil
				},
			}

			svc := NewOrderService(mockUser)
			_, err := svc.CreateOrder(tt.userID, "goods")

			// 断言结果
			if tt.wantErr {
				if err == nil {
					t.Fatal("期望出错，但是成功了")
				}
			} else {
				if err != nil {
					t.Fatalf("期望成功，但是失败了: %v", err)
				}
			}

			// 断言 Mock 被调用了
			if !mockUser.GetUserCalled {
				t.Fatal("GetUser 应该被调用")
			}
		})
	}

	t.Log("✅ 表驱动测试全部通过（4 个用例）")
}
