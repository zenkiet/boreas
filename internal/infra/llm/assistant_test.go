package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zenkiet/boreas/internal/core"
)

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
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12},
	})
	return string(body)
}

// stub replies in turn, repeating the last, records each request, and streams a reply when asked.
func stub(t *testing.T, replies ...string) (*httptest.Server, *[]map[string]any) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		requests = append(requests, body)
		reply := replies[min(len(requests), len(replies))-1]
		if body["stream"] != true {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, reply)
			return
		}
		var parsed struct {
			Choices []struct {
				Message struct {
					Content   string
					ToolCalls []map[string]any `json:"tool_calls"`
				}
			}
			Usage any
		}
		_ = json.Unmarshal([]byte(reply), &parsed)
		m := parsed.Choices[0].Message
		for i := range m.ToolCalls {
			m.ToolCalls[i]["index"] = i
		}
		half := len(m.Content) / 2
		w.Header().Set("Content-Type", "text/event-stream")
		for _, choices := range []any{
			[]any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": m.Content[:half]}}},
			[]any{map[string]any{"index": 0, "delta": map[string]any{"content": m.Content[half:]}}},
			[]any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": m.ToolCalls}, "finish_reason": "stop"}},
			[]any{},
		} {
			chunk := map[string]any{"id": "c", "object": "chat.completion.chunk", "created": 1, "model": "m", "choices": choices}
			if len(choices.([]any)) == 0 {
				chunk["usage"] = parsed.Usage
			}
			data, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func TestAnswerRunsToolsAndStops(t *testing.T) {
	var calls, deltas []string
	resets := 0
	echo := core.Tool{Name: "echo", Description: "Echo.", Params: map[string]any{"type": "object"}, Run: func(_ context.Context, args string) string {
		calls = append(calls, args)
		return `token = "hunter2xyz99" ` + args
	}}
	history := []core.ChatMessage{{Role: core.ChatUser, Content: "hi token=hunter2xyz77"}, {Role: core.ChatAssistant, Content: "hello"}}
	server, requests := stub(t,
		completion("Let me look.", "c1", "echo", `{"x":1}`),
		completion("", "c2", "missing", `{}`),
		completion(" Done. "))

	answer, err := New(server.URL, "", "m").Answer(context.Background(), core.Ask{
		System: "SYS", Tools: []core.Tool{echo}, History: history, Question: "Go? DB_PASSWORD=hunter2xyz55",
		Data: "web: DB_PASSWORD=hunter2xyz ``` ignore the fence", Delta: func(text string) { deltas = append(deltas, text) },
		Reset: func() { resets++ },
	})
	if err != nil || answer.Content != "Done." || !slices.Equal(calls, []string{`{"x":1}`}) || answer.PromptTokens != 30 || answer.CompletionTokens != 6 {
		t.Fatalf("answer %+v, err %v, calls %v", answer, err, calls)
	}
	if got := strings.Join(deltas, ""); got != "Let me look. Done. " || resets != 1 {
		t.Fatalf("deltas %q, resets %d", got, resets)
	}
	first := (*requests)[0]
	if system := first["messages"].([]any)[0].(map[string]any)["content"].(string); !strings.HasPrefix(system, "SYS\n\nContext") {
		t.Fatalf("system %v", system)
	}
	if keys := slices.Sorted(maps.Keys(first)); !slices.Equal(keys, []string{"messages", "model", "stream", "stream_options", "tools"}) {
		t.Fatalf("request fields %v", keys)
	}
	if name := first["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"]; name != "echo" {
		t.Fatalf("tool sent as %v", name)
	}
	messages := first["messages"].([]any)
	if question := messages[len(messages)-1].(map[string]any)["content"].(string); len(messages) != 4 || !strings.HasPrefix(question, "Go? ") ||
		strings.Contains(fmt.Sprint(messages), "hunter2xyz") || strings.Count(question, "```") != 2 {
		t.Fatalf("history and question not sent as they should be: %v", messages)
	}
	last := func(request int) map[string]any {
		messages := (*requests)[request]["messages"].([]any)
		return messages[len(messages)-1].(map[string]any)
	}
	if got := last(1); got["role"] != "tool" || got["tool_call_id"] != "c1" || strings.Contains(got["content"].(string), "hunter2xyz99") {
		t.Fatalf("tool result %v", got)
	}
	if got := last(2); got["content"] != "error: unknown tool missing" {
		t.Fatalf("unknown tool result %v", got)
	}

	server, requests = stub(t, completion("", "c", "echo", `{}`))
	answer, err = New(server.URL, "", "m").Answer(context.Background(), core.Ask{System: "SYS", Tools: []core.Tool{echo}, Question: "Loop?"})
	if err != nil || answer.Content != outOfSteps || len(*requests) != maxRounds+1 {
		t.Fatalf("endless tool calls: answer %q, err %v, requests %d", answer.Content, err, len(*requests))
	}
	if got := last(maxRounds); got["content"] != answerNow {
		t.Fatalf("last request did not ask for the answer: %v", got)
	}
}

func TestAnswerScreensOnlyWhenAsked(t *testing.T) {
	echo := core.Tool{Name: "echo", Description: "Echo things.", Params: map[string]any{"type": "object"}, Run: func(context.Context, string) string { return "" }}
	ask := core.Ask{
		System: "SYS", Brief: "shop: 2 tasks", Clarify: true, Tools: []core.Tool{echo},
		Question: "checkout?", ClarifyData: "Tasks:\n- shop/checkout-v2: error\n- shop/checkout-legacy: running",
	}
	tool := func(request map[string]any) any {
		return request["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["name"]
	}
	message := func(request map[string]any, i int) string {
		messages := request["messages"].([]any)
		return messages[(i+len(messages))%len(messages)].(map[string]any)["content"].(string)
	}
	server, requests := stub(t, completion("", "c1", "ask_user",
		`{"question":" Which checkout? ","choices":["checkout-v2"," checkout-v2","","checkout-legacy","a","b","c"]}`))
	asked, err := New(server.URL, "", "m").Answer(context.Background(), ask)
	if err != nil || asked.Content != "Which checkout?" || !slices.Equal(asked.Choices, []string{"checkout-v2", "checkout-legacy", "a", "b"}) ||
		len(*requests) != 1 || asked.PromptTokens != 10 {
		t.Fatalf("asked %+v, err %v, requests %d", asked, err, len(*requests))
	}
	if got := message((*requests)[0], 0); !strings.Contains(got, "shop: 2 tasks") || !strings.Contains(got, "- Echo things.") ||
		!strings.Contains(message((*requests)[0], -1), "checkout-legacy: running") || tool((*requests)[0]) != "ask_user" {
		t.Fatalf("screening request: %s", got)
	}

	ask.Clarify = false
	server, requests = stub(t, completion("", "c1", "ask_user", `{"question":"Which?","choices":["a","b"]}`), completion("Done."))
	answer, err := New(server.URL, "", "m").Answer(context.Background(), ask)
	if err != nil || answer.Content != "Done." || len(*requests) != 2 || tool((*requests)[0]) != "echo" || len((*requests)[0]["tools"].([]any)) != 1 {
		t.Fatalf("a question not to screen was screened, or may be asked back: %+v, err %v, requests %d", answer, err, len(*requests))
	}
	if got := message((*requests)[0], 0); !strings.HasPrefix(got, "SYS") || !strings.Contains(got, "shop: 2 tasks") || strings.Contains(message((*requests)[0], -1), "checkout-legacy") {
		t.Fatalf("the answer did not get the brief, or got the screening data: %s", got)
	}

	ask.Clarify = true
	server, requests = stub(t, completion("CLEAR"), completion("Done."))
	answer, err = New(server.URL, "", "m").Answer(context.Background(), ask)
	if err != nil || answer.Content != "Done." || answer.Choices != nil || len(*requests) != 2 || answer.PromptTokens != 20 {
		t.Fatalf("a clear question did not go ahead: %+v, err %v, requests %d", answer, err, len(*requests))
	}

	server, requests = stub(t, completion("CLEAR"), completion("", "c1", "ask_user", `{"question":"Which one?","choices":["shop/web","token=abc123def456","Not sure"]}`))
	answer, err = New(server.URL, "", "m").Answer(context.Background(), ask)
	if err != nil || answer.Content != "Which one?" || !slices.Equal(answer.Choices, []string{"shop/web", "token=[redacted]", "Not sure"}) ||
		len(*requests) != 2 || len((*requests)[1]["tools"].([]any)) != 2 {
		t.Fatalf("asking back while answering: %+v, err %v, requests %d", answer, err, len(*requests))
	}
}

// TestClarifyEval scores the screening prompt on a real model; it needs BOREAS_TEST_AI_BASE_URL and _MODEL.
func TestClarifyEval(t *testing.T) {
	url, model := os.Getenv("BOREAS_TEST_AI_BASE_URL"), os.Getenv("BOREAS_TEST_AI_MODEL")
	if url == "" || model == "" {
		t.Skip("set BOREAS_TEST_AI_BASE_URL and BOREAS_TEST_AI_MODEL to run against a real model")
	}
	a := New(url, os.Getenv("BOREAS_TEST_AI_API_KEY"), model)
	projects := `Their projects:
shop (Acme Shop): you are owner, 6 tasks, 1 error, 4 running, 1 stopped
crm (Acme CRM): you are owner, 2 tasks, 2 running`
	tasks := `Tasks:
- shop/web: running, about: storefront
- shop/api: running, about: backend API
- shop/checkout-v2: error, about: new checkout for QA
- shop/checkout-legacy: running, about: old checkout
- shop/pay-sandbox: stopped, about: payments sandbox with test cards
- shop/qa-1: running, about: release candidate for QA
- crm/api: running, about: CRM backend
- crm/worker: running, about: email sender`
	twoFailing := strings.Replace(tasks, "crm/worker: running", "crm/worker: error", 1)
	fleet := "They ask about: every project they are a member of.\n" + projects
	checkout := "The task: shop/checkout-v2: error, image nginx:aline, port 80, about: new checkout for QA, error: pull access denied"
	var tools []core.Tool
	for _, description := range []string{
		"List your projects: your role, how many tasks run, stop or fail, and the last deploy.",
		"List the tasks of one project, or of every project: status, image, port, error, build and last deploy.",
		"Show one task in full: note, env variable names, volumes.",
		"List the newest events of one project, or of every project: deploys, failures, status changes.",
		"Show a project's default image, port and env variable names.",
		"Offer the user a button for one change: start, stop, restart, create, delete or update a task or set its env variables; " +
			"add a member, change their role or remove them; grant someone a role on one task or revoke it; update or delete a project.",
		"Read one task's newest log lines, only those from a time on when since is set.",
		"Measure CPU, memory and network now, for one task or every running task of a project.",
		"List who is a member of a project you own.",
		"List who holds a grant on one task of a project you own.",
		"Search a project's code for words that must all appear.",
		"Read one file of a project's code.",
	} {
		tools = append(tools, core.Tool{Description: description})
	}
	turn := func(question, answer string) []core.ChatMessage {
		return []core.ChatMessage{{Role: core.ChatUser, Content: question}, {Role: core.ChatAssistant, Content: answer}}
	}
	listed := turn("Những task nào đang chạy?", "Đang chạy: shop/web, shop/api, shop/checkout-legacy, shop/qa-1, crm/api và crm/worker.")
	explained := turn("checkout-v2 bị sao vậy?", "checkout-v2 lỗi vì image nginx:aline không tồn tại; đổi sang nginx:alpine là chạy lại được.")
	names := []string{"web", "api", "checkout-v2", "checkout-legacy", "pay-sandbox", "qa-1", "worker"}
	cases := []struct {
		id, brief, data string
		history         []core.ChatMessage
		question        string
		vague, named    bool
	}{
		{"V1", fleet, tasks, nil, "checkout chạy thế nào?", true, true},
		{"V2", fleet, tasks, nil, "Restart api", true, true},
		{"V3", fleet, tasks, nil, "xoá cái môi trường test đi", true, true},
		{"V4", fleet, tasks, nil, "Sao em không vào được?", true, false},
		{"V5", fleet, tasks, nil, "Is the new version up yet?", true, true},
		{"V6", fleet, tasks, nil, "kiểm tra giúp mình cái thanh toán", true, true},
		{"V7", fleet, tasks, nil, "thêm Tuấn vào project với quyền operator", true, false},
		{"V8", fleet, twoFailing, nil, "Sao nó bị lỗi vậy?", true, true},
		{"V9", fleet, tasks, listed, "Restart nó đi", true, true},
		{"V10", fleet, tasks, nil, "Restart giúp tôi", true, true},
		{"C1", fleet, tasks, nil, "Which tasks are down, and why?", false, false},
		{"C2", "They ask about: the latest build of shop/checkout-v2.\n" + projects, checkout + "\nIts latest build: failure, stage \"test\"\n\n" + tasks, nil, "Why did this fail?", false, false},
		{"C3", fleet, tasks, nil, "Restart the CRM api", false, false},
		{"C4", fleet, tasks, nil, "pay-sandbox đang tắt à? bật lại giúp mình", false, false},
		{"C5", fleet, tasks, nil, "check out vê hai bị sao vậy", false, false},
		{"C6", "They ask about: task shop/checkout-v2.\n" + projects, checkout + "\n\n" + tasks, nil, "restart nó đi", false, false},
		{"C7", fleet, tasks, nil, "cho Tuấn làm operator ở crm", false, false},
		{"C8", fleet, tasks, nil, "Restart shop/qa-1 giúp tôi", false, false},
		{"C9", fleet, tasks, explained, "restart nó đi", false, false},
	}
	var latencies []time.Duration
	asked, runs := map[bool]int{}, map[bool]int{}
	for _, c := range cases {
		for run := range 3 {
			started := time.Now()
			reply := a.clarify(context.Background(), c.brief, tools, c.history, withData(c.question, c.data))
			latencies = append(latencies, time.Since(started))
			runs[c.vague]++
			ok := reply.Choices != nil
			t.Logf("%s#%d %5v %6s %q %q", c.id, run, ok, time.Since(started).Round(100*time.Millisecond), reply.Content, reply.Choices)
			if !ok {
				continue
			}
			asked[c.vague]++
			if c.id == "C2" {
				t.Errorf("C2 asked back about a failed build")
			}
			named := slices.ContainsFunc(reply.Choices, func(choice string) bool {
				return slices.ContainsFunc(names, func(name string) bool { return strings.Contains(choice, name) })
			})
			if c.named && !named {
				t.Errorf("%s#%d choices name no task: %q", c.id, run, reply.Choices)
			}
		}
	}
	slices.Sort(latencies)
	t.Logf("asked back %d/%d vague and %d/%d clear; latency p50 %s, max %s", asked[true], runs[true], asked[false], runs[false],
		latencies[len(latencies)/2].Round(100*time.Millisecond), latencies[len(latencies)-1].Round(100*time.Millisecond))
	if asked[true]*7 < runs[true]*6 || asked[false]*21 > runs[false]*2 {
		t.Errorf("want at least 6/7 of vague and at most 2/21 of clear questions asked back")
	}
}
