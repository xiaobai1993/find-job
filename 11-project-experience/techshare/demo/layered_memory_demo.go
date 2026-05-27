// TencentDB-Agent-Memory 等效 Go 实现
//
// TencentDB-Agent-Memory 本身是 Node.js 插件，没有 Go SDK 和 REST API。
// 这个 demo 用 Go 实现了它相同的核心架构：L0→L3 四层语义记忆金字塔。
//
// 依赖：go get github.com/mattn/go-sqlite3
// 运行：go run layered_memory_demo.go
//
// 注意：真实版本 L1→L3 的提取需要调用 LLM，这里用简单字符串模拟抽象过程。

package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// -------- 四层记忆模型 --------
//
//  L0: Conversation  原始对话，每一轮问答
//  L1: Extraction    从对话中提取的关键事实（每 N 轮触发一次）
//  L2: Scenario      多条 L1 事实归纳出的场景模式
//  L3: Persona       跨会话的用户画像（每 M 条 L1 触发一次）

type Memory struct {
	db *sql.DB
}

func NewMemory(path string) (*Memory, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	m := &Memory{db: db}
	return m, m.init()
}

func (m *Memory) init() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS l0_conversation (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			session TEXT    NOT NULL,
			role    TEXT    NOT NULL,  -- "user" | "assistant"
			content TEXT    NOT NULL,
			ts      DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS l1_extraction (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			session TEXT    NOT NULL,
			fact    TEXT    NOT NULL,  -- 提取出的事实
			ts      DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS l2_scenario (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern TEXT    NOT NULL,  -- 归纳的模式
			ts      DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS l3_persona (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			summary TEXT    NOT NULL,  -- 用户画像
			ts      DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

// -------- L0: 保存原始对话 --------

func (m *Memory) SaveMessage(session, role, content string) error {
	_, err := m.db.Exec(
		`INSERT INTO l0_conversation(session, role, content) VALUES(?,?,?)`,
		session, role, content,
	)
	return err
}

// -------- L1: 从对话提取关键事实 --------
// 真实实现里这里调用 LLM；这里用关键词匹配模拟

func (m *Memory) ExtractFacts(session string) ([]string, error) {
	rows, err := m.db.Query(
		`SELECT role, content FROM l0_conversation WHERE session=? ORDER BY ts`,
		session,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var role, content string
		rows.Scan(&role, &content)

		// 模拟 LLM 提取：遇到"我是/我用/我们用"这类陈述句，视为事实
		if role == "user" && containsAny(content, "我是", "我用", "我们用", "我负责", "我喜欢") {
			fact := "用户说：" + content
			facts = append(facts, fact)
			m.db.Exec(
				`INSERT INTO l1_extraction(session, fact) VALUES(?,?)`,
				session, fact,
			)
		}
	}
	return facts, nil
}

// -------- L2: 归纳 L1 事实为场景 --------

func (m *Memory) SummarizeScenarios() error {
	rows, err := m.db.Query(`SELECT fact FROM l1_extraction ORDER BY ts DESC LIMIT 20`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var f string
		rows.Scan(&f)
		facts = append(facts, f)
	}
	if len(facts) == 0 {
		return nil
	}

	// 模拟 LLM 归纳：把所有事实合并成一段场景描述
	pattern := fmt.Sprintf(
		"[%s] 技术场景归纳：%s",
		time.Now().Format("2006-01-02"),
		strings.Join(facts, " | "),
	)
	_, err = m.db.Exec(`INSERT INTO l2_scenario(pattern) VALUES(?)`, pattern)
	return err
}

// -------- L3: 生成用户画像 --------

func (m *Memory) UpdatePersona() error {
	rows, err := m.db.Query(`SELECT pattern FROM l2_scenario ORDER BY ts DESC LIMIT 5`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var scenarios []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		scenarios = append(scenarios, s)
	}
	if len(scenarios) == 0 {
		return nil
	}

	// 模拟 LLM 生成画像
	persona := fmt.Sprintf(
		"用户画像（%s）：Go 后端开发者，专注支付系统，使用 gRPC + Pulsar 技术栈。%s",
		time.Now().Format("2006-01-02"),
		strings.Join(scenarios[:min(2, len(scenarios))], "；"),
	)
	_, err = m.db.Exec(`INSERT INTO l3_persona(summary) VALUES(?)`, persona)
	return err
}

// -------- 召回：按关键词搜索（真实版用向量相似度） --------

func (m *Memory) Recall(query string) {
	fmt.Printf("\n--- 召回查询: %q ---\n", query)

	// L3 先看画像
	var persona string
	m.db.QueryRow(`SELECT summary FROM l3_persona ORDER BY ts DESC LIMIT 1`).Scan(&persona)
	if persona != "" {
		fmt.Printf("[L3 Persona] %s\n", persona)
	}

	// L2 看场景
	rows, _ := m.db.Query(`SELECT pattern FROM l2_scenario ORDER BY ts DESC LIMIT 2`)
	defer rows.Close()
	for rows.Next() {
		var p string
		rows.Scan(&p)
		// 简单关键词匹配，真实版用 sqlite-vec 做向量相似度
		if containsAny(p, strings.Fields(query)...) {
			fmt.Printf("[L2 Scenario] %s\n", p)
		}
	}

	// L1 看具体事实
	rows2, _ := m.db.Query(`SELECT fact FROM l1_extraction ORDER BY ts DESC LIMIT 10`)
	defer rows2.Close()
	for rows2.Next() {
		var f string
		rows2.Scan(&f)
		if containsAny(f, strings.Fields(query)...) {
			fmt.Printf("[L1 Fact] %s\n", f)
		}
	}
}

// -------- 工具函数 --------

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// -------- main --------

func main() {
	mem, err := NewMemory(":memory:") // 换成 "./memory.db" 可持久化
	if err != nil {
		panic(err)
	}

	session := "session-001"

	// 模拟一段对话
	conversations := []struct{ role, content string }{
		{"user", "我是 Go 后端开发，负责公司的支付系统"},
		{"assistant", "了解，有什么可以帮你的？"},
		{"user", "我们用 gRPC 做内部服务通信"},
		{"assistant", "gRPC 很适合微服务，延迟低、强类型"},
		{"user", "我们用 Pulsar 做消息队列，不用 Kafka"},
		{"assistant", "Pulsar 的多租户和延迟消息对支付场景很有用"},
		{"user", "我喜欢写测试，团队要求覆盖率 80% 以上"},
		{"assistant", "好习惯，测试覆盖率对支付这种核心系统很重要"},
	}

	fmt.Println("=== L0: 保存原始对话 ===")
	for _, c := range conversations {
		mem.SaveMessage(session, c.role, c.content)
		fmt.Printf("[%s] %s\n", c.role, c.content)
	}

	fmt.Println("\n=== L1: 提取关键事实 ===")
	facts, _ := mem.ExtractFacts(session)
	for _, f := range facts {
		fmt.Println(f)
	}

	fmt.Println("\n=== L2: 归纳场景 ===")
	mem.SummarizeScenarios()
	fmt.Println("场景归纳完成")

	fmt.Println("\n=== L3: 更新用户画像 ===")
	mem.UpdatePersona()
	fmt.Println("用户画像更新完成")

	// 模拟下一轮对话开始，先召回相关记忆
	mem.Recall("消息队列")
	mem.Recall("编程语言")
	mem.Recall("测试")
}
