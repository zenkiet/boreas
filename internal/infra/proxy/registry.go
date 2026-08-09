// Package proxy provides the task route registry and reverse proxy handler.
package proxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

type route struct {
	project  string
	task     string
	prefix   string
	basePath string
	target   *url.URL
	proxy    *httputil.ReverseProxy
}

// Registry is concurrency-safe and keys routes by project because task names are only locally unique.
type Registry struct {
	mu        sync.RWMutex
	routes    map[string]*route
	transport *http.Transport
}

func routeKey(project, task string) string { return project + "/" + task }

func New(dialTimeout, responseHeaderTimeout time.Duration) *Registry {
	dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
	return &Registry{
		routes: make(map[string]*route),
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, DialContext: dialer.DialContext,
			ForceAttemptHTTP2: true, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second,
			ResponseHeaderTimeout: responseHeaderTimeout,
		},
	}
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
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, fmt.Sprint(port))}
	entry := &route{
		project: project, task: task,
		prefix:   "/" + project + "/" + task,
		basePath: "/" + project + "/" + task + "/",
		target:   target,
	}
	entry.proxy = r.newReverseProxy(entry)
	r.mu.Lock()
	r.routes[routeKey(project, task)] = entry
	r.mu.Unlock()
	return nil
}

func (r *Registry) Unregister(_ context.Context, project, task string) error {
	key := routeKey(project, task)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.routes[key]; !exists {
		return core.ErrNotFound
	}
	delete(r.routes, key)
	return nil
}

func (r *Registry) CloseIdleConnections() { r.transport.CloseIdleConnections() }

func (r *Registry) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	trimmed := strings.TrimPrefix(request.URL.Path, "/")
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, request)
		return
	}
	r.mu.RLock()
	entry := r.routes[routeKey(parts[0], parts[1])]
	r.mu.RUnlock()
	if entry == nil {
		http.NotFound(w, request)
		return
	}
	// Redirect to the base path so relative assets do not resolve under "/project/".
	if len(parts) == 2 {
		location := entry.basePath
		if request.URL.RawQuery != "" {
			location += "?" + request.URL.RawQuery
		}
		http.Redirect(w, request, location, http.StatusPermanentRedirect)
		return
	}
	entry.proxy.ServeHTTP(w, request)
}

func (r *Registry) newReverseProxy(entry *route) *httputil.ReverseProxy {
	proxy := &httputil.ReverseProxy{Transport: r.transport}
	proxy.Director = func(request *http.Request) {
		originalHost := request.Host
		originalProto := "http"
		if request.TLS != nil {
			originalProto = "https"
		}
		request.URL.Scheme = entry.target.Scheme
		request.URL.Host = entry.target.Host
		request.URL.Path = strings.TrimPrefix(request.URL.Path, entry.prefix)
		if request.URL.Path == "" {
			request.URL.Path = "/"
		}
		request.URL.RawPath = ""
		request.Host = entry.target.Host
		request.Header.Set("Accept-Encoding", "identity")
		request.Header.Set("X-Forwarded-Host", originalHost)
		request.Header.Set("X-Forwarded-Proto", originalProto)
		request.Header.Set("X-Forwarded-Prefix", entry.basePath)
		request.Header.Set("X-Boreas-Project", entry.project)
		request.Header.Set("X-Boreas-Task", entry.task)
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		rewriteLocation(response, entry)
		if !isHTML(response.Header.Get("Content-Type")) || response.Header.Get("Content-Encoding") != "" {
			return nil
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("read HTML response: %w", err)
		}
		if err := response.Body.Close(); err != nil {
			return fmt.Errorf("close HTML response: %w", err)
		}
		body = injectBase(body, entry.basePath)
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		response.Header.Set("Content-Length", fmt.Sprint(len(body)))
		response.Header.Del("ETag")
		response.Header.Del("Content-MD5")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}
	return proxy
}

func rewriteLocation(response *http.Response, entry *route) {
	value := response.Header.Get("Location")
	if value == "" {
		return
	}
	location, err := url.Parse(value)
	if err != nil {
		return
	}
	if location.IsAbs() {
		if !strings.EqualFold(location.Host, entry.target.Host) {
			return
		}
		location.Scheme, location.Host = "", ""
	}
	if strings.HasPrefix(location.Path, entry.basePath) {
		response.Header.Set("Location", location.String())
		return
	}
	location.Path = entry.basePath + strings.TrimPrefix(location.Path, "/")
	if location.Path == entry.basePath && value != "/" && value != "" {
		location.Path = entry.basePath + strings.TrimPrefix(value, "./")
	}
	response.Header.Set("Location", location.String())
}

var (
	baseTagPattern = regexp.MustCompile(`(?is)<base\b[^>]*>`)
	headPattern    = regexp.MustCompile(`(?is)<head\b[^>]*>`)
	htmlPattern    = regexp.MustCompile(`(?is)<html\b[^>]*>`)
)

func injectBase(document []byte, basePath string) []byte {
	document = baseTagPattern.ReplaceAll(document, nil)
	location := headPattern.FindIndex(document)
	tag := []byte(`<base href="` + basePath + `">`)
	if location == nil {
		head := append(append([]byte(`<head>`), tag...), []byte(`</head>`)...)
		if htmlLocation := htmlPattern.FindIndex(document); htmlLocation != nil {
			result := make([]byte, 0, len(document)+len(head))
			result = append(result, document[:htmlLocation[1]]...)
			result = append(result, head...)
			result = append(result, document[htmlLocation[1]:]...)
			return result
		}
		return append(head, document...)
	}
	result := make([]byte, 0, len(document)+len(tag))
	result = append(result, document[:location[1]]...)
	result = append(result, tag...)
	result = append(result, document[location[1]:]...)
	return result
}

func isHTML(contentType string) bool {
	mediaType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	return strings.EqualFold(mediaType, "text/html")
}
