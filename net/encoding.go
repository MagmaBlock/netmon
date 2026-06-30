package net

import (
	"bytes"
	"io"
	"os/exec"
	"runtime"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Windows 中文系统下，ping / ipconfig / route print / PowerShell 等
// 进程的 stdout/stderr 都使用 OEM / ANSI 代码页（中文为 GBK / CP936），
// 直接按 UTF-8 处理会出现乱码。这里集中提供解码工具。

// decodeLocal 将系统命令的字节输出按本机代码页解码为 UTF-8。
// 在非 Windows 上原样返回（命令输出本身即 UTF-8 或 ASCII）。
func decodeLocal(b []byte) string {
	if runtime.GOOS != "windows" {
		return string(b)
	}
	// GBK 解码对纯 ASCII 是恒等的，所以无论内容是否含中文都可安全通过。
	r := transform.NewReader(bytes.NewReader(b), simplifiedchinese.GBK.NewDecoder())
	out, err := io.ReadAll(r)
	if err != nil || len(out) == 0 {
		return string(b)
	}
	return string(out)
}

// localReader 把命令的 stdout 流包装成 UTF-8 流（Windows 上转 GBK，
// 其它平台直接透传），用于流式读取 ping 进程输出。
func localReader(r io.Reader) io.Reader {
	if runtime.GOOS != "windows" {
		return r
	}
	return transform.NewReader(r, simplifiedchinese.GBK.NewDecoder())
}

// commandOutput 运行命令并把 stdout 解码为 UTF-8 后返回。
func commandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		// 即使非零退出码，stdout 通常仍有内容
		return decodeLocal(out), err
	}
	return decodeLocal(out), nil
}

// commandCombinedOutput 同 commandOutput，但同时捕获 stderr。
func commandCombinedOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return decodeLocal(out), err
}