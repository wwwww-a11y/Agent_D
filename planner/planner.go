package planner

import (
	"context"
	"deepseek-agent/config"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cohesion-org/deepseek-go"
)

type PlanItem struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Tool        string `json:"tool"`
	Args        string `json:"args"`
	Status      string `json:"status"`
}

type Plan struct {
	Goal  string     `json:"goal"`
	Steps []PlanItem `json:"steps"`
}

type Planner struct {
	client *deepseek.Client
}

func NewPlanner() *Planner {
	return &Planner{client: deepseek.NewClient(config.DeepSeekAPIKey)}
}

// MakePlan 初始规划
func (p *Planner) MakePlan(userInput, toolDesc, envPrompt string) (*Plan, error) {
	systemPrompt := `你是一个任务规划器。请结合【环境信息】把用户任务拆解为可执行的步骤。
输出严格 JSON:
{
  "goal": "任务目标",
  "steps": [
    {"id": 1, "description": "步骤描述", "tool": "工具名", "args": "工具参数的JSON字符串"}
  ]
}
规则：
- 优先利用环境中已有的文件/目录，避免重复创建
- 文件路径一律使用相对沙箱的路径
- 不需要工具的步骤 tool 填 "none",args 填 "{}"
- 只输出 JSON,不要解释、不要 markdown 代码块`

	userPrompt := fmt.Sprintf("%s\n\n可用工具:\n%s\n\n用户任务:%s", envPrompt, toolDesc, userInput)

	return p.call(userPrompt, systemPrompt)
}

// Replan 动态重规划
func (p *Planner) Replan(originalGoal, toolDesc, envPrompt, history, reason string) (*Plan, error) {
	systemPrompt := `你是一个动态重规划器。原计划执行遇到阻塞，需要你根据最新情况重新规划。
输出严格 JSON:
{
  "goal": "调整后的目标",
  "steps": [
    {"id": 1, "description": "步骤描述", "tool": "工具名", "args": "工具参数的JSON字符串"}
  ]
}
规则：
- 已完成的部分不要重复执行，直接跳过
- 失败步骤要吸取教训，换一种可行策略
- 结合最新【环境信息】判断哪些文件/目录已存在
- 如果原目标已无法达成，可以调整目标为“最小可行目标”
- 只输出 JSON,不要解释`

	userPrompt := fmt.Sprintf(`%s

🔧可用工具：
%s

原始目标：%s
触发重规划的原因：%s
历史执行记录：
%s

请给出新的执行计划。`, envPrompt, toolDesc, originalGoal, reason, history)

	return p.call(userPrompt, systemPrompt)
}

func (p *Planner) call(userPrompt, systemPrompt string) (*Plan, error) {
	req := &deepseek.ChatCompletionRequest{
		Model: config.DeepSeekModel,
		Messages: []deepseek.ChatCompletionMessage{
			{Role: deepseek.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: deepseek.ChatMessageRoleUser, Content: userPrompt},
		},
		ResponseFormat: &deepseek.ResponseFormat{Type: "json_object"},
	}

	ctx := context.Background()
	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("规划请求失败: %w", err)
	}

	content := stripCodeFence(strings.TrimSpace(resp.Choices[0].Message.Content))

	var plan Plan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("计划解析失败: %w\n原始输出: %s", err, content)
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("计划为空")
	}
	for i := range plan.Steps {
		plan.Steps[i].Status = "pending"
	}
	return &plan, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
