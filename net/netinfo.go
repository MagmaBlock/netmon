package net

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdnet "net"
	stdhttp "net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ifaceDetail 是单块网卡的详尽信息
type ifaceDetail struct {
	Name   string
	Status string // active/down/unknown
	MAC    string
	MTU    string
	IPv4   string
	Mask   string
	IPv6   string
	RxBytes int64
	TxBytes int64
	HasStat bool
}

// gatewayHealth 网关连通性快照
type gatewayHealth struct {
	LatencyMs float64
	LossPct   float64
	OK        bool
}

// pubResult 公网IP + 归属地
type pubResult struct {
	IP  string
	Geo string // 拿到则如 "北京 / CN  [AS9808 CMNET]"，拿不到则空
	OK  bool
}

// CollectNetworkInfo 收集网络排障与质量相关的信息，返回每行字符串
func CollectNetworkInfo() []string {
	gw, iface := func() (string, string) {
		ip, ic, _ := DefaultGateway()
		return ip, ic
	}()

	// 并发收集所有耗时项
	var (
		detail     ifaceDetail
		dnsServers []string
		dnsLatMs   float64
		dnsOK      bool
		gwHealth   gatewayHealth
		pubV4, pubV6, pubPref pubResult
	)

	var wg sync.WaitGroup
	wg.Add(5)

	go func() { defer wg.Done()
		detail = collectIfaceDetail(iface)
	}()
	go func() { defer wg.Done()
		dnsServers = collectDNSServers()
	}()
	go func() { defer wg.Done()
		dnsLatMs, dnsOK = dnsResolveLatency("www.baidu.com", 3*time.Second)
	}()
	go func() { defer wg.Done()
		if gw != "" {
			gwHealth = gatewayPingHealth(gw, 4)
		}
	}()
	go func() { defer wg.Done()
		// 三个 itdog 请求串行，避免被限流
		pubV4 = fetchPubAndGeo("https://4.itdog.cn")
		pubV6 = fetchPubAndGeo("https://6.itdog.cn")
		pubPref = fetchPubAndGeo("https://v.itdog.cn")
	}()
	wg.Wait()

	var L []string
	L = append(L, "── 基本信息 ──")
	L = append(L, fmt.Sprintf("  活动接口 :  %s", orNA(iface)))
	L = append(L, fmt.Sprintf("  默认网关 :  %s", orNA(gw)))
	L = append(L, "")

	if detail.Name != "" {
		L = append(L, fmt.Sprintf("── 网卡 %s ──", detail.Name))
		if detail.Status != "" {
			L = append(L, fmt.Sprintf("  状态     :  %s", detail.Status))
		}
		if detail.MAC != "" {
			L = append(L, fmt.Sprintf("  MAC 地址 :  %s", detail.MAC))
		}
		if detail.MTU != "" {
			L = append(L, fmt.Sprintf("  MTU      :  %s", detail.MTU))
		}
		if detail.IPv4 != "" {
			L = append(L, fmt.Sprintf("  IPv4     :  %s", detail.IPv4))
			L = append(L, fmt.Sprintf("  子网掩码 :  %s", detail.Mask))
		}
		if detail.IPv6 != "" {
			L = append(L, fmt.Sprintf("  IPv6     :  %s", detail.IPv6))
		}
		if detail.HasStat {
			L = append(L, fmt.Sprintf("  收 (RX)  :  %s", FmtBytes(float64(detail.RxBytes))))
			L = append(L, fmt.Sprintf("  发 (TX)  :  %s", FmtBytes(float64(detail.TxBytes))))
		}
		L = append(L, "")
	}

	L = append(L, "── DNS ──")
	for i, sv := range dnsServers {
		if i >= 10 {
			break
		}
		L = append(L, fmt.Sprintf("    %s", sv))
	}
	if len(dnsServers) == 0 {
		L = append(L, "    (无)")
	}
	if dnsOK {
		L = append(L, fmt.Sprintf("  解析延迟 :  %.2f ms  (www.baidu.com)", dnsLatMs))
	} else {
		L = append(L, "  解析延迟 :  (失败)")
	}
	L = append(L, "")

	if gw != "" {
		L = append(L, "── 网关连通性 ──")
		if gwHealth.OK {
			L = append(L, fmt.Sprintf("  最小延迟 :  %.2f ms", gwHealth.LatencyMs))
			L = append(L, fmt.Sprintf("  丢包率   :  %.1f%%", gwHealth.LossPct))
		} else {
			L = append(L, "  网关不可达或无数据")
		}
		L = append(L, "")
	}

	L = append(L, "── 公网 IP  (via itdog.cn) ──")
	L = append(L, fmt.Sprintf("  IPv4  :  %s", orFail(pubV4.OK, pubV4.IP)))
	if pubV4.OK {
		L = append(L, fmt.Sprintf("           归属地: %s", orFailGeo(pubV4.Geo)))
	}
	L = append(L, fmt.Sprintf("  IPv6  :  %s", orFail(pubV6.OK, pubV6.IP)))
	if pubV6.OK {
		L = append(L, fmt.Sprintf("           归属地: %s", orFailGeo(pubV6.Geo)))
	}
	L = append(L, fmt.Sprintf("  优先版:  %s", orFail(pubPref.OK, pubPref.IP)))
	return L
}

