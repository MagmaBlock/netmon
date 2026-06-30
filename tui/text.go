package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	netpkg "github.com/magma/netmon/net"
)

// textModel 用于网络信息等纯文本滚动视图，可静态或异步收集
type textModel struct {
	title   string
	lines   []string
	top     int
	width   int
	height  int
	static  bool   // 静态文本（无需收集），如网关未找到的提示
	collected bool
}

func newTextModel(title string, lines []string, w, h int) textModel {
	return textModel{title: title, lines: lines, width: w, height: h}
}

func (m textModel) Init() tea.Cmd { return nil }

// startCollect 切到网络信息视图时由顶层调用，发起收集 goroutine
func (m textModel) startCollect() tea.Cmd {
	return func() tea.Msg {
		lines := netpkg.CollectNetworkInfo()
		return infoCollectedMsg{lines: lines}
	}
}

type infoCollectedMsg struct {
	lines []string
}

func (m textModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case infoCollectedMsg:
		m.lines = msg.lines
		m.collected = true
		return m, nil

	case tea.KeyMsg:
		maxl := m.height - 3
		if maxl < 1 {
			maxl = 1
		}
		switch msg.String() {
		case "up", "k":
			if m.top > 0 {
				m.top--
			}
		case "down", "j":
			if m.top < len(m.lines)-maxl {
				m.top++
			}
		case "pgup":
			m.top -= maxl
			if m.top < 0 {
				m.top = 0
			}
		case "pgdown":
			m.top += maxl
			if m.top > len(m.lines)-maxl {
				m.top = len(m.lines) - maxl
			}
			if m.top < 0 {
				m.top = 0
			}
		case "q", "esc":
			return m, sendCmd(returnToMenuMsg{})
		}
	}
	return m, nil
}

func (m textModel) View() string {
	maxl := m.height - 3
	if maxl < 1 {
		maxl = 1
	}
	var content []string
	end := m.top + maxl
	if end > len(m.lines) {
		end = len(m.lines)
	}
	visible := m.lines[m.top:end]
	for _, l := range visible {
		content = append(content, " "+truncateLine(strings.TrimSpace(l), m.width-2))
	}
	// 填充空白
	for i := len(visible); i < maxl; i++ {
		content = append(content, "")
	}
	if !m.collected && !m.static && len(m.lines) == 0 {
		content = []string{dimStyle.Render(" 正在收集...")}
	}
	footerText := " q 返回 "
	if len(m.lines) > maxl && !m.static {
		footerText = " " + infoLine(m.top, end, len(m.lines)) + "  ↑↓/j k 滚动   PgUp/PgDn   q 返回 "
	} else if !m.static {
		footerText = " ↑↓/j k 滚动   q 返回 "
	}
	return renderScreen(header(m.title, "", m.width), footerText, content, m.width, m.height)
}

func infoLine(a, b, total int) string {
	a++
	if a > total {
		a = total
	}
	if total == 0 {
		a = 0
	}
	return "行 1-" + itoa(b) + "/" + itoa(total)
}