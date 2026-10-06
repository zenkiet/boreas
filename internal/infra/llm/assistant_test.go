package llm

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/zenkiet/boreas/internal/core"
)

type fakeCode struct{ scopes [][]string }

func (f *fakeCode) Repos(context.Context) ([]string, error) { return nil, nil }

func (f *fakeCode) Search(_ context.Context, repos []string, _ string) ([]core.CodeMatch, error) {
	f.scopes = append(f.scopes, repos)
	return []core.CodeMatch{{Repo: repos[0], Path: "src/receipt.ts", Line: 7, Snippet: "surcharge"}}, nil
}

func (f *fakeCode) Read(_ context.Context, repos []string, repo, path string) (core.CodeFile, error) {
	f.scopes = append(f.scopes, repos)
	return core.CodeFile{Repo: repo, Path: path, Content: "export const surcharge = 3", URL: "https://example.com/" + path}, nil
}

func completion(content string, call ...string) string {
	message := map[string]any{"role": "assistant", "content": content}
	if len(call) == 3 {
		message["tool_calls"] = []any{map[string]any{
			"id": call[0], "type": "function",
			"function": map[string]any{"name": call[1], "arguments": call[2]},
		}}
	}
	body, _ := json.Marshal(map[string]any{
		"id": "c", "object": "chat.completion", "created": 1, "model": "m",
		"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": message}},
	})
	return string(body)
}

// stub answers each request with the next reply, repeating the last one, and records every request body.
func stub(t *testing.T, replies ...string) (*httptest.Server, *[]map[string]any) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, replies[min(len(requests), len(replies))-1])
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestAnswerReadsOnlyTheProjectAndStops(t *testing.T) {
	repos := []string{"github.com/acme/web"}
	history := []core.ChatMessage{{Role: core.ChatUser, Content: "hi"}, {Role: core.ChatAssistant, Content: "hello"}}
	server, requests := stub(t,
		completion("", "c1", "search_code", `{"query":"surcharge"}`),
		completion("", "c2", "read_file", `{"repo":"github.com/acme/web","path":"src/receipt.ts"}`),
		completion(" Surcharge is 3%. "))
	code := &fakeCode{}

	answer, err := New(server.URL, "", "m").Answer(context.Background(), code, repos, history, "How is the surcharge shown?")
	if err != nil || answer.Role != core.ChatAssistant || answer.Content != "Surcharge is 3%." {
		t.Fatalf("answer %+v, err %v", answer, err)
	}
	if !slices.Equal(answer.Sources, []core.CodeSource{{Repo: repos[0], Path: "src/receipt.ts", URL: "https://example.com/src/receipt.ts"}}) {
		t.Fatalf("sources %+v", answer.Sources)
	}
	for _, scope := range code.scopes {
		if !slices.Equal(scope, repos) {
			t.Fatalf("a tool read outside the project: %v", scope)
		}
	}
	first, second := (*requests)[0], (*requests)[1]
	if keys := slices.Sorted(maps.Keys(first)); !slices.Equal(keys, []string{"messages", "model", "tools"}) {
		t.Fatalf("request fields %v", keys)
	}
	if messages := first["messages"].([]any); len(messages) != 4 {
		t.Fatalf("history and question not sent: %v", messages)
	}
	messages := second["messages"].([]any)
	if last := messages[len(messages)-1].(map[string]any); last["role"] != "tool" || last["tool_call_id"] != "c1" {
		t.Fatalf("tool result %v", last)
	}

	server, requests = stub(t, completion("", "c", "search_code", `{"query":"loop"}`))
	answer, err = New(server.URL, "", "m").Answer(context.Background(), code, repos, nil, "Loop?")
	if err != nil || answer.Content != outOfSteps || len(*requests) != maxRounds+1 {
		t.Fatalf("endless tool calls: answer %q, err %v, requests %d", answer.Content, err, len(*requests))
	}
	final := (*requests)[maxRounds]["messages"].([]any)
	if last := final[len(final)-1].(map[string]any); last["content"] != answerNow {
		t.Fatalf("last request did not ask for the answer: %v", last)
	}
}

func TestRedactHidesSecretsNotRules(t *testing.T) {
	code := `const apiKey = "AbC123xyz789";
db := "Server=db;User Id=sa;Password=Pa55word;"
url := "postgres://boreas:s3cretpw@db/boreas"
aws := "AKIAABCDEFGHIJKLMNOP"
error := "Password must be 8 characters"
if (password.length < 8 && task-management-component-list) {}`
	got := redact(code)
	for _, secret := range []string{"AbC123xyz789", "Pa55word", "s3cretpw", "AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(got, secret) {
			t.Errorf("%s leaked:\n%s", secret, got)
		}
	}
	for _, rule := range []string{"Password must be 8 characters", "password.length < 8", "task-management-component-list"} {
		if !strings.Contains(got, rule) {
			t.Errorf("%q lost:\n%s", rule, got)
		}
	}
}
