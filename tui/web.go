package tui

import (
	"github.com/charmbracelet/bubbletea"
	netpkg "github.com/magma/netmon/net"
)

type webSelectModel struct {
	cfg    Config
	sel    int
	width  int
	height int
}

func newWebSelectModel(cfg Config, w, h int) webSelectModel {
	return webSelectModel{cfg: cfg, width: w, height: h}
}

func (m webSelectModel) Init() tea.Cmd { return nil }

func (m webSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		n := len(m.cfg.WebSites)
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
				it := m.cfg.WebSites[m.sel]
				_ = netpkg.OpenURL(it.URL)
				return m, sendCmd(returnToMenuMsg{})
			}
		case "q", "esc":
			return m, sendCmd(returnToMenuMsg{})
		}
	}
	return m, nil
}

func (m webSelectModel) View() string {
	var content []string
	items := m.cfg.WebSites
	for i, it := range items {
		if i == m.sel {
			content = append(content, "  "+selStyle.Render(padTo(cursorMark+" "+it.Name, m.width-4)))
		} else {
			content = append(content, "  "+itemStyle.Render("  "+it.Name))
		}
	}
	return renderScreen(header("打开网页", "", m.width), " ↑↓/j k 选择   Enter 打开浏览器   q 返回", content, m.width, m.height)
}