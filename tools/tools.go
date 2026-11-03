package tools

import (
	"context"
	"deepseek-agent/config"
	"deepseek-agent/mcpclient"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	{
		Type: "function",
		Function: deepseek.Function{
			Name:        "get_weather",
			Description: "根据城市 adcode 查询实况天气。adcode 是高德城市编码，例如北京 110000、上海 310000、深圳 440300。如果用户只给了城市名，你需要先根据常识推断 adcode，或告知用户提供编码。",
			Parameters: &deepseek.FunctionParameters{
				Type: "object",
				Properties: map[string]interface{}{
					"city": map[string]interface{}{
						"type":        "string",
						"description": "城市 adcode(高德城市编码),如 110000",
					},
				},
				Required: []string{"city"},
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
		sb.WriteString(fmt.Sprintf("- %s: %s\\n", t.Function.Name, t.Function.Description))
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

// ExecuteTool 路由：none 短路 → 本地工具 → MCP 工具
func ExecuteTool(name string, args map[string]interface{}) (string, error) {
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
			return "", fmt.Errorf("❌创建目录失败: %w", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("❌写入失败: %w", err)
		}
		return fmt.Sprintf("成功写入 %s(%d 字节)", filename, len(content)), nil

	case "list_files":
		base, _ := filepath.Abs(config.SandboxDir)
		if err := os.MkdirAll(base, 0755); err != nil {
			return "", fmt.Errorf("❌初始化沙箱失败: %w", err)
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return "", fmt.Errorf("❌读取沙箱失败: %w", err)
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

	case "get_weather":
		city, _ := args["city"].(string)
		if city == "" {
			return "", fmt.Errorf("缺少城市 adcode")
		}
		if config.AMapKey == "" {
			return "", fmt.Errorf("🤯未配置 AMAP_KEY,请在 .env 中设置")
		}
		return fetchAMapWeather(city)

	default:
		return "", fmt.Errorf("未知本地工具: %s", name)
	}
}

// fetchAMapWeather 调用高德天气 API
func fetchAMapWeather(adcode string) (string, error) {
	url := fmt.Sprintf(
		"https://restapi.amap.com/v3/weather/weatherInfo?city=%s&key=%s",
		adcode, config.AMapKey,
	)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("请求高德天气失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Status string `json:"status"`
		Info   string `json:"info"`
		Lives  []struct {
			City          string `json:"city"`
			Weather       string `json:"weather"`
			Temperature   string `json:"temperature"`
			WindDirection string `json:"winddirection"`
			WindPower     string `json:"windpower"`
			Humidity      string `json:"humidity"`
			ReportTime    string `json:"reporttime"`
		} `json:"lives"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析天气数据失败: %w", err)
	}
	if result.Status != "1" || len(result.Lives) == 0 {
		return "", fmt.Errorf("高德返回异常: status=%s info=%s", result.Status, result.Info)
	}

	l := result.Lives[0]
	return fmt.Sprintf(
		"城市: %s\n天气: %s\n温度: %s°C\n风向: %s\n风力: %s级\n湿度: %s%%\n发布时间: %s",
		l.City, l.Weather, l.Temperature, l.WindDirection, l.WindPower, l.Humidity, l.ReportTime,
	), nil
}