func orFail(ok bool, v string) string {
	if ok && v != "" {
		return v
	}
	return "(获取失败)"
}

func orFailGeo(s string) string {
	if s == "" {
		return "(查询失败)"
	}
	return s
}

// ---- 并发各模块 ----

func collectIfaceDetail(iface string) ifaceDetail {
	d := ifaceDetail{Name: iface}
	if iface == "" {
		return d
	}
	switch runtime.GOOS {
	case "darwin":
		return ifaceDetailDarwin(iface)
	case "linux":
		return ifaceDetailLinux(iface)
	case "windows":
		return ifaceDetailWindows(iface)
	}
	return d
}

// ifaceDetailWindows 用 PowerShell Get-NetAdapter / Get-NetIPAddress /
// Get-NetIPInterface / Get-NetAdapterStatistics 获取单块网卡的详尽信息，
// 不再解析 ipconfig /all（中文版字段名为中文且输出含 GBK 字节）。
func ifaceDetailWindows(iface string) ifaceDetail {
	d := ifaceDetail{Name: iface}
	if iface == "" {
		return d
	}
	// PowerShell -Command 之后的额外参数不会绑定到脚本的 param()，
	// 这里把接口名（单引号包裹，内部单引号转义为 '')直接嵌入脚本。
	alias := strings.ReplaceAll(iface, "'", "''")
	const script = `$ErrorActionPreference = 'SilentlyContinue'
$a = Get-NetAdapter -Name '%s'
if (-not $a) { $a = Get-NetAdapter | Where-Object { $_.InterfaceDescription -eq '%s' } }
if (-not $a) { exit 1 }
$idx = $a.InterfaceIndex
$ip  = Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv4 | Where-Object { $_.PrefixOrigin -ne 'WellKnown' } | Select-Object -First 1
$nli = Get-NetIPInterface -InterfaceIndex $idx -AddressFamily IPv4
$ip6 = Get-NetIPAddress -InterfaceIndex $idx -AddressFamily IPv6   | Where-Object { $_.PrefixOrigin -eq 'Global' -or $_.PrefixOrigin -eq 'Dhcp' -or $_.PrefixOrigin -eq 'RouterAdvertisement' } | Select-Object -First 1
$stat = Get-NetAdapterStatistics -Name $a.Name
"ALIAS="   + $a.Name
"MAC="     + $a.MacAddress
"MTU="     + $nli.NlMtu
"STATUS="  + $a.Status
if ($ip)  { "IP="  + $ip.IPAddress  + "/" + $ip.PrefixLength }
if ($ip6) { "IP6=" + $ip6.IPAddress }
if ($stat) {
	"RX=" + $stat.ReceivedBytes
	"TX=" + $stat.OutboundBytes
}`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf(script, alias, alias)).Output()
	if err != nil {
		return d
	}
	for _, line := range strings.Split(decodeLocal(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "ALIAS="):
			d.Name = strings.TrimPrefix(line, "ALIAS=")
		case strings.HasPrefix(line, "MAC="):
			d.MAC = strings.TrimPrefix(line, "MAC=")
		case strings.HasPrefix(line, "MTU="):
			d.MTU = strings.TrimPrefix(line, "MTU=")
		case strings.HasPrefix(line, "STATUS="):
			st := strings.TrimPrefix(line, "STATUS=")
			d.Status = st
			if strings.EqualFold(st, "Up") {
				d.Status = "up"
			} else if strings.EqualFold(st, "Disabled") || strings.EqualFold(st, "Disconnected") {
				d.Status = "down"
			}
		case strings.HasPrefix(line, "IP="):
			ipstr := strings.TrimPrefix(line, "IP=")
			fields := strings.Split(ipstr, "/")
			d.IPv4 = fields[0]
			if len(fields) > 1 {
				d.Mask = cidrToMask(fields[1])
			}
		case strings.HasPrefix(line, "IP6="):
			d.IPv6 = strings.TrimPrefix(line, "IP6=")
		case strings.HasPrefix(line, "RX="):
			n, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "RX=")), 10, 64)
			d.RxBytes = n
			d.HasStat = true
		case strings.HasPrefix(line, "TX="):
			n, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "TX=")), 10, 64)
			d.TxBytes = n
			d.HasStat = true
		}
	}
	return d
}

