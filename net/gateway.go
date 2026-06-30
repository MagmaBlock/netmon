package net

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// DefaultGateway 返回默认网关 IP 与出口接口名（取不到时为空字符串，err 可能为 nil）
func DefaultGateway() (ip, iface string, err error) {
	switch runtime.GOOS {
	case "darwin":
		return gatewayDarwin()
	case "linux":
		return gatewayLinux()
	case "windows":
		return gatewayWindows()
	default:
		return "", "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func gatewayDarwin() (string, string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", "", err
	}
	s := string(out)
	gw := regMatch(s, `gateway:\s*(\S+)`)
	iface := regMatch(s, `interface:\s*(\S+)`)
	return gw, iface, nil
}

func gatewayLinux() (string, string, error) {
	out, err := exec.Command("ip", "route").Output()
	if err != nil {
		return "", "", err
	}
	s := string(out)
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "default") {
			fields := strings.Fields(line)
			gw := ""
			iface := ""
			for i, f := range fields {
				if f == "via" && i+1 < len(fields) {
					gw = fields[i+1]
				}
				if f == "dev" && i+1 < len(fields) {
					iface = fields[i+1]
				}
			}
			if gw != "" {
				return gw, iface, nil
			}
		}
	}
	return "", "", nil
}

func gatewayWindows() (string, string, error) {
	// 优先用 PowerShell：按 RouteMetric 升序选最低优先级网关，
	// 这能避免被 Radmin VPN / Tailscale / Wintun 等虚拟网卡
	// 注册的 0.0.0.0 路由"抢先匹配"（之前的 route print 解析
	// 取了第一个匹配项，常误判为 26.0.0.1 这类 VPN 网关）。
	if gw, iface, ok := gatewayWindowsPS(); ok {
		return gw, iface, nil
	}
	return gatewayWindowsRoutePrint()
}

// gatewayWindowsPS 走 PowerShell Get-NetRoute / Get-NetIPInterface。
func gatewayWindowsPS() (gw, iface string, ok bool) {
	const script = `$r = Get-NetRoute -DestinationPrefix 0.0.0.0/0 -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -First 1
if (-not $r -or -not $r.NextHop) { exit 1 }
$alias = (Get-NetIPInterface -InterfaceIndex $r.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue).InterfaceAlias
Write-Output ('GW=' + $r.NextHop)
if ($alias) { Write-Output ('IF=' + $alias) }`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", "", false
	}
	for _, line := range strings.Split(decodeLocal(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "GW="):
			gw = strings.TrimSpace(strings.TrimPrefix(line, "GW="))
		case strings.HasPrefix(line, "IF="):
			iface = strings.TrimSpace(strings.TrimPrefix(line, "IF="))
		}
	}
	if gw == "" || !looksLikeIPv4(gw) {
		return "", "", false
	}
	return gw, iface, true
}

// gatewayWindowsRoutePrint 用 route print 兜底，按 metric 选最低者。
// 因 PowerShell 不可用时（精简版 Windows / 策略限制）才走该路径。
func gatewayWindowsRoutePrint() (string, string, error) {
	out, err := commandOutput("route", "print")
	if err != nil {
		return "", "", err
	}
	re := regexp.MustCompile(`^\s*0\.0\.0\.0\s+0\.0\.0\.0\s+(\d+\.\d+\.\d+\.\d+)\s+(\d+\.\d+\.\d+\.\d+)\s+(\d+)\s*$`)
	bestMetric := -1
	var bestGW, bestIfaceIP string
	for _, line := range strings.Split(out, "\n") {
		m := re.FindStringSubmatch(line)
		if len(m) < 4 {
			continue
		}
		metric, _ := strconv.Atoi(strings.TrimSpace(m[3]))
		if bestMetric < 0 || metric < bestMetric {
			bestMetric = metric
			bestGW = m[1]
			bestIfaceIP = m[2]
		}
	}
	if bestGW == "" {
		return "", "", nil
	}
	return bestGW, bestIfaceIP, nil
}

func looksLikeIPv4(s string) bool {
	m, err := regexp.MatchString(`^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`, s)
	if !m || err != nil {
		return false
	}
	parts := strings.Split(s, ".")
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

func regMatch(s, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}