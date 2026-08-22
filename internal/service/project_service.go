package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

type ProjectService struct {
	projects      core.ProjectStore
	credentials   core.CredentialStore
	notifications core.NotificationStore
	grants        core.GrantStore
	tasks         core.TaskStore
}

func NewProjectService(
	projects core.ProjectStore, credentials core.CredentialStore,
	notifications core.NotificationStore, grants core.GrantStore, tasks core.TaskStore,
) (*ProjectService, error) {
	if projects == nil || credentials == nil || notifications == nil || grants == nil || tasks == nil {
		return nil, errors.Join(core.ErrInvalidInput,
			errors.New("project, credential, notification, grant, and task stores are required"))
	}
	return &ProjectService{
		projects: projects, credentials: credentials,
		notifications: notifications, grants: grants, tasks: tasks,
	}, nil
}

func (s *ProjectService) Notifications(ctx context.Context, acc core.ProjectAccess, limit int) ([]core.Notification, error) {
	notifications, err := s.notifications.List(ctx, acc.Project.ID, acc.UserID, acc.AllTasks, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return notifications, nil
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
	DefaultImage         string
	DefaultPort          int
	DefaultEnv           map[string]string
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
	port := in.DefaultPort
	if port == 0 {
		port = 80
	}
	if port < 1 || port > 65535 {
		return core.Project{}, errors.Join(core.ErrInvalidInput, errors.New("default port must be between 1 and 65535"))
	}
	if err := core.ValidateEnv(in.DefaultEnv); err != nil {
		return core.Project{}, err
	}
	if err := s.checkCredential(ctx, in.RegistryCredentialID); err != nil {
		return core.Project{}, err
	}
	project, err := s.projects.Create(ctx, core.Project{
		Slug: in.Slug, Name: name, RegistryCredentialID: in.RegistryCredentialID,
		DefaultImage: strings.TrimSpace(in.DefaultImage), DefaultPort: port,
		DefaultEnv: maps.Clone(in.DefaultEnv), CreatedBy: &actor.ID,
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
	DefaultImage         *string
	DefaultPort          *int
	DefaultEnv           *map[string]string
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
	if in.DefaultImage != nil {
		project.DefaultImage = strings.TrimSpace(*in.DefaultImage)
	}
	if in.DefaultPort != nil {
		if *in.DefaultPort < 1 || *in.DefaultPort > 65535 {
			return core.Project{}, errors.Join(core.ErrInvalidInput, errors.New("default port must be between 1 and 65535"))
		}
		project.DefaultPort = *in.DefaultPort
	}
	if in.DefaultEnv != nil {
		if err := core.ValidateEnv(*in.DefaultEnv); err != nil {
			return core.Project{}, err
		}
		project.DefaultEnv = maps.Clone(*in.DefaultEnv)
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
	if role.Rank() == 0 {
		return errors.Join(core.ErrInvalidInput,
			errors.New("role must be viewer, operator, member, or owner"))
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

// Access resolves the caller's effective role, taking the higher of their project
// membership and any grant on the requested task. Administrators own every project.
func (s *ProjectService) Access(ctx context.Context, actor core.User, slug, taskName string) (core.ProjectAccess, error) {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return core.ProjectAccess{}, err
	}
	acc := core.ProjectAccess{Project: project, UserID: actor.ID}
	if actor.IsAdmin() {
		acc.Role, acc.AllTasks = core.ProjectRoleOwner, true
		return acc, nil
	}
	member, err := s.projects.GetMember(ctx, project.ID, actor.ID)
	switch {
	case err == nil:
		acc.Role, acc.AllTasks = member.Role, true
	case !errors.Is(err, core.ErrNotFound):
		return core.ProjectAccess{}, fmt.Errorf("get project member: %w", err)
	}
	granted, err := s.grantedRole(ctx, project.ID, actor.ID, taskName)
	if err != nil {
		return core.ProjectAccess{}, err
	}
	if granted.Rank() > acc.Role.Rank() {
		acc.Role = granted
	}
	// An unreachable project or task must be indistinguishable from one that does not exist.
	if acc.Role == "" {
		return core.ProjectAccess{}, core.ErrNotFound
	}
	return acc, nil
}

// grantedRole reports what task grants alone give the caller. Routes without a task in
// their path get viewer, because holding any grant proves the caller belongs in the project.
func (s *ProjectService) grantedRole(
	ctx context.Context, projectID, userID uuid.UUID, taskName string,
) (core.ProjectRole, error) {
	if taskName == "" {
		granted, err := s.grants.AnyInProject(ctx, projectID, userID)
		if err != nil {
			return "", fmt.Errorf("check task grants: %w", err)
		}
		if granted {
			return core.ProjectRoleViewer, nil
		}
		return "", nil
	}
	role, err := s.grants.Role(ctx, projectID, userID, taskName)
	if err != nil {
		return "", fmt.Errorf("get task grant: %w", err)
	}
	return role, nil
}

func (s *ProjectService) ListGrants(ctx context.Context, slug, taskName string) ([]core.TaskGrant, error) {
	task, err := s.task(ctx, slug, taskName)
	if err != nil {
		return nil, err
	}
	grants, err := s.grants.ListForTask(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("list task grants: %w", err)
	}
	return grants, nil
}

// Grant raises a user's role on one task. Owner is rejected because it only means
// something at project scope.
func (s *ProjectService) Grant(ctx context.Context, slug, taskName string, userID uuid.UUID, role core.ProjectRole) error {
	if role.Rank() < core.ProjectRoleViewer.Rank() || role.Rank() > core.ProjectRoleMember.Rank() {
		return errors.Join(core.ErrInvalidInput, errors.New("role must be viewer, operator, or member"))
	}
	task, err := s.task(ctx, slug, taskName)
	if err != nil {
		return err
	}
	if err := s.grants.Grant(ctx, core.TaskGrant{TaskID: task.ID, UserID: userID, Role: role}); err != nil {
		return fmt.Errorf("grant task: %w", err)
	}
	return nil
}

func (s *ProjectService) Revoke(ctx context.Context, slug, taskName string, userID uuid.UUID) error {
	task, err := s.task(ctx, slug, taskName)
	if err != nil {
		return err
	}
	if err := s.grants.Revoke(ctx, task.ID, userID); err != nil {
		return fmt.Errorf("revoke task grant: %w", err)
	}
	return nil
}

func (s *ProjectService) task(ctx context.Context, slug, taskName string) (core.Task, error) {
	if err := core.ValidateTaskName(taskName); err != nil {
		return core.Task{}, err
	}
	project, err := s.Get(ctx, slug)
	if err != nil {
		return core.Task{}, err
	}
	task, err := s.tasks.GetByName(ctx, project.ID, taskName)
	if err != nil {
		return core.Task{}, fmt.Errorf("get task %q: %w", taskName, err)
	}
	return task, nil
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
