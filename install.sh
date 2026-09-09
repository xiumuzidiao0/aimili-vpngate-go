#!/usr/bin/env bash
set -e

# ==============================================================================
# AimiliVPN (Go 高性能版) 一键安装与终端管理脚本
# 支持系统: Ubuntu / Debian / CentOS / RHEL / AlmaLinux / Rocky / Fedora / Alpine
# ==============================================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;36m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
PLAIN='\033[0m'
BOLD='\033[1m'

INSTALL_DIR="/opt/aimilivpn"
BIN_PATH="${INSTALL_DIR}/aimilivpn"
CONFIG_FILE="${INSTALL_DIR}/config.env"
SERVICE_FILE="/etc/systemd/system/aimilivpn.service"
GITHUB_REPO="https://github.com/xiumuzidiao0/aimili-vpngate-go.git"

# 1. 检查 Root 权限
check_root() {
    if [ "$(id -u)" != "0" ]; then
        echo -e "${RED}错误: 必须以 root 权限运行此脚本。请使用: sudo bash $0${PLAIN}"
        exit 1
    fi
}

# 2. 检查操作系统包管理器
detect_os() {
    OS_TYPE=""
    PKG_MGR=""
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_TYPE=$ID
    fi

    case "$OS_TYPE" in
        ubuntu|debian)
            PKG_MGR="apt-get"
            export DEBIAN_FRONTEND=noninteractive
            ;;
        alpine)
            PKG_MGR="apk"
            ;;
        centos|rhel|rocky|almalinux|fedora|ol|amzn)
            if command -v dnf >/dev/null 2>&1; then
                PKG_MGR="dnf"
            else
                PKG_MGR="yum"
            fi
            ;;
        *)
            echo -e "${RED}错误: 不支持的操作系统 ($OS_TYPE)！${PLAIN}"
            exit 1
            ;;
    esac
}

# 3. 检测 CPU 架构
detect_arch() {
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64|amd64)
            GO_ARCH="amd64"
            ;;
        aarch64|arm64)
            GO_ARCH="arm64"
            ;;
        i386|i686)
            GO_ARCH="386"
            ;;
        armv7l|armv7)
            GO_ARCH="armv6l"
            ;;
        *)
            echo -e "${RED}错误: 不受支持的 CPU 架构: $ARCH${PLAIN}"
            exit 1
            ;;
    esac
}

# 4. 安装基础依赖
install_dependencies() {
    echo -e "\n${YELLOW}[1/4] 正在安装系统基础依赖...${PLAIN}"
    if [ "$PKG_MGR" = "apt-get" ]; then
        apt-get update -q || true
        apt-get install -y openvpn curl git ca-certificates iptables iproute2 psmisc
    elif [ "$PKG_MGR" = "apk" ]; then
        apk update || true
        apk add openvpn curl git ca-certificates iptables iproute2 psmisc bash
    elif [ "$PKG_MGR" = "dnf" ] || [ "$PKG_MGR" = "yum" ]; then
        if [ "$OS_TYPE" != "fedora" ] && [ "$OS_TYPE" != "amzn" ]; then
            $PKG_MGR install -y epel-release || true
        fi
        $PKG_MGR install -y openvpn curl git ca-certificates iptables iproute psmisc || \
        $PKG_MGR install -y openvpn curl git ca-certificates iptables iproute2 psmisc
    fi
}

# 5. 安装或确保 Go 编译环境
ensure_go() {
    detect_arch
    if command -v go >/dev/null 2>&1; then
        echo -e "${GREEN}检测到系统中已安装 Go: $(go version)${PLAIN}"
        return 0
    fi

    if [ -x "/usr/local/go/bin/go" ]; then
        export PATH="/usr/local/go/bin:$PATH"
        return 0
    fi

    if [ -x "$HOME/.local/go/bin/go" ]; then
        export PATH="$HOME/.local/go/bin:$PATH"
        return 0
    fi

    echo -e "${YELLOW}正在安装 Go 1.22 编译环境 (架构: ${GO_ARCH})...${PLAIN}"
    GO_TAR="go1.22.6.linux-${GO_ARCH}.tar.gz"
    if ! curl -sSL -f "https://go.dev/dl/${GO_TAR}" | tar -xz -C /usr/local; then
        echo -e "${YELLOW}正在尝试从国内与高可用镜像源下载 Go 1.22...${PLAIN}"
        curl -sSL -f "https://golang.google.cn/dl/${GO_TAR}" | tar -xz -C /usr/local || true
    fi
    export PATH="/usr/local/go/bin:$PATH"
    echo 'export PATH="/usr/local/go/bin:$PATH"' >> /etc/profile
}

