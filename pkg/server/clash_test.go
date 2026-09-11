package server

import (
	"strings"
	"testing"

	"aimili-vpngate-go/pkg/singbox"
)

func TestGenerateClashYAML(t *testing.T) {
	nodes := []singbox.Node{
		{
			Name:        "Japan-Reality-01",
			Tag:         "jp-reality",
			Protocol:    "VLESS-Reality",
			RawProtocol: "reality",
			Network:     "tcp",
			Port:        443,
			Address:     "1.2.3.4",
			UUID:        "b831381d-6324-4d53-ad4f-8cda48b30811",
			Flow:        "xtls-rprx-vision",
			SNI:         "www.apple.com",
			PBK:         "1111222233334444555566667777888899990000111=",
		},
		{
			Name:        "US-Hy2-02",
			Tag:         "us-hy2",
			Protocol:    "Hysteria2",
			RawProtocol: "hy2",
			Port:        8443,
			Address:     "5.6.7.8",
			Password:    "secretpassword",
			SNI:         "gateway.example.com",
		},
		{
			Name:        "SG-TUIC-03",
			Tag:         "sg-tuic",
			Protocol:    "TUIC",
			RawProtocol: "tuic",
			Port:        9443,
			Address:     "9.10.11.12",
			UUID:        "d931381d-6324-4d53-ad4f-8cda48b30822",
			Password:    "tuicpass",
			SNI:         "sg.example.com",
		},
		{
			Name:        "KR-SS-04",
			Tag:         "kr-ss",
			Protocol:    "Shadowsocks",
			RawProtocol: "ss",
			Port:        8388,
			Address:     "13.14.15.16",
			SSMethod:    "2022-blake3-aes-128-gcm",
			Password:    "sspassword",
		},
	}

	yaml := GenerateClashYAML(nodes, "47.238.2.197")

	// Verify required sections
	if !strings.Contains(yaml, "port: 7890") {
		t.Fatalf("expected global port: 7890")
	}
	if !strings.Contains(yaml, "proxies:") {
		t.Fatalf("expected proxies section")
	}
	if !strings.Contains(yaml, "proxy-groups:") {
		t.Fatalf("expected proxy-groups section")
	}
	if !strings.Contains(yaml, "rules:") {
		t.Fatalf("expected rules section")
	}

	// Verify VLESS Reality node
	if !strings.Contains(yaml, "type: vless") {
		t.Fatalf("expected vless proxy")
	}
	if !strings.Contains(yaml, "reality-opts:") || !strings.Contains(yaml, "public-key: 1111222233334444555566667777888899990000111=") {
		t.Fatalf("expected reality-opts and public-key")
	}

	// Verify Hysteria2 node
	if !strings.Contains(yaml, "type: hysteria2") || !strings.Contains(yaml, "alpn:\n      - h3") {
		t.Fatalf("expected hysteria2 proxy with h3 alpn")
	}

	// Verify TUIC node
	if !strings.Contains(yaml, "type: tuic") || !strings.Contains(yaml, "congestion-controller: bbr") {
		t.Fatalf("expected tuic proxy with bbr")
	}

	// Verify Shadowsocks node
	if !strings.Contains(yaml, "type: ss") || !strings.Contains(yaml, "cipher: 2022-blake3-aes-128-gcm") {
		t.Fatalf("expected shadowsocks proxy with 2022 cipher")
	}

	// Verify proxy groups contain node names
	if !strings.Contains(yaml, "name: \"🚀 节点选择\"") {
		t.Fatalf("expected 🚀 节点选择 group")
	}
	if !strings.Contains(yaml, "\"Japan-Reality-01\"") {
		t.Fatalf("expected Japan-Reality-01 in proxy groups")
	}
}
