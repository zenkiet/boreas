package apprise

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

var failure = core.Notification{
	TaskName: "web", Status: core.NotificationFailure,
	Title: "Deploy failed: demo/web", Body: "registry unavailable",
}

func TestSendPostsApprisePayload(t *testing.T) {
	var got map[string]string
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("payload is not JSON: %v", err)
		}
	}))
	defer server.Close()

	if err := New(server.URL, time.Second).Send(context.Background(), failure); err != nil {
		t.Fatal(err)
	}
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	// Apprise reads exactly these keys, and "type" must carry the status verbatim.
	if got["title"] != failure.Title || got["body"] != failure.Body || got["type"] != "failure" {
		t.Fatalf("payload = %v", got)
	}
}

func TestSendWithoutURLIsANoOp(t *testing.T) {
	if err := New("", time.Second).Send(context.Background(), failure); err != nil {
		t.Fatalf("an unconfigured sender must not fail: %v", err)
	}
}

func TestSendReportsRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	if err := New(server.URL, time.Second).Send(context.Background(), failure); err == nil {
		t.Fatal("a rejected notification must be reported")
	}
}

func TestSendAddsSubscribedTargets(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("payload is not JSON: %v", err)
		}
	}))
	defer server.Close()

	sender := New(server.URL, time.Second)
	sender.Targets = func(context.Context, core.Notification) []string {
		return []string{"fcm://p/a/", "fcm://p/b/"}
	}
	if err := sender.Send(context.Background(), failure); err != nil {
		t.Fatal(err)
	}
	// Apprise's stateless endpoint accepts urls as a list, so it must not be flattened.
	urls, ok := got["urls"].([]any)
	if !ok || len(urls) != 2 || urls[0] != "fcm://p/a/" || urls[1] != "fcm://p/b/" {
		t.Fatalf("payload = %v", got)
	}
	if got["title"] != failure.Title || got["type"] != "failure" {
		t.Fatalf("payload = %v", got)
	}
}

func TestSendWithNoSubscribersSkipsRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()

	sender := New(server.URL, time.Second)
	sender.Targets = func(context.Context, core.Notification) []string { return nil }
	if err := sender.Send(context.Background(), failure); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("a notification with no subscribers must not be posted")
	}
}

func TestFCMURL(t *testing.T) {
	got := FCMURL("my-project", "/config/key.json", "cH9x:APA91bH-x_9Kd")
	want := "fcm://my-project/cH9x:APA91bH-x_9Kd/?keyfile=%2Fconfig%2Fkey.json&priority=high"
	if got != want {
		t.Fatalf("FCMURL() = %q, want %q", got, want)
	}
}
