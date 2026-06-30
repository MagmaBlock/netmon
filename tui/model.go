package tui

import (
	"github.com/charmbracelet/bubbletea"
)

// PingTarget 是一个 ping 目标
type PingTarget struct {
	Kind    string // "icmp" | "tcp"
	Host    string
	Port    int
	Display string
	IsGW    bool
	GWIface string
}

// SpeedSource 测速源
type SpeedSource struct {
	Name string
	URL  string
}

// WebSite 打开网页项
type WebSite struct {
	Name string
	URL  string
}

// Config 全局可配置项
type Config struct {
	PingHosts  []PingTarget
	SpeedItems []SpeedSource
	WebSites   []WebSite
	TCPPort    int
}

// DefaultConfig 返回内置默认配置
func DefaultConfig() Config {
	return Config{
		TCPPort: 443,
		PingHosts: []PingTarget{
			{Kind: "icmp", Host: "magma.ink", Display: "magma.ink   (默认)"},
			{Kind: "icmp", Host: "baidu.com", Display: "baidu.com"},
			{Kind: "icmp", Host: "cloudflare.com", Display: "cloudflare.com"},
			{Kind: "icmp", Host: "github.com", Display: "github.com"},
			{Kind: "icmp", Host: "google.com", Display: "google.com"},
		},
		SpeedItems: []SpeedSource{
			{
				Name: "原神 5.5.0",
				URL:  "https://autopatchcn.yuanshen.com/client_app/download/pc_zip/20250314105313_OcRjEyGXX8Txtqm4/YuanShen_5.5.0.zip.001",
			},
			{
				Name: "星穹铁道 4.3.0",
				URL:  "https://autopatchcn.bhsr.com/client/cn/20260523104353_kjwMxQcpFWHse2S2/PC/download/StarRail_4.3.0.7z.007",
			},
		},
		WebSites: []WebSite{
			{Name: "itdog.cn 本地测速", URL: "https://www.itdog.cn/localhost/"},
			{Name: "ip138.com", URL: "https://www.ip138.com/"},
		},
	}
}

// state 枚举顶层视图状态
type state int

const (
	stateMenu state = iota
	stateMultiSelect
	statePing
	stateSpeedSelect
	stateSpeed
	stateInfo
	stateWebSelect
	stateText // 通用滚动文本（网络信息）
	stateQuit
)

// Model 顶层模型
type Model struct {
	cfg   Config
	state state
	width int
	height int

	menu       menuModel
	multiSel   multiSelectModel
	ping       pingModel
	speedSel   speedSelectModel
	speed      speedModel
	info       textModel
	webSel     webSelectModel
	text       textModel // generic text view (reused for info)
}

// NewModel 构造顶层模型
func NewModel(cfg Config) Model {
	return Model{
		cfg:   cfg,
		state: stateMenu,
		menu:  newMenuModel(cfg),
	}
}

// Init 启动命令
func (m Model) Init() tea.Cmd {
	switch m.state {
	case stateText:
		if !m.text.static && len(m.text.lines) == 0 {
			return m.text.startCollect()
		}
	case statePing:
		return m.ping.Init()
	case stateSpeed:
		return m.speed.Init()
	}
	return nil
}

// Update 顶层事件路由
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.menu.width, m.menu.height = msg.Width, msg.Height
		m.multiSel.width, m.multiSel.height = msg.Width, msg.Height
		m.ping.width, m.ping.height = msg.Width, msg.Height
		m.speedSel.width, m.speedSel.height = msg.Width, msg.Height
		m.speed.width, m.speed.height = msg.Width, msg.Height
		m.webSel.width, m.webSel.height = msg.Width, msg.Height
		m.text.width, m.text.height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}

	case openPingMsg:
		// 切换到 ping 视图，targets 已就绪
		m.state = statePing
		m.ping = newPingModel(m.cfg, msg.targets, m.width, m.height)
		return m, m.ping.Init()

	case openMultiSelectMsg:
		m.state = stateMultiSelect
		m.multiSel = newMultiSelectModel(m.cfg, msg.action, m.width, m.height)
		return m, m.multiSel.Init()

	case openSpeedSelectMsg:
		m.state = stateSpeedSelect
		m.speedSel = newSpeedSelectModel(m.cfg, m.width, m.height)
		return m, nil

	case openSpeedMsg:
		m.state = stateSpeed
		m.speed = newSpeedModel(msg.name, msg.url, m.width, m.height)
		return m, m.speed.Init()

	case openInfoMsg:
		m.state = stateText
		m.text = newTextModel("网络信息", nil, m.width, m.height)
		return m, m.text.startCollect()

	case openWebSelectMsg:
		m.state = stateWebSelect
		m.webSel = newWebSelectModel(m.cfg, m.width, m.height)
		return m, nil

	case returnToMenuMsg:
		m.state = stateMenu
		m.menu = newMenuModel(m.cfg)
		m.menu.width, m.menu.height = m.width, m.height
		return m, nil

	case openTextMsg:
		return handleOpenText(m, msg)

	case quitMsg:
		return m, tea.Quit
	}

	// 派发到当前状态对应的子模型
	switch m.state {
	case stateMenu:
		mm, cmd := m.menu.Update(msg)
		m.menu = mm.(menuModel)
		return m, cmd
	case stateMultiSelect:
		mm, cmd := m.multiSel.Update(msg)
		m.multiSel = mm.(multiSelectModel)
		return m, cmd
	case statePing:
		mm, cmd := m.ping.Update(msg)
		m.ping = mm.(pingModel)
		return m, cmd
	case stateSpeedSelect:
		mm, cmd := m.speedSel.Update(msg)
		m.speedSel = mm.(speedSelectModel)
		return m, cmd
	case stateSpeed:
		mm, cmd := m.speed.Update(msg)
		m.speed = mm.(speedModel)
		return m, cmd
	case stateText:
		mm, cmd := m.text.Update(msg)
		m.text = mm.(textModel)
		return m, cmd
	case stateWebSelect:
		mm, cmd := m.webSel.Update(msg)
		m.webSel = mm.(webSelectModel)
		return m, cmd
	}
	return m, nil
}

// View 顶层渲染
func (m Model) View() string {
	switch m.state {
	case stateMenu:
		return m.menu.View()
	case stateMultiSelect:
		return m.multiSel.View()
	case statePing:
		return m.ping.View()
	case stateSpeedSelect:
		return m.speedSel.View()
	case stateSpeed:
		return m.speed.View()
	case stateText:
		return m.text.View()
	case stateWebSelect:
		return m.webSel.View()
	}
	return ""
}

// ===== 子模型间切换的消息 =====

type openPingMsg struct {
	targets []PingTarget
}

type openMultiSelectMsg struct {
	action string // "icmp" | "tcp"
}

type openSpeedSelectMsg struct{}

type openSpeedMsg struct {
	name string
	url  string
}

type openInfoMsg struct{}

type openWebSelectMsg struct{}

type returnToMenuMsg struct{}

type quitMsg struct{}