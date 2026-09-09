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

# 5. 安装或确保 Go 编译器
ensure_go() {
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

    echo -e "${YELLOW}正在安装 Go 1.22 编译环境 (用于极速单二进制编译)...${PLAIN}"
    GO_TAR="go1.22.6.linux-${GO_ARCH}.tar.gz"
    curl -sSL "https://go.dev/dl/${GO_TAR}" | tar -xz -C /usr/local
    export PATH="/usr/local/go/bin:$PATH"
    echo 'export PATH="/usr/local/go/bin:$PATH"' >> /etc/profile
}

# 6. 生成默认配置文件
generate_default_config() {
    mkdir -p "${INSTALL_DIR}"
    mkdir -p "${INSTALL_DIR}/data"

    if [ ! -f "${CONFIG_FILE}" ]; then
        RAND_PATH=$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 8 || echo "aimili")
        RAND_PASS=$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 12 || echo "aimilivpn")

        cat > "${CONFIG_FILE}" <<EOF
# AimiliVPN 运行环境变量配置
DATA_DIR=${INSTALL_DIR}/data
UI_HOST=::
UI_PORT=8787
UI_PATH=${RAND_PATH}
UI_USERNAME=admin
UI_PASSWORD=${RAND_PASS}
LOCAL_PROXY_HOST=127.0.0.1
LOCAL_PROXY_PORT=7928
LOCAL_PROXY_MAX_CONNECTIONS=512
CHECK_INTERVAL_SECONDS=20
FETCH_INTERVAL_SECONDS=900
TARGET_VALID_NODES=5
EOF
        chmod 600 "${CONFIG_FILE}"
        echo -e "${GREEN}已生成安全初始配置 (管理密码: ${RAND_PASS}, 入口路径: /${RAND_PATH})${PLAIN}"
    fi
}

# 7. 编译并部署二进制文件
build_and_deploy() {
    echo -e "\n${YELLOW}[2/4] 正在准备 AimiliVPN 可执行程序...${PLAIN}"
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    if [ -f "${SCRIPT_DIR}/bin/aimilivpn" ]; then
        echo -e "  -> 发现本地已预编译二进制文件，直接安装部署..."
        cp -f "${SCRIPT_DIR}/bin/aimilivpn" "${BIN_PATH}"
    elif [ -f "${SCRIPT_DIR}/cmd/aimilivpn/main.go" ]; then
        echo -e "  -> 正在从当前源码编译目标二进制程序..."
        ensure_go
        cd "${SCRIPT_DIR}"
        CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/aimilivpn
    else
        echo -e "  -> 正在从 GitHub 官方仓库拉取最新源码并构建..."
        ensure_go
        TMP_DIR=$(mktemp -d)
        git clone --depth 1 "${GITHUB_REPO}" "${TMP_DIR}"
        cd "${TMP_DIR}"
        CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/aimilivpn
        rm -rf "${TMP_DIR}"
    fi

    chmod +x "${BIN_PATH}"
    ln -sf "${BIN_PATH}" /usr/local/bin/aimilivpn
    ln -sf "${SCRIPT_DIR}/install.sh" /usr/local/bin/ml 2>/dev/null || ln -sf "${INSTALL_DIR}/install.sh" /usr/local/bin/ml 2>/dev/null || true
    cp -f "${BASH_SOURCE[0]}" "${INSTALL_DIR}/install.sh" 2>/dev/null || true
    chmod +x "${INSTALL_DIR}/install.sh" 2>/dev/null || true
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
    echo -e " ${BOLD}终端管理命令${PLAIN}   : 在终端随时输入 ${CYAN}ml${PLAIN} 或 ${CYAN}aimilivpn${PLAIN} 唤出管理菜单"
    echo -e "${GREEN}==================================================================${PLAIN}\n"
}

# ==============================================================================
# 终端交互式管理功能 (CLI Menu)
# ==============================================================================

show_service_status() {
    if systemctl is-active --quiet aimilivpn; then
        echo -e "${GREEN}● 运行中 (Active)${PLAIN}"
    else
        echo -e "${RED}● 已停止 (Inactive)${PLAIN}"
    fi
}

