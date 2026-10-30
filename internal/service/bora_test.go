package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

func collect(events <-chan BoraEvent) []BoraEvent {
	var out []BoraEvent
	for e := range events {
		out = append(out, e)
	}
	return out
}

func eventNames(events []BoraEvent) string {
	var names []string
	for _, e := range events {
		names = append(names, e.Name)
	}
	return strings.Join(names, " ")
}

// An image reaches the model with its question and again on a follow-up, never another user's, and goes with its conversation.
func TestBoraImages(t *testing.T) {
	svc, _ := newProjects(t)
	store, objects := newFakeBoraStore(), newFakeObjects()
	svc.Conversations = store
	ctx, owner, fleet := context.Background(), member(), core.BoraAbout{Kind: "fleet"}
	if _, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "shop"}); err != nil {
		t.Fatal(err)
	}
	asks, proposed := make(chan core.Ask, 1), make(chan string, 1)
	svc.Assist = func(ctx context.Context, _ uuid.UUID, ask core.Ask) (string, []string, error) {
		asks <- ask
		for _, tool := range ask.Tools {
			if tool.Name == "propose_action" {
				proposed <- tool.Run(ctx, `{"op":"project.delete","project":"shop"}`)
			}
		}
		return "A red error.", nil, nil
	}
	if _, err := svc.UploadBoraImage(ctx, owner, "image/png", []byte("png")); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("an upload without a bucket: %v", err)
	}
	svc.Images = objects
	if _, err := svc.UploadBoraImage(ctx, owner, "text/html; charset=utf-8", []byte("<p>")); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("an upload that is no image: %v", err)
	}
	shot, err := svc.UploadBoraImage(ctx, owner, "image/png", []byte("png"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartBora(ctx, member(), fleet, "What is this?", "", shot); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("someone else asked with the image: %v", err)
	}

	events, err := svc.StartBora(ctx, owner, fleet, "What is this?", "", shot)
	if err != nil {
		t.Fatal(err)
	}
	id := collect(events)[0].ID
	if ask := <-asks; ask.Clarify || len(ask.Images) != 1 || string(ask.Images[0].Data) != "png" || ask.Images[0].MIME != "image/png" {
		t.Fatalf("first ask %+v", ask)
	}
	if got := <-proposed; !strings.Contains(got, "or an image") {
		t.Fatalf("a change proposed beside an image: %q", got)
	}
	saved, err := store.Get(ctx, owner.ID, false, id)
	if err != nil || !slices.Equal(saved.Messages[0].Images, []uuid.UUID{shot}) || saved.Messages[1].Images == nil {
		t.Fatalf("saved %+v, %v", saved, err)
	}

	events, err = svc.AskBora(ctx, owner, id, "And the bottom left?", "")
	if err != nil {
		t.Fatal(err)
	}
	collect(events)
	<-proposed
	if ask := <-asks; ask.Clarify || len(ask.Images) != 1 {
		t.Fatalf("a follow-up lost the image: %+v", ask)
	}
	if err := svc.DeleteBora(ctx, owner, &id); err != nil || !slices.Equal(objects.deleted, []string{imageKey(owner.ID, shot)}) {
		t.Fatalf("delete: %v, removed %v", err, objects.deleted)
	}
}

