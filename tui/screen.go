package tui

import "strings"

// renderScreen 把 (header, content 行, footer) 组装成 恰好 height 行的视图：
//   - header 占顶部 2 行（标题 + 分隔线）
//   - content 从第 3 行开始向下排
//   - footer 固定在第 height 行
func renderScreen(headerText, footerText string, content []string, width, height int) string {
	if height <= 0 {
		height = 24
	}
	var b strings.Builder
	b.WriteString(headerText)
	b.WriteString("\n")
	avail := height - 3 // 头部 2 行 + footer 1 行
	if avail < 1 {
		avail = 1
	}
	// 滚动：若内容超出可视区，截断到最后一屏
	start := 0
	if len(content) > avail {
		start = len(content) - avail
	}
	visible := content[start:]
	for _, l := range visible {
		b.WriteString(l)
		b.WriteString("\n")
	}
	// 填充空白
	used := len(visible)
	for i := used; i < avail; i++ {
		b.WriteString("\n")
	}
	b.WriteString(footer(footerText, width))
	return b.String()
}