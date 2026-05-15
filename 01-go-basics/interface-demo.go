package main

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
)

// ======================================
// Part 1: Interface 示例
// ======================================

// Shape 接口定义
type Shape interface {
	Area() float64
	Perimeter() float64
}

// Circle 实现 Shape
type Circle struct {
	Radius float64
}

func (c Circle) Area() float64 {
	return math.Pi * c.Radius * c.Radius
}

func (c Circle) Perimeter() float64 {
	return 2 * math.Pi * c.Radius
}

// Rectangle 实现 Shape
type Rectangle struct {
	Width  float64
	Height float64
}

func (r Rectangle) Area() float64 {
	return r.Width * r.Height
}

func (r Rectangle) Perimeter() float64 {
	return 2 * (r.Width + r.Height)
}

// 经典 nil interface 陷阱
type MyError struct {
	Msg string
}

func (e *MyError) Error() string {
	return e.Msg
}

func GetError() error {
	var err *MyError = nil
	return err // 返回的不是 nil interface！
}

func demoNilInterface() {
	fmt.Println("=== nil interface 陷阱 ===")
	err := GetError()
	if err != nil {
		fmt.Println("❌ 进入了错误分支！err != nil")
		fmt.Printf("   err type: %T, value: %v\n", err, err)
	} else {
		fmt.Println("✅ err 是 nil")
	}
	fmt.Println()
}

// 类型断言
func demoTypeAssertion() {
	fmt.Println("=== 类型断言 ===")
	var s Shape = Circle{Radius: 5}

	// 安全断言
	c, ok := s.(Circle)
	if ok {
		fmt.Printf("✅ Circle 半径: %v\n", c.Radius)
	}

	// 类型开关
	printShapeInfo(s)
	fmt.Println()
}

func printShapeInfo(s Shape) {
	switch v := s.(type) {
	case Circle:
		fmt.Printf("这是圆形，半径 = %.1f\n", v.Radius)
	case Rectangle:
		fmt.Printf("这是矩形，宽 = %.1f, 高 = %.1f\n", v.Width, v.Height)
	default:
		fmt.Println("未知形状")
	}
}

// ======================================
// Part 2: 泛型示例
// ======================================

// Number 约束接口
type Number interface {
	~int | ~int64 | ~float64
}

// 泛型 Max 函数
func Max[T Number](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// 泛型 Map 函数
func Map[T, U any](s []T, f func(T) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = f(v)
	}
	return result
}

// 泛型结构体
type Pair[T, U any] struct {
	First  T
	Second U
}

func demoGenerics() {
	fmt.Println("=== 泛型示例 ===")

	// Max 函数
	fmt.Printf("Max(10, 20) = %d\n", Max(10, 20))
	fmt.Printf("Max(3.14, 2.71) = %.2f\n", Max(3.14, 2.71))

	// Map 函数
	nums := []int{1, 2, 3, 4, 5}
	doubled := Map(nums, func(n int) int { return n * 2 })
	fmt.Printf("Map 翻倍: %v\n", doubled)

	strings := Map(nums, func(n int) string {
		return fmt.Sprintf("num:%d", n)
	})
	fmt.Printf("Map 转字符串: %v\n", strings)

	// 泛型结构体
	p := Pair[string, int]{First: "age", Second: 25}
	fmt.Printf("Pair: %+v\n", p)

	fmt.Println()
}

// ======================================
// Part 3: 反射示例
// ======================================

type User struct {
	Name  string `json:"name" validate:"required"`
	Age   int    `json:"age" validate:"min=18"`
	Email string `json:"email" validate:"required,email"`
}

func demoTypeVsKind() {
	fmt.Println("=== Type vs Kind ===")

	type MyInt int
	var x MyInt = 42

	t := reflect.TypeOf(x)
	fmt.Printf("Type: %s\n", t.Name())   // MyInt
	fmt.Printf("Kind: %s\n", t.Kind())   // int
	fmt.Println()
}

