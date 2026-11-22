package web

import (
	"compress/gzip"
	"io"
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

func TestHandlerRevalidatesFilesByETag(t *testing.T) {
	h := Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	etag := rr.Header().Get("ETag")
	if etag == "" || rr.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("headers = %v, want an ETag and no-cache (run 'make frontend')", rr.Header())
	}
	request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	request.Header.Set("If-None-Match", etag)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, request)
	if rr.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 for an unchanged file", rr.Code)
	}
}

func TestHandlerServesPrecompressedFiles(t *testing.T) {
	h := Handler()
	for accept, want := range map[string]string{"gzip, deflate, br, zstd": "br", "gzip": "gzip", "": ""} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Accept-Encoding", accept)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, request)
		header := rr.Header()
		if header.Get("Content-Encoding") != want || !strings.HasPrefix(header.Get("Content-Type"), "text/html") || header.Get("Vary") != "Accept-Encoding" {
			t.Fatalf("Accept-Encoding %q: headers = %v, want %q (run 'make frontend')", accept, header, want)
		}
		if want == "gzip" {
			body, err := gzip.NewReader(rr.Body)
			if err != nil {
				t.Fatal(err)
			}
			if html, _ := io.ReadAll(body); !strings.HasPrefix(string(html), "<!doctype html>") {
				t.Fatalf("gzip body = %.40q, want the page", html)
			}
		}
	}
}

func TestHandlerAnswersMissingFilesWith404(t *testing.T) {
	h := Handler()
	for path, want := range map[string]int{
		"/favicon.ico": http.StatusNotFound, "/.well-known/ai-catalog.json": http.StatusNotFound,
		"/llms.txt": http.StatusOK, "/portal/v1.2": http.StatusOK,
	} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != want {
			t.Fatalf("%s = %d, want %d (run 'make frontend')", path, rr.Code, want)
		}
	}
}

func TestSupportServesTheLoaderWithTheInbox(t *testing.T) {
	get := func(h http.Handler, etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/_boreas/support.js", nil)
		r.Header.Set("If-None-Match", etag)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr
	}
	if rr := get(Support(SupportConfig{}), ""); rr.Code != http.StatusOK || rr.Body.Len() != 0 {
		t.Fatalf("without an inbox: status=%d body=%q, want an empty script", rr.Code, rr.Body)
	}

	h := Support(SupportConfig{BaseURL: "https://desk.example", InboxID: "inbox-1"})
	rr := get(h, "")
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Header().Get("Content-Type"), "text/javascript") ||
		!strings.HasPrefix(body, `self.LibredeskSettings={"baseURL":"https://desk.example","inboxID":"inbox-1"};`) ||
		!strings.Contains(body, "/widget.js") {
		t.Fatalf("status=%d body=%q", rr.Code, body)
	}
	if again := get(h, rr.Header().Get("ETag")); again.Code != http.StatusNotModified {
		t.Fatalf("revalidation status = %d, want 304", again.Code)
	}
}
