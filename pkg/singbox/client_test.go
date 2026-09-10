package singbox

import (
	"encoding/json"
	"testing"
)

func TestParseProtocolsJSON(t *testing.T) {
	raw := `{
		"ok": true,
		"protocols": [
			{
				"id": "reality",
				"name": "VLESS-REALITY",
				"recommended": true,
				"transport": "tcp",
				"tls": "reality",
				"description": "抗封锁协议",
				"args": ["port", "uuid", "sni"]
			}
		]
	}`

	var resp ProtocolsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal protocols: %v", err)
	}
	if len(resp.Protocols) != 1 || resp.Protocols[0].ID != "reality" {
		t.Fatalf("unexpected protocols: %+v", resp)
	}
}

func TestParseNodesJSON(t *testing.T) {
	raw := `{
		"ok": true,
		"count": 1,
		"nodes": [
			{
				"name": "VLESS-REALITY-8443.json",
				"protocol": "VLESS-REALITY",
				"port": 8443,
				"outbound": "socks5://127.0.0.1:7928",
				"url": "vless://test@1.1.1.1:8443"
			}
		]
	}`

	var resp NodesResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal nodes: %v", err)
	}
	if resp.Count != 1 || resp.Nodes[0].Port != 8443 || resp.Nodes[0].Outbound != "socks5://127.0.0.1:7928" {
		t.Fatalf("unexpected nodes: %+v", resp)
	}
}

func TestParseSubJSON(t *testing.T) {
	raw := `{
		"ok": true,
		"enabled": true,
		"sub_url": "http://1.1.1.1:12345/tok/sub.yaml",
		"port": 12345,
		"node_count": 2,
		"nodes": ["vless://node1", "hysteria2://node2"]
	}`

	var resp SubResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("failed to unmarshal sub: %v", err)
	}
	if !resp.Enabled || resp.NodeCount != 2 || len(resp.Nodes) != 2 {
		t.Fatalf("unexpected sub: %+v", resp)
	}
}
