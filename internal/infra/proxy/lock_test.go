package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zenkiet/boreas/internal/core"
	"golang.org/x/crypto/bcrypt"
)

func TestRegistryLock(t *testing.T) {
	var seen string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Cookie")
		io.WriteString(w, "app "+r.URL.RequestURI())
	}))
	defer app.Close()
	registry := New(0, 0)
	defer registry.CloseIdleConnections()
	host, port := serverAddress(t, app.URL)
	registry.Register(context.Background(), "p", "t", host, port)
	registry.SetRoutes("p", "t", []core.Route{{Path: "/api", URL: app.URL}})
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	registry.SetLock("p", "t", string(hash))

	get := func(path, cookie string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Cookie", cookie)
		return serve(registry, request)
	}
	unlock := func(password string) *httptest.ResponseRecorder {
		form := strings.NewReader(url.Values{"boreas_password": {password}}.Encode())
		request := httptest.NewRequest(http.MethodPost, "/p/t/orders?id=7", form)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("X-Forwarded-Proto", "https")
		return serve(registry, request)
	}
	for _, path := range []string{"/p/t/", "/p/t/api/x"} {
		if code := get(path, "").Code; code != http.StatusUnauthorized {
			t.Fatalf("%s without the cookie: %d, want 401", path, code)
		}
	}
	if code := get("/p/t", "").Code; code != http.StatusPermanentRedirect {
		t.Fatalf("/p/t must redirect before the lock, or the cookie's path never matches: %d", code)
	}
	if rr := unlock("wrong horse"); rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "Wrong password") {
		t.Fatalf("wrong password: %d %s", rr.Code, rr.Body)
	}
	rr := unlock("correct horse")
	cookies := rr.Result().Cookies()
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/p/t/orders?id=7" || len(cookies) != 1 ||
		cookies[0].Path != "/p/t/" || !cookies[0].HttpOnly || !cookies[0].Secure ||
		cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge != 86400 {
		t.Fatalf("unlock: %d to %q with %+v", rr.Code, rr.Header().Get("Location"), cookies)
	}
	jar := "sid=1; " + cookies[0].Name + "=" + cookies[0].Value + "; theme=dark"
	for _, path := range []string{"/p/t/orders", "/p/t/api/x"} {
		if rr := get(path, jar); rr.Code != http.StatusOK || seen != "sid=1; theme=dark" {
			t.Fatalf("%s with the cookie: %d, upstream saw cookies %q", path, rr.Code, seen)
		}
	}

	registry.Unregister(context.Background(), "p", "t")
	registry.SetRoutes("p", "t", nil)
	registry.Register(context.Background(), "p", "t", host, port)
	if code := get("/p/t/", "").Code; code != http.StatusUnauthorized {
		t.Fatalf("a stopped task came back open: %d", code)
	}
	l := &lock{hash: hash}
	if l.valid(l.sign(time.Now().Add(-time.Second)), time.Now()) ||
		l.valid("9999999999."+strings.Split(l.sign(time.Now().Add(time.Hour)), ".")[1], time.Now()) {
		t.Fatal("an expired or re-dated cookie must not open the task")
	}

	hash, _ = bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	registry.SetLock("p", "t", string(hash))
	if code := get("/p/t/orders", jar).Code; code != http.StatusUnauthorized {
		t.Fatalf("the old password's cookie still opens the task: %d", code)
	}
	for range 10 {
		unlock("wrong horse")
	}
	if code := unlock("correct horse").Code; code != http.StatusTooManyRequests {
		t.Fatalf("11th try in a row: %d, want 429", code)
	}
	registry.SetLock("p", "t", "")
	if code := get("/p/t/orders", "").Code; code != http.StatusOK {
		t.Fatalf("an opened task still asks: %d", code)
	}
}
