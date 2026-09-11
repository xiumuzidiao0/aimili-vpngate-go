package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"aimili-vpngate-go/pkg/singbox"
	"aimili-vpngate-go/pkg/stats"
)

type AvailableOutbound struct {
	Port      int    `json:"port"`
	Addr      string `json:"addr"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	IsDefault bool   `json:"is_default"`
}

type SingBoxOverviewResponse struct {
	OK                 bool                    `json:"ok"`
	Installed          bool                    `json:"installed"`
	Status             *singbox.StatusResponse `json:"status,omitempty"`
	Nodes              []singbox.Node          `json:"nodes"`
	NodeCount          int                     `json:"node_count"`
	Subscription       *singbox.SubResponse    `json:"subscription,omitempty"`
	Protocols          []singbox.ProtocolInfo  `json:"protocols"`
	AvailableOutbounds []AvailableOutbound     `json:"available_outbounds"`
	Error              string                  `json:"error,omitempty"`
}

// resolveHTTPOutboundURL converts an input port/address into a standard HTTP proxy URL with credentials if required.
func (s *Server) resolveHTTPOutboundURL(outboundRaw string) string {
	raw := strings.TrimSpace(outboundRaw)
	if raw == "" || strings.EqualFold(raw, "direct") || strings.EqualFold(raw, "none") || strings.EqualFold(raw, "default") {
		return "direct"
	}

	var targetPort int
	clean := raw
	if strings.Contains(clean, "://") {
		parts := strings.SplitN(clean, "://", 2)
		clean = parts[1]
	}
	if strings.Contains(clean, "@") {
		parts := strings.SplitN(clean, "@", 2)
		clean = parts[1]
	}
	clean = strings.Trim(clean, "/")

	if strings.Contains(clean, ":") {
		_, pStr, err := net.SplitHostPort(clean)
		if err == nil {
			targetPort, _ = strconv.Atoi(pStr)
		}
	} else if p, err := strconv.Atoi(clean); err == nil {
		targetPort = p
	}

	if targetPort <= 0 {
		targetPort = s.cfg.ProxyPort
	}
	if targetPort <= 0 {
		targetPort = 7928
	}

	// Check authentication configuration for this port
	authMode := "none"
	authUser := ""
	authPass := ""

	if s.portMgr != nil {
		if rule := s.portMgr.GetRule(targetPort); rule != nil {
			authMode = rule.AuthMode
			authUser = rule.AuthUser
			authPass = rule.AuthPass
		} else if targetPort == s.cfg.ProxyPort {
			authMode = "default_web"
		}
	} else if targetPort == s.cfg.ProxyPort {
		authMode = "default_web"
	}

	if authMode == "default_web" {
		if s.cfg.IsUIAuthEnabled() {
			authUser = s.cfg.UIUsername
			authPass = s.cfg.UIPassword
		} else {
			authMode = "none"
		}
	}

	if (authMode == "default_web" || authMode == "custom") && authUser != "" {
		return fmt.Sprintf("http://%s:%s@127.0.0.1:%d", authUser, authPass, targetPort)
	}

	return fmt.Sprintf("http://127.0.0.1:%d", targetPort)
}

func (s *Server) getAvailableOutbounds() []AvailableOutbound {
	var list []AvailableOutbound
	seenPorts := make(map[int]bool)

	// 1. Default proxy port
	defPort := s.cfg.ProxyPort
	if defPort <= 0 {
		defPort = 7928
	}
	defAddr := s.resolveHTTPOutboundURL(fmt.Sprintf("%d", defPort))
	authNote := "免密"
	if strings.Contains(defAddr, "@") {
		authNote = "密码保护"
	}

	list = append(list, AvailableOutbound{
		Port:      defPort,
		Addr:      defAddr,
		Type:      "http",
		Label:     fmt.Sprintf("AimiliVPN 默认出口 (PORT %d - HTTP %s)", defPort, authNote),
		IsDefault: true,
	})
	seenPorts[defPort] = true

	// 2. Extra Multi-Port rules
	if s.portMgr != nil {
		for _, rule := range s.portMgr.GetRules() {
			if !seenPorts[rule.Port] && rule.Port > 0 {
				ruleAddr := s.resolveHTTPOutboundURL(fmt.Sprintf("%d", rule.Port))
				ruleAuthNote := "免密"
				if strings.Contains(ruleAddr, "@") {
					ruleAuthNote = "密码保护"
				}

				boundDesc := ""
				tCount := len(rule.BoundTunnelIDs)
				gCount := len(rule.BoundGroupIDs)
				if tCount > 0 && gCount > 0 {
					boundDesc = fmt.Sprintf(" - %d 隧道, %d 动态组", tCount, gCount)
				} else if tCount > 0 {
					boundDesc = fmt.Sprintf(" - %d 隧道", tCount)
				} else if gCount > 0 {
					boundDesc = fmt.Sprintf(" - %d 动态组", gCount)
				}
				list = append(list, AvailableOutbound{
					Port:      rule.Port,
					Addr:      ruleAddr,
					Type:      "http",
					Label:     fmt.Sprintf("多端口出口 (PORT %d - HTTP %s%s)", rule.Port, ruleAuthNote, boundDesc),
					IsDefault: false,
				})
				seenPorts[rule.Port] = true
			}
		}
	}

	// 3. Direct exit option
	list = append(list, AvailableOutbound{
		Port:      0,
		Addr:      "direct",
		Type:      "direct",
		Label:     "直连出口 (VPS 本机原生网络，无链式分流)",
		IsDefault: false,
	})

	return list
}

func (s *Server) handleSingBoxOverview(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	outbounds := s.getAvailableOutbounds()

	resp := SingBoxOverviewResponse{
		OK:                 true,
		Installed:          s.singboxClient.IsInstalled(),
		AvailableOutbounds: outbounds,
		Nodes:              []singbox.Node{},
		Protocols:          []singbox.ProtocolInfo{},
	}

	if !resp.Installed {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	ctx := r.Context()

	// Get status
	if st, err := s.singboxClient.GetStatus(ctx); err == nil {
		resp.Status = st
	}

	// Get protocols
	if pr, err := s.singboxClient.GetProtocols(ctx); err == nil {
		resp.Protocols = pr
	}

	// Get nodes
	if nds, err := s.singboxClient.ListNodes(ctx); err == nil {
		resp.Nodes = nds
		resp.NodeCount = len(nds)
	}

	// Get subscription
	if sub, err := s.singboxClient.GetSubscription(ctx); err == nil {
		resp.Subscription = sub
	}
	if resp.Subscription == nil {
		resp.Subscription = &singbox.SubResponse{
			OK:      true,
			Enabled: false,
		}
	}
	resp.Subscription.ClashSubURL = s.buildClashSubURL(r)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSingBoxStatus(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	status, err := s.singboxClient.GetStatus(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handleSingBoxProtocols(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	protos, err := s.singboxClient.GetProtocols(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "protocols": protos})
}

func (s *Server) handleSingBoxListNodes(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	nodes, err := s.singboxClient.ListNodes(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "count": len(nodes), "nodes": nodes})
}

type AddNodeRequest struct {
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	UUID     string `json:"uuid"`
	Password string `json:"password"`
	SNI      string `json:"sni"`
	Host     string `json:"host"`
	Outbound string `json:"outbound"`
}

func (s *Server) handleSingBoxAddNode(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	var req AddNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "无效的请求参数"})
		return
	}

	proto := strings.TrimSpace(req.Protocol)
	if proto == "" {
		proto = "reality"
	}

	cred := strings.TrimSpace(req.UUID)
	if cred == "" {
		cred = strings.TrimSpace(req.Password)
	}
	if cred == "" {
		cred = "auto"
	}

	sni := strings.TrimSpace(req.SNI)
	if sni == "" {
		sni = strings.TrimSpace(req.Host)
	}
	if sni == "" {
		sni = "auto"
	}

	resolvedOutbound := s.resolveHTTPOutboundURL(req.Outbound)
	node, err := s.singboxClient.AddNode(r.Context(), proto, req.Port, cred, sni, resolvedOutbound)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "msg": "节点创建成功", "node": node})
}

type SetOutboundRequest struct {
	Target   string `json:"target"`
	Outbound string `json:"outbound"`
}

func (s *Server) handleSingBoxSetOutbound(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	var req SetOutboundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "无效的请求参数"})
		return
	}

	resolvedOutbound := s.resolveHTTPOutboundURL(req.Outbound)
	resp, err := s.singboxClient.SetOutbound(r.Context(), req.Target, resolvedOutbound)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// syncSingBoxOutboundCredentials automatically updates all sing-box chained inbounds
// whenever local proxy port authentication rules or web credentials change.
func (s *Server) syncSingBoxOutboundCredentials(ctx context.Context) {
	if s.singboxClient == nil || !s.singboxClient.IsInstalled() {
		return
	}

	nodes, err := s.singboxClient.ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		return
	}

	for _, n := range nodes {
		if n.Outbound == "direct" || n.OutboundPort <= 0 {
			continue
		}

		expectedOutbound := s.resolveHTTPOutboundURL(fmt.Sprintf("%d", n.OutboundPort))
		if expectedOutbound == "direct" {
			continue
		}

		// If protocol or credentials changed, automatically update the node's outbound config
		if n.Outbound != expectedOutbound {
			stats.LogInfo("SingBoxSync", "正在自动同步更新节点 [%s] 链式出站凭据: %s -> %s", n.Name, n.Outbound, expectedOutbound)
			_, _ = s.singboxClient.SetOutbound(ctx, n.Name, expectedOutbound)
		}
	}
}

func (s *Server) handleSingBoxDeleteNode(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		var req struct {
			Target string `json:"target"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			target = req.Target
		}
	}

	if target == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "缺少 target 参数"})
		return
	}

	err := s.singboxClient.DeleteNode(r.Context(), target)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "msg": "节点删除成功", "target": target})
}

