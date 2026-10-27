// Package proxy provides the task route registry and reverse proxy handler.
package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

// loopHeader marks a request a route sent; seeing it again means the route points back at Boreas.
const loopHeader = "X-Boreas-Route"

const checkTimeout = 3 * time.Second

// hop is one way through the proxy: a task's container, or one of its routes when path is set.
type hop struct {
	project string
	task    string
	path    string
	public  string
	target  *url.URL
	proxy   *httputil.ReverseProxy
}

// site outlives the container: routes keep answering while the task is stopped.
type site struct {
	task   *hop
	routes []*hop // longest path first
}

// Registry is concurrency-safe and keys sites by project because task names are only locally unique.
type Registry struct {
	mu        sync.RWMutex
	sites     map[string]site
	transport *http.Transport
	insecure  *http.Transport
	Fallback  http.Handler
}

func (r *Registry) miss(w http.ResponseWriter, request *http.Request) {
	if r.Fallback != nil {
		r.Fallback.ServeHTTP(w, request)
		return
	}
	http.NotFound(w, request)
}

func routeKey(project, task string) string { return project + "/" + task }

func New(dialTimeout, responseHeaderTimeout time.Duration) *Registry {
	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment, DialContext: dialer.DialContext,
		ForceAttemptHTTP2: true, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
	insecure := transport.Clone()
	insecure.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // a route's any_cert opts into self-signed backends
	return &Registry{sites: make(map[string]site), transport: transport, insecure: insecure}
}

func (r *Registry) Register(_ context.Context, project, task, host string, port int) error {
	if err := core.ValidateProjectSlug(project); err != nil {
		return err
	}
	if err := core.ValidateTaskName(task); err != nil {
		return err
	}
	if strings.TrimSpace(host) == "" || port < 1 || port > 65535 {
		return errors.Join(core.ErrInvalidInput, errors.New("proxy host and valid port are required"))
	}
	entry := r.newHop(project, task, "", &url.URL{Scheme: "http", Host: net.JoinHostPort(host, fmt.Sprint(port))}, false)
	r.update(project, task, func(s *site) { s.task = entry })
	return nil
}

func (r *Registry) Unregister(_ context.Context, project, task string) error {
	r.update(project, task, func(s *site) { s.task = nil })
	return nil
}

// SetRoutes replaces a task's routes; stored routes are already clean, so an unparsable one cannot occur.
func (r *Registry) SetRoutes(project, task string, routes []core.Route) {
	hops := make([]*hop, 0, len(routes))
	for _, route := range routes {
		if target, err := url.Parse(route.URL); err == nil {
			hops = append(hops, r.newHop(project, task, route.Path, target, route.AnyCert))
		}
	}
	slices.SortFunc(hops, func(a, b *hop) int { return len(b.path) - len(a.path) })
	r.update(project, task, func(s *site) { s.routes = hops })
}

func (r *Registry) update(project, task string, change func(*site)) {
	key := routeKey(project, task)
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sites[key]
	change(&s)
	if s.task == nil && len(s.routes) == 0 {
		delete(r.sites, key)
		return
	}
	r.sites[key] = s
}

func (r *Registry) CloseIdleConnections() {
	r.transport.CloseIdleConnections()
	r.insecure.CloseIdleConnections()
}

// Check skips redirects because RoundTrip never follows them, and counts any status as reached.
func (r *Registry) Check(ctx context.Context, route core.Route) core.RouteCheck {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, route.URL, nil)
	if err != nil {
		return core.RouteCheck{Error: kind(err)}
	}
	response, err := r.transportFor(route.AnyCert).RoundTrip(request)
	if err != nil {
		return core.RouteCheck{Error: kind(err), Took: time.Since(started)}
	}
	response.Body.Close()
	return core.RouteCheck{Status: response.StatusCode, Took: time.Since(started)}
}

func (r *Registry) transportFor(anyCert bool) *http.Transport {
	if anyCert {
		return r.insecure
	}
	return r.transport
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(request.URL.Path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		r.miss(w, request)
		return
	}
	r.mu.RLock()
	s := r.sites[routeKey(parts[0], parts[1])]
	r.mu.RUnlock()
	if i := slices.IndexFunc(s.routes, func(h *hop) bool { return under(request.URL.Path, h.public) }); i >= 0 {
		if request.Header.Get(loopHeader) != "" {
			http.Error(w, "this route points back at Boreas", http.StatusLoopDetected)
			return
		}
		s.routes[i].proxy.ServeHTTP(w, request)
		return
	}
	if s.task == nil {
		r.miss(w, request)
		return
	}
	// Redirect to the base path so relative assets do not resolve under "/project/".
	if len(parts) == 2 {
		location := s.task.public + "/"
		if request.URL.RawQuery != "" {
			location += "?" + request.URL.RawQuery
		}
		http.Redirect(w, request, location, http.StatusPermanentRedirect)
		return
	}
	s.task.proxy.ServeHTTP(w, request)
}

