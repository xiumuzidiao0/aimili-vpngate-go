package nodes

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

var requiredColumns = []string{
	"HostName",
	"IP",
	"Score",
	"Ping",
	"Speed",
	"CountryLong",
	"CountryShort",
	"NumVpnSessions",
	"OpenVPN_ConfigData_Base64",
}

func parseRemoteFromConfig(configText string) (string, int, string) {
	proto := "udp"
	port := 1194
	ip := ""

	lines := strings.Split(configText, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "proto ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				p := strings.ToLower(fields[1])
				if strings.Contains(p, "tcp") {
					proto = "tcp"
				} else {
					proto = "udp"
				}
			}
		} else if strings.HasPrefix(line, "remote ") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				ip = fields[1]
				if p, err := strconv.Atoi(fields[2]); err == nil && p > 0 && p < 65536 {
					port = p
				}
				if len(fields) >= 4 {
					pr := strings.ToLower(fields[3])
					if strings.Contains(pr, "tcp") {
						proto = "tcp"
					} else if strings.Contains(pr, "udp") {
						proto = "udp"
					}
				}
			}
		}
	}
	return ip, port, proto
}

func ParseVPNGateCSV(data []byte, maxRows int) ([]*Node, error) {
	if len(data) == 0 {
		return nil, errors.New("vpngate csv is empty")
	}
	if len(data) > MaxSnapshotBytes {
		return nil, fmt.Errorf("vpngate csv exceeds max size %d bytes", MaxSnapshotBytes)
	}

	// Filter out comment lines starting with '*' and empty lines
	var buf bytes.Buffer
	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "*") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "#")
		buf.WriteString(trimmed)
		buf.WriteString("\n")
	}

	reader := csv.NewReader(&buf)
	reader.FieldsPerRecord = -1

	headers, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read csv headers: %w", err)
	}

	colIdx := make(map[string]int)
	for i, h := range headers {
		colIdx[strings.TrimSpace(h)] = i
	}

	maxRequiredIndex := -1
	for _, col := range requiredColumns {
		idx, ok := colIdx[col]
		if !ok {
			return nil, fmt.Errorf("missing required column '%s'", col)
		}
		if idx > maxRequiredIndex {
			maxRequiredIndex = idx
		}
	}

	var nodes []*Node
	seenIPs := make(map[string]bool)

	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			continue
		}
		if len(record) <= maxRequiredIndex {
			continue
		}

		ip := strings.TrimSpace(record[colIdx["IP"]])
		if ip == "" || seenIPs[ip] {
			continue
		}

		rawConfig := strings.TrimSpace(record[colIdx["OpenVPN_ConfigData_Base64"]])
		if rawConfig == "" {
			continue
		}

		configText, err := DecodeConfig(rawConfig)
		if err != nil {
			continue
		}

		if err := ValidateOpenVPNConfig(configText); err != nil {
			continue
		}

		remoteIP, port, proto := parseRemoteFromConfig(configText)
		if remoteIP == "" {
			remoteIP = ip
		}

		score, _ := strconv.ParseInt(strings.TrimSpace(record[colIdx["Score"]]), 10, 64)
		ping, _ := strconv.Atoi(strings.TrimSpace(record[colIdx["Ping"]]))
		speed, _ := strconv.ParseInt(strings.TrimSpace(record[colIdx["Speed"]]), 10, 64)
		numSessions, _ := strconv.Atoi(strings.TrimSpace(record[colIdx["NumVpnSessions"]]))

		var uptime, totalUsers, totalTraffic int64
		if idx, ok := colIdx["Uptime"]; ok && idx < len(record) {
			uptime, _ = strconv.ParseInt(strings.TrimSpace(record[idx]), 10, 64)
		}
		if idx, ok := colIdx["TotalUsers"]; ok && idx < len(record) {
			totalUsers, _ = strconv.ParseInt(strings.TrimSpace(record[idx]), 10, 64)
		}
		if idx, ok := colIdx["TotalTraffic"]; ok && idx < len(record) {
			totalTraffic, _ = strconv.ParseInt(strings.TrimSpace(record[idx]), 10, 64)
		}

		var logType, operator, message string
		if idx, ok := colIdx["LogType"]; ok && idx < len(record) {
			logType = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["Operator"]; ok && idx < len(record) {
			operator = strings.TrimSpace(record[idx])
		}
		if idx, ok := colIdx["Message"]; ok && idx < len(record) {
			message = strings.TrimSpace(record[idx])
		}

		nodeID := net.JoinHostPort(ip, strconv.Itoa(port))

		node := &Node{
			ID:             nodeID,
			HostName:       strings.TrimSpace(record[colIdx["HostName"]]),
			IP:             ip,
			Score:          score,
			Ping:           ping,
			Speed:          speed,
			CountryLong:    strings.TrimSpace(record[colIdx["CountryLong"]]),
			CountryShort:   strings.ToUpper(strings.TrimSpace(record[colIdx["CountryShort"]])),
			NumVpnSessions: numSessions,
			Uptime:         uptime,
			TotalUsers:     totalUsers,
			TotalTraffic:   totalTraffic,
			LogType:        logType,
			Operator:       operator,
			Message:        message,
			ConfigData:     configText,
			Proto:          proto,
			Port:           port,
			LatencyMs:      -1,
		}

		nodes = append(nodes, node)
		seenIPs[ip] = true

		if maxRows > 0 && len(nodes) >= maxRows {
			break
		}
	}

	if len(nodes) == 0 {
		return nil, errors.New("no valid nodes found in snapshot")
	}

	return nodes, nil
}
