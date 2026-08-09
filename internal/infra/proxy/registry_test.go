package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestRegistryProxyPathHeadersAndHTML(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/some/path" || req.URL.RawQuery != "x=1" {
			t.Errorf("upstream URL = %s", req.URL.String())
		}
		if req.Header.Get("X-Boreas-Project") != "team" {
			t.Errorf("project header = %q", req.Header.Get("X-Boreas-Project"))
		}
		if req.Header.Get("X-Boreas-Task") != "Task.1" {
			t.Errorf("task header = %q", req.Header.Get("X-Boreas-Task"))
		}
		if req.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("encoding = %q", req.Header.Get("Accept-Encoding"))
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
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "first")
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "second")
	}))
	defer second.Close()

	registry := New(0, 0)
	defer registry.CloseIdleConnections()
	host, port := serverAddress(t, first.URL)
	registry.Register(context.Background(), "alpha", "web", host, port)
	host, port = serverAddress(t, second.URL)
	registry.Register(context.Background(), "beta", "web", host, port)

	r := httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/alpha/web/", nil))
	if r.Body.String() != "first" {
		t.Fatalf("alpha/web = %q", r.Body.String())
	}
	r = httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/beta/web/", nil))
	if r.Body.String() != "second" {
		t.Fatalf("beta/web = %q", r.Body.String())
	}

	if err := registry.Unregister(context.Background(), "alpha", "web"); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/beta/web/", nil))
	if r.Body.String() != "second" {
		t.Fatal("unregistering alpha/web removed beta/web")
	}
}

func TestRegistryDoesNotModifyNonHTML(t *testing.T) {
	const payload = `<head><base href="/leave/">`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, payload)
	}))
	defer upstream.Close()
	host, port := serverAddress(t, upstream.URL)
	registry := New(0, 0)
	registry.Register(context.Background(), "team", "id", host, port)
	r := httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/team/id/data", nil))
	if r.Body.String() != payload {
		t.Fatalf("body = %q", r.Body.String())
	}
}

func TestRegistryDoesNotModifyEncodedHTML(t *testing.T) {
	const payload = `encoded <head><base href="/leave/">`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "br")
		io.WriteString(w, payload)
	}))
	defer upstream.Close()
	host, port := serverAddress(t, upstream.URL)
	registry := New(0, 0)
	registry.Register(context.Background(), "team", "id", host, port)
	r := httptest.NewRecorder()
	registry.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/team/id/", nil))
	if r.Body.String() != payload {
		t.Fatalf("body = %q", r.Body.String())
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
