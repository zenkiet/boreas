package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/zenkiet/boreas/internal/core"
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
	request.Header.Set("Sec-Fetch-Dest", "document")
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

func serve(registry *Registry, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	registry.ServeHTTP(recorder, request)
	return recorder
}

func TestRegistryRoutes(t *testing.T) {
	var seen http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		if strings.HasSuffix(r.URL.Path, "/login") {
			w.Header().Set("Location", "/base/home")
			w.Header().Add("Set-Cookie", "sid=1; Path=/; Domain=pay.lan; HttpOnly")
		}
		io.WriteString(w, r.URL.RequestURI())
	}))
	defer backend.Close()
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "app "+r.URL.RequestURI())
	}))
	defer app.Close()

	registry := New(0, 0)
	defer registry.CloseIdleConnections()
	registry.SetRoutes("p", "t", []core.Route{
		{Path: "/api", URL: backend.URL},
		{Path: "/api/v2", URL: backend.URL + "/two"},
		{Path: "/keep", URL: backend.URL + "/api"},
		{Path: "/pay", URL: backend.URL + "/base"},
	})
	if got := serve(registry, httptest.NewRequest(http.MethodGet, "/p/t/api/orders", nil)).Body.String(); got != "/orders" {
		t.Fatalf("a route must answer while its task is stopped, got %q", got)
	}
	host, port := serverAddress(t, app.URL)
	registry.Register(context.Background(), "p", "t", host, port)

	for path, want := range map[string]string{
		"/p/t/orders":           "app /orders",
		"/p/t/api/orders?id=7":  "/orders?id=7",
		"/p/t/keep/orders":      "/api/orders",
		"/p/t/pay/v2/charge":    "/base/v2/charge",
		"/p/t/api":              "/",
		"/p/t/apis":             "app /apis",
		"/p/t/api/v2/x":         "/two/x",
		"/p/t/api/a%2Fb?x=%2F1": "/a%2Fb?x=%2F1",
	} {
		if got := serve(registry, httptest.NewRequest(http.MethodGet, path, nil)).Body.String(); got != want {
			t.Errorf("%s reached %q, want %q", path, got, want)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/p/t/pay/login", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-For", "203.0.113.7")
	request.Header.Set("Accept-Encoding", "br")
	response := serve(registry, request)
	if got := strings.Join([]string{
		seen.Get("X-Forwarded-For"), seen.Get("X-Forwarded-Proto"), seen.Get("X-Forwarded-Prefix"), seen.Get("X-Boreas-Route"), seen.Get("Accept-Encoding"),
		response.Header().Get("Location"), response.Header().Get("Set-Cookie"),
	}, " | "); got != "203.0.113.7, 192.0.2.1 | https | /p/t/pay/ | /pay | br | /p/t/pay/home | sid=1; Path=/p/t/; HttpOnly" {
		t.Fatalf("headers = %q", got)
	}

	request = httptest.NewRequest(http.MethodGet, "/p/t/api/x", nil)
	request.Header.Set("X-Boreas-Route", "/api")
	if code := serve(registry, request).Code; code != http.StatusLoopDetected {
		t.Fatalf("a route reaching Boreas again must stop, got %d", code)
	}
}

func TestRegistryUnreachableRoute(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	app := httptest.NewServer(http.NotFoundHandler())
	defer app.Close()
	registry := New(0, 0)
	host, port := serverAddress(t, app.URL)
	registry.Register(context.Background(), "p", "t", host, port)
	registry.SetRoutes("p", "t", []core.Route{{Path: "/down", URL: closed.URL}})

	response := serve(registry, httptest.NewRequest(http.MethodGet, "/p/t/down/x", nil))
	if body := response.Body.String(); response.Code != http.StatusBadGateway || body != `{"error":"route_unreachable","route":"/down"}`+"\n" {
		t.Fatalf("API caller got %d %s", response.Code, body)
	}
	request := httptest.NewRequest(http.MethodGet, "/p/t/down/x", nil)
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	body := serve(registry, request).Body.String()
	for _, want := range []string{
		"t can’t reach /down right now", "The connection was refused.", "The app itself is running",
		`href="boreas://projects/p/tasks/t"`,
	} {
		if !strings.Contains(body, want) || strings.Contains(body, closed.URL) {
			t.Fatalf("page lacks %q or shows the address:\n%s", want, body)
		}
	}
}

func TestRegistryCheck(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	selfSigned := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer selfSigned.Close()
	registry := New(0, 0)
	for _, c := range []struct {
		route core.Route
		want  core.RouteCheck
	}{
		{core.Route{URL: closed.URL}, core.RouteCheck{Error: "refused"}},
		{core.Route{URL: selfSigned.URL}, core.RouteCheck{Error: "certificate"}},
		{core.Route{URL: selfSigned.URL, AnyCert: true}, core.RouteCheck{Status: http.StatusTeapot}},
	} {
		got := registry.Check(context.Background(), c.route)
		if got.Status != c.want.Status || got.Error != c.want.Error {
			t.Fatalf("Check(%+v) = %+v", c.route, got)
		}
	}
}

func TestKind(t *testing.T) {
	for err, want := range map[error]string{
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}: "refused",
		fmt.Errorf("dial: %w", context.DeadlineExceeded):                                   "timeout",
		&net.DNSError{IsNotFound: true}:                                                    "not_found",
		&tls.CertificateVerificationError{}:                                                "certificate",
		errors.New("reset"):                                                                "unreachable",
	} {
		if got := kind(err); got != want {
			t.Errorf("kind(%v) = %q, want %q", err, got, want)
		}
	}
}
