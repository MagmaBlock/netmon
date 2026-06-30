package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	netpkg "github.com/magma/netmon/net"
)

type speedModel struct {
	name       string
	url        string
	width      int
	height     int
	ctx        context.Context
	cancel     context.CancelFunc
	ch         chan netpkg.Progress
	progress   netpkg.Progress
	recentInst float64
	recentAvg  float64
	started    bool
	finished   bool
}

func newSpeedModel(name, url string, w, h int) speedModel {
	ctx, cancel := context.WithCancel(context.Background())
	return speedModel{
		name:   name,
		url:    url,
		width:  w,
		height: h,
		ctx:    ctx,
		cancel: cancel,
		ch:     make(chan netpkg.Progress, 8),
	}
}

func (m speedModel) Init() tea.Cmd {
	go netpkg.Speedtest(m.ctx, m.url, 200, m.ch)
	return waitForSpeed(m.ch)
}

type speedProgressMsg struct {
	p netpkg.Progress
}

func waitForSpeed(ch chan netpkg.Progress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return speedFinishedMsg{}
		}
		return speedProgressMsg{p: p}
	}
}

type speedFinishedMsg struct{}

func (m speedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case speedProgressMsg:
		m.started = true
		m.progress = msg.p
		sampleDur := msg.p.LastSampleDur().Seconds()
		if sampleDur > 0.05 {
			m.recentInst = float64(msg.p.SampleBytes) / sampleDur
		}
		elapsed := msg.p.Elapsed().Seconds()
		if elapsed > 0.05 {
			m.recentAvg = float64(msg.p.Current) / elapsed
		} else {
			m.recentAvg = m.recentInst
		}
		if msg.p.Done {
			m.finished = true
			return m, nil
		}
		return m, waitForSpeed(m.ch)

	case speedFinishedMsg:
		m.finished = true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			if !m.finished {
				m.cancel()
			}
			return m, sendCmd(returnToMenuMsg{})
		}
		if m.finished {
			return m, sendCmd(returnToMenuMsg{})
		}
	}
	return m, nil
}

func (m speedModel) View() string {
	var content []string
	content = append(content, dimStyle.Render(" URL:"))
	content = append(content, "   "+truncateLine(m.url, m.width-4))
	content = append(content, "")

	total := m.progress.Total
	cur := m.progress.Current
	pct := 0.0
	if total > 0 {
		pct = float64(cur) / float64(total) * 100
	}
	sp := "未知"
	if total > 0 {
		sp = netpkg.FmtBytes(float64(total))
	}
	content = append(content, fmt.Sprintf(" 总大小  : %s", sp))
	content = append(content, "")

	barW := m.width - 8
	if barW > 54 {
		barW = 54
	}
	if barW < 10 {
		barW = 10
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(cur) / float64(total)
	}
	filled := int(ratio * float64(barW))
	if filled > barW {
		filled = barW
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barW-filled)
	pctTxt := "--"
	if total > 0 {
		pctTxt = fmt.Sprintf("%5.1f%%", pct)
	}
	content = append(content, " "+barStyle.Render("["+bar+"]")+" "+pctTxt)
	content = append(content, "")

	curStr := netpkg.FmtBytes(float64(cur))
	if total > 0 {
		curStr = fmt.Sprintf("%s   (%.1f%%)", curStr, pct)
	}
	content = append(content, fmt.Sprintf(" 已下载  : %s", curStr))
	content = append(content, "")

	content = append(content, " 实时速度: "+hlStyle.Render(fmt.Sprintf("%12s", netpkg.FmtSpeed(m.recentInst))))
	content = append(content, fmt.Sprintf(" 平均速度: %12s", netpkg.FmtSpeed(m.recentAvg)))
	content = append(content, fmt.Sprintf(" 已用时间: %s", netpkg.FmtTime(m.progress.Elapsed().Seconds())))

	if m.finished {
		content = append(content, okStyle.Render(" 状态    : 下载完成 ✓"))
	} else if m.recentInst > 0 && total > 0 && cur < total {
		eta := float64(total-cur) / m.recentInst
		content = append(content, okStyle.Render(fmt.Sprintf(" 预计剩余: %s", netpkg.FmtTime(eta))))
	} else {
		content = append(content, dimStyle.Render(" 预计剩余: 计算中..."))
	}
	content = append(content, "")

	var footerText string
	if m.finished {
		footerText = " 下载完成，按任意键返回 "
	} else {
		footerText = " q 停止下载并返回 "
	}
	return renderScreen(header("下载测速 — "+m.name, "", m.width), footerText, content, m.width, m.height)
}