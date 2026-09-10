package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/proxy"
	"aimili-vpngate-go/pkg/stats"
	"aimili-vpngate-go/pkg/tunnel"
)

type StatusResponse struct {
	VPN            any                    `json:"vpn"`
	Traffic        stats.TrafficSnapshot  `json:"traffic"`
	ProxyAddr      string                 `json:"proxy_addr"`
	NodeCount      int                    `json:"node_count"`
	NodeSource     string                 `json:"node_source"`
	BlacklistCount int                    `json:"blacklist_count"`
	AdminPath      string                 `json:"admin_path"`
	Version        string                 `json:"version"`
	Tunnels        []*tunnel.Tunnel       `json:"tunnels"`
	PortRules      []proxy.PortRule       `json:"port_rules"`
	DynamicGroups  []*tunnel.DynamicGroup `json:"dynamic_groups"`
}

func (s *Server) buildStatusResponse() StatusResponse {
	vpnState := s.vpn.Snapshot()
	traffic := stats.GetTrafficTracker().Snapshot()
	_, source, _, count := s.pool.Status()
	blCount := s.pool.Blacklist().Count()

	proxyHost := s.cfg.ProxyHost
	if strings.Contains(proxyHost, ":") {
		proxyHost = "[" + proxyHost + "]"
	}

	var tunnels []*tunnel.Tunnel
	if s.tunnelPool != nil {
		tunnels = s.tunnelPool.ListTunnels()
	}
	var portRules []proxy.PortRule
	if s.portMgr != nil {
		portRules = s.portMgr.GetRules()
	}
	var dynamicGroups []*tunnel.DynamicGroup
	if s.dynamicMgr != nil {
		dynamicGroups = s.dynamicMgr.ListGroups()
	}

	return StatusResponse{
		VPN:            vpnState,
		Traffic:        traffic,
		ProxyAddr:      fmt.Sprintf("%s:%d", proxyHost, s.cfg.ProxyPort),
		NodeCount:      count,
		NodeSource:     source,
		BlacklistCount: blCount,
		AdminPath:      s.cfg.UIPath,
		Version:        config.Version,
		Tunnels:        tunnels,
		PortRules:      portRules,
		DynamicGroups:  dynamicGroups,
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := s.buildStatusResponse()
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	candidates := s.pool.GetCandidates()
	s.writeJSON(w, http.StatusOK, candidates)
}

type ProbeNodesRequest struct {
	NodeIDs []string `json:"node_ids"`
}

func (s *Server) handleProbeNodes(w http.ResponseWriter, r *http.Request) {
	var req ProbeNodesRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	go func() {
		s.pool.ProbeSpecificNodes(context.Background(), req.NodeIDs)
	}()

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "已在后台启动节点可用性探测与测速",
	})
}

type FavoriteRequest struct {
	NodeID string `json:"node_id"`
}

func (s *Server) handleToggleFavorite(w http.ResponseWriter, r *http.Request) {
	var req FavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		s.writeError(w, http.StatusBadRequest, "缺少 node_id 参数")
		return
	}

	isFav := s.pool.Favorites().Toggle(req.NodeID)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"node_id":     req.NodeID,
		"is_favorite": isFav,
	})
}

type ConnectRequest struct {
	NodeID string `json:"node_id"`
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req ConnectRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	var target *nodes.Node
	if req.NodeID != "" {
		target = s.pool.GetNodeByID(req.NodeID)
		if target == nil {
			s.writeError(w, http.StatusBadRequest, "指定节点不存在或已被过滤")
			return
		}
	} else {
		target = s.pool.SelectBest()
		if target == nil {
			s.writeError(w, http.StatusBadRequest, "当前无可用候选节点，请先刷新节点源")
			return
		}
	}

	go func() {
		_ = s.vpn.Connect(target)
	}()

	s.writeJSON(w, http.StatusOK, map[string]any{
		"message": "已发起连接请求",
		"node_id": target.ID,
		"ip":      target.IP,
		"country": target.CountryShort,
	})
}

