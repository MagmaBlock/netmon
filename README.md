# netmon

> 跨平台网络监测 TUI —— 单二进制、零运行时依赖，用 Go + [bubbletea](https://github.com/charmbracelet/bubbletea) 重写自 Python/curses 版。

一个命令行下的网络排障与测速工具，把日常要敲的一串命令（`ping` / `nc` / `curl` / `ifconfig` / `route` / `scutil --dns` …）收进一个 TUI，方向键即可切换。

## 功能

| # | 功能        | 说明 |
|---|------------|------|
| 1 | ICMP Ping  | 调用系统 `ping`（免 root），单/多选**分屏**同时 ping 多目标，默认勾选 magma.ink / cloudflare / 网关 |
| 2 | TCP Ping   | 纯 Go `net.DialTimeout`，**无 nc 依赖**，跨平台一致 |
| 3 | 下载测速    | 纯 Go `net/http`，**不落盘**，实时进度条 / 速度 / ETA，`q` 中止 |
| 4 | Ping 网关  | 自动取默认网关后走 ICMP |
| 5 | 网络信息    | 网卡 / DNS / 网关连通性 / 公网 IP，**面向排障与质量**（详见下） |
| 6 | 打开网页    | 平台默认浏览器（`open` / `xdg-open` / `start`） |

### 网络信息视图覆盖

- **基本信息**：活动接口、默认网关
- **网卡**：状态（active/up/down）、MAC、MTU、IPv4、子网掩码、IPv6、RX/TX 流量统计
- **DNS**：服务器列表 + 实测一次 `www.baidu.com` 解析延迟（毫秒）
- **网关连通性**：4 次 ping 默认网关，提取最小延迟 + 丢包率
- **公网 IP**：via [itdog.cn](https://itdog.cn)
  - IPv4：`4.itdog.cn` / IPv6：`6.itdog.cn` / 优先版：`v.itdog.cn`
  - 归属地：再请求 `ipinfo.io`，显示 `城市 / 省 / 国家  [ASN 运营商]`

所有耗时项（网卡 / DNS / DNS 延迟 / 网关 ping / 公网 IP）**并发收集**，三个 itdog 串行以避免被限流。

## 平台

| 平台 | GOOS | GOARCH | 备注 |
|------|------|--------|------|
| macOS | darwin  | arm64 / amd64 | 当前开发机 |
| Linux | linux   | amd64 / arm64 | 服务器 / 桌面 / 树莓派 |
| Windows | windows | amd64 | Win10+ 终端推荐 |

ICMP 走系统 `ping` 命令（无需 root）；TCP/测速/公网IP 走纯 Go，单二进制开箱即用。

## 编译

```bash
go build -o dist/netmon . && ./dist/netmon
```

交叉编译：

```bash
mkdir -p dist
for t in "darwin arm64" "darwin amd64" "linux amd64" "linux arm64" "windows amd64"; do
  set -- $t
  ext=""; [ "$1" = "windows" ] && ext=".exe"
  GOOS=$1 GOARCH=$2 go build -o "dist/netmon-$1-$2$ext" .
done
```

## 使用

```bash
./netmon          # 进入主菜单
./netmon 1        # 快捷直达 ICMP Ping（默认目标）
./netmon 2        #   TCP Ping
./netmon 3        #   下载测速
./netmon 4        #   Ping 网关
./netmon 5        #   网络信息
./netmon 6        #   打开网页（在浏览器打开首项）
```

### 通用按键

- `↑↓` / `j k`：上下选择
- `Space`：多选切换（a 全选 / n 全不选）
- `Enter`：确认
- `q` / `Esc`：返回 / 退出
- `Ctrl+C`：强制退出

## 项目结构

```
netmon/
├── go.mod / go.sum
├── main.go              # 入口、主菜单编排、快捷直达参数 1-6
├── net/                  # 网络逻辑（平台分派）
│   ├── ping.go          # ICMP（系统 ping）+ TCP（纯 Go net.Dial）
│   ├── gateway.go       # 取默认网关（darwin/linux/windows）
│   ├── netinfo.go       # 网络信息收集（并发）
│   ├── speedtest.go     # 下载测速（net/http 流式 + 实时计量）
│   └── open.go          # 打开浏览器（平台分派）
├── tui/                  # bubbletea 视图层
│   ├── model.go         # 顶层 Model + 状态机路由
│   ├── menu.go / multiselect.go / ping.go
│   ├── speedselect.go / speed.go / text.go / web.go
│   ├── style.go / layout.go / screen.go
├── netmon.py             # 原 Python/curses 参考实现
├── PLAN.md               # 开发计划
└── dist/                 # 编译产物（gitignored）
```

## 开发

- Go 1.26+
- 国内网络环境如拉取依赖失败：`go env -w GOPROXY=https://goproxy.cn,direct`
- 见 [PLAN.md](PLAN.md) 了解设计取舍与开发阶段

## License

MIT