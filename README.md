# netmon

跨平台网络监测 TUI 工具（Go + bubbletea）。

## 功能

- ICMP Ping（单/多选分屏，默认勾选 magma.ink / cloudflare / 网关）
- TCP Ping（纯 Go，无 nc 依赖）
- 下载测速（实时速度/进度条，不落盘）
- Ping 网关
- 网络信息（IP / 子网掩码 / 网关 / DHCP / DNS / 公网 IP）
- 快捷打开测速网页

## 平台

macOS / Linux / Windows，编译为零依赖单二进制。

## 开发

见 [PLAN.md](PLAN.md)。

## 编译

```bash
go build -o dist/netmon && ./dist/netmon
```

交叉编译见 PLAN.md「编译与发布」。
