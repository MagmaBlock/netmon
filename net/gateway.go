package net

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
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
	out, err := exec.Command("route", "print").Output()
	if err != nil {
		return "", "", err
	}
	s := string(out)
	re := regexp.MustCompile(`0\.0\.0\.0\s+0\.0\.0\.0\s+(\d+\.\d+\.\d+\.\d+)`)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1], "", nil
	}
	return "", "", nil
}

func regMatch(s, pattern string) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}