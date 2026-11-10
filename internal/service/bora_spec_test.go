package service

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/option"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/infra/llm"
)

func TestBoraSpec(t *testing.T) {
	svc, _ := newProjects(t)
	repos := map[string]string{"shop": "github.com/acme/web", "crm": "github.com/acme/crm"}
	svc.Code, svc.Specs, svc.Conversations = fakeCode{contexts: repos}, newFakeSpecStore(), newFakeBoraStore()
	var changes atomic.Int32
	svc.Changed = func() { changes.Add(1) }
	ctx, owner := context.Background(), member()
	for _, slug := range []string{"shop", "crm", "bare"} {
		if _, err := svc.Create(ctx, owner, CreateProjectInput{Slug: slug}); err != nil {
			t.Fatal(err)
		}
	}
	ticket := "Export invoices as CSV\nThe export signs with password=hunter2."
	if _, err := svc.StartSpec(ctx, owner, "wo-718", []string{"shop"}, ticket); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("without a plan model: %v", err)
	}
	for _, tc := range []struct {
		key, ticket string
		projects    []string
		want        error
	}{
		{"wo 718", ticket, []string{"shop"}, core.ErrInvalidInput},
		{"wo-718", " ", []string{"shop"}, core.ErrInvalidInput},
		{"wo-718", strings.Repeat("é", ticketRunes+1), []string{"shop"}, core.ErrInvalidInput},
		{"wo-718", ticket, nil, core.ErrInvalidInput},
		{"wo-718", ticket, []string{"a", "b", "c", "d", "e", "f"}, core.ErrInvalidInput},
		{"wo-718", ticket, []string{"shop", "bare"}, core.ErrForbidden},
		{"wo-718", ticket, []string{"shop", "nope"}, core.ErrNotFound},
	} {
		if _, err := svc.StartSpec(ctx, owner, tc.key, tc.projects, tc.ticket); !errors.Is(err, tc.want) {
			t.Errorf("start %q %v: %v, want %v", tc.key, tc.projects, err, tc.want)
		}
	}

	var script func(context.Context, core.Ask) (string, error)
	svc.Plan = func(ctx context.Context, _ uuid.UUID, ask core.Ask) (string, []string, error) {
		answer, err := script(ctx, ask)
		return answer, nil, err
	}
	run := func(ctx context.Context, ask core.Ask, name, args string) string {
		if i := slices.IndexFunc(ask.Tools, func(tool core.Tool) bool { return tool.Name == name }); i >= 0 {
			return ask.Tools[i].Run(ctx, args)
		}
		return "not offered"
	}
	settle := func(id uuid.UUID) core.BoraSpec {
		t.Helper()
		if events, _ := svc.FollowSpec(ctx, owner, id); events != nil {
			collect(events)
		}
		spec, err := svc.BoraSpec(ctx, owner, id)
		if err != nil {
			t.Fatal(err)
		}
		return spec
	}
	decide := func(ctx context.Context, ask core.Ask) {
		if got := run(ctx, ask, "propose_decisions", `{"findings":"Invoices live in crm.","decisions":[]}`); !strings.HasPrefix(got, "error") {
			t.Errorf("no decisions: %q", got)
		}
		run(ctx, ask, "propose_decisions", `{"findings":"Invoices live in crm.","decisions":[{"text":"Admins export","by_ai":false},{"text":"UTF-8 with a BOM","by_ai":true}]}`)
	}

	hold := make(chan struct{})
	script = func(ctx context.Context, ask core.Ask) (string, error) {
		<-hold
		if !slices.EqualFunc(ask.Tools, []string{"search_code", "read_file", "code_history", "code_diff", "blame", "ask_sourcebot", "propose_decisions", "ask_questions"},
			func(tool core.Tool, name string) bool { return tool.Name == name }) ||
			ask.Rounds != 10 || ask.Limit != specDataLimit || !strings.Contains(ask.Data, "The ticket wo-718:\nExport invoices as CSV") ||
			strings.Contains(ask.Data+ask.Brief, "hunter2") || !strings.Contains(ask.System, "Write everything in English") {
			t.Errorf("first ask: %+v", ask)
		}
		run(ctx, ask, "search_code", `{"project":"crm","query":"invoice"}`)
		run(ctx, ask, "ask_questions", `{"findings":"Invoices live in crm.","questions":[
			{"text":"Who exports?","why":"Roles differ.","multi":false,"options":[{"label":"Admins","detail":"a"},{"label":"Everyone","detail":"b"}]},
			{"text":"Which columns?","why":"Width.","multi":true,"options":[{"label":"Total","detail":""},{"label":"Tax","detail":""}]}]}`)
		if got := run(ctx, ask, "ask_questions", `{"findings":"","questions":[{"text":"Who?","why":"","multi":false,"options":[{"label":"Admins","detail":""}]}]}`); !strings.HasPrefix(got, "error") {
			t.Errorf("a question with one option: %q", got)
		}
		return "", nil
	}
	started, err := svc.StartSpec(ctx, owner, "wo-718", []string{"shop", "crm", "shop"}, ticket)
	if err != nil || !started.Running || started.State != core.SpecResearching || !slices.Equal(started.Slugs, []string{"crm", "shop"}) || started.CreatedByName != owner.Username {
		t.Fatalf("started %+v, %v", started, err)
	}
	id := started.ID
	events, _ := svc.FollowSpec(ctx, owner, id)
	close(hold)
	got := collect(events)
	spec := settle(id)
	if eventNames(got) != "step step step done" || got[0].Key != "research" || got[1].Text != "Searching crm's code…" || !got[2].Finished {
		t.Errorf("events %s: %+v", eventNames(got), got)
	}
	if spec.Status() != core.SpecAsking || spec.Title != "Export invoices as CSV" || strings.Contains(spec.Body.Ticket, "hunter2") ||
		spec.Body.Findings != "Invoices live in crm." || len(spec.Body.Rounds) != 1 || spec.Body.Rounds[0].Questions[1].ID != "Q1.2" {
		t.Fatalf("after the first round %+v", spec)
	}

	for _, tc := range []struct {
		round   int
		answers []core.SpecAnswer
		want    error
	}{
		{2, nil, core.ErrConflict},
		{1, []core.SpecAnswer{{Question: "Q9.9"}}, core.ErrInvalidInput},
		{1, []core.SpecAnswer{{Question: "Q1.1", Options: []string{"Admins", "Everyone"}}}, core.ErrInvalidInput},
		{1, []core.SpecAnswer{{Question: "Q1.2", Options: []string{"Color"}}}, core.ErrInvalidInput},
	} {
		if _, err := svc.AnswerSpec(ctx, owner, id, tc.round, tc.answers, ""); !errors.Is(err, tc.want) {
			t.Errorf("answer %d %+v: %v, want %v", tc.round, tc.answers, err, tc.want)
		}
	}
	hold = make(chan struct{})
	script = func(ctx context.Context, ask core.Ask) (string, error) {
		<-hold
		if !strings.Contains(ask.Data, "Q1.1 Who exports?\nAnswer: Admins; with an audit line") || !strings.Contains(ask.Data, "Q1.2 Which columns?\nAnswer: left to Bora") {
			t.Errorf("the answers are not sent: %s", ask.Data)
		}
		decide(ctx, ask)
		return "", nil
	}
	if _, err := svc.AnswerSpec(ctx, owner, id, 1, []core.SpecAnswer{{Question: "Q1.1", Options: []string{"Admins"}, Other: "with an audit line"}}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AnswerSpec(ctx, owner, id, 1, nil, ""); !errors.Is(err, core.ErrConflict) {
		t.Errorf("a second answer while Bora reads: %v", err)
	}
	close(hold)
	if spec = settle(id); spec.Status() != core.SpecConfirming || len(spec.Body.Decisions) != 2 || spec.Body.Decisions[1] != (core.SpecDecision{ID: "D2", Text: "UTF-8 with a BOM", ByAI: true}) {
		t.Fatalf("decisions %+v", spec)
	}

	if _, err := svc.AnswerSpec(ctx, owner, id, 1, nil, " "); !errors.Is(err, core.ErrInvalidInput) {
		t.Errorf("a correction without a note: %v", err)
	}
	script = func(ctx context.Context, ask core.Ask) (string, error) {
		if !strings.Contains(ask.Data, "The team adds: No audit line.") || run(ctx, ask, "ask_questions", "") == "not offered" {
			t.Errorf("the correction: %s", ask.Data)
		}
		decide(ctx, ask)
		return "", nil
	}
	if _, err := svc.AnswerSpec(ctx, owner, id, 1, nil, "No audit line."); err != nil {
		t.Fatal(err)
	}
	if spec = settle(id); spec.Status() != core.SpecConfirming || len(spec.Body.Rounds) != 2 {
		t.Fatalf("after the correction %+v", spec)
	}

	writer := func() func(context.Context, core.Ask) (string, error) {
		ready, planned := make(chan struct{}), atomic.Int32{}
		return func(ctx context.Context, ask core.Ask) (string, error) {
			switch {
			case strings.HasPrefix(ask.Question, "Call write_brief"):
				purpose := "Lets admins take invoices out of the product as a CSV file."
				for _, bad := range []string{
					`{"title":"t","change":"Export","why":"` + purpose + `","contract":"c","capabilities":[]}`,
					`{"title":"t","change":"add-export","why":"` + purpose + `","contract":"c","capabilities":[{"path":"invoice-export","purpose":"` + purpose +
						`","requirements":[{"name":"x","text":"Admins see Export","projects":["shop"]}]}]}`,
					`{"title":"t","change":"add-export","why":"` + purpose + `","contract":"c","capabilities":[{"path":"invoice-export","purpose":"` + purpose +
						`","requirements":[{"name":"x","text":"Admins SHALL see Export","projects":["bare"]}]}]}`,
				} {
					if got := run(ctx, ask, "write_brief", bad); !strings.HasPrefix(got, "error") {
						t.Errorf("a brief OpenSpec would reject: %q", got)
					}
				}
				run(ctx, ask, "write_brief", `{"title":"Export invoices","change":"wo-718-add-invoice-export","why":"`+purpose+`","contract":"GET /invoices.csv",
					"capabilities":[{"path":"billing/invoice-export","purpose":"`+purpose+`","requirements":[
						{"name":"Export button","text":"The invoice list SHALL show Export to admins","projects":["shop"]},
						{"name":"CSV totals","text":"The CSV MUST carry each invoice's total","projects":["crm"]},
						{"name":"No button for others","text":"The invoice list SHALL hide Export from everyone else","projects":["shop","crm"]}]}]}`)
			case strings.HasPrefix(ask.Question, "Call write_plan"):
				if planned.Add(1) == 2 {
					close(ready)
				}
				select {
				case <-ready:
				case <-time.After(5 * time.Second):
					return "", errors.New("the plans ran one after another")
				}
				slug := strings.Fields(ask.Question)[3]
				other := map[string]string{"shop": "crm", "crm": "shop"}[slug]
				if got := run(ctx, ask, "search_code", `{"project":"`+other+`","query":"x"}`); !strings.Contains(got, "not a member of a project named "+other) {
					t.Errorf("the plan of %s read %s: %q", slug, other, got)
				}
				run(ctx, ask, "read_file", `{"project":"`+slug+`","repo":"`+repos[slug]+`","path":"src/fee.ts","ref":""}`)
				if got := run(ctx, ask, "write_plan", `{"design":"d","tasks":[]}`); !strings.HasPrefix(got, "error") {
					t.Errorf("a plan without tasks: %q", got)
				}
				run(ctx, ask, "write_plan", `{"design":"## Context\nPlan for `+slug+` token=abc123def","tasks":[{"title":"Export token=abc123def","items":["Add the button; a test clicks it","Wire the route; curl returns CSV"]}]}`)
			case strings.HasPrefix(ask.Question, "Call write_tests"):
				if !strings.Contains(ask.Data, "The plan for crm:\n## Context\nPlan for crm") || !strings.Contains(ask.Data, "- [ ] 1.2 Wire the route") {
					t.Errorf("the plans are not sent to the tests: %s", ask.Data)
				}
				for _, bad := range []string{
					`{"cases":[{"project":"shop","criteria":["AC9"],"title":"t","steps":["s"],"expected":"e"}]}`,
					`{"cases":[{"project":"shop","criteria":["AC1"],"title":"t","steps":["s"],"expected":"e"}]}`,
				} {
					if got := run(ctx, ask, "write_tests", bad); !strings.HasPrefix(got, "error") {
						t.Errorf("an unknown or missing requirement: %q", got)
					}
				}
				run(ctx, ask, "write_tests", `{"cases":[{"project":"shop","criteria":["AC1","AC3"],"title":"Admin exports","steps":["Open invoices","Tap Export"],"expected":"A CSV downloads"},
					{"project":"crm","criteria":["AC2"],"title":"Totals","steps":["Open the CSV"],"expected":"Totals match"}]}`)
			}
			return "", nil
		}
	}
	script = writer()
	if _, err := svc.WriteSpec(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	spec = settle(id)
	if spec.Status() != core.SpecReady || spec.Title != "Export invoices" || spec.Body.Change != "wo-718-add-invoice-export" || len(spec.Body.Plans) != 2 ||
		spec.Body.Plans[1].Design != "## Context\nPlan for shop token=[redacted]" ||
		!slices.Equal(spec.Body.Plans[1].Sources, []core.CodeSource{{Repo: "github.com/acme/web", Path: "src/fee.ts"}}) || len(spec.Body.Cases) != 2 {
		t.Fatalf("written %+v", spec)
	}
	md, err := svc.ExportSpec(ctx, owner, id, "shop", "https://boreas.test")
	change := "\n## openspec/changes/wo-718-add-invoice-export/"
	for _, want := range []string{
		"# wo-718: Export invoices", "openspec validate wo-718-add-invoice-export --strict", "staging at https://boreas.test/shop/wo-718/",
		change + ".openspec.yaml\n\n````yaml\nschema: spec-driven\ncreated: ",
		change + "proposal.md", "## What Changes\n\n- Admins export (decided by the team)\n- UTF-8 with a BOM (assumed by Bora)",
		"### New Capabilities\n\n- `billing/invoice-export`: Lets admins", "- `src/fee.ts` in github.com/acme/web",
		change + "specs/billing/invoice-export/spec.md\n\n````markdown\n# Spec Delta\n\n## Purpose\n\nLets admins",
		"### Requirement: Export button\nThe invoice list SHALL show Export to admins\n\n#### Scenario: TC1 Admin exports\n- **WHEN** Open invoices\n- **AND** Tap Export\n- **THEN** A CSV downloads",
		"### Requirement: No button for others", change + "design.md", "## Contract between the projects\n\nGET /invoices.csv",
		change + "tasks.md\n\n````markdown\n# Tasks\n\n## 1. Export token=[redacted]\n\n- [ ] 1.1 Add the button; a test clicks it\n- [ ] 1.2 Wire",
		"## Ticket\n\n````text\nExport invoices as CSV",
	} {
		if err != nil || !strings.Contains(md, want) {
			t.Errorf("the export lacks %q (%v):\n%s", want, err, md)
		}
	}
	if strings.Contains(md, "CSV totals") || strings.Contains(md, "TC2") || strings.Contains(md, "hunter2") || strings.Contains(md, "abc123def") {
		t.Errorf("the export holds another project's requirement or a secret:\n%s", md)
	}
	if _, err := svc.ExportSpec(ctx, owner, id, "bare", ""); !errors.Is(err, core.ErrInvalidInput) {
		t.Errorf("export of a project the spec does not plan: %v", err)
	}
	if list, err := svc.BoraSpecs(ctx, owner, "shop", "WO-718"); err != nil || len(list) != 1 {
		t.Errorf("listed %d, %v", len(list), err)
	}

	write := writer()
	script = func(ctx context.Context, ask core.Ask) (string, error) {
		if strings.HasPrefix(ask.Question, "Call write_brief") {
			return "", nil
		}
		return write(ctx, ask)
	}
	if _, err := svc.WriteSpec(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if spec = settle(id); spec.Status() != core.SpecFailed {
		t.Fatalf("a rewrite without a brief %+v", spec)
	}
	script = func(ctx context.Context, _ core.Ask) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if _, err := svc.RetrySpec(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExportSpec(ctx, owner, id, "shop", ""); !errors.Is(err, core.ErrConflict) {
		t.Errorf("export while Bora writes again: %v", err)
	}
	if err := svc.StopSpec(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if spec = settle(id); spec.Status() != core.SpecFailed || len(spec.Body.Plans) != 2 {
		t.Fatalf("stopped %+v", spec)
	}
	script = writer()
	if _, err := svc.RetrySpec(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if spec = settle(id); spec.Status() != core.SpecReady {
		t.Fatalf("retried %+v", spec)
	}
	if _, err := svc.RetrySpec(ctx, owner, id); !errors.Is(err, core.ErrConflict) {
		t.Errorf("retry of a ready spec: %v", err)
	}

	script = func(context.Context, core.Ask) (string, error) { return "", nil }
	unfinished, err := svc.StartSpec(ctx, owner, "wo-719", []string{"shop"}, "Another ticket")
	if spec = settle(unfinished.ID); err != nil || spec.Status() != core.SpecFailed {
		t.Errorf("a step the model left unfinished: %+v, %v", spec, err)
	}

	script = func(ctx context.Context, _ core.Ask) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	var running []uuid.UUID
	for _, key := range []string{"wo-1", "wo-2"} {
		spec, err := svc.StartSpec(ctx, owner, key, []string{"shop"}, "t")
		if err != nil {
			t.Fatal(err)
		}
		running = append(running, spec.ID)
	}
	if _, err := svc.StartSpec(ctx, owner, "wo-3", []string{"shop"}, "t"); !errors.Is(err, core.ErrTooMany) {
		t.Errorf("a third spec at once: %v", err)
	}
	if err := svc.DeleteBora(ctx, owner, nil); err != nil || svc.specTurn(running[0]) == nil {
		t.Errorf("deleting every conversation stopped a spec: %v", err)
	}
	var wg sync.WaitGroup
	for _, sid := range running {
		wg.Go(func() {
			_ = svc.StopSpec(ctx, owner, sid)
			settle(sid)
		})
	}
	wg.Wait()

	if err := svc.DeleteSpec(ctx, member(), id); !errors.Is(err, core.ErrForbidden) {
		t.Errorf("a stranger deleted the spec: %v", err)
	}
	if err := svc.DeleteSpec(ctx, owner, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BoraSpec(ctx, owner, id); !errors.Is(err, core.ErrNotFound) || changes.Load() == 0 {
		t.Errorf("after delete: %v, %d changes", err, changes.Load())
	}
}

func TestSpecQuestionsEval(t *testing.T) {
	url, model := os.Getenv("BOREAS_TEST_AI_BASE_URL"), os.Getenv("BOREAS_TEST_AI_MODEL")
	if url == "" || model == "" {
		t.Skip("set BOREAS_TEST_AI_BASE_URL and BOREAS_TEST_AI_MODEL to run against a real model")
	}
	a := llm.New(url, os.Getenv("BOREAS_TEST_AI_API_KEY"), model, option.WithRequestTimeout(5*time.Minute))
	svc, _ := newProjects(t)
	svc.Specs = newFakeSpecStore()
	svc.Plan = func(ctx context.Context, _ uuid.UUID, ask core.Ask) (string, []string, error) {
		reply, err := a.Answer(ctx, ask)
		return reply.Content, reply.Choices, err
	}
	ctx, owner := context.Background(), member()
	for _, slug := range []string{"shop", "crm"} {
		if _, err := svc.Create(ctx, owner, CreateProjectInput{Slug: slug}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		id, ticket string
		vague      bool
	}{
		{"V1", "Thêm chức năng xuất báo cáo doanh thu cho cửa hàng", true},
		{"V2", "Customers should be able to cancel an order", true},
		{"V3", "Cho phép khách đặt lịch hẹn lặp lại", true},
		{"C1", "Rename the button \"Save\" to \"Save changes\" on the profile page of shop. Only the label changes: same style, same action, " +
			"no translation, no API change. Done when the profile page shows \"Save changes\".", false},
	}
	passed, runs := 0, 0
	for _, c := range cases {
		for run := range 2 {
			runs++
			spec, err := svc.StartSpec(ctx, owner, "wo-1", []string{"shop", "crm"}, c.ticket)
			if err != nil {
				t.Fatal(err)
			}
			if events, _ := svc.FollowSpec(ctx, owner, spec.ID); events != nil {
				for _, e := range collect(events) {
					if e.Err != nil {
						t.Log(e.Err)
					}
				}
			}
			spec, _ = svc.BoraSpec(ctx, owner, spec.ID)
			var questions []core.SpecQuestion
			if len(spec.Body.Rounds) > 0 {
				questions = spec.Body.Rounds[0].Questions
			}
			same := !slices.ContainsFunc(questions, func(q core.SpecQuestion) bool { return language(q.Text) != language(c.ticket) })
			ok := same && (c.vague && spec.State == core.SpecAsking && len(questions) >= 2 || !c.vague && len(questions) <= 2 && spec.Status() != core.SpecFailed)
			if ok {
				passed++
			}
			t.Logf("%s#%d %v %s, %d decisions: %+v", c.id, run, ok, spec.Status(), len(spec.Body.Decisions), questions)
		}
	}
	if passed*8 < runs*7 {
		t.Errorf("passed %d/%d, want at least 7/8", passed, runs)
	}
}
