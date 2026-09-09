package nodes

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	MaxSnapshotBytes = 12 * 1024 * 1024 // 12 MB
	MaxConfigBytes   = 128 * 1024       // 128 KB
)

var safeDirectives = map[string]bool{
	"auth":                  true,
	"cipher":                true,
	"client":                true,
	"comp-lzo":              true,
	"compress":              true,
	"connect-retry":         true,
	"connect-retry-max":     true,
	"connect-timeout":       true,
	"data-ciphers":          true,
	"data-ciphers-fallback": true,
	"dev":                   true,
	"dev-type":              true,
	"dhcp-option":           true,
	"explicit-exit-notify":  true,
	"keepalive":             true,
	"key-direction":         true,
	"mute":                  true,
	"nobind":                true,
	"persist-key":           true,
	"persist-tun":           true,
	"ping":                  true,
	"ping-restart":          true,
	"ping-timer-rem":        true,
	"proto":                 true,
	"pull":                  true,
	"rcvbuf":                true,
	"remote":                true,
	"remote-cert-tls":       true,
	"remote-random":         true,
	"remote-random-hostname": true,
	"reneg-sec":             true,
	"resolv-retry":          true,
	"route-delay":           true,
	"sndbuf":                true,
	"tls-cipher":            true,
	"tls-ciphersuites":      true,
	"tls-client":            true,
	"tls-version-min":       true,
	"verb":                  true,
	"verify-x509-name":      true,
}

var safeInlineBlocks = map[string]bool{
	"ca":        true,
	"cert":      true,
	"key":       true,
	"tls-auth":  true,
	"tls-crypt": true,
}

var (
	tagOpenRegex  = regexp.MustCompile(`^<([a-zA-Z0-9_\-]+)>$`)
	tagCloseRegex = regexp.MustCompile(`^</([a-zA-Z0-9_\-]+)>$`)
)

func DecodeConfig(encoded string) (string, error) {
	compact := strings.Join(strings.Fields(encoded), "")
	if compact == "" {
		return "", errors.New("empty openvpn config")
	}

	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	if len(decoded) == 0 || len(decoded) > MaxConfigBytes {
		return "", fmt.Errorf("config size out of bounds: %d bytes", len(decoded))
	}

	return string(decoded), nil
}

func ValidateOpenVPNConfig(configText string) error {
	lines := strings.Split(configText, "\n")
	var currentBlock string
	hasRemote := false

	for lineIdx, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if currentBlock != "" {
			if matches := tagCloseRegex.FindStringSubmatch(line); len(matches) == 2 {
				closingTag := strings.ToLower(matches[1])
				if closingTag != currentBlock {
					return fmt.Errorf("mismatched closing tag </%s>, expected </%s> at line %d", closingTag, currentBlock, lineIdx+1)
				}
				currentBlock = ""
			}
			continue
		}

		if matches := tagOpenRegex.FindStringSubmatch(line); len(matches) == 2 {
			tag := strings.ToLower(matches[1])
			if !safeInlineBlocks[tag] {
				return fmt.Errorf("unsafe inline block <%s> at line %d", tag, lineIdx+1)
			}
			currentBlock = tag
			continue
		}

		parts := strings.Fields(line)
		directive := strings.ToLower(parts[0])

		if !safeDirectives[directive] {
			return fmt.Errorf("unsafe or disallowed directive '%s' at line %d", directive, lineIdx+1)
		}

		if directive == "remote" {
			hasRemote = true
		}
	}

	if currentBlock != "" {
		return fmt.Errorf("unclosed inline block <%s>", currentBlock)
	}

	if !hasRemote {
		return errors.New("openvpn config missing required 'remote' directive")
	}

	return nil
}
