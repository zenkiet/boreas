package sourcebot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenkiet/boreas/internal/core"
)

func TestScopeConfinesSearchAndReads(t *testing.T) {
	var query string
	var sourceQueries []map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sourcebot-Api-Key") != "key" {
			t.Errorf("missing api key on %s", r.URL.Path)
		}
		switch r.URL.Path {
		case "/api/repos":
			repos := make([]map[string]string, 100)
			if r.URL.Query().Get("page") == "2" {
				repos = repos[:1]
			}
			for i := range repos {
				repos[i] = map[string]string{"repoName": "github.com/acme/web"}
			}
			_ = json.NewEncoder(w).Encode(repos)
		case "/api/search":
			var body struct{ Query string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			query = body.Query
			file := func(repo, name string) map[string]any {
				return map[string]any{
					"repository": repo, "fileName": map[string]any{"text": name},
					"chunks": []map[string]any{{"content": "surcharge", "contentStart": map[string]any{"lineNumber": 7}}},
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []any{
				file("github.com/acme/web", "src/receipt.ts"),
				file("github.com/acme/web", "Api/Web.Release.config"),
				file("github.com/acme/secret", "src/receipt.ts"),
			}})
		case "/api/source":
			sourceQueries = append(sourceQueries, r.URL.Query())
			_ = json.NewEncoder(w).Encode(map[string]any{"source": "export const x = 1"})
		}
	}))
	defer server.Close()
	client, repos, ctx := New(server.URL, "key"), []string{"github.com/acme/web"}, context.Background()

	if all, err := client.Repos(ctx); err != nil || len(all) != 101 {
		t.Fatalf("repos across pages: %d, err %v", len(all), err)
	}
	matches, err := client.Search(ctx, repos, "surcharge")
	if err != nil || len(matches) != 1 || matches[0].Path != "src/receipt.ts" || matches[0].Line != 7 {
		t.Fatalf("matches %+v, err %v", matches, err)
	}
	if query != `(repo:^github\.com/acme/web$) (surcharge)` {
		t.Fatalf("query %q", query)
	}
	for _, path := range []string{"../etc/passwd", "Api/Web.Release.config", "app/.ENV.local", ".vs/x.json", "deploy/prod.env", ".aws/credentials"} {
		if _, err := client.Read(ctx, repos, "github.com/acme/web", path); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("read %q: %v", path, err)
		}
	}
	if _, err := client.Read(ctx, repos, "github.com/acme/secret", "src/receipt.ts"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("read outside the project's repos: %v", err)
	}
	if _, err := client.Read(ctx, repos, "github.com/acme/web", "src/a.ts&repo=github.com/acme/other"); err != nil {
		t.Fatal(err)
	}
	if len(sourceQueries) != 1 || sourceQueries[0]["repo"][0] != "github.com/acme/web" || len(sourceQueries[0]["repo"]) != 1 {
		t.Fatalf("source requests %v", sourceQueries)
	}
}
