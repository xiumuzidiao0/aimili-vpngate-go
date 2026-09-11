package nodes

import (
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
)

const testCSVHeader = "HostName,IP,Score,Ping,Speed,CountryLong,CountryShort,NumVpnSessions,OpenVPN_ConfigData_Base64"

func TestParseVPNGateCSVRejectsShortRows(t *testing.T) {
	data := []byte(testCSVHeader + "\nonly-host\n")
	if _, err := ParseVPNGateCSV(data, 0); err == nil {
		t.Fatal("expected malformed short row to be rejected")
	}
}

func TestParseVPNGateCSVIPv6NodeID(t *testing.T) {
	configText := "client\ndev tun\nproto udp\nremote 2001:db8::1 1194\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(configText))
	row := fmt.Sprintf("example.test,2001:db8::1,100,20,1000000,Test,JP,1,%s", encoded)

	nodes, err := ParseVPNGateCSV([]byte(testCSVHeader+"\n"+row+"\n"), 0)
	if err != nil {
		t.Fatalf("ParseVPNGateCSV failed: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected one node, got %d", len(nodes))
	}

	expectedID := net.JoinHostPort("2001:db8::1", "1194")
	if nodes[0].ID != expectedID {
		t.Fatalf("expected IPv6 node ID %q, got %q", expectedID, nodes[0].ID)
	}
	if strings.Contains(nodes[0].ID, "2001:db8::1:1194") {
		t.Fatalf("IPv6 node ID is ambiguous: %q", nodes[0].ID)
	}
}
