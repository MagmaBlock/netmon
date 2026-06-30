package tui

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
)

type speedSelectModel struct {
	cfg    Config
	sel    int
	width  int
	height int
}

func newSpeedSelectModel(cfg Config, w, h int) speedSelectModel {
	return speedSelectModel{cfg: cfg, width: w, height: h}
}

func (m speedSelectModel) Init() tea.Cmd { return nil }

func (m speedSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		n := len(m.cfg.SpeedItems)
		switch msg.String() {
		case "up", "k":
			if n > 0 {
				m.sel = (m.sel - 1 + n) % n
			}
		case "down", "j":
			if n > 0 {
				m.sel = (m.sel + 1) % n
			}
		case "enter":
			if n > 0 {
				it := m.cfg.SpeedItems[m.sel]
				return m, sendCmd(openSpeedMsg{name: it.Name, url: it.URL})
			}
		case "q", "esc":
			return m, sendCmd(returnToMenuMsg{})
		}
	}
	return m, nil
}

func (m speedSelectModel) View() string {
	var b strings.Builder
	b.WriteString(header("下载测速 — 选择测速源", "", m.width))
	b.WriteString("\n")
	top := 3
	avail := m.height - top - 1
	items := m.cfg.SpeedItems
	n := len(items)
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
		label := cursorMark + " " + items[i].Name
		if i == m.sel {
			label = padTo(label, m.width-4)
			b.WriteString("  ")
			b.WriteString(selStyle.Render(label))
		} else {
			b.WriteString("  ")
			b.WriteString(itemStyle.Render("  " + items[i].Name))
		}
		b.WriteString("\n")
	}
	for i := top + min(n, avail); i < m.height-1; i++ {
		b.WriteString("\n")
	}
	b.WriteString(footer(" ↑↓/j k 选择   Enter 开始   q 返回", m.width))
	return b.String()
}