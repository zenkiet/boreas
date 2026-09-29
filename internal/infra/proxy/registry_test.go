package proxy

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestRegistryProxyPathHeadersAndHTML(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got := strings.Join([]string{
			req.URL.String(), req.Header.Get("X-Boreas-Project"),
			req.Header.Get("X-Boreas-Task"), req.Header.Get("Accept-Encoding"),
		}, " "); got != "/some/path?x=1 team Task.1 identity" {
			t.Errorf("upstream saw %q", got)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<HTML><HEAD><base href="/old/"><title>x</title></HEAD><body></body></HTML>`)
	}))
	defer upstream.Close()
	host, port := serverAddress(t, upstream.URL)
	registry := New(0, 0)
	defer registry.CloseIdleConnections()
	if err := registry.Register(context.Background(), "team", "Task.1", host, port); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://boreas/team/Task.1/some/path?x=1", nil)
	registry.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `<base href="/team/Task.1/">`) || strings.Contains(body, "/old/") {
		t.Fatalf("HTML = %s", body)
	}
}

func TestRegistryRejectsInvalidNames(t *testing.T) {
	registry := New(0, 0)
	if err := registry.Register(context.Background(), "API", "task", "127.0.0.1", 80); err == nil {
		t.Fatal("expected invalid project slug to be rejected")
	}
	if err := registry.Register(context.Background(), "api", "task", "127.0.0.1", 80); err == nil {
		t.Fatal("expected reserved project slug to be rejected")
	}
	if err := registry.Register(context.Background(), "team", "bad name", "127.0.0.1", 80); err == nil {
		t.Fatal("expected invalid task name to be rejected")
	}
}

func TestRegistryRedirectAndLocationRewrite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/login?next=1")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	host, port := serverAddress(t, upstream.URL)
	registry := New(0, 0)
	registry.Register(context.Background(), "team", "id", host, port)

	r := httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/team/id?q=1", nil))
	if r.Code != http.StatusPermanentRedirect || r.Header().Get("Location") != "/team/id/?q=1" {
		t.Fatalf("redirect = %d %q", r.Code, r.Header().Get("Location"))
	}
	r = httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/team/id/private", nil))
	if r.Header().Get("Location") != "/team/id/login?next=1" {
		t.Fatalf("Location = %q", r.Header().Get("Location"))
	}
}

func TestRegistrySameTaskNameInDifferentProjects(t *testing.T) {
	registry := New(0, 0)
	defer registry.CloseIdleConnections()
	for _, project := range []string{"alpha", "beta"} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, project)
		}))
		t.Cleanup(upstream.Close)
		host, port := serverAddress(t, upstream.URL)
		registry.Register(context.Background(), project, "web", host, port)
	}
	get := func(path string) string {
		r := httptest.NewRecorder()
		registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		return r.Body.String()
	}
	if a, b := get("/alpha/web/"), get("/beta/web/"); a != "alpha" || b != "beta" {
		t.Fatalf("alpha/web = %q, beta/web = %q", a, b)
	}
	if err := registry.Unregister(context.Background(), "alpha", "web"); err != nil {
		t.Fatal(err)
	}
	if get("/beta/web/") != "beta" {
		t.Fatal("unregistering alpha/web removed beta/web")
	}
}

func TestRegistryLeavesNonHTMLAndEncodedHTMLAlone(t *testing.T) {
	const payload = `<head><base href="/leave/">`
	for _, header := range []http.Header{
		{"Content-Type": {"application/json"}},
		{"Content-Type": {"text/html"}, "Content-Encoding": {"br"}},
	} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			maps.Copy(w.Header(), header)
			io.WriteString(w, payload)
		}))
		t.Cleanup(upstream.Close)
		host, port := serverAddress(t, upstream.URL)
		registry := New(0, 0)
		registry.Register(context.Background(), "team", "id", host, port)
		r := httptest.NewRecorder()
		registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/team/id/", nil))
		if r.Body.String() != payload {
			t.Fatalf("%v: body = %q", header, r.Body.String())
		}
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	registry := New(0, 0)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			task := fmt.Sprintf("id-%d", i)
			_ = registry.Register(context.Background(), "team", task, "127.0.0.1", 8000+i)
			_ = registry.Unregister(context.Background(), "team", task)
		}(i)
		go func(i int) {
			defer wg.Done()
			registry.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, fmt.Sprintf("/team/id-%d/", i), nil))
		}(i)
	}
	wg.Wait()
}

func TestRegistryFallbackServesUnroutedPaths(t *testing.T) {
	registry := New(0, 0)
	registry.Fallback = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	for _, path := range []string{"/", "/blogic-view", "/blogic-view/", "/blogic-view/missing"} {
		rr := httptest.NewRecorder()
		registry.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusTeapot {
			t.Fatalf("%s status=%d, want fallback", path, rr.Code)
		}
	}

	registry.Fallback = nil
	rr := httptest.NewRecorder()
	registry.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/blogic-view/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404 without a fallback", rr.Code)
	}
}

func serverAddress(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	fmt.Sscanf(u.Port(), "%d", &port)
	return u.Hostname(), port
}
