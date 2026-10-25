# nftables Control Panel (nftables Web 控制面板)

🔥 一个轻量级、零外部依赖的 Linux nftables 端口转发与防火墙 Web 管理面板。基于 Go 语言开发，单二进制内嵌 Web 前端，支持各种主流与嵌入式 Linux 架构。

---

## ✨ 核心特性

1. **零外部依赖 / 单二进制分发**：前端静态文件通过 `//go:embed` 打包在二进制内部，无需安装 Node.js、Nginx 或任何运行库。
2. **多架构全覆盖**：通过 GitHub Actions 自动交叉编译生成 `amd64`、`arm64` (aarch64)、`armv7`、`386`、`mipsle`、`riscv64` 等预编译包。
3. **环境自适应与自愈**：
   - 运行时**自动检测 `nftables`**，若未安装则自动调用系统包管理器（`apk` / `apt` / `dnf` / `yum` / `pacman` / `zypper`）完成无缝安装与服务自启。
   - 运行时**自动检查并开启 `net.ipv4.ip_forward`**（IPv4 转发），并自动写入持久化配置（`/etc/sysctl.d/99-nftables-forward.conf`）。
4. **端口灵活分配**：
   - 默认无参数运行时，**自动在 10000 - 60000 间随机挑选空闲端口**，并在控制台打印访问链接。
   - 支持通过 `-p` 或 `-port` 手动指定固定端口（如 `-p 8888`）。
5. **安全防锁保护**：启动时自动在 `inet filter` 链中放行当前 Web 服务端口与 SSH 22 端口，防止防火墙规则误改把自己封禁在外部。
6. **现代化暗黑 UI**：
   - 端口转发 (DNAT / Redirect)：支持 TCP / UDP / BOTH、随机端口生成、目标地址解析（`IP:端口` / 纯 IP / 本机）、规则删除。
   - 规则集持久化：一键持久化保存到 `/etc/nftables.conf`，重启不丢失。
   - 完整 Ruleset 查看与一键清空救急 (Flush)。

---

## 🚀 快速使用

### 1. 一键下载运行 (根据机器架构)

直接从 [GitHub Releases](https://github.com/M0eBa1/nftables_control_panel/releases) 下载对应架构的压缩包：

```bash
# 以 Linux amd64 (x86_64) 为例：
curl -sL https://github.com/M0eBa1/nftables_control_panel/releases/latest/download/nftables_control_panel_linux_amd64.tar.gz | tar -xz
chmod +x nftables_control_panel_linux_amd64

# 以 Linux arm64 (aarch64, 树莓派/香橙派/路由器等) 为例：
curl -sL https://github.com/M0eBa1/nftables_control_panel/releases/latest/download/nftables_control_panel_linux_arm64.tar.gz | tar -xz
chmod +x nftables_control_panel_linux_arm64
```

### 2. 启动服务

```bash
# 默认随机端口启动
./nftables_control_panel_linux_amd64

# 或指定端口启动 (例如 8888)
./nftables_control_panel_linux_amd64 -p 8888
```

启动后控制台将输出如下提示：
```text
==================================================
       nftables Control Panel v1.0.0
       开源地址: https://github.com/M0eBa1/nftables_control_panel
==================================================
[Init] net.ipv4.ip_forward 已处于开启状态

🚀 控制面板已成功启动!
  ▶ 本机访问: http://127.0.0.1:28443
  ▶ 网络访问: http://192.168.1.100:28443
--------------------------------------------------
  按 Ctrl+C 可停止服务
==================================================
```

通过浏览器打开打印的 URL 即可进入控制面板！

---

## ⚙️ 命令行参数

| 参数 | 缩写 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-port` | `-p` | `0` (随机) | Web 管理界面的监听端口 (1-65535) |
| `-host` | `-h` | `0.0.0.0` | Web 服务绑定的网络地址 |
| `-version` | `-v` | `false` | 查看程序当前版本 |

---

## 🛠️ 后台常驻 (systemd / OpenWrt / Alpine)

### Systemd (Debian / Ubuntu / CentOS)

```bash
cat << 'EOF' > /etc/systemd/system/nftables-panel.service
[Unit]
Description=nftables Control Panel
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/nftables_control_panel -p 8888
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now nftables-panel
```

---

## 📦 本地编译 (开发者)

本程序无任何第三方依赖包，仅依赖 Go 标准库：

```bash
git clone https://github.com/M0eBa1/nftables_control_panel.git
cd nftables_control_panel

# 本地编译当前架构
CGO_ENABLED=0 go build -ldflags="-s -w" -o nftables_control_panel .
```
