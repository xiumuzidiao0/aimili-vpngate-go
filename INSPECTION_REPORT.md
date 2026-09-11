# AimiliVPN Go 项目检查结果报告

## 1. 检查结论

**原始检查结论：暂时不建议进入生产环境（No-Go）。**

**当前状态：报告中的代码层问题已完成修复并通过自动化回归；真实 OpenVPN、race detector 和长时间运行测试仍需在隔离 Linux VM 中补做。**

本次检查确认项目基础测试、跨架构构建、节点镜像和隔离路由生命周期能够通过，但存在两个直接阻断上线的 P0 问题和多个高优先级安全、并发及进程生命周期问题：

- 主程序实际代理监听地址没有遵守 `LOCAL_PROXY_HOST`，默认会监听所有网卡。
- 当前 Go 1.22.6 构建工具链存在 35 个可达标准库漏洞。
- Web 控制台存在存储型 XSS 风险。
- CSV 异常输入可导致进程 panic。
- 隧道快速停止和替换时存在设备槽位复用竞态。
- 自定义代理密码等数据使用了过宽文件权限。
- 启动时会终止宿主机上所有 OpenVPN 进程。

建议先修复两个 P0 问题及 XSS，再处理并发生命周期、权限和凭据相关问题，最后在隔离 Linux VM 中完成真实 OpenVPN 和长稳测试。

### 1.1 修复后状态

| 原问题 | 当前状态 | 主要处理 |
| --- | --- | --- |
| P0-01 代理监听所有网卡 | 已修复 | `PortListener` 遵守 `ProxyHost`，默认绑定 `127.0.0.1` |
| P0-02 Go 1.22.6 漏洞 | 已修复 | 工具链、CI、Dockerfile 升级至 Go 1.25.13 |
| P0-03 Web 控制台 XSS | 已修复 | 动态文本统一 HTML 转义，事件参数使用安全 JS 参数序列化 |
| P1-01 CSV 短行 panic | 已修复 | 解析前校验必需列长度，并增加短行回归测试 |
| P1-02 隧道设备复用竞态 | 已修复 | 引入清理完成屏障，进程、文件、路由清理后才释放设备槽位 |
| P1-03 凭据文件权限过宽 | 已修复 | 数据目录改为 `0700`，状态和凭据文件改为 `0600` |
| P1-04 解锁数据竞争 | 已修复 | 增加 `Tunnel.SetUnlock`，读写统一受 `Tunnel.mu` 保护 |
| P1-05 全局终止 OpenVPN | 已修复 | 删除 `killall -9 openvpn` |
| P1-06 管理入口明文默认公网 | 已修复 | 默认绑定回环地址，安装配置同步调整并增加明文 HTTP 警告 |
| P1-07 安装包缺少哈希校验 | 已修复 | 安装器从 GitHub 获取 SHA-256，并强制校验下载二进制 |
| P2-01 IPv6 地址拼接错误 | 已修复 | 统一使用 `net.JoinHostPort` |
| P2-02 `rp_filter` 状态泄漏 | 已修复 | 保存原值，最后一个隧道清理后恢复 |
| P2-03 拉取忽略 Context | 已修复 | 网络拉取从调用方 Context 派生超时 Context |
| P2-04 HTTP 请求边界不足 | 已修复 | 增加读取超时、空闲超时、Header 限制和 1 MiB 请求体限制 |
| P2-05 SSE CORS 过宽 | 已修复 | 删除 `Access-Control-Allow-Origin: *` |
| P2-06 关键模块测试不足 | 部分改善 | 增加监听、CSV、IPv6、权限、并发清理、请求限制和 netns 测试；总覆盖率 32.5% -> 34.3% |

### 1.2 修复后复测