func TestBoraTurns(t *testing.T) {
	svc, tasks := newProjects(t)
	store := newFakeBoraStore()
	svc.Conversations = store
	ctx, owner := context.Background(), member()
	shop, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: shop.ID, Name: "web", Status: core.StatusRunning, Description: "storefront"})
	asks, answers := make(chan core.Ask, 1), make(chan string)
	svc.Assist = func(ctx context.Context, _ uuid.UUID, ask core.Ask) (string, []string, error) {
		asks <- ask
		ask.Delta("Looking. ")
		ask.Reset()
		for _, tool := range ask.Tools {
			if tool.Name == "list_tasks" {
				tool.Run(ctx, `{"project":"shop"}`)
			}
		}
		ask.Delta(" <say>Web refused DB_PASS")
		ask.Delta("WORD=hunter2.</say>\nWeb ")
		select {
		case answer := <-answers:
			return answer, nil, nil
		case <-ctx.Done():
			return "", nil, ctx.Err()
		}
	}
	// Redacted, this line outgrows the 1000 characters a subject may send, which must not fail a follow-up.
	line := "DB_PASSWORD=hunter2 refused " + strings.Repeat(".", 972)
	about := core.BoraAbout{Kind: "log", Project: "shop", Task: "web", At: "2026-10-12T08:00:00Z", Line: line}

	// The phone locks: the first stream drops, and the turn runs on.
	gone, drop := context.WithCancel(ctx)
	events, err := svc.StartBora(gone, owner, about, "  Why did web fail?  ", "/projects/shop")
	if err != nil {
		t.Fatal(err)
	}
	id := (<-events).ID
	drop()
	ask := <-asks
	if !ask.Clarify || len(ask.History) != 0 || !strings.Contains(ask.Data, "The log line, at 2026-10-12T08:00:00Z") ||
		!strings.Contains(ask.Data, "- shop/web: running, about: storefront") || strings.Contains(ask.Brief, "storefront") || strings.Contains(ask.Brief+ask.Data, "hunter2") ||
		!slices.ContainsFunc(ask.Tools, func(tool core.Tool) bool { return tool.Name == "set_title" }) {
		t.Fatalf("first ask %+v", ask)
	}
	pending, err := svc.BoraConversation(ctx, owner, id)
	if err != nil || pending.Answering == nil || pending.Answering.Question != "Why did web fail?" ||
		pending.Title != "shop/web: Why did web fail?" || len(pending.Messages) != 0 {
		t.Fatalf("a conversation on its first turn: %+v, %v", pending, err)
	}
	if _, err := svc.AskBora(ctx, owner, id, "Again?", ""); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("a second ask while a turn runs: %v", err)
	}
	again := svc.FollowBora(ctx, owner, id)
	answers <- "<say>Web refused DB_PASSWORD=hunter2.</say>\nWeb is down: DB_PASSWORD=hunter2 was refused." + strings.Repeat(" More.", 700)
	got := collect(again)
	done := got[len(got)-1]
	if eventNames(got) != "conversation delta reset step say delta done" || got[3].Text != "Looking at the tasks of shop…" ||
		got[4].Text != "Web refused DB_PASSWORD=[redacted]" || done.Message.Say != got[4].Text || got[5].Text != "Web " ||
		done.Title != "shop/web: Why did web fail?" || done.Message.Index != 1 ||
		!strings.HasPrefix(done.Message.Content, "Web is down: DB_PASSWORD=[redacted] was refused.") || utf8.RuneCountInString(done.Message.Content) != answerRunes {
		t.Fatalf("replayed %s: %+v", eventNames(got), got)
	}
	if svc.FollowBora(ctx, owner, id) != nil {
		t.Fatal("an answered turn still runs")
	}
	saved, err := store.Get(ctx, owner.ID, false, id)
	if err != nil || len(saved.Messages) != 2 || saved.Messages[0].Content != "Why did web fail?" || strings.Contains(saved.About.Line, "hunter2") ||
		saved.ProjectID == nil || *saved.ProjectID != shop.ID || !slices.Equal(saved.Projects, []uuid.UUID{shop.ID}) {
		t.Fatalf("saved %+v, %v", saved, err)
	}

	for _, tc := range []struct {
		user  core.User
		index int
		line  string
		want  error
	}{
		{owner, 0, "Task web restarted.", core.ErrInvalidInput},
		{owner, 1, " ", core.ErrInvalidInput},
		{member(), 1, "Task web restarted.", core.ErrNotFound},
		{owner, 1, " Task web restarted. ", nil},
	} {
		if err := svc.AddBoraRan(ctx, tc.user, id, tc.index, tc.line); !errors.Is(err, tc.want) {
			t.Errorf("ran %d %q: %v, want %v", tc.index, tc.line, err, tc.want)
		}
	}
	events, err = svc.AskBora(ctx, owner, id, "And now?", "")
	if err != nil {
		t.Fatal(err)
	}
	ask = <-asks
	if len(ask.History) != 2 || !ask.Clarify || !strings.Contains(ask.Brief, "- Task web restarted.") ||
		slices.ContainsFunc(ask.Tools, func(tool core.Tool) bool { return tool.Name == "set_title" }) {
		t.Fatalf("follow-up ask %+v", ask)
	}
	svc.StopBora(owner, id)
	if got := collect(events); eventNames(got) != "delta reset step say delta stopped" {
		t.Fatalf("a stopped turn sent %s", eventNames(got))
	}
	if saved, _ := store.Get(ctx, owner.ID, false, id); len(saved.Messages) != 2 {
		t.Fatalf("a stopped turn saved %d messages", len(saved.Messages))
	}

	svc.Assist = func(ctx context.Context, _ uuid.UUID, ask core.Ask) (string, []string, error) {
		for _, tool := range ask.Tools {
			if tool.Name == "set_title" {
				tool.Run(ctx, `{"title":"  Lỗi   web  token=abc123def "}`)
				tool.Run(ctx, `{"title":"second"}`)
			}
		}
		return "ok", nil, nil
	}
	events, err = svc.StartBora(ctx, owner, core.BoraAbout{Kind: "project", Project: "shop"}, "hi", "")
	got = collect(events)
	if err != nil || got[len(got)-1].Title != "Lỗi web token=[redacted]" {
		t.Fatalf("named %+v, %v", got, err)
	}
	if saved, _ := store.Get(ctx, owner.ID, false, got[0].ID); !slices.Equal(saved.Projects, []uuid.UUID{shop.ID}) {
		t.Fatalf("a project's conversation that read nothing records %v", saved.Projects)
	}
}

func TestBoraHistory(t *testing.T) {
	svc, _ := newProjects(t)
	store := newFakeBoraStore()
	svc.Conversations = store
	ctx, owner := context.Background(), member()
	if _, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "shop"}); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	var messages []core.BoraMessage
	for i := range 14 {
		m := boraMessage(i, core.ChatUser, fmt.Sprint("q", i), time.Now())
		if i%2 == 1 {
			m.Role, m.Ran = core.ChatAssistant, []string{fmt.Sprint("ran ", i)}
		}
		messages = append(messages, m)
	}
	messages[13].Choices = []string{"shop/web", "Not sure: answer with your best guess"}
	store.convs[id] = core.BoraConversation{ID: id, UserID: owner.ID, About: core.BoraAbout{Kind: "fleet"}, Messages: messages}
	var ask core.Ask
	svc.Assist = func(ctx context.Context, _ uuid.UUID, a core.Ask) (string, []string, error) {
		ask = a
		for _, tool := range a.Tools {
			if tool.Name == "propose_action" {
				tool.Run(ctx, `{"op":"task.restart","project":"shop","task":"web","reason":"stuck"}`)
			}
		}
		return "Which one?", []string{"a", "b"}, nil
	}
	events, err := svc.AskBora(ctx, owner, id, "Not sure: answer with your best guess", "")
	if err != nil {
		t.Fatal(err)
	}
	got := collect(events)
	done := got[len(got)-1]
	if len(ask.History) != 12 || ask.History[0].Content != "q2" || ask.Clarify || strings.Count(ask.Brief, "- ran ") != 7 {
		t.Fatalf("ask %+v", ask)
	}
	if eventNames(got) != "say done" || done.Message.Say != "Which one?" || done.Message.Index != 15 || !slices.Equal(done.Message.Choices, []string{"a", "b"}) ||
		done.Message.Actions == nil || len(done.Message.Actions) != 0 || done.Title != "" {
		t.Fatalf("done %+v", done)
	}
}