func demoStructFields() {
	fmt.Println("=== 结构体字段操作 ===")

	u := User{Name: "Alice", Age: 30, Email: "alice@example.com"}
	t := reflect.TypeOf(u)
	v := reflect.ValueOf(u)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)
		fmt.Printf("%s (%s): %v\n", field.Name, field.Type, value.Interface())
		fmt.Printf("  json tag: %s\n", field.Tag.Get("json"))
		fmt.Printf("  validate tag: %s\n", field.Tag.Get("validate"))
	}
	fmt.Println()
}

func demoSetValue() {
	fmt.Println("=== 反射修改值 ===")

	u := User{Name: "Alice", Age: 30}
	fmt.Printf("修改前: %+v\n", u)

	// 必须传指针 + 调用 Elem()
	v := reflect.ValueOf(&u).Elem()

	nameField := v.FieldByName("Name")
	if nameField.CanSet() {
		nameField.SetString("Bob")
	}

	ageField := v.FieldByName("Age")
	if ageField.CanSet() {
		ageField.SetInt(31)
	}

	fmt.Printf("修改后: %+v\n", u)
	fmt.Println()
}

func demoCallMethod() {
	fmt.Println("=== 动态调用方法 ===")

	calc := Calculator{}
	v := reflect.ValueOf(calc)

	method := v.MethodByName("Add")
	args := []reflect.Value{
		reflect.ValueOf(10),
		reflect.ValueOf(20),
	}

	results := method.Call(args)
	fmt.Printf("Calculator.Add(10, 20) = %d\n", results[0].Int())
	fmt.Println()
}

type Calculator struct{}

func (c Calculator) Add(a, b int) int {
	return a + b
}

// ======================================
// Part 4: 综合示例 - 通用校验器
// ======================================

func Validate(obj any) []error {
	var errs []error

	v := reflect.ValueOf(obj)
	t := reflect.TypeOf(obj)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		t = t.Elem()
	}

	if v.Kind() != reflect.Struct {
		return append(errs, fmt.Errorf("只支持校验结构体"))
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		if field.PkgPath != "" {
			continue
		}

		tag := field.Tag.Get("validate")
		if tag == "" {
			continue
		}

		rules := strings.Split(tag, ",")
		for _, rule := range rules {
			err := validateField(field.Name, rule, value)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func validateField(fieldName, rule string, v reflect.Value) error {
	switch {
	case rule == "required":
		if isZero(v) {
			return fmt.Errorf("❌ %s 不能为空", fieldName)
		}
	case rule == "email":
		if v.Kind() != reflect.String {
			return fmt.Errorf("❌ %s 必须是字符串", fieldName)
		}
		email := v.String()
		matched, _ := regexp.MatchString(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`, email)
		if !matched {
			return fmt.Errorf("❌ %s 不是有效的邮箱格式", fieldName)
		}
	case strings.HasPrefix(rule, "min="):
		var min int
		fmt.Sscanf(rule, "min=%d", &min)
		if v.Kind() == reflect.Int && int(v.Int()) < min {
			return fmt.Errorf("❌ %s 不能小于 %d", fieldName, min)
		}
	}
	return nil
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int64, reflect.Int32:
		return v.Int() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.Len() == 0
	default:
		return false
	}
}

type RegisterRequest struct {
	Username string `validate:"required"`
	Email    string `validate:"required,email"`
	Age      int    `validate:"min=18"`
}

func demoValidator() {
	fmt.Println("=== 通用校验器 ===")

	req := RegisterRequest{
		Username: "",
		Email:    "not-an-email",
		Age:      16,
	}

	errs := Validate(req)
	if len(errs) > 0 {
		fmt.Println("校验失败:")
		for _, err := range errs {
			fmt.Println(" ", err)
		}
	} else {
		fmt.Println("✅ 校验通过")
	}
	fmt.Println()
}

// ======================================
// Main
// ======================================

func main() {
	fmt.Println("========== Interface + 泛型 + 反射 完整示例 ==========")
	fmt.Println()

	demoNilInterface()
	demoTypeAssertion()
	demoGenerics()
	demoTypeVsKind()
	demoStructFields()
	demoSetValue()
	demoCallMethod()
	demoValidator()

	fmt.Println("========== 运行完成 ==========")
}
