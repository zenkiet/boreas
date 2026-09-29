package httptransport

import (
	"net/http"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

type metricEntry struct {
	TaskName       string    `json:"task"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryBytes    int64     `json:"memory_bytes"`
	MemoryLimit    int64     `json:"memory_limit"`
	NetworkRXBytes int64     `json:"network_rx_bytes"`
	NetworkTXBytes int64     `json:"network_tx_bytes"`
	ObservedAt     time.Time `json:"observed_at"`
}

// Serves both routes; the project route leaves the task wildcard empty, which the service reads as every task.
func (h *Handler) streamMetrics(w http.ResponseWriter, r *http.Request) {
	samples, err := h.tasks.Metrics(r.Context(), accessFrom(r.Context()), r.PathValue("name"))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	sse(w, r, samples, func(s core.TaskMetric) any { return metricEntry(s) })
}
