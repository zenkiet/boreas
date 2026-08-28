package postgres

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/pkg/database"
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BOREAS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set BOREAS_TEST_DATABASE_URL to run the Postgres store tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.NewPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.RunMigrations(ctx, pool, nil); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedProject(t *testing.T, pool *pgxpool.Pool) core.Project {
	t.Helper()
	ctx := context.Background()
	store := NewProjectStore(pool)
	project, err := store.Create(ctx, core.Project{
		Slug:        "test-" + uuid.New().String()[:8],
		Name:        "integration test",
		DefaultPort: 80,
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tasks WHERE project_id = $1`, project.ID)
		_ = store.Delete(context.Background(), project.ID)
	})
	return project
}

func TestTaskStoreRoundTrip(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	store := NewTaskStore(pool)

	created, err := store.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "web", Description: "front door", Note: "## Setup\n`make db`",
		Image: "nginx:alpine", Status: core.StatusCreating, Port: 8080,
		Labels: map[string]string{"tier": "front"},
		Env:    map[string]string{"KEY": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == uuid.Nil || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("database defaults were not returned: %+v", created)
	}
	if created.Labels["tier"] != "front" || created.Env["KEY"] != "value" {
		t.Fatalf("JSONB round trip failed: %+v", created)
	}
	if created.Description != "front door" || created.Note != "## Setup\n`make db`" {
		t.Fatalf("description/note round trip failed: %+v", created)
	}

	fetched, err := store.GetByName(ctx, project.ID, "web")
	if err != nil {
		t.Fatal(err)
	}
	if fetched.ID != created.ID {
		t.Fatal("GetByName returned a different task")
	}

	// The trigger owns updated_at, so it must advance without the app setting it.
	time.Sleep(5 * time.Millisecond)
	fetched.Status, fetched.ContainerIP = core.StatusRunning, "10.0.0.5"
	updated, err := store.Update(ctx, fetched)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("updated_at did not advance: %v -> %v", created.UpdatedAt, updated.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatal("created_at changed on update")
	}
	if updated.Description != "front door" || updated.Note != created.Note {
		t.Fatalf("update lost description/note: %+v", updated)
	}

	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestProjectDefaultsRoundTrip(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := NewProjectStore(pool)

	project := seedProject(t, pool)
	if project.DefaultImage != "" || project.DefaultPort != 80 || len(project.DefaultEnv) != 0 {
		t.Fatalf("column defaults = %+v, want \"\", 80, empty", project)
	}

	project.DefaultImage, project.DefaultPort = "nginx:alpine", 8080
	project.DefaultEnv = map[string]string{"APP_ENV": "dev"}
	updated, err := store.Update(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DefaultImage != "nginx:alpine" || updated.DefaultPort != 8080 || updated.DefaultEnv["APP_ENV"] != "dev" {
		t.Fatalf("update round trip = %+v", updated)
	}

	fetched, err := store.GetBySlug(ctx, project.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.DefaultImage != "nginx:alpine" || fetched.DefaultPort != 8080 || fetched.DefaultEnv["APP_ENV"] != "dev" {
		t.Fatalf("select round trip = %+v", fetched)
	}

	fetched.DefaultPort = 70000
	if _, err := store.Update(ctx, fetched); err == nil {
		t.Fatal("the database must reject an out-of-range default port")
	}
}

func TestNotificationStoreRoundTrip(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	store := NewNotificationStore(pool)

	for _, n := range []core.Notification{
		{ProjectID: project.ID, TaskName: "web", Status: core.NotificationSuccess, Title: "first", Body: "image-a"},
		{ProjectID: project.ID, TaskName: "web", Status: core.NotificationFailure, Title: "second", Body: "boom"},
	} {
		created, err := store.Create(ctx, n)
		if err != nil {
			t.Fatal(err)
		}
		if created.ID == uuid.Nil || created.CreatedAt.IsZero() {
			t.Fatalf("database defaults were not returned: %+v", created)
		}
	}

	listed, err := store.List(ctx, project.ID, uuid.Nil, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Title != "second" {
		t.Fatalf("want newest first, got %+v", listed)
	}
	if listed[0].Status != core.NotificationFailure || listed[0].Body != "boom" {
		t.Fatalf("round trip lost fields: %+v", listed[0])
	}

	limited, err := store.List(ctx, project.ID, uuid.Nil, true, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].Title != "second" {
		t.Fatalf("limit ignored: %+v", limited)
	}

	if _, err := store.List(ctx, uuid.New(), uuid.Nil, true, 10); err != nil {
		t.Fatalf("an unknown project must list empty, got %v", err)
	}
	if _, err := store.Create(ctx, core.Notification{
		ProjectID: project.ID, TaskName: "web", Status: "bogus", Title: "rejected",
	}); err == nil {
		t.Fatal("the database must reject an unknown status")
	}
}

func TestNotificationSeenPerUser(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	store := NewNotificationStore(pool)

	alice := seedUser(t, pool, "seen-alice", core.RoleUser)
	bob := seedUser(t, pool, "seen-bob", core.RoleUser)
	created, err := store.Create(ctx, core.Notification{
		ProjectID: project.ID, TaskName: "web", Status: core.NotificationInfo, Title: "created",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Seen {
		t.Fatalf("a fresh notification must be unseen: %+v", created)
	}

	if err := store.MarkSeen(ctx, created.ID, project.ID, alice.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkSeen(ctx, created.ID, project.ID, alice.ID, true); err != nil {
		t.Fatalf("marking twice must be idempotent: %v", err)
	}

	listed, err := store.List(ctx, project.ID, alice.ID, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || !listed[0].Seen {
		t.Fatalf("the marking user must read it as seen: %+v", listed)
	}
	listed, err = store.List(ctx, project.ID, bob.ID, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Seen {
		t.Fatalf("another user's view must stay unseen: %+v", listed)
	}

	if err := store.MarkUnseen(ctx, created.ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkUnseen(ctx, created.ID, alice.ID); err != nil {
		t.Fatalf("unseen must be idempotent: %v", err)
	}
	listed, err = store.List(ctx, project.ID, alice.ID, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Seen {
		t.Fatalf("unseen must clear the mark: %+v", listed)
	}

	// Without a grant on the task, a non-member's mark is a no-op.
	if err := store.MarkSeen(ctx, created.ID, project.ID, bob.ID, false); err != nil {
		t.Fatal(err)
	}
	listed, err = store.List(ctx, project.ID, bob.ID, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Seen {
		t.Fatalf("an out-of-reach mark must be a no-op: %+v", listed)
	}
}

func TestNotificationsSurviveTaskDeletion(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	tasks := NewTaskStore(pool)
	store := NewNotificationStore(pool)

	task, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "web", Image: "nginx:alpine",
		Status: core.StatusRunning, Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, core.Notification{
		ProjectID: project.ID, TaskName: task.Name,
		Status: core.NotificationSuccess, Title: "deployed", Body: "nginx:alpine",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}

	listed, err := store.List(ctx, project.ID, uuid.Nil, true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].TaskName != "web" {
		t.Fatalf("deleting a task erased its deploy history: %+v", listed)
	}
}

func TestTaskStoreErrorMapping(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	store := NewTaskStore(pool)

	if _, err := store.GetByName(ctx, project.ID, "absent"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}

	task := core.Task{ProjectID: project.ID, Name: "dup", Image: "img", Status: core.StatusUnknown, Port: 80}
	if _, err := store.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, task); !errors.Is(err, core.ErrAlreadyExists) {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
	}
}

func TestTaskNameUniquePerProject(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	first, second := seedProject(t, pool), seedProject(t, pool)
	store := NewTaskStore(pool)

	for _, projectID := range []uuid.UUID{first.ID, second.ID} {
		if _, err := store.Create(ctx, core.Task{
			ProjectID: projectID, Name: "shared", Image: "img", Status: core.StatusUnknown, Port: 80,
		}); err != nil {
			t.Fatalf("the same task name must be allowed in another project: %v", err)
		}
	}

	tasks, err := store.List(ctx, first.ID, uuid.Nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("List must be scoped to one project, got %d tasks", len(tasks))
	}
}

func TestProjectDeleteRestrictedByTasks(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	projects, tasks := NewProjectStore(pool), NewTaskStore(pool)

	task, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "web", Image: "img", Status: core.StatusUnknown, Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := projects.Delete(ctx, project.ID); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	if err := tasks.Delete(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := projects.Delete(ctx, project.ID); err != nil {
		t.Fatal(err)
	}
}

func TestForeignKeyViolationMapsToConflict(t *testing.T) {
	err := mapError("delete project", &pgconn.PgError{Code: foreignKeyViolation})
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

func TestSchemaRejectsReservedProjectSlug(t *testing.T) {
	pool := newPool(t)
	store := NewProjectStore(pool)
	if _, err := store.Create(context.Background(), core.Project{
		Slug: "api", Name: "reserved", DefaultPort: 80,
	}); err == nil {
		t.Fatal("the database CHECK constraint should reject the reserved slug 'api'")
	}
}

func TestUserAndTokenStores(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	users, tokens := NewUserStore(pool), NewTokenStore(pool)

	user, err := users.Create(ctx, core.User{
		Username: "user-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     core.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), user.ID) })

	if _, err := users.GetByUsername(ctx, user.Username); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Create(ctx, core.User{
		Username: user.Username, Email: "other@example.com", Role: core.RoleUser,
	}); !errors.Is(err, core.ErrAlreadyExists) {
		t.Fatalf("got %v, want ErrAlreadyExists", err)
	}

	hash := uuid.New().String()
	now := time.Now().UTC()
	createdToken, err := tokens.Create(ctx, core.AuthToken{
		UserID: user.ID, Kind: core.TokenKindSession, TokenHash: hash,
		ValidFrom: now, ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if createdToken.ID == uuid.Nil || createdToken.Kind != core.TokenKindSession || createdToken.CreatedAt.IsZero() {
		t.Fatalf("database defaults were not returned: %+v", createdToken)
	}
	stored, err := tokens.GetByHash(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RevokedAt != nil {
		t.Fatal("a new token must not be revoked")
	}

	apiOwner := user.ID
	otherUser, err := users.Create(ctx, core.User{
		Username: "token-other-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     core.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), otherUser.ID) })
	apiHash := uuid.New().String()
	apiToken, err := tokens.Create(ctx, core.AuthToken{
		UserID: apiOwner, Name: "ci", Kind: core.TokenKindAPI, TokenHash: apiHash,
		ValidFrom: now, ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := tokens.ListAPITokens(ctx, apiOwner)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != apiToken.ID || listed[0].Name != "ci" {
		t.Fatalf("listed API tokens = %+v", listed)
	}
	if err := tokens.RevokeByID(ctx, otherUser.ID, apiToken.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("another user revoked the token: %v", err)
	}
	if err := tokens.RevokeByID(ctx, apiOwner, apiToken.ID); err != nil {
		t.Fatal(err)
	}
	revokedAPI, err := tokens.GetByHash(ctx, apiHash)
	if err != nil || revokedAPI.RevokedAt == nil {
		t.Fatalf("API token was not revoked: %+v %v", revokedAPI, err)
	}
	if err := tokens.Revoke(ctx, hash); err != nil {
		t.Fatal(err)
	}
	if stored, err = tokens.GetByHash(ctx, hash); err != nil || stored.RevokedAt == nil {
		t.Fatalf("token was not revoked: %+v %v", stored, err)
	}

	if err := users.Delete(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.GetByHash(ctx, hash); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("tokens should cascade with the user, got %v", err)
	}
}

func TestProjectMembership(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	users, projects := NewUserStore(pool), NewProjectStore(pool)

	user, err := users.Create(ctx, core.User{
		Username: "member-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     core.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), user.ID) })

	if err := projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: user.ID, Role: core.ProjectRoleMember,
	}); err != nil {
		t.Fatal(err)
	}
	if err := projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: user.ID, Role: core.ProjectRoleOwner,
	}); err != nil {
		t.Fatal(err)
	}
	member, err := projects.GetMember(ctx, project.ID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if member.Role != core.ProjectRoleOwner || member.Username != user.Username {
		t.Fatalf("unexpected member: %+v", member)
	}

	mine, err := projects.ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].ID != project.ID {
		t.Fatalf("ListForUser = %+v", mine)
	}

	if err := projects.RemoveMember(ctx, project.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := projects.RemoveMember(ctx, project.ID, user.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestTaskGrantsDieWithTheirTask(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	users, tasks := NewUserStore(pool), NewTaskStore(pool)
	grants, notifications := NewGrantStore(pool), NewNotificationStore(pool)

	user, err := users.Create(ctx, core.User{
		Username: "grantee-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     core.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), user.ID) })

	web, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "web", Image: "img", Status: core.StatusRunning, Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "db", Image: "img", Status: core.StatusRunning, Port: 80,
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"web", "db"} {
		if _, err := notifications.Create(ctx, core.Notification{
			ProjectID: project.ID, TaskName: name, Status: core.NotificationSuccess, Title: name,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := grants.Grant(ctx, core.TaskGrant{
		TaskID: web.ID, UserID: user.ID, Role: core.ProjectRoleOperator,
	}); err != nil {
		t.Fatal(err)
	}

	role, err := grants.Role(ctx, project.ID, user.ID, "web")
	if err != nil || role != core.ProjectRoleOperator {
		t.Fatalf("granted role = %q, %v", role, err)
	}
	if role, err := grants.Role(ctx, project.ID, user.ID, "db"); err != nil || role != "" {
		t.Fatalf("ungranted task must yield no role, got %q, %v", role, err)
	}

	// Filtering happens in SQL, so a grantee's list and feed carry only what was granted.
	scoped, err := tasks.List(ctx, project.ID, user.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 1 || scoped[0].Name != "web" {
		t.Fatalf("task list ignored grants: %+v", scoped)
	}
	feed, err := notifications.List(ctx, project.ID, user.ID, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 1 || feed[0].TaskName != "web" {
		t.Fatalf("notification feed ignored grants: %+v", feed)
	}

	if err := tasks.Delete(ctx, web.ID); err != nil {
		t.Fatal(err)
	}
	if ok, err := grants.AnyInProject(ctx, project.ID, user.ID); err != nil || ok {
		t.Fatalf("the grant outlived its task: any=%v err=%v", ok, err)
	}
	// The row survives for members, but no longer reaches the grantee whose grant is gone.
	feed, err = notifications.List(ctx, project.ID, user.ID, false, 10)
	if err != nil || len(feed) != 0 {
		t.Fatalf("deploy history stayed visible after the grant died: %+v, %v", feed, err)
	}
	if all, err := notifications.List(ctx, project.ID, user.ID, true, 10); err != nil || len(all) != 2 {
		t.Fatalf("members must still see the full history: %+v, %v", all, err)
	}
}

func TestGrantedProjectIsListedWithoutMembership(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	users, tasks, grants := NewUserStore(pool), NewTaskStore(pool), NewGrantStore(pool)
	projects := NewProjectStore(pool)

	project.DefaultEnv = map[string]string{"SECRET_KEY": "top-secret"}
	if _, err := projects.Update(ctx, project); err != nil {
		t.Fatal(err)
	}

	user, err := users.Create(ctx, core.User{
		Username: "grantee-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     core.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), user.ID) })

	if mine, err := projects.ListForUser(ctx, user.ID); err != nil || len(mine) != 0 {
		t.Fatalf("a stranger must see nothing: %+v, %v", mine, err)
	}
	task, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "web", Image: "img", Status: core.StatusRunning, Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := grants.Grant(ctx, core.TaskGrant{
		TaskID: task.ID, UserID: user.ID, Role: core.ProjectRoleViewer,
	}); err != nil {
		t.Fatal(err)
	}
	mine, err := projects.ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].ID != project.ID {
		t.Fatalf("a grant must reveal its project without a membership row: %+v", mine)
	}
	// The list must mask what the single-project route masks, or secrets leak through the fan-out.
	if len(mine[0].DefaultEnv) != 0 {
		t.Fatalf("default_env leaked to a grantee through the project list: %+v", mine[0].DefaultEnv)
	}

	// A member of the same project must still receive the defaults.
	if err := projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: user.ID, Role: core.ProjectRoleViewer,
	}); err != nil {
		t.Fatal(err)
	}
	mine, err = projects.ListForUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].DefaultEnv["SECRET_KEY"] != "top-secret" {
		t.Fatalf("a member must still see task form defaults: %+v", mine)
	}
}

func TestCredentialStore(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	store := NewCredentialStore(pool)

	credential, err := store.Create(ctx, core.RegistryCredential{
		Name: "cred-" + uuid.New().String()[:8], Registry: core.RegistryGHCR,
		Username: "bot", Token: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), credential.ID) })

	fetched, err := store.Get(ctx, credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Token != "secret" || fetched.Registry != core.RegistryGHCR {
		t.Fatalf("unexpected credential: %+v", fetched)
	}
	if _, err := store.Get(ctx, uuid.New()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	pool := newPool(t)
	if err := database.RunMigrations(context.Background(), pool, nil); err != nil {
		t.Fatalf("re-running migrations failed: %v", err)
	}
}

func TestAPITokenSchemaConstraints(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	user, err := users.Create(ctx, core.User{
		Username: "constraints-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     core.RoleUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), user.ID) })
	now := time.Now().UTC()
	for name, args := range map[string][]any{
		"blank name": {user.ID, "", "api", uuid.New().String(), now, now.Add(time.Hour)},
		"bad kind":   {user.ID, "ci", "other", uuid.New().String(), now, now.Add(time.Hour)},
		"bad window": {user.ID, "ci", "api", uuid.New().String(), now, now},
		"over 90":    {user.ID, "ci", "api", uuid.New().String(), now, now.Add(90*24*time.Hour + time.Second)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(ctx, `INSERT INTO auth_tokens
				(user_id, name, kind, token_hash, valid_from, expires_at)
				VALUES ($1, $2, $3, $4, $5, $6)`, args...)
			if err == nil {
				t.Fatal("database constraint accepted invalid token")
			}
		})
	}
}

// seedUser returns a throwaway account that is removed when the test ends.
func seedUser(t *testing.T, pool *pgxpool.Pool, prefix string, role core.UserRole) core.User {
	t.Helper()
	users := NewUserStore(pool)
	user, err := users.Create(context.Background(), core.User{
		Username: prefix + "-" + uuid.New().String()[:8],
		Email:    uuid.New().String()[:8] + "@example.com",
		Role:     role,
	})
	if err != nil {
		t.Fatalf("create %s: %v", prefix, err)
	}
	t.Cleanup(func() { _ = users.Delete(context.Background(), user.ID) })
	return user
}

// Push delivery must reach exactly the audience that the notification feed already
// shows, or a browser would leak deploys the same account cannot list over the API.
func TestPushTokensMatchNotificationAudience(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	project := seedProject(t, pool)
	tasks, grants := NewTaskStore(pool), NewGrantStore(pool)
	projects, users, push := NewProjectStore(pool), NewUserStore(pool), NewPushStore(pool)

	web, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "web", Image: "img", Status: core.StatusRunning, Port: 80,
	})
	if err != nil {
		t.Fatal(err)
	}

	admin := seedUser(t, pool, "push-admin", core.RoleAdmin)
	member := seedUser(t, pool, "push-member", core.RoleUser)
	grantee := seedUser(t, pool, "push-grantee", core.RoleUser)
	outsider := seedUser(t, pool, "push-outsider", core.RoleUser)
	disabled := seedUser(t, pool, "push-disabled", core.RoleAdmin)

	if err := projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: member.ID, Role: core.ProjectRoleMember,
	}); err != nil {
		t.Fatal(err)
	}
	if err := grants.Grant(ctx, core.TaskGrant{
		TaskID: web.ID, UserID: grantee.ID, Role: core.ProjectRoleViewer,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	disabled.DisabledAt = &now
	if _, err := users.Update(ctx, disabled); err != nil {
		t.Fatal(err)
	}

	token := map[string]string{}
	for name, user := range map[string]core.User{
		"admin": admin, "member": member, "grantee": grantee,
		"outsider": outsider, "disabled": disabled,
	} {
		token[name] = "tok-" + uuid.New().String()
		if err := push.Create(ctx, user.ID, token[name]); err != nil {
			t.Fatalf("subscribe %s: %v", name, err)
		}
	}

	tokens, err := push.Tokens(ctx, project.ID, web.Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin", "member", "grantee"} {
		if !slices.Contains(tokens, token[name]) {
			t.Errorf("%s must receive deploys of a task they can list", name)
		}
	}
	for _, name := range []string{"outsider", "disabled"} {
		if slices.Contains(tokens, token[name]) {
			t.Errorf("%s must not receive deploys they cannot list", name)
		}
	}

	// A grant covers one task, so a sibling task must not reach the grantee.
	api, err := tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: "api", Image: "img", Status: core.StatusRunning, Port: 81,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tokens, err = push.Tokens(ctx, project.ID, api.Name); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tokens, token["grantee"]) {
		t.Error("a task grant must not widen to the whole project")
	}
	if !slices.Contains(tokens, token["member"]) {
		t.Error("a project member must receive every task in the project")
	}

	// The subscription dies with its user, so a deleted account leaves no target behind.
	if err := users.Delete(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if tokens, err = push.Tokens(ctx, project.ID, web.Name); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tokens, token["admin"]) {
		t.Error("deleting a user must cascade to their subscriptions")
	}
}

func TestPushSubscriptionMovesToItsLatestOwner(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	push := NewPushStore(pool)

	first := seedUser(t, pool, "push-first", core.RoleAdmin)
	second := seedUser(t, pool, "push-second", core.RoleAdmin)
	shared := "tok-" + uuid.New().String()

	if err := push.Create(ctx, first.ID, shared); err != nil {
		t.Fatal(err)
	}
	var createdAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT created_at FROM push_subscriptions WHERE token = $1`, shared,
	).Scan(&createdAt); err != nil {
		t.Fatal(err)
	}
	// A browser handed to another user must stop notifying the previous one.
	if err := push.Create(ctx, second.ID, shared); err != nil {
		t.Fatalf("re-registering a device must not conflict: %v", err)
	}
	var movedAt time.Time
	if err := pool.QueryRow(ctx,
		`SELECT created_at FROM push_subscriptions WHERE token = $1`, shared,
	).Scan(&movedAt); err != nil {
		t.Fatal(err)
	}
	if !movedAt.Equal(createdAt) {
		t.Fatalf("re-registering changed created_at: %v -> %v", createdAt, movedAt)
	}
	if err := push.Delete(ctx, first.ID, shared); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("the previous owner must no longer own the token, got %v", err)
	}
	if err := push.Delete(ctx, second.ID, shared); err != nil {
		t.Fatalf("the current owner must be able to unsubscribe: %v", err)
	}
}
