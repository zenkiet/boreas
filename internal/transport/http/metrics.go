package httptransport

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type metricEntry struct {
	Task           string    `json:"task"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryBytes    int64     `json:"memory_bytes"`
	MemoryLimit    int64     `json:"memory_limit"`
	NetworkRXBytes int64     `json:"network_rx_bytes"`
	NetworkTXBytes int64     `json:"network_tx_bytes"`
	ObservedAt     time.Time `json:"observed_at"`
}

// Serves both routes: the project one leaves the task wildcard empty, which the service
// reads as "every task".
func (h *Handler) streamMetrics(w http.ResponseWriter, r *http.Request) {
	samples, err := h.tasks.Metrics(r.Context(), accessFrom(r.Context()), r.PathValue("name"))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	for {
		frame := ": heartbeat\n\n"
		select {
		case sample, ok := <-samples:
			if !ok {
				return
			}
			payload, _ := json.Marshal(metricEntry{
				Task: sample.TaskName, CPUPercent: sample.CPUPercent,
				MemoryBytes: sample.MemoryBytes, MemoryLimit: sample.MemoryLimit,
				NetworkRXBytes: sample.NetworkRXBytes, NetworkTXBytes: sample.NetworkTXBytes,
				ObservedAt: sample.ObservedAt,
			})
			frame = "data: " + string(payload) + "\n\n"
		case <-heartbeat.C:
		case <-r.Context().Done():
			return
		}
		if _, err := io.WriteString(w, frame); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}