# 辅助生成随机字符串与密码
rand_str() {
    local len="${1:-8}"
    head -c 32 /dev/urandom | tr -dc 'A-Za-z0-9' | head -c "$len" || echo "aimili"
}

rand_pass() {
    local len="${1:-12}"
    head -c 32 /dev/urandom | tr -dc 'A-Za-z0-9' | head -c "$len" || echo "aimilivpn123"
}

# 兼容管道执行与终端直接执行的用户输入函数
prompt_input() {
    local prompt_msg="$1"
    local default_val="$2"
    local result_var="$3"
    local input_val=""

    if [ -t 0 ]; then
        read -p "$prompt_msg" input_val
    elif [ -c /dev/tty ]; then
        read -p "$prompt_msg" input_val </dev/tty
    fi

    if [ -z "$input_val" ]; then
        eval "$result_var=\"$default_val\""
    else
        eval "$result_var=\"$input_val\""
    fi
}

# 6. 交互式自定义配置与保存
configure_install_params() {
    mkdir -p "${INSTALL_DIR}"
    mkdir -p "${INSTALL_DIR}/data"

    local def_web_port="8787"
    local def_path=$(rand_str 8)
    local def_user="admin"
    local def_pass=$(rand_pass 12)
    local def_proxy_port="7928"

    # 如果已有配置文件，优先以已有值作为默认
    if [ -f "${CONFIG_FILE}" ]; then
        local exist_web=$(get_config_val "UI_PORT")
        local exist_path=$(get_config_val "UI_PATH")
        local exist_user=$(get_config_val "UI_USERNAME")
        local exist_pass=$(get_config_val "UI_PASSWORD")
        local exist_proxy=$(get_config_val "LOCAL_PROXY_PORT")

        [ -n "$exist_web" ] && def_web_port="$exist_web"
        [ -n "$exist_path" ] && def_path="$exist_path"
        [ -n "$exist_user" ] && def_user="$exist_user"
        [ -n "$exist_pass" ] && def_pass="$exist_pass"
        [ -n "$exist_proxy" ] && def_proxy_port="$exist_proxy"
    fi

    echo -e "\n${BLUE}==================================================================${PLAIN}"
    echo -e "${BLUE}             AimiliVPN 初始部署参数自定义配置                     ${PLAIN}"
    echo -e "${BLUE}  (直接按回车可全部采用括号内的推荐默认值或安全随机值)             ${PLAIN}"
    echo -e "${BLUE}==================================================================${PLAIN}"

    # 1. Web 端口
    while true; do
        prompt_input "1. 请输入 Web 管理控制台端口 [1-65535] (默认: ${def_web_port}): " "${def_web_port}" custom_web_port
        if [[ "$custom_web_port" =~ ^[0-9]+$ ]] && [ "$custom_web_port" -ge 1 ] && [ "$custom_web_port" -le 65535 ]; then
            break
        else
            echo -e "${RED}错误: 端口必须是 1 至 65535 之间的纯数字！${PLAIN}"
        fi
    done

    # 2. 安全访问路径
    prompt_input "2. 请输入 Web 后台安全访问路径 (默认: ${def_path}): " "${def_path}" custom_path
    custom_path=$(echo "${custom_path}" | tr -d '/' | tr -d ' ')
    [ -z "$custom_path" ] && custom_path="${def_path}"

    # 3. 管理员账号
    prompt_input "3. 请输入 Web 管理员账号 (默认: ${def_user}): " "${def_user}" custom_user
    [ -z "$custom_user" ] && custom_user="${def_user}"

    # 4. 管理员密码
    prompt_input "4. 请输入 Web 管理员密码 (默认随机密码: ${def_pass}): " "${def_pass}" custom_pass
    [ -z "$custom_pass" ] && custom_pass="${def_pass}"

    # 5. 代理出站端口
    while true; do
        prompt_input "5. 请输入 本地统一代理监听端口 [1-65535] (默认: ${def_proxy_port}): " "${def_proxy_port}" custom_proxy_port
        if [[ "$custom_proxy_port" =~ ^[0-9]+$ ]] && [ "$custom_proxy_port" -ge 1 ] && [ "$custom_proxy_port" -le 65535 ]; then
            if [ "$custom_proxy_port" = "$custom_web_port" ]; then
                echo -e "${RED}错误: 代理端口不能与 Web 管理端口 ($custom_web_port) 相同！${PLAIN}"
            else
                break
            fi
        else
            echo -e "${RED}错误: 端口必须是 1 至 65535 之间的纯数字！${PLAIN}"
        fi
    done

    echo -e "\n${GREEN}------------------- 您配置的参数确认 -------------------${PLAIN}"
    echo -e " Web 管理端口   : ${CYAN}${custom_web_port}${PLAIN}"
    echo -e " 安全访问路径   : ${CYAN}/${custom_path}${PLAIN}"
    echo -e " 管理员账号     : ${YELLOW}${custom_user}${PLAIN}"
    echo -e " 管理员密码     : ${YELLOW}${custom_pass}${PLAIN}"
    echo -e " 本地代理端口   : ${CYAN}${custom_proxy_port}${PLAIN}"
    echo -e "${GREEN}-------------------------------------------------------${PLAIN}"

    cat > "${CONFIG_FILE}" <<EOF
# AimiliVPN 运行环境变量配置
DATA_DIR=${INSTALL_DIR}/data
UI_HOST=::
UI_PORT=${custom_web_port}
UI_PATH=${custom_path}
UI_USERNAME=${custom_user}
UI_PASSWORD=${custom_pass}
LOCAL_PROXY_HOST=127.0.0.1
LOCAL_PROXY_PORT=${custom_proxy_port}
LOCAL_PROXY_MAX_CONNECTIONS=512
CHECK_INTERVAL_SECONDS=20
FETCH_INTERVAL_SECONDS=900
TARGET_VALID_NODES=5
EOF
    chmod 600 "${CONFIG_FILE}"
}

