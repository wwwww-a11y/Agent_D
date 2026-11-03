package reflector

import (
	"context"
	"deepseek-agent/config"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cohesion-org/deepseek-go"
)

type Reflection struct {
	Reason   string `json:"reason"`
	Strategy string `json:"strategy"`
	Tool     string `json:"tool"`
	Args     string `json:"args"`
	GiveUp   bool   `json:"give_up"`
}

type Reflector struct {
	client *deepseek.Client
}

func NewReflector() *Reflector {
	return &Reflector{client: deepseek.NewClient(config.DeepSeekAPIKey)}
}

// Reflect 带环境上下文反思
func (r *Reflector) Reflect(stepDesc, tool, args, errMsg, history, envPrompt string) (*Reflection, error) {
	systemPrompt := `你是一个错误分析器。工具执行失败了，请结合【环境信息】分析原因并给出修正方案。
输出严格 JSON：
{
  "reason": "失败原因分析",
  "strategy": "修正策略描述",
  "tool": "建议的工具名",
  "args": "建议的新参数JSON字符串",
  "give_up": false
}
如果判断无法修复（权限、目标不存在等），give_up 设为 true。
只输出 JSON，不要解释。`

	userPrompt := fmt.Sprintf(`%s

失败的步骤：%s
使用工具：%s
传入参数：%s
错误信息：%s
历史执行记录：
%s`, envPrompt, stepDesc, tool, args, errMsg, history)

	req := &deepseek.ChatCompletionRequest{
		Model: config.DeepSeekModel,
		Messages: []deepseek.ChatCompletionMessage{
			{Role: deepseek.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: deepseek.ChatMessageRoleUser, Content: userPrompt},
		},
		ResponseFormat: &deepseek.ResponseFormat{Type: "json_object"},
	}

	ctx := context.Background()
	resp, err := r.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("反思请求失败: %w", err)
	}

	content := stripFence(strings.TrimSpace(resp.Choices[0].Message.Content))

	var ref Reflection
	if err := json.Unmarshal([]byte(content), &ref); err != nil {
		return nil, fmt.Errorf("反思解析失败: %w\n原始: %s", err, content)
	}
	return &ref, nil
}

func stripFence(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
