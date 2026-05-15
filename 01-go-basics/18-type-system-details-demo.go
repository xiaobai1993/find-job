// Go 类型系统细节演示：别名 vs 定义、可比较类型、相等性判断
// go run 18-type-system-details-demo.go
package main

import (
	"fmt"
	"reflect"
)

// ============================================================
// 1. 类型定义（Type Definition）- 全新的类型
// ============================================================

type MyInt int // 全新的类型，和 int 不是同一个类型

func demoTypeDefinition() {
	fmt.Println("=== 1. 类型定义（Type Definition）===")

	var a int = 10
	var b MyInt

	// b = a  // ❌ 编译错误！类型不匹配
	b = MyInt(a) // ✅ 必须显式转换

	fmt.Printf("a 的类型: %T, 值: %d\n", a, a)
	fmt.Printf("b 的类型: %T, 值: %d\n", b, b)
	fmt.Println("类型定义创建了全新的类型，必须显式转换")
	fmt.Println()
}

// ============================================================
// 2. 类型别名（Type Alias）- 同一个类型
// ============================================================

type MyIntAlias = int // int 的别名，和 int 是同一个类型

func demoTypeAlias() {
	fmt.Println("=== 2. 类型别名（Type Alias）===")

	var a int = 10
	var b MyIntAlias

	b = a // ✅ 不需要转换，就是同一个类型

	fmt.Printf("a 的类型: %T\n", a)
	fmt.Printf("b 的类型: %T\n", b)
	fmt.Println("类型别名就是同一个类型，不需要转换")
	fmt.Println()
}

// ============================================================
// 3. 可比较类型 vs 不可比较类型
// ============================================================

type Point struct {
	X, Y int
}

type BadPoint struct {
	X, Y int
	Tags []string // 包含 slice，整个结构体就不可比较了
}

func demoComparableTypes() {
	fmt.Println("=== 3. 可比较类型 ===")

	// 基础类型可比较
	fmt.Println("int 1 == 1:", 1 == 1) // ✅ true

	// 指针可比较
	x := 100
	p1 := &x
	p2 := &x
	fmt.Println("指针 p1 == p2:", p1 == p2) // ✅ true，指向同一个地址

	// 结构体可比较（所有字段都可比较）
	point1 := Point{1, 2}
	point2 := Point{1, 2}
	fmt.Println("结构体 point1 == point2:", point1 == point2) // ✅ true

	// 空 struct 可比较
	var s1, s2 struct{}
	fmt.Println("空 struct s1 == s2:", s1 == s2) // ✅ true

	// 切片只能和 nil 比较
	s := []int{1, 2}
	// s == s  // ❌ 编译错误！slice 不能直接比较

	fmt.Println("slice == nil:", s == nil) // ✅ 可以和 nil 比较

	// map 也只能和 nil 比较
	m := map[string]int{"a": 1}
	fmt.Println("map == nil:", m == nil) // ✅ 可以和 nil 比较

	// 函数也只能和 nil 比较
	var f func() = nil
	fmt.Println("func == nil:", f == nil) // ✅ 可以和 nil 比较

	fmt.Println()
}

// ============================================================
// 4. interface 相等性判断的坑
// ============================================================

type MyInterface interface {
	String() string
}

type MyString string

func (m MyString) String() string {
	return string(m)
}

func demoInterfaceEquality() {
	fmt.Println("=== 4. interface 相等性的坑 ===")

	var a interface{} = 100 // type=int, value=100
	var b interface{} = MyString("100") // type=MyString, value="100"

	fmt.Println("a type: type=%T, value=%v", a, a)
	fmt.Println("b type: type=%T, value=%v", b, b)
	fmt.Println("a == b:", a == b) // ❌ false！类型不同，即使值看起来一样

	fmt.Println()

	// 经典坑：interface != nil 但 value 是 nil
	var p *int = nil
	var i interface{} = p

	fmt.Println("i == nil:", i == nil) // ❌ false！type 不是 nil (type=*int)
	fmt.Println("i 的 type 是 *int，不是 nil，所以整体不等于 nil")

	fmt.Println()
}

// ============================================================
// 5. reflect.DeepEqual 深层比较
// ============================================================

func demoDeepEqual() {
	fmt.Println("=== 5. reflect.DeepEqual 深层比较 ===")

	s1 := []int{1, 2, 3}
	s2 := []int{1, 2, 3}

	// s1 == s2  // ❌ 编译错误
	fmt.Println("DeepEqual slice:", reflect.DeepEqual(s1, s2)) // ✅ true

	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"b": 2, "a": 1}
	fmt.Println("DeepEqual map:", reflect.DeepEqual(m1, m2)) // ✅ true

	fmt.Println()
}

// ============================================================
// 6. 类型断言
// ============================================================

func demoTypeAssertion() {
	fmt.Println("=== 6. 类型断言 ===")

	var i interface{} = "hello"

	// 单返回值，类型不对就 panic
	s := i.(string)
	fmt.Println("string:", s)

	// 双返回值，安全，不会 panic
	f, ok := i.(float64)
	if ok {
		fmt.Println("float64:", f)
	} else {
		fmt.Println("不是 float64 类型")
	}

	fmt.Println()
}

// ============================================================
// main
// ============================================================

func main() {
	demoTypeDefinition()
	demoTypeAlias()
	demoComparableTypes()
	demoInterfaceEquality()
	demoDeepEqual()
	demoTypeAssertion()

	fmt.Println("✅ 所有示例运行完成！")
}
