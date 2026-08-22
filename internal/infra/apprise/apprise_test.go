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
