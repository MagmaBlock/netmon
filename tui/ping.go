package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	netpkg "github.com/magma/netmon/net"
)

type pingModel struct {
	cfg     Config
	targets []PingTarget
	width   int
	height  int

	buffers  [][]string
	channels []chan string
	cancels  []context.CancelFunc
	finished []bool

	startedAt time.Time
}

func newPingModel(cfg Config, targets []PingTarget, w, h int) pingModel {
	m := pingModel{
		cfg:      cfg,
		targets:  targets,
		width:    w,
		height:   h,
		buffers:  make([][]string, len(targets)),
		channels: make([]chan string, len(targets)),
		cancels:  make([]context.CancelFunc, len(targets)),
		finished: make([]bool, len(targets)),
		startedAt: time.Now(),
	}
	for i := range targets {
		m.channels[i] = make(chan string, 32)
	}
	return m
}

func (m pingModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	for i, t := range m.targets {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancels[i] = cancel
		ch := m.channels[i]
		if t.Kind == "icmp" {
			err := netpkg.Ping(ctx, t.Host, ch)
			if err != nil {
				// 直接把错误塞进 buffer
				m.buffers[i] = []string{"启动失败: " + err.Error()}
				m.finished[i] = true
				continue
			}
		} else {
			go netpkg.TCPPing(ctx, t.Host, t.Port, ch)
		}
		cmds = append(cmds, waitForPingLine(i, ch))
	}
	return tea.Batch(cmds...)
}

type pingLineMsg struct {
	idx  int
	line string
}
type pingDoneMsg struct {
	idx int
}

func waitForPingLine(idx int, ch chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return pingDoneMsg{idx: idx}
		}
		return pingLineMsg{idx: idx, line: line}
	}
}

func (m pingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pingLineMsg:
		m.buffers[msg.idx] = append(m.buffers[msg.idx], msg.line)
		if len(m.buffers[msg.idx]) > 200 {
			m.buffers[msg.idx] = m.buffers[msg.idx][len(m.buffers[msg.idx])-120:]
		}
		return m, waitForPingLine(msg.idx, m.channels[msg.idx])

	case pingDoneMsg:
		m.finished[msg.idx] = true
		// 没有进一步 cmd：channel 已关闭
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			for _, c := range m.cancels {
				if c != nil {
					c()
				}
			}
			return m, sendCmd(returnToMenuMsg{})
		}
	}
	return m, nil
}

func (m pingModel) View() string {
	N := len(m.targets)
	var b strings.Builder
	title := fmt.Sprintf("多目标 Ping — %d 个目标", N)
	if N == 1 {
		t := m.targets[0]
		title = "ICMP Ping — " + t.Host
		if t.Kind == "tcp" {
			title = fmt.Sprintf("TCP Ping — %s:%d", t.Host, t.Port)
		}
		if t.IsGW && t.GWIface != "" {
			title += "  (网关 · 接口 " + t.GWIface + ")"
		} else if t.IsGW {
			title += "  (网关)"
		}
	}
	b.WriteString(header(title, "", m.width))
	b.WriteString("\n")

	top := 2
	bottom := m.height - 1
	avail := bottom - top
	if avail < 1 {
		avail = 1
	}
	if N*3+1 > avail {
		b.WriteString(errStyle.Render(fmt.Sprintf(" 终端太小或目标太多: %d 个目标需要至少 %d 行", N, 3*N+3)))
		for i := top; i < m.height-1; i++ {
			b.WriteString("\n")
		}
		b.WriteString(footer(" q 返回 ", m.width))
		return b.String()
	}
	panelH := avail / N
	if panelH < 3 {
		panelH = 3
	}
	extra := avail - panelH*N
	y := top
	for i, t := range m.targets {
		ph := panelH + boolToInt(i < extra)
		tag := "ICMP"
		if t.Kind == "tcp" {
			tag = fmt.Sprintf("TCP:%d", t.Port)
		}
		label := fmt.Sprintf(" %s  (%s) ", t.Display, tag)
		fill := m.width - lipgloss.Width(label) - 1
		if fill < 0 {
			fill = 0
		}
		b.WriteString(accStyle.Render(label + strings.Repeat("─", fill)))
		b.WriteString("\n")
		y++
		resultLines := ph - 1
		buf := m.buffers[i]
		var show []string
		if len(buf) > resultLines {
			show = buf[len(buf)-resultLines:]
		} else {
			show = buf
		}
		for _, l := range show {
			b.WriteString(" ")
			b.WriteString(truncateLine(l, m.width-2))
			b.WriteString("\n")
		}
		// 填充空白
		for j := len(show); j < resultLines; j++ {
			b.WriteString("\n")
		}
		y += resultLines
	}
	// 行数不足补空
	for ; y < m.height-1; y++ {
		b.WriteString("\n")
	}
	b.WriteString(footer(" q 停止 ", m.width))
	return b.String()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}