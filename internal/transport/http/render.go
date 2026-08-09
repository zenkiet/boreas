package httptransport

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/zenkiet/boreas/internal/core"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, max int64, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeBadRequest(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
}

func writeServiceError(w http.ResponseWriter, logger *log.Logger, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, core.ErrInvalidInput):
		status, message = http.StatusBadRequest, "invalid request"
	case errors.Is(err, core.ErrUnauthorized):
		status, message = http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, core.ErrForbidden):
		status, message = http.StatusForbidden, "forbidden"
	case errors.Is(err, core.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, core.ErrAlreadyExists), errors.Is(err, core.ErrConflict):
		status, message = http.StatusConflict, "conflict"
	default:
		logger.Printf("http transport: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": message})
}
