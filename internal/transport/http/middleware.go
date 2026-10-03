package httptransport

import (
	"cmp"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

const (
	allowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	allowedHeaders = "Authorization, Content-Type"
)

func recoverPanic(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic serving request",
					"method", r.Method, "path", r.URL.Path,
					"panic", recovered, "stack", string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

// Unwrap exposes streaming capabilities through the recorder.
func (w *responseRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responseRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (h *Handler) publishing(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rw := &responseRecorder{ResponseWriter: w}
		next(rw, r)
		if cmp.Or(rw.status, http.StatusOK) < http.StatusBadRequest {
			h.events.Publish()
		}
	}
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := &responseRecorder{ResponseWriter: w}
		defer func() {
			logger.Info("request",
				"method", r.Method, "uri", r.URL.RequestURI(),
				"status", cmp.Or(rw.status, http.StatusOK), "bytes", rw.bytes, "duration", time.Since(started))
		}()
		next.ServeHTTP(rw, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
