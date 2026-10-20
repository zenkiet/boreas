// Package fcm sends push straight to FCM HTTP v1, so tokens FCM has forgotten can be pruned.
package fcm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
)

const (
	ttl     = 86400 // seconds; the Activity feed keeps the record, so a day-old push is noise
	maxBody = 1000  // runes; FCM rejects a message over 4 KiB and failed-deploy bodies run long
)

// Sender is a no-op without a project, so push stays optional.
type Sender struct {
	url    string
	client *http.Client
	tokens func(context.Context, uuid.UUID, string) ([]string, error)
	remove func(context.Context, []string) error
	logger *slog.Logger
}

// New reads the keyfile once so a broken one stops startup instead of failing every send.
func New(project, keyfile string, tokens func(context.Context, uuid.UUID, string) ([]string, error),
	remove func(context.Context, []string) error, logger *slog.Logger,
) (*Sender, error) {
	if project == "" {
		return &Sender{}, nil
	}
	key, err := os.ReadFile(keyfile)
	if err != nil {
		return nil, fmt.Errorf("read FCM keyfile: %w", err)
	}
	creds, err := google.CredentialsFromJSONWithType(context.Background(), key, google.ServiceAccount,
		"https://www.googleapis.com/auth/firebase.messaging")
	if err != nil {
		return nil, fmt.Errorf("parse FCM keyfile: %w", err)
	}
	return &Sender{
		url:    "https://fcm.googleapis.com/v1/projects/" + project + "/messages:send",
		client: oauth2.NewClient(context.Background(), creds.TokenSource),
		tokens: tokens, remove: remove, logger: logger,
	}, nil
}

// Send is best effort with no retry: the Activity feed already holds the record.
func (s *Sender) Send(ctx context.Context, n core.Notification) error {
	if s.url == "" {
		return nil
	}
	tokens, err := s.tokens(ctx, n.ProjectID, n.TaskName)
	if err != nil {
		return err
	}
	var (
		mu     sync.Mutex
		gone   []string
		failed int
		g      errgroup.Group
	)
	g.SetLimit(8)
	for _, token := range tokens {
		g.Go(func() error {
			forgotten, err := s.post(ctx, token, n)
			mu.Lock()
			defer mu.Unlock()
			if forgotten {
				gone = append(gone, token)
			} else if err != nil {
				failed++
				s.logger.Warn("push failed", "token", token[:min(8, len(token))], "error", err)
			}
			return nil
		})
	}
	_ = g.Wait()
	if len(gone) > 0 {
		// ponytail: a send that used up the whole timeout leaves its dead tokens to the next send.
		if err := s.remove(ctx, gone); err != nil {
			return err
		}
	}
	s.logger.Info("push", "sent", len(tokens)-len(gone)-failed, "removed", len(gone), "failed", failed)
	return nil
}

// post reports whether FCM has forgotten the token.
func (s *Sender) post(ctx context.Context, token string, n core.Notification) (bool, error) {
	payload, err := json.Marshal(message(token, n, time.Now()))
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return false, nil
	}
	var st status
	_ = json.NewDecoder(resp.Body).Decode(&st)
	if st.forgotten() {
		return true, nil
	}
	return false, fmt.Errorf("%s: %s", resp.Status, st.Error.Message)
}

func message(token string, n core.Notification, now time.Time) map[string]any {
	android, apns, web := "NORMAL", "5", "normal"
	if n.Status == core.NotificationFailure {
		android, apns, web = "HIGH", "10", "high"
	}
	body := []rune(n.Body)
	return map[string]any{"message": map[string]any{
		"token":        token,
		"notification": map[string]string{"title": n.Title, "body": string(body[:min(len(body), maxBody)])},
		"data":         map[string]string{"type": string(n.Status)},
		"android":      map[string]string{"priority": android, "ttl": strconv.Itoa(ttl) + "s"},
		"apns": map[string]any{"headers": map[string]string{
			"apns-priority": apns, "apns-expiration": strconv.FormatInt(now.Unix()+ttl, 10),
		}},
		"webpush": map[string]any{"headers": map[string]string{"TTL": strconv.Itoa(ttl), "Urgency": web}},
	}}
}

type status struct {
	Error struct {
		Message string `json:"message"`
		Details []struct {
			ErrorCode       string `json:"errorCode"`
			FieldViolations []struct {
				Field string `json:"field"`
			} `json:"fieldViolations"`
		} `json:"details"`
	} `json:"error"`
}

// forgotten keeps a token on any other error: a bad payload, quota or outage is not the device's fault.
func (st status) forgotten() bool {
	for _, d := range st.Error.Details {
		if d.ErrorCode == "UNREGISTERED" || d.ErrorCode == "SENDER_ID_MISMATCH" {
			return true
		}
		for _, v := range d.FieldViolations {
			if v.Field == "message.token" {
				return true
			}
		}
	}
	return false
}
