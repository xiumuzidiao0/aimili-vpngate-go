package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	binaryPath string
}

type ProtocolInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Recommended bool     `json:"recommended"`
	Transport   string   `json:"transport"`
	TLS         string   `json:"tls"`
	Description string   `json:"description"`
	Args        []string `json:"args"`
}

type ProtocolsResponse struct {
	OK        bool           `json:"ok"`
	Protocols []ProtocolInfo `json:"protocols"`
}

type Node struct {
	Name           string `json:"name"`
	Tag            string `json:"tag"`
	Protocol       string `json:"protocol"`
	RawProtocol    string `json:"raw_protocol"`
	Network        string `json:"network"`
	Port           int    `json:"port"`
	Address        string `json:"address"`
	UUID           string `json:"uuid"`
	Password       string `json:"password"`
	Username       string `json:"username"`
	SSMethod       string `json:"ss_method"`
	SNI            string `json:"sni"`
	Host           string `json:"host"`
	Path           string `json:"path"`
	PBK            string `json:"pbk"`
	Flow           string `json:"flow"`
	Outbound       string `json:"outbound"`
	OutboundType   string `json:"outbound_type"`
	OutboundServer string `json:"outbound_server"`
	OutboundPort   int    `json:"outbound_port"`
	OutboundUser   string `json:"outbound_user"`
	URL            string `json:"url"`
}

type NodesResponse struct {
	OK    bool   `json:"ok"`
	Count int    `json:"count"`
	Nodes []Node `json:"nodes"`
	Error string `json:"error,omitempty"`
}

type SingleNodeResponse struct {
	OK     bool   `json:"ok"`
	Msg    string `json:"msg,omitempty"`
	Target string `json:"target,omitempty"`
	Node   *Node  `json:"node,omitempty"`
	Error  string `json:"error,omitempty"`
}

type OutboundResponse struct {
	OK           bool     `json:"ok"`
	Msg          string   `json:"msg,omitempty"`
	Outbound     string   `json:"outbound,omitempty"`
	Target       string   `json:"target,omitempty"`
	UpdatedCount int      `json:"updated_count,omitempty"`
	Targets      []string `json:"targets,omitempty"`
	Node         *Node    `json:"node,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type SubResponse struct {
	OK           bool     `json:"ok"`
	Enabled      bool     `json:"enabled"`
	SubURL       string   `json:"sub_url,omitempty"`
	Port         int      `json:"port,omitempty"`
	Token        string   `json:"token,omitempty"`
	Filename     string   `json:"filename,omitempty"`
	NodeCount    int      `json:"node_count"`
	Nodes        []string `json:"nodes"`
	CaddyRunning bool     `json:"caddy_running"`
	Msg          string   `json:"msg,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type StatusResponse struct {
	OK   bool `json:"ok"`
	Core struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		ScriptVersion string `json:"script_version"`
		Running       bool   `json:"running"`
		PID           int    `json:"pid"`
	} `json:"core"`
	Caddy struct {
		Running bool   `json:"running"`
		PID     int    `json:"pid"`
		Version string `json:"version"`
	} `json:"caddy"`
	NodeCount           int    `json:"node_count"`
	SubscriptionEnabled bool   `json:"subscription_enabled"`
	Installed           bool   `json:"installed"`
	BinaryPath          string `json:"binary_path"`
	Error               string `json:"error,omitempty"`
}

func NewClient() *Client {
	candidates := []string{
		"/usr/local/bin/sing-box",
		"/usr/bin/sing-box",
		"/etc/sing-box/sh/sing-box.sh",
		"/home/xmzd/sing-box/sing-box.sh",
	}

	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return &Client{binaryPath: p}
		}
	}

	return &Client{binaryPath: "/usr/local/bin/sing-box"}
}

func (c *Client) IsInstalled() bool {
	if _, err := os.Stat(c.binaryPath); err == nil {
		return true
	}
	_, err := exec.LookPath("sing-box")
	return err == nil
}