func under(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

func (r *Registry) newHop(project, task, path string, target *url.URL, anyCert bool) *hop {
	h := &hop{project: project, task: task, path: path, public: "/" + project + "/" + task + path, target: target}
	h.proxy = &httputil.ReverseProxy{
		Transport:      r.transportFor(anyCert),
		Rewrite:        h.rewrite,
		ModifyResponse: h.modifyResponse,
		ErrorHandler:   func(w http.ResponseWriter, req *http.Request, err error) { r.fail(w, req, h, err) },
	}
	return h
}

func (h *hop) rewrite(pr *httputil.ProxyRequest) {
	out := pr.Out.URL
	out.Scheme, out.Host = h.target.Scheme, h.target.Host
	out.Path = h.target.Path + strings.TrimPrefix(pr.In.URL.Path, h.public)
	out.RawPath = h.target.EscapedPath() + strings.TrimPrefix(pr.In.URL.EscapedPath(), h.public)
	pr.Out.Host = ""
	// Behind a TLS proxy only the proxy knows the browser's address and that it used https.
	pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
	pr.SetXForwarded()
	if proto := pr.In.Header.Get("X-Forwarded-Proto"); proto != "" {
		pr.Out.Header.Set("X-Forwarded-Proto", proto)
	}
	pr.Out.Header.Set("X-Forwarded-Prefix", h.public+"/")
	pr.Out.Header.Set("X-Boreas-Project", h.project)
	pr.Out.Header.Set("X-Boreas-Task", h.task)
	if h.path != "" {
		pr.Out.Header.Set(loopHeader, h.path)
	} else {
		// Any HTML's <base> needs rewriting, a service worker's own fetch of the app shell included, so it must arrive plain.
		pr.Out.Header.Set("Accept-Encoding", "identity")
	}
}

func (h *hop) modifyResponse(response *http.Response) error {
	h.rewriteLocation(response.Header)
	h.rewriteCookies(response.Header)
	if h.path != "" || !isHTML(response.Header.Get("Content-Type")) || response.Header.Get("Content-Encoding") != "" {
		return nil
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read HTML response: %w", err)
	}
	if err := response.Body.Close(); err != nil {
		return fmt.Errorf("close HTML response: %w", err)
	}
	// Only a <base> the app declared moves under the task: adding one would re-aim a page's own relative URLs.
	body = baseTagPattern.ReplaceAllFunc(body, h.base)
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", fmt.Sprint(len(body)))
	response.Header.Del("ETag")
	response.Header.Del("Content-MD5")
	return nil
}

// rewriteLocation maps the target's own redirects under the public path; other hosts and relative paths stay.
func (h *hop) rewriteLocation(header http.Header) {
	location, err := url.Parse(header.Get("Location"))
	if err != nil || location.Host != "" && !strings.EqualFold(location.Host, h.target.Host) {
		return
	}
	rest, ok := strings.CutPrefix(location.Path, h.target.Path)
	if !ok || !strings.HasPrefix(location.Path, "/") || under(location.Path, h.public) || rest != "" && rest[0] != '/' {
		return
	}
	location.Scheme, location.Host, location.Path, location.RawPath = "", "", h.public+rest, ""
	header.Set("Location", location.String())
}

// rewriteCookies scopes cookies to the task: tasks share one origin, so Path is all that keeps them apart.
func (h *hop) rewriteCookies(header http.Header) {
	for i, line := range header["Set-Cookie"] {
		cookie, err := http.ParseSetCookie(line)
		if err != nil {
			continue
		}
		// ponytail: a narrower Path widens to the whole task; map it under the task if an app needs that.
		cookie.Path, cookie.Domain = "/"+h.project+"/"+h.task+"/", ""
		if rewritten := cookie.String(); rewritten != "" {
			header["Set-Cookie"][i] = rewritten
		}
	}
}

//go:embed error.html
var errorHTML string

var errorPage = template.Must(template.New("error").Parse(errorHTML))

var reasons = map[string]string{
	"refused":     "The connection was refused.",
	"timeout":     "It didn’t answer in time.",
	"not_found":   "Its name couldn’t be found.",
	"certificate": "Its certificate isn’t trusted.",
	"unreachable": "It couldn’t be reached.",
}

// fail never shows the address: the page is public and the address is what routes hide.
func (r *Registry) fail(w http.ResponseWriter, request *http.Request, h *hop, err error) {
	if request.Header.Get("Sec-Fetch-Mode") != "navigate" {
		body := map[string]string{"error": "task_unreachable"}
		if h.path != "" {
			body = map[string]string{"error": "route_unreachable", "route": h.path}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(body)
		return
	}
	r.mu.RLock()
	running := h.path != "" && r.sites[routeKey(h.project, h.task)].task != nil
	r.mu.RUnlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	_ = errorPage.Execute(w, map[string]any{
		"Project": h.project, "Task": h.task, "Route": h.path, "Reason": reasons[kind(err)], "Running": running,
	})
}

func kind(err error) string {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "refused"
	}
	if e, ok := errors.AsType[net.Error](err); ok && e.Timeout() {
		return "timeout"
	}
	if e, ok := errors.AsType[*net.DNSError](err); ok && e.IsNotFound {
		return "not_found"
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return "certificate"
	}
	return "unreachable"
}

var (
	baseTagPattern  = regexp.MustCompile(`(?is)<base\b[^>]*>`)
	baseHrefPattern = regexp.MustCompile(`(?is)\bhref\s*=\s*["']([^"']*)["']`)
)

// base maps the app's <base> under the task as rewriteLocation maps its redirects: the path it is served at stays
// (/client/ becomes /team/task/client/); one already under the task stays as it is; any other the task root.
func (h *hop) base(tag []byte) []byte {
	href := "/"
	if m := baseHrefPattern.FindSubmatch(tag); m != nil && bytes.HasPrefix(m[1], []byte("/")) && !bytes.HasPrefix(m[1], []byte("//")) {
		href = string(m[1])
	}
	if !under(href, h.public) {
		href = h.public + href
	}
	return []byte(`<base href="` + href + `">`)
}

func isHTML(contentType string) bool {
	mediaType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	return strings.EqualFold(mediaType, "text/html")
}
