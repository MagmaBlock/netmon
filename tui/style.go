package tui

import "github.com/charmbracelet/lipgloss"

// 颜色与样式
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("36")) // cyan

	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	rulerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	selStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("36")).
			Foreground(lipgloss.Color("0")) // selected row

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	hlStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("220")) // yellow

	okStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("46")) // green

	errStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196")) // red

	accStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("201")) // magenta

	barStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("201"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	checkedMark = "☑"
	uncheckedMark = "☐"
	cursorMark  = "▶"
	blankMark   = "  "
)