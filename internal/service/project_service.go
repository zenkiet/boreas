package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

type ProjectService struct {
	projects    core.ProjectStore
	credentials core.CredentialStore
}

func NewProjectService(projects core.ProjectStore, credentials core.CredentialStore) (*ProjectService, error) {
	if projects == nil || credentials == nil {
		return nil, errors.Join(core.ErrInvalidInput, errors.New("project and credential stores are required"))
	}
	return &ProjectService{projects: projects, credentials: credentials}, nil
}

// List scopes non-admin results to memberships to enforce project visibility.
func (s *ProjectService) List(ctx context.Context, actor core.User) ([]core.Project, error) {
	var (
		projects []core.Project
		err      error
	)
	if actor.IsAdmin() {
		projects, err = s.projects.List(ctx)
	} else {
		projects, err = s.projects.ListForUser(ctx, actor.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return projects, nil
}

func (s *ProjectService) Get(ctx context.Context, slug string) (core.Project, error) {
	if err := core.ValidateProjectSlug(slug); err != nil {
		return core.Project{}, err
	}
	project, err := s.projects.GetBySlug(ctx, slug)
	if err != nil {
		return core.Project{}, fmt.Errorf("get project %q: %w", slug, err)
	}
	return project, nil
}

type CreateProjectInput struct {
	Slug                 string
	Name                 string
	RegistryCredentialID *uuid.UUID
}

// Create makes the caller an owner so the project is never ownerless.
func (s *ProjectService) Create(ctx context.Context, actor core.User, in CreateProjectInput) (core.Project, error) {
	if err := core.ValidateProjectSlug(in.Slug); err != nil {
		return core.Project{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = in.Slug
	}
	if err := s.checkCredential(ctx, in.RegistryCredentialID); err != nil {
		return core.Project{}, err
	}
	project, err := s.projects.Create(ctx, core.Project{
		Slug: in.Slug, Name: name, RegistryCredentialID: in.RegistryCredentialID, CreatedBy: &actor.ID,
	})
	if err != nil {
		return core.Project{}, fmt.Errorf("create project: %w", err)
	}
	if err := s.projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: actor.ID, Role: core.ProjectRoleOwner,
	}); err != nil {
		return core.Project{}, fmt.Errorf("add project owner: %w", err)
	}
	return project, nil
}

type UpdateProjectInput struct {
	Name                 *string
	RegistryCredentialID **uuid.UUID
}

func (s *ProjectService) Update(ctx context.Context, slug string, in UpdateProjectInput) (core.Project, error) {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return core.Project{}, err
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return core.Project{}, errors.Join(core.ErrInvalidInput, errors.New("project name must not be empty"))
		}
		project.Name = *in.Name
	}
	if in.RegistryCredentialID != nil {
		if err := s.checkCredential(ctx, *in.RegistryCredentialID); err != nil {
			return core.Project{}, err
		}
		project.RegistryCredentialID = *in.RegistryCredentialID
	}
	updated, err := s.projects.Update(ctx, project)
	if err != nil {
		return core.Project{}, fmt.Errorf("update project: %w", err)
	}
	return updated, nil
}

func (s *ProjectService) Delete(ctx context.Context, slug string) error {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	if err := s.projects.Delete(ctx, project.ID); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func (s *ProjectService) ListMembers(ctx context.Context, slug string) ([]core.ProjectMember, error) {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	members, err := s.projects.ListMembers(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

func (s *ProjectService) AddMember(ctx context.Context, slug string, userID uuid.UUID, role core.ProjectRole) error {
	if role != core.ProjectRoleOwner && role != core.ProjectRoleMember {
		return errors.Join(core.ErrInvalidInput, errors.New("role must be owner or member"))
	}
	project, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	if err := s.projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: userID, Role: role,
	}); err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// RemoveMember refuses to leave a project without an owner.
func (s *ProjectService) RemoveMember(ctx context.Context, slug string, userID uuid.UUID) error {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	members, err := s.projects.ListMembers(ctx, project.ID)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}
	owners, targetIsOwner := 0, false
	for _, member := range members {
		if member.Role != core.ProjectRoleOwner {
			continue
		}
		owners++
		if member.UserID == userID {
			targetIsOwner = true
		}
	}
	if targetIsOwner && owners == 1 {
		return fmt.Errorf("cannot remove the last owner of project %q: %w", slug, core.ErrConflict)
	}
	if err := s.projects.RemoveMember(ctx, project.ID, userID); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// Access treats administrators as owners of every project.
func (s *ProjectService) Access(ctx context.Context, actor core.User, slug string) (core.ProjectRole, error) {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return "", err
	}
	if actor.IsAdmin() {
		return core.ProjectRoleOwner, nil
	}
	member, err := s.projects.GetMember(ctx, project.ID, actor.ID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return "", core.ErrForbidden
		}
		return "", fmt.Errorf("get project member: %w", err)
	}
	return member.Role, nil
}

func (s *ProjectService) ListCredentials(ctx context.Context) ([]core.RegistryCredential, error) {
	credentials, err := s.credentials.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	return credentials, nil
}

type CreateCredentialInput struct {
	Name     string
	Registry core.RegistryKind
	Username string
	Token    string
}

func (s *ProjectService) CreateCredential(ctx context.Context, actor core.User, in CreateCredentialInput) (core.RegistryCredential, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Username) == "" || strings.TrimSpace(in.Token) == "" {
		return core.RegistryCredential{}, errors.Join(core.ErrInvalidInput,
			errors.New("credential name, username, and token are required"))
	}
	if in.Registry != core.RegistryGHCR && in.Registry != core.RegistryDockerHub {
		return core.RegistryCredential{}, errors.Join(core.ErrInvalidInput,
			errors.New("registry must be ghcr or dockerhub"))
	}
	credential, err := s.credentials.Create(ctx, core.RegistryCredential{
		Name: in.Name, Registry: in.Registry, Username: in.Username, Token: in.Token, CreatedBy: &actor.ID,
	})
	if err != nil {
		return core.RegistryCredential{}, fmt.Errorf("create credential: %w", err)
	}
	return credential, nil
}

func (s *ProjectService) DeleteCredential(ctx context.Context, id uuid.UUID) error {
	if err := s.credentials.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	return nil
}

func (s *ProjectService) checkCredential(ctx context.Context, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	if _, err := s.credentials.Get(ctx, *id); err != nil {
		return fmt.Errorf("get registry credential: %w", err)
	}
	return nil
}
