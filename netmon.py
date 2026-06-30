#!/usr/bin/env python3
# netmon — 网络监测 TUI (macOS, curses)
# 用法: netmon            进入主菜单
#       netmon <1-6>      快捷直达 (1=ICMP 2=TCP 3=测速 4=网关 5=网络信息 6=网页)

import curses, subprocess, threading, time, os, re, sys, queue
import unicodedata, tempfile, shutil, locale

# ============== 配置 ==============
PING_HOSTS = [
    ("magma.ink", "默认"),
    ("baidu.com", ""),
    ("cloudflare.com", ""),
    ("github.com", ""),
    ("google.com", ""),
]
SPEED_ITEMS = [
    ("原神 5.5.0",
     "https://autopatchcn.yuanshen.com/client_app/download/pc_zip/20250314105313_OcRjEyGXX8Txtqm4/YuanShen_5.5.0.zip.001"),
    ("星穹铁道 4.3.0",
     "https://autopatchcn.bhsr.com/client/cn/20260523104353_kjwMxQcpFWHse2S2/PC/download/StarRail_4.3.0.7z.007"),
]
WEB_SITES = [
    ("itdog.cn 本地测速", "https://www.itdog.cn/localhost/"),
    ("ip138.com",         "https://www.ip138.com/"),
]
TCP_PORT = 443

# 颜色对
C_TITLE, C_SEL, C_HL, C_OK, C_WARN, C_ERR, C_DIM, C_ACC = range(1, 9)

def init_colors():
    curses.start_color()
    try:
        curses.use_default_colors()
        bg = -1
    except Exception:
        bg = curses.COLOR_BLACK
    curses.init_pair(C_TITLE, curses.COLOR_CYAN,    bg)
    curses.init_pair(C_SEL,   curses.COLOR_BLACK,   curses.COLOR_CYAN)
    curses.init_pair(C_HL,    curses.COLOR_YELLOW,  bg)
    curses.init_pair(C_OK,    curses.COLOR_GREEN,   bg)
    curses.init_pair(C_WARN,  curses.COLOR_YELLOW,  bg)
    curses.init_pair(C_ERR,   curses.COLOR_RED,     bg)
    curses.init_pair(C_DIM,   curses.COLOR_WHITE,   bg)
    curses.init_pair(C_ACC,   curses.COLOR_MAGENTA, bg)

# ============== 工具 ==============
def w_len(s):
    n = 0
    for c in s:
        n += 2 if unicodedata.east_asian_width(c) in ("W", "F") else 1
    return n

def w_trunc(s, maxw):
    if maxw <= 0:
        return ""
    n = 0
    out = ""
    for c in s:
        cw = 2 if unicodedata.east_asian_width(c) in ("W", "F") else 1
        if n + cw > maxw:
            break
        out += c
        n += cw
    return out

def pad_to(s, width):
    s = w_trunc(s, width)
    return s + " " * (width - w_len(s))

def safe_addstr(win, y, x, s, attr=0):
    h, w = win.getmaxyx()
    if y < 0 or y >= h or x < 0 or x >= w:
        return
    room = w - x - (1 if y == h - 1 else 0)
    if room <= 0:
        return
    s = w_trunc(s, room)
    try:
        win.addstr(y, x, s, attr)
    except curses.error:
        pass

def header(stdscr, title, subtitle=""):
    h, w = stdscr.getmaxyx()
    safe_addstr(stdscr, 0, 1, title, curses.color_pair(C_TITLE) | curses.A_BOLD)
    if subtitle:
        safe_addstr(stdscr, 0, max(1, w - w_len(subtitle) - 1), subtitle, curses.color_pair(C_DIM))
    safe_addstr(stdscr, 1, 0, "─" * w, curses.color_pair(C_DIM))

def footer_line(stdscr, text):
    h, w = stdscr.getmaxyx()
    safe_addstr(stdscr, h - 1, 0, " " + w_trunc(text, w - 2), curses.color_pair(C_DIM))

