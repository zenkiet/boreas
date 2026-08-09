package httptransport

import (
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

func recoverPanic(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, recovered, debug.Stack())
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

func logRequests(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := &responseRecorder{ResponseWriter: w}
		defer func() {
			status := rw.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.Printf("%s %s status=%d bytes=%d duration=%s", r.Method, r.URL.RequestURI(), status, rw.bytes, time.Since(started))
		}()
		next.ServeHTTP(rw, r)
	})
}

func cors(cfg CORSConfig) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[origin] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				_, exact := allowed[origin]
				_, wildcard := allowed["*"]
				if exact || wildcard {
					value := origin
					if wildcard && !cfg.AllowCredentials {
						value = "*"
					}
					w.Header().Set("Access-Control-Allow-Origin", value)
					w.Header().Add("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
					if cfg.AllowCredentials {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
