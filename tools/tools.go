package tools

import (
	"context"
	"deepseek-agent/config"
	"deepseek-agent/mcpclient"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cohesion-org/deepseek-go"
)

var mcpClient *mcpclient.MCPClient

var localTools = []deepseek.Tool{
	{
		Type: "function",
		Function: deepseek.Function{
			Name:        "get_current_time",
			Description: "获取当前的系统日期和时间",
			Parameters: &deepseek.FunctionParameters{
				Type:       "object",
				Properties: map[string]interface{}{},
			},
		},
	},
	{
		Type: "function",
		Function: deepseek.Function{
			Name:        "write_file",
			Description: "在沙箱目录内创建文件并写入内容",
			Parameters: &deepseek.FunctionParameters{
				Type: "object",
				Properties: map[string]interface{}{
					"filename": map[string]interface{}{"type": "string", "description": "文件名（相对沙箱），如 test.txt"},
					"content":  map[string]interface{}{"type": "string", "description": "文件内容"},
				},
				Required: []string{"filename", "content"},
			},
		},
	},
	{
		Type: "function",
		Function: deepseek.Function{
			Name:        "list_files",
			Description: "列出沙箱目录下的文件",
			Parameters: &deepseek.FunctionParameters{
				Type:       "object",
				Properties: map[string]interface{}{},
			},
		},
	},
}

func InitMCP(configPath string) error {
	c, err := mcpclient.NewMCPClient(configPath)
	if err != nil {
		return err
	}
	mcpClient = c
	return nil
}

func CloseMCP() {
	if mcpClient != nil {
		mcpClient.Close()
	}
}

func GetAvailableTools() []deepseek.Tool {
	all := make([]deepseek.Tool, len(localTools))
	copy(all, localTools)

	if mcpClient != nil {
		all = append(all, mcpClient.GetDeepSeekTools()...)
	}
	return all
}

func ToolDescription() string {
	var sb strings.Builder
	sb.WriteString("【本地工具】\n")
	for _, t := range localTools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description))
	}

	if mcpClient != nil {
		mcpTools := mcpClient.GetDeepSeekTools()
		if len(mcpTools) > 0 {
			sb.WriteString("\n【MCP 工具】\n")
			for _, t := range mcpTools {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description))
			}
		}
	}
	return sb.String()
}

// ExecuteTool 修复：none/空 提前短路
func ExecuteTool(name string, args map[string]interface{}) (string, error) {
	// 占位符：无需工具，直接推进
	if name == "none" || name == "" {
		return "（无需工具，直接推进）", nil
	}

	if isLocalTool(name) {
		return executeLocal(name, args)
	}
	if mcpClient != nil && mcpClient.IsMCPTool(name) {
		return mcpClient.CallTool(context.Background(), name, args)
	}
	return "", fmt.Errorf("未知工具: %s", name)
}

func isLocalTool(name string) bool {
	for _, t := range localTools {
		if t.Function.Name == name {
			return true
		}
	}
	return false
}

func safePath(filename string) (string, error) {
	base, err := filepath.Abs(config.SandboxDir)
	if err != nil {
		return "", err
	}
	target := filepath.Join(base, filepath.Clean(filename))
	if !strings.HasPrefix(target, base+string(os.PathSeparator)) && target != base {
		return "", fmt.Errorf("路径越界，禁止访问沙箱外: %s", filename)
	}
	return target, nil
}

func executeLocal(name string, args map[string]interface{}) (string, error) {
	switch name {
	case "get_current_time":
		return time.Now().Format("2006-01-02 15:04:05"), nil

	case "write_file":
		filename, _ := args["filename"].(string)
		content, _ := args["content"].(string)
		if filename == "" {
			return "", fmt.Errorf("缺少文件名")
		}
		path, err := safePath(filename)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", fmt.Errorf("创建目录失败: %w", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("写入失败: %w", err)
		}
		return fmt.Sprintf("成功写入 %s（%d 字节）", filename, len(content)), nil

	case "list_files":
		base, _ := filepath.Abs(config.SandboxDir)
		if err := os.MkdirAll(base, 0755); err != nil {
			return "", fmt.Errorf("初始化沙箱失败: %w", err)
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return "", fmt.Errorf("读取沙箱失败: %w", err)
		}
		if len(entries) == 0 {
			return "沙箱目录为空", nil
		}
		var sb strings.Builder
		sb.WriteString("沙箱目录文件:\n")
		for _, e := range entries {
			suffix := ""
			if e.IsDir() {
				suffix = "/"
			}
			sb.WriteString("- " + e.Name() + suffix + "\n")
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("未知本地工具: %s", name)
	}
}