func TestBoraGateCountsDetachedTurns(t *testing.T) {
	svc, _ := newProjects(t)
	svc.Conversations = newFakeBoraStore()
	ctx, owner, other := context.Background(), member(), member()
	for _, user := range []core.User{owner, other} {
		if _, err := svc.Create(ctx, user, CreateProjectInput{Slug: "p" + user.ID.String()[:8]}); err != nil {
			t.Fatal(err)
		}
	}
	svc.Assist = func(ctx context.Context, _ uuid.UUID, _ core.Ask) (string, []string, error) {
		<-ctx.Done()
		return "", nil, ctx.Err()
	}
	fleet := core.BoraAbout{Kind: "fleet"}
	events, err := svc.StartBora(ctx, owner, fleet, "q", "")
	if err != nil {
		t.Fatal(err)
	}
	first := (<-events).ID
	gone, drop := context.WithCancel(ctx)
	drop()
	for range turnsPerUser - 1 {
		if _, err := svc.StartBora(gone, owner, fleet, "q", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.StartBora(ctx, owner, fleet, "q", ""); !errors.Is(err, core.ErrTooMany) {
		t.Fatalf("a fourth turn: %v", err)
	}
	if _, err := svc.StartBora(gone, other, fleet, "q", ""); err != nil {
		t.Fatalf("another user was refused: %v", err)
	}
	if err := svc.DeleteBora(ctx, owner, &first); err != nil {
		t.Fatalf("deleting a conversation on its first turn: %v", err)
	}
	if got := collect(events); eventNames(got) != "stopped" {
		t.Fatalf("a deleted conversation's turn sent %s", eventNames(got))
	}
	if err := svc.DeleteBora(ctx, owner, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := svc.StartBora(gone, owner, fleet, "q", ""); err == nil {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("stopped turns kept their places: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	svc.StopAllBora()
}

func TestBoraCutsLogsAtLines(t *testing.T) {
	body := strings.Repeat("y", logTail-11) + "\n"
	if got := cut("DB_PASSWORD=supersecret\n" + body); got != body {
		t.Errorf("kept %q…, want only whole lines", got[:30])
	}
	if got := cut("a\nb\n"); got != "a\nb\n" {
		t.Errorf("short logs cut to %q", got)
	}
}

func TestBoraCode(t *testing.T) {
	svc, tasks := newProjects(t)
	svc.Code = fakeCode{contexts: map[string]string{"shop": "github.com/acme/web", "crm": "github.com/acme/crm"}}
	ctx, owner := context.Background(), member()
	var shop core.Project
	for _, slug := range []string{"shop", "crm", "bare"} {
		project, err := svc.Create(ctx, owner, CreateProjectInput{Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		if slug == "shop" {
			shop = project
		}
	}
	if _, err := svc.Create(ctx, admin(), CreateProjectInput{Slug: "vault"}); err != nil {
		t.Fatal(err)
	}
	head, before := strings.Repeat("a", 40), strings.Repeat("b", 40)
	seedTask(t, tasks, core.Task{ProjectID: shop.ID, Name: "web", Revision: core.Revision{Repo: "github.com/acme/web", Commit: head, Previous: before}})
	run := func(b *bora, name, args string) string {
		tools := b.tools(false)
		if i := slices.IndexFunc(tools, func(tool core.Tool) bool { return tool.Name == name }); i >= 0 {
			return tools[i].Run(ctx, args)
		}
		return "not offered"
	}
	as := func(user core.User, about core.BoraAbout) *bora {
		b, err := svc.newBora(ctx, user, about, false)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	fleet := as(owner, core.BoraAbout{Kind: "fleet"})
	web := `"project":"shop","repo":"github.com/acme/web"`
	for _, tc := range []struct{ name, args, want string }{
		{"search_code", `{"project":"","query":"fee"}`, "name the project"},
		{"search_code", `{"project":"vault","query":"fee"}`, "not a member of a project named vault"},
		{"search_code", `{"project":"bare","query":"fee"}`, "Sourcebot has no search context named bare"},
		{"list_tasks", `{"project":"bare"}`, "Bora does not answer about bare"},
		{"search_code", `{"project":"shop","query":"fee"}`, "github.com/acme/web src/fee.ts:7"},
		{"read_file", `{"project":"shop","repo":"github.com/acme/crm","path":"src/fee.ts","ref":""}`, "not part of shop's code"},
		{"read_file", `{` + web + `,"path":"src/fee.ts","ref":""}`, "fee = total * 0.02"},
		{"code_history", `{` + web + `,"ref":"HEAD..` + head + `","path":"","query":""}`, "c0ffee00c0ff 2026-10-28 Dev: feat(MYL-1): fee at HEAD..aaaa"},
		{"code_history", `{` + web + `,"ref":"","path":"","query":""}`, "… and 30 older commits"},
		{"code_diff", `{` + web + `,"base":"","head":"x","path":""}`, "needs base and head"},
		{"code_history", `{"project":"shop","repo":"","ref":"","path":"","query":""}`, "name the repository"},
		{"code_diff", `{` + web + `,"base":"` + before + `","head":"` + head + `","path":""}`, "31 commits:"},
		{"blame", `{` + web + `,"path":"src/fee.ts","ref":""}`, "lines 1-3: c0ffee00c0ff 2026-10-28 Dev: feat(MYL-1): fee"},
		{"task_details", `{"project":"shop","task":"web"}`, "runs commit " + head + " of github.com/acme/web; the deploy before ran " + before},
	} {
		if got := run(fleet, tc.name, tc.args); !strings.Contains(got, tc.want) {
			t.Errorf("%s %s: %q", tc.name, tc.args, got)
		}
	}
	diff := run(fleet, "code_diff", `{`+web+`,"base":"`+before+`","head":"`+head+`","path":""}`)
	if fee, lock := strings.Index(diff, "+fee = 2"), strings.Index(diff, "+lock"); !strings.Contains(diff, "2 files changed:\nM package-lock.json (+5000 -0)\nM src/fee.ts (+1 -1)\n") ||
		fee < 0 || lock < fee || len(diff) > diffText+100 || !strings.HasSuffix(diff, "… cut: pass path for one file\n") {
		t.Errorf("the small change goes first and the lockfile is cut: %d bytes\n%.600s", len(diff), diff)
	}
	if brief := fleet.brief("", nil); !strings.Contains(brief, "does not answer about yet, as Sourcebot has no search context named after them: bare.") ||
		strings.Contains(brief, "bare (") {
		t.Errorf("the brief must name bare as not served, and list it with no project: %s", brief)
	}
	if !fleet.readsCode() || !fleet.tainted || !slices.Equal(fleet.sources, []core.CodeSource{{Repo: "github.com/acme/web", Path: "src/fee.ts"}}) {
		t.Errorf("after reading code: tainted %v, sources %v", fleet.tainted, fleet.sources)
	}
	if got := run(as(owner, core.BoraAbout{Kind: "task", Project: "shop", Task: "web"}), "search_code", `{"project":"","query":"fee"}`); !strings.Contains(got, "src/fee.ts") {
		t.Errorf("the subject's project by default: %q", got)
	}
	if got := run(as(admin(), core.BoraAbout{Kind: "fleet"}), "search_code", `{"project":"shop","query":"fee"}`); !strings.Contains(got, "src/fee.ts") {
		t.Errorf("an admin: %q", got)
	}
	if _, err := svc.newBora(ctx, owner, core.BoraAbout{Kind: "project", Project: "bare"}, false); !errors.Is(err, core.ErrForbidden) {
		t.Errorf("a project without a search context: %v", err)
	}
	accesses, _, err := svc.Fleet(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for i, served := range svc.BoraServes(ctx, accesses) {
		if slug := accesses[i].Project.Slug; served != (slug != "bare") {
			t.Errorf("Bora serves %s: %v", slug, served)
		}
	}
	if _, err := svc.newBora(ctx, member(), core.BoraAbout{Kind: "fleet"}, false); !errors.Is(err, core.ErrForbidden) {
		t.Errorf("a member of no project: %v", err)
	}
	svc.Code = fakeCode{contexts: map[string]string{"shop": "github.com/acme/web"}, fail: errors.New("sourcebot /api/search: 403 Forbidden DEPLOYMENT_RESTRICTED")}
	if got := run(fleet, "search_code", `{"project":"shop","query":"fee"}`); got != "error: sourcebot /api/search: 403 Forbidden DEPLOYMENT_RESTRICTED" {
		t.Errorf("what Sourcebot said is lost: %q", got)
	}
	svc.Code = nil
	if b := as(owner, core.BoraAbout{Kind: "project", Project: "bare"}); b.readsCode() || run(b, "search_code", `{"project":"bare","query":"fee"}`) != "not offered" {
		t.Error("without Sourcebot Bora serves every project, with no code tools")
	}
}

func TestBoraProposeChecks(t *testing.T) {
	svc, tasks := newProjects(t)
	users := newFakeUserStore()
	svc.Users = users
	ctx := context.Background()
	add := func(name string) core.User {
		user, _ := users.Create(ctx, core.User{Username: name, Role: core.RoleUser})
		return user
	}
	owner, boss, vic, gil, dee := add("olive"), admin(), add("vic"), add("gil"), add("dee")
	add("otto")
	dee.DisabledAt = &time.Time{}
	users.users[dee.ID] = dee
	shop, _ := svc.Create(ctx, owner, CreateProjectInput{Slug: "shop", DefaultImage: "nginx"})
	bare, _ := svc.Create(ctx, owner, CreateProjectInput{Slug: "bare"})
	seedTask(t, tasks, core.Task{ProjectID: shop.ID, Name: "web"})
	seedTask(t, tasks, core.Task{ProjectID: shop.ID, Name: "api"})
	seedTask(t, tasks, core.Task{ProjectID: bare.ID, Name: "x"})
	if err := errors.Join(svc.AddMember(ctx, "shop", vic.ID, core.ProjectRoleViewer), svc.Grant(ctx, "shop", "api", vic.ID, core.ProjectRoleOperator),
		svc.Grant(ctx, "shop", "web", gil.ID, core.ProjectRoleOperator), svc.Grant(ctx, "shop", "web", dee.ID, core.ProjectRoleViewer)); err != nil {
		t.Fatal(err)
	}
	credential, _ := svc.credentials.Create(ctx, core.RegistryCredential{Name: "ghcr-ci"})

	as := func(user core.User) *bora {
		b, err := svc.newBora(ctx, user, core.BoraAbout{Kind: "fleet"}, false)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	say := func(b *bora, in proposal) string {
		out, err := b.propose(ctx, in)
		if err != nil {
			return err.Error()
		}
		return out
	}
	o, v, a := as(owner), as(vic), as(boss)
	no := false
	for _, tc := range []struct {
		who  *bora
		in   proposal
		want string
	}{
		{o, proposal{Op: "nope", Project: "shop"}, "op must be one of"},
		{o, proposal{Op: "task.start", Project: "crm", Task: "web"}, "not a member of a project named crm"},
		{o, proposal{Op: "task.start", Project: "shop", Task: "bad name!"}, "not a task name"},
		{o, proposal{Op: "task.start", Project: "shop", Task: "ghost"}, "no task ghost in shop"},
		{v, proposal{Op: "task.restart", Project: "shop", Task: "web"}, "you are viewer on shop/web"},
		{v, proposal{Op: "Task.Restart", Project: "shop", Task: "api", Reason: " stuck, token=abc123def "}, "proposed"},
		{v, proposal{Op: "task.restart", Project: "shop", Task: "api"}, "already has a button"},
		{v, proposal{Op: "task.delete", Project: "shop", Task: "api"}, "needs member"},
		{o, proposal{Op: "task.create", Project: "shop", Task: "web"}, "already exists"},
		{v, proposal{Op: "task.create", Project: "shop", Task: "new"}, "you are viewer on shop/new"},
		{o, proposal{Op: "task.create", Project: "bare", Task: "new"}, "no default image"},
		{o, proposal{Op: "task.create", Project: "bare", Task: "new", Image: " nginx:alpine ", Port: 8080}, "proposed"},
		{o, proposal{Op: "task.update", Project: "bare", Task: "x", AutoRestart: &no}, "needs a description"},
		{o, proposal{Op: "task.update", Project: "bare", Task: "x", Volumes: map[string]string{}}, "needs a description"},
		{o, proposal{Op: "task.update", Project: "bare", Task: "x", DevStatus: "done"}, "dev_status must be"},
		{o, proposal{Op: "task.update", Project: "bare", Task: "x", Port: 70000}, "port must be"},
		{o, proposal{Op: "task.update", Project: "bare", Task: "x", Volumes: map[string]string{"data": "certs"}}, "volumes map"},
		{o, proposal{Op: "task.update", Project: "bare", Task: "x", Image: "nginx:alpine", AutoRestart: &no, Set: map[string]string{"A": "1"}, Volumes: map[string]string{}}, "proposed"},
		{o, proposal{Op: "task.env", Project: "bare", Task: "x"}, "name variables"},
		{o, proposal{Op: "task.env", Project: "bare", Task: "x", Set: map[string]string{"bad-key": "1"}}, "not a variable name"},
		{o, proposal{Op: "task.env", Project: "bare", Task: "x", Set: map[string]string{"BOREAS_TASK": "1"}}, "Boreas sets"},
		{o, proposal{Op: "task.env", Project: "bare", Task: "x", Set: map[string]string{"A": "1"}, Unset: []string{"A"}}, "both set and unset"},
		{v, proposal{Op: "member.add", Project: "shop", Username: "otto", Role: "viewer"}, "you are viewer on shop; member.add needs owner"},
		{o, proposal{Op: "member.add", Project: "shop", Username: "otto", Role: "viewer"}, "no one named otto is in shop"},
		{o, proposal{Op: "member.add", Project: "shop", Username: "nobody", Role: "viewer"}, "no one named nobody is in shop"},
		{o, proposal{Op: "member.add", Project: "shop", Username: "dee", Role: "viewer"}, "no one named dee is in shop"},
		{o, proposal{Op: "member.add", Project: "shop", Username: "vic", Role: "viewer"}, "already a member"},
		{o, proposal{Op: "member.add", Project: "shop", Username: "gil"}, "role must be"},
		{o, proposal{Op: "member.add", Project: "shop", Username: "GIL", Role: "operator"}, "proposed"},
		{o, proposal{Op: "member.role", Project: "shop", Username: "gil", Role: "owner"}, "gil is not a member"},
		{o, proposal{Op: "member.role", Project: "shop", Username: "vic", Role: "operator"}, "proposed"},
		{o, proposal{Op: "member.remove", Project: "shop", Username: "vic"}, "proposed"},
		{o, proposal{Op: "grant.add", Project: "shop", Task: "web", Username: "vic", Role: "owner"}, "role must be viewer, operator or member"},
		{o, proposal{Op: "grant.add", Project: "shop", Task: "web", Username: "vic", Role: "member"}, "proposed"},
		{o, proposal{Op: "grant.remove", Project: "shop", Task: "api", Username: "gil"}, "gil holds no grant on api"},
		{o, proposal{Op: "grant.remove", Project: "shop", Task: "web", Username: "gil"}, "proposed"},
		{o, proposal{Op: "project.update", Project: "shop"}, "needs a name"},
		{o, proposal{Op: "project.update", Project: "bare", Name: "Bare", Set: map[string]string{}}, "proposed"},
		{o, proposal{Op: "project.update", Project: "shop", RegistryCredential: "ghcr-ci"}, "only an admin"},
		{a, proposal{Op: "project.update", Project: "shop", RegistryCredential: "nope"}, "no registry credential named nope"},
		{a, proposal{Op: "project.update", Project: "shop", Name: "Shop", Port: 8080, Set: map[string]string{"LOG_LEVEL": "debug"}, RegistryCredential: "GHCR-CI"}, "proposed"},
		{a, proposal{Op: "member.add", Project: "shop", Username: "nobody", Role: "viewer"}, "no active user named nobody"},
		{a, proposal{Op: "member.add", Project: "shop", Username: "otto", Role: "viewer"}, "proposed"},
		{v, proposal{Op: "project.delete", Project: "shop"}, "needs owner"},
		{o, proposal{Op: "project.delete", Project: "shop"}, "proposed"},
	} {
		if got := say(tc.who, tc.in); !strings.Contains(got, tc.want) {
			t.Errorf("%+v: %q, want %q", tc.in, got, tc.want)
		}
	}
	find := func(actions []core.BoraAction, op core.BoraOp) core.BoraAction {
		return actions[slices.IndexFunc(actions, func(x core.BoraAction) bool { return x.Op == op })]
	}
	if got := find(v.actions, "task.restart"); got.Reason != "stuck, token=[redacted]" || got.Task != "api" {
		t.Errorf("restart %+v", got)
	}
	if got := find(o.actions, "task.create"); got.Image != "nginx:alpine" || got.Port != 8080 {
		t.Errorf("create %+v", got)
	}
	update := find(o.actions, "task.update")
	if raw, _ := json.Marshal(update); update.Image != "nginx:alpine" || update.AutoRestart == nil || *update.AutoRestart || update.Set != nil || strings.Contains(string(raw), "volumes") {
		t.Errorf("update %s", raw)
	}
	if got := find(o.actions, "member.add"); got.UserID != gil.ID || got.Username != "gil" || got.Role != core.ProjectRoleOperator {
		t.Errorf("member.add %+v", got)
	}
	if got := find(o.actions, "member.remove"); got.UserID != vic.ID || got.Role != "" {
		t.Errorf("member.remove %+v", got)
	}
	if got := find(a.actions, "project.update"); got.Name != "Shop" || got.DefaultPort != 8080 || got.Port != 0 ||
		got.DefaultEnvSet["LOG_LEVEL"] != "debug" || got.RegistryCredentialID == nil || *got.RegistryCredentialID != credential.ID {
		t.Errorf("project.update %+v", got)
	}
	for len(o.actions) < maxActions {
		o.actions = append(o.actions, core.BoraAction{})
	}
	if got := say(o, proposal{Op: "task.stop", Project: "bare", Task: "x"}); !strings.Contains(got, "at most") {
		t.Errorf("past the cap: %q", got)
	}
	o.tainted = true
	if got := say(o, proposal{Op: "task.update", Project: "bare", Task: "x", Note: "n"}); !strings.Contains(got, "after reading code") {
		t.Errorf("an update after reading code: %q", got)
	}
	if got := say(o, proposal{Op: "task.stop", Project: "bare", Task: "x"}); strings.Contains(got, "after reading code") {
		t.Errorf("a stop after reading code: %q", got)
	}
}

func TestBoraScope(t *testing.T) {
	svc, tasks := newProjects(t)
	store := newFakeBoraStore()
	svc.Conversations = store
	ctx, owner, grantee := context.Background(), member(), member()
	shop, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "shop", DefaultImage: "nginx", DefaultEnv: map[string]string{"API_TOKEN": "s3cret"}})
	if err != nil {
		t.Fatal(err)
	}
	vault, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "vault"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: shop.ID, Name: "web", Status: core.StatusRunning, Description: "storefront", Env: map[string]string{"DB_PASSWORD": "hunter2"}})
	seedTask(t, tasks, core.Task{ProjectID: vault.ID, Name: "safe"})
	if err := svc.Grant(ctx, "vault", "safe", grantee.ID, core.ProjectRoleOperator); err != nil {
		t.Fatal(err)
	}
	event := func(project core.Project, task, title string) core.Notification {
		n, _ := svc.notifications.Create(ctx, core.Notification{ProjectID: project.ID, Project: project.Slug, TaskName: task, Type: "deploy_failed", Title: title, Body: "boom"})
		return n
	}
	alert := event(shop, "web", "Deploy failed")
	hidden := event(vault, "safe", "Vault failed")
	ask := func(user core.User, about core.BoraAbout) (BoraEvent, error) {
		events, err := svc.StartBora(ctx, user, about, "Why?", "/projects")
		if err != nil {
			return BoraEvent{}, err
		}
		got := collect(events)
		return got[len(got)-1], nil
	}
	if _, err := ask(owner, core.BoraAbout{Kind: "fleet"}); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("no assistant: %v", err)
	}
	var asked core.Ask
	svc.Assist = func(_ context.Context, _ uuid.UUID, a core.Ask) (string, []string, error) {
		asked = a
		for _, tool := range a.Tools {
			if tool.Name == "propose_action" {
				tool.Run(ctx, `{"op":"task.restart","project":"shop","task":"web","reason":"stuck"}`)
			}
		}
		return "answer", nil, nil
	}
	var logOptions core.LogOptions
	svc.Logs = func(_ context.Context, _, _ string, opts core.LogOptions) (string, error) {
		logOptions = opts
		return "a line\n", nil
	}
	svc.Metrics = func(ctx context.Context, _ core.ProjectAccess, _ string) (<-chan core.TaskMetric, error) {
		// Two replayed samples from the last minute come before the live one Bora must report.
		ago := time.Now().Add(-30 * time.Second)
		samples := make(chan core.TaskMetric, 3)
		samples <- core.TaskMetric{TaskName: "web", CPUPercent: 50, ObservedAt: ago}
		samples <- core.TaskMetric{TaskName: "web", CPUPercent: 60, ObservedAt: ago.Add(time.Second)}
		samples <- core.TaskMetric{TaskName: "web", CPUPercent: 12.5, MemoryBytes: 64 << 20, ObservedAt: time.Now()}
		go func() {
			<-ctx.Done()
			close(samples)
		}()
		return samples, nil
	}
	if _, err := ask(grantee, core.BoraAbout{Kind: "fleet"}); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("a user with grants only: %v", err)
	}
	if err := svc.AddMember(ctx, "shop", grantee.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		about core.BoraAbout
		want  error
	}{
		{core.BoraAbout{Kind: "project", Project: "vault"}, core.ErrForbidden},
		{core.BoraAbout{Kind: "project", Project: "nope"}, core.ErrNotFound},
		{core.BoraAbout{Kind: "task", Project: "shop", Task: "ghost"}, core.ErrNotFound},
		{core.BoraAbout{Kind: "alert", Project: "shop", ID: uuid.New()}, core.ErrNotFound},
		{core.BoraAbout{Kind: "alert", Project: "shop", ID: hidden.ID}, core.ErrNotFound},
		{core.BoraAbout{Kind: "log", Project: "shop", Task: "web"}, core.ErrInvalidInput},
		{core.BoraAbout{Kind: "weird"}, core.ErrInvalidInput},
		{core.BoraAbout{Kind: "fleet", Project: "shop"}, core.ErrInvalidInput},
		{core.BoraAbout{Kind: "task", Project: "shop", Task: "web", Line: "x"}, core.ErrInvalidInput},
	} {
		if _, err := ask(grantee, tc.about); !errors.Is(err, tc.want) {
			t.Errorf("%+v: %v, want %v", tc.about, err, tc.want)
		}
	}

	done, err := ask(grantee, core.BoraAbout{Kind: "alert", Project: "shop", ID: alert.ID})
	if err != nil || done.Name != "done" || done.Message.Content != "answer" || len(done.Message.Actions) != 0 {
		t.Fatalf("a viewer's answer %+v, err %v", done, err)
	}
	if !strings.Contains(asked.Data, "Deploy failed") || !strings.Contains(asked.Data, "- shop/web: running, about: storefront") ||
		strings.Contains(asked.Brief, "Deploy failed") || strings.Contains(asked.Brief, "storefront") ||
		strings.Contains(asked.Brief+asked.Data, "vault") || strings.Contains(asked.Brief+asked.Data, "hunter2") {
		t.Fatalf("brief %q, data %q", asked.Brief, asked.Data)
	}
	run := func(name, args string) string {
		i := slices.IndexFunc(asked.Tools, func(t core.Tool) bool { return t.Name == name })
		if i < 0 {
			return "not offered"
		}
		return asked.Tools[i].Run(ctx, args)
	}
	for _, tc := range []struct{ tool, args, want, never string }{
		{"list_projects", `{}`, "shop (shop): you are viewer, 1 tasks", "vault"},
		{"list_tasks", `{"project":""}`, "web", "safe"},
		{"task_details", `{"project":"shop","task":"web"}`, "env names: DB_PASSWORD", "hunter2"},
		{"project_settings", `{"project":"shop"}`, "default env names: API_TOKEN", "s3cret"},
		{"activity", `{"project":"","limit":1}`, "Deploy failed", "Vault failed"},
		{"task_logs", `{"project":"shop","task":"web","tail":0,"since":"2026-10-11T08:00:00Z"}`, "a line", "error"},
		{"members", `{"project":"shop"}`, "not offered", ""},
	} {
		if got := run(tc.tool, tc.args); !strings.Contains(got, tc.want) || tc.never != "" && strings.Contains(got, tc.never) {
			t.Errorf("%s: %q", tc.tool, got)
		}
	}
	if logOptions.Tail != 200 || logOptions.Since.IsZero() {
		t.Errorf("task_logs read %+v", logOptions)
	}
	started := time.Now()
	if got := run("metrics", `{"project":"shop","task":"web"}`); !strings.Contains(got, "web: CPU 12.5%, memory 64 MiB") || time.Since(started) > time.Second {
		t.Errorf("metrics %q after %s", got, time.Since(started))
	}

	done, err = ask(owner, core.BoraAbout{Kind: "task", Project: "shop", Task: "web"})
	if err != nil || len(done.Message.Actions) != 1 || done.Message.Actions[0].Op != "task.restart" {
		t.Fatalf("an owner's answer %+v, err %v", done, err)
	}
	if got := run("members", `{"project":"shop"}`); !strings.Contains(got, "owner") {
		t.Errorf("an owner's members: %q", got)
	}
	if err := svc.Grant(ctx, "shop", "web", member().ID, core.ProjectRoleOperator); err != nil {
		t.Fatal(err)
	}
	if got := run("grants", `{"project":"shop","task":"web"}`); !strings.Contains(got, "operator") {
		t.Errorf("an owner's grants: %q", got)
	}
	if got := run("list_tasks", `{"project":"vault"}`); !strings.Contains(got, "not a member of a project named vault") || strings.Contains(asked.Data, "safe") {
		t.Errorf("a task's conversation reached another project: %q", got)
	}

	svc.Assist = func(_ context.Context, _ uuid.UUID, a core.Ask) (string, []string, error) {
		asked = a
		run("task_details", `{"project":"shop","task":"web"}`)
		return "answer", nil, nil
	}
	events, err := svc.StartBora(ctx, owner, core.BoraAbout{Kind: "fleet"}, "Why?", "")
	if err != nil {
		t.Fatal(err)
	}
	id := (<-events).ID
	collect(events)
	if saved, err := store.Get(ctx, owner.ID, false, id); err != nil || asked.Data != "" || !strings.Contains(asked.ClarifyData, "vault/safe") ||
		!slices.Equal(saved.Projects, []uuid.UUID{shop.ID}) {
		t.Errorf("fleet: data %q, screening data %q, projects %v, err %v", asked.Data, asked.ClarifyData, saved.Projects, err)
	}
}

func TestBoraSpoken(t *testing.T) {
	sentence := strings.Repeat("a", 99) + "."
	for in, want := range map[string]string{
		"**Web** is [down](https://x.io). Run `npm ci`!\n- then retry.":   "Web is down. Run npm ci!",
		strings.Repeat(sentence+" ", 4):                                   sentence + " " + sentence,
		"v1.2 failed: DB_PASSWORD=hunter2 refused":                        "v1.2 failed: DB_PASSWORD=[redacted] refused",
		strings.Repeat("word ", 80):                                       strings.TrimSpace(strings.Repeat("word ", 60)),
		"Lúc 15:30 có 3 task lỗi, CPU 35%.":                               "Lúc mười lăm giờ ba mươi có ba task lỗi, CPU ba mươi lăm phần trăm.",
		"Đã chạy lúc 08:05 và 15h00.":                                     "Đã chạy lúc tám giờ năm phút và mười lăm giờ.",
		"Còn 1.250, 1.050, 1.005, 105, 21, 15, 11 và 2.000.000.":          "Còn một nghìn hai trăm năm mươi, một nghìn không trăm năm mươi, một nghìn không trăm linh năm, một trăm linh năm, hai mươi mốt, mười lăm, mười một và hai triệu.",
		"Tăng 2,5 rồi 2.5, bản v1.2.3 dùng 2GB.":                          "Tăng hai phẩy năm rồi hai chấm năm, bản một chấm hai chấm ba dùng hai GB.",
		"Task api-gateway và billing_worker đã chạy.":                     "Task api gateway và billing worker đã chạy.",
		"The api-gateway restarted 3 times at 15:30.":                     "The api-gateway restarted 3 times at 15:30.",
		"Api gateway bị lỗi. Restarted 3 times.":                          "Api gateway bị lỗi. Restarted ba times.",
		"Build mất 2m14s, chạy lúc 15h, xong 15h30p, chờ 1h05m và 500ms.": "Build mất hai phút mười bốn giây, chạy lúc mười lăm giờ, xong mười lăm giờ ba mươi phút, chờ một giờ năm phút và năm trăm mi li giây.",
		"Màn hình 1920x1080, phiên bản v3.0 và v2.":                       "Màn hình một nghìn chín trăm hai mươi x một nghìn không trăm tám mươi, phiên bản ba chấm không và phiên bản hai.",
		"Lỗi từ 2026-10-18 và 2026-04-01 trên k8s.":                       "Lỗi từ ngày mười tám tháng mười năm hai nghìn không trăm hai mươi sáu và ngày một tháng tư năm hai nghìn không trăm hai mươi sáu trên k tám s.",
	} {
		if got := spoken(in); got != want {
			t.Errorf("spoken(%.40q) = %q, want %q", in, got, want)
		}
	}
}

func TestBoraReadsEveryUint64(t *testing.T) {
	if got := viNumber(1_000_000_000_005); got != "một nghìn tỷ không trăm linh năm" {
		t.Errorf("viNumber(10^12+5) = %q", got)
	}
	if got := viNumber(math.MaxUint64); !strings.HasPrefix(got, "mười tám tỷ tỷ ") {
		t.Errorf("viNumber(MaxUint64) = %q", got)
	}
}

func TestBoraSayer(t *testing.T) {
	for _, chunks := range [][]string{{"<say></say>", "Hello."}, {"<say>", strings.Repeat("a", sayHold)}} {
		var events []BoraEvent
		w := &sayer{publish: func(e BoraEvent) { events = append(events, e) }}
		for _, chunk := range chunks {
			w.write(chunk)
		}
		if len(events) == 0 || slices.ContainsFunc(events, func(e BoraEvent) bool { return e.Name == "say" || strings.Contains(e.Text, sayOpen) }) {
			t.Errorf("%q streamed %+v, want only the answer, untagged", chunks[0], events)
		}
	}
}
