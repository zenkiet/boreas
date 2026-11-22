package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

const testDeviceToken = "cH9x2Qk7RtqB:APA91bH-x9Kd2Qw_ErTyUiOp"

func pushHandler(push PushStore) http.Handler {
	return APIHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{}, push, NewHub(), Hooks{}, "test", slog.New(slog.DiscardHandler))
}

// Both directions pass the caller's own ID, which is what scopes the store to their devices.
func TestPushSubscriptionsActAsTheCaller(t *testing.T) {
	var calls []string
	h := pushHandler(&stubPush{
		create: func(_ context.Context, userID uuid.UUID, token string, events core.PushEvents) error {
			calls = append(calls, "create "+userID.String()+" "+token+" "+string(events))
			return nil
		},
		delete: func(_ context.Context, userID uuid.UUID, token string) error {
			calls = append(calls, "delete "+userID.String()+" "+token)
			return nil
		},
	})
	sub := do(h, authed(http.MethodPost, "/api/v1/push/subscriptions", strings.NewReader(`{"token":"`+testDeviceToken+`"}`)))
	unsub := do(h, authed(http.MethodDelete, "/api/v1/push/subscriptions/"+testDeviceToken, nil))
	caller := " " + testMember.ID.String() + " " + testDeviceToken
	if sub.Code != http.StatusCreated || unsub.Code != http.StatusOK ||
		strings.Join(calls, ",") != "create"+caller+" important,delete"+caller {
		t.Fatalf("statuses %d/%d, store calls %v", sub.Code, unsub.Code, calls)
	}
}

func TestSubscribePushTakesItsEvents(t *testing.T) {
	var stored []core.PushEvents
	h := pushHandler(&stubPush{create: func(_ context.Context, _ uuid.UUID, _ string, events core.PushEvents) error {
		stored = append(stored, events)
		return nil
	}})
	for body, want := range map[string]int{`"all"`: http.StatusCreated, `"important"`: http.StatusCreated, `"failures"`: http.StatusBadRequest} {
		rr := do(h, authed(http.MethodPost, "/api/v1/push/subscriptions",
			strings.NewReader(`{"token":"`+testDeviceToken+`","events":`+body+`}`)))
		if rr.Code != want {
			t.Fatalf("events %s: status %d, want %d", body, rr.Code, want)
		}
	}
	if !slices.Contains(stored, core.PushAll) || len(stored) != 2 {
		t.Fatalf("stored %v", stored)
	}
}

// No FCM token holds a separator or whitespace, so one that does must be rejected
// before it reaches the store.
func TestSubscribePushRejectsMalformedTokenWithoutStoring(t *testing.T) {
	called := false
	push := &stubPush{create: func(context.Context, uuid.UUID, string, core.PushEvents) error {
		called = true
		return nil
	}}
	h := pushHandler(push)
	for _, token := range []string{"", "abc,def", "abc def"} {
		rr := do(h, authed(http.MethodPost, "/api/v1/push/subscriptions",
			strings.NewReader(`{"token":"`+token+`"}`)))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("token=%q status=%d body=%s", token, rr.Code, rr.Body.String())
		}
	}
	if called {
		t.Fatal("an invalid token must not reach the store")
	}
}

func TestDeviceLabel(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148":                                "iPhone",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)":                                                     "iPad",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8 Build/AP2A; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/126.0 Mobile Safari/537.36": "Android",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15":                        "Safari on Mac",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0":                "Edge on Windows",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/131.0 Mobile/15E148 Safari/604.1":       "Chrome on iPhone",
		"Mozilla/5.0 (Linux; Android 14; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36":                              "Chrome on Android",
		"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0":                                                                       "Firefox on Linux",
		"curl/8.7.1": "",
	} {
		if got := deviceLabel(ua); got != want {
			t.Errorf("deviceLabel(%q) = %q, want %q", ua, got, want)
		}
	}
}
