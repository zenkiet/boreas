package httptransport

import (
	"log"
	"net/http"
	"strings"
)

// ApplicationHandler routes the protected API separately so proxied task traffic stays public.
func ApplicationHandler(api, proxy http.Handler, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1" || strings.HasPrefix(r.URL.Path, "/api/v1/"):
			api.ServeHTTP(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"service":"boreas","status":"healthy"}`))
		case proxy != nil:
			proxy.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	return recoverPanic(logger, logRequests(logger, handler))
}
