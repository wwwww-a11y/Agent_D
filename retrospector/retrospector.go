package retrospector

import (
	"context"
	"deepseek-agent/config"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cohesion-org/deepseek-go"
)

// Retrospection 全局复盘结果
type Retrospection struct {
	GoalAchieved bool     `json:"goal_achieved"`
	Summary      string   `json:"summary"`
	RootCauses   []string `json:"root_causes"`
	Suggestions  []string `json:"suggestions"`
	QualityScore int      `json:"quality_score"`
}

type Retrospector struct {
	client *deepseek.Client
}

func NewRetrospector() *Retrospector {
	return &Retrospector{client: deepseek.NewClient(config.DeepSeekAPIKey)}
}

// Review 对整轮执行做全局复盘
func (r *Retrospector) Review(userInput, goal, history, envPrompt string) (*Retrospection, error) {
	systemPrompt := `你是一个严格的任务复盘专家。请对整轮 Agent 执行做全局复盘。
输出严格 JSON：
{
  "goal_achieved": true/false,
  "summary": "完成情况概述",
  "root_causes": ["若未达成，列出根本原因"],
  "suggestions": ["下次改进建议"],
  "quality_score": 0-10
}
评判标准：
- goal_achieved：用户原始意图是否被真正满足（不是“执行了步骤”就算达成）
- root_causes：区分“工具报错”和“根本设计缺陷”
- 只输出 JSON，不要解释`

	userPrompt := fmt.Sprintf(`%s

用户原始任务：%s
计划目标：%s
执行记录：
%s`, envPrompt, userInput, goal, history)

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
		return nil, fmt.Errorf("复盘请求失败: %w", err)
	}

	content := stripFence(strings.TrimSpace(resp.Choices[0].Message.Content))

	var ret Retrospection
	if err := json.Unmarshal([]byte(content), &ret); err != nil {
		return nil, fmt.Errorf("复盘解析失败: %w\n原始: %s", err, content)
	}
	return &ret, nil
}

func stripFence(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
