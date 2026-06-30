package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// multiSelectModel ICMP/TCP 选择目标的多选菜单
type multiSelectModel struct {
	cfg     Config
	action  string // "icmp" / "tcp"
	labels  []string
	checked []bool
	sel     int
	width   int
	height  int
	// ICMP 用：网关目标索引（labels 末尾追加，gwIdx == len(PingHosts) 时为网关）
	gwIdx int
	gw    string
	gwIface string
}

func newMultiSelectModel(cfg Config, action string, w, h int) multiSelectModel {
	m := multiSelectModel{cfg: cfg, action: action, width: w, height: h, gwIdx: -1}
	for _, ph := range cfg.PingHosts {
		m.labels = append(m.labels, ph.Display)
	}
	m.checked = make([]bool, len(m.labels))
	if action == "icmp" {
		// 默认勾选 magma.ink / cloudflare / 网关
		if len(m.labels) > 0 {
			m.checked[0] = true // magma.ink
		}
		if len(m.labels) > 2 {
			m.checked[2] = true // cloudflare.com
		}
		// 异步取网关；先以无网关占位，等 ready 再补一行
	}
	return m
}

func (m multiSelectModel) Init() tea.Cmd {
	if m.action == "icmp" {
		return func() tea.Msg {
			gw, iface, _ := defaultGateway()
			return gatewayReadyMsg{gw: gw, iface: iface}
		}
	}
	return nil
}

type gatewayReadyMsg struct {
	gw    string
	iface string
}

func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case gatewayReadyMsg:
		if msg.gw != "" {
			m.gw = msg.gw
			m.gwIface = msg.iface
			label := "网关  " + msg.gw
			if msg.iface != "" {
				label += "   (接口 " + msg.iface + ")"
			}
			m.labels = append(m.labels, label)
			m.checked = append(m.checked, true) // 默认勾选
			m.gwIdx = len(m.labels) - 1
		}
		return m, nil

	case tea.KeyMsg:
		n := len(m.labels)
		switch msg.String() {
		case "up", "k":
			if n > 0 {
				m.sel = (m.sel - 1 + n) % n
			}
		case "down", "j":
			if n > 0 {
				m.sel = (m.sel + 1) % n
			}
		case " ":
			if n > 0 {
				m.checked[m.sel] = !m.checked[m.sel]
			}
		case "a":
			for i := range m.checked {
				m.checked[i] = true
			}
		case "n":
			for i := range m.checked {
				m.checked[i] = false
			}
		case "enter":
			return m, m.startPing()
		case "q", "esc":
			return m, sendCmd(returnToMenuMsg{})
		}
	}
	return m, nil
}

// startPing 把当前勾选项转成 PingTarget 列表，发出 openPingMsg
func (m multiSelectModel) startPing() tea.Cmd {
	var targets []PingTarget
	for i, c := range m.checked {
		if !c {
			continue
		}
		if i == m.gwIdx {
			disp := "网关 " + m.gw
			targets = append(targets, PingTarget{Kind: "icmp", Host: m.gw, Display: disp, IsGW: true, GWIface: m.gwIface})
			continue
		}
		ph := m.cfg.PingHosts[i]
		if m.action == "icmp" {
			targets = append(targets, PingTarget{Kind: "icmp", Host: ph.Host, Display: ph.Host})
		} else {
			targets = append(targets, PingTarget{Kind: "tcp", Host: ph.Host, Port: m.cfg.TCPPort, Display: ph.Host + ":" + itoa(m.cfg.TCPPort)})
		}
	}
	if len(targets) == 0 {
		return nil
	}
	return sendCmd(openPingMsg{targets: targets})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func (m multiSelectModel) View() string {
	var b strings.Builder
	kindName := "ICMP"
	if m.action == "tcp" {
		kindName = "TCP"
	}
	b.WriteString(header(kindName+" Ping — 选择目标 (可多选)", "Space 多选  Enter 开始  单选则单视图", m.width))
	b.WriteString("\n")

	top := 3
	avail := m.height - top - 1
	n := len(m.labels)
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
		mark := uncheckedMark
		if m.checked[i] {
			mark = checkedMark
		}
		label := cursorMark + " " + mark + " " + m.labels[i]
		if i == m.sel {
			label = padTo(label, m.width-4)
			b.WriteString("  ")
			b.WriteString(selStyle.Render(label))
		} else {
			label = "  " + mark + " " + m.labels[i]
			b.WriteString("  ")
			b.WriteString(itemStyle.Render(label))
		}
		b.WriteString("\n")
	}
	for i := top + min(n, avail); i < m.height-1; i++ {
		b.WriteString("\n")
	}
	cnt := 0
	for _, c := range m.checked {
		if c {
			cnt++
		}
	}
	_ = lipgloss.Width
	b.WriteString(footer(" Space 切换选中   a 全选   n 全不选   Enter 开始("+itoa(cnt)+")   q 取消", m.width))
	return b.String()
}