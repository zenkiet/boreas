package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const testDeviceToken = "cH9x2Qk7RtqB:APA91bH-x9Kd2Qw_ErTyUiOp"

func pushHandler(push PushStore) http.Handler {
	return APIHandler(stubTasks{}, &stubAuth{user: testMember}, &stubProjects{}, push, "test", slog.New(slog.DiscardHandler))
}

// Both directions pass the caller's own ID, which is what scopes the store to their devices.
func TestPushSubscriptionsActAsTheCaller(t *testing.T) {
	var calls []string
	record := func(op string) func(context.Context, uuid.UUID, string) error {
		return func(_ context.Context, userID uuid.UUID, token string) error {
			calls = append(calls, op+" "+userID.String()+" "+token)
			return nil
		}
	}
	h := pushHandler(&stubPush{create: record("create"), delete: record("delete")})
	sub := do(h, authed(http.MethodPost, "/api/v1/push/subscriptions", strings.NewReader(`{"token":"`+testDeviceToken+`"}`)))
	unsub := do(h, authed(http.MethodDelete, "/api/v1/push/subscriptions/"+testDeviceToken, nil))
	caller := " " + testMember.ID.String() + " " + testDeviceToken
	if sub.Code != http.StatusCreated || unsub.Code != http.StatusOK || strings.Join(calls, ",") != "create"+caller+",delete"+caller {
		t.Fatalf("statuses %d/%d, store calls %v", sub.Code, unsub.Code, calls)
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
