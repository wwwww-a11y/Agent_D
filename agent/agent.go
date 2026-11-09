package agent

import (
	"context"
	"deepseek-agent/config"
	"deepseek-agent/memory"
	"deepseek-agent/planner"
	"deepseek-agent/reflector"
	"deepseek-agent/retrospector"
	"deepseek-agent/sensor"
	"deepseek-agent/tools"
	"deepseek-agent/ui"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cohesion-org/deepseek-go"
)

type Agent struct {
	mem          *memory.Memory
	planner      *planner.Planner
	reflector    *reflector.Reflector
	retrospector *retrospector.Retrospector
	sensor       *sensor.Sensor
}

func NewAgent(mem *memory.Memory) *Agent {
	return &Agent{
		mem:          mem,
		planner:      planner.NewPlanner(),
		reflector:    reflector.NewReflector(),
		retrospector: retrospector.NewRetrospector(),
		sensor:       sensor.NewSensor(tools.ToolDescription),
	}
}

// Run 主流程：闲聊短路 → 感知 → 规划 → 执行 → 复盘 → 总结
func (a *Agent) Run(userInput string) {
	ui.Task(userInput)

	// ========== 闲聊短路 ==========
	if a.isChitchat(userInput) {
		ui.Stage(1, "快速回复")
		ui.Note("闲聊输入，跳过规划与执行，直接回复")
		resp, err := callDeepSeek(&deepseek.ChatCompletionRequest{
			Model: config.DeepSeekModel,
			Messages: []deepseek.ChatCompletionMessage{
				{Role: deepseek.ChatMessageRoleSystem, Content: config.AgentSystemPrompt},
				{Role: deepseek.ChatMessageRoleUser, Content: userInput},
			},
		})
		if err != nil {
			ui.Fail(fmt.Sprintf("回复失败: %v", err))
			return
		}
		printReply(resp)
		a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleUser, Content: userInput})
		a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleAssistant, Content: resp})
		return
	}

	// ========== 0. 环境感知 ==========
	ui.Stage(1, "环境感知")
	ui.Note("采集系统信息、沙箱目录与可用工具…")
	snapshot := a.sensor.Snapshot()
	envPrompt := snapshot.ToPrompt()
	ui.Meta("系统", fmt.Sprintf("%s/%s  ·  %s", snapshot.OS, snapshot.Arch, snapshot.GoVersion))
	ui.Meta("沙箱", snapshot.SandboxDir)
	ui.Block("沙箱内容", snapshot.SandboxTree)

	// ========== 1. 规划 ==========
	ui.Stage(2, "任务规划")
	ui.Note("结合环境信息，把任务拆成可执行步骤")
	plan, err := a.planner.MakePlan(userInput, tools.ToolDescription(), envPrompt)
	if err != nil {
		ui.Fail(fmt.Sprintf("规划失败: %v", err))
		return
	}
	a.printPlan(plan)

	// ========== 2. 人类确认 ==========
	if config.RequireConfirm {
		ui.Prompt("是否执行此计划? (y/n): ")
		var ans string
		fmt.Scanln(&ans)
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			ui.Warn("用户取消执行")
			return
		}
	}

	// ========== 3. 执行循环 ==========
	ui.Stage(3, "计划执行")
	var history strings.Builder
	currentPlan := plan
	replanCount := 0
	overallSuccess := true

	for {
		stepFailed := false

		for i := range currentPlan.Steps {
			step := &currentPlan.Steps[i]
			if step.Status == "done" || step.Status == "skipped" {
				continue
			}

			ui.Step(step.ID, step.Description, step.Tool)

			ok := a.executeStepWithRetry(step, &history)
			if !ok {
				stepFailed = true
				step.Status = "failed"
				break
			}
			step.Status = "done"
		}

		if !stepFailed {
			break
		}

		if replanCount >= config.MaxReplans {
			ui.Fail(fmt.Sprintf("已达最大重规划次数 (%d)，终止执行", config.MaxReplans))
			overallSuccess = false
			break
		}

		replanCount++
		ui.Replan(replanCount)

		freshEnv := a.sensor.Snapshot().ToPrompt()
		reason := fmt.Sprintf("步骤执行失败，历史记录如下：\n%s", history.String())

		newPlan, err := a.planner.Replan(
			currentPlan.Goal, tools.ToolDescription(), freshEnv,
			history.String(), reason,
		)
		if err != nil {
			ui.Fail(fmt.Sprintf("重规划失败: %v", err))
			overallSuccess = false
			break
		}

		ui.Note("采纳新计划：")
		a.printPlan(newPlan)
		currentPlan = newPlan
	}

	// ========== 4. 全局复盘 ==========
	ui.Stage(4, "全局复盘")
	ui.Note("判断目标是否真正达成，给出评分与根因")
	finalEnv := a.sensor.Snapshot().ToPrompt()
	retro, err := a.retrospector.Review(userInput, currentPlan.Goal, history.String(), finalEnv)
	if err != nil {
		ui.Fail(fmt.Sprintf("复盘失败: %v", err))
	} else {
		ui.Score(retro.GoalAchieved, retro.QualityScore)
		ui.Meta("概述", retro.Summary)
		if len(retro.RootCauses) > 0 {
			ui.Block("根因", strings.Join(retro.RootCauses, "\n"))
		}
		if len(retro.Suggestions) > 0 {
			ui.Block("建议", strings.Join(retro.Suggestions, "\n"))
		}
	}

	// ========== 5. 总结 ==========
	a.summarize(userInput, currentPlan, history.String(), envPrompt, overallSuccess, retro)
}

