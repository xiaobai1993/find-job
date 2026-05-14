package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ============================================
// 演示 1：小写字段有 tag 也读不到
// ============================================
type BadUser struct {
	name string `json:"name"` // 小写，读不到！
	Age  int    `json:"age"`  // 大写，正常
}

func demo1PrivateField() {
	fmt.Println("=== 演示 1：小写字段有 tag 也读不到 ===")
	u := BadUser{name: "张三", Age: 18}
	b, _ := json.Marshal(u)
	fmt.Printf("序列化结果: %s\n", string(b))
	fmt.Println("💡 输出里没有 name 字段！因为小写是私有字段，反射读不到 tag")
	fmt.Println()
}

// ============================================
// 演示 2：omitempty 大坑（支付场景必踩）
// ============================================
type Order struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount,omitempty"` // ❌ 大坑！0 会被忽略
}

type OrderFixed struct {
	OrderID string `json:"order_id"`
	Amount  *int   `json:"amount,omitempty"` // ✅ 指针，nil 才忽略，0 不会
}

func demo2Omitempty() {
	fmt.Println("=== 演示 2：omitempty 把 0 忽略了（支付场景大坑） ===")

	// 金额本来就是 0（免费订单）
	o := Order{OrderID: "123", Amount: 0}
	b, _ := json.Marshal(o)
	fmt.Printf("值类型 Amount = 0: %s\n", string(b))
	fmt.Println("⚠️  amount 字段没了！对账的时候你就哭了")

	// 用指针就没问题
	amount := 0
	o2 := OrderFixed{OrderID: "123", Amount: &amount}
	b2, _ := json.Marshal(o2)
	fmt.Printf("指针类型 Amount = 0: %s\n", string(b2))
	fmt.Println("✅ amount 正常输出，0 不会被忽略")
	fmt.Println()
}

// ============================================
// 演示 3：Get vs Lookup 的区别
// ============================================
type User struct {
	Name string `json:"name"`            // 有 tag
	Age  int    `json:""`               // 显式设置为空串
	Addr string                          // 没有 tag
}

func demo3GetVsLookup() {
	fmt.Println("=== 演示 3：Get() vs Lookup() 的区别 ===")
	t := reflect.TypeOf(User{})

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := field.Name

		// 用 Get 读
		getVal := field.Tag.Get("json")

		// 用 Lookup 读
		lookupVal, ok := field.Tag.Lookup("json")

		fmt.Printf("%s: Get() = %q, Lookup() = (%q, %v)\n",
			name, getVal, lookupVal, ok)
	}

	fmt.Println("💡 Age 字段显式设置了 json 空串，所以 Lookup 返回 ok=true")
	fmt.Println("   Addr 字段完全没写 json tag，所以 Lookup 返回 ok=false")
	fmt.Println("   不要用 Get() == \"\" 判断 tag 有没有设置，要用 Lookup！")
	fmt.Println()
}

// ============================================
// 演示 4：匿名结构体字段冲突
// ============================================
type BaseID struct {
	ID int `json:"id"`
}

type UserID struct {
	ID int `json:"user_id"`
}

type BadStruct struct {
	BaseID
	UserID // 两个 ID 冲突！
	Name string `json:"name"`
}

func demo4FieldConflict() {
	fmt.Println("=== 演示 4：匿名结构体字段冲突 ===")

	s := BadStruct{
		BaseID: BaseID{ID: 1},
		UserID: UserID{ID: 2},
		Name:   "张三",
	}
	b, _ := json.Marshal(s)
	fmt.Printf("冲突时序列化结果: %s\n", string(b))
	fmt.Println("⚠️  两个 ID 都没输出！字段冲突时两个都会被忽略，不会报错！")
	fmt.Println()
}

// ============================================
// 演示 5：自定义 tag 解析
// ============================================
type Config struct {
	Host string `default:"localhost"`
	Port int    `default:"3306"`
	Debug bool   `default:"true"`
}

// SetDefault 读取 default tag 并设置默认值
func SetDefault(v interface{}) {
	t := reflect.TypeOf(v).Elem()
	val := reflect.ValueOf(v).Elem()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		defaultVal, ok := field.Tag.Lookup("default")
		if !ok {
			continue
		}

		f := val.Field(i)
		switch field.Type.Kind() {
		case reflect.String:
			if f.String() == "" {
				f.SetString(defaultVal)
			}
		case reflect.Int:
			if f.Int() == 0 {
				n, _ := strconv.Atoi(defaultVal)
				f.SetInt(int64(n))
			}
		case reflect.Bool:
			if !f.Bool() {
				b, _ := strconv.ParseBool(defaultVal)
				f.SetBool(b)
			}
		}
	}
}

func demo5CustomTag() {
	fmt.Println("=== 演示 5：自定义 default tag ===")

	cfg := Config{} // 所有字段都是零值
	fmt.Printf("设置默认值前: %+v\n", cfg)

	SetDefault(&cfg)
	fmt.Printf("设置默认值后: %+v\n", cfg)
	fmt.Println("✅ 所有默认值都从 tag 里读出来并设置好了")
	fmt.Println()
}

// ============================================
// 演示 6："-" 忽略字段
// ============================================
type UserWithPassword struct {
	Name     string `json:"name"`
	Password string `json:"-"` // 永远不序列化
}

func demo6IgnoreField() {
	fmt.Println("=== 演示 6：\"-\" 忽略字段 ===")
	u := UserWithPassword{Name: "张三", Password: "123456"}
	b, _ := json.Marshal(u)
	fmt.Printf("序列化结果: %s\n", string(b))
	fmt.Println("✅ password 字段完全没输出，安全！")
	fmt.Println()
}

// ============================================
// main
// ============================================
func main() {
	demo1PrivateField()
	demo2Omitempty()
	demo3GetVsLookup()
	demo4FieldConflict()
	demo5CustomTag()
	demo6IgnoreField()

	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有坑演示完成！记住这几个坑，面试少踩雷，生产少背锅")
}
