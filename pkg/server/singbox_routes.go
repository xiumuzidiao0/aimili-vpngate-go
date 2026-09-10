package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"aimili-vpngate-go/pkg/singbox"
)

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

	node, err := s.singboxClient.AddNode(r.Context(), proto, req.Port, cred, sni, req.Outbound)
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

	resp, err := s.singboxClient.SetOutbound(r.Context(), req.Target, req.Outbound)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
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

func (s *Server) handleSingBoxGetSub(w http.ResponseWriter, r *http.Request) {
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