func ifaceDetailDarwin(iface string) ifaceDetail {
	d := ifaceDetail{Name: iface}
	out, err := exec.Command("ifconfig", iface).Output()
	if err != nil {
		return d
	}
	s := string(out)
	if strings.Contains(s, "status: active") {
		d.Status = "active"
	} else if strings.Contains(s, "status: inactive") {
		d.Status = "inactive"
	}
	if m := regexp.MustCompile(`ether\s+([0-9a-fA-F:]{17})`).FindStringSubmatch(s); len(m) > 1 {
		d.MAC = m[1]
	}
	if m := regexp.MustCompile(`mtu\s+(\d+)`).FindStringSubmatch(s); len(m) > 1 {
		d.MTU = m[1]
	}
	if m := regexp.MustCompile(`inet\s+(\S+)\s+netmask\s+(\S+)`).FindStringSubmatch(s); len(m) > 1 {
		d.IPv4 = m[1]
		d.Mask = hexToDotted(m[2])
	}
	if m := regexp.MustCompile(`inet6\s+([^\s%]+)`).FindStringSubmatch(s); len(m) > 1 {
		d.IPv6 = m[1]
	}
	// 流量统计：netstat -bi 行解析
	d.RxBytes, d.TxBytes, d.HasStat = darwinIfStat(iface)
	return d
}

func darwinIfStat(iface string) (rx, tx int64, ok bool) {
	out, err := exec.Command("netstat", "-bi").Output()
	if err != nil {
		return 0, 0, false
	}
	// 表头形如: Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
	// 先扫一遍找到 Ibytes / Obytes 列下标，再按列对齐数据行
	lines := strings.Split(string(out), "\n")
	if len(lines) == 0 {
		return 0, 0, false
	}
	hdr := strings.Fields(lines[0])
	idxOf := func(name string) int {
		for i, h := range hdr {
			if h == name {
				return i
			}
		}
		return -1
	}
	ibIdx := idxOf("Ibytes")
	obIdx := idxOf("Obytes")
	if ibIdx < 0 || obIdx < 0 {
		return 0, 0, false
	}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) <= ibIdx || len(fields) <= obIdx {
			continue
		}
		if fields[0] != iface {
			continue
		}
		r, err1 := strconv.ParseInt(fields[ibIdx], 10, 64)
		t, err2 := strconv.ParseInt(fields[obIdx], 10, 64)
		if err1 == nil && err2 == nil {
			return r, t, true
		}
	}
	return 0, 0, false
}

