# netmon — 跨平台网络监测工具 开发计划

## 目标

用 **Go + bubbletea** 重写现有 Python/curses 版 `netmon`，编译为**零依赖单二进制**，
覆盖 macOS / Linux / Windows 三大系统。保留现有全部功能与交互体验。

## 现状

- 当前实现：`netmon.py`（Python 3 + curses TUI），仅 macOS
- 位置：`~/.local/bin/netmon`（运行时）
- 功能：ICMP/TCP ping、分屏多 ping、下载测速（实时）、网关 ping、网络信息、打开网页

## 技术栈

- 语言：Go 1.26（已装 `brew install go`）
- TUI：`github.com/charmbracelet/bubbletea` + `bubbles` + `lipgloss`（样式）
- 模块：`go mod init github.com/magma/netmon`
- 依赖：bubbletea 系 + `golang.org/x/net/icmp`（可选，ICMP）

## 目标平台 / 编译矩阵

| 平台 | GOOS | GOARCH | 说明 |
|------|------|--------|------|
| macOS arm64 | darwin | arm64 | 当前开发机 |
| Linux amd64 | linux | amd64 | 服务器/桌面 |
| Linux arm64 | linux | arm64 | 树莓派/ARM |
| Windows amd64 | windows | amd64 | Win 桌面 |

编译命令（交叉编译，无需目标机）：
```bash
GOOS=darwin  GOARCH=arm64 go build -o dist/netmon-darwin-arm64
GOOS=linux   GOARCH=amd64 go build -o dist/netmon-linux-amd64
GOOS=linux   GOARCH=arm64 go build -o dist/netmon-linux-arm64
GOOS=windows GOARCH=amd64 go build -o dist/netmon-windows-amd64.exe
```

## 功能清单与跨平台实现策略

### 1. ICMP Ping
- **策略**：调用系统 `ping` 命令（避免 raw socket 的 root 要求）
- **平台差异**：
  - macOS/Linux：`ping <host>`（无限），输出 `64 bytes from ... time=X ms`
  - Windows：`ping -t <host>`（无限），输出 `来自 ... 时间=Xms`
- **实现**：`os/exec` 起进程，按行读 stdout，正则提取时间，推送结果行
- **多 ping 分屏**：每个目标一个 goroutine + channel，复用现有分屏布局逻辑

### 2. TCP Ping
- **策略**：纯 Go `net.DialTimeout`，**不依赖 nc**（三平台统一）
- **实现**：循环 `net.DialTimeout("tcp", host:port, 2s)`，计时毫秒，间隔 1s
- 无需任何外部命令，Windows 也能用

### 3. 下载测速（实时）
- **策略**：纯 Go `net/http`，**不依赖 curl**
- **实现**：
  - `http.Head` 或首字节读响应头取 `Content-Length`（总大小）
  - `io.Copy` 到 `io.Discard`（不落盘），用自定义 `Reader` 包装 `resp.Body`，每 N ms 统计已读字节数
  - 实时计算：已下载 / 进度条 / 实时速度 / 平均速度 / 已用时间 / ETA
  - `Ctrl+C` 或 q 中止：`context.Cancel` + 关闭 Body
- 优势：无临时文件、无磁盘占用、跨平台

### 4. Ping 网关
- **策略**：先取默认网关，再走 ICMP ping
- **平台差异（取网关）**：
  - macOS：`route -n get default` → 解析 `gateway:`
  - Linux：`ip route` → 解析 `default via X`，或 `route -n` 兜底
  - Windows：优先 `Get-NetRoute -DestinationPrefix 0.0.0.0/0 | Sort RouteMetric`（含 metric 选最低，避开 Radmin VPN/Tailscale 等虚拟网卡注册的 0.0.0.0 抢先匹配）；`route print` 按 metric 兜底
- **实现**：平台分派函数 `defaultGateway() (ip, iface string, err error)`

### 5. 网络信息（排障与质量向）
- **公网 IP**：走 itdog 三端点
  - IPv4：`curl 4.itdog.cn` / IPv6：`curl 6.itdog.cn` / 优先版：`curl v.itdog.cn`
  - 归属地：再请求 `ipinfo.io/<ip>/json`，拼接 `city / region / country  [org]`
