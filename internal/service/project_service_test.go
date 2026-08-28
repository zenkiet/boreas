package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

func newProjects(t *testing.T) (*ProjectService, *fakeProjectStore, *fakeCredentialStore) {
	t.Helper()
	svc, projects, credentials, _ := newProjectsWithTasks(t)
	return svc, projects, credentials
}

func newProjectsWithTasks(t *testing.T) (*ProjectService, *fakeProjectStore, *fakeCredentialStore, *fakeTaskStore) {
	t.Helper()
	projects, credentials := newFakeProjectStore(), newFakeCredentialStore()
	tasks := newFakeTaskStore()
	projects.tasks = tasks
	svc, err := NewProjectService(
		projects, credentials, newFakeNotificationStore(tasks), newFakeGrantStore(tasks), tasks)
	if err != nil {
		t.Fatal(err)
	}
	return svc, projects, credentials, tasks
}

func admin() core.User { return core.User{ID: uuid.New(), Username: "admin", Role: core.RoleAdmin} }

func member() core.User { return core.User{ID: uuid.New(), Username: "member", Role: core.RoleUser} }

func TestCreateProjectMakesCallerOwner(t *testing.T) {
	svc, store, _ := newProjects(t)
	actor := member()
	project, err := svc.Create(context.Background(), actor, CreateProjectInput{Slug: "team-alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "team-alpha" {
		t.Fatalf("name should default to the slug: %q", project.Name)
	}
	stored, err := store.GetMember(context.Background(), project.ID, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Role != core.ProjectRoleOwner {
		t.Fatalf("creator role = %q, want owner", stored.Role)
	}
}

func TestCreateProjectTaskDefaults(t *testing.T) {
	svc, _, _ := newProjects(t)
	project, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	if project.DefaultImage != "" || project.DefaultPort != 80 || len(project.DefaultEnv) != 0 {
		t.Fatalf("unset defaults = %+v, want \"\", 80, empty", project)
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
		"port too high": {Slug: "bad-port", DefaultPort: 65536},
		"reserved env":  {Slug: "bad-env", DefaultEnv: map[string]string{"BOREAS_PORT": "1"}},
	} {
		if _, err := svc.Create(context.Background(), member(), in); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestUpdateProjectTaskDefaults(t *testing.T) {
	svc, _, _ := newProjects(t)
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

func TestCreateProjectRejectsReservedAndInvalidSlugs(t *testing.T) {
	svc, _, _ := newProjects(t)
	for _, slug := range []string{"api", "admin", "Upper", "-dash", ""} {
		if _, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: slug}); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("slug %q: got %v, want ErrInvalidInput", slug, err)
		}
	}
}

func TestAccessRules(t *testing.T) {
	svc, _, _ := newProjects(t)
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

// seedTask puts a task in the store directly; ProjectService only reads them.
func seedTask(t *testing.T, tasks *fakeTaskStore, projectID uuid.UUID, name string) core.Task {
	t.Helper()
	task, err := tasks.Create(context.Background(), core.Task{
		ProjectID: projectID, Name: name, Image: "img", Status: core.StatusRunning, Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestGrantRaisesRoleButNeverLowersIt(t *testing.T) {
	svc, _, _, tasks := newProjectsWithTasks(t)
	ctx := context.Background()
	owner := member()
	project, err := svc.Create(ctx, owner, CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, project.ID, "web")
	seedTask(t, tasks, project.ID, "db")

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
	svc, _, _, tasks := newProjectsWithTasks(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	web := seedTask(t, tasks, project.ID, "web")
	seedTask(t, tasks, project.ID, "db")

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

	// Deleting the task drops the grant, and with it every trace of access.
	if err := tasks.Delete(ctx, web.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Access(ctx, grantee, "team", ""); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("access outlived the task it pointed at: %v", err)
	}
}

// The project list fans out to every reachable project, so it must mask exactly what the
// single-project route masks. Leaking there is worse: no caller can guard against it.
func TestProjectListHidesDefaultEnvFromGrantees(t *testing.T) {
	svc, store, _, tasks := newProjectsWithTasks(t)
	ctx := context.Background()
	owner := member()
	project, err := svc.Create(ctx, owner, CreateProjectInput{
		Slug: "team", DefaultEnv: map[string]string{"SECRET_KEY": "top-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, project.ID, "web")

	grantee := member()
	if err := svc.Grant(ctx, "team", "web", grantee.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	listed, err := svc.List(ctx, grantee)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("a grant must reveal its project: %+v", listed)
	}
	if len(listed[0].DefaultEnv) != 0 {
		t.Fatalf("default_env leaked through the project list: %+v", listed[0].DefaultEnv)
	}

	// Masking must not cost members their defaults, nor corrupt the stored row.
	mine, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].DefaultEnv["SECRET_KEY"] != "top-secret" {
		t.Fatalf("a member lost the task form defaults: %+v", mine)
	}
	stored, err := store.GetBySlug(ctx, "team")
	if err != nil || stored.DefaultEnv["SECRET_KEY"] != "top-secret" {
		t.Fatalf("masking must not mutate the stored project: %+v, %v", stored, err)
	}
}

func TestGrantRejectsOwner(t *testing.T) {
	svc, _, _, tasks := newProjectsWithTasks(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, project.ID, "web")
	for _, role := range []core.ProjectRole{core.ProjectRoleOwner, "", "root"} {
		err := svc.Grant(ctx, "team", "web", member().ID, role)
		if !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("Grant(%q): got %v, want ErrInvalidInput", role, err)
		}
	}
}

func TestMarkNotificationSeenIsPerUser(t *testing.T) {
	projects, credentials := newFakeProjectStore(), newFakeCredentialStore()
	tasks := newFakeTaskStore()
	projects.tasks = tasks
	notifications := newFakeNotificationStore(tasks)
	svc, err := NewProjectService(projects, credentials, notifications, newFakeGrantStore(tasks), tasks)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	project := projects.add("team")
	seedTask(t, tasks, project.ID, "web")
	n, err := notifications.Create(ctx, core.Notification{
		ProjectID: project.ID, TaskName: "web", Status: core.NotificationInfo, Title: "created",
	})
	if err != nil {
		t.Fatal(err)
	}

	alice, bob := member(), member()
	accFor := func(u core.User) core.ProjectAccess {
		return core.ProjectAccess{Project: project, UserID: u.ID, AllTasks: true}
	}
	if err := svc.MarkNotificationSeen(ctx, accFor(alice), n.ID); err != nil {
		t.Fatal(err)
	}

	listed, err := svc.Notifications(ctx, accFor(alice), 10)
	if err != nil || len(listed) != 1 || !listed[0].Seen {
		t.Fatalf("the marking user must read it as seen: %+v, %v", listed, err)
	}
	listed, err = svc.Notifications(ctx, accFor(bob), 10)
	if err != nil || len(listed) != 1 || listed[0].Seen {
		t.Fatalf("another user's view must stay unseen: %+v, %v", listed, err)
	}

	if err := svc.MarkNotificationUnseen(ctx, accFor(alice), n.ID); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.Notifications(ctx, accFor(alice), 10)
	if err != nil || len(listed) != 1 || listed[0].Seen {
		t.Fatalf("unseen must clear the mark: %+v, %v", listed, err)
	}
	if err := svc.MarkNotificationUnseen(ctx, accFor(alice), n.ID); err != nil {
		t.Fatalf("unseen must be idempotent: %v", err)
	}

	// A grantee without a grant on the task cannot mark it.
	eve := member()
	granteeAcc := core.ProjectAccess{Project: project, UserID: eve.ID, AllTasks: false}
	if err := svc.MarkNotificationSeen(ctx, granteeAcc, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Grant(ctx, "team", "web", eve.ID, core.ProjectRoleViewer); err != nil {
		t.Fatal(err)
	}
	listed, err = svc.Notifications(ctx, granteeAcc, 10)
	if err != nil || len(listed) != 1 || listed[0].Seen {
		t.Fatalf("an out-of-reach mark must be a no-op: %+v, %v", listed, err)
	}
}

func TestGrantNotifiesAssignment(t *testing.T) {
	svc, _, _, tasks := newProjectsWithTasks(t)
	ctx := context.Background()
	project, err := svc.Create(ctx, member(), CreateProjectInput{Slug: "team", Name: "Team Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	seedTask(t, tasks, project.ID, "web")

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
	if len(notified) != 1 {
		t.Fatalf("want 1 notification, got %+v", notified)
	}
	assigned := notified[0]
	if assigned.Status != core.NotificationInfo || assigned.ProjectID != project.ID ||
		assigned.TaskName != "web" || assigned.Title != "👤 Task Assigned • Team Alpha" ||
		assigned.Body != "web: assigned to nam (member)" {
		t.Fatalf("unexpected notification: %+v", assigned)
	}

	// Without a user store the message still reports the granted role.
	svc.Users, notified = nil, nil
	if err := svc.Grant(ctx, "team", "web", grantee.ID, core.ProjectRoleOperator); err != nil {
		t.Fatal(err)
	}
	if len(notified) != 1 || notified[0].Body != "web: operator access granted" {
		t.Fatalf("unexpected fallback notification: %+v", notified)
	}
}

func TestListScopedByRole(t *testing.T) {
	svc, _, _ := newProjects(t)
	first, second := member(), member()
	if _, err := svc.Create(context.Background(), first, CreateProjectInput{Slug: "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), second, CreateProjectInput{Slug: "two"}); err != nil {
		t.Fatal(err)
	}

	mine, err := svc.List(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].Slug != "one" {
		t.Fatalf("a user should only see their own projects: %+v", mine)
	}

	all, err := svc.List(context.Background(), admin())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("an admin should see every project: %+v", all)
	}
}

func TestRemoveMemberProtectsLastOwner(t *testing.T) {
	svc, _, _ := newProjects(t)
	owner, second := member(), member()
	if _, err := svc.Create(context.Background(), owner, CreateProjectInput{Slug: "team"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(context.Background(), "team", owner.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("removing the last owner: got %v, want ErrConflict", err)
	}

	if err := svc.AddMember(context.Background(), "team", second.ID, core.ProjectRoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveMember(context.Background(), "team", owner.ID); err != nil {
		t.Fatalf("removing an owner while another remains: %v", err)
	}
}

func TestAddMemberRejectsUnknownRole(t *testing.T) {
	svc, _, _ := newProjects(t)
	if _, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: "team"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(context.Background(), "team", uuid.New(), "superuser"); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

func TestCredentialLifecycleAndValidation(t *testing.T) {
	svc, _, _ := newProjects(t)
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
}

func TestCreateProjectRejectsUnknownCredential(t *testing.T) {
	svc, _, _ := newProjects(t)
	missing := uuid.New()
	_, err := svc.Create(context.Background(), member(), CreateProjectInput{
		Slug: "team", RegistryCredentialID: &missing,
	})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestNewProjectServiceRequiresCredentialStore(t *testing.T) {
	tasks := newFakeTaskStore()
	_, err := NewProjectService(
		newFakeProjectStore(), nil, newFakeNotificationStore(tasks), newFakeGrantStore(tasks), tasks)
	if !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}
