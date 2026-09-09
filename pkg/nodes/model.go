package nodes

import (
	"fmt"
	"time"
)

type Node struct {
	ID             string    `json:"id"`
	HostName       string    `json:"hostname"`
	IP             string    `json:"ip"`
	Score          int64     `json:"score"`
	Ping           int       `json:"ping"`
	Speed          int64     `json:"speed"` // in bps
	CountryLong    string    `json:"country_long"`
	CountryShort   string    `json:"country_short"`
	NumVpnSessions int       `json:"num_vpn_sessions"`
	Uptime         int64     `json:"uptime"`
	TotalUsers     int64     `json:"total_users"`
	TotalTraffic   int64     `json:"total_traffic"`
	LogType        string    `json:"log_type"`
	Operator       string    `json:"operator"`
	Message        string    `json:"message"`
	ConfigData     string    `json:"-"` // Raw ovpn config string, omit from general API json
	Proto          string    `json:"proto"`
	Port           int       `json:"port"`
	LatencyMs      int       `json:"latency_ms"` // Real-time measured TCP ping in ms (-1 if unreachable)
	LastChecked    time.Time `json:"last_checked"`

	// IP 归属地与网络类型探测
	IPType     string `json:"ip_type"` // residential (住宅) / hosting (机房) / mobile (移动) / unknown
	ISP        string `json:"isp"`
	City       string `json:"city"`
	Region     string `json:"region"`
	IsHosting  bool   `json:"is_hosting"`
	IsFavorite bool   `json:"is_favorite"`
}

func (n *Node) String() string {
	return fmt.Sprintf("[%s] %s (%s:%d %s, Score: %d, Speed: %.2f Mbps, Ping: %dms)",
		n.CountryShort, n.HostName, n.IP, n.Port, n.Proto, n.Score, float64(n.Speed)/1000000.0, n.Ping)
}