| 检查项 | 结果 |
| --- | --- |
| `go test ./...` | 通过 |
| `go test -shuffle=on -count=10 ./...` | 通过 |
| `go vet ./...` | 通过 |
| `staticcheck ./...` | 通过 |
| `gosec ./...` | 通过 |
| `govulncheck ./...` | 0 个可达漏洞 |
| `gofmt -l` | 无输出 |
| `bash -n install.sh`、`bash -n scripts/build.sh` | 通过 |
| 四架构构建 | `amd64`、`arm64`、`386`、`arm` 全部通过 |
| 隔离 network namespace 路由测试 | 通过，路由和 `rp_filter` 均恢复 |
| 在线节点镜像端到端测试 | 99 个有效节点，SHA-256 与仓库镜像一致 |
| 内联前端脚本语法和 HTML 转义样本 | 通过 |

## 2. 原始检查信息

| 项目 | 内容 |
| --- | --- |
| 检查日期 | 2026-09-11 UTC |
| Git 提交 | `0816e23a611bf521d566e4ff46c396081a81b138` |
| 分支 | `main` |
| 项目版本 | `2.5.0` |
| 基线 Go 版本 | `go1.22.6 linux/arm64` |
| 修复验证 Go 版本 | `go1.25.13 linux/arm64` |
| 运行内核 | `Linux 6.6.89-Gold_bug aarch64` |
| 当前用户 | `uid=1000(xmzd)`，无免密 sudo |
| sing-box | `1.14.0 linux/arm64` |
| OpenVPN | 未安装 |
| CodeGraph 索引 | 65 个文件、931 个节点、2,199 条边、44 个路由 |

## 3. 原始检查范围

本次检查覆盖以下内容：

- Go 编译、格式、静态检查、单元测试、覆盖率和漏洞扫描。
- CodeGraph 符号、调用方、影响面和模块依赖分析。
- VPNGate CSV 拉取、解析、校验、快照和持久化。
- OpenVPN 隧道、设备分配、策略路由和进程生命周期源码检查。
- 代理端口绑定、SOCKS5、HTTP CONNECT、认证和流量转发。
- Web API、中间件、认证、安全响应头、SSE 和配置持久化。
- sing-box 客户端、订阅、链式代理凭据和 Watchdog。
- 安装脚本、发布工作流、Dockerfile 和 systemd 配置。
- 隔离 Linux 网络命名空间中的策略路由创建和清理。
- 8 个线上节点数据源的可达性和镜像端到端生成。

## 4. 原始自动化检查结果

| 检查项 | 结果 | 说明 |
| --- | --- | --- |
| `go test ./...`（Go 1.22.6） | 通过 | 所有现有测试通过 |
| `go test ./...`（Go 1.25.13） | 通过 | 所有现有测试通过 |
| `go test -shuffle=on -count=10 ./...` | 通过 | 乱序重复测试通过 |
| `go vet ./...`（Go 1.22.6） | 通过 | 未输出问题 |
| `go vet ./...`（Go 1.25.13） | 失败 | 发现 IPv6 地址拼接问题 |
| `gofmt -l` | 失败 | 8 个 Go 文件未格式化 |
| `staticcheck ./...` | 失败 | 4 个问题 |
| `govulncheck ./...`（Go 1.22.6） | 失败 | 35 个可达标准库漏洞 |
| `govulncheck ./...`（Go 1.25.13） | 通过 | 0 个可达漏洞 |
| `go test -race ./...` | 环境阻断 | ThreadSanitizer 不支持当前 39 位 VMA 布局 |
| 四架构交叉构建（Go 1.22.6） | 通过 | `amd64`、`arm64`、`386`、`arm` |
| 四架构交叉构建（Go 1.25.13） | 通过 | `amd64`、`arm64`、`386`、`arm` |
| 总测试覆盖率 | 32.5% | `config`、`vpn`、`stats` 为 0% |

格式检查未通过的文件：

- `cmd/mirror/main.go`
- `pkg/server/watchdog.go`
- `pkg/tunnel/dynamic.go`
- `pkg/tunnel/circuit_test.go`
- `pkg/tunnel/model.go`
- `pkg/nodes/model.go`
- `pkg/nodes/snapshot.go`
- `pkg/nodes/validator.go`

Staticcheck 报告：

- `pkg/nodes/parser.go:80`：可简化 `if` 为无条件 `strings.TrimPrefix`。
- `pkg/notify/telegram.go:24`：字段 `mu` 未使用。
- `pkg/proxy/scheduler_test.go:11`：类型 `mockPool` 未使用。
- `pkg/proxy/scheduler_test.go:15`：方法 `mockPool.GetHealthyTunnels` 未使用。

