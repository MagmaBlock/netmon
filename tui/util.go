package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	netpkg "github.com/magma/netmon/net"
)

// defaultGateway 调用 net 包
func defaultGateway() (string, string, error) {
	return netpkg.DefaultGateway()
}

// DefaultGateway 公开供 main 调用
func DefaultGateway() (string, string, error) { return defaultGateway() }

// OpenURL 公开供 main 调用
func OpenURL(url string) error { return netpkg.OpenURL(url) }

// openTextMsg 直接展示静态文本（无收集动作）
type openTextMsg struct {
	title string
	lines []string
}

// handleOpenText 切到 stateText 展示静态文本
func handleOpenText(m Model, msg openTextMsg) (Model, tea.Cmd) {
	m.state = stateText
	tm := newTextModel(msg.title, msg.lines, m.width, m.height)
	tm.static = true
	m.text = tm
	return m, nil
}

// ===== 顶层 Model 的导出快捷构造（供 main 的快捷直达参数用） =====

// NewPingModel 构造独立的 ping 视图（顶层 Model）
func NewPingModel(cfg Config, targets []PingTarget) Model {
	m := NewModel(cfg)
	m.state = statePing
	m.ping = newPingModel(cfg, targets, 80, 24)
	// 在 tea.WindowSizeMsg 来之前给一个默认尺寸；Init 起来时窗口尺寸会给到
	return m
}

// NewSpeedModel 构造独立的测速视图
func NewSpeedModel(name, url string) Model {
	cfg := DefaultConfig()
	m := NewModel(cfg)
	m.state = stateSpeed
	m.speed = newSpeedModel(name, url, 80, 24)
	return m
}

// NewInfoModel 构造独立的网络信息视图
func NewInfoModel(cfg Config) Model {
	m := NewModel(cfg)
	m.state = stateText
	m.text = newTextModel("网络信息", nil, 80, 24)
	return m
}

var _ = tea.Quit