package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesFilesAndFallsBackToShell(t *testing.T) {
	h := Handler()
	for path, contentType := range map[string]string{
		"/":                        "text/html",
		"/robots.txt":              "text/plain",
		"/_app/version.json":       "application/json",
		"/blogic-view/pos-express": "text/html",
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rr.Header().Get("Content-Type"); rr.Code != http.StatusOK || !strings.HasPrefix(got, contentType) {
			t.Fatalf("%s: status=%d type=%q, want 200 %s (run 'make frontend')", path, rr.Code, got, contentType)
		}
	}
}
