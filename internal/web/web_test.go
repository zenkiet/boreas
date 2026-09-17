package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesFilesAndFallsBackToShell(t *testing.T) {
	h := Handler()

	for _, path := range []string{"/", "/robots.txt"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d, want 200 (run 'make frontend')", path, rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/blogic-view/pos-express", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "<!doctype html>") {
		t.Fatalf("shell status=%d body=%.60q", rr.Code, rr.Body.String())
	}
}

func TestHandlerCachesImmutableAssets(t *testing.T) {
	rr := httptest.NewRecorder()
	Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/_app/version.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d, want the embedded _app tree (all: prefix missing?)", rr.Code)
	}
}
