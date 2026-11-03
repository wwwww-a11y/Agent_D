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
	fmt.Printf("\n👤用户: %s\n", userInput)

	// ========== 闲聊短路 ==========
	if a.isChitchat(userInput) {
		fmt.Println("\n  [直接回复]")
		resp, err := callDeepSeek(&deepseek.ChatCompletionRequest{
			Model: config.DeepSeekModel,
			Messages: []deepseek.ChatCompletionMessage{
				{Role: deepseek.ChatMessageRoleSystem, Content: config.AgentSystemPrompt},
				{Role: deepseek.ChatMessageRoleUser, Content: userInput},
			},
		})
		if err != nil {
			fmt.Printf("☹️回复失败: %v\n", err)
			return
		}
		printReply(resp)
		a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleUser, Content: userInput})
		a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleAssistant, Content: resp})
		return
	}

	// ========== 0. 环境感知 ==========
	fmt.Println("\n😶[环境感知] 正在采集环境信息...")
	snapshot := a.sensor.Snapshot()
	envPrompt := snapshot.ToPrompt()
	fmt.Printf("🔉系统: %s/%s | 沙箱: %s\n", snapshot.OS, snapshot.Arch, snapshot.SandboxDir)
	fmt.Printf("👀沙箱内容预览:\n%s", indent(snapshot.SandboxTree, "   "))

	// ========== 1. 规划 ==========
	fmt.Println("\n📅[规划阶段] 结合环境拆解任务...")
	plan, err := a.planner.MakePlan(userInput, tools.ToolDescription(), envPrompt)
	if err != nil {
		fmt.Printf("❌规划失败: %v\n", err)
		return
	}
	a.printPlan(plan)

	// ========== 2. 人类确认 ==========
	if config.RequireConfirm {
		fmt.Print("\n🤔是否执行此计划?(y/n): ")
		var ans string
		fmt.Scanln(&ans)
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("🖐🏻用户取消执行")
			return
		}
	}

	// ========== 3. 执行循环 ==========
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

			fmt.Printf("\n👣[步骤 %d] %s\n", step.ID, step.Description)

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
			fmt.Printf("\n⭕已达最大重规划次数 (%d)，终止执行\n", config.MaxReplans)
			overallSuccess = false
			break
		}

		replanCount++
		fmt.Printf("\n⭕[动态重规划 #%d] 原计划受阻，正在重新规划...\n", replanCount)

		freshEnv := a.sensor.Snapshot().ToPrompt()
		reason := fmt.Sprintf("步骤执行失败，历史记录如下：\n%s", history.String())

		newPlan, err := a.planner.Replan(
			currentPlan.Goal, tools.ToolDescription(), freshEnv,
			history.String(), reason,
		)
		if err != nil {
			fmt.Printf("❌重规划失败: %v\n", err)
			overallSuccess = false
			break
		}

		fmt.Println("🆕新计划：")
		a.printPlan(newPlan)
		currentPlan = newPlan
	}

	// ========== 4. 全局复盘 ==========
	fmt.Println("\n🔵[全局复盘] 正在评估任务达成情况...")
	finalEnv := a.sensor.Snapshot().ToPrompt()
	retro, err := a.retrospector.Review(userInput, currentPlan.Goal, history.String(), finalEnv)
	if err != nil {
		fmt.Printf("❌复盘失败: %v\n", err)
	} else {
		fmt.Printf("📆目标达成: %v\n", retro.GoalAchieved)
		fmt.Printf("✅质量评分: %d/10\n", retro.QualityScore)
		fmt.Printf("🍀概述: %s\n", retro.Summary)
		if len(retro.RootCauses) > 0 {
			fmt.Printf("🪾根因:\n")
			for _, r := range retro.RootCauses {
				fmt.Printf(" - %s\n", r)
			}
		}
		if len(retro.Suggestions) > 0 {
			fmt.Printf("🙋🏻建议:\n")
			for _, s := range retro.Suggestions {
				fmt.Printf(" - %s\n", s)
			}
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
			fmt.Printf("❌参数解析失败，按空参数处理: %v\n", err)
			args = map[string]interface{}{}
		}
	}

	for attempt := 0; attempt <= config.MaxReflectRetries; attempt++ {
		fmt.Printf("⚒️执行工具: %s | 参数: %s\n", step.Tool, step.Args)

		result, err := tools.ExecuteTool(step.Tool, args)
		if err == nil {
			fmt.Printf("🟩成功: %s\n", result)
			history.WriteString(fmt.Sprintf("步骤%d %s: 成功 - %s\n", step.ID, step.Description, result))
			// 如果这步是纯文本回复（none），把 result 当回复输出
			if step.Tool == "none" || step.Tool == "" {
				printReply(step.Description)
			}
			return true
		}

		fmt.Printf("🔺失败: %v\n", err)
		history.WriteString(fmt.Sprintf("步骤%d %s: 失败 - %v\n", step.ID, step.Description, err))

		if attempt >= config.MaxReflectRetries {
			fmt.Println("❗已达最大重试次数")
			return false
		}

		freshEnv := a.sensor.Snapshot().ToPrompt()

		fmt.Printf("🥲[局部反思 %d] 正在分析失败原因...\n", attempt+1)
		ref, rerr := a.reflector.Reflect(
			step.Description, step.Tool, step.Args, err.Error(),
			history.String(), freshEnv,
		)
		if rerr != nil {
			fmt.Printf("❌反思失败: %v\n", rerr)
			return false
		}

		fmt.Printf("🔷原因: %s\n", ref.Reason)
		fmt.Printf("🔷策略: %s\n", ref.Strategy)

		if ref.GiveUp {
			fmt.Println("💠模型判断单步无法修复，交由重规划处理")
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
		fmt.Println("⛏️应用修正，准备重试...")
	}
	return false
}

func (a *Agent) summarize(userInput string, plan *planner.Plan, history, envPrompt string, success bool, retro *retrospector.Retrospection) {
	fmt.Println("\n🟫[总结阶段] 正在生成执行报告...")

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
		fmt.Printf("❗总结失败: %v\n", err)
		return
	}
	printReply(resp)

	a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleUser, Content: userInput})
	a.mem.Add(deepseek.ChatCompletionMessage{Role: deepseek.ChatMessageRoleAssistant, Content: resp})
}

func (a *Agent) printPlan(plan *planner.Plan) {
	fmt.Printf("  目标: %s\n", plan.Goal)
	for _, s := range plan.Steps {
		fmt.Printf("[%d] %s 🔨(tool: %s)\n", s.ID, s.Description, s.Tool)
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

// printReply 用分隔符高亮最终回复，避免被日志淹没
func printReply(reply string) {
	sep := strings.Repeat("=", 60)
	fmt.Printf("\n%s\n💬回复:\n%s\n%s\n", sep, reply, sep)
}
