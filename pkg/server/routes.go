package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aimili-vpngate-go/pkg/config"
	"aimili-vpngate-go/pkg/nodes"
	"aimili-vpngate-go/pkg/stats"
)

type StatusResponse struct {
	VPN            any                   `json:"vpn"`
	Traffic        stats.TrafficSnapshot `json:"traffic"`
	ProxyAddr      string                `json:"proxy_addr"`
	NodeCount      int                   `json:"node_count"`
	NodeSource     string                `json:"node_source"`
	BlacklistCount int                   `json:"blacklist_count"`
	AdminPath      string                `json:"admin_path"`
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

	return StatusResponse{
		VPN:            vpnState,
		Traffic:        traffic,
		ProxyAddr:      fmt.Sprintf("%s:%d", proxyHost, s.cfg.ProxyPort),
		NodeCount:      count,
		NodeSource:     source,
		BlacklistCount: blCount,
		AdminPath:      s.cfg.UIPath,
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