// isChitchat 简单判断：无工具需求、纯闲聊
func (a *Agent) isChitchat(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return true
	}
	// 纯问候/闲聊关键词
	chitchatPatterns := []string{
		"你好", "您好", "hi", "hello", "嗨", "在吗", "早上好", "晚上好",
		"谢谢", "感谢", "再见", "拜拜", "你是谁", "你能做什么",
	}
	lower := strings.ToLower(input)
	// 如果输入很短且包含闲聊关键词，直接走闲聊
	if len([]rune(input)) <= 10 {
		for _, p := range chitchatPatterns {
			if strings.Contains(lower, p) {
				return true
			}
		}
	}
	return false
}

func (a *Agent) executeStepWithRetry(step *planner.PlanItem, history *strings.Builder) bool {
	var args map[string]interface{}
	if step.Args != "" {
		if err := json.Unmarshal([]byte(step.Args), &args); err != nil {
			ui.Warn(fmt.Sprintf("参数解析失败，按空参数处理: %v", err))
			args = map[string]interface{}{}
		}
	}

	for attempt := 0; attempt <= config.MaxReflectRetries; attempt++ {
		ui.Tool(step.Tool, step.Args)

		result, err := tools.ExecuteTool(step.Tool, args)
		if err == nil {
			ui.OK(result)
			history.WriteString(fmt.Sprintf("步骤%d %s: 成功 - %s\n", step.ID, step.Description, result))
			// 无工具步骤（none）只推进，描述已在步骤标题中显示，不重复打印
			return true
		}

		ui.Fail(err.Error())
		history.WriteString(fmt.Sprintf("步骤%d %s: 失败 - %v\n", step.ID, step.Description, err))

		if attempt >= config.MaxReflectRetries {
			ui.Fail("已达最大重试次数，交给重规划处理")
			return false
		}

		freshEnv := a.sensor.Snapshot().ToPrompt()

		ui.Reflect(attempt + 1)
		ref, rerr := a.reflector.Reflect(
			step.Description, step.Tool, step.Args, err.Error(),
			history.String(), freshEnv,
		)
		if rerr != nil {
			ui.Fail(fmt.Sprintf("反思失败: %v", rerr))
			return false
		}

		ui.Meta("原因", ref.Reason)
		ui.Meta("策略", ref.Strategy)

		if ref.GiveUp {
			ui.Warn("模型判断单步无法修复，交由重规划处理")
			return false
		}

		if ref.Tool != "" {
			step.Tool = ref.Tool
		}
		if ref.Args != "" {
			step.Args = ref.Args
			args = map[string]interface{}{}
			_ = json.Unmarshal([]byte(ref.Args), &args)
		}
		ui.Note("已应用修正，准备重试…")
	}
	return false
}

func (a *Agent) summarize(userInput string, plan *planner.Plan, history, envPrompt string, success bool, retro *retrospector.Retrospection) {
	ui.Stage(5, "生成最终回复")
	ui.Note("把执行过程与复盘结论写成中文报告")

	status := "成功"
	if !success {
		status = "未完全成功"
	}

	var retroText string
	if retro != nil {
		retroText = fmt.Sprintf("复盘结论：\n- 目标达成: %v\n- 质量评分: %d/10\n- 概述: %s\n- 根因: %v\n- 建议: %v",
			retro.GoalAchieved, retro.QualityScore, retro.Summary, retro.RootCauses, retro.Suggestions)
	}

	prompt := fmt.Sprintf("%s\n\n用户任务:%s\n计划目标:%s\n执行状态:%s\n执行记录:\n%s\n\n%s\n\n请用中文简洁总结：完成了什么、遇到什么问题、结果如何、下一步建议。",
		envPrompt, userInput, plan.Goal, status, history, retroText)

	req := &deepseek.ChatCompletionRequest{
		Model: config.DeepSeekModel,
		Messages: []deepseek.ChatCompletionMessage{
			{Role: deepseek.ChatMessageRoleUser, Content: prompt},
		},
	}

	resp, err := callDeepSeek(req)
	if err != nil {
		ui.Fail(fmt.Sprintf("总结失败: %v", err))
		return
	}
	printReply(resp)

	a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleUser, Content: userInput})
	a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleAssistant, Content: resp})
}

func (a *Agent) printPlan(plan *planner.Plan) {
	ui.Meta("目标", plan.Goal)
	for _, s := range plan.Steps {
		ui.PlanRow(s.ID, s.Description, s.Tool)
	}
}

// callDeepSeek 统一调用入口（带重试，规避偶发网络错误）
func callDeepSeek(req *deepseek.ChatCompletionRequest) (string, error) {
	client := deepseek.NewClient(config.DeepSeekAPIKey)
	resp, err := client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n") + "\n"
}

// printReply 用边框高亮最终回复，避免被日志淹没
func printReply(reply string) {
	ui.Reply("💬 最终回复", reply)
}
