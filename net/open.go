package net

import (
	"os/exec"
	"runtime"
)

// OpenURL 在平台默认浏览器打开 url
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start()
	}
	return exec.Command("open", url).Start()
}