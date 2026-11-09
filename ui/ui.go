package ui

import (
	"fmt"
	"os"
	"strings"
)

// 终端显示层：统一负责排版、缩进、配色与边框。
// 想关掉颜色：设置环境变量 NO_COLOR=1 即可（纯文本输出）。
var colorEnabled = os.Getenv("NO_COLOR") == ""

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

func paint(code, s string) string {
	if !colorEnabled || s == "" {
		return s
	}
	return code + s + reset
}

// isWide 判断字符是否按两格宽度显示（中文、emoji，以及中文终端里按 2 宽渲染的模糊字符）
func isWide(r rune) bool {
	return r == 0x00B7 || // · 中间点：中文 Windows 终端按 2 格宽渲染
		(r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0xA4CF) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1FAFF)
}

// Width 字符串显示宽度
func Width(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func pad(s string, w int) string {
	if Width(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-Width(s))
}

// Banner 程序启动标题
func Banner(model, sandbox string) {
	const w = 64       // 整行总宽
	const inner = w - 4 // "│  " 后可用宽度（左 │ + 2 空格 + inner + 右 │）
	bar := func(s, color string) {
		pad := ""
		if n := inner - Width(s); n > 0 {
			pad = strings.Repeat(" ", n)
		}
		fmt.Println(paint(cyan+bold, "│") + "  " + paint(color, s) + pad + paint(cyan+bold, "│"))
	}
	fmt.Println()
	fmt.Println(paint(cyan+bold, "╭"+strings.Repeat("─", w-2)+"╮"))
	bar("DeepSeek Go Agent", bold)
	bar("感知 ・ 规划 ・ 反思 ・ 重规划 ・ 复盘 ・ 记忆 ・ MCP", gray)
	fmt.Println(paint(cyan+bold, "├"+strings.Repeat("─", w-2)+"┤"))
	bar("模型  "+model, gray)
	bar("沙箱  "+sandbox, gray)
	bar("命令  /reset  /stats  /new <会话ID>  exit", gray)
	fmt.Println(paint(cyan+bold, "╰"+strings.Repeat("─", w-2)+"╯"))
}

// Task 一次任务的开始
func Task(input string) {
	fmt.Println()
	fmt.Println(paint(gray, strings.Repeat("━", 62)))
	fmt.Println("  " + paint(bold+blue, "▶ 用户任务") + "   " + input)
	fmt.Println(paint(gray, strings.Repeat("━", 62)))
}

// Stage 阶段标题（带序号，让"现在走到第几步"一目了然）
func Stage(no int, name string) {
	fmt.Println()
	fmt.Println("  " + paint(cyan+bold, fmt.Sprintf("▍%d  %s", no, name)))
	fmt.Println("  " + paint(gray, strings.Repeat("┈", 60)))
}

// Note 说明性小字
func Note(s string) {
	fmt.Println("     " + paint(gray, s))
}

// Meta 键值行（超长自动折行，保持对齐）
func Meta(k, v string) {
	lines := wrap(v, 52)
	if len(lines) <= 1 {
		fmt.Println("     " + paint(gray, pad(k, 10)) + "  " + v)
		return
	}
	fmt.Println("     " + paint(gray, pad(k, 10)) + "  " + lines[0])
	for _, l := range lines[1:] {
		fmt.Println("     " + strings.Repeat(" ", 12) + l)
	}
}

// Block 带标题的多行块（缩进排列，过长自动折行）
func Block(title, body string) {
	fmt.Println("     " + paint(gray, title))
	for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- "))
		if l == "" {
			continue
		}
		for i, seg := range wrap(l, 54) {
			if i == 0 {
				fmt.Println("       " + paint(gray, "·") + " " + seg)
			} else {
				fmt.Println("         " + seg)
			}
		}
	}
}

// PlanRow 计划中的一行步骤：工具名靠左对齐，描述在后
func PlanRow(id int, desc, tool string) {
	t := tool
	if t == "" {
		t = "none"
	}
	fmt.Println("     " + paint(gray, fmt.Sprintf("%d.", id)+" ") +
		paint(cyan, pad(t, 20)) + desc)
}

// Step 开始执行某一步
func Step(id int, desc, tool string) {
	fmt.Println()
	fmt.Println("     " + paint(cyan, fmt.Sprintf("▸ 步骤 %d", id)) + "  " + paint(bold, desc))
	fmt.Println("       " + paint(gray, "调用工具  "+tool))
}

// Tool 工具调用明细
func Tool(name, args string) {
	if args == "" {
		args = "{}"
	}
	fmt.Println("       " + paint(gray, "入参  ") + paint(gray, args))
}

// OK 成功（多行结果后续行自动对齐）
func OK(s string) {
	lines := strings.Split(s, "\n")
	fmt.Println("       " + paint(green, "✓") + " " + lines[0])
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		fmt.Println("         " + l)
	}
}

// Fail 失败
func Fail(s string) {
	fmt.Println("       " + paint(red, "✗") + " " + s)
}

// Warn 警告
func Warn(s string) {
	fmt.Println("       " + paint(yellow, "!") + " " + s)
}

// Reflect 局部反思
func Reflect(no int) {
	fmt.Println("       " + paint(yellow, fmt.Sprintf("↻ 局部反思 #%d", no)) + paint(gray, "  分析失败原因…"))
}

// Replan 动态重规划
func Replan(no int) {
	fmt.Println()
	fmt.Println("     " + paint(yellow, fmt.Sprintf("↻ 动态重规划 #%d", no)) + paint(gray, "  基于最新环境重写计划"))
}

// Score 复盘分数
func Score(achieved bool, score int) {
	mark := paint(green, "✓ 目标达成")
	if !achieved {
		mark = paint(red, "✗ 未完全达成")
	}
	fmt.Println("     " + mark + "        " + paint(bold, fmt.Sprintf("%d", score)) + paint(gray, " / 10  质量评分"))
}

// Prompt 需要用户输入时的提示（不换行）
func Prompt(s string) {
	fmt.Print("\n     " + paint(yellow, "?") + " " + s)
}

// UserPrompt 主输入提示符
func UserPrompt() {
	fmt.Print("\n  " + paint(cyan+bold, "▶ 你: "))
}

// Reply 最终回复，装进一个框里（左右竖线严格对齐）
func Reply(title, body string) {
	const c = 66 // 内容区宽度（左 │ + 空格 + c 内容 + 右 │ = c+3）
	dashes := strings.Repeat("─", c+1)
	fill := func(s string) string {
		pad := ""
		if n := c - Width(s); n > 0 {
			pad = strings.Repeat(" ", n)
		}
		return s + pad
	}
	fmt.Println(paint(green, "╭"+dashes+"╮"))
	fmt.Println(paint(green, "│") + " " + paint(bold+green, fill(title)) + paint(green, "│"))
	fmt.Println(paint(green, "├"+dashes+"┤"))
	for _, l := range wrap(body, c) {
		fmt.Println(paint(green, "│") + " " + fill(l) + paint(green, "│"))
	}
	fmt.Println(paint(green, "╰"+dashes+"╯"))
}

// wrap 按显示宽度折行（中文按 2 宽计算）
func wrap(text string, width int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		line := ""
		lw := 0
		for _, r := range para {
			rw := 1
			if isWide(r) {
				rw = 2
			}
			if lw+rw > width {
				out = append(out, line)
				line, lw = "", 0
			}
			line += string(r)
			lw += rw
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
