package net

import (
	"context"
	"fmt"
	"io"
	stdnet "net/http"
	"time"
)

// Progress 是测速过程中的进度快照
type Progress struct {
	Total       int64
	Current     int64
	Start       time.Time
	SampleStart time.Time // 本次采样起点
	SampleBytes int64     // 本次采样新增字节
	Done        bool
}

// Speedtest 下载 url（不落盘），tickMs 周期发送 Progress 到 ch，直到完成或 ctx 取消。
// 退出时 close(ch)。
func Speedtest(ctx context.Context, url string, tickMs int, ch chan<- Progress) {
	defer close(ch)
	start := time.Now()

	// HEAD 尝试拿 Content-Length
	var total int64
	c := stdnet.Client{Timeout: 15 * time.Second}
	if hreq, err := stdnet.NewRequestWithContext(ctx, "HEAD", url, nil); err == nil {
		if hr, err := c.Do(hreq); err == nil {
			total = hr.ContentLength
			hr.Body.Close()
		}
	}

	// GET 流式
	dl := stdnet.Client{}
	req, err := stdnet.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	resp, err := dl.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if total <= 0 {
		total = resp.ContentLength
	}

	reader := &countingReader{src: resp.Body}
	ticker := time.NewTicker(time.Duration(tickMs) * time.Millisecond)
	defer ticker.Stop()

	sampleStart := start
	sampleBytes := int64(0)

	// 把 read 放进 goroutine，主循环靠 ticker/ticker 推送进度
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, reader)
		close(done)
	}()

	snap := func() Progress {
		now := time.Now()
		cur := reader.n
		delta := cur - sampleBytes
		p := Progress{
			Total:       total,
			Current:     cur,
			Start:       start,
			SampleStart: sampleStart,
			SampleBytes: delta,
		}
		sampleStart = now
		sampleBytes = cur
		return p
	}

	for {
		select {
		case <-ctx.Done():
			ch <- Progress{Total: total, Current: reader.n, Start: start, SampleStart: sampleStart, SampleBytes: reader.n - sampleBytes, Done: true}
			return
		case <-done:
			ch <- Progress{Total: total, Current: reader.n, Start: start, SampleStart: sampleStart, SampleBytes: reader.n - sampleBytes, Done: true}
			return
		case <-ticker.C:
			ch <- snap()
		}
	}
}

type countingReader struct {
	src io.Reader
	n   int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.src.Read(p)
	c.n += int64(n)
	return n, err
}

// FmtBytes 将字节数格式化为人类可读单位
func FmtBytes(n float64) string {
	for _, u := range []string{"B", "KB", "MB", "GB", "TB"} {
		if n < 1024 {
			return fmt.Sprintf("%.2f %s", n, u)
		}
		n /= 1024
	}
	return fmt.Sprintf("%.2f PB", n)
}

func FmtSpeed(bps float64) string {
	return FmtBytes(bps) + "/s"
}

func FmtTime(s float64) string {
	if s < 0 {
		return "--:--:--"
	}
	sec := int(s)
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
}

// Elapsed 返回从开始到现在经过的时间
func (p Progress) Elapsed() time.Duration {
	if p.Start.IsZero() {
		return 0
	}
	return time.Since(p.Start)
}

// LastSampleDur 返回距上一次采样的时间间隔
func (p Progress) LastSampleDur() time.Duration {
	if p.SampleStart.IsZero() {
		return 0
	}
	return time.Since(p.SampleStart)
}