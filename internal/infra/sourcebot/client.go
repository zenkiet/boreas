// Package sourcebot reads code indexed by a self-hosted Sourcebot through its free REST API.
package sourcebot

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

const (
	responseLimit = 8 << 20
	// Sourcebot spends its match budget file by file, so ask for many and keep a few per file for breadth.
	searchMatches = 200
	searchFiles   = 25
	fileChunks    = 2
	commitsPage   = 30
	// Contexts change by hand in Sourcebot's config, so what it said about one holds this long.
	contextTTL = 5 * time.Minute
	// The project list waits on Has, so a slow Sourcebot holds it at most this long.
	probeTimeout = 3 * time.Second
)

// ponytail: a secret hardcoded in ordinary source still reaches the client; extend this list as such files turn up.
var blockedNames = []string{
	"*.config", "appsettings*.json", ".env*", "*.pem", "*.key", "*.pfx", "*.p12", "*.jks", "*.keystore",
	"*secret*", "id_rsa*", ".git-credentials", ".npmrc", "*.tfvars", "*.pubxml*", "*.publishsettings", ".vs",
	"*.env", "*.tfstate*", "credentials", "credentials.json", "credentials.y*ml*", ".netrc", ".pgpass", "kubeconfig*",
	"*.ppk", ".dockerconfigjson", ".aws", ".ssh", ".docker",
}

// gitRef is a branch, tag, commit or a range of them; it never starts like a git option.
var gitRef = regexp.MustCompile(`^\w[\w./^~@{}-]{0,199}$`)

type Client struct {
	url, key string
	http     *http.Client
	mu       sync.Mutex
	seen     map[string]answer // whether a project has a context, or a context a repository
}

type answer struct {
	yes bool
	at  time.Time
}

type treeNode struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Children []treeNode `json:"children"`
}

func New(baseURL, apiKey string) *Client {
	return &Client{url: strings.TrimSuffix(baseURL, "/"), key: apiKey, http: &http.Client{Timeout: 20 * time.Second}, seen: map[string]answer{}}
}

// Has reports which projects have a search context named after them; one Sourcebot cannot answer about in time has none.
func (c *Client) Has(ctx context.Context, projects []string) []bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	has := make([]bool, len(projects))
	var wg sync.WaitGroup
	for i, project := range projects {
		wg.Go(func() { has[i], _ = c.matches(ctx, project, "context:"+project) })
	}
	wg.Wait()
	return has
}

// matches asks whether query finds anything, once per contextTTL; while Sourcebot cannot answer, its last answer stands.
// ponytail: a failure is not remembered, so a hung Sourcebot costs each project list up to probeTimeout; remember it if that bites.
func (c *Client) matches(ctx context.Context, key, query string) (bool, error) {
	c.mu.Lock()
	last, ok := c.seen[key]
	c.mu.Unlock()
	if ok && time.Since(last.at) < contextTTL {
		return last.yes, nil
	}
	var resp struct {
		Files []json.RawMessage `json:"files"`
	}
	_, err := c.call(ctx, http.MethodPost, "/api/search", nil, map[string]any{"query": query, "matches": 1}, &resp)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return last.yes, err
	}
	yes := err == nil && len(resp.Files) > 0
	c.mu.Lock()
	c.seen[key] = answer{yes, time.Now()}
	c.mu.Unlock()
	return yes, nil
}

// confine lets a call reach only a repository of the project's context, at a ref that cannot pass for a git option.
func (c *Client) confine(ctx context.Context, project, repo string, refs ...string) error {
	for _, ref := range refs {
		if ref != "" && !gitRef.MatchString(ref) {
			return fmt.Errorf("%w: a ref is a branch, tag or commit, or a range of them such as a..b", core.ErrInvalidInput)
		}
	}
	if !core.ValidRepoName(repo) {
		return fmt.Errorf("%w: no repository %q", core.ErrNotFound, repo)
	}
	in, err := c.matches(ctx, project+" "+repo, "context:"+project+" repo:^"+regexp.QuoteMeta(repo)+"$")
	if !in {
		return cmp.Or(err, fmt.Errorf("%w: %s is not part of %s's code", core.ErrNotFound, repo, project))
	}
	return nil
}