get_live_info() {
    local port=$(get_config_val "UI_PORT")
    local path=$(get_config_val "UI_PATH")
    local user=$(get_config_val "UI_USERNAME")
    local pass=$(get_config_val "UI_PASSWORD")

    # Call local API to get real-time state
    local api_res
    api_res=$(curl -s -u "${user}:${pass}" "http://127.0.0.1:${port}/api/status" 2>/dev/null || echo "")
    echo "$api_res"
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
    echo -e "\n${YELLOW}=== 修改管理后台账号与密码 ===${PLAIN}"
    local curr_user=$(get_config_val "UI_USERNAME")
    local curr_pass=$(get_config_val "UI_PASSWORD")
    echo -e "当前管理账号: ${CYAN}${curr_user}${PLAIN}"
    echo -e "当前管理密码: ${CYAN}${curr_pass}${PLAIN}"
    echo ""
    read -p "请输入新的管理账号 (直接回车保持不变): " new_user
    read -p "请输入新的管理密码 (直接回车保持不变): " new_pass

    local changed=0
    if [ -n "$new_user" ]; then
        set_config_val "UI_USERNAME" "$new_user"
        changed=1
    fi
    if [ -n "$new_pass" ]; then
        set_config_val "UI_PASSWORD" "$new_pass"
        changed=1
    fi

    if [ "$changed" = "1" ]; then
        systemctl restart aimilivpn
        echo -e "${GREEN}管理凭据已更新并重启服务生效！${PLAIN}"
    else
        echo -e "未作任何修改。"
    fi
    read -p "按回车键返回主菜单..."
}

menu_modify_ports() {
    echo -e "\n${YELLOW}=== 修改管理端口与代理端口 ===${PLAIN}"
    local curr_web_port=$(get_config_val "UI_PORT")
    local curr_proxy_port=$(get_config_val "LOCAL_PROXY_PORT")
    echo -e "当前 Web 控制台端口: ${CYAN}${curr_web_port}${PLAIN}"
    echo -e "当前 本地代理端口   : ${CYAN}${curr_proxy_port}${PLAIN}"
    echo ""
    read -p "请输入新的 Web 管理端口 (1-65535, 回车保持不变): " new_web
    read -p "请输入新的 本地代理端口 (1-65535, 回车保持不变): " new_proxy

    local changed=0
    if [ -n "$new_web" ]; then
        set_config_val "UI_PORT" "$new_web"
        changed=1
    fi
    if [ -n "$new_proxy" ]; then
        set_config_val "LOCAL_PROXY_PORT" "$new_proxy"
        changed=1
    fi

    if [ "$changed" = "1" ]; then
        systemctl restart aimilivpn
        echo -e "${GREEN}端口已更新并重启服务生效！${PLAIN}"
    else
        echo -e "未作任何修改。"
    fi
    read -p "按回车键返回主菜单..."
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
    echo -e "\n${YELLOW}正在从 GitHub 检测并构建最新版本...${PLAIN}"
    cd "${INSTALL_DIR}" 2>/dev/null || cd /tmp
    TMP_DIR=$(mktemp -d)
    git clone --depth 1 "${GITHUB_REPO}" "${TMP_DIR}"
    cd "${TMP_DIR}"
    ensure_go
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/aimilivpn
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
        rm -f /usr/local/bin/aimilivpn /usr/local/bin/ml
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
        echo -e "  ${GREEN}[7]${PLAIN} 修改管理账号/密码      ${GREEN}[8]${PLAIN} 修改 Web 与代理端口"
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
            8) menu_modify_ports ;;
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

# 如果直接带参数 menu，或已安装且未指定任何参数，则进入交互式菜单
if [ "$1" = "menu" ]; then
    main_menu
    exit 0
fi

if [ -f "${BIN_PATH}" ] && [ -f "${SERVICE_FILE}" ] && [ -z "$1" ]; then
    main_menu
    exit 0
fi

# 否则执行全流程自动化安装
detect_os
detect_arch
install_dependencies
generate_default_config
build_and_deploy
install_service
print_install_success
