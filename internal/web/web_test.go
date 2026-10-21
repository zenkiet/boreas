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

func TestHandlerServesThePrerenderedPolicy(t *testing.T) {
	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/legal/privacy", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Bora&#39;s voice") {
		t.Fatalf("status=%d, want the prerendered policy, not the shell (run 'make frontend')", rr.Code)
	}
}
