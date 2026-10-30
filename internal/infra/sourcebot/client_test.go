package sourcebot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/zenkiet/boreas/internal/core"
)

func file(repo, name string) map[string]any {
	return map[string]any{
		"repository": repo, "fileName": map[string]any{"text": name},
		"chunks": []map[string]any{{"content": "surcharge", "contentStart": map[string]any{"lineNumber": 7}}},
	}
}

func node(name, kind string, children ...any) map[string]any {
	return map[string]any{"name": name, "type": kind, "children": append([]any{}, children...)}
}

// sourcebot answers like Sourcebot for a context "shop" holding github.com/acme/web only; seen returns the
// searches it got and the query string of the last request.
func sourcebot(t *testing.T) (server *httptest.Server, seen func() ([]string, map[string][]string)) {
	var mu sync.Mutex
	var searches []string
	var queries []map[string][]string
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sourcebot-Api-Key") != "key" {
			t.Errorf("missing api key on %s", r.URL.Path)
		}
		mu.Lock()
		defer mu.Unlock()
		queries = append(queries, r.URL.Query())
		reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/api/search":
			var body struct{ Query string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			searches = append(searches, body.Query)
			switch body.Query {
			case "context:nope":
				w.WriteHeader(http.StatusNotFound)
				reply(map[string]any{"errorCode": "SEARCH_CONTEXT_NOT_FOUND", "message": `Search context "nope" not found`})
			case "context:shop", `context:shop repo:^github\.com/acme/web$`:
				reply(map[string]any{"files": []any{file("github.com/acme/web", "src/receipt.ts")}})
			case "context:shop (surcharge)":
				reply(map[string]any{"files": []any{
					file("github.com/acme/web", "src/receipt.ts"),
					file("github.com/acme/web", "Api/Web.Release.config"),
					file("github.com/acme/secret", "src/receipt.ts"),
				}})
			default:
				reply(map[string]any{"files": []any{}})
			}
		case "/api/source":
			reply(map[string]any{"source": "export const x = 1", "externalWebUrl": "https://example.com/blob"})
		case "/api/tree":
			reply(map[string]any{"tree": node("root", "tree",
				node("src", "tree", node("lib", "tree", node("fee.ts", "blob")), node("a.ts", "blob"), node(".env", "blob")))})
		case "/api/commits":
			w.Header().Set("X-Total-Count", "42")
			reply([]map[string]any{{"hash": "abc", "date": "2026-10-28T10:00:00+07:00", "message": "feat(MYL-1): fee", "refs": "", "authorName": "Dev"}})
		case "/api/diff":
			hunk := func(body string) []any {
				return []any{map[string]any{"oldRange": map[string]int{"start": 1, "lines": 2}, "newRange": map[string]int{"start": 1, "lines": 3}, "body": body}}
			}
			reply(map[string]any{"files": []any{
				map[string]any{"oldPath": "appsettings.json", "newPath": "appsettings.json", "hunks": hunk("-a\n+b\n")},
				map[string]any{"oldPath": "old.ts", "newPath": "config/.env", "hunks": hunk("-a\n+b\n")},
				map[string]any{"oldPath": nil, "newPath": "src/fee.ts", "hunks": hunk("+x\n+y\n kept\n-z\n")},
			}})
		case "/api/blame":
			reply(map[string]any{
				"ranges": []any{
					map[string]any{"hash": "c1", "startLine": 1, "lineCount": 2},
					map[string]any{"hash": "c1", "startLine": 3, "lineCount": 1},
					map[string]any{"hash": "c2", "startLine": 4, "lineCount": 5},
				},
				"commits": map[string]any{
					"c1": map[string]any{"date": "2026-10-01T00:00:00Z", "message": "first", "authorName": "A"},
					"c2": map[string]any{"date": "2026-10-02T00:00:00Z", "message": "second", "authorName": "B"},
				},
			})
		}
	}))
	t.Cleanup(server.Close)
	return server, func() ([]string, map[string][]string) {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(searches), queries[len(queries)-1]
	}
}

