// Package sourcebot reads code indexed by a self-hosted Sourcebot through its free REST API.
package sourcebot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

const (
	responseLimit = 8 << 20
	reposPage     = 100
	// Sourcebot spends its match budget file by file, so ask for many and keep a few per file for breadth.
	searchMatches = 200
	searchFiles   = 25
	fileChunks    = 2
)

// ponytail: a secret hardcoded in ordinary source still reaches the client; extend this list as such files turn up.
var blockedNames = []string{
	"*.config", "appsettings*.json", ".env*", "*.pem", "*.key", "*.pfx", "*.p12", "*.jks", "*.keystore",
	"*secret*", "id_rsa*", ".git-credentials", ".npmrc", "*.tfvars", "*.pubxml*", "*.publishsettings", ".vs",
	"*.env", "*.tfstate*", "credentials", "credentials.json", "credentials.y*ml*", ".netrc", ".pgpass", "kubeconfig*",
	"*.ppk", ".dockerconfigjson", ".aws", ".ssh", ".docker",
}

type Client struct {
	url, key string
	http     *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{url: strings.TrimSuffix(baseURL, "/"), key: apiKey, http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Repos(ctx context.Context) ([]string, error) {
	var names []string
	for page := 1; ; page++ {
		var repos []struct {
			RepoName string `json:"repoName"`
		}
		query := url.Values{"page": {strconv.Itoa(page)}, "perPage": {strconv.Itoa(reposPage)}}
		if err := c.call(ctx, http.MethodGet, "/api/repos", query, nil, &repos); err != nil {
			return nil, err
		}
		for _, repo := range repos {
			names = append(names, repo.RepoName)
		}
		if len(repos) < reposPage {
			return names, nil
		}
	}
}

func (c *Client) Search(ctx context.Context, repos []string, query string) ([]core.CodeMatch, error) {
	filters := make([]string, len(repos))
	for i, repo := range repos {
		filters[i] = "repo:^" + regexp.QuoteMeta(repo) + "$"
	}
	body := map[string]any{"query": "(" + strings.Join(filters, " or ") + ") (" + query + ")", "matches": searchMatches, "contextLines": 2}
	var resp struct {
		Files []struct {
			FileName struct {
				Text string `json:"text"`
			} `json:"fileName"`
			Repository string `json:"repository"`
			Chunks     []struct {
				Content string `json:"content"`
				Start   struct {
					Line int `json:"lineNumber"`
				} `json:"contentStart"`
			} `json:"chunks"`
		} `json:"files"`
	}
	if err := c.call(ctx, http.MethodPost, "/api/search", nil, body, &resp); err != nil {
		return nil, err
	}
	var matches []core.CodeMatch
	files := 0
	for _, file := range resp.Files {
		// The query's repo filter is text a crafted query can break out of; this check is the real boundary.
		if _, ok := allowedPath(file.FileName.Text); !ok || !slices.Contains(repos, file.Repository) {
			continue
		}
		if files++; files > searchFiles {
			break
		}
		for _, chunk := range file.Chunks[:min(len(file.Chunks), fileChunks)] {
			matches = append(matches, core.CodeMatch{Repo: file.Repository, Path: file.FileName.Text, Line: chunk.Start.Line, Snippet: chunk.Content})
		}
	}
	return matches, nil
}

func (c *Client) Read(ctx context.Context, repos []string, repo, file string) (core.CodeFile, error) {
	cleaned, ok := allowedPath(file)
	if !ok || !slices.Contains(repos, repo) {
		return core.CodeFile{}, core.ErrNotFound
	}
	var resp struct {
		Source string `json:"source"`
		URL    string `json:"externalWebUrl"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/source", url.Values{"repo": {repo}, "path": {cleaned}}, nil, &resp); err != nil {
		return core.CodeFile{}, err
	}
	return core.CodeFile{Repo: repo, Path: cleaned, Content: resp.Source, URL: resp.URL}, nil
}

func allowedPath(file string) (string, bool) {
	if file == "" || strings.ContainsAny(file, "\\\x00") {
		return "", false
	}
	cleaned := path.Clean(strings.TrimPrefix(file, "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	for _, segment := range strings.Split(strings.ToLower(cleaned), "/") {
		for _, pattern := range blockedNames {
			if blocked, _ := path.Match(pattern, segment); blocked {
				return "", false
			}
		}
	}
	return cleaned, true
}

func (c *Client) call(ctx context.Context, method, endpoint string, query url.Values, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode sourcebot request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	target := c.url + endpoint
	if query != nil {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return fmt.Errorf("build sourcebot request: %w", err)
	}
	req.Header.Set("X-Sourcebot-Api-Key", c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call sourcebot %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	// Only a missing file is a not-found; a 404 elsewhere means a wrong BOREAS_SOURCEBOT_URL.
	if resp.StatusCode == http.StatusNotFound && endpoint == "/api/source" {
		return core.ErrNotFound
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("call sourcebot %s: %s", endpoint, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, responseLimit)).Decode(out); err != nil {
		return fmt.Errorf("decode sourcebot %s: %w", endpoint, err)
	}
	return nil
}