def human_bytes(n):
    if n is None:
        return "-"
    n = float(n)
    for u in ("B", "KB", "MB", "GB", "TB"):
        if n < 1024:
            return f"{n:.2f} {u}"
        n /= 1024
    return f"{n:.2f} PB"

def human_speed(bps):
    return human_bytes(bps) + "/s"

def fmt_time(s):
    if s is None or s < 0:
        return "--:--:--"
    s = int(s)
    return f"{s // 3600:02d}:{s % 3600 // 60:02d}:{s % 60:02d}"

def hex_to_dotted(h):
    h = h.lower().replace("0x", "")
    h = h.zfill(8)
    return f"{int(h[0:2],16)}.{int(h[2:4],16)}.{int(h[4:6],16)}.{int(h[6:8],16)}"

def progress_bar(width, ratio):
    ratio = max(0.0, min(1.0, ratio))
    filled = int(ratio * width)
    return "█" * filled + "░" * (width - filled)

# ============== 网络信息 ==============
def get_default_route():
    try:
        out = subprocess.check_output(["route", "-n", "get", "default"],
                                      text=True, stderr=subprocess.DEVNULL, timeout=3)
        gw = re.search(r"gateway:\s*(\S+)", out)
        iface = re.search(r"interface:\s*(\S+)", out)
        return (gw.group(1) if gw else None,
                iface.group(1) if iface else None)
    except Exception:
        return None, None

def get_content_length(url):
    try:
        out = subprocess.check_output(["curl", "-sI", "-L", "--max-time", "10", url],
                                      text=True, timeout=15)
        matches = re.findall(r"(?i)content-length:\s*(\d+)", out)
        if matches:
            return int(matches[-1])
    except Exception:
        pass
    return 0

def collect_network_info():
    L = []
    gw, iface = get_default_route()
    L.append(f"  活动接口 :  {iface or '(未检出)'}")
    L.append(f"  默认网关 :  {gw or '(未检出)'}")
    L.append("")

    # IP / 掩码
    if iface:
        try:
            out = subprocess.check_output(["ifconfig", iface], text=True, timeout=3)
            m = re.search(r"inet\s+(\S+)\s+netmask\s+(\S+)", out)
            if m:
                L.append(f"  IP 地址  :  {m.group(1)}")
                L.append(f"  子网掩码 :  {hex_to_dotted(m.group(2))}")
            else:
                L.append("  IP 地址  :  (未获取)")
        except Exception as e:
            L.append(f"  IP 地址  :  (读取失败: {e})")
    L.append("")

    # DHCP
    if iface:
        try:
            pkt = subprocess.check_output(["ipconfig", "getpacket", iface],
                                          text=True, stderr=subprocess.DEVNULL, timeout=3)
            sid = re.search(r"server_identifier\s*\(ip\):\s*(\S+)", pkt)
            lease = re.search(r"lease_time\s*\(uint32\):\s*(\S+)", pkt)
            router = re.search(r"router\s*\(ip_mult\):\s*(\S+)", pkt)
            dns_dhcp = re.search(r"domain_name_server\s*\(ip_mult\):\s*(\S+)", pkt)
            L.append("  DHCP :")
            L.append(f"    服务器 :  {sid.group(1) if sid else '-'}")
            if lease:
                v = lease.group(1)
                sec = int(v, 0) if v.startswith("0x") else int(v)
                L.append(f"    租约   :  {sec}s ({sec // 60} 分)")
            L.append(f"    路由器 :  {router.group(1).strip('{}') if router else '-'}")
            if dns_dhcp:
                L.append(f"    DNS    :  {dns_dhcp.group(1).strip('{}')}")
        except Exception:
            L.append("  DHCP :  (无信息，可能为静态配置)")
    L.append("")

    # DNS (scutil)
    try:
        dns = subprocess.check_output(["scutil", "--dns"], text=True, timeout=3)
        servers = sorted(set(re.findall(r"nameserver\[\d+\]\s*:\s*(\S+)", dns)))
        L.append("  DNS :")
        for s in servers[:10]:
            L.append(f"    {s}")
        if not servers:
            L.append("    (无)")
    except Exception:
        L.append("  DNS :  (读取失败)")
    L.append("")

    # 公网 IP
    L.append("  公网 IP :")
    for svc in ("https://ifconfig.me", "https://api.ipify.org"):
        try:
            pub = subprocess.check_output(["curl", "-s", "--max-time", "5", svc],
                                          text=True, timeout=8).strip()
            if pub and re.match(r"^[\d.:a-fA-F]+$", pub):
                L.append(f"    {pub}")
                break
        except Exception:
            continue
    else:
        L.append("    (获取失败)")
    return L