func (c *Client) execAPI(ctx context.Context, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"api"}, args...)
	cmd := exec.CommandContext(ctx, c.binaryPath, cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = strings.TrimSpace(stdout.String())
		}
		if errText == "" {
			errText = err.Error()
		}
		return nil, fmt.Errorf("%s (exit code %v)", errText, cmd.ProcessState.ExitCode())
	}

	return stdout.Bytes(), nil
}

func (c *Client) GetStatus(ctx context.Context) (*StatusResponse, error) {
	var resp StatusResponse
	if !c.IsInstalled() {
		resp.OK = true
		resp.Installed = false
		resp.BinaryPath = c.binaryPath
		resp.Core.Name = "sing-box"
		resp.Core.Version = "not installed"
		return &resp, nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "status")
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse status json: %w", err)
	}

	resp.Installed = true
	resp.BinaryPath = c.binaryPath
	return &resp, nil
}

func (c *Client) GetProtocols(ctx context.Context) ([]ProtocolInfo, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "protocols")
	if err != nil {
		return nil, err
	}

	var resp ProtocolsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse protocols json: %w", err)
	}

	return resp.Protocols, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]Node, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "list")
	if err != nil {
		return nil, err
	}

	var resp NodesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse nodes list json: %w", err)
	}

	return resp.Nodes, nil
}

func (c *Client) GetNode(ctx context.Context, name string) (*Node, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "info", name)
	if err != nil {
		return nil, err
	}

	var resp SingleNodeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse node info json: %w", err)
	}

	return resp.Node, nil
}

func (c *Client) AddNode(ctx context.Context, protocol, port, uuidOrPass, sniOrHost, outbound string) (*Node, error) {
	if port == "" {
		port = "auto"
	}
	if uuidOrPass == "" {
		uuidOrPass = "auto"
	}
	if sniOrHost == "" {
		sniOrHost = "auto"
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	args := []string{"add", protocol, port, uuidOrPass, sniOrHost}
	if outbound != "" {
		args = append(args, outbound)
	}

	data, err := c.execAPI(ctxTimeout, args...)
	if err != nil {
		return nil, err
	}

	var resp SingleNodeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse add node response: %w", err)
	}

	if !resp.OK {
		return nil, fmt.Errorf("add node failed: %s", resp.Error)
	}

	return resp.Node, nil
}

func (c *Client) SetOutbound(ctx context.Context, nameOrAll, outbound string) (*OutboundResponse, error) {
	if nameOrAll == "" {
		nameOrAll = "all"
	}
	if outbound == "" {
		outbound = "direct"
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "outbound", nameOrAll, outbound)
	if err != nil {
		return nil, err
	}

	var resp OutboundResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse outbound response: %w", err)
	}

	if !resp.OK {
		return nil, fmt.Errorf("set outbound failed: %s", resp.Error)
	}

	return &resp, nil
}

func (c *Client) DeleteNode(ctx context.Context, nameOrAll string) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "del", nameOrAll)
	if err != nil {
		return err
	}

	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("failed to parse del response: %w", err)
	}

	if !resp.OK {
		return fmt.Errorf("delete node failed: %s", resp.Error)
	}

	return nil
}

func (c *Client) GetSubscription(ctx context.Context) (*SubResponse, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "sub", "get")
	if err != nil {
		return nil, err
	}

	var resp SubResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse subscription json: %w", err)
	}

	return &resp, nil
}

func (c *Client) SyncSubscription(ctx context.Context) (*SubResponse, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	data, err := c.execAPI(ctxTimeout, "sub", "sync")
	if err != nil {
		return nil, err
	}

	var resp SubResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse subscription sync response: %w", err)
	}

	return &resp, nil
}

func (c *Client) InitSubscription(ctx context.Context, port int) (*SubResponse, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	args := []string{"sub", "init"}
	if port > 0 {
		args = append(args, fmt.Sprintf("%d", port))
	}

	data, err := c.execAPI(ctxTimeout, args...)
	if err != nil {
		return nil, err
	}

	var resp SubResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse subscription init response: %w", err)
	}

	return &resp, nil
}
