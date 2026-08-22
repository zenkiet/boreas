package apprise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

// Sender is a no-op when URL is empty, so external delivery stays optional.
type Sender struct {
	url    string
	client *http.Client
}

func New(url string, timeout time.Duration) *Sender {
	return &Sender{url: url, client: &http.Client{Timeout: timeout}}
}

func (s *Sender) Send(ctx context.Context, n core.Notification) error {
	if s.url == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]string{
		"title": n.Title,
		"body":  n.Body,
		"type":  string(n.Status),
	})
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