# ============== 通用视图 ==============
def menu_widget(stdscr, title, items, subtitle=""):
    h, w = stdscr.getmaxyx()
    sel = 0
    n = len(items)
    stdscr.timeout(-1)
    while True:
        stdscr.erase()
        header(stdscr, title, subtitle)
        top = 3
        avail = h - top - 1
        # 窗口内滚动
        if sel < 0:
            sel = 0
        if sel >= n:
            sel = n - 1
        first = max(0, sel - avail // 2)
        if first + avail > n:
            first = max(0, n - avail)
        for i in range(first, min(n, first + avail)):
            row = top + (i - first)
            if row >= h - 1:
                break
            is_sel = (i == sel)
            label = ("▶ " if is_sel else "  ") + items[i]
            attr = curses.color_pair(C_SEL) | curses.A_BOLD if is_sel else curses.color_pair(C_DIM)
            safe_addstr(stdscr, row, 2, pad_to(label, w - 4), attr)
        footer_line(stdscr, " ↑↓/j k 选择   Enter 确认   q/Esc 返回")
        stdscr.refresh()
        key = stdscr.getch()
        if key in (curses.KEY_UP, ord("k"), ord("K")):
            sel = (sel - 1) % n
        elif key in (curses.KEY_DOWN, ord("j"), ord("J")):
            sel = (sel + 1) % n
        elif key in (curses.KEY_ENTER, 10, 13):
            return sel
        elif key in (ord("q"), ord("Q"), 27):
            return -1

def multi_select_menu(stdscr, title, items, subtitle="", default_checked=None):
    """多选菜单: Space 切换选中, Enter 返回选中索引列表, q 取消返回 []"""
    h, w = stdscr.getmaxyx()
    sel = 0
    n = len(items)
    if default_checked is None:
        checked = [False] * n
    else:
        checked = list(default_checked)[:n]
        while len(checked) < n:
            checked.append(False)
    stdscr.timeout(-1)
    while True:
        stdscr.erase()
        header(stdscr, title, subtitle)
        top = 3
        avail = h - top - 1
        if sel < 0:
            sel = 0
        if sel >= n:
            sel = n - 1
        first = max(0, sel - avail // 2)
        if first + avail > n:
            first = max(0, n - avail)
        for i in range(first, min(n, first + avail)):
            row = top + (i - first)
            if row >= h - 1:
                break
            is_sel = (i == sel)
            mark = "☑" if checked[i] else "☐"
            label = ("▶ " if is_sel else "  ") + mark + " " + items[i]
            attr = curses.color_pair(C_SEL) | curses.A_BOLD if is_sel else curses.color_pair(C_DIM)
            safe_addstr(stdscr, row, 2, pad_to(label, w - 4), attr)
        cnt = sum(checked)
        footer_line(stdscr, f" Space 切换选中   a 全选   n 全不选   Enter 开始({cnt})   q 取消")
        stdscr.refresh()
        stdscr.nodelay(False)
        key = stdscr.getch()
        if key in (curses.KEY_UP, ord("k"), ord("K")):
            sel = (sel - 1) % n
        elif key in (curses.KEY_DOWN, ord("j"), ord("J")):
            sel = (sel + 1) % n
        elif key == ord(" "):
            checked[sel] = not checked[sel]
        elif key == ord("a"):
            checked = [True] * n
        elif key == ord("n"):
            checked = [False] * n
        elif key in (curses.KEY_ENTER, 10, 13):
            return [i for i in range(n) if checked[i]]
        elif key in (ord("q"), ord("Q"), 27):
            return []

def text_view(stdscr, title, lines):
    h, w = stdscr.getmaxyx()
    top = 0
    maxl = h - 3
    stdscr.timeout(-1)
    while True:
        stdscr.erase()
        header(stdscr, title)
        for i in range(maxl):
            idx = top + i
            if idx >= len(lines):
                break
            safe_addstr(stdscr, 2 + i, 1, lines[idx])
        info = f" 行 {top + 1}-{min(top + maxl, len(lines))}/{len(lines)}   ↑↓/j k 滚动   PgUp/PgDn   q 返回"
        footer_line(stdscr, info)
        stdscr.refresh()
        key = stdscr.getch()
        if key in (curses.KEY_UP, ord("k"), ord("K")):
            top = max(0, top - 1)
        elif key in (curses.KEY_DOWN, ord("j"), ord("J")):
            top = max(0, min(len(lines) - maxl, top + 1))
        elif key in (curses.KEY_PPAGE, 2):
            top = max(0, top - maxl)
        elif key in (curses.KEY_NPAGE, 6):
            top = max(0, min(len(lines) - maxl, top + maxl))
        elif key in (ord("q"), ord("Q"), 27):
            return

def stream_view(stdscr, title, q, done_fn, on_quit=None, footer="q 停止并返回"):
    h, w = stdscr.getmaxyx()
    lines = []
    stdscr.timeout(100)
    finished_announced = False
    while True:
        # 读取新行
        drained = False
        try:
            while True:
                lines.append(q.get_nowait())
                drained = True
                if len(lines) > 5000:
                    lines = lines[-3000:]
        except queue.Empty:
            pass
        done = bool(done_fn()) if done_fn else False

        stdscr.erase()
        header(stdscr, title)
        maxl = h - 3
        show = lines[-maxl:] if len(lines) > maxl else lines
        for i, l in enumerate(show):
            safe_addstr(stdscr, 2 + i, 0, w_trunc(l, w))

        if done and not drained:
            if not finished_announced:
                finished_announced = True
            footer_line(stdscr, footer + "   [已完成，按任意键返回]")
        else:
            footer_line(stdscr, footer)
        stdscr.refresh()

        if done and not drained:
            stdscr.nodelay(False)
            stdscr.getch()
            if on_quit:
                on_quit()
            return

        key = stdscr.getch()
        if key in (ord("q"), ord("Q"), 27):
            if on_quit:
                on_quit()
            return

# ============== 功能 ==============
def do_icmp_ping(stdscr, host, title_extra=""):
    q = queue.Queue()
    try:
        proc = subprocess.Popen(["ping", host],
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                text=True, bufsize=1)
    except Exception as e:
        stdscr.timeout(-1)
        stdscr.erase()
        header(stdscr, f"ICMP Ping — {host}")
        safe_addstr(stdscr, 3, 2, f"启动失败: {e}", curses.color_pair(C_ERR))
        stdscr.getch()
        return

    def reader():
        try:
            for line in proc.stdout:
                q.put(line.rstrip())
        finally:
            pass

    threading.Thread(target=reader, daemon=True).start()
    title = f"ICMP Ping — {host}"
    if title_extra:
        title += f"  ({title_extra})"
    stream_view(stdscr, title, q,
                done_fn=lambda: proc.poll() is not None,
                on_quit=lambda: proc.terminate(),
                footer=" q 停止 ")

def do_tcp_ping(stdscr, host, port):
    q = queue.Queue()
    stop_evt = threading.Event()

    def loop():
        seq = 0
        while not stop_evt.is_set():
            seq += 1
            t0 = time.time()
            ok = False
            try:
                r = subprocess.run(["nc", "-z", "-G", "2", "-w", "2", host, str(port)],
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=3)
                ok = (r.returncode == 0)
            except subprocess.TimeoutExpired:
                ok = False
            except Exception:
                ok = False
            ms = (time.time() - t0) * 1000
            ts = time.strftime("%H:%M:%S")
            if ok:
                q.put(f" [{ts}] seq={seq:<4} {host}:{port}   连通   {ms:>6.0f} ms")
            else:
                q.put(f" [{ts}] seq={seq:<4} {host}:{port}   超时/失败")
            stop_evt.wait(1.0)

    threading.Thread(target=loop, daemon=True).start()
    stream_view(stdscr, f"TCP Ping — {host}:{port}", q,
                done_fn=lambda: False,
                on_quit=lambda: stop_evt.set(),
                footer=" q 停止 ")

def _icmp_reader(proc, q):
    try:
        for line in proc.stdout:
            q.put(line.rstrip())
    except Exception:
        pass

def _tcp_loop(host, port, q, stop_evt):
    seq = 0
    while not stop_evt.is_set():
        seq += 1
        t0 = time.time()
        ok = False
        try:
            r = subprocess.run(["nc", "-z", "-G", "2", "-w", "2", host, str(port)],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=3)
            ok = (r.returncode == 0)
        except Exception:
            ok = False
        ms = (time.time() - t0) * 1000
        ts = time.strftime("%H:%M:%S")
        q.put(f"[{ts}] {ms:>5.0f}ms" if ok else f"[{ts}] 超时/失败")
        stop_evt.wait(1.0)

def multi_ping_view(stdscr, targets):
    """targets: list of (kind, host) 或 (kind, host, port). kind in icmp/tcp.
    分屏同时显示多个目标的 ping。"""
    h, w = stdscr.getmaxyx()
    N = len(targets)
    top_y = 2
    bottom_y = h - 1
    avail = bottom_y - top_y
    panel_h = avail // N
    if panel_h < 3:
        stdscr.timeout(-1)
        stdscr.erase()
        header(stdscr, "多目标 Ping")
        safe_addstr(stdscr, 3, 2, f"终端太小或目标太多: {N} 个目标需要至少 {3*N+3} 行",
                    curses.color_pair(C_ERR))
        safe_addstr(stdscr, 5, 2, "按任意键返回", curses.color_pair(C_DIM))
        stdscr.getch()
        return
    extra = avail - panel_h * N  # 余数分给前几个面板

    queues = []
    procs = []
    stop_evts = []
    for t in targets:
        kind = t[0]
        host = t[1]
        port = t[2] if len(t) > 2 and t[2] is not None else TCP_PORT
        q = queue.Queue()
        queues.append(q)
        if kind == "icmp":
            try:
                proc = subprocess.Popen(["ping", host], stdout=subprocess.PIPE,
                                        stderr=subprocess.STDOUT, text=True, bufsize=1)
                procs.append(proc)
                stop_evts.append(None)
                threading.Thread(target=_icmp_reader, args=(proc, q), daemon=True).start()
            except Exception as e:
                q.put(f"启动失败: {e}")
                procs.append(None)
                stop_evts.append(None)
        else:
            stop = threading.Event()
            stop_evts.append(stop)
            procs.append(None)
            threading.Thread(target=_tcp_loop, args=(host, port, q, stop), daemon=True).start()

    buffers = [[] for _ in range(N)]
    stdscr.nodelay(True)
    try:
        while True:
            for i in range(N):
                try:
                    while True:
                        buffers[i].append(queues[i].get_nowait())
                        if len(buffers[i]) > 200:
                            buffers[i] = buffers[i][-120:]
                except queue.Empty:
                    pass

            stdscr.erase()
            header(stdscr, f"多目标 Ping — {N} 个目标")
            y = top_y
            for i, t in enumerate(targets):
                kind = t[0]
                host = t[1]
                port = t[2] if len(t) > 2 and t[2] is not None else TCP_PORT
                display = t[3] if len(t) > 3 else host
                ph = panel_h + (1 if i < extra else 0)
                tag = "ICMP" if kind == "icmp" else f"TCP:{port}"
                label = f" {display}  ({tag}) "
                # 标题行 (充当分隔)
                fill = w - w_len(label) - 1
                safe_addstr(stdscr, y, 0, label + "─" * max(0, fill),
                            curses.color_pair(C_ACC) | curses.A_BOLD)
                y += 1
                result_lines = ph - 1
                show = buffers[i][-result_lines:]
                for l in show:
                    safe_addstr(stdscr, y, 1, w_trunc(l, w - 2))
                    y += 1
                for _ in range(result_lines - len(show)):
                    y += 1
            footer_line(stdscr, " q 停止全部 ")
            stdscr.refresh()

            # 分段等待以便及时响应 q
            quit_flag = False
            deadline = time.time() + 0.25
            while time.time() < deadline:
                key = stdscr.getch()
                if key in (ord("q"), ord("Q"), 27):
                    quit_flag = True
                    break
                time.sleep(0.05)
            if quit_flag:
                break
    finally:
        for p in procs:
            if p is not None:
                try:
                    p.terminate()
                except Exception:
                    pass
        for st in stop_evts:
            if st is not None:
                st.set()

def do_gateway_ping(stdscr):
    gw, iface = get_default_route()
    if not gw:
        stdscr.timeout(-1)
        stdscr.erase()
        header(stdscr, "Ping 网关")
        safe_addstr(stdscr, 3, 2, "未找到默认网关", curses.color_pair(C_ERR))
        safe_addstr(stdscr, 5, 2, "按任意键返回", curses.color_pair(C_DIM))
        stdscr.getch()
        return
    do_icmp_ping(stdscr, gw, title_extra=f"网关 · 接口 {iface}" if iface else "网关")

def do_network_info(stdscr):
    stdscr.timeout(-1)
    stdscr.erase()
    header(stdscr, "网络信息")
    safe_addstr(stdscr, 3, 2, "正在收集...", curses.color_pair(C_DIM))
    stdscr.refresh()
    lines = collect_network_info()
    text_view(stdscr, "网络信息", lines)

def do_speedtest(stdscr, name, url):
    h, w = stdscr.getmaxyx()
    stdscr.timeout(-1)
    stdscr.erase()
    header(stdscr, f"下载测速 — {name}")
    safe_addstr(stdscr, 3, 2, "获取文件大小...", curses.color_pair(C_DIM))
    stdscr.refresh()
    total = get_content_length(url)

    # 临时文件 (测完即删，不保存)
    fd, tmp = tempfile.mkstemp(suffix=".netmon", dir="/tmp")
    os.close(fd)

    if total:
        du = shutil.disk_usage("/tmp")
        if total > du.free * 0.9:
            stdscr.erase()
            header(stdscr, f"下载测速 — {name}")
            safe_addstr(stdscr, 3, 2, "磁盘空间不足:", curses.color_pair(C_ERR))
            safe_addstr(stdscr, 4, 4, f"需要 {human_bytes(total)}  可用 {human_bytes(du.free)}")
            safe_addstr(stdscr, 6, 2, "按任意键返回", curses.color_pair(C_DIM))
            stdscr.getch()
            try:
                os.unlink(tmp)
            except OSError:
                pass
            return

    proc = subprocess.Popen(["curl", "-s", "-o", tmp, url],
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    start = time.time()
    last_t = start
    last_sz = 0.0
    done = False

    stdscr.timeout(500)
    while True:
        try:
            cur = os.path.getsize(tmp)
        except OSError:
            cur = last_sz
        now = time.time()
        dt = now - last_t
        if dt < 0.05:
            dt = 0.05
        inst = (cur - last_sz) / dt
        elapsed = now - start
        avg = cur / elapsed if elapsed > 0.05 else 0
        last_t = now
        last_sz = cur

        if proc.poll() is not None:
            done = True

        stdscr.erase()
        header(stdscr, f"下载测速 — {name}")
        y = 3
        safe_addstr(stdscr, y, 2, "URL:", curses.color_pair(C_DIM)); y += 1
        safe_addstr(stdscr, y, 4, w_trunc(url, w - 6), curses.color_pair(C_DIM)); y += 1
        safe_addstr(stdscr, y, 2, f"总大小  : {human_bytes(total) if total else '未知'}"); y += 2

        pct = (cur / total * 100) if total else 0
        ratio = (cur / total) if total else 0
        safe_addstr(stdscr, y, 2,
                    f"已下载  : {human_bytes(cur)}" + (f"   ({pct:.1f}%)" if total else "")); y += 2

        bar_w = max(10, min(w - 8, 54))
        bar = progress_bar(bar_w, ratio)
        safe_addstr(stdscr, y, 2,
                    f"[{bar}] {pct:5.1f}%" if total else f"[{bar}]  --",
                    curses.color_pair(C_ACC) | curses.A_BOLD); y += 2

        safe_addstr(stdscr, y, 2, f"实时速度: {human_speed(inst):>12}",
                    curses.color_pair(C_HL) | curses.A_BOLD); y += 1
        safe_addstr(stdscr, y, 2, f"平均速度: {human_speed(avg):>12}"); y += 1
        safe_addstr(stdscr, y, 2, f"已用时间: {fmt_time(elapsed)}"); y += 1

        if done:
            safe_addstr(stdscr, y, 2, "状态    : 下载完成 ✓",
                        curses.color_pair(C_OK) | curses.A_BOLD)
        elif inst > 0 and total and cur < total:
            eta = (total - cur) / inst
            safe_addstr(stdscr, y, 2, f"预计剩余: {fmt_time(eta)}", curses.color_pair(C_OK))
        else:
            safe_addstr(stdscr, y, 2, "预计剩余: 计算中...", curses.color_pair(C_DIM))
        y += 2

        if done:
            safe_addstr(stdscr, y, 2, "下载完成，按任意键返回", curses.color_pair(C_OK))
            footer_line(stdscr, " 任意键返回 ")
            stdscr.refresh()
            stdscr.nodelay(False)
            stdscr.getch()
            break
        else:
            footer_line(stdscr, " q 停止下载并返回 ")

        stdscr.refresh()
        key = stdscr.getch()
        if key in (ord("q"), ord("Q"), 27):
            proc.terminate()
            try:
                proc.wait(timeout=2)
            except Exception:
                proc.kill()
            break

    try:
        os.unlink(tmp)
    except OSError:
        pass

# ============== 主菜单 ==============
def main_menu(stdscr):
    items = [
        ("ICMP Ping",        "icmp"),
        ("TCP  Ping",        "tcp"),
        ("下载测速 (实时)",  "speed"),
        ("Ping 网关",        "gw"),
        ("网络信息",         "info"),
        ("打开网页",         "web"),
        ("退出",             "quit"),
    ]
    while True:
        sel = menu_widget(stdscr, "网络监测工具",
                          [i[0] for i in items],
                          subtitle="macOS · curses TUI")
        if sel < 0 or items[sel][1] == "quit":
            return
        action = items[sel][1]

        if action in ("icmp", "tcp"):
            labels = []
            for host, tag in PING_HOSTS:
                labels.append(host + (f"   ({tag})" if tag else ""))
            kind_name = "ICMP" if action == "icmp" else "TCP"
            # ICMP: 追加网关为单独可选目标
            gw, gw_iface = (None, None)
            gw_idx = -1
            if action == "icmp":
                gw, gw_iface = get_default_route()
                if gw:
                    labels.append("网关  " + gw + (f"   (接口 {gw_iface})" if gw_iface else ""))
                    gw_idx = len(labels) - 1
            # 默认勾选: ICMP -> magma.ink / cloudflare / 网关 ; TCP -> 无默认
            defaults = [False] * len(labels)
            if action == "icmp":
                defaults[0] = True  # magma.ink
                if len(PING_HOSTS) > 2:
                    defaults[2] = True  # cloudflare.com
                if gw_idx >= 0:
                    defaults[gw_idx] = True
            chosen = multi_select_menu(stdscr,
                                       f"{kind_name} Ping — 选择目标 (可多选)",
                                       labels,
                                       subtitle="Space 多选  Enter 开始  单选则单视图",
                                       default_checked=defaults)
            if not chosen:
                continue

            def _target_for(idx):
                if idx == gw_idx:
                    disp = "网关 " + gw
                    return ("icmp", gw, None, disp)
                return ("icmp" if action == "icmp" else "tcp",
                        PING_HOSTS[idx][0],
                        TCP_PORT if action == "tcp" else None)

            if len(chosen) == 1:
                idx = chosen[0]
                if idx == gw_idx:
                    do_icmp_ping(stdscr, gw,
                                 title_extra=f"网关 · 接口 {gw_iface}" if gw_iface else "网关")
                else:
                    host = PING_HOSTS[idx][0]
                    if action == "icmp":
                        do_icmp_ping(stdscr, host)
                    else:
                        do_tcp_ping(stdscr, host, TCP_PORT)
            else:
                targets = [_target_for(idx) for idx in chosen]
                multi_ping_view(stdscr, targets)

        elif action == "speed":
            idx = menu_widget(stdscr, "下载测速 — 选择测速源",
                              [n for n, _ in SPEED_ITEMS])
            if idx < 0:
                continue
            do_speedtest(stdscr, SPEED_ITEMS[idx][0], SPEED_ITEMS[idx][1])

        elif action == "gw":
            do_gateway_ping(stdscr)

        elif action == "info":
            do_network_info(stdscr)

        elif action == "web":
            idx = menu_widget(stdscr, "打开网页", [n for n, _ in WEB_SITES])
            if idx < 0:
                continue
            subprocess.Popen(["open", WEB_SITES[idx][1]])

def run_with_arg(stdscr, arg):
    curses.curs_set(0)
    init_colors()
    if not arg:
        main_menu(stdscr)
        return
    mapping = {
        "1": ("icmp", 0),
        "2": ("tcp", 0),
        "3": ("speed", 0),
        "4": ("gw", None),
        "5": ("info", None),
        "6": ("web", 0),
    }
    if arg not in mapping:
        main_menu(stdscr)
        return
    action, idx = mapping[arg]
    if action == "icmp":
        do_icmp_ping(stdscr, PING_HOSTS[idx][0])
    elif action == "tcp":
        do_tcp_ping(stdscr, PING_HOSTS[idx][0], TCP_PORT)
    elif action == "speed":
        do_speedtest(stdscr, SPEED_ITEMS[idx][0], SPEED_ITEMS[idx][1])
    elif action == "gw":
        do_gateway_ping(stdscr)
    elif action == "info":
        do_network_info(stdscr)
    elif action == "web":
        subprocess.Popen(["open", WEB_SITES[idx][1]])

def main(stdscr):
    curses.curs_set(0)
    init_colors()
    main_menu(stdscr)

if __name__ == "__main__":
    try:
        locale.setlocale(locale.LC_ALL, "")
    except Exception:
        pass
    arg = sys.argv[1] if len(sys.argv) > 1 else None
    try:
        if arg:
            curses.wrapper(lambda s: run_with_arg(s, arg))
        else:
            curses.wrapper(main)
    except KeyboardInterrupt:
        pass