## 5. 原始风险问题清单

| ID | 等级 | 问题 | 状态 |
| --- | --- | --- | --- |
| P0-01 | 严重 | 代理监听地址忽略 `LOCAL_PROXY_HOST`，默认监听所有网卡 | 已复现 |
| P0-02 | 严重 | Go 1.22.6 存在 35 个可达标准库漏洞 | 已确认 |
| P0-03 | 严重 | Web 控制台存储型 XSS | 源码确认 |
| P1-01 | 高 | 畸形 CSV 短行导致 panic | 已复现 |
| P1-02 | 高 | 隧道停止时提前释放设备槽位，存在路由清理竞态 | 源码确认 |
| P1-03 | 高 | 自定义代理密码以 `0644` 持久化 | 源码确认 |
| P1-04 | 高 | 解锁结果存在无锁写入数据竞争 | 源码确认 |
| P1-05 | 高 | 启动时终止宿主机所有 OpenVPN 进程 | 源码确认 |
| P1-06 | 高 | 默认管理后台使用明文 HTTP 和 Basic Auth | 部署配置确认 |
| P1-07 | 高 | 安装器不校验发布包 SHA-256 | 源码确认 |
| P2-01 | 中 | IPv6 节点探测地址拼接错误 | Go vet 确认 |
| P2-02 | 中 | 全局 `rp_filter` 被修改且不会恢复 | 源码确认 |
| P2-03 | 中 | 节点拉取不遵守调用方 Context | 源码确认 |
| P2-04 | 中 | HTTP 服务缺少请求头、请求体和空闲超时限制 | 源码确认 |
| P2-05 | 中 | SSE 对任意来源开放 CORS | 源码确认 |
| P2-06 | 中 | 核心网络和配置模块测试覆盖不足 | 覆盖率确认 |

## 6. P0 原始问题与修复方案

### P0-01 代理监听地址未生效

**位置：**