func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	s.vpn.Disconnect("用户控制台主动断开")
	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "已成功断开 VPN",
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	go func() {
		_ = s.pool.Refresh(context.Background())
	}()

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "已在后台启动节点列表刷新与测速",
	})
}

func (s *Server) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	list := s.pool.Blacklist().List()
	s.writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	recent := stats.GetRingLog().Recent(100)
	s.writeJSON(w, http.StatusOK, recent)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings := s.cfg.GetSettings()
	s.writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req config.SettingsDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "请求参数解析失败")
		return
	}

	if req.UIPort > 0 && (req.UIPort < 1 || req.UIPort > 65535) {
		s.writeError(w, http.StatusBadRequest, "Web 端口必须在 1 至 65535 之间")
		return
	}
	if req.ProxyPort > 0 && (req.ProxyPort < 1 || req.ProxyPort > 65535) {
		s.writeError(w, http.StatusBadRequest, "代理端口必须在 1 至 65535 之间")
		return
	}
	if req.UIPort > 0 && req.ProxyPort > 0 && req.UIPort == req.ProxyPort {
		s.writeError(w, http.StatusBadRequest, "Web 端口不能与代理端口相同")
		return
	}

	if err := s.cfg.UpdateSettings(req); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("更新配置失败: %v", err))
		return
	}

	current := s.cfg.GetSettings()
	stats.LogInfo("Server", "管理员通过 Web 控制台更新了系统配置: 账号=%s, 路径=/%s, Web端口=%d, 代理端口=%d",
		current.UIUsername, current.UIPath, current.UIPort, current.ProxyPort)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"message":  "配置修改成功并已持久化保存！",
		"settings": current,
	})
}

func (s *Server) handleListTunnels(w http.ResponseWriter, r *http.Request) {
	if s.tunnelPool == nil {
		s.writeJSON(w, http.StatusOK, []*tunnel.Tunnel{})
		return
	}
	s.writeJSON(w, http.StatusOK, s.tunnelPool.ListTunnels())
}

type StartTunnelRequest struct {
	NodeID string `json:"node_id"`
}

func (s *Server) handleStartTunnel(w http.ResponseWriter, r *http.Request) {
	var req StartTunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		s.writeError(w, http.StatusBadRequest, "缺少 node_id 参数")
		return
	}

	node := s.pool.GetNodeByID(req.NodeID)
	if node == nil {
		s.writeError(w, http.StatusBadRequest, "节点不存在或已被过滤")
		return
	}

	if s.tunnelPool == nil {
		s.writeError(w, http.StatusInternalServerError, "隧道池未初始化")
		return
	}

	tun, err := s.tunnelPool.StartTunnel(node)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"message": "隧道正在发起连接...",
		"tunnel":  tun.Snapshot(),
	})
}

type StopTunnelRequest struct {
	TunnelID string `json:"tunnel_id"`
}

func (s *Server) handleStopTunnel(w http.ResponseWriter, r *http.Request) {
	var req StopTunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TunnelID == "" {
		s.writeError(w, http.StatusBadRequest, "缺少 tunnel_id 参数")
		return
	}

	if s.tunnelPool == nil {
		s.writeError(w, http.StatusInternalServerError, "隧道池未初始化")
		return
	}

	if err := s.tunnelPool.StopTunnel(req.TunnelID); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "指定隧道已成功断开并释放",
	})
}

func (s *Server) handleGetPortRules(w http.ResponseWriter, r *http.Request) {
	if s.portMgr == nil {
		s.writeJSON(w, http.StatusOK, []proxy.PortRule{})
		return
	}
	s.writeJSON(w, http.StatusOK, s.portMgr.GetRules())
}

type SetPortRulesRequest struct {
	Rules []proxy.PortRule `json:"rules"`
}

func (s *Server) handleSetPortRules(w http.ResponseWriter, r *http.Request) {
	var req SetPortRulesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "请求格式解析失败")
		return
	}

	if s.portMgr == nil {
		s.writeError(w, http.StatusInternalServerError, "代理端口管理器未初始化")
		return
	}

	if err := s.portMgr.ApplyRules(req.Rules); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"message": "端口分流规则已更新生效！",
		"rules":   s.portMgr.GetRules(),
	})
}

