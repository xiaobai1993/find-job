// OpenViking 最小 Go Demo
// 前提：OpenViking server 已在本地跑起来
//
//   pip install openviking
//   openviking-server init   # 配置 LLM key
//   openviking-server start  # 默认监听 :1934
//
// 运行：go run openviking_demo.go

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// -------- 客户端 --------

type OVClient struct {
	base   string
	apiKey string
	http   *http.Client
}

func NewOVClient(base, apiKey string) *OVClient {
	return &OVClient{
		base:   base,
		apiKey: apiKey,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *OVClient) call(method, path string, body any) (map[string]any, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, c.base+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		// 非 JSON 也打出来便于调试
		return map[string]any{"raw": string(raw)}, nil
	}
	return result, nil
}

// -------- API 操作 --------

// AddResource 把一个 URL / GitHub repo 作为"资源记忆"存入
func (c *OVClient) AddResource(url string) (map[string]any, error) {
	return c.call("POST", "/api/v1/resources", map[string]any{"url": url})
}

// Search 语义搜索所有记忆
func (c *OVClient) Search(query string) (map[string]any, error) {
	return c.call("POST", "/api/v1/search/find", map[string]any{"query": query})
}

// CreateSession 创建一个对话会话
func (c *OVClient) CreateSession() (string, error) {
	res, err := c.call("POST", "/api/v1/sessions", nil)
	if err != nil {
		return "", err
	}
	id, ok := res["id"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response: %v", res)
	}
	return id, nil
}

// AddMessage 向会话追加一条消息
func (c *OVClient) AddMessage(sessionID, role, content string) error {
	_, err := c.call("POST", "/api/v1/sessions/"+sessionID+"/messages", map[string]any{
		"role":    role,
		"content": content,
	})
	return err
}

// CommitSession 提交会话 → 触发 LLM 提取长期记忆
func (c *OVClient) CommitSession(sessionID string) (map[string]any, error) {
	return c.call("POST", "/api/v1/sessions/"+sessionID+"/commit", nil)
}

// -------- main --------

func printJSON(label string, v any) {
	b, _ := json.MarshalIndent(v, "  ", "  ")
	fmt.Printf("\n=== %s ===\n  %s\n", label, string(b))
}

func main() {
	// 替换为你本地 openviking-server 的 root key（init 时会打印）
	client := NewOVClient("http://localhost:1934", "your-root-key-here")

	// 1. 添加一个资源（OpenViking 会异步爬取并建索引）
	res, err := client.AddResource("https://github.com/volcengine/OpenViking")
	if err != nil {
		fmt.Printf("AddResource error: %v\n", err)
	} else {
		printJSON("AddResource", res)
	}

	// 2. 创建会话，模拟一段对话
	sessionID, err := client.CreateSession()
	if err != nil {
		fmt.Printf("CreateSession error: %v\n", err)
		return
	}
	fmt.Printf("\n=== Session created: %s ===\n", sessionID)

	// 3. 追加对话消息（这些消息会在 Commit 时被提取成长期记忆）
	msgs := []struct{ role, content string }{
		{"user", "我是 Go 后端开发，负责支付系统"},
		{"assistant", "了解，我会记住你的技术背景"},
		{"user", "我们团队内部服务全部用 gRPC，不用 HTTP"},
		{"assistant", "好的，gRPC + Protobuf，性能确实更好"},
		{"user", "我们用 Pulsar 做消息队列，不用 Kafka"},
		{"assistant", "明白了，Pulsar 的延迟消息和多租户特性很适合支付场景"},
	}
	for _, m := range msgs {
		if err := client.AddMessage(sessionID, m.role, m.content); err != nil {
			fmt.Printf("AddMessage error: %v\n", err)
		}
	}
	fmt.Println("Messages added.")

	// 4. Commit → LLM 从对话里提取关键记忆，写入长期存储
	commit, err := client.CommitSession(sessionID)
	if err != nil {
		fmt.Printf("CommitSession error: %v\n", err)
	} else {
		printJSON("CommitSession (memory extracted)", commit)
	}

	// 5. 下一轮对话前，先搜一下相关记忆注入给 LLM
	results, err := client.Search("这个用户用什么消息队列？")
	if err != nil {
		fmt.Printf("Search error: %v\n", err)
	} else {
		printJSON("Search results", results)
	}

	// 6. 再搜一个
	results2, err := client.Search("用户的编程语言偏好")
	if err != nil {
		fmt.Printf("Search error: %v\n", err)
	} else {
		printJSON("Search results 2", results2)
	}
}
