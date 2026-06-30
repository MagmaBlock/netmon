package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// header 渲染顶部标题栏 + 分隔线
func header(title, subtitle string, width int) string {
	if width <= 0 {
		width = 80
	}
	t := titleStyle.Render(title)
	if subtitle != "" {
		// 右对齐副标题
		sub := dimStyle.Render(subtitle)
		// 计算需要的空格
		tw := lipgloss.Width(t)
		sw := lipgloss.Width(sub)
		gap := width - tw - sw
		if gap < 1 {
			gap = 1
		}
		t = t + strings.Repeat(" ", gap) + sub
	}
	line := rulerStyle.Render(strings.Repeat("─", width))
	return t + "\n" + line
}

// footer 渲染底部提示行
func footer(text string, width int) string {
	return footerStyle.Render(" " + text)
}

// truncateLine 截断到不超过 width 宽度（按 lipgloss 宽度）
func truncateLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}