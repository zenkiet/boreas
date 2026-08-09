package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

func newProjects(t *testing.T) (*ProjectService, *fakeProjectStore, *fakeTaskStore, *fakeCredentialStore) {
	t.Helper()
	projects, tasks, credentials := newFakeProjectStore(), newFakeTaskStore(), newFakeCredentialStore()
	svc, err := NewProjectService(projects, tasks, credentials)
	if err != nil {
		t.Fatal(err)
	}
	return svc, projects, tasks, credentials
}

func admin() core.User  { return core.User{ID: uuid.New(), Username: "admin", Role: core.RoleAdmin} }
func member() core.User { return core.User{ID: uuid.New(), Username: "member", Role: core.RoleUser} }

func TestCreateProjectMakesCallerOwner(t *testing.T) {
	svc, store, _, _ := newProjects(t)
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

func TestCreateProjectRejectsReservedAndInvalidSlugs(t *testing.T) {
	svc, _, _, _ := newProjects(t)
	for _, slug := range []string{"api", "admin", "Upper", "-dash", ""} {
		if _, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: slug}); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("slug %q: got %v, want ErrInvalidInput", slug, err)
		}
	}
}

func TestAccessRules(t *testing.T) {
	svc, _, _, _ := newProjects(t)
	owner := member()
	_, err := svc.Create(context.Background(), owner, CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}

	role, err := svc.Access(context.Background(), owner, "team")
	if err != nil || role != core.ProjectRoleOwner {
		t.Fatalf("owner access = %q, %v", role, err)
	}

	outsider := member()
	if _, err := svc.Access(context.Background(), outsider, "team"); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("outsider access: got %v, want ErrForbidden", err)
	}

	role, err = svc.Access(context.Background(), admin(), "team")
	if err != nil || role != core.ProjectRoleOwner {
		t.Fatalf("admin access = %q, %v", role, err)
	}

	if err := svc.AddMember(context.Background(), "team", outsider.ID, core.ProjectRoleMember); err != nil {
		t.Fatal(err)
	}
	role, err = svc.Access(context.Background(), outsider, "team")
	if err != nil || role != core.ProjectRoleMember {
		t.Fatalf("added member access = %q, %v", role, err)
	}
}

func TestListScopedByRole(t *testing.T) {
	svc, _, _, _ := newProjects(t)
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

func TestDeleteProjectRequiresNoTasks(t *testing.T) {
	svc, _, tasks, _ := newProjects(t)
	project, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: "team"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(context.Background(), core.Task{ProjectID: project.ID, Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), "team"); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}

	stored, err := tasks.GetByName(context.Background(), project.ID, "web")
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Delete(context.Background(), stored.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), "team"); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveMemberProtectsLastOwner(t *testing.T) {
	svc, _, _, _ := newProjects(t)
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
	svc, _, _, _ := newProjects(t)
	if _, err := svc.Create(context.Background(), member(), CreateProjectInput{Slug: "team"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(context.Background(), "team", uuid.New(), "superuser"); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

func TestCredentialLifecycleAndValidation(t *testing.T) {
	svc, _, _, _ := newProjects(t)
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
	svc, _, _, _ := newProjects(t)
	missing := uuid.New()
	_, err := svc.Create(context.Background(), member(), CreateProjectInput{
		Slug: "team", RegistryCredentialID: &missing,
	})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