# 7. 编译并部署二进制文件
build_and_deploy() {
    echo -e "\n${YELLOW}[2/4] 正在准备 AimiliVPN 可执行程序...${PLAIN}"
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local dl_ok=0

    # 1. 优先尝试极速下载已发布的官方跨平台静态二进制文件 (约 5.8MB, 无需等待编译)
    local release_url="https://github.com/xiumuzidiao0/aimili-vpngate-go/releases/download/v2.0.1/aimilivpn_linux_${GO_ARCH}"
    echo -e "  -> 正在检测并极速下载已发布的官方静态二进制包 (${GO_ARCH})..."
    if curl -sSL -f -m 30 "${release_url}" -o "${BIN_PATH}" && [ -s "${BIN_PATH}" ]; then
        echo -e "${GREEN}  -> 二进制预编译包下载完成！${PLAIN}"
        dl_ok=1
    fi

    # 2. 若无法下载发行包，自动降级至就地编译
    if [ "$dl_ok" = "0" ]; then
        if [ -f "${SCRIPT_DIR}/bin/aimilivpn" ]; then
            echo -e "  -> 发现本地已预编译二进制文件，直接安装部署..."
            cp -f "${SCRIPT_DIR}/bin/aimilivpn" "${BIN_PATH}"
        elif [ -f "${SCRIPT_DIR}/cmd/aimilivpn/main.go" ]; then
            echo -e "  -> 正在从本地源码就地编译目标程序 (约需 10-30 秒)..."
            ensure_go
            cd "${SCRIPT_DIR}"
            CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/aimilivpn
        else
            echo -e "  -> 正在从 GitHub 官方仓库拉取最新源码并构建..."
            ensure_go
            TMP_DIR=$(mktemp -d)
            git clone --depth 1 "${GITHUB_REPO}" "${TMP_DIR}"
            cd "${TMP_DIR}"
            echo -e "  -> 正在就地编译二进制程序 (低配 VPS 首次编译约需 20~40 秒，请耐心稍候)..."
            CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/aimilivpn
            rm -rf "${TMP_DIR}"
        fi
    fi

    chmod +x "${BIN_PATH}"

    # 部署本地兜底镜像
    mkdir -p "${INSTALL_DIR}/mirror"
    if [ -f "${SCRIPT_DIR}/mirror/vpngate.csv" ]; then
        cp -f "${SCRIPT_DIR}/mirror/vpngate.csv" "${INSTALL_DIR}/mirror/" 2>/dev/null || true
    fi
    if [ ! -f "${INSTALL_DIR}/mirror/vpngate.csv" ]; then
        echo -e "  -> 正在下载初始节点快照镜像..."
        curl -sSL -m 10 "https://cdn.jsdelivr.net/gh/baoweise-bot/aimili-vpngate@main/mirror/vpngate.csv" -o "${INSTALL_DIR}/mirror/vpngate.csv" 2>/dev/null || true
    fi

    # 确保完整的管理脚本安装到 /opt/aimilivpn/install.sh
    if [ -f "${BASH_SOURCE[0]}" ] && [ -s "${BASH_SOURCE[0]}" ]; then
        cp -f "${BASH_SOURCE[0]}" "${INSTALL_DIR}/install.sh"
    else
        echo -e "  -> 正在下载本地管理脚本至 ${INSTALL_DIR}/install.sh ..."
        curl -sSL "https://raw.githubusercontent.com/xiumuzidiao0/aimili-vpngate-go/main/install.sh" -o "${INSTALL_DIR}/install.sh"
    fi
    chmod +x "${INSTALL_DIR}/install.sh"

    # 创建全局快捷命令 ml 和 aimili 到 /usr/bin 与 /usr/local/bin
    cat > /usr/bin/ml <<'EOF'
#!/usr/bin/env bash
exec bash /opt/aimilivpn/install.sh menu "$@"
EOF
    chmod +x /usr/bin/ml
    cp -f /usr/bin/ml /usr/local/bin/ml 2>/dev/null || true
    cp -f /usr/bin/ml /usr/bin/aimili 2>/dev/null || true

    ln -sf "${BIN_PATH}" /usr/bin/aimilivpn 2>/dev/null || true
    ln -sf "${BIN_PATH}" /usr/local/bin/aimilivpn 2>/dev/null || true
}