func ifaceDetailLinux(iface string) ifaceDetail {
	d := ifaceDetail{Name: iface}
	out, err := exec.Command("ip", "addr", "show", iface).Output()
	if err == nil {
		s := string(out)
		if strings.Contains(s, "state UP") {
			d.Status = "up"
		} else if strings.Contains(s, "state DOWN") {
			d.Status = "down"
		}
		if m := regexp.MustCompile(`link/ether\s+([0-9a-fA-F:]{17})`).FindStringSubmatch(s); len(m) > 1 {
			d.MAC = m[1]
		}
		if m := regexp.MustCompile(`mtu\s+(\d+)`).FindStringSubmatch(s); len(m) > 1 {
			d.MTU = m[1]
		}
		if m := regexp.MustCompile(`inet\s+(\d+\.\d+\.\d+\.\d+)/(\d+)`).FindStringSubmatch(s); len(m) > 1 {
			d.IPv4 = m[1]
			d.Mask = cidrToMask(m[2])
		}
		if m := regexp.MustCompile(`inet6\s+(\S+)\s`).FindStringSubmatch(s); len(m) > 1 {
			d.IPv6 = m[1]
		}
	}
	// 流量统计：/proc/net/dev
	if data, err := os.ReadFile("/proc/net/dev"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) != 2 {
				continue
			}
			if strings.TrimSpace(fields[0]) != iface {
				continue
			}
			parts := strings.Fields(fields[1])
			if len(parts) >= 16 {
				rx, _ := strconv.ParseInt(parts[0], 10, 64)
				tx, _ := strconv.ParseInt(parts[8], 10, 64)
				d.RxBytes = rx
				d.TxBytes = tx
				d.HasStat = true
			}
		}
	}
	return d
}

func collectDNSServers() []string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("scutil", "--dns").Output()
		if err != nil {
			return nil
		}
		return uniqueSorted(regexp.MustCompile(`nameserver\[\d+\]\s*:\s*(\S+)`).FindAllStringSubmatch(string(out), -1))
	case "linux":
		out, err := exec.Command("resolvectl", "status").Output()
		if err == nil {
			servers := uniqueSorted(regexp.MustCompile(`\b(\d+\.\d+\.\d+\.\d+)\b`).FindAllStringSubmatch(string(out), -1))
			if len(servers) > 0 {
				return servers
			}
		}
		// 兜底读 /etc/resolv.conf
		if data, err := osReadFile("/etc/resolv.conf"); err == nil {
			return uniqueSorted(regexp.MustCompile(`^nameserver\s+(\S+)`).FindAllStringSubmatch(data, -1))
		}
		return nil
	case "windows":
		const script = `Get-DnsClientServerAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | ForEach-Object { $_.ServerAddresses } | Sort-Object -Unique`
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
		if err != nil {
			return nil
		}
		var servers []string
		seen := map[string]bool{}
		for _, line := range strings.Split(decodeLocal(out), "\n") {
			v := strings.TrimSpace(line)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			servers = append(servers, v)
		}
		return servers
	}
	return nil
}

// dnsResolveLatency 实测一次 hostname 解析耗时
func dnsResolveLatency(host string, timeout time.Duration) (ms float64, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	// 用 net 包的 Resolver，使用系统 DNS
	r := &stdnet.Resolver{}
	_, err := r.LookupHost(ctx, host)
	if err != nil {
		return 0, false
	}
	return float64(time.Since(start).Microseconds()) / 1000.0, true
}

