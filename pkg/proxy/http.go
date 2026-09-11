package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"aimili-vpngate-go/pkg/tunnel"
)

const (
	proxyAuthRequiredResponse = "HTTP/1.1 407 Proxy Authentication Required\r\n" +
		"Proxy-Authenticate: Basic realm=\"AimiliVPN\"\r\n" +
		"Content-Length: 32\r\n" +
		"Connection: close\r\n\r\n" +
		"Proxy Authentication Required.\n"

	connectionEstablishedResponse = "HTTP/1.1 200 Connection Established\r\n\r\n"
)

func handleHTTP(client net.Conn, br *bufio.Reader, auth *Authenticator, devName string, tun *tunnel.Tunnel) error {
	req, err := http.ReadRequest(br)
	if err != nil {
		return fmt.Errorf("failed to read http request: %w", err)
	}

	if auth.IsEnabled() && !auth.VerifyHTTP(req) {
		_, _ = client.Write([]byte(proxyAuthRequiredResponse))
		return fmt.Errorf("http proxy authentication failed")
	}

	if req.Method == http.MethodConnect {
		return handleConnect(client, req, devName, tun)
	}

	return handlePlainHTTP(client, req, devName, tun)
}

func handleConnect(client net.Conn, req *http.Request, devName string, tun *tunnel.Tunnel) error {
	targetAddr := req.RequestURI
	if !strings.Contains(targetAddr, ":") {
		targetAddr = net.JoinHostPort(targetAddr, "443")
	}

	upstream, err := dialUpstream(targetAddr, devName, 10*time.Second)
	if err != nil {
		if tun != nil {
			tun.RecordFailure()
		}
		resp := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\nFailed to connect to %s\n", len(targetAddr)+23, targetAddr)
		_, _ = client.Write([]byte(resp))
		return fmt.Errorf("dial %s failed: %w", targetAddr, err)
	}
	if tun != nil {
		tun.RecordSuccess()
	}

	if _, err := client.Write([]byte(connectionEstablishedResponse)); err != nil {
		_ = upstream.Close()
		return err
	}

	relay(client, upstream)
	return nil
}

func handlePlainHTTP(client net.Conn, req *http.Request, devName string, tun *tunnel.Tunnel) error {
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "80")
	}

	upstream, err := dialUpstream(host, devName, 10*time.Second)
	if err != nil {
		if tun != nil {
			tun.RecordFailure()
		}
		resp := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\nFailed to connect to %s\n", len(host)+23, host)
		_, _ = client.Write([]byte(resp))
		return fmt.Errorf("dial %s failed: %w", host, err)
	}
	defer upstream.Close()

	if tun != nil {
		tun.RecordSuccess()
	}

	// Clean hop-by-hop, proxy, and privacy-leaking headers
	cleanPrivacyHeaders(req)

	// Forward original request to upstream
	if err := req.Write(upstream); err != nil {
		return fmt.Errorf("forward request failed: %w", err)
	}

	// Pipe upstream response to client
	relay(client, upstream)
	return nil
}

// cleanPrivacyHeaders strips headers that reveal proxy usage, client real IP, or infrastructure details.
func cleanPrivacyHeaders(req *http.Request) {
	privacyHeaders := []string{
		"Proxy-Authorization",
		"Proxy-Connection",
		"Proxy-Authenticate",
		"Via",
		"X-Forwarded-For",
		"X-Forwarded-Proto",
		"X-Forwarded-Host",
		"X-Forwarded-Server",
		"X-Real-IP",
		"Forwarded",
		"CF-Connecting-IP",
		"True-Client-IP",
		"X-Client-IP",
		"X-Cluster-Client-IP",
		"Fastly-Client-IP",
		"X-Originating-IP",
	}

	for _, h := range privacyHeaders {
		req.Header.Del(h)
	}
}