# 8. 安装 systemd 服务
install_service() {
    echo -e "\n${YELLOW}[3/4] 正在配置系统守护进程服务...${PLAIN}"
    cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=AimiliVPN Go Gateway Service
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${CONFIG_FILE}
ExecStart=${BIN_PATH}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable aimilivpn.service
    systemctl restart aimilivpn.service
}

# 9. 读取配置中的指定项
get_config_val() {
    local key="$1"
    if [ -f "${CONFIG_FILE}" ]; then
        grep "^${key}=" "${CONFIG_FILE}" | cut -d'=' -f2- | tr -d '\r\n'
    fi
}

# 10. 更新配置中的指定项
set_config_val() {
    local key="$1"
    local val="$2"
    if [ -f "${CONFIG_FILE}" ]; then
        if grep -q "^${key}=" "${CONFIG_FILE}"; then
            sed -i "s|^${key}=.*|${key}=${val}|g" "${CONFIG_FILE}"
        else
            echo "${key}=${val}" >> "${CONFIG_FILE}"
        fi
    fi
}

# 11. 获取服务器公网 IP
get_public_ip() {
    local ip
    ip=$(curl -s4m 3 https://api.ipify.org || curl -s4m 3 https://ip.sb || curl -s4m 3 https://checkip.amazonaws.com || echo "你的VPS公网IP")
    echo "$ip"
}

# 12. 打印安装成功面板
print_install_success() {
    local ip=$(get_public_ip)
    local port=$(get_config_val "UI_PORT")
    local path=$(get_config_val "UI_PATH")
    local user=$(get_config_val "UI_USERNAME")
    local pass=$(get_config_val "UI_PASSWORD")
    local proxy_port=$(get_config_val "LOCAL_PROXY_PORT")

    echo -e "\n${GREEN}==================================================================${PLAIN}"
    echo -e "${GREEN}             🎉 AimiliVPN (Go 高性能版) 安装部署完成！              ${PLAIN}"
    echo -e "${GREEN}==================================================================${PLAIN}"
    echo -e " ${BOLD}Web 管理控制台${PLAIN} : ${CYAN}http://${ip}:${port}/${path}${PLAIN}"
    echo -e " ${BOLD}管理账号${PLAIN}       : ${YELLOW}${user}${PLAIN}"
    echo -e " ${BOLD}管理密码${PLAIN}       : ${YELLOW}${pass}${PLAIN}"
    echo -e " ${BOLD}本地自适应代理${PLAIN} : ${GREEN}127.0.0.1:${proxy_port}${PLAIN} (HTTP/HTTPS/SOCKS5 单端口)"
    echo -e " ${BOLD}终端管理命令${PLAIN}   : 在终端随时输入 ${CYAN}ml${PLAIN} 唤出管理控制中心"
    echo -e "${GREEN}==================================================================${PLAIN}\n"
}

# ==============================================================================
# 终端交互式管理功能 (CLI Menu)
# ==============================================================================

show_service_status() {
    if systemctl is-active --quiet aimilivpn 2>/dev/null; then
        echo -e "${GREEN}● 运行中 (Active)${PLAIN}"
    else
        echo -e "${RED}● 已停止 (Inactive)${PLAIN}"
    fi
}

menu_start() {
    systemctl start aimilivpn
    echo -e "${GREEN}已发送启动指令。${PLAIN}"
    sleep 1
}

menu_stop() {
    systemctl stop aimilivpn
    echo -e "${YELLOW}已发送停止指令。${PLAIN}"
    sleep 1
}

menu_restart() {
    systemctl restart aimilivpn
    echo -e "${GREEN}已发送重启指令。${PLAIN}"
    sleep 1
}

menu_logs() {
    echo -e "${CYAN}正在查看实时运行日志 (按 Ctrl+C 退出)...${PLAIN}"
    journalctl -u aimilivpn -f -n 50
}

menu_modify_credentials() {
    while true; do
        clear
        local curr_user=$(get_config_val "UI_USERNAME")
        local curr_pass=$(get_config_val "UI_PASSWORD")
        echo -e "${BLUE}=======================================================${PLAIN}"
        echo -e "${BLUE}                 管理账号与密码管理                    ${PLAIN}"
        echo -e "${BLUE}=======================================================${PLAIN}"
        echo -e "  当前管理账号: ${YELLOW}${curr_user}${PLAIN}"
        echo -e "  当前管理密码: ${YELLOW}${curr_pass}${PLAIN}"
        echo -e "-------------------------------------------------------"
        echo -e "  [1] 自定义修改账号与密码"
        echo -e "  [2] 一键随机重置 12 位安全强密码"
        echo -e "  [0] 返回主菜单"
        echo -e "${BLUE}=======================================================${PLAIN}"
        read -p "请选择操作 [0-2]: " cred_choice

        case "$cred_choice" in
            1)
                echo ""
                read -p "请输入新管理账号 (回车保持不变): " new_u
                read -p "请输入新管理密码 (不能为空, 回车保持不变): " new_p
                [ -n "$new_u" ] && set_config_val "UI_USERNAME" "$new_u"
                [ -n "$new_p" ] && set_config_val "UI_PASSWORD" "$new_p"
                systemctl restart aimilivpn
                echo -e "${GREEN}账号密码已更新并重启服务！${PLAIN}"
                sleep 1.5
                ;;
            2)
                local rand_p=$(rand_pass 12)
                set_config_val "UI_PASSWORD" "$rand_p"
                systemctl restart aimilivpn
                echo -e "${GREEN}密码已成功重置为: ${YELLOW}${rand_p}${PLAIN}"
                read -p "按回车键继续..."
                ;;
            0)
                break
                ;;
            *)
                echo -e "${RED}输入无效${PLAIN}"; sleep 1
                ;;
        esac
    done
}

