package net

import (
	"bufio"
	"context"
	"fmt"
	"io"
	stdnet "net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// PingResult 是一次 ping 的输出行
type PingResult struct {
	Line string
}

// ICMPLine 解析后的结构化单行结果
type ICMPLine struct {
	Raw    string
	MS     float64
	OK     bool
	HasMS  bool
}

var (
	// macOS/Linux: "64 bytes from ... time=12.3 ms" / "time=12.3 ms"
	reTimeML = regexp.MustCompile(`time[=<]([\d.]+)\s*ms`)
	// Windows: "时间=12ms" / "时间=12ms" (中文) / "time=12ms" (英文)
	reTimeWin = regexp.MustCompile(`(?i)(?:时间|time)[=<](\d+)\s*ms`)
)

// Ping 启动系统 ping 流式读取，将原始行通过 ch 发出，ctx 控制停止。
// 返回的 error 仅表示启动失败。
func Ping(ctx context.Context, host string, ch chan string) error {
	args := pingArgs(host)
	cmd := exec.CommandContext(ctx, "ping", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 ping 失败: %w", err)
	}
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(localReader(stdout))
		sc.Buffer(make([]byte, 0, 4096), 64*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- line:
			}
		}
		_ = cmd.Wait()
	}()
	return nil
}

func pingArgs(host string) []string {
	if runtime.GOOS == "windows" {
		return []string{"-t", host}
	}
	return []string{host}
}

// ParseICMPLine 解析一行 ping 输出为结构化结果
func ParseICMPLine(line string) ICMPLine {
	r := ICMPLine{Raw: line}
	if runtime.GOOS == "windows" {
		if m := reTimeWin.FindStringSubmatch(line); len(m) > 1 {
			fmt.Sscanf(m[1], "%f", &r.MS)
			r.OK = true
			r.HasMS = true
		}
		if strings.Contains(line, "请求超时") || strings.Contains(line, "Request timed out") {
			r.HasMS = false
		}
	} else {
		if m := reTimeML.FindStringSubmatch(line); len(m) > 1 {
			fmt.Sscanf(m[1], "%f", &r.MS)
			r.OK = true
			r.HasMS = true
		}
		if strings.Contains(line, "Request timeout") || strings.Contains(line, " Destination Host Unreachable") {
			r.HasMS = false
		}
	}
	return r
}

// TCPPing 持续对 host:port 做 TCP 连通性测试，结果通过 ch 发出，直到 ctx 取消。
func TCPPing(ctx context.Context, host string, port int, ch chan string) {
	addr := fmt.Sprintf("%s:%d", host, port)
	seq := 0
	for {
		seq++
		select {
		case <-ctx.Done():
			close(ch)
			return
		default:
		}
		start := time.Now()
		var ok bool
		d := stdnet.Dialer{Timeout: 2 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", addr)
		ms := time.Since(start).Seconds() * 1000
		if err == nil {
			ok = true
			_ = conn.Close()
		}
		ts := time.Now().Format("15:04:05")
		var line string
		if ok {
			line = fmt.Sprintf(" [%s] seq=%-4d %s:%d   连通   %6.0f ms", ts, seq, host, port, ms)
		} else {
			line = fmt.Sprintf(" [%s] seq=%-4d %s:%d   超时/失败", ts, seq, host, port)
		}
		select {
		case <-ctx.Done():
			close(ch)
			return
		case ch <- line:
		}
		// 等待 1 秒（可被打断）
		select {
		case <-ctx.Done():
			close(ch)
			return
		case <-time.After(time.Second):
		}
	}
}

// Stop 用于 io.Discard 占位，避免未使用 import
var _ = io.Discard