package postgres

import (
	"context"
	"errors"
	"os"
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
		Slug: "test-" + uuid.New().String()[:8],
		Name: "integration test",
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
		ProjectID: project.ID, Name: "web", Image: "nginx:alpine",
		Status: core.StatusCreating, Port: 8080,
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

	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, created.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
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

	tasks, err := store.List(ctx, first.ID)
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
	if _, err := store.Create(context.Background(), core.Project{Slug: "api", Name: "reserved"}); err == nil {
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
