# AimiliVPN (Go 高性能重构版)

**面向 Linux VPS 的高性能 VPNGate 节点管理与 HTTP / HTTPS / SOCKS5 单端口统一代理网关**

采用 **Go 1.22+ 原生高并发模型与零拷贝技术** 对原 Python 架构进行深度重构。编译后生成**单一可执行二进制文件**（内置现代化响应式 Web 控制台），内存占用极低（< 15MB），专为资源受限的轻量级 Linux VPS（如 256MB / 512MB 内存机型）打造。

---

## 快速安装与终端管理

### 1. 一键全自动部署 (推荐)

使用 `root` 用户在受支持的 Linux VPS (Ubuntu / Debian / CentOS / Rocky / AlmaLinux / Alpine) 上执行：

```bash
curl -sSL https://raw.githubusercontent.com/xiumuzidiao0/aimili-vpngate-go/main/install.sh | bash
```

安装完成后，脚本将：
- 自动检测并安装所有系统网络依赖与 OpenVPN；
- 部署 `aimilivpn` 守护进程并注册 `systemd` 服务开机自启；
- 创建终端管理快捷命令 `ml`。

### 2. 终端交互式管理中心

在终端输入以下命令，即可随时打开可视化管理菜单：

```bash
ml
```

控制中心支持：
- 实时查看服务运行状态、Web 入口与管理凭据；
- 一键启动、停止、重启服务；
- 实时追踪运行日志（`journalctl` 流式显示）；
- 智能一键切换最优节点 / 浏览候选节点；
- 动态修改管理员账号、密码、Web 端口与代理端口；
- 一键检查并在线更新最新版本。

---

## 核心特性与架构升级

| 特性 | 原 Python 版本 | Go 重构版本 |
| :--- | :--- | :--- |
| **可执行体积与分发** | 需 Python 3.10+、多依赖脚本 | **单一静态二进制文件（8.3MB）**，零外部语言依赖 |
| **内存占用** | 80MB ~ 150MB | **< 15MB 内存**，抗压能力大幅提升 |
| **代理并发模型** | 线程（Thread）+ GIL 限制 | **Goroutine + Linux Epoll**，轻松承载数千长连接并发 |
| **网络数据转发** | 用户态内存复制 | **Linux `splice` 内核零拷贝**，降低 CPU 消耗与传输延迟 |
| **Web 控制台与通信** | 手工拼接 HTML 字符串，1 秒长轮询 | **`go:embed` 内嵌现代响应式前端，支持 SSE 实时推流** |
| **系统与安全设置** | 仅支持终端脚本或环境变量 | **Web 控制台直接修改账号、密码、Web 端口、安全路径与代理端口** |
| **IP 属性智能探测** | 仅基础归属 | **智能识别 🏠 住宅宽带 IP vs 🏢 机房 IP vs 📱 移动 IP，支持按网络类型精准筛选** |
| **数据指标监控** | 无 | **内置 Prometheus `/metrics` 指标接口**，支持 Grafana 监控 |
| **安全防御** | 基础白名单 | **增强型 OpenVPN 词法语法白名单校验**，彻底阻断恶意节点代码注入 |

---

## 目录结构说明

```text
aimili-vpngate-go/
├── cmd/
│   └── aimilivpn/
│       └── main.go          # 统一主入口、信号监听与优雅退出协调
├── pkg/
│   ├── config/              # 环境变量读取与默认配置管理器
│   ├── nodes/               # VPNGate 节点拉取、安全校验、节点池与动态黑名单
│   ├── proxy/               # 7928 单端口多协议自适应网关 (SOCKS5 / HTTP CONNECT)
│   ├── vpn/                 # OpenVPN 进程生命周期监管、心跳保活与自动故障转移
│   ├── stats/               # 实时上下行速率统计 (Bps) 与内存环形日志总线
│   └── server/              # Web 管理 API、SSE 实时事件推流与中间件
├── web/
│   ├── dist/index.html      # 现代化响应式 Web 管理面板源码
│   └── embed.go             # go:embed 静态资源内嵌打包
├── scripts/
│   ├── build.sh             # 多架构交叉编译脚本 (amd64, arm64, 386, arm)
│   └── aimilivpn.service    # systemd 系统守护进程配置模板
├── Dockerfile               # 多阶段极简容器镜像构建
└── go.mod
```

---

## 快速上手与编译

### 1. 本地直接编译与运行

确保已安装 Go 1.22+ 环境：

```bash
# 编译
go build -o aimilivpn ./cmd/aimilivpn

# 运行 (需要 root 权限以管理 TUN 网络设备)
sudo ./aimilivpn
```

### 2. 交叉编译跨平台二进制

```bash
chmod +x scripts/build.sh
./scripts/build.sh
```

产物将输出在 `dist/` 目录下，包含各平台独立可执行文件及对应 `.gz` 压缩包。

---

## 常用环境变量与配置项

可以通过环境变量或配置文件动态调整系统参数：

| 环境变量 | 默认值 | 作用描述 |
| :--- | :--- | :--- |
| `UI_HOST` | `::` | Web 管理控制台监听地址（IPv6 失败自动回退 IPv4） |
| `UI_PORT` | `8787` | Web 控制台端口 |
| `UI_PATH` | 随机生成 / `aimili` | 访问后台的安全路径前缀（防爆破与扫描探测） |
| `UI_USERNAME` | `admin` | Web 控制台管理员账号 |
| `UI_PASSWORD` | `aimilivpn` | Web 控制台管理员密码 |
| `LOCAL_PROXY_HOST` | `127.0.0.1` | 本地代理监听地址 |
| `LOCAL_PROXY_PORT` | `7928` | 本地自适应代理端口（同时支持 SOCKS5 和 HTTP 代理） |
| `LOCAL_PROXY_USER` | 空 (不启用) | 本地代理认证用户名（可选） |
| `LOCAL_PROXY_PASS` | 空 (不启用) | 本地代理认证密码（可选） |
| `DISCOVERY_COUNTRIES` | 空 (全选) | 允许选取的国家代码，逗号分隔，如 `JP,US,KR` |
| `CHECK_INTERVAL_SECONDS` | `20` | 心跳探活周期（秒），检测到异常断线自动触发故障转移 |

---

## 服务集成 (systemd)

在 VPS 宿主机上注册为系统服务：

```bash
# 复制二进制到目标目录
sudo mkdir -p /usr/local/aimilivpn
sudo cp aimilivpn /usr/local/aimilivpn/

# 安装 systemd 服务
sudo cp scripts/aimilivpn.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now aimilivpn

# 查看运行日志
journalctl -u aimilivpn -f
```