menu_modify_ports_and_path() {
    while true; do
        clear
        local curr_web=$(get_config_val "UI_PORT")
        local curr_proxy=$(get_config_val "LOCAL_PROXY_PORT")
        local curr_path=$(get_config_val "UI_PATH")
        local ip=$(get_public_ip)

        echo -e "${BLUE}=======================================================${PLAIN}"
        echo -e "${BLUE}               端口与后台安全路径管理                  ${PLAIN}"
        echo -e "${BLUE}=======================================================${PLAIN}"
        echo -e "  当前 Web 管理端口 : ${CYAN}${curr_web}${PLAIN}"
        echo -e "  当前 本地代理端口 : ${CYAN}${curr_proxy}${PLAIN}"
        echo -e "  当前 安全访问路径 : ${CYAN}/${curr_path}${PLAIN}"
        echo -e "  当前完整访问入口  : ${YELLOW}http://${ip}:${curr_web}/${curr_path}${PLAIN}"
        echo -e "-------------------------------------------------------"
        echo -e "  [1] 修改 Web 管理后台端口"
        echo -e "  [2] 修改 本地自适应代理端口"
        echo -e "  [3] 修改 后台安全访问路径"
        echo -e "  [4] 一键随机重新生成安全路径 (防扫描防爆破)"
        echo -e "  [0] 返回主菜单"
        echo -e "${BLUE}=======================================================${PLAIN}"
        read -p "请选择操作 [0-4]: " p_choice

        case "$p_choice" in
            1)
                read -p "请输入新的 Web 管理端口 [1-65535]: " n_web
                if [[ "$n_web" =~ ^[0-9]+$ ]] && [ "$n_web" -ge 1 ] && [ "$n_web" -le 65535 ]; then
                    if [ "$n_web" = "$curr_proxy" ]; then
                        echo -e "${RED}错误: Web 端口不能与代理端口相同！${PLAIN}"; sleep 1.5
                    else
                        set_config_val "UI_PORT" "$n_web"
                        systemctl restart aimilivpn
                        echo -e "${GREEN}Web 管理端口已更新为 $n_web 并重启生效！${PLAIN}"; sleep 1.5
                    fi
                else
                    echo -e "${RED}端口必须为 1-65535 的数字！${PLAIN}"; sleep 1.5
                fi
                ;;
            2)
                read -p "请输入新的 本地代理端口 [1-65535]: " n_proxy
                if [[ "$n_proxy" =~ ^[0-9]+$ ]] && [ "$n_proxy" -ge 1 ] && [ "$n_proxy" -le 65535 ]; then
                    if [ "$n_proxy" = "$curr_web" ]; then
                        echo -e "${RED}错误: 代理端口不能与 Web 端口相同！${PLAIN}"; sleep 1.5
                    else
                        set_config_val "LOCAL_PROXY_PORT" "$n_proxy"
                        systemctl restart aimilivpn
                        echo -e "${GREEN}本地代理端口已更新为 $n_proxy 并重启生效！${PLAIN}"; sleep 1.5
                    fi
                else
                    echo -e "${RED}端口必须为 1-65535 的数字！${PLAIN}"; sleep 1.5
                fi
                ;;
            3)
                read -p "请输入新的安全访问路径 (无需斜杠，例如 admin 或 myvpn): " n_path
                n_path=$(echo "${n_path}" | tr -d '/' | tr -d ' ')
                if [ -n "$n_path" ]; then
                    set_config_val "UI_PATH" "$n_path"
                    systemctl restart aimilivpn
                    echo -e "${GREEN}安全路径已更新为 /${n_path} 并重启生效！${PLAIN}"; sleep 1.5
                fi
                ;;
            4)
                local rand_p=$(rand_str 8)
                set_config_val "UI_PATH" "$rand_p"
                systemctl restart aimilivpn
                echo -e "${GREEN}安全访问路径已随机更新为: ${YELLOW}/${rand_p}${PLAIN}"
                read -p "按回车键继续..."
                ;;
            0)
                break
                ;;
            *)
                echo -e "${RED}输入无效${PLAIN}"; sleep 1
                ;;
        esac
    done
}