func TestContextConfinesEveryCall(t *testing.T) {
	server, seen := sourcebot(t)
	client, ctx := New(server.URL, "key"), context.Background()

	for range 2 {
		if has := client.Has(ctx, []string{"shop", "nope"}); !slices.Equal(has, []bool{true, false}) {
			t.Fatalf("has %v", has)
		}
	}
	if searches, _ := seen(); len(searches) != 2 {
		t.Fatalf("contexts asked %d times, want once each: %v", len(searches), searches)
	}
	matches, err := client.Search(ctx, "shop", "surcharge")
	if err != nil || len(matches) != 1 || matches[0].Path != "src/receipt.ts" || matches[0].Line != 7 {
		t.Fatalf("matches %+v, err %v", matches, err)
	}
	for _, path := range []string{"../etc/passwd", "Api/Web.Release.config", "app/.ENV.local", ".vs/x.json", "deploy/prod.env", ".aws/credentials"} {
		if _, err := client.Read(ctx, "shop", "github.com/acme/web", "", path); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("read %q: %v", path, err)
		}
	}
	for _, repo := range []string{"github.com/acme/secret", `x$ or context:other repo:^y`} {
		if _, err := client.Read(ctx, "shop", repo, "", "src/receipt.ts"); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("read outside the context, %q: %v", repo, err)
		}
	}
	if _, err := client.Read(ctx, "shop", "github.com/acme/web", "--output=/tmp/x", "src/a.ts"); !errors.Is(err, core.ErrInvalidInput) {
		t.Errorf("a ref like an option: %v", err)
	}
	read, err := client.Read(ctx, "shop", "github.com/acme/web", "abc123", "src/a.ts&repo=github.com/acme/other")
	if err != nil || read.URL != "https://example.com/blob" {
		t.Fatalf("read %+v, err %v", read, err)
	}
	if _, got := seen(); got["repo"][0] != "github.com/acme/web" || len(got["repo"]) != 1 || got["ref"][0] != "abc123" {
		t.Fatalf("source query %v", got)
	}
	for dir, want := range map[string]string{"src/": "lib/\na.ts\n", "src/lib/": "fee.ts\n", "/": "src/\n"} {
		folder, err := client.Read(ctx, "shop", "github.com/acme/web", "", dir)
		if err != nil || folder.Content != want {
			t.Errorf("folder %s: %+v, err %v", dir, folder, err)
		}
	}
	if _, err := client.Read(ctx, "shop", "github.com/acme/web", "", "src/nope/"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("a missing folder: %v", err)
	}
}

func TestHistoryDiffAndBlame(t *testing.T) {
	server, seen := sourcebot(t)
	client, ctx := New(server.URL, "key"), context.Background()

	commits, total, err := client.Commits(ctx, "shop", "github.com/acme/web", "HEAD..abc", "src/", "MYL-1")
	if err != nil || total != 42 || len(commits) != 1 || commits[0].Subject != "feat(MYL-1): fee" || commits[0].Author != "Dev" {
		t.Fatalf("commits %+v, total %d, err %v", commits, total, err)
	}
	if _, got := seen(); got["ref"][0] != "HEAD..abc" || got["path"][0] != "src" || got["query"][0] != "MYL-1" {
		t.Fatalf("commits query %v", got)
	}
	changes, err := client.Diff(ctx, "shop", "github.com/acme/web", "abc^", "def", "")
	if err != nil || len(changes) != 1 {
		t.Fatalf("a secret file passed the diff: %+v, err %v", changes, err)
	}
	if c := changes[0]; c.Path != "src/fee.ts" || c.Status != "A" || c.Added != 2 || c.Removed != 1 || !strings.HasPrefix(c.Body, "@@ -1,2 +1,3 @@") {
		t.Fatalf("change %+v", c)
	}
	ranges, err := client.Blame(ctx, "shop", "github.com/acme/web", "", "src/fee.ts")
	if err != nil || len(ranges) != 2 || ranges[0].Lines != 3 || ranges[0].Commit.Author != "A" || ranges[1].Start != 4 {
		t.Fatalf("neighbouring ranges of one commit stay apart: %+v, err %v", ranges, err)
	}
}

func TestErrorsCarrySourcebotsReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/source" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"errorCode":"DEPLOYMENT_RESTRICTED","message":"This deployment is restricted."}`))
	}))
	defer server.Close()
	client := New(server.URL, "key")
	_, err := client.Search(context.Background(), "shop", "fee")
	if err == nil || !strings.Contains(err.Error(), "DEPLOYMENT_RESTRICTED This deployment is restricted.") {
		t.Fatalf("search: %v", err)
	}
	if _, err := client.call(context.Background(), http.MethodGet, "/api/source", nil, nil, nil); err == nil || errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a bare 404 means a wrong URL, not a missing file: %v", err)
	}
}
