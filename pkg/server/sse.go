package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aimili-vpngate-go/pkg/stats"
)

type SSEHub struct {
	server *Server
}

func NewSSEHub(s *Server) *SSEHub {
	return &SSEHub{server: s}
}

func (hub *SSEHub) HandleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	logChan := stats.GetRingLog().Subscribe()
	defer stats.GetRingLog().Unsubscribe(logChan)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Initial ping
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case entry, ok := <-logChan:
			if !ok {
				return
			}
			data, err := json.Marshal(entry)
			if err == nil {
				_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
				flusher.Flush()
			}

		case <-ticker.C:
			// Send heartbeat state
			state := hub.server.buildStatusResponse()
			data, err := json.Marshal(state)
			if err == nil {
				_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}