func (s *Server) buildClashSubURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = net.JoinHostPort(s.cfg.UIHost, fmt.Sprintf("%d", s.cfg.UIPort))
	}
	secret := strings.Trim(s.cfg.UIPath, "/")
	if secret != "" {
		return fmt.Sprintf("%s://%s/%s/api/singbox/subscription/clash", scheme, host, secret)
	}
	return fmt.Sprintf("%s://%s/api/singbox/subscription/clash", scheme, host)
}

func (s *Server) handleSingBoxClashSub(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	nodes, err := s.singboxClient.ListNodes(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("# 获取 sing-box 节点列表失败: %v\n", err)))
		return
	}

	serverHost := extractHostFromRequest(r.Host)
	if serverHost == "" || serverHost == "127.0.0.1" || serverHost == "localhost" {
		serverHost = s.cfg.UIHost
	}

	yamlContent := GenerateClashYAML(nodes, serverHost)

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"singbox-clash.yaml\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(yamlContent))
}

func (s *Server) handleSingBoxGetSub(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "clash" || strings.Contains(r.Header.Get("User-Agent"), "Clash") || strings.Contains(r.Header.Get("User-Agent"), "clash") || strings.Contains(r.Header.Get("User-Agent"), "Mihomo") {
		s.handleSingBoxClashSub(w, r)
		return
	}

	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	sub, err := s.singboxClient.GetSubscription(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	if sub == nil {
		sub = &singbox.SubResponse{
			OK:      true,
			Enabled: false,
		}
	}
	sub.ClashSubURL = s.buildClashSubURL(r)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sub)
}

func (s *Server) handleSingBoxSyncSub(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	sub, err := s.singboxClient.SyncSubscription(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sub)
}

func (s *Server) handleSingBoxInitSub(w http.ResponseWriter, r *http.Request) {
	if s.singboxClient == nil {
		s.singboxClient = singbox.NewClient()
	}

	var req struct {
		Port int `json:"port"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Port == 0 {
		if pStr := r.URL.Query().Get("port"); pStr != "" {
			req.Port, _ = strconv.Atoi(pStr)
		}
	}

	sub, err := s.singboxClient.InitSubscription(r.Context(), req.Port)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sub)
}