menu_switch_node() {
    local port=$(get_config_val "UI_PORT")
    local user=$(get_config_val "UI_USERNAME")
    local pass=$(get_config_val "UI_PASSWORD")

    echo -e "${CYAN}正在请求快速切换至当前延迟最低的最优节点...${PLAIN}"
    curl -s -X POST -u "${user}:${pass}" "http://127.0.0.1:${port}/api/connect" >/dev/null && \
        echo -e "${GREEN}已向守护程序发送节点切换指令！${PLAIN}" || \
        echo -e "${RED}连接失败，请确认服务已启动。${PLAIN}"
    sleep 1.5
}

menu_list_nodes() {
    local port=$(get_config_val "UI_PORT")
    local user=$(get_config_val "UI_USERNAME")
    local pass=$(get_config_val "UI_PASSWORD")

    echo -e "\n${CYAN}正在获取当前优质候选节点列表...${PLAIN}"
    curl -s -u "${user}:${pass}" "http://127.0.0.1:${port}/api/nodes" | grep -o '{"id":[^}]*}' | head -n 10 | while read -r line; do
        echo "$line"
    done
    echo ""
    read -p "按回车键返回主菜单..."
}

menu_update() {
    echo -e "\n${YELLOW}正在检测最新发行版本...${PLAIN}"
    local release_url="https://github.com/xiumuzidiao0/aimili-vpngate-go/releases/download/v2.0.1/aimilivpn_linux_${GO_ARCH}"
    if curl -sSL -f -m 30 "${release_url}" -o "${BIN_PATH}.tmp" && [ -s "${BIN_PATH}.tmp" ]; then
        mv -f "${BIN_PATH}.tmp" "${BIN_PATH}"
        chmod +x "${BIN_PATH}"
        curl -sSL "https://raw.githubusercontent.com/xiumuzidiao0/aimili-vpngate-go/main/install.sh" -o "${INSTALL_DIR}/install.sh" 2>/dev/null || true
        chmod +x "${INSTALL_DIR}/install.sh" 2>/dev/null || true
        systemctl restart aimilivpn
        echo -e "${GREEN}AimiliVPN 已成功极速更新至最新构建并重启！${PLAIN}"
        sleep 2
        return
    fi

    echo -e "  -> 正在从 GitHub 拉取源码就地重新编译..."
    cd "${INSTALL_DIR}" 2>/dev/null || cd /tmp
    TMP_DIR=$(mktemp -d)
    git clone --depth 1 "${GITHUB_REPO}" "${TMP_DIR}"
    cd "${TMP_DIR}"
    ensure_go
    echo -e "  -> 正在就地编译 (约需 20-30 秒)..."
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/aimilivpn
    cp -f "${TMP_DIR}/install.sh" "${INSTALL_DIR}/install.sh" 2>/dev/null || true
    chmod +x "${INSTALL_DIR}/install.sh" 2>/dev/null || true
    mkdir -p "${INSTALL_DIR}/mirror"
    cp -f "${TMP_DIR}/mirror/vpngate.csv" "${INSTALL_DIR}/mirror/" 2>/dev/null || true
    rm -rf "${TMP_DIR}"
    systemctl restart aimilivpn
    echo -e "${GREEN}AimiliVPN 已成功更新至最新构建并重启！${PLAIN}"
    sleep 2
}

