package proxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"aimili-vpngate-go/pkg/tunnel"
)

const (
	socks5Version = 0x05

	// Auth methods
	authMethodNoAuth   = 0x00
	authMethodUserPass = 0x02
	authMethodNoAccept = 0xff

	// Commands
	cmdConnect = 0x01

	// Address types
	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	// Reply codes
	repSuccess           = 0x00
	repGeneralFailure    = 0x01
	repCommandNotSupport = 0x07
	repAtypNotSupport    = 0x08
)

func handleSocks5(client net.Conn, auth *Authenticator, devName string, tun *tunnel.Tunnel) error {
	// 1. Negotiation
	// Client sends: VER (1 byte) | NMETHODS (1 byte) | METHODS (1-255 bytes)
	verBuf := make([]byte, 1)
	if _, err := io.ReadFull(client, verBuf); err != nil {
		return fmt.Errorf("read socks5 version failed: %w", err)
	}
	if verBuf[0] != socks5Version {
		return fmt.Errorf("invalid socks5 version: %d", verBuf[0])
	}

	header := make([]byte, 1) // NMETHODS
	if _, err := io.ReadFull(client, header); err != nil {
		return fmt.Errorf("read nmethods failed: %w", err)
	}
	nMethods := int(header[0])
	if nMethods <= 0 {
		return errors.New("socks5 nmethods must be > 0")
	}

	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(client, methods); err != nil {
		return fmt.Errorf("read methods failed: %w", err)
	}

	methodMap := make(map[byte]bool)
	for _, m := range methods {
		methodMap[m] = true
	}

	if auth.IsEnabled() {
		if !methodMap[authMethodUserPass] {
			_, _ = client.Write([]byte{socks5Version, authMethodNoAccept})
			return errors.New("client does not support user/password auth")
		}
		if _, err := client.Write([]byte{socks5Version, authMethodUserPass}); err != nil {
			return err
		}

		// RFC 1929: VER (0x01) | ULEN | UNAME | PLEN | PASSWD
		authVer := make([]byte, 1)
		if _, err := io.ReadFull(client, authVer); err != nil || authVer[0] != 0x01 {
			_, _ = client.Write([]byte{0x01, 0x01})
			return errors.New("invalid socks5 subnegotiation auth version")
		}

		uLenBuf := make([]byte, 1)
		if _, err := io.ReadFull(client, uLenBuf); err != nil {
			return err
		}
		uLen := int(uLenBuf[0])
		username := make([]byte, uLen)
		if _, err := io.ReadFull(client, username); err != nil {
			return err
		}

		pLenBuf := make([]byte, 1)
		if _, err := io.ReadFull(client, pLenBuf); err != nil {
			return err
		}
		pLen := int(pLenBuf[0])
		password := make([]byte, pLen)
		if _, err := io.ReadFull(client, password); err != nil {
			return err
		}

		if !auth.Verify(string(username), string(password)) {
			_, _ = client.Write([]byte{0x01, 0x01}) // Status != 0 -> auth failure
			return errors.New("socks5 user/password authentication failed")
		}
		if _, err := client.Write([]byte{0x01, 0x00}); err != nil { // Status 0 -> success
			return err
		}
	} else {
		if !methodMap[authMethodNoAuth] {
			_, _ = client.Write([]byte{socks5Version, authMethodNoAccept})
			return errors.New("client does not support no-auth")
		}
		if _, err := client.Write([]byte{socks5Version, authMethodNoAuth}); err != nil {
			return err
		}
	}

	// 2. Request details: VER | CMD | RSV | ATYP | DST.ADDR | DST.PORT
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(client, reqHeader); err != nil {
		return fmt.Errorf("read socks5 request header failed: %w", err)
	}

	if reqHeader[0] != socks5Version {
		return fmt.Errorf("invalid socks5 version in request: %d", reqHeader[0])
	}

	cmd := reqHeader[1]
	if cmd != cmdConnect {
		_, _ = client.Write([]byte{socks5Version, repCommandNotSupport, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
		return fmt.Errorf("unsupported socks5 command: %d", cmd)
	}

	atyp := reqHeader[3]
	var targetHost string
	switch atyp {
	case atypIPv4:
		ipv4 := make([]byte, 4)
		if _, err := io.ReadFull(client, ipv4); err != nil {
			return err
		}
		targetHost = net.IP(ipv4).String()
	case atypDomain:
		dLenBuf := make([]byte, 1)
		if _, err := io.ReadFull(client, dLenBuf); err != nil {
			return err
		}
		domain := make([]byte, int(dLenBuf[0]))
		if _, err := io.ReadFull(client, domain); err != nil {
			return err
		}
		targetHost = string(domain)
	case atypIPv6:
		ipv6 := make([]byte, 16)
		if _, err := io.ReadFull(client, ipv6); err != nil {
			return err
		}
		targetHost = net.IP(ipv6).String()
	default:
		_, _ = client.Write([]byte{socks5Version, repAtypNotSupport, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
		return fmt.Errorf("unsupported socks5 atyp: %d", atyp)
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(client, portBuf); err != nil {
		return err
	}
	targetPort := binary.BigEndian.Uint16(portBuf)
	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(int(targetPort)))

	// 3. Connect to upstream via selected tunnel devName
	upstream, err := dialUpstream(targetAddr, devName, 10*time.Second)
	if err != nil {
		if tun != nil {
			tun.RecordFailure()
		}
		_, _ = client.Write([]byte{socks5Version, repGeneralFailure, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0})
		return fmt.Errorf("dial upstream %s failed: %w", targetAddr, err)
	}
	if tun != nil {
		tun.RecordSuccess()
	}

	// 4. Send success reply
	// Reply: VER | REP | RSV | ATYP | BND.ADDR | BND.PORT
	reply := []byte{socks5Version, repSuccess, 0x00, atypIPv4, 0, 0, 0, 0, 0, 0}
	if _, err := client.Write(reply); err != nil {
		_ = upstream.Close()
		return err
	}

	// 5. Bidirectional forward
	relay(client, upstream)
	return nil
}
