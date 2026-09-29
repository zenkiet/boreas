package httptransport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/zenkiet/boreas/internal/core"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// decode answers 400 itself on a malformed, oversized or trailing body, so callers only return.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil || dec.Decode(&struct{}{}) != io.EOF {
		writeBadRequest(w)
		return false
	}
	return true
}

func writeBadRequest(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
}

// reply builds v before checking err, so v must be safe to build from zero values.
func (h *Handler) reply(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, status, v)
}

func writeServiceError(w http.ResponseWriter, logger *slog.Logger, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, core.ErrInvalidInput):
		status, message = http.StatusBadRequest, "invalid request"
	case errors.Is(err, core.ErrUnauthorized):
		w.Header().Set("WWW-Authenticate", `Bearer realm="Boreas"`)
		status, message = http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, core.ErrForbidden):
		status, message = http.StatusForbidden, "forbidden"
	case errors.Is(err, core.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, core.ErrAlreadyExists), errors.Is(err, core.ErrConflict):
		status, message = http.StatusConflict, "conflict"
	default:
		logger.Error("http transport", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}