// gatewayPingHealth 用系统 ping 快速测若干次，返回最小延迟与丢包率
func gatewayPingHealth(gw string, count int) gatewayHealth {
	if count <= 0 {
		count = 4
	}
	args := pingHealthArgs(count)
	args = append(args, gw)
	out, _ := commandCombinedOutput("ping", args...)
	// 即使有丢包 ping 也可能 exit 1 但输出可解析
	if out == "" {
		return gatewayHealth{}
	}
	s := out
	h := gatewayHealth{OK: true}
	// 看到任何 packets transmitted 即认为跑了
	if !strings.Contains(s, "packets transmitted") && !strings.Contains(s, "Packets:") && !strings.Contains(s, "已发送") {
		h.OK = false
	}

	// RTT: Linux/macOS: "rtt min/avg/max/mdev = 0.123/0.456/0.789/0.012 ms"
	if m := regexp.MustCompile(`(?:rtt|round-trip)\s+min/avg/max/[a-z]*\s*=\s*([\d.]+)/([\d.]+)/([\d.]+)`).FindStringSubmatch(s); len(m) > 1 {
		minV, _ := strconv.ParseFloat(m[1], 64)
		h.LatencyMs = minV
	}
	// RTT: Windows: "最短 = 0ms，最长 = 1ms，平均 = 0ms" / "Minimum = 0ms, Maximum = 1ms, Average = 0ms"
	if runtime.GOOS == "windows" {
		if m := regexp.MustCompile(`(?:最短|Minimum)\s*=\s*(\d+)\s*ms`).FindStringSubmatch(s); len(m) > 1 {
			v, _ := strconv.ParseFloat(m[1], 64)
			h.LatencyMs = v
		}
	}
	// Loss: Linux/macOS "4 packets transmitted, 4 received, 0% packet loss"
	if m := regexp.MustCompile(`(\d+(?:\.\d+)?)%\s*packet loss`).FindStringSubmatch(s); len(m) > 1 {
		pct, _ := strconv.ParseFloat(m[1], 64)
		h.LossPct = pct
	}
	// Windows: "(25% 丢失)" / "(25% loss)"
	if m := regexp.MustCompile(`\((\d+(?:\.\d+)?)%\s*(?:丢失|loss)\)`).FindStringSubmatch(s); len(m) > 1 {
		pct, _ := strconv.ParseFloat(m[1], 64)
		h.LossPct = pct
	}
	return h
}

func pingHealthArgs(count int) []string {
	if runtime.GOOS == "windows" {
		return []string{"-n", strconv.Itoa(count), "-w", "2000"}
	}
	if runtime.GOOS == "darwin" {
		// macOS 的 -W 是等待一个响应的超时（毫秒）
		return []string{"-c", strconv.Itoa(count), "-W", "2000"}
	}
	// Linux 的 -W 是超时（秒）
	return []string{"-c", strconv.Itoa(count), "-W", "2"}
}

// fetchPubAndGeo 从 endpoint 取一个公网 IP，并尝试用 ipinfo.io 查归属地
func fetchPubAndGeo(endpoint string) pubResult {
	c := stdhttp.Client{Timeout: 6 * time.Second}
	resp, err := c.Get(endpoint)
	if err != nil {
		return pubResult{}
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	ip := strings.TrimSpace(string(body))
	if ip == "" || !looksLikeIP(ip) {
		return pubResult{}
	}
	geo := fetchGeo(ip)
	return pubResult{IP: ip, Geo: geo, OK: true}
}

func fetchGeo(ip string) string {
	c := stdhttp.Client{Timeout: 6 * time.Second}
	resp, err := c.Get("https://ipinfo.io/" + ip + "/json")
	if err != nil {
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var j map[string]any
	if err := json.Unmarshal(body, &j); err != nil {
		return ""
	}
	get := func(k string) string {
		if v, ok := j[k].(string); ok {
			return v
		}
		return ""
	}
	parts := []string{}
	for _, k := range []string{"city", "region", "country"} {
		if v := get(k); v != "" {
			parts = append(parts, v)
		}
	}
	out := strings.Join(parts, " / ")
	if org := get("org"); org != "" {
		out += "  [" + org + "]"
	}
	return out
}

func looksLikeIP(s string) bool {
	return regexp.MustCompile(`^[\d.:a-fA-F]+$`).MatchString(s)
}

// 保留旧 helper
func orNA(s string) string {
	if s == "" {
		return "(未检出)"
	}
	return s
}

func hexToDotted(h string) string {
	h = strings.ToLower(strings.TrimPrefix(h, "0x"))
	for len(h) < 8 {
		h = "0" + h
	}
	b := make([]string, 4)
	for i := 0; i < 4; i++ {
		v, _ := strconv.ParseInt(h[i*2:i*2+2], 16, 0)
		b[i] = strconv.Itoa(int(v))
	}
	return strings.Join(b, ".")
}

func cidrToMask(cidr string) string {
	n, err := strconv.Atoi(cidr)
	if err != nil {
		return ""
	}
	mask := uint32(0xffffffff) << (32 - n)
	return fmt.Sprintf("%d.%d.%d.%d", mask>>24&0xff, mask>>16&0xff, mask>>8&0xff, mask&0xff)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func uniqueSorted(matches [][]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func osReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}