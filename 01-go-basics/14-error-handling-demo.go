// Go 错误处理最佳实践演示
// go run 14-error-handling-demo.go
package main

import (
	"errors"
	"fmt"
	"os"
)

// ============================================================
// 1. errors.Is 示例
// ============================================================

var ErrNotFound = errors.New("not found")

func GetUser(id int) (*User, error) {
	if id <= 0 {
		return nil, fmt.Errorf("get user %d: %w", id, ErrNotFound)
	}
	return &User{ID: id, Name: "test"}, nil
}

type User struct {
	ID   int
	Name string
}

func demoErrorsIs() {
	fmt.Println("=== 1. errors.Is 示例 ===")
	_, err := GetUser(-1)
	if errors.Is(err, ErrNotFound) {
		fmt.Println("✅ 错误链中包含 ErrNotFound")
	} else {
		fmt.Println("❌ 不匹配")
	}
	fmt.Println()
}

// ============================================================
// 2. errors.As 示例
// ============================================================

type BizError struct {
	Code    int
	Message string
	Cause   error
}

func (e *BizError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("biz error code=%d: %s, cause=%v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("biz error code=%d: %s", e.Code, e.Message)
}

func (e *BizError) Unwrap() error {
	return e.Cause
}

func CreateUser(name string) error {
	if len(name) == 0 {
		return &BizError{
			Code:    40001,
			Message: "name is empty",
		}
	}
	return nil
}

func demoErrorsAs() {
	fmt.Println("=== 2. errors.As 示例 ===")
	err := CreateUser("")
	var bizErr *BizError
	if errors.As(err, &bizErr) {
		fmt.Printf("✅ 提取到 BizError: code=%d, msg=%s\n", bizErr.Code, bizErr.Message)
	}
	fmt.Println()
}

// ============================================================
// 3. 错误包装：%w vs %v
// ============================================================

func demoErrorWrap() {
	fmt.Println("=== 3. 错误包装：%w vs %v ===")
	originalErr := os.ErrNotExist

	wrappedW := fmt.Errorf("wrap with %%w: %w", originalErr)
	wrappedV := fmt.Errorf("wrap with %%v: %v", originalErr)

	fmt.Println("%w 包装后 errors.Is 能匹配:", errors.Is(wrappedW, os.ErrNotExist)) // true
	fmt.Println("%v 包装后 errors.Is 能匹配:", errors.Is(wrappedV, os.ErrNotExist)) // false
	fmt.Println()
}

// ============================================================
// 4. 自定义错误
// ============================================================

func demoCustomError() {
	fmt.Println("=== 4. 自定义错误示例 ===")
	err := &BizError{Code: 500, Message: "internal error", Cause: os.ErrPermission}
	fmt.Println("Error():", err.Error())
	fmt.Println("Unwrap():", err.Unwrap())
	fmt.Println()
}

// ============================================================
// 5. errors.Join (Go 1.20+)
// ============================================================

func demoErrorsJoin() {
	fmt.Println("=== 5. errors.Join 示例 ===")
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")
	joined := errors.Join(err1, err2, err3)
	fmt.Println("Joined error:", joined)
	fmt.Println("Contains err1:", errors.Is(joined, err1))
	fmt.Println("Contains err2:", errors.Is(joined, err2))
	fmt.Println()
}

// ============================================================
// main
// ============================================================

func main() {
	demoErrorsIs()
	demoErrorsAs()
	demoErrorWrap()
	demoCustomError()
	demoErrorsJoin()

	fmt.Println("✅ 所有示例运行完成！")
}