- [cmd/aimilivpn/main.go](cmd/aimilivpn/main.go#L52)
- [pkg/proxy/manager.go](pkg/proxy/manager.go#L97)
- [pkg/proxy/listener.go](pkg/proxy/listener.go#L69)
- [pkg/proxy/gateway.go](pkg/proxy/gateway.go#L39)

**问题：**

主程序使用 `MultiPortManager.StartAll()` 启动代理，而 `PortListener.Start()` 固定使用：

```go
addr := fmt.Sprintf(":%d", l.rule.Port)
```

这会绑定到所有 IPv4/IPv6 网卡。`LOCAL_PROXY_HOST` 只在 `Gateway` 中读取，但 `Gateway` 没有被主程序使用，仅被测试调用。

**复现证据：**

临时回归测试配置：

```go
cfg.ProxyHost = "127.0.0.1"
```

实际监听地址为：

```text
[::]:44103
```

**影响：**

- 用户以为代理仅监听本机，实际可能暴露到公网。
- 若代理设置为免密，或管理凭据泄漏，可能形成公开转发代理。
- 与 README 和安装配置描述不一致。

**建议修复：**

- 将 `cfg.ProxyHost` 传入并保存到 `PortListener`。
- 使用 `net.JoinHostPort(cfg.ProxyHost, strconv.Itoa(rule.Port))` 绑定。
- 默认值保持 `127.0.0.1`。
- 需要公网监听时要求显式配置，并在启动日志中打印实际监听地址。
- 删除或统一未使用的旧 `Gateway` 实现。

**验收标准：**

- `LOCAL_PROXY_HOST=127.0.0.1` 时 `ss -lntp` 仅显示 `127.0.0.1:port`。
- 从另一台主机无法建立代理连接。
- 配置为 `0.0.0.0` 或 `::` 时才允许外部连接。

### P0-02 Go 工具链漏洞

**位置：**

- [go.mod](go.mod#L3)
- [Dockerfile](Dockerfile#L1)
- [deploy/workflows/release.yml](deploy/workflows/release.yml#L23)
- [install.sh](install.sh#L147)

**问题：**

基线 Go 1.22.6 的 `govulncheck` 报告 35 个代码可达的标准库漏洞，涉及：

- `crypto/tls`
- `net/http`
- `net/url`
- `crypto/x509`
- `encoding/asn1`
- `encoding/pem`
- `os/exec`

发布工作流、Dockerfile 和本地工具链仍使用 Go 1.22 系列。

**修复验证：**

使用临时安装的 Go 1.25.13 重新执行：

```bash
go test ./...
go test -shuffle=on -count=10 ./...
go vet ./...
govulncheck ./...
```

测试全部通过，漏洞扫描结果为 0。四架构构建也全部成功。

**建议修复：**

- 将 `go.mod` 的最低版本或 `toolchain` 升级至已修复版本。
- 将 Dockerfile 更新为 `golang:1.25.13-alpine` 或后续安全补丁版本。
- 将 GitHub Actions `setup-go` 更新至同一版本。
- 安装脚本检测已安装 Go 版本，过低时下载受支持版本。
- 重新构建并重新发布所有架构二进制。

**验收标准：**

- 发布构建使用的 Go 版本明确可追溯。
- `govulncheck ./...` 无可达漏洞。
- `go version -m` 显示发布二进制的工具链版本已更新。

### P0-03 Web 控制台存储型 XSS

**位置：**

- [web/dist/index.html](web/dist/index.html#L1736)
- [web/dist/index.html](web/dist/index.html#L1863)
- [web/dist/index.html](web/dist/index.html#L2055)
- [web/dist/index.html](web/dist/index.html#L2077)
- [web/dist/index.html](web/dist/index.html#L2674)

**问题：**

多个渲染函数将服务端返回的数据直接拼入 `innerHTML`，没有 HTML 转义。数据来源包括：

- VPNGate CSV 中的 `HostName`、`CountryLong`、`Message`、`Operator`。
- IP 富化接口返回的 `ISP`、`City`、`Region`。
- OpenVPN stdout 生成的日志消息。
- 动态组名称、屏蔽原因等用户输入。

例如日志渲染直接执行：

```javascript
row.innerHTML = `... ${entry.message}`;
```

**影响：**

恶意 VPNGate 节点、被污染的 IP 富化响应或 OpenVPN 日志可注入 HTML 和脚本。脚本将在管理页面同源执行，可调用已认证的管理 API。

**建议修复：**

- 所有动态文本使用 `textContent` 或经过严格 HTML 转义后再插入。
- 避免使用带用户数据的 `innerHTML`。
- 对 `onclick` 内插字符串改用事件监听器，避免 JS 字符串注入。
- 增加 `Content-Security-Policy`，至少禁止内联脚本和外部脚本。
- 对服务端输入字段增加长度和字符限制。

**验收标准：**

- 节点字段和日志注入 `<img src=x onerror=...>` 后只显示为文本。
- 浏览器无脚本执行、无外连请求。
- CSP 生效且现有 UI 功能不回归。

## 7. P1 原始问题与修复方案

### P1-01 畸形 CSV 导致 panic

**位置：**

- [pkg/nodes/parser.go](pkg/nodes/parser.go#L64)
- [pkg/nodes/parser.go](pkg/nodes/parser.go#L118)

`csv.Reader.FieldsPerRecord` 被设置为 `-1`，但解析逻辑直接访问固定列，例如：

```go
ip := strings.TrimSpace(record[colIdx["IP"]])
```

当 CSV 表头完整、数据行字段不足时，会触发：

```text
panic: runtime error: index out of range [1] with length 1
```

该问题已在临时回归测试中稳定复现。由于数据来自外部镜像源，恶意或损坏上游数据可能导致服务中断。

**建议：**

- 每行读取后先检查长度是否大于所有必需列的最大索引。
- 对短行执行跳过或返回错误，禁止直接索引。
- 增加异常 CSV 表驱动测试。

### P1-02 隧道设备槽位复用竞态

**位置：**

- [pkg/tunnel/pool.go](pkg/tunnel/pool.go#L407)
- [pkg/tunnel/pool.go](pkg/tunnel/pool.go#L414)
- [pkg/tunnel/pool.go](pkg/tunnel/pool.go#L428)
- [pkg/tunnel/pool.go](pkg/tunnel/pool.go#L444)
- [pkg/tunnel/pool.go](pkg/tunnel/pool.go#L312)

`StopTunnel()` 删除隧道并释放设备索引，但监控协程可能尚未完成 `cmd.Wait()`、策略路由清理和接口清理。快速轮换时，新隧道可能复用同一个 `tunN`，随后旧协程删除新隧道的路由规则。

**建议：**

- 以 `cmd.Wait()` 完成作为进程退出屏障。
- 使用每个设备的引用计数或 generation ID，防止旧协程清理新设备。
- 只有确认进程退出、路由和文件清理完成后才释放设备槽位。
- 增加并发启动、停止和替换测试。

### P1-03 自定义代理凭据权限过宽

**位置：**

- [pkg/config/config.go](pkg/config/config.go#L116)
- [pkg/proxy/listener.go](pkg/proxy/listener.go#L23)
- [pkg/proxy/manager.go](pkg/proxy/manager.go#L81)

`PortRule` 包含 `AuthPass`，而 `port_rules.json` 使用 `0644` 写入。数据目录由 `0755` 创建，本地其他用户可能读取自定义代理密码。

**建议：**

- 数据目录改为 `0700` 或 `0750`。
- `port_rules.json` 改为 `0600`。
- 其他状态文件按敏感程度至少调整为 `0600`。
- 如果允许，代理密码应使用独立的加密或密钥管理方案。

### P1-04 解锁结果数据竞争

**位置：**

- [pkg/server/routes.go](pkg/server/routes.go#L504)
- [pkg/server/routes.go](pkg/server/routes.go#L523)
- [pkg/tunnel/model.go](pkg/tunnel/model.go#L112)

后台探测完成后直接执行：

```go
t.Unlock = res
```

`Tunnel.Snapshot()` 在 `t.mu.RLock()` 下读取 `t.Unlock`，写路径没有加锁，构成数据竞争。

**建议：**

- 通过 `Tunnel` 方法在写锁内更新 `Unlock`。
- 返回 `Unlock` 时深拷贝。
- 在可运行 race detector 的环境重新执行 `go test -race ./...`。

### P1-05 启动时终止所有 OpenVPN 进程

**位置：**

- [cmd/aimilivpn/main.go](cmd/aimilivpn/main.go#L44)
- [pkg/vpn/route_linux.go](pkg/vpn/route_linux.go#L30)

主程序启动时调用 `killall -9 openvpn`，会终止同一宿主机上与 AimiliVPN 无关的 OpenVPN 进程。

**建议：**

- 删除全局 `killall`。
- 只清理本程序持久化记录的 PID、进程组和隧道配置文件。
- 使用启动锁和 PID 文件避免重复实例。
- 清理前校验进程命令行、启动时间和所属进程组。

### P1-06 管理入口明文 HTTP

**位置：**

- [pkg/config/config.go](pkg/config/config.go#L150)
- [pkg/server/middleware.go](pkg/server/middleware.go#L35)
- [install.sh](install.sh#L306)

默认 `UI_HOST=::`，安装脚本写入 `UI_HOST=::`，管理界面使用 HTTP Basic Auth。直接暴露公网时，账号、密码和会话数据可能被链路监听。

**建议：**

- 默认绑定 `127.0.0.1`，由反向代理提供 TLS。
- 或内置 TLS、自动证书和 HSTS。
- 若必须公网 HTTP，至少增加明确的启动警告和部署检测。
- 增加登录限速、失败锁定和审计。

### P1-07 安装包未校验 SHA-256

**位置：**

- [install.sh](install.sh#L98)
- [install.sh](install.sh#L113)
- [deploy/workflows/release.yml](deploy/workflows/release.yml#L44)

发布流程生成 `SHA256SUMS.txt`，但安装器下载二进制后仅检查 ELF 文件头，没有校验发布哈希或签名。第三方加速源被篡改时无法发现。

**建议：**

- 从固定可信地址下载 `SHA256SUMS.txt`。
- 下载后强制校验 SHA-256。
- 更完善的方式是校验 Sigstore、minisign 或 GPG 签名。
- 校验失败时立即中止安装或更新。

## 8. P2 原始问题与修复方案

### P2-01 IPv6 节点探测地址错误

[pkg/nodes/pool.go](pkg/nodes/pool.go#L307) 使用：

```go
addr := fmt.Sprintf("%s:%d", target.IP, target.Port)
```

IPv6 地址会形成无效的 `host:port`。Go 1.25 `go vet` 已报告该问题。

修复方式：

```go
addr := net.JoinHostPort(target.IP, strconv.Itoa(target.Port))
```

### P2-02 全局 `rp_filter` 状态泄漏

[pkg/tunnel/pool.go](pkg/tunnel/pool.go#L460) 会设置：

```text
net.ipv4.conf.all.rp_filter=2
```

[pkg/tunnel/pool.go](pkg/tunnel/pool.go#L472) 的清理函数不会恢复原值。该设置会影响宿主机上的所有网络接口。

建议记录修改前的值，并在最后一个隧道关闭或程序退出时恢复；同时检查 `sysctl` 命令错误，不能只记录“已配置”。

### P2-03 节点拉取忽略 Context

[pkg/nodes/fetcher.go](pkg/nodes/fetcher.go#L92) 为每个数据源创建 `context.Background()`，忽略调用方传入的 `ctx`。程序退出或请求取消不能终止正在进行的拉取，多个数据源叠加时可能最长阻塞约 160 秒。

建议从调用方派生带超时的 Context，并将上游 Context 的取消信号向下传递。

### P2-04 HTTP 请求边界不足

[pkg/server/server.go](pkg/server/server.go#L134) 未设置：

- `ReadHeaderTimeout`
- `IdleTimeout`
- `MaxHeaderBytes`

多个 API 直接使用 `json.NewDecoder(r.Body)`，也没有 `http.MaxBytesReader`。

建议为普通 API 设置请求体上限，为 SSE 单独配置长连接处理，并增加请求头、空闲连接和慢请求测试。

### P2-05 SSE CORS 过宽

[pkg/server/sse.go](pkg/server/sse.go#L30) 设置：

```go
w.Header().Set("Access-Control-Allow-Origin", "*")
```

在认证关闭或管理路径被获取时，任意网页可以读取状态和日志流。建议限制允许来源，或删除该 CORS 头并仅依赖同源访问。

### P2-06 测试覆盖不足

总覆盖率为 32.5%。以下关键区域为 0% 或缺少真实运行测试：

- `pkg/config`
- `pkg/vpn`
- `pkg/stats`
- OpenVPN 真实建连、断线和故障转移
- 并发隧道快速替换
- 真实路由和防火墙环境
- 多端口代理长时间传输
- 1 小时压测和 24 小时 soak

## 9. 已通过的检查

### 9.1 节点数据源

检查的 8 个数据源当前全部返回 HTTP 200：

1. Fastly jsDelivr
2. GitHub Pages
3. GitHub Raw
4. ghproxy
5. 项目 GitHub Raw
6. cdn.jsdelivr
7. VPNGate HTTPS API
8. VPNGate HTTP API

使用 `cmd/mirror` 在工作区外执行端到端镜像生成：

- 拉取字节数：`1,333,852`
- 解析有效节点：`99`
- 生成文件 SHA-256：`df1f496ba1475b98add50d1447d030d194b26ecd620a7f98ad4c8903240b5cbf`
- 与仓库 `mirror/vpngate.csv` 哈希一致

### 9.2 隔离网络路由

在用户级 Linux network namespace 中创建 `tun0` dummy 接口并调用项目路由函数，验证：

- Table 100 默认路由成功创建。
- `oif tun0 lookup 100` 策略规则成功创建。
- 清理函数可以删除规则和路由。
- 测试未修改宿主机主路由表或 SSH 网络。

### 9.3 代理基础协议

现有测试验证了：

- SOCKS5 无认证协商。
- SOCKS5 CONNECT 和双向转发。
- HTTP CONNECT 和双向转发。
- 代理认证基础逻辑。
- Hop-by-hop 和隐私头清理。

真实多隧道绑定、认证组合、IPv6 目标、慢连接和长时间传输仍未覆盖。

### 9.4 sing-box 基础集成

当前环境 sing-box 1.14.0 可用，在线测试通过：

- `GetProtocols`
- `GetStatus`
- `ListNodes`

测试环境当前没有已配置的订阅节点。

## 10. 仍需补做的检查

以下项目未能在当前环境完成：

1. **真实 OpenVPN 建连和断线测试**

   当前系统未安装 OpenVPN，也没有可用的免密 root。未执行真实握手、路由绑定、故障转移和进程清理测试。

2. **Race Detector**

   Go 1.22.6 和 Go 1.25.13 均因当前内核 39 位 VMA 布局失败：

   ```text
   FATAL: ThreadSanitizer: unsupported VMA range
   FATAL: Found 39 - Supported 48
   ```

   需要在普通 x86_64 或受支持的 arm64 Linux VM 中重新执行：

   ```bash
   go test -race ./...
   ```

3. **1 小时压力测试**

   未执行 100/500/1000 并发连接、SSE 长连接和频繁隧道轮换压测。

4. **24 小时 Soak Test**

   未验证长期内存、goroutine、文件描述符、子进程、路由规则和网络连接是否收敛。

5. **真实公网代理出口验证**

   未验证多端口绑定后各出口公网 IP、DNS 无泄漏和长连接稳定性。

6. **容器运行验证**

   未在具备 `CAP_NET_ADMIN` 和 `/dev/net/tun` 的隔离容器中验证 Dockerfile 和路由行为。

## 11. 整改顺序

### 第一阶段：上线阻断项

1. 修复 `PortListener` 监听地址和默认绑定策略。
2. 升级所有构建环境至修复漏洞的 Go 版本。
3. 修复 Web 控制台 XSS。

### 第二阶段：高优先级稳定性与安全

4. 修复 CSV 短行 panic。
5. 修复隧道设备槽位释放和异步清理竞态。
6. 收紧凭据和状态文件权限。
7. 修复 `Tunnel.Unlock` 数据竞争。
8. 删除全局 `killall openvpn`。
9. 为管理入口提供 TLS 或默认仅监听本机。
10. 为安装和更新流程增加发行包哈希校验。

### 第三阶段：中优先级加固

11. 修复 IPv6 地址拼接。
12. 恢复 `rp_filter` 原值并检查路由命令错误。
13. 让节点拉取正确响应取消信号。
14. 增加 HTTP 请求边界、CORS 限制和安全头。
15. 补齐配置、VPN、统计和故障注入测试。

## 12. 最终验收标准

满足以下条件后可重新评估上线：

- 所有 P0 和 P1 问题关闭，并有对应回归测试。
- `govulncheck ./...` 为零可达漏洞。
- 在受支持的 Linux VM 中 `go test -race ./...` 通过。
- `LOCAL_PROXY_HOST=127.0.0.1` 时端口只监听回环地址。
- 恶意 CSV 和 XSS 样本不会导致 panic 或脚本执行。
- 快速启停 100 次后无残留 `tun` 接口、路由规则、OpenVPN 子进程和临时文件。
- 宿主机 SSH 和主路由在全部故障注入中保持可用。
- 24 小时 soak test 后内存、goroutine、文件描述符和连接数回落到合理基线。
- 安装器和更新器强制校验发布文件完整性。

## 13. 检查说明

- 本报告基于提交 `0816e23a611bf521d566e4ff46c396081a81b138`。
- 所有临时复现测试均已删除。
- 已按本报告完成项目源代码、部署配置和前端修复。
- 新增监听绑定、CSV、IPv6、文件权限、请求限制、隧道清理和 netns 回归测试。
- 临时 Go 1.25.13、staticcheck、gosec 和镜像输出位于 `/tmp`，不属于仓库内容。
- CodeGraph 索引位于 `.codegraph/`，当前 Git 工作区包含本次修复改动。