menu_uninstall() {
    echo -e "\n${RED}警告: 即将完全卸载 AimiliVPN 服务及其配置文件！${PLAIN}"
    read -p "确认卸载吗？(y/N): " confirm
    if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
        systemctl stop aimilivpn 2>/dev/null || true
        systemctl disable aimilivpn 2>/dev/null || true
        rm -f "${SERVICE_FILE}"
        systemctl daemon-reload
        rm -f /usr/local/bin/aimilivpn /usr/bin/aimilivpn /usr/local/bin/ml /usr/bin/ml /usr/bin/aimili
        rm -rf "${INSTALL_DIR}"
        echo -e "${GREEN}AimiliVPN 已完全卸载干净。${PLAIN}"
        exit 0
    else
        echo -e "已取消卸载。"
        sleep 1
    fi
}

main_menu() {
    while true; do
        clear
        local ip=$(get_public_ip)
        local port=$(get_config_val "UI_PORT")
        local path=$(get_config_val "UI_PATH")
        local user=$(get_config_val "UI_USERNAME")
        local pass=$(get_config_val "UI_PASSWORD")
        local proxy_port=$(get_config_val "LOCAL_PROXY_PORT")

        echo -e "${BLUE}==================================================================${PLAIN}"
        echo -e "${BLUE}           ⚡ AimiliVPN (Go 高性能版) 终端控制中心                ${PLAIN}"
        echo -e "${BLUE}==================================================================${PLAIN}"
        echo -e "  ${BOLD}服务运行状态${PLAIN} : $(show_service_status)"
        echo -e "  ${BOLD}Web 控制台入口${PLAIN}: ${CYAN}http://${ip}:${port}/${path}${PLAIN}"
        echo -e "  ${BOLD}管理账号/密码${PLAIN} : ${YELLOW}${user}${PLAIN} / ${YELLOW}${pass}${PLAIN}"
        echo -e "  ${BOLD}本地代理端口${PLAIN}   : ${GREEN}127.0.0.1:${proxy_port}${PLAIN} (HTTP & SOCKS5 单端口自适应)"
        echo -e "${BLUE}------------------------------------------------------------------${PLAIN}"
        echo -e "  ${GREEN}[1]${PLAIN} 启动服务               ${GREEN}[2]${PLAIN} 停止服务"
        echo -e "  ${GREEN}[3]${PLAIN} 重启服务               ${GREEN}[4]${PLAIN} 查看实时运行日志"
        echo -e "  ${GREEN}[5]${PLAIN} 智能切换最优节点       ${GREEN}[6]${PLAIN} 查看当前候选节点列表"
        echo -e "  ${GREEN}[7]${PLAIN} 修改管理账号/密码      ${GREEN}[8]${PLAIN} 修改 Web/代理端口与安全路径"
        echo -e "  ${GREEN}[9]${PLAIN} 检查并在线更新版本     ${RED}[10]${PLAIN} 卸载 AimiliVPN"
        echo -e "  ${YELLOW}[0]${PLAIN} 退出终端管理"
        echo -e "${BLUE}==================================================================${PLAIN}"
        read -p "请输入选项 [0-10]: " choice

        case "$choice" in
            1) menu_start ;;
            2) menu_stop ;;
            3) menu_restart ;;
            4) menu_logs ;;
            5) menu_switch_node ;;
            6) menu_list_nodes ;;
            7) menu_modify_credentials ;;
            8) menu_modify_ports_and_path ;;
            9) menu_update ;;
            10) menu_uninstall ;;
            0) exit 0 ;;
            *) echo -e "${RED}输入无效，请重新选择${PLAIN}"; sleep 1 ;;
        esac
    done
}

# ==============================================================================
# 脚本主执行入口
# ==============================================================================
check_root
detect_os
detect_arch

# 如果直接带参数 menu，或已安装且未指定任何参数，则进入交互式菜单
if [ "$1" = "menu" ]; then
    main_menu
    exit 0
fi

if [ -f "${BIN_PATH}" ] && [ -f "${SERVICE_FILE}" ] && [ -z "$1" ]; then
    main_menu
    exit 0
fi

# 首次执行或指定全新安装：进入交互式参数配置与全流程安装
install_dependencies
configure_install_params
build_and_deploy
install_service
print_install_success
