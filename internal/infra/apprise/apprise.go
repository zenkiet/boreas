package apprise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

// Sender is a no-op when URL is empty, so external delivery stays optional.
type Sender struct {
	url    string
	client *http.Client
	// Targets, when set, restricts delivery to the devices allowed to see this
	// notification; Apprise's configured destinations are then not used.
	Targets func(context.Context, core.Notification) []string
}

func New(url string, timeout time.Duration) *Sender {
	return &Sender{url: url, client: &http.Client{Timeout: timeout}}
}

func FCMURL(project, keyfile, token string) string {
	return fmt.Sprintf("fcm://%s/%s/?keyfile=%s&priority=high",
		url.PathEscape(project), url.PathEscape(token), url.QueryEscape(keyfile))
}

func (s *Sender) Send(ctx context.Context, n core.Notification) error {
	if s.url == "" {
		return nil
	}
	body := map[string]any{
		"title": n.Title,
		"body":  n.Body,
		"type":  string(n.Status),
	}
	if s.Targets != nil {
		urls := s.Targets(ctx, n)
		if len(urls) == 0 {
			return nil
		}
		body["urls"] = urls
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("post notification: %s", resp.Status)
	}
	return nil
}
