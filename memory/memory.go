package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cohesion-org/deepseek-go"
)

// Memory 封装会话记忆
type Memory struct {
	mu          sync.Mutex
	SessionID   string                           `json:"session_id"`
	Messages    []deepseek.ChatCompletionMessage `json:"messages"`
	MaxMessages int                              `json:"max_messages"`
	CreatedAt   time.Time                        `json:"created_at"`
	UpdatedAt   time.Time                        `json:"updated_at"`
	storageDir  string
}

// NewMemory 创建或加载会话记忆
func NewMemory(sessionID string, maxMessages int, storageDir string) (*Memory, error) {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("创建记忆目录失败: %w", err)
	}

	m := &Memory{
		SessionID:   sessionID,
		MaxMessages: maxMessages,
		storageDir:  storageDir,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := m.load(); err == nil {
		fmt.Printf("🤔已加载历史会话 [%s]，共 %d 条消息\n", m.SessionID, len(m.Messages))
	} else {
		fmt.Printf("🆕创建新会话 [%s]\n", m.SessionID)
	}

	return m, nil
}

func (m *Memory) filePath() string {
	return filepath.Join(m.storageDir, m.SessionID+".json")
}

func (m *Memory) load() error {
	data, err := os.ReadFile(m.filePath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, m)
}

func (m *Memory) save() error {
	m.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath(), data, 0644)
}

// Add 添加消息并触发压缩与持久化
func (m *Memory) Add(msg deepseek.ChatCompletionMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Messages = append(m.Messages, msg)
	m.compress()

	if err := m.save(); err != nil {
		fmt.Printf("☹️记忆持久化失败: %v\n", err)
	}
}

// compress 保留 system 消息 + 最近 N 条
func (m *Memory) compress() {
	if len(m.Messages) <= m.MaxMessages {
		return
	}

	var systemMsgs []deepseek.ChatCompletionMessage
	var others []deepseek.ChatCompletionMessage
	for _, msg := range m.Messages {
		if msg.Role == deepseek.ChatMessageRoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			others = append(others, msg)
		}
	}

	keep := m.MaxMessages - len(systemMsgs)
	if keep < 1 {
		keep = 1
	}
	if len(others) > keep {
		dropped := len(others) - keep
		others = others[dropped:]
		fmt.Printf("🟣记忆压缩：丢弃 %d 条最早消息，保留最近 %d 条\n", dropped, keep)
	}

	m.Messages = append(systemMsgs, others...)
}

// GetMessages 返回消息副本
func (m *Memory) GetMessages() []deepseek.ChatCompletionMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := make([]deepseek.ChatCompletionMessage, len(m.Messages))
	copy(cp, m.Messages)
	return cp
}

// Reset 清空当前会话记忆
func (m *Memory) Reset() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Messages = nil
	m.UpdatedAt = time.Now()
	return m.save()
}

// Stats 返回记忆统计信息
func (m *Memory) Stats() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return fmt.Sprintf("会话: %s | 消息数: %d/%d | 创建: %s | 更新: %s",
		m.SessionID,
		len(m.Messages),
		m.MaxMessages,
		m.CreatedAt.Format("01-02 15:04"),
		m.UpdatedAt.Format("01-02 15:04"),
	)
}
