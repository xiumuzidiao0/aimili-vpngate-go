package server

import (
	"fmt"
	"net"
	"strings"

	"aimili-vpngate-go/pkg/singbox"
)

// GenerateClashYAML converts sing-box inbound nodes into standard Clash Meta / Mihomo YAML configuration.
func GenerateClashYAML(nodes []singbox.Node, defaultServerHost string) string {
	var sb strings.Builder

	// 1. Header / Global config
	sb.WriteString("port: 7890\n")
	sb.WriteString("socks-port: 7891\n")
	sb.WriteString("allow-lan: false\n")
	sb.WriteString("mode: rule\n")
	sb.WriteString("log-level: info\n")
	sb.WriteString("ipv6: false\n\n")

	sb.WriteString("proxies:\n")

	type ProxyItem struct {
		Name string
		YAML string
	}

	var proxyItems []ProxyItem
	usedNames := make(map[string]int)

	for _, n := range nodes {
		// Clean server address
		srv := strings.TrimSpace(n.Address)
		if srv == "" || srv == "0.0.0.0" || srv == "127.0.0.1" || srv == "localhost" {
			srv = defaultServerHost
		}
		if srv == "" {
			continue
		}

		port := n.Port
		if port <= 0 {
			continue
		}

		// Clean unique name
		baseName := strings.TrimSpace(n.Name)
		if baseName == "" {
			baseName = strings.TrimSpace(n.Tag)
		}
		if baseName == "" {
			baseName = fmt.Sprintf("Node-%s-%d", n.Protocol, n.Port)
		}

		usedNames[baseName]++
		finalName := baseName
		if count := usedNames[baseName]; count > 1 {
			finalName = fmt.Sprintf("%s (%d)", baseName, count)
		}

		escapedName := escapeYAMLString(finalName)
		rawProto := strings.ToLower(strings.TrimSpace(n.RawProtocol))
		fullProto := strings.ToLower(strings.TrimSpace(n.Protocol))

		var py strings.Builder

		if strings.Contains(rawProto, "vless") || strings.Contains(fullProto, "vless") || strings.Contains(rawProto, "reality") || strings.Contains(fullProto, "reality") {
			py.WriteString(fmt.Sprintf("  - name: \"%s\"\n", escapedName))
			py.WriteString("    type: vless\n")
			py.WriteString(fmt.Sprintf("    server: %s\n", srv))
			py.WriteString(fmt.Sprintf("    port: %d\n", port))
			py.WriteString(fmt.Sprintf("    uuid: %s\n", n.UUID))
			netType := strings.ToLower(n.Network)
			if netType == "" {
				netType = "tcp"
			}
			py.WriteString(fmt.Sprintf("    network: %s\n", netType))
			py.WriteString("    tls: true\n")
			py.WriteString("    udp: true\n")

			flow := n.Flow
			if flow == "" && n.PBK != "" {
				flow = "xtls-rprx-vision"
			}
			if flow != "" {
				py.WriteString(fmt.Sprintf("    flow: %s\n", flow))
			}

			sni := n.SNI
			if sni == "" {
				sni = n.Host
			}
			if sni != "" {
				py.WriteString(fmt.Sprintf("    servername: %s\n", sni))
			}
			py.WriteString("    client-fingerprint: chrome\n")

			if n.PBK != "" {
				py.WriteString("    reality-opts:\n")
				py.WriteString(fmt.Sprintf("      public-key: %s\n", n.PBK))
				py.WriteString("      short-id: \"\"\n")
			}

		} else if strings.Contains(rawProto, "hy2") || strings.Contains(fullProto, "hy2") || strings.Contains(rawProto, "hysteria2") || strings.Contains(fullProto, "hysteria2") {
			py.WriteString(fmt.Sprintf("  - name: \"%s\"\n", escapedName))
			py.WriteString("    type: hysteria2\n")
			py.WriteString(fmt.Sprintf("    server: %s\n", srv))
			py.WriteString(fmt.Sprintf("    port: %d\n", port))
			pwd := n.Password
			if pwd == "" {
				pwd = n.UUID
			}
			py.WriteString(fmt.Sprintf("    password: \"%s\"\n", escapeYAMLString(pwd)))
			if n.SNI != "" {
				py.WriteString(fmt.Sprintf("    sni: %s\n", n.SNI))
			}
			py.WriteString("    skip-cert-verify: true\n")
			py.WriteString("    alpn:\n")
			py.WriteString("      - h3\n")

		} else if strings.Contains(rawProto, "tuic") || strings.Contains(fullProto, "tuic") {
			py.WriteString(fmt.Sprintf("  - name: \"%s\"\n", escapedName))
			py.WriteString("    type: tuic\n")
			py.WriteString(fmt.Sprintf("    server: %s\n", srv))
			py.WriteString(fmt.Sprintf("    port: %d\n", port))
			py.WriteString(fmt.Sprintf("    uuid: %s\n", n.UUID))
			py.WriteString(fmt.Sprintf("    password: \"%s\"\n", escapeYAMLString(n.Password)))
			if n.SNI != "" {
				py.WriteString(fmt.Sprintf("    sni: %s\n", n.SNI))
			}
			py.WriteString("    skip-cert-verify: true\n")
			py.WriteString("    alpn:\n")
			py.WriteString("      - h3\n")
			py.WriteString("    congestion-controller: bbr\n")
			py.WriteString("    reduce-rtt: true\n")
			py.WriteString("    udp-relay-mode: native\n")

		} else if strings.Contains(rawProto, "ss") || strings.Contains(fullProto, "ss") || strings.Contains(rawProto, "shadowsocks") || strings.Contains(fullProto, "shadowsocks") {
			py.WriteString(fmt.Sprintf("  - name: \"%s\"\n", escapedName))
			py.WriteString("    type: ss\n")
			py.WriteString(fmt.Sprintf("    server: %s\n", srv))
			py.WriteString(fmt.Sprintf("    port: %d\n", port))
			cipher := n.SSMethod
			if cipher == "" {
				cipher = "2022-blake3-aes-128-gcm"
			}
			py.WriteString(fmt.Sprintf("    cipher: %s\n", cipher))
			py.WriteString(fmt.Sprintf("    password: \"%s\"\n", escapeYAMLString(n.Password)))
			py.WriteString("    udp: true\n")

		} else if strings.Contains(rawProto, "trojan") || strings.Contains(fullProto, "trojan") {
			py.WriteString(fmt.Sprintf("  - name: \"%s\"\n", escapedName))
			py.WriteString("    type: trojan\n")
			py.WriteString(fmt.Sprintf("    server: %s\n", srv))
			py.WriteString(fmt.Sprintf("    port: %d\n", port))
			pwd := n.Password
			if pwd == "" {
				pwd = n.UUID
			}
			py.WriteString(fmt.Sprintf("    password: \"%s\"\n", escapeYAMLString(pwd)))
			sni := n.SNI
			if sni == "" {
				sni = n.Host
			}
			if sni != "" {
				py.WriteString(fmt.Sprintf("    sni: %s\n", sni))
			}
			py.WriteString("    skip-cert-verify: true\n")
			py.WriteString("    udp: true\n")

		} else if strings.Contains(rawProto, "vmess") || strings.Contains(fullProto, "vmess") {
			py.WriteString(fmt.Sprintf("  - name: \"%s\"\n", escapedName))
			py.WriteString("    type: vmess\n")
			py.WriteString(fmt.Sprintf("    server: %s\n", srv))
			py.WriteString(fmt.Sprintf("    port: %d\n", port))
			py.WriteString(fmt.Sprintf("    uuid: %s\n", n.UUID))
			py.WriteString("    alterId: 0\n")
			py.WriteString("    cipher: auto\n")
			py.WriteString("    udp: true\n")
			netType := strings.ToLower(n.Network)
			if netType == "" {
				netType = "tcp"
			}
			py.WriteString(fmt.Sprintf("    network: %s\n", netType))
			if netType == "ws" {
				py.WriteString("    ws-opts:\n")
				path := n.Path
				if path == "" {
					path = "/"
				}
				py.WriteString(fmt.Sprintf("      path: %s\n", path))
				h := n.Host
				if h == "" {
					h = n.SNI
				}
				if h != "" {
					py.WriteString("      headers:\n")
					py.WriteString(fmt.Sprintf("        Host: %s\n", h))
				}
			}
		}

		if py.Len() > 0 {
			proxyItems = append(proxyItems, ProxyItem{
				Name: finalName,
				YAML: py.String(),
			})
		}
	}

	if len(proxyItems) == 0 {
		sb.WriteString("  # 暂无已配置的 sing-box 代理节点\n")
	} else {
		for _, pi := range proxyItems {
			sb.WriteString(pi.YAML)
		}
	}

	// 2. Proxy Groups
	sb.WriteString("\nproxy-groups:\n")

	// 2.1 节点选择 (Manual select)
	sb.WriteString("  - name: \"🚀 节点选择\"\n")
	sb.WriteString("    type: select\n")
	sb.WriteString("    proxies:\n")
	sb.WriteString("      - \"♻️ 自动选择\"\n")
	sb.WriteString("      - \"⚡ 故障转移\"\n")
	sb.WriteString("      - \"DIRECT\"\n")
	for _, pi := range proxyItems {
		sb.WriteString(fmt.Sprintf("      - \"%s\"\n", escapeYAMLString(pi.Name)))
	}

	// 2.2 自动选择 (URL Test)
	sb.WriteString("\n  - name: \"♻️ 自动选择\"\n")
	sb.WriteString("    type: url-test\n")
	sb.WriteString("    url: http://www.gstatic.com/generate_204\n")
	sb.WriteString("    interval: 300\n")
	sb.WriteString("    tolerance: 50\n")
	sb.WriteString("    proxies:\n")
	if len(proxyItems) == 0 {
		sb.WriteString("      - \"DIRECT\"\n")
	} else {
		for _, pi := range proxyItems {
			sb.WriteString(fmt.Sprintf("      - \"%s\"\n", escapeYAMLString(pi.Name)))
		}
	}

	// 2.3 故障转移 (Fallback)
	sb.WriteString("\n  - name: \"⚡ 故障转移\"\n")
	sb.WriteString("    type: fallback\n")
	sb.WriteString("    url: http://www.gstatic.com/generate_204\n")
	sb.WriteString("    interval: 300\n")
	sb.WriteString("    proxies:\n")
	if len(proxyItems) == 0 {
		sb.WriteString("      - \"DIRECT\"\n")
	} else {
		for _, pi := range proxyItems {
			sb.WriteString(fmt.Sprintf("      - \"%s\"\n", escapeYAMLString(pi.Name)))
		}
	}

	// 2.4 漏网之鱼
	sb.WriteString("\n  - name: \"🐟 漏网之鱼\"\n")
	sb.WriteString("    type: select\n")
	sb.WriteString("    proxies:\n")
	sb.WriteString("      - \"🚀 节点选择\"\n")
	sb.WriteString("      - \"DIRECT\"\n")

	// 3. Routing Rules
	sb.WriteString("\nrules:\n")
	sb.WriteString("  - DOMAIN-SUFFIX,local,DIRECT\n")
	sb.WriteString("  - IP-CIDR,127.0.0.0/8,DIRECT,no-resolve\n")
	sb.WriteString("  - IP-CIDR,172.16.0.0/12,DIRECT,no-resolve\n")
	sb.WriteString("  - IP-CIDR,192.168.0.0/16,DIRECT,no-resolve\n")
	sb.WriteString("  - IP-CIDR,10.0.0.0/8,DIRECT,no-resolve\n")
	sb.WriteString("  - GEOIP,LAN,DIRECT,no-resolve\n")
	sb.WriteString("  - GEOIP,CN,DIRECT\n")
	sb.WriteString("  - MATCH,🚀 节点选择\n")

	return sb.String()
}

func escapeYAMLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

// extractHostFromRequest gets the clean host/domain/IP from an incoming HTTP request
func extractHostFromRequest(host string) string {
	h := strings.TrimSpace(host)
	if strings.Contains(h, ":") {
		if sh, _, err := net.SplitHostPort(h); err == nil {
			return sh
		}
	}
	return h
}
