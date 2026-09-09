package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"aimili-vpngate-go/pkg/config"
)

func TestAuthenticator(t *testing.T) {
	auth := NewAuthenticator("user1", "pass123")
	if !auth.IsEnabled() {
		t.Fatal("expected auth to be enabled")
	}
	if !auth.Verify("user1", "pass123") {
		t.Fatal("expected credentials to verify successfully")
	}
	if auth.Verify("user1", "wrongpass") {
		t.Fatal("expected wrong password to fail")
	}

	noAuth := NewAuthenticator("", "")
	if noAuth.IsEnabled() {
		t.Fatal("expected auth to be disabled")
	}
	if !noAuth.Verify("any", "any") {
		t.Fatal("expected any credentials to pass when disabled")
	}
}

func TestGatewaySocks5AndHTTP(t *testing.T) {
	// 1. Setup a dummy target TCP echo server
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen echo server: %v", err)
	}
	defer echoLn.Close()

	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	echoPort := echoLn.Addr().(*net.TCPAddr).Port

	// 2. Setup Gateway on random port
	cfg := &config.Config{
		ProxyHost:           "127.0.0.1",
		ProxyPort:           0, // will pick dynamic port
		ProxyMaxConnections: 64,
	}

	// Listen dynamically
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen gateway: %v", err)
	}
	gatewayPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	cfg.ProxyPort = gatewayPort

	gw := NewGateway(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)

	// Test 3: SOCKS5 Client request
	t.Run("SOCKS5 Protocol", func(t *testing.T) {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", gatewayPort))
		if err != nil {
			t.Fatalf("failed to dial gateway: %v", err)
		}
		defer conn.Close()

		// Handshake: VER 0x05, NMETHODS 1, METHOD 0x00 (No auth)
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
		resp := make([]byte, 2)
		if _, err := io.ReadFull(conn, resp); err != nil || resp[0] != 0x05 || resp[1] != 0x00 {
			t.Fatalf("invalid socks5 handshake reply: %v", resp)
		}

		// Connect command: VER 0x05, CMD 0x01, RSV 0x00, ATYP 0x01 (IPv4 127.0.0.1), PORT echoPort
		portBytes := []byte{byte(echoPort >> 8), byte(echoPort & 0xff)}
		req := append([]byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1}, portBytes...)
		if _, err := conn.Write(req); err != nil {
			t.Fatalf("failed to send socks5 connect: %v", err)
		}

		connectReply := make([]byte, 10)
		if _, err := io.ReadFull(conn, connectReply); err != nil || connectReply[1] != 0x00 {
			t.Fatalf("failed socks5 connect reply: %v", connectReply)
		}

		// Send payload through proxy to echo server
		testMsg := "hello-socks5-aimili"
		_, _ = conn.Write([]byte(testMsg))
		buf := make([]byte, len(testMsg))
		if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != testMsg {
			t.Fatalf("echo verification failed, got: %s", string(buf))
		}
	})

	// Test 4: HTTP CONNECT Protocol
	t.Run("HTTP CONNECT Protocol", func(t *testing.T) {
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", gatewayPort))
		if err != nil {
			t.Fatalf("failed to dial gateway: %v", err)
		}
		defer conn.Close()

		connectReq := fmt.Sprintf("CONNECT 127.0.0.1:%d HTTP/1.1\r\nHost: 127.0.0.1:%d\r\n\r\n", echoPort, echoPort)
		_, _ = conn.Write([]byte(connectReq))

		br := bufio.NewReader(conn)
		resp, err := http.ReadResponse(br, nil)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("invalid http connect response: %v, status: %v", err, resp)
		}

		testMsg := "hello-http-connect-aimili"
		_, _ = conn.Write([]byte(testMsg))
		buf := make([]byte, len(testMsg))
		if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != testMsg {
			t.Fatalf("echo verification failed over http connect, got: %s", string(buf))
		}
	})
}
