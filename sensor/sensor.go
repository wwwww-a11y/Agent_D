package sensor

import (
	"deepseek-agent/config"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// EnvSnapshot 环境快照
type EnvSnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	GoVersion   string    `json:"go_version"`
	WorkDir     string    `json:"work_dir"`
	SandboxDir  string    `json:"sandbox_dir"`
	SandboxTree string    `json:"sandbox_tree"`
	WorkTree    string    `json:"work_tree"`
	ToolsInfo   string    `json:"tools_info"`
}

// Sensor 环境感知器
type Sensor struct {
	MaxDepth     int
	MaxEntries   int
	ToolDescFunc func() string
}

func NewSensor(toolDescFunc func() string) *Sensor {
	return &Sensor{
		MaxDepth:     2,
		MaxEntries:   50,
		ToolDescFunc: toolDescFunc,
	}
}

// Snapshot 采集一次完整环境快照
func (s *Sensor) Snapshot() *EnvSnapshot {
	wd, _ := os.Getwd()
	return &EnvSnapshot{
		Timestamp:   time.Now(),
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GoVersion:   runtime.Version(),
		WorkDir:     wd,
		SandboxDir:  absPath(config.SandboxDir),
		SandboxTree: s.scanTree(config.SandboxDir),
		WorkTree:    s.scanTreeShallow(wd),
		ToolsInfo:   s.ToolDescFunc(),
	}
}

// ToPrompt 转成给模型看的文本
func (e *EnvSnapshot) ToPrompt() string {
	var sb strings.Builder
	sb.WriteString("【环境信息】\n")
	sb.WriteString(fmt.Sprintf("- 系统: %s/%s\n", e.OS, e.Arch))
	sb.WriteString(fmt.Sprintf("- Go 版本: %s\n", e.GoVersion))
	sb.WriteString(fmt.Sprintf("- 当前时间: %s\n", e.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("- 工作目录: %s\n", e.WorkDir))
	sb.WriteString(fmt.Sprintf("- 沙箱目录: %s\n", e.SandboxDir))
	sb.WriteString("\n【沙箱内容】\n")
	sb.WriteString(e.SandboxTree)
	sb.WriteString("\n【工作目录(浅层)】\n")
	sb.WriteString(e.WorkTree)
	sb.WriteString("\n【可用工具】\n")
	sb.WriteString(e.ToolsInfo)
	return sb.String()
}

func (s *Sensor) scanTree(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Sprintf("(无法解析路径 %s: %v)\n", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "(目录不存在或不可读)\n"
	}

	var sb strings.Builder
	count := 0
	s.walk(abs, "", 0, &sb, &count)
	if count == 0 {
		sb.WriteString("(空目录)\n")
	}
	if count >= s.MaxEntries {
		sb.WriteString(fmt.Sprintf("... (已截断，最多显示 %d 项)\n", s.MaxEntries))
	}
	return sb.String()
}

func (s *Sensor) walk(dir, prefix string, depth int, sb *strings.Builder, count *int) {
	if depth > s.MaxDepth || *count >= s.MaxEntries {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		if *count >= s.MaxEntries {
			return
		}
		*count++
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		sb.WriteString(prefix + "- " + e.Name() + suffix + "\n")
		if e.IsDir() {
			s.walk(filepath.Join(dir, e.Name()), prefix+"  ", depth+1, sb, count)
		}
	}
}

func (s *Sensor) scanTreeShallow(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return ""
	}
	var sb strings.Builder
	n := 0
	for _, e := range entries {
		if n >= 15 {
			sb.WriteString("... (已截断)\n")
			break
		}
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		sb.WriteString("- " + e.Name() + suffix + "\n")
		n++
	}
	return sb.String()
}

func absPath(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}
