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
	status, message := errorStatus(err)
	switch status {
	case http.StatusUnauthorized:
		w.Header().Set("WWW-Authenticate", `Bearer realm="Boreas"`)
	case http.StatusInternalServerError:
		logger.Error("http transport", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func errorStatus(err error) (status int, message string) {
	switch {
	case errors.Is(err, core.ErrInvalidInput):
		return http.StatusBadRequest, "invalid request"
	case errors.Is(err, core.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, core.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, core.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, core.ErrAlreadyExists), errors.Is(err, core.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, core.ErrTooMany):
		return http.StatusTooManyRequests, "too many requests"
	}
	return http.StatusInternalServerError, "internal server error"
}

// errorKind names a status as the app's ApiErrorKind does.
func errorKind(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid-input"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not-found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate-limited"
	}
	return "server"
}

func (h *Handler) noContent(w http.ResponseWriter, err error) {
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
