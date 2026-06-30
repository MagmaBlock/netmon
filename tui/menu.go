package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type menuModel struct {
	cfg    Config
	items  []menuItem
	sel    int
	width  int
	height int
}

type menuItem struct {
	label string
	act  string // "icmp"|"tcp"|"speed"|"gw"|"info"|"web"|"quit"
}

func newMenuModel(cfg Config) menuModel {
	return menuModel{
		cfg: cfg,
		items: []menuItem{
			{"ICMP Ping", "icmp"},
			{"TCP  Ping", "tcp"},
			{"下载测速 (实时)", "speed"},
			{"Ping 网关", "gw"},
			{"网络信息", "info"},
			{"打开网页", "web"},
			{"退出", "quit"},
		},
	}
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.sel = (m.sel - 1 + len(m.items)) % len(m.items)
		case "down", "j":
			m.sel = (m.sel + 1) % len(m.items)
		case "enter":
			it := m.items[m.sel]
			switch it.act {
			case "icmp", "tcp":
				return m, sendCmd(openMultiSelectMsg{action: it.act})
			case "speed":
				return m, sendCmd(openSpeedSelectMsg{})
			case "gw":
				// 直接构造一个仅含网关的目标
				return m, gatewayPingCmd(m.cfg)
			case "info":
				return m, sendCmd(openInfoMsg{})
			case "web":
				return m, sendCmd(openWebSelectMsg{})
			case "quit":
				return m, sendCmd(quitMsg{})
			}
		case "q", "esc":
			return m, sendCmd(quitMsg{})
		}
	}
	return m, nil
}

func (m menuModel) View() string {
	var b strings.Builder
	b.WriteString(header("网络监测工具", subtitleForOS(), m.width))
	b.WriteString("\n")

	top := 3
	avail := m.height - top - 1
	n := len(m.items)
	first := 0
	if n > 0 {
		first = m.sel - avail/2
		if first < 0 {
			first = 0
		}
		if first+avail > n {
			first = n - avail
		}
		if first < 0 {
			first = 0
		}
	}
	for i := first; i < n && i < first+avail; i++ {
		it := m.items[i]
		label := cursorMark + " " + it.label
		if i == m.sel {
			label = padTo(label, m.width-4)
			b.WriteString("  ")
			b.WriteString(selStyle.Render(label))
		} else {
			b.WriteString("  ")
			b.WriteString(itemStyle.Render(cursorBlank() + " " + it.label))
		}
		b.WriteString("\n")
	}
	// 占位填满
	for i := top + min(n, avail); i < m.height-1; i++ {
		b.WriteString("\n")
	}
	b.WriteString(footer(" ↑↓/j k 选择   Enter 确认   q/Esc 退出", m.width))
	return b.String()
}

func cursorBlank() string {
	// 与 cursorMark("▶") 等宽的占位
	return " "
}

func subtitleForOS() string {
	return "Go · bubbletea TUI"
}

// gatewayPingCmd 取默认网关并构造 ping 目标
func gatewayPingCmd(cfg Config) tea.Cmd {
	return func() tea.Msg {
		gw, iface, _ := defaultGateway()
		if gw == "" {
			return openTextMsg{title: "Ping 网关", lines: []string{"", "  未找到默认网关", "", "  按任意键返回"}}
		}
		disp := "网关 " + gw
		if iface != "" {
			disp += "   (接口 " + iface + ")"
		}
		t := PingTarget{Kind: "icmp", Host: gw, Display: disp, IsGW: true, GWIface: iface}
		return openPingMsg{targets: []PingTarget{t}}
	}
}

func padTo(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// sendCmd 包一个无副作用的命令
func sendCmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}