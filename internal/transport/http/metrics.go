package httptransport

import (
	"net/http"

	"github.com/zenkiet/boreas/internal/core"
)

// Serves both routes; the project route leaves the task wildcard empty, which the service reads as every task.
func (h *Handler) streamMetrics(w http.ResponseWriter, r *http.Request) {
	samples, err := h.tasks.Metrics(r.Context(), accessFrom(r.Context()), r.PathValue("name"))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	sse(w, r, samples, func(s core.TaskMetric) (string, any) { return "", s })
}