func (s *Server) handleListTunnelGroups(w http.ResponseWriter, r *http.Request) {
	if s.dynamicMgr == nil {
		s.writeJSON(w, http.StatusOK, []*tunnel.DynamicGroup{})
		return
	}
	s.writeJSON(w, http.StatusOK, s.dynamicMgr.ListGroups())
}

func (s *Server) handleSaveTunnelGroup(w http.ResponseWriter, r *http.Request) {
	var g tunnel.DynamicGroup
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		s.writeError(w, http.StatusBadRequest, "请求格式解析失败")
		return
	}

	if s.dynamicMgr == nil {
		s.writeError(w, http.StatusInternalServerError, "动态隧道组管理器未初始化")
		return
	}

	if err := s.dynamicMgr.SaveGroup(&g); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if g.Enabled {
		go s.dynamicMgr.EvaluateGroup(context.Background(), &g)
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"message": "动态自适应隧道组已保存并启动评估！",
		"group":   s.dynamicMgr.GetGroup(g.ID),
	})
}

type DeleteGroupRequest struct {
	ID string `json:"id"`
}

func (s *Server) handleDeleteTunnelGroup(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		var req DeleteGroupRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		id = req.ID
	}

	if id == "" {
		s.writeError(w, http.StatusBadRequest, "缺少 id 参数")
		return
	}

	if s.dynamicMgr == nil {
		s.writeError(w, http.StatusInternalServerError, "动态隧道组管理器未初始化")
		return
	}

	s.dynamicMgr.DeleteGroup(id)
	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "动态自适应组已删除并释放关联出口",
	})
}

func (s *Server) handleEvaluateTunnelGroups(w http.ResponseWriter, r *http.Request) {
	if s.dynamicMgr == nil {
		s.writeError(w, http.StatusInternalServerError, "动态隧道组管理器未初始化")
		return
	}

	go func() {
		for _, g := range s.dynamicMgr.ListGroups() {
			if g.Enabled {
				s.dynamicMgr.EvaluateGroup(context.Background(), g)
			}
		}
	}()

	s.writeJSON(w, http.StatusOK, map[string]string{
		"message": "已在后台启动全量动态自适应组评估与轮换",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	tr := stats.GetTrafficTracker().Snapshot()
	state := s.vpn.Snapshot()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "# HELP aimili_traffic_upload_bytes_total Total bytes uploaded through proxy\n")
	_, _ = fmt.Fprintf(w, "# TYPE aimili_traffic_upload_bytes_total counter\n")
	_, _ = fmt.Fprintf(w, "aimili_traffic_upload_bytes_total %d\n", tr.TotalUploadBytes)

	_, _ = fmt.Fprintf(w, "# HELP aimili_traffic_download_bytes_total Total bytes downloaded through proxy\n")
	_, _ = fmt.Fprintf(w, "# TYPE aimili_traffic_download_bytes_total counter\n")
	_, _ = fmt.Fprintf(w, "aimili_traffic_download_bytes_total %d\n", tr.TotalDownloadBytes)

	_, _ = fmt.Fprintf(w, "# HELP aimili_active_connections Current active proxy client connections\n")
	_, _ = fmt.Fprintf(w, "# TYPE aimili_active_connections gauge\n")
	_, _ = fmt.Fprintf(w, "aimili_active_connections %d\n", tr.ActiveConnections)

	connectedVal := 0
	if state.TunnelReady {
		connectedVal = 1
	}
	_, _ = fmt.Fprintf(w, "# HELP aimili_vpn_connected Status of VPN tunnel (1 = connected, 0 = disconnected)\n")
	_, _ = fmt.Fprintf(w, "# TYPE aimili_vpn_connected gauge\n")
	_, _ = fmt.Fprintf(w, "aimili_vpn_connected %d\n", connectedVal)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	s.writeJSON(w, code, map[string]string{"error": msg})
}
