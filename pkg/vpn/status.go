package vpn

import (
	"time"

	"aimili-vpngate-go/pkg/nodes"
)

type ConnectionStatus string

const (
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusReconnecting ConnectionStatus = "reconnecting"
	StatusFailed       ConnectionStatus = "failed"
)

type StateSnapshot struct {
	Status         ConnectionStatus `json:"status"`
	StatusText     string           `json:"status_text"`
	ActiveNodeID   string           `json:"active_node_id"`
	ActiveNode     *nodes.Node      `json:"active_node,omitempty"`
	TunnelReady    bool             `json:"tunnel_ready"`
	ProxyReady     bool             `json:"proxy_ready"`
	ConnectedAt    time.Time        `json:"connected_at,omitempty"`
	UptimeSeconds  int64            `json:"uptime_seconds"`
	LastMessage    string           `json:"last_message"`
	ReconnectCount int              `json:"reconnect_count"`
}
