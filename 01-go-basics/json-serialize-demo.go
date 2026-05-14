package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ============================================
// 演示 1：string vs []byte JSON 序列化差异
// ============================================
type Demo1 struct {
	Str string `json:"str"`
	Bts []byte `json:"bts"`
}

func demo1BasicDiff() {
	fmt.Println("=== 演示 1：string vs []byte 序列化差异 ===")

	d := Demo1{
		Str: "hello",
		Bts: []byte("hello"),
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))

	fmt.Println()
	fmt.Println("💡 注意：[]byte 被 base64 编码了！")
	fmt.Println("   原始: hello")
	fmt.Println("   base64: aGVsbG8=")
	fmt.Println()
}

// ============================================
// 演示 2：空 []byte vs nil []byte 差异
// ============================================
type Demo2 struct {
	Empty []byte `json:"empty"` // 空数组，不是 nil
	Nil   []byte `json:"nil"`   // nil
}

func demo2EmptyVsNil() {
	fmt.Println("=== 演示 2：空 []byte vs nil []byte 序列化差异 ===")

	d := Demo2{
		Empty: make([]byte, 0),
		Nil:   nil,
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	fmt.Println(string(b))

	fmt.Println()
	fmt.Println("💡 空的序列化成 \"\"，nil 序列化成 null")
	fmt.Println("   前端处理这两个值的逻辑完全不一样！")
	fmt.Println()
}

// ============================================
// 演示 3：json.RawMessage 的坑
// ============================================
type Demo3Bad struct {
	Data json.RawMessage `json:"data"` // ❌ 值类型，会被 base64！
}

type Demo3Good struct {
	Data *json.RawMessage `json:"data"` // ✅ 指针类型，不会被 base64
}

func demo3RawMessage() {
	fmt.Println("=== 演示 3：json.RawMessage 的坑 ===")

	raw := json.RawMessage(`{"name":"张三","age":18}`)

	// ❌ 错误写法：值类型
	bad := Demo3Bad{Data: raw}
	bBad, _ := json.MarshalIndent(bad, "", "  ")
	fmt.Println("❌ 值类型 json.RawMessage 结果：")
	fmt.Println(string(bBad))

	fmt.Println()

	// ✅ 正确写法：指针类型
	good := Demo3Good{Data: &raw}
	bGood, _ := json.MarshalIndent(good, "", "  ")
	fmt.Println("✅ 指针类型 *json.RawMessage 结果：")
	fmt.Println(string(bGood))

	fmt.Println()
	fmt.Println("💡 90% 的人都踩过这个坑！RawMessage 本质就是 type RawMessage []byte")
	fmt.Println("   值类型会被当成 []byte 做 base64 编码！")
	fmt.Println()
}

// ============================================
// 演示 4：跨语言调用的坑
// ============================================
type APIResponseBad struct {
	Token []byte `json:"token"` // ❌ 不要对外接口这么写！
}

type APIResponseGood struct {
	Token string `json:"token"` // ✅ 对外接口永远用 string
}

func demo4APIPitfall() {
	fmt.Println("=== 演示 4：对外接口的坑 ===")

	token := []byte("abc123xyz")

	// ❌ 错误写法
	bad := APIResponseBad{Token: token}
	bBad, _ := json.MarshalIndent(bad, "", "  ")
	fmt.Println("❌ 用 []byte 当接口字段：")
	fmt.Println(string(bBad))

	fmt.Println()

	// ✅ 正确写法
	good := APIResponseGood{Token: string(token)}
	bGood, _ := json.MarshalIndent(good, "", "  ")
	fmt.Println("✅ 用 string 当接口字段：")
	fmt.Println(string(bGood))

	fmt.Println()
	fmt.Println("💡 前端/Java/Python 收到 base64 后不会自动解码！")
	fmt.Println("   他们会以为 token 就是 'YWJjMTIzeHl6' 这个字符串本身")
	fmt.Println()
}

// ============================================
// 演示 5：什么时候可以用 []byte 当 JSON 字段
// ============================================
type ImageResponse struct {
	ImageData []byte `json:"image_data"` // ✅ 二进制数据，本来就应该 base64
	FileName  string `json:"file_name"`
}

func demo5BinaryData() {
	fmt.Println("=== 演示 5：什么时候可以用 []byte ===")

	img := ImageResponse{
		ImageData: []byte{0x89, 0x50, 0x4E, 0x47}, // PNG 文件头
		FileName:  "test.png",
	}
	b, _ := json.MarshalIndent(img, "", "  ")
	fmt.Println(string(b))

	fmt.Println()
	fmt.Println("💡 真正的二进制数据（图片、加密哈希、压缩包）就应该用 []byte")
	fmt.Println("   接收方知道这是二进制，会主动 base64 解码")
	fmt.Println()
}

// ============================================
// 演示 6：反序列化也会自动 base64 解码
// ============================================
func demo6Deserialize() {
	fmt.Println("=== 演示 6：反序列化也会自动解码 ===")

	jsonStr := `{"bts":"aGVsbG8="}`

	var d Demo1
	json.Unmarshal([]byte(jsonStr), &d)

	fmt.Printf("反序列化后 d.Bts = %q\n", d.Bts)
	fmt.Printf("转成 string 就是: %s\n", string(d.Bts))

	fmt.Println()
	fmt.Println("💡 Go 内部会自动 base64 解码，所以 Go 调用 Go 可能发现不了这个坑")
	fmt.Println("   只有跨语言调用的时候才会炸！")
	fmt.Println()
}

// ============================================
// main
// ============================================
func main() {
	demo1BasicDiff()
	demo2EmptyVsNil()
	demo3RawMessage()
	demo4APIPitfall()
	demo5BinaryData()
	demo6Deserialize()

	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println("✅ 所有坑演示完成！核心总结：")
	fmt.Println("1. []byte 会被 JSON 库自动 base64 编码")
	fmt.Println("2. 空 []byte → \"\"，nil []byte → null")
	fmt.Println("3. json.RawMessage 必须用指针，否则也会被 base64")
	fmt.Println("4. 对外 API 字段永远不要用 []byte，转成 string")
	fmt.Println("5. 只有真正的二进制数据才应该用 []byte")
}
