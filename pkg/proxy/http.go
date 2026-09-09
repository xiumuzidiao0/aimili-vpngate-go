package proxy

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	proxyAuthRequiredResponse = "HTTP/1.1 407 Proxy Authentication Required\r\n" +
		"Proxy-Authenticate: Basic realm=\"AimiliVPN\"\r\n" +
		"Content-Length: 32\r\n" +
		"Connection: close\r\n\r\n" +
		"Proxy Authentication Required.\n"

	connectionEstablishedResponse = "HTTP/1.1 200 Connection Established\r\n\r\n"
)

func handleHTTP(client net.Conn, br *bufio.Reader, auth *Authenticator, devName string) error {
	req, err := http.ReadRequest(br)
	if err != nil {
		return fmt.Errorf("failed to read http request: %w", err)
	}

	if auth.IsEnabled() && !auth.VerifyHTTP(req) {
		_, _ = client.Write([]byte(proxyAuthRequiredResponse))
		return fmt.Errorf("http proxy authentication failed")
	}

	if req.Method == http.MethodConnect {
		return handleConnect(client, req, devName)
	}

	return handlePlainHTTP(client, req, devName)
}

func handleConnect(client net.Conn, req *http.Request, devName string) error {
	targetAddr := req.RequestURI
	if !strings.Contains(targetAddr, ":") {
		targetAddr = net.JoinHostPort(targetAddr, "443")
	}

	upstream, err := dialUpstream(targetAddr, devName, 10*time.Second)
	if err != nil {
		resp := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\nFailed to connect to %s\n", len(targetAddr)+23, targetAddr)
		_, _ = client.Write([]byte(resp))
		return fmt.Errorf("dial %s failed: %w", targetAddr, err)
	}

	if _, err := client.Write([]byte(connectionEstablishedResponse)); err != nil {
		_ = upstream.Close()
		return err
	}

	relay(client, upstream)
	return nil
}

func handlePlainHTTP(client net.Conn, req *http.Request, devName string) error {
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "80")
	}

	upstream, err := dialUpstream(host, devName, 10*time.Second)
	if err != nil {
		resp := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\nFailed to connect to %s\n", len(host)+23, host)
		_, _ = client.Write([]byte(resp))
		return fmt.Errorf("dial %s failed: %w", host, err)
	}
	defer upstream.Close()

	// Clean hop-by-hop and proxy headers
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Proxy-Connection")

	// Forward original request to upstream
	if err := req.Write(upstream); err != nil {
		return fmt.Errorf("forward request failed: %w", err)
	}

	// Pipe upstream response to client
	relay(client, upstream)
	return nil
}
