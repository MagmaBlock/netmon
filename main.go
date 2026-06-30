package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/magma/netmon/tui"
)

// 快捷直达参数：1=ICMP 2=TCP 3=测速 4=网关 5=网络信息 6=网页
func main() {
	cfg := tui.DefaultConfig()

	// 支持快捷参数，直接跳到对应功能（仅 ICMP/TCP/测速跳到默认目标/菜单）
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if err := runShortcut(arg, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	p := tea.NewProgram(tui.NewModel(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "运行出错:", err)
		os.Exit(1)
	}
}

func runShortcut(arg string, cfg tui.Config) error {
	switch arg {
	case "1":
		// ICMP Ping magma.ink
		targets := []tui.PingTarget{cfg.PingHosts[0]}
		return runPing(cfg, targets)
	case "2":
		targets := []tui.PingTarget{{
			Kind: "tcp", Host: cfg.PingHosts[0].Host, Port: cfg.TCPPort,
			Display: cfg.PingHosts[0].Host,
		}}
		return runPing(cfg, targets)
	case "3":
		return runSpeed(cfg, cfg.SpeedItems[0].Name, cfg.SpeedItems[0].URL)
	case "4":
		// 网关：尝试取默认网关直接 ping
		gwURL, iface, err := tui.DefaultGateway()
		if err != nil || gwURL == "" {
			return fmt.Errorf("未找到默认网关")
		}
		disp := "网关 " + gwURL
		if iface != "" {
			disp += "  (接口 " + iface + ")"
		}
		return runPing(cfg, []tui.PingTarget{{Kind: "icmp", Host: gwURL, Display: disp, IsGW: true, GWIface: iface}})
	case "5":
		return runInfo(cfg)
	case "6":
		if err := tui.OpenURL(cfg.WebSites[0].URL); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("未知参数: %s", arg)
}

func runPing(cfg tui.Config, targets []tui.PingTarget) error {
	p := tea.NewProgram(tui.NewPingModel(cfg, targets), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runSpeed(cfg tui.Config, name, url string) error {
	p := tea.NewProgram(tui.NewSpeedModel(name, url), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runInfo(cfg tui.Config) error {
	p := tea.NewProgram(tui.NewInfoModel(cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}