# AimiliVPN (Go 高性能重构版)

<div align="center">

**面向 Linux VPS 的现代化 VPNGate 节点自适应管理、多出口流量调度与边缘抗封锁单端口代理网关**

[![正式版本](https://img.shields.io/github/v/release/xiumuzidiao0/aimili-vpngate-go?style=flat-square&label=正式版&color=16a34a)](https://github.com/xiumuzidiao0/aimili-vpngate-go/releases/latest)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Platform](https://img.shields.io/badge/平台-amd64%20%7C%20arm64%20%7C%20386%20%7C%20arm-6366f1?style=flat-square)](https://github.com/xiumuzidiao0/aimili-vpngate-go/releases/latest)
[![License](https://img.shields.io/badge/License-GPL--3.0-334155?style=flat-square)](LICENSE)

</div>

采用 **Go 1.22+ 原生高并发模型与系统底层零拷贝技术** 对传统 VPN 代理系统进行工业级重构。编译后生成**单一可执行二进制文件**（内置 5 视图现代化响应式 SPA Web 控制台），内存常驻极低（< 15MB），专为资源受限的轻量级 Linux VPS（如 256MB / 512MB 内存机型）打造，同时具备高吞吐、零泄漏与强隔离能力。

---

## 快速安装与终端管理

### 1. 一键极速安装与部署 (推荐)

使用 `root` 用户在受支持的 Linux VPS (Ubuntu / Debian / CentOS / Rocky / AlmaLinux / Alpine) 上执行：

```bash
curl -sSL https://raw.githubusercontent.com/xiumuzidiao0/aimili-vpngate-go/main/install.sh | bash
```

安装脚本将自动：
- 识别 CPU 架构，优先从官方多加速镜像源秒级下载预编译静态二进制包（约 6.5MB）；
- 自动安装配置系统级网络依赖与 OpenVPN；
- 部署 `aimilivpn` 守护进程并注册 `systemd` 服务实现开机自愈自启；
- 创建全局快捷指令 `ml` 与 `aimili`。

### 2. 终端极速更新与命令行快捷操作

系统支持全自动免交互命令行操作，亦可打开交互式菜单：

```bash
ml update         # 从 GitHub Release 官方源秒级极速更新至最新发行版本并自动热重启
ml status         # 查看当前运行状态、Web 入口与管理账密
ml restart        # 安全平滑重启 AimiliVPN 服务
ml logs           # 查看实时 journalctl 运行日志流
ml menu           # 打开终端交互式可视化控制中心
```

---

## 核心系统级优化与高级架构 (v2.5.0)

### 1. 👑 系统主出口网关组 (`system-primary`，独占 `tun0`)
- **网卡强隔离保障**：底层严格保留 `devIndex = 0`（`tun0`）专属于系统主网关出口，并发多出口自适应池统一从 `tun1`、`tun2`... 向上单调递增，彻底杜绝自适应组与主网关争抢网卡的冲突；
- **自适应策略化接管**：将主连接升级为持久化系统特殊自适应组，支持按国家、网络类型、解锁能力与择优指标自动选拔与保活。

### 2. 🤖 节点解锁能力多维智能筛选 (AI 模型 & 流媒体)
- 内置流式网络解锁状态评估模型，自适应隧道组支持按需精准选拔出口：
  - `🌐 不限解锁能力 (none)`：通用全量优选；
  - `🤖 必须支持 AI 模型 (ai)`：智能筛选支持 **OpenAI (ChatGPT) / Claude** 的节点；
  - `🎬 必须支持主流流媒体 (streaming)`：智能筛选支持 **Netflix 原生解锁 / Google** 的节点；
  - `⭐ 全解锁 (full / all)`：必须同时满足 **AI 模型 + 主流流媒体** 双重解锁能力。

### 3. 🚀 跨洋长肥管道（BDP）TCP 缓冲区与延迟调优
- **512KB 套接字缓冲区**：在 OpenVPN 模板中注入 `sndbuf 524288`、`rcvbuf 524288` 与 `txqueuelen 1000`，打破跨洋跨洲网络（50ms~200ms RTT）下内核默认小缓冲区导致的 TCP 拥塞窗口快速停顿；
- **全链路 TCP NoDelay**：对所有本地接入套接字与隧道拨号连接强制启用 `SetNoDelay(true)`，彻底禁用 Nagle 算法，消除本地中转与链式代理微小延迟。

### 4. ⚡ 毫秒级滑动失败窗口熔断器 (Circuit Breaker) & 15秒公网真出网主动探针
- **公网真出网门禁**：新隧道完成 TLS 握手拿到内网 IP 后，必须通过 `CheckTunnelConnectivity` 现场向公网（Cloudflare / Google）收发测试，只有真正具备公网出网能力的节点才允许加入出口池，当场排除假死志愿节点；
- **15秒周期自愈探针**：自适应组每 15 秒主动向每个在线隧道发包探测，一旦节点因房东断网等原因出现丢包，在 15 秒内自动判定失效并触发替补；
- **毫秒级跨池逃生**：当某个端口绑定的组发生故障时，调度器在毫秒级内自动回退至全池健康出口（如 `tun0`），绝不向断网节点送死流量，杜绝 502 Bad Gateway。

### 5. 📦 客户端订阅多格式导出 (Clash Meta / Mihomo 专属 YAML 导出)
- **纯 Go 零依赖生成器**：实现 `GenerateClashYAML()`，将 sing-box 入站节点标准化生成完整的 Clash Meta / Mihomo 配置；
- **专业策略组装配**：包含 `🚀 节点选择`、`♻️ 自动选择 (URL-Test)`、`⚡ 故障转移 (Fallback)` 与 `🐟 漏网之鱼`，内置国内外 GEOIP 分流规则；
- **免密安全更新**：客户端通过已验证的安全管理路径（如 `/enter/api/singbox/subscription/clash`）或 Token 即可直接更新配置，无需弹出 Basic Auth 认证框；Web 控制台支持一键复制与直接下载 YAML 文件。

### 6. 🛡️ sing-box 边缘抗封锁入站 & 7×24h 后台自愈守护 (Watchdog)
- **入站矩阵管理**：支持可视化管理 VLESS-REALITY、Hysteria2、TUIC、Shadowsocks、AnyTLS 等抗审查入站协议；
- **双向链式凭据同步**：在 Web 切换各节点出口（直连 vs 本地代理端口）时，自动根据端口认证规则无缝转换与同步凭据（免密、管理密码、独立账密）；
- **自愈守护进程**：后台 30 秒独立心跳巡检，一旦 sing-box 发生意外 OOM 或终止，自愈守护程序在 30 秒内安全拉起，实现 7×24 小时无人值守。

### 7. 🔄 故障隔离屏蔽库与 3 小时定期探活复活机制
- **告别误杀**：严格收窄拉黑标准，手动断开与正常退役的节点 100% 不会被加入屏蔽库；
- **3小时自动探活复活**：后台协程每 3 小时自动对屏蔽库全量节点执行 TCP 探活，一旦节点恢复连通，自动解封并放回候选池；Web 弹窗支持一键手动探活复活。

### 8. 💻 五大视图模块化 SPA Web 控制台
- 采用原生 Vanilla JS + CSS Tokens 构建响应式 SPA 架构，彻底消除单页杂乱堆叠：
  - **📊 运行概览 (Dashboard)**：实时上下行网速、活跃连接、主网关卡片、流式系统日志；
  - **🚀 边缘入站 (sing-box)**：抗封锁入站节点列表、链式代理出口选择、Clash 订阅管理；
  - **🔀 多端口分流 (Matrix)**：M:N 代理端口规则矩阵、动态自适应隧道组参数管理；
  - **🌐 优质节点 (Nodes)**：全量候选节点筛选、TCP 延迟测速、流媒体与 AI 解锁探测；
  - **⚙️ 系统设置 (Settings)**：Web 端口、管理账号密码、安全路径、代理端口配置；
- **桌面可折叠侧边栏**：支持 230px 完整模式与 66px 迷你图标模式自由切换，状态通过 `localStorage` 自动持久化记忆；移动端自适应为底部原生导航栏。

---

## 核心特性架构对比

| 特性 | 原 Python 版本 | Go 重构版本 (v2.5.0) |
| :--- | :--- | :--- |
| **程序分发与体积** | 需 Python 3.10+、海量依赖脚本 | **单一静态二进制文件（约 6.5MB）**，零外部语言依赖 |
| **内存与 CPU 消耗** | 80MB ~ 150MB | **< 15MB 内存，CPU 占用 < 1%**，抗压能力大幅跃升 |
| **并发代理模型** | Thread + 全局解释器锁 (GIL) 瓶颈 | **Goroutine + Linux Epoll**，轻松承载数千长连接并发 |
| **主连接设备管理** | 设备跳跃无序 | **`tun0` 专享独占保留**，封装为系统自适应组 |
| **多出口并发调度** | 仅支持单一主连接 | **支持 `tun1..tun63` 多出口并发，M:N 端口调度 (轮询/随机/定时)** |
| **动态自适应维护** | 无 | **自动按国家/家宽属性/AI解锁能力维持 Top N 节点在线** |
| **健康与出网检测** | 无主动检测 | **15秒周期真实出网发包探测，握手真通网门禁** |
| **故障熔断自愈** | 无，长达数十秒死等超时 | **毫秒级滑动失败窗口熔断器 + 跨池自动兜底容灾** |
| **屏蔽库管理** | 盲目拉黑 24 小时 | **精准定性防误杀 + 3小时自动探活复活 + Web 一键自愈** |
| **边缘抗封锁入站** | 无 | **深度集成 sing-box (Reality / Hy2 / TUIC / SS / AnyTLS) 链式代理** |
| **客户端生态导出** | 仅单节点通用 URL | **一键导出 Clash Meta / Mihomo 格式全功能 YAML 订阅** |
| **守护与高可用** | 意外退出即断网 | **内置 7×24 小时 sing-box Watchdog 自愈守护协程** |
| **网络性能调优** | 系统默认小缓冲区 | **BDP TCP 512KB Socket 发送/接收缓冲区 + 全链路 NoDelay** |
| **Web 控制台设计** | 简陋单页 HTML 拼接 | **五大视图现代化 SPA 架构，持久化折叠侧栏与移动底栏** |
| **宿主机安全性** | 曾有主路由被篡改风险 | **虚拟网卡严格限制在私有策略路由表 (Table 100+N)，主机 SSH 100% 隔离** |

---

## 目录结构说明

```text
aimili-vpngate-go/
├── cmd/
│   └── aimilivpn/
│       └── main.go          # 统一主入口、信号监听与组件生命周期协调
├── pkg/
│   ├── config/              # 环境变量、持久化配置与版本号定义
│   ├── nodes/               # VPNGate 数据拉取、安全白名单、IP 属性富化与屏蔽库
│   ├── proxy/               # 7928+ 多端口调度器、协议探测、SOCKS5 / HTTP CONNECT 代理网关
│   ├── vpn/                 # OpenVPN 进程监管、TUN 检测与主连接管理器
│   ├── tunnel/              # 虚拟网卡池分配、路由隔离、自适应组、熔断器与解锁检测
│   ├── singbox/             # sing-box 客户端控制 API 交互与状态同步
│   ├── stats/               # 实时网络速率计量 (Bps) 与内存环形日志总线
│   ├── notify/              # Telegram Bot 交互、告警推送与指令响应
│   └── server/              # Web 控制台 RESTful API、Clash YAML 导出、Watchdog 与中间件
├── web/
│   ├── dist/index.html      # 现代化响应式 5 视图 SPA 控制台前端源码
│   └── embed.go             # go:embed 静态资产打包
├── scripts/
│   ├── build.sh             # 多架构交叉编译与自动压缩脚本
│   └── aimilivpn.service    # systemd 系统守护进程配置模板
├── Dockerfile               # 多阶段极简容器构建镜像
├── VERSION                  # 语义化版本号标识
└── go.mod
```

---

## 快速上手与本地编译

### 1. 本地直接编译运行

确保机器已安装 Go 1.22+ 环境：

```bash
# 编译当前架构二进制
go build -ldflags="-s -w" -o aimilivpn ./cmd/aimilivpn

# 启动运行 (需要 root 权限以管理虚拟网卡与策略路由)
sudo ./aimilivpn
```

### 2. 交叉编译全架构发行包

```bash
chmod +x scripts/build.sh
./scripts/build.sh
```

编译产物将输出在 `dist/` 目录中：
- `dist/aimilivpn_linux_amd64` (及其 `.gz` 压缩包)
- `dist/aimilivpn_linux_arm64` (及其 `.gz` 压缩包)
- `dist/aimilivpn_linux_386` (及其 `.gz` 压缩包)
- `dist/aimilivpn_linux_arm` (及其 `.gz` 压缩包)

---

## 常见问题排查与技术细节

### Q1: 为什么主连接与自适应组各出口互不干扰？
A: 系统采用 Linux 高级策略路由隔离体系。主连接独占 `tun0`（绑定路由表 `Table 100`），并发自适应组出口有序占用 `tun1`、`tun2`...（分别绑定 `Table 101`、`Table 102`...）。主路由表（`main table`）保留宿主机 `eth0` 默认网关，**宿主机 SSH 22 端口及网络 100% 隔离安全**。

### Q2: 遇到部分志愿节点握手成功但无法出网怎么办？
A: 系统内置了 15 秒高频出网真实性探针及握手门禁测试。任何握手成功但无法出网（志愿房东断网或防火墙拦截）的节点，会在 15 秒内自动被熔断器剔除，系统自动选取下一个真实通网的候选节点替补，保障出口池时刻处于可用状态。

### Q3: 客户端如何导入 Clash Meta 订阅？
A: 在 Web 控制台「边缘入站」页面顶部，点击 **「📋 复制 Clash 订阅」** 即可获取专属链接，直接粘贴到 Clash Verge Rev、Mihomo Party、Clash Meta for Android 等客户端中即可一键更新使用。
