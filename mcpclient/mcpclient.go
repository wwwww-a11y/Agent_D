package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/cohesion-org/deepseek-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	ServerName  string
}

type MCPClient struct {
	mu       sync.Mutex
	sessions map[string]*mcp.ClientSession
	tools    []MCPTool
}

func NewMCPClient(configPath string) (*MCPClient, error) {
	client := &MCPClient{
		sessions: make(map[string]*mcp.ClientSession),
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return client, fmt.Errorf("读取 MCP 配置失败: %w", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return client, fmt.Errorf("解析 MCP 配置失败: %w", err)
	}

	if len(cfg.MCPServers) == 0 {
		fmt.Println("📭 未配置任何 MCP 服务器，跳过 MCP 工具加载")
		return client, nil
	}

	for name, sc := range cfg.MCPServers {
		if err := client.connect(name, sc); err != nil {
			fmt.Printf("⚠️  MCP 服务器 [%s] 连接失败: %v\n", name, err)
			continue
		}
	}

	fmt.Printf("🔌 MCP 已连接 %d 个服务器，发现 %d 个工具\n",
		len(client.sessions), len(client.tools))
	return client, nil
}

func (c *MCPClient) connect(name string, sc MCPServerConfig) error {
	ctx := context.Background()

	cmd := exec.Command(sc.Command, sc.Args...)
	cmd.Env = os.Environ()
	for k, v := range sc.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "deepseek-agent",
		Version: "1.0.0",
	}, nil)

	transport := &mcp.CommandTransport{Command: cmd}
	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	c.sessions[name] = session
	fmt.Printf("✅ MCP 服务器 [%s] 已连接\n", name)

	toolsResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		fmt.Printf("⚠️  服务器 [%s] 获取工具失败: %v\n", name, err)
		return nil
	}

	for _, t := range toolsResult.Tools {
		c.tools = append(c.tools, MCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schemaToMap(t.InputSchema),
			ServerName:  name,
		})
		fmt.Printf("   🔧 发现工具: %s - %s\n", t.Name, t.Description)
	}

	return nil
}

func schemaToMap(schema any) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	if m, ok := schema.(map[string]interface{}); ok {
		return m
	}
	data, _ := json.Marshal(schema)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	return m
}

func (c *MCPClient) GetDeepSeekTools() []deepseek.Tool {
	c.mu.Lock()
	defer c.mu.Unlock()

	var result []deepseek.Tool
	for _, t := range c.tools {
		props := t.InputSchema["properties"]
		required, _ := t.InputSchema["required"].([]interface{})

		var reqStrs []string
		for _, r := range required {
			if s, ok := r.(string); ok {
				reqStrs = append(reqStrs, s)
			}
		}

		result = append(result, deepseek.Tool{
			Type: "function",
			Function: deepseek.Function{
				Name:        t.Name,
				Description: t.Description,
				Parameters: &deepseek.FunctionParameters{
					Type:       "object",
					Properties: props.(map[string]interface{}),
					Required:   reqStrs,
				},
			},
		})
	}
	return result
}

func (c *MCPClient) IsMCPTool(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var serverName string
	for _, t := range c.tools {
		if t.Name == name {
			serverName = t.ServerName
			break
		}
	}
	if serverName == "" {
		return "", fmt.Errorf("MCP 工具不存在: %s", name)
	}

	session, ok := c.sessions[serverName]
	if !ok {
		return "", fmt.Errorf("MCP 服务器未连接: %s", serverName)
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("MCP 调用失败: %w", err)
	}
	if res.IsError {
		var errMsg string
		for _, content := range res.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				errMsg += text.Text
			}
		}
		return "", fmt.Errorf("MCP 工具返回错误: %s", errMsg)
	}

	var result string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			result += text.Text
		}
	}
	return result, nil
}

func (c *MCPClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for name, session := range c.sessions {
		session.Close()
		fmt.Printf("🔌 MCP 服务器 [%s] 已断开\n", name)
	}
}
