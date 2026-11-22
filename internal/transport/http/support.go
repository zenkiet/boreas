package httptransport

import (
	"cmp"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/zenkiet/boreas/internal/core"
)

type Hooks struct {
	LibredeskSecret string
}

type libredeskEvent struct {
	Event   string `json:"event" example:"conversation.created"`
	Payload struct {
		InboxName   string `json:"inbox_name,omitempty" example:"shop"`
		LastMessage string `json:"last_message,omitempty" example:"The checkout page is blank"`
		Contact     struct {
			FirstName string `json:"first_name,omitempty"`
			LastName  string `json:"last_name,omitempty"`
			Email     string `json:"email,omitempty"`
		} `json:"contact"`
	} `json:"payload"`
}

var untrusted = strings.NewReplacer("<", "‹", ">", "›")

func (h *Handler) libredeskHook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil || !h.signedByLibredesk(body, r.Header.Get("X-Libredesk-Signature")) {
		writeServiceError(w, h.logger, core.ErrNotFound)
		return
	}
	var event libredeskEvent
	if err := json.Unmarshal(body, &event); err != nil {
		writeBadRequest(w)
		return
	}
	if event.Event == "conversation.created" {
		chat := event.Payload
		who := cmp.Or(strings.TrimSpace(chat.Contact.FirstName+" "+chat.Contact.LastName), chat.Contact.Email, "Visitor")
		if err := h.projects.SupportRequested(r.Context(), chat.InboxName, untrusted.Replace(who+": "+chat.LastMessage)); err != nil {
			h.logger.Warn("libredesk inbox matches no project", "inbox", chat.InboxName, "error", err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) signedByLibredesk(body []byte, signature string) bool {
	if h.hooks.LibredeskSecret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.hooks.LibredeskSecret))
	mac.Write(body)
	return hmac.Equal([]byte(signature), []byte("sha256="+hex.EncodeToString(mac.Sum(nil))))
}
