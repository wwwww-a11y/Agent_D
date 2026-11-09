package main

import (
	"bufio"
	"deepseek-agent/agent"
	"deepseek-agent/config"
	"deepseek-agent/memory"
	"deepseek-agent/tools"
	"deepseek-agent/ui"
	"fmt"
	"os"
	"strings"
)

func main() {
	config.Load()

	ui.Banner(config.DeepSeekModel, config.SandboxDir)
	ui.Note("正在连接 MCP 服务器…")
	if err := tools.InitMCP(config.MCPConfigPath); err != nil {
		ui.Warn(fmt.Sprintf("MCP 初始化失败: %v（将仅使用本地工具）", err))
	}
	defer tools.CloseMCP()

	sessionID := "default"
	mem, err := memory.NewMemory(sessionID, config.MemoryMaxMessages, config.MemoryDir)
	if err != nil {
		ui.Fail(fmt.Sprintf("初始化记忆失败: %v", err))
		os.Exit(1)
	}

	myAgent := agent.NewAgent(mem)
	reader := bufio.NewReader(os.Stdin)

	for {
		ui.UserPrompt()
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch {
		case input == "":
			continue
		case input == "exit" || input == "quit":
			ui.Note("再见。")
			return
		case input == "/reset":
			if err := mem.Reset(); err != nil {
				ui.Fail(fmt.Sprintf("重置失败: %v", err))
			} else {
				ui.Note("会话记忆已清空")
			}
			continue
		case input == "/stats":
			ui.Meta("会话统计", mem.Stats())
			continue
		case strings.HasPrefix(input, "/new "):
			newID := strings.TrimSpace(strings.TrimPrefix(input, "/new "))
			if newID == "" {
				ui.Warn("请提供会话 ID，例如 /new work")
				continue
			}
			newMem, err := memory.NewMemory(newID, config.MemoryMaxMessages, config.MemoryDir)
			if err != nil {
				ui.Fail(fmt.Sprintf("切换会话失败: %v", err))
				continue
			}
			mem = newMem
			myAgent = agent.NewAgent(mem)
			ui.Note("已切换到会话 [" + newID + "]")
			continue
		}

		myAgent.Run(input)
	}
}
