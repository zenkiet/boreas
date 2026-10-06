package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

func newProjects(t *testing.T) (*ProjectService, *fakeTaskStore) {
	t.Helper()
	projects, tasks := newFakeProjectStore(), newFakeTaskStore()
	projects.tasks = tasks
	svc, err := NewProjectService(
		projects, newFakeCredentialStore(), newFakeNotificationStore(tasks), newFakeGrantStore(tasks), tasks)
	if err != nil {
		t.Fatal(err)
	}
	return svc, tasks
}

func admin() core.User { return core.User{ID: uuid.New(), Username: "admin", Role: core.RoleAdmin} }

func member() core.User { return core.User{ID: uuid.New(), Username: "member", Role: core.RoleUser} }

func TestCreateProjectTaskDefaults(t *testing.T) {
	svc, _ := newProjects(t)
	project, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "bare" || project.DefaultImage != "" || project.DefaultPort != 80 || len(project.DefaultEnv) != 0 {
		t.Fatalf("unset defaults = %+v, want name=slug, \"\", 80, empty", project)
	}

	project, err = svc.Create(context.Background(), member(), CreateProjectInput{
		Slug: "preset", DefaultImage: "  nginx:alpine  ", DefaultPort: 8080,
		DefaultEnv: map[string]string{"APP_ENV": "dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.DefaultImage != "nginx:alpine" || project.DefaultPort != 8080 || project.DefaultEnv["APP_ENV"] != "dev" {
		t.Fatalf("supplied defaults = %+v", project)
	}

	for name, in := range map[string]CreateProjectInput{
		"reserved slug": {Slug: "api"},
		"port too high": {Slug: "bad-port", DefaultPort: 65536},
		"reserved env":  {Slug: "bad-env", DefaultEnv: map[string]string{"BOREAS_PORT": "1"}},
	} {
		if _, err := svc.Create(context.Background(), member(), in); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestUpdateProjectTaskDefaults(t *testing.T) {
	svc, _ := newProjects(t)
	if _, err := svc.Create(context.Background(), member(), CreateProjectInput{
		Slug: "team", DefaultImage: "nginx:alpine", DefaultPort: 8080,
		DefaultEnv: map[string]string{"APP_ENV": "dev"},
	}); err != nil {
		t.Fatal(err)
	}

	renamed := "Renamed"
	project, err := svc.Update(context.Background(), "team", UpdateProjectInput{Name: &renamed})
	if err != nil {
		t.Fatal(err)
	}
	if project.DefaultImage != "nginx:alpine" || project.DefaultPort != 8080 || project.DefaultEnv["APP_ENV"] != "dev" {
		t.Fatalf("omitted defaults were changed: %+v", project)
	}

	blank, empty := "", map[string]string{}
	project, err = svc.Update(context.Background(), "team", UpdateProjectInput{
		DefaultImage: &blank, DefaultEnv: &empty,
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.DefaultImage != "" || len(project.DefaultEnv) != 0 || project.DefaultPort != 8080 {
		t.Fatalf("clearing defaults = %+v", project)
	}

	badPort, reserved := 0, map[string]string{"BASE_HREF": "/"}
	for name, in := range map[string]UpdateProjectInput{
		"port zero":    {DefaultPort: &badPort},
		"reserved env": {DefaultEnv: &reserved},
	} {
		if _, err := svc.Update(context.Background(), "team", in); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestAccessRules(t *testing.T) {
	svc, _ := newProjects(t)
	owner := member()
	_, err := svc.Create(context.Background(), owner, CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}

	acc, err := svc.Access(context.Background(), owner, "team", "")
	if err != nil || acc.Role != core.ProjectRoleOwner || !acc.AllTasks {
		t.Fatalf("owner access = %+v, %v", acc, err)
	}

	// An unreachable project must look missing rather than merely forbidden.
	outsider := member()
	if _, err := svc.Access(context.Background(), outsider, "team", ""); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("outsider access: got %v, want ErrNotFound", err)
	}

	acc, err = svc.Access(context.Background(), admin(), "team", "")
	if err != nil || acc.Role != core.ProjectRoleOwner || !acc.AllTasks {
		t.Fatalf("admin access = %+v, %v", acc, err)
	}

	if err := svc.AddMember(context.Background(), "team", outsider.ID, core.ProjectRoleMember); err != nil {
		t.Fatal(err)
	}
	acc, err = svc.Access(context.Background(), outsider, "team", "")
	if err != nil || acc.Role != core.ProjectRoleMember || !acc.AllTasks {
		t.Fatalf("added member access = %+v, %v", acc, err)
	}
}

// seedTask puts a task in the store directly, so no runtime call precedes the one under test.
func seedTask(t *testing.T, tasks *fakeTaskStore, task core.Task) core.Task {
	t.Helper()
	task.Image, task.Port = "img", 80
	created, err := tasks.Create(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestGrantRaisesRoleButNeverLowersIt(t *testing.T) {
	svc, tasks := newProjects(t)
	ctx := context.Background()
	owner := member()
	project, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "web"})
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "db"})

	// A project viewer granted operator on one task deploys that task and only that one.
	viewer := member()
	if err := svc.AddMember(ctx, "team", viewer.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := svc.Grant(ctx, "team", "web", viewer.ID, core.ProjectRoleOperator); err != nil {
		t.Fatal(err)
	}
	acc, err := svc.Access(ctx, viewer, "team", "web")
	if err != nil || acc.Role != core.ProjectRoleOperator {
		t.Fatalf("granted task role = %+v, %v; want operator", acc, err)
	}
	acc, err = svc.Access(ctx, viewer, "team", "db")
	if err != nil || acc.Role != core.ProjectRoleViewer {
		t.Fatalf("ungranted task role = %+v, %v; want the project role", acc, err)
	}

	// A weaker grant must not take anything away from a stronger membership.
	if err := svc.Grant(ctx, "team", "web", owner.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	acc, err = svc.Access(ctx, owner, "team", "web")
	if err != nil || acc.Role != core.ProjectRoleOwner {
		t.Fatalf("grant lowered an owner: %+v, %v", acc, err)
	}
}

func TestGranteeReachesOnlyGrantedTasks(t *testing.T) {
	svc, tasks := newProjects(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "web"})
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "db"})

	grantee := member()
	if err := svc.Grant(ctx, "team", "web", grantee.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	// A grant alone reaches the project envelope, but only as a viewer of what was granted.
	acc, err := svc.Access(ctx, grantee, "team", "")
	if err != nil || acc.Role != core.ProjectRoleViewer || acc.AllTasks {
		t.Fatalf("envelope access = %+v, %v", acc, err)
	}
	// An ungranted task must look missing, not merely forbidden, or its name leaks.
	if _, err := svc.Access(ctx, grantee, "team", "db"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("ungranted task: got %v, want ErrNotFound", err)
	}
}

// The fleet must scope tasks and roles exactly as Access does per request, or its buttons lie.
func TestFleetScopesTasksAndRoles(t *testing.T) {
	svc, tasks := newProjects(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "web"})
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "db"})
	viewer, grantee := member(), member()
	if err := svc.AddMember(ctx, "team", viewer.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	for _, user := range []core.User{viewer, grantee} {
		if err := svc.Grant(ctx, "team", "web", user.ID, core.ProjectRoleOperator); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.notifications.Create(ctx, core.Notification{
		ProjectID: project.ID, TaskName: "web", Type: core.NotificationDeployed, Status: core.NotificationSuccess,
	}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		actor core.User
		want  string
	}{
		{admin(), "db=owner web=owner+deploy"},
		{viewer, "db=viewer web=operator+deploy"},
		{grantee, "web=operator+deploy"},
	} {
		_, fleet, err := svc.Fleet(ctx, tc.actor)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, task := range fleet[project.ID] {
			entry := task.Name + "=" + string(task.Role)
			if task.LastDeploy != nil {
				entry += "+deploy"
			}
			got = append(got, entry)
		}
		slices.Sort(got)
		if strings.Join(got, " ") != tc.want {
			t.Fatalf("fleet = %v, want %s", got, tc.want)
		}
	}
}

func TestGrantRejectsOwner(t *testing.T) {
	svc, tasks := newProjects(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "web"})
	for _, role := range []core.ProjectRole{core.ProjectRoleOwner, "", "root"} {
		err := svc.Grant(ctx, "team", "web", member().ID, role)
		if !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("Grant(%q): got %v, want ErrInvalidInput", role, err)
		}
	}
}

func TestGrantNotifiesAssignment(t *testing.T) {
	svc, tasks := newProjects(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team", Name: "Team Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, core.Task{ProjectID: project.ID, Name: "web"})

	var notified []core.Notification
	svc.Notify = func(_ context.Context, n core.Notification) { notified = append(notified, n) }
	users := newFakeUserStore()
	grantee, err := users.Create(ctx, core.User{Username: "nam"})
	if err != nil {
		t.Fatal(err)
	}
	svc.Users = users

	if err := svc.Grant(ctx, "team", "web", grantee.ID, core.ProjectRoleMember); err != nil {
		t.Fatal(err)
	}
	want := core.Notification{
		ProjectID: project.ID, TaskName: "web", Type: core.NotificationTaskAssigned,
		Status: core.NotificationInfo, Title: "👤 Task Assigned • Team Alpha", Body: "web: assigned to nam (member)",
	}
	if len(notified) != 1 || notified[0] != want {
		t.Fatalf("notified %+v, want %+v", notified, want)
	}

	svc.Users, notified = nil, nil
	if err := svc.Grant(ctx, "team", "web", grantee.ID, core.ProjectRoleOperator); err != nil {
		t.Fatal(err)
	}
	if len(notified) != 1 || notified[0].Body != "web: operator access granted" {
		t.Fatalf("unexpected fallback notification: %+v", notified)
	}
}

func TestRemoveMemberProtectsLastOwner(t *testing.T) {
	svc, _ := newProjects(t)
	owner, second := member(), member()
	if _, err := svc.Create(context.Background(), owner, CreateProjectInput{Slug: "team"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(context.Background(), "team", owner.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("removing the last owner: got %v, want ErrConflict", err)
	}
	if err := svc.AddMember(context.Background(), "team", second.ID, "superuser"); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("unknown role: got %v, want ErrInvalidInput", err)
	}

	if err := svc.AddMember(context.Background(), "team", second.ID, core.ProjectRoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(context.Background(), "team", owner.ID); err != nil {
		t.Fatalf("removing an owner while another remains: %v", err)
	}
}

func TestCredentialLifecycleAndValidation(t *testing.T) {
	svc, _ := newProjects(t)
	actor := admin()
	credential, err := svc.CreateCredential(context.Background(), actor, CreateCredentialInput{
		Name: "ghcr-bot", Registry: core.RegistryGHCR, Username: "bot", Token: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	listed, err := svc.ListCredentials(context.Background())
	if err != nil || len(listed) != 1 {
		t.Fatalf("list credentials = %+v, %v", listed, err)
	}

	if _, err := svc.CreateCredential(context.Background(), actor, CreateCredentialInput{
		Name: "bad", Registry: "quay", Username: "u", Token: "t",
	}); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("unknown registry: got %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateCredential(context.Background(), actor, CreateCredentialInput{
		Name: "", Registry: core.RegistryGHCR, Username: "u", Token: "t",
	}); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("empty name: got %v, want ErrInvalidInput", err)
	}

	if err := svc.DeleteCredential(context.Background(), credential.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCredential(context.Background(), uuid.New()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	if _, err := svc.Create(context.Background(), member(), CreateProjectInput{
		Slug: "team", RegistryCredentialID: &credential.ID,
	}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a deleted credential was accepted: got %v, want ErrNotFound", err)
	}
}

type fakeCode struct{ indexed []string }

func (f fakeCode) Repos(context.Context) ([]string, error) { return f.indexed, nil }

func (fakeCode) Search(_ context.Context, repos []string, _ string) ([]core.CodeMatch, error) {
	return []core.CodeMatch{{Repo: repos[0]}}, nil
}

func (fakeCode) Read(context.Context, []string, string, string) (core.CodeFile, error) {
	return core.CodeFile{}, nil
}

func TestProjectRepositoriesMustBeIndexed(t *testing.T) {
	svc, _ := newProjects(t)
	svc.Code = fakeCode{indexed: []string{"github.com/acme/web"}}
	ctx := context.Background()
	if _, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team"}); err != nil {
		t.Fatal(err)
	}
	for _, repos := range [][]string{{"acme/web"}, {"github.com/acme/other"}, {"github.com/acme/web", "github.com/acme/web"}} {
		if _, err := svc.Update(ctx, "team", UpdateProjectInput{Repositories: &repos}); !errors.Is(err, core.ErrInvalidInput) {
			t.Errorf("repositories %v: %v", repos, err)
		}
	}
	repos := []string{"github.com/acme/web"}
	project, err := svc.Update(ctx, "team", UpdateProjectInput{Repositories: &repos})
	if err != nil || !slices.Equal(project.Repositories, repos) {
		t.Fatalf("project %+v, err %v", project, err)
	}
}

type fakeChats struct{ chats map[uuid.UUID]core.Chat }

func (f *fakeChats) Create(_ context.Context, chat core.Chat) (core.Chat, error) {
	chat.ID, chat.ProjectSlug = uuid.New(), "team"
	f.chats[chat.ID] = chat
	return chat, nil
}

func (f *fakeChats) Get(_ context.Context, id, userID uuid.UUID) (core.Chat, error) {
	chat, ok := f.chats[id]
	if !ok || chat.UserID != userID {
		return core.Chat{}, core.ErrNotFound
	}
	return chat, nil
}

func (f *fakeChats) List(_ context.Context, userID uuid.UUID) ([]core.Chat, error) {
	var chats []core.Chat
	for _, chat := range f.chats {
		if chat.UserID == userID {
			chats = append(chats, chat)
		}
	}
	return chats, nil
}

func (f *fakeChats) Append(_ context.Context, id uuid.UUID, messages []core.ChatMessage) error {
	chat := f.chats[id]
	chat.Messages = append(chat.Messages, messages...)
	f.chats[id] = chat
	return nil
}

func (f *fakeChats) Delete(_ context.Context, id, _ uuid.UUID) error {
	delete(f.chats, id)
	return nil
}

func TestChatsArePrivateToMembers(t *testing.T) {
	svc, _ := newProjects(t)
	svc.Code = fakeCode{indexed: []string{"github.com/acme/web"}}
	svc.Chats = &fakeChats{chats: map[uuid.UUID]core.Chat{}}
	var histories [][]core.ChatMessage
	svc.Assist = func(_ context.Context, _ uuid.UUID, _ []string, history []core.ChatMessage, question string) (core.ChatMessage, error) {
		histories = append(histories, history)
		return core.ChatMessage{Role: core.ChatAssistant, Content: "answer to " + question}, nil
	}
	ctx, owner, stranger := context.Background(), member(), member()
	if _, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "team"}); err != nil {
		t.Fatal(err)
	}
	repos := []string{"github.com/acme/web"}
	if _, err := svc.Update(ctx, "team", UpdateProjectInput{Repositories: &repos}); err != nil {
		t.Fatal(err)
	}
	acc, err := svc.Access(ctx, owner, "team", "")
	if err != nil {
		t.Fatal(err)
	}

	chat, err := svc.StartChat(ctx, acc, "  How is the\nsurcharge shown?  ")
	if err != nil || chat.Title != "How is the surcharge shown?" || len(chat.Messages) != 2 {
		t.Fatalf("chat %+v, err %v", chat, err)
	}
	if _, err := svc.StartChat(ctx, core.ProjectAccess{Project: acc.Project}, "q"); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("a task grantee started a chat: %v", err)
	}
	if _, err := svc.Reply(ctx, stranger, chat.ID, "Refunds?"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("someone else replied in the chat: %v", err)
	}
	if _, err := svc.Reply(ctx, owner, chat.ID, "Refunds?"); err != nil || len(histories) != 2 || len(histories[1]) != 2 {
		t.Fatalf("reply err %v, histories %v", err, histories)
	}
	if stored, err := svc.GetChat(ctx, owner, chat.ID); err != nil || len(stored.Messages) != 4 {
		t.Fatalf("stored chat %+v, err %v", stored, err)
	}

	leaver := member()
	if err := svc.AddMember(ctx, "team", leaver.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	acc, _ = svc.Access(ctx, leaver, "team", "")
	left, err := svc.StartChat(ctx, acc, "Refunds?")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(ctx, "team", leaver.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetChat(ctx, leaver, left.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("a former member read the project's chat: %v", err)
	}
	if chats, err := svc.ListChats(ctx, leaver); err != nil || len(chats) != 0 {
		t.Fatalf("a former member listed %v, err %v", chats, err)
	}
	if chats, err := svc.ListChats(ctx, owner); err != nil || len(chats) != 1 {
		t.Fatalf("owner listed %v, err %v", chats, err)
	}
}