- **网卡详情**：状态(active/up/down)、MAC、MTU、IPv4、子网掩码、IPv6、RX/TX 字节流量
  - macOS：`ifconfig` + `netstat -bi`（按表头列名定位 Ibytes/Obytes）
  - Linux：`ip addr show` + `/proc/net/dev`
- **DNS**：服务器列表 + 实测一次 `www.baidu.com` 解析延迟（毫秒）
  - macOS：`scutil --dns` / Linux：`resolvectl status` 兜底 `/etc/resolv.conf` / Windows：`Get-DnsClientServerAddress`
- **网关连通性**：对默认网关跑 4 次 ping，提取最小延迟与丢包率（正则适配 macOS `round-trip`、Linux `rtt`、Windows 中英文）
- **并发收集**：网卡详情 / DNS / DNS延迟 / 网关ping / 公网IP 五路 goroutine 并发；三个 itdog 请求串行避免限流
- **平台差异最大**，按平台分派收集：
  - **macOS**：`ifconfig`、`scutil --dns`、`netstat -bi`
  - **Linux**：`ip addr`、`resolvectl status` 或 `/etc/resolv.conf`、`/proc/net/dev`
  - **Windows**：`Get-NetAdapter` / `Get-NetIPAddress` / `Get-NetIPInterface` / `Get-NetAdapterStatistics`（PowerShell cmdlet，输出 UTF-8，含网卡状态/MAC/MTU/IPv4+前缀/IPv6/RX/TX）
- **子网掩码**：macOS ifconfig 返回十六进制 `0xffffff00` → 转点分十进制；Linux 由 CIDR 转换；Windows 由 `Get-NetIPAddress` PrefixLength 按 CIDR 转换
- **Windows 中文编码**：`ping` / `ipconfig` / `route print` 等进程在中文系统使用 GBK（CP936）代码页输出，直接按 UTF-8 处理会乱码；统一用 `golang.org/x/text/encoding/simplifiedchinese.GBK` 解码 stdout（流式与批量两种入口），中文字段（如「时间」「请求超时」「已发送」「丢失」）正则在解码后的 UTF-8 上匹配
- **实现**：`collectNetworkInfo() []string`，内部 `switch runtime.GOOS`

### 6. 打开网页
- **平台差异**：
  - macOS：`open <url>`
  - Linux：`xdg-open <url>`
  - Windows：`exec.Command("cmd", "/c", "start", url)`
- **实现**：`openURL(url)` 分派

## 架构设计

```
netmon/
├── go.mod
├── go.sum
├── main.go                 # 入口 + 菜单编排
├── tui/
│   ├── menu.go             # 主菜单/多选菜单组件
│   ├── pingview.go         # 单 ping / 分屏多 ping 视图
│   ├── speedview.go        # 测速实时视图（进度条/速度）
│   ├── textview.go         # 网络信息等纯文本滚动视图
│   └── style.go            # lipgloss 样式/颜色
├── net/
│   ├── ping.go             # ICMP ping（系统命令分派）
│   ├── tpping.go           # TCP ping（纯 Go net.Dial）
│   ├── gateway.go          # 取默认网关（平台分派）
│   ├── netinfo.go          # 网络信息收集（平台分派）
│   ├── speedtest.go        # 下载测速（net/http 实时）
│   └── open.go             # 打开网页（平台分派）
├── dist/                   # 编译产物（gitignore）
└── netmon.py               # 参考（原 Python 版）
```

### bubbletea 架构要点
- 每个视图是一个 `tea.Model`（实现 Init/Update/View）
- 主菜单 → 子菜单 → 视图，用 `tea.Model` 切换（返回下一个 model）
- ping 结果：goroutine 通过 `tea.Program.Send` 发送 `resultMsg` 到主循环更新
- 测速：goroutine 定时 Send 进度消息，View 渲染进度条
- 分屏：lipgloss 并排布局多个子块，各 ping goroutine 独立 Send

