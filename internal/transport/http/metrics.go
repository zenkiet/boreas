package httptransport

import (
	"net/http"

	"github.com/zenkiet/boreas/internal/core"
)

// Serves both routes; the project route leaves the task wildcard empty, which the service reads as every task.
func (h *Handler) streamMetrics(w http.ResponseWriter, r *http.Request) {
	samples, err := h.tasks.Metrics(r.Context(), r.PathValue("name"), accessFrom(r.Context()))
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	sse(w, r, samples, func(s core.TaskMetric) (string, any) { return "", s })
}

func (h *Handler) streamFleetMetrics(w http.ResponseWriter, r *http.Request) {
	accesses, err := h.projects.List(r.Context(), userFrom(r.Context()))
	var samples <-chan core.TaskMetric
	if err == nil {
		samples, err = h.tasks.Metrics(r.Context(), "", accesses...)
	}
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	sse(w, r, samples, func(s core.TaskMetric) (string, any) { return "", s })
}
