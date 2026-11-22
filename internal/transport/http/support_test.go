package httptransport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLibredeskHookReportsNewChats(t *testing.T) {
	var reported []string
	projects := &stubProjects{support: func(slug, detail string) error {
		reported = append(reported, slug+"|"+detail)
		return nil
	}}
	send := func(hooks Hooks, body, secret string) int {
		h := APIHandler(stubTasks{}, &stubAuth{}, projects, &stubPush{}, NewHub(), hooks, "test", slog.New(slog.DiscardHandler))
		r := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/libredesk", strings.NewReader(body))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		r.Header.Set("X-Libredesk-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		return do(h, r).Code
	}
	on := Hooks{LibredeskSecret: "s3cret"}
	chat := `{"event":"conversation.created","payload":{"inbox_name":"shop","last_message":"<!channel> help","contact":{"first_name":"Lan"}}}`

	for _, c := range []struct {
		name         string
		hooks        Hooks
		body, secret string
		want         int
	}{
		{"bad signature", on, chat, "wrong", http.StatusNotFound},
		{"no secret set", Hooks{}, chat, "", http.StatusNotFound},
		{"other event", on, `{"event":"message.created","payload":{}}`, "s3cret", http.StatusNoContent},
		{"malformed event", on, `{"event":`, "s3cret", http.StatusBadRequest},
	} {
		if code := send(c.hooks, c.body, c.secret); code != c.want {
			t.Errorf("%s: %d, want %d", c.name, code, c.want)
		}
	}
	if len(reported) != 0 {
		t.Fatalf("reported %v before a signed new chat", reported)
	}
	if code := send(on, chat, "s3cret"); code != http.StatusNoContent || len(reported) != 1 || reported[0] != "shop|Lan: ‹!channel› help" {
		t.Fatalf("new chat: %d, reported %v", code, reported)
	}
}