### 平台分派模式
```go
// net/gateway.go
func defaultGateway() (ip, iface string, err error) {
    switch runtime.GOOS {
    case "darwin":
        return gatewayDarwin()
    case "linux":
        return gatewayLinux()
    case "windows":
        return gatewayWindows()
    default:
        return "", "", fmt.Errorf("unsupported platform")
    }
}
```
可选：用 `//go:build darwin` 构建标签拆文件，避免 switch 过长。初版先用 switch，后续重构。

## 开发阶段

### 阶段 1：骨架 + 主菜单
- `go mod init`，装 bubbletea
- 主菜单 model（方向键导航），功能项占位
- lipgloss 样式（标题栏、选中高亮、颜色）
- **验收**：能跑，菜单能上下选、Enter 进占位视图、q 返回

### 阶段 2：ICMP Ping（单 + 多选分屏）
- `net/ping.go`：系统 ping 分派（先做 macOS，预留 Linux/Windows）
- 单 ping 视图（流式输出）
- 多选菜单（Space 勾选、a 全选、默认勾选）
- 分屏多 ping 视图（goroutine + Send）
- 网关选项注入 ICMP 菜单 + 默认勾选 magma.ink/cloudflare/网关
- **验收**：对齐 Python 版的 ICMP 体验

### 阶段 3：TCP Ping
- `net/tpping.go`：纯 Go `net.DialTimeout` 循环
- 复用 ping 视图/分屏（ICMP/TCP 共用视图，只换数据源）
- **验收**：TCP ping 单/多均正常，无 nc 依赖

### 阶段 4：下载测速（实时）
- `net/speedtest.go`：net/http + 包装 Reader 实时统计
- 测速视图：进度条、实时/平均速度、ETA
- q 中止（context cancel）
- **验收**：实时速度跳动正常，中止无残留

### 阶段 5：网关 Ping + 网络信息
- `net/gateway.go`：三平台取网关
- `net/netinfo.go`：三平台收集网络信息
- 网络信息文本滚动视图
- **验收**：三平台信息正确（macOS 先验，Linux/Windows 待跨平台环境验）

### 阶段 6：打开网页 + 收尾
- `net/open.go`：三平台打开
- 主菜单"打开网页"子菜单
- 全功能对照 Python 版走查

### 阶段 7：跨平台验证 + 编译
- 完善 Linux/Windows 平台分派（命令解析）
- 交叉编译 4 个二进制到 `dist/`
- `.gitignore`（dist/、二进制）
- README（编译/使用说明）
- **验收**：4 个二进制产出；macOS 实测通过；Linux/Windows 逻辑走查（条件允许则实测）

## 编译与发布

```bash
# 本机快速编译验证
go build -o dist/netmon && ./dist/netmon

# 交叉编译全部目标
mkdir -p dist
for t in "darwin arm64" "linux amd64" "linux arm64" "windows amd64"; do
  set -- $t
  ext=""; [ "$1" = "windows" ] && ext=".exe"
  GOOS=$1 GOARCH=$2 go build -o "dist/netmon-$1-$2$ext"
done
```

## 注意事项

- **ICMP 无 root**：用系统 ping 命令规避；若后续要纯 Go ICMP，`golang.org/x/net/icmp` 在 macOS/Linux 需 unprivileged ICMP（系统开启），Windows 支持有限——暂不做
- **Windows 终端**：bubbletea 支持 Windows 终端（Windows Terminal/ConEmu），cmd.exe 旧版可能颜色异常
- **子网掩码**：macOS 十六进制需转换，Linux/Windows 直接点分
- **Linux DNS**：systemd 系统 `resolvectl`，老系统 `/etc/resolv.conf`，都要兜底
- **中文宽度**：lipgloss 自带宽字符处理，不用手写 wcwidth

## 开发约定

- 先 macOS 跑通，再补 Linux/Windows 分派
- 每阶段可独立验证
- 保留 `netmon.py` 作功能对照参考
- 最终二进制覆盖 `~/.local/bin/netmon`（macOS）
