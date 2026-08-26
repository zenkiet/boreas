package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

const testDeviceToken = "cH9x2Qk7RtqB:APA91bH-x9Kd2Qw_ErTyUiOp"

func pushHandler(push PushStore) http.Handler {
	return APIHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{}, push, slog.New(slog.DiscardHandler))
}

func TestSubscribePushRecordsTokenForCurrentUser(t *testing.T) {
	var gotUser uuid.UUID
	var gotToken string
	push := &stubPush{create: func(_ context.Context, userID uuid.UUID, token string) error {
		gotUser, gotToken = userID, token
		return nil
	}}
	rr := do(pushHandler(push), authed(http.MethodPost, "/api/v1/push/subscriptions",
		strings.NewReader(`{"token":"`+testDeviceToken+`"}`)))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotUser != testMember.ID || gotToken != testDeviceToken {
		t.Fatalf("user=%s token=%q", gotUser, gotToken)
	}
}

// A comma would split one target into two inside the Apprise URL, so it must be
// rejected before it reaches the store.
func TestSubscribePushRejectsMalformedTokenWithoutStoring(t *testing.T) {
	called := false
	push := &stubPush{create: func(context.Context, uuid.UUID, string) error {
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

func TestUnsubscribePushScopesToCurrentUser(t *testing.T) {
	var gotUser uuid.UUID
	var gotToken string
	push := &stubPush{delete: func(_ context.Context, userID uuid.UUID, token string) error {
		gotUser, gotToken = userID, token
		return nil
	}}
	rr := do(pushHandler(push), authed(http.MethodDelete, "/api/v1/push/subscriptions/"+testDeviceToken, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotUser != testMember.ID || gotToken != testDeviceToken {
		t.Fatalf("user=%s token=%q", gotUser, gotToken)
	}
}

func TestUnsubscribePushReportsAnotherUsersToken(t *testing.T) {
	push := &stubPush{delete: func(context.Context, uuid.UUID, string) error {
		return core.ErrNotFound
	}}
	rr := do(pushHandler(push), authed(http.MethodDelete, "/api/v1/push/subscriptions/"+testDeviceToken, nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPushSubscriptionsRequireAuthentication(t *testing.T) {
	h := pushHandler(&stubPush{})
	for _, r := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/push/subscriptions", strings.NewReader(`{"token":"x"}`)),
		httptest.NewRequest(http.MethodDelete, "/api/v1/push/subscriptions/"+testDeviceToken, nil),
	} {
		if rr := do(h, r); rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", r.Method, rr.Code)
		}
	}
}