func (c *Client) Search(ctx context.Context, project, query string) ([]core.CodeMatch, error) {
	body := map[string]any{"query": "context:" + project + " (" + query + ")", "matches": searchMatches, "contextLines": 2}
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
	if _, err := c.call(ctx, http.MethodPost, "/api/search", nil, body, &resp); err != nil {
		return nil, err
	}
	var matches []core.CodeMatch
	files := 0
	for _, file := range resp.Files {
		// The query is text a crafted one can break out of; this check is the real boundary.
		if _, ok := allowedPath(file.FileName.Text); !ok || c.confine(ctx, project, file.Repository) != nil {
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

func (c *Client) Read(ctx context.Context, project, repo, ref, file string) (core.CodeFile, error) {
	if err := c.confine(ctx, project, repo, ref); err != nil {
		return core.CodeFile{}, err
	}
	if dir, ok := strings.CutSuffix(file, "/"); ok || file == "" {
		return c.list(ctx, repo, ref, dir)
	}
	cleaned, ok := allowedPath(file)
	if !ok {
		return core.CodeFile{}, core.ErrNotFound
	}
	query := url.Values{"repo": {repo}, "path": {cleaned}}
	if ref != "" {
		query.Set("ref", ref)
	}
	var resp struct {
		Source string `json:"source"`
		URL    string `json:"externalWebUrl"`
	}
	if _, err := c.call(ctx, http.MethodGet, "/api/source", query, nil, &resp); err != nil {
		return core.CodeFile{}, err
	}
	return core.CodeFile{Repo: repo, Path: cleaned, Content: resp.Source, URL: resp.URL}, nil
}

// list names what a folder holds, a folder with a trailing /.
func (c *Client) list(ctx context.Context, repo, ref, dir string) (core.CodeFile, error) {
	paths := []string{}
	if dir != "" {
		cleaned, ok := allowedPath(dir)
		if !ok {
			return core.CodeFile{}, core.ErrNotFound
		}
		dir, paths = cleaned, []string{cleaned}
	}
	var resp struct {
		Tree treeNode `json:"tree"`
	}
	body := map[string]any{"repoName": repo, "revisionName": cmp.Or(ref, "HEAD"), "paths": paths}
	if _, err := c.call(ctx, http.MethodPost, "/api/tree", nil, body, &resp); err != nil {
		return core.CodeFile{}, err
	}
	// The tree opens every folder on the way to the one asked for.
	node := resp.Tree
	if dir != "" {
		for name := range strings.SplitSeq(dir, "/") {
			i := slices.IndexFunc(node.Children, func(n treeNode) bool { return n.Name == name })
			if i < 0 {
				return core.CodeFile{}, fmt.Errorf("%w: no folder %s", core.ErrNotFound, dir)
			}
			node = node.Children[i]
		}
	}
	var out strings.Builder
	for _, child := range node.Children {
		if _, ok := allowedPath(child.Name); ok {
			out.WriteString(child.Name)
			if child.Type == "tree" {
				out.WriteByte('/')
			}
			out.WriteByte('\n')
		}
	}
	return core.CodeFile{Repo: repo, Path: dir + "/", Content: out.String()}, nil
}

func (c *Client) Commits(ctx context.Context, project, repo, ref, file, query string) ([]core.Commit, int, error) {
	if err := c.confine(ctx, project, repo, ref); err != nil {
		return nil, 0, err
	}
	params := url.Values{"repo": {repo}, "perPage": {strconv.Itoa(commitsPage)}}
	if file != "" {
		cleaned, ok := allowedPath(file)
		if !ok {
			return nil, 0, core.ErrNotFound
		}
		params.Set("path", cleaned)
	}
	if ref != "" {
		params.Set("ref", ref)
	}
	if query != "" {
		params.Set("query", query)
	}
	var resp []struct {
		Hash, Date, Message, Refs, AuthorName string
	}
	header, err := c.call(ctx, http.MethodGet, "/api/commits", params, nil, &resp)
	if err != nil {
		return nil, 0, err
	}
	commits := make([]core.Commit, len(resp))
	for i, r := range resp {
		commits[i] = core.Commit{Hash: r.Hash, Date: r.Date, Author: r.AuthorName, Subject: r.Message, Refs: r.Refs}
	}
	total, _ := strconv.Atoi(header.Get("X-Total-Count"))
	return commits, max(total, len(commits)), nil
}

func (c *Client) Diff(ctx context.Context, project, repo, base, head, file string) ([]core.CodeChange, error) {
	if err := c.confine(ctx, project, repo, base, head); err != nil {
		return nil, err
	}
	params := url.Values{"repo": {repo}, "base": {base}, "head": {head}}
	if file != "" {
		cleaned, ok := allowedPath(file)
		if !ok {
			return nil, core.ErrNotFound
		}
		params.Set("path", cleaned)
	}
	var resp struct {
		Files []struct {
			OldPath, NewPath *string
			Hunks            []struct {
				OldRange, NewRange struct{ Start, Lines int }
				Heading, Body      string
			}
		}
	}
	if _, err := c.call(ctx, http.MethodGet, "/api/diff", params, nil, &resp); err != nil {
		return nil, err
	}
	var changes []core.CodeChange
	for _, f := range resp.Files {
		before, after := deref(f.OldPath), deref(f.NewPath)
		// A secret file must not reach the model under either of its names.
		if _, ok := allowedPath(cmp.Or(before, after)); !ok {
			continue
		}
		if _, ok := allowedPath(cmp.Or(after, before)); !ok {
			continue
		}
		change := core.CodeChange{Path: after, Status: "M"}
		switch {
		case before == "":
			change.Status = "A"
		case after == "":
			change.Path, change.Status = before, "D"
		case before != after:
			change.Path, change.Status = before+" → "+after, "R"
		}
		var body strings.Builder
		for _, h := range f.Hunks {
			fmt.Fprintf(&body, "@@ -%d,%d +%d,%d @@ %s\n", h.OldRange.Start, h.OldRange.Lines, h.NewRange.Start, h.NewRange.Lines, h.Heading)
			for line := range strings.Lines(h.Body) {
				body.WriteString(line)
				switch line[0] {
				case '+':
					change.Added++
				case '-':
					change.Removed++
				}
			}
			if !strings.HasSuffix(h.Body, "\n") {
				body.WriteByte('\n')
			}
		}
		change.Body = body.String()
		changes = append(changes, change)
	}
	return changes, nil
}

func (c *Client) Blame(ctx context.Context, project, repo, ref, file string) ([]core.BlameRange, error) {
	if err := c.confine(ctx, project, repo, ref); err != nil {
		return nil, err
	}
	cleaned, ok := allowedPath(file)
	if !ok {
		return nil, core.ErrNotFound
	}
	params := url.Values{"repo": {repo}, "path": {cleaned}}
	if ref != "" {
		params.Set("ref", ref)
	}
	var resp struct {
		Ranges []struct {
			Hash                 string
			StartLine, LineCount int
		}
		Commits map[string]struct{ Date, Message, AuthorName string }
	}
	if _, err := c.call(ctx, http.MethodGet, "/api/blame", params, nil, &resp); err != nil {
		return nil, err
	}
	var ranges []core.BlameRange
	for _, r := range resp.Ranges {
		// Neighbouring ranges of one commit read as one.
		if n := len(ranges); n > 0 && ranges[n-1].Commit.Hash == r.Hash && ranges[n-1].Start+ranges[n-1].Lines == r.StartLine {
			ranges[n-1].Lines += r.LineCount
			continue
		}
		commit := resp.Commits[r.Hash]
		ranges = append(ranges, core.BlameRange{
			Start: r.StartLine, Lines: r.LineCount,
			Commit: core.Commit{Hash: r.Hash, Date: commit.Date, Author: commit.AuthorName, Subject: commit.Message},
		})
	}
	return ranges, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

func (c *Client) call(ctx context.Context, method, endpoint string, query url.Values, body, out any) (http.Header, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode sourcebot request: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	target := c.url + endpoint
	if query != nil {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return nil, fmt.Errorf("build sourcebot request: %w", err)
	}
	req.Header.Set("X-Sourcebot-Api-Key", c.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call sourcebot %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		var failure struct{ ErrorCode, Message string }
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&failure)
		// Only Sourcebot's own not-found is one; a bare 404 means a wrong BOREAS_SOURCEBOT_URL.
		if resp.StatusCode == http.StatusNotFound && failure.ErrorCode != "" {
			return nil, fmt.Errorf("%w: %s", core.ErrNotFound, failure.Message)
		}
		return nil, fmt.Errorf("sourcebot %s: %s", endpoint, strings.TrimSpace(resp.Status+" "+failure.ErrorCode+" "+failure.Message))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, responseLimit)).Decode(out); err != nil {
		return nil, fmt.Errorf("decode sourcebot %s, at most %d MiB: %w", endpoint, responseLimit>>20, err)
	}
	return resp.Header, nil
}
