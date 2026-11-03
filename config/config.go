package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

var (
	DeepSeekAPIKey    string
	DeepSeekModel     string
	AgentMaxLoops     int
	AgentSystemPrompt string
	MemoryDir         string
	MemoryMaxMessages int
	SandboxDir        string
	MaxReflectRetries int
	MaxReplans        int
	RequireConfirm    bool
	MCPConfigPath     string
)

// Load 加载 .env 文件并初始化配置
func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("   未找到 .env 文件，将使用系统环境变量")
	}

	DeepSeekAPIKey = getEnv("DEEPSEEK_API_KEY", "")
	DeepSeekModel = getEnv("DEEPSEEK_MODEL", "deepseek-chat")
	AgentSystemPrompt = getEnv("AGENT_SYSTEM_PROMPT", "你是一个乐于助人的助手。")
	MemoryDir = getEnv("MEMORY_DIR", "./data/memory")
	SandboxDir = getEnv("SANDBOX_DIR", "./data/sandbox")
	MCPConfigPath = getEnv("MCP_CONFIG_PATH", "./mcp_servers.json")

	AgentMaxLoops = getEnvInt("AGENT_MAX_LOOPS", 5)
	MemoryMaxMessages = getEnvInt("MEMORY_MAX_MESSAGES", 40)
	MaxReflectRetries = getEnvInt("MAX_REFLECT_RETRIES", 2)
	MaxReplans = getEnvInt("MAX_REPLANS", 2)
	RequireConfirm = getEnv("REQUIRE_CONFIRM", "true") == "true"

	if DeepSeekAPIKey == "" {
		log.Fatal("❌缺少 DEEPSEEK_API_KEY，请在 .env 文件中配置")
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("⚠️%s 解析失败，用默认 %d\n", key, def)
		return def
	}
	return v
}
