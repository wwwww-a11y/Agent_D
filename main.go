package main

import (
	"bufio"
	"deepseek-agent/agent"
	"deepseek-agent/config"
	"deepseek-agent/memory"
	"deepseek-agent/tools"
	"fmt"
	"os"
	"strings"
)

func main() {
	config.Load()

	fmt.Println("🛜正在连接 MCP 服务器...")
	if err := tools.InitMCP(config.MCPConfigPath); err != nil {
		fmt.Printf("⚠️MCP 初始化失败: %v(将仅使用本地工具)\n", err)
	}
	defer tools.CloseMCP()

	fmt.Println("=== DeepSeek Go Agent(感知 + 规划 + 反思 + 重规划 + 复盘 + 记忆 + MCP)===")
	fmt.Printf("模型: %s | 沙箱: %s\n", config.DeepSeekModel, config.SandboxDir)
	fmt.Println("命令: /reset | /stats | /new <会话ID> | exit")

	sessionID := "default"
	mem, err := memory.NewMemory(sessionID, config.MemoryMaxMessages, config.MemoryDir)
	if err != nil {
		fmt.Printf("❌初始化记忆失败: %v\n", err)
		os.Exit(1)
	}

	myAgent := agent.NewAgent(mem)
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\n👱你: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch {
		case input == "":
			continue
		case input == "exit" || input == "quit":
			fmt.Println("🖐️再见！")
			return
		case input == "/reset":
			if err := mem.Reset(); err != nil {
				fmt.Printf("❌重置失败: %v\n", err)
			} else {
				fmt.Println("🧹会话记忆已清空")
			}
			continue
		case input == "/stats":
			fmt.Printf("🔢%s\n", mem.Stats())
			continue
		case strings.HasPrefix(input, "/new "):
			newID := strings.TrimSpace(strings.TrimPrefix(input, "/new "))
			if newID == "" {
				fmt.Println("⚠️请提供会话 ID,例如 /new work")
				continue
			}
			newMem, err := memory.NewMemory(newID, config.MemoryMaxMessages, config.MemoryDir)
			if err != nil {
				fmt.Printf("❌切换会话失败: %v\n", err)
				continue
			}
			mem = newMem
			myAgent = agent.NewAgent(mem)
			fmt.Printf("🔗已切换到会话 [%s]\n", newID)
			continue
		}

		myAgent.Run(input)
	}
}
