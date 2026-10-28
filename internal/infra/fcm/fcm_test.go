package fcm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

func TestMessagePriorityFollowsStatusAndBodyIsCapped(t *testing.T) {
	const want = `{"message":{"android":{"priority":"%s","ttl":"86400s"},` +
		`"apns":{"headers":{"apns-expiration":"86400","apns-priority":"%s"}},"data":{"type":"%s"},` +
		`"notification":{"body":"%s","title":"t"},"token":"tok","webpush":{"headers":{"TTL":"86400","Urgency":"%s"}}}}`
	for _, c := range []struct {
		n    core.Notification
		want string
	}{
		{
			core.Notification{Status: core.NotificationFailure, Title: "t", Body: strings.Repeat("é", 1500)},
			fmt.Sprintf(want, "HIGH", "10", "failure", strings.Repeat("é", 1000), "high"),
		},
		{
			core.Notification{Status: core.NotificationInfo, Title: "t", Body: "b"},
			fmt.Sprintf(want, "NORMAL", "5", "info", "b", "normal"),
		},
	} {
		if got, _ := json.Marshal(message("tok", c.n, time.Unix(0, 0))); string(got) != c.want {
			t.Fatalf("got  %s\nwant %s", got, c.want)
		}
	}
}

func TestSendPrunesOnlyForgottenTokens(t *testing.T) {
	replies := map[string]struct {
		code int
		body string
	}{
		"ok":           {200, `{}`},
		"unregistered": {404, `{"error":{"details":[{"errorCode":"UNREGISTERED"}]}}`},
		"mismatch":     {403, `{"error":{"details":[{"errorCode":"SENDER_ID_MISMATCH"}]}}`},
		"invalid":      {400, `{"error":{"details":[{"errorCode":"INVALID_ARGUMENT"},{"fieldViolations":[{"field":"message.token"}]}]}}`},
		"payload":      {400, `{"error":{"details":[{"errorCode":"INVALID_ARGUMENT"}]}}`},
		"busy":         {503, `{"error":{"details":[{"errorCode":"UNAVAILABLE"}]}}`},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Message struct{ Token string } }
		_ = json.NewDecoder(r.Body).Decode(&req)
		reply, ok := replies[req.Message.Token]
		if !ok { // a device that never answers must not hold up the rest
			<-r.Context().Done()
			return
		}
		w.WriteHeader(reply.code)
		_, _ = w.Write([]byte(reply.body))
	}))
	defer server.Close()

	tokens := append(slices.Collect(maps.Keys(replies)), "hang")
	var removed []string
	s := &Sender{
		url: server.URL, client: server.Client(), logger: slog.New(slog.DiscardHandler),
		tokens: func(context.Context, core.Notification) ([]string, error) { return tokens, nil },
		remove: func(_ context.Context, gone []string) error { removed = gone; return nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Send(ctx, core.Notification{}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(removed)
	if !slices.Equal(removed, []string{"invalid", "mismatch", "unregistered"}) {
		t.Fatalf("removed %v", removed)
	}
}
