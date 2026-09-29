package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
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
	// Optional wiring: Notify reports task assignments; Users, when also set, names the grantee.
	Notify func(context.Context, core.Notification)
	Users  core.UserStore
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

func (s *ProjectService) Notifications(
	ctx context.Context, actor core.User, projectID, before *uuid.UUID, limit int,
) ([]core.Notification, error) {
	notifications, err := s.notifications.List(ctx, actor.ID, actor.IsAdmin(), projectID, before, limit)
	return notifications, wrap("list notifications", err)
}

func (s *ProjectService) MarkNotificationsSeen(ctx context.Context, actor core.User, ids []uuid.UUID) error {
	return wrap("mark notifications seen", s.notifications.MarkSeen(ctx, actor.ID, actor.IsAdmin(), ids))
}

func (s *ProjectService) MarkNotificationUnseen(ctx context.Context, acc core.ProjectAccess, id uuid.UUID) error {
	return wrap("mark notification unseen", s.notifications.MarkUnseen(ctx, id, acc.UserID))
}

// List reports each reachable project with the caller's role in it; administrators own them all.
func (s *ProjectService) List(ctx context.Context, actor core.User) ([]core.ProjectAccess, error) {
	if !actor.IsAdmin() {
		accesses, err := s.projects.ListForUser(ctx, actor.ID)
		return accesses, wrap("list projects", err)
	}
	projects, err := s.projects.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	accesses := make([]core.ProjectAccess, len(projects))
	for i, project := range projects {
		accesses[i] = core.ProjectAccess{Project: project, UserID: actor.ID, Role: core.ProjectRoleOwner, AllTasks: true}
	}
	return accesses, nil
}

// Fleet groups by project ID every task the actor reaches, with their effective role on each.
// ponytail: one task query per project; fold into one SQL query if projects reach the hundreds.
func (s *ProjectService) Fleet(ctx context.Context, actor core.User) ([]core.ProjectAccess, map[uuid.UUID][]core.FleetTask, error) {
	accesses, err := s.List(ctx, actor)
	if err != nil {
		return nil, nil, err
	}
	grants, err := s.GrantedRoles(ctx, actor.ID)
	if err != nil {
		return nil, nil, err
	}
	deploys, err := s.notifications.LastDeploys(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list last deploys: %w", err)
	}
	fleet := make(map[uuid.UUID][]core.FleetTask, len(accesses))
	for _, acc := range accesses {
		tasks, err := s.tasks.List(ctx, acc.Project.ID, acc.UserID, acc.AllTasks)
		if err != nil {
			return nil, nil, fmt.Errorf("list tasks: %w", err)
		}
		for _, task := range tasks {
			entry := core.FleetTask{Task: task, Role: acc.Role.Max(grants[task.ID])}
			if deploy, ok := deploys[task.ID]; ok {
				entry.LastDeploy = &deploy
			}
			fleet[acc.Project.ID] = append(fleet[acc.Project.ID], entry)
		}
	}
	return accesses, fleet, nil
}

func (s *ProjectService) GrantedRoles(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]core.ProjectRole, error) {
	roles, err := s.grants.ForUser(ctx, userID)
	return roles, wrap("list user grants", err)
}

// Directory lists every project and task for the anonymous environment index.
func (s *ProjectService) Directory(ctx context.Context) ([]core.Project, []core.Task, error) {
	projects, err := s.projects.List(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list projects: %w", err)
	}
	tasks, err := s.tasks.ListAll(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list tasks: %w", err)
	}
	return projects, tasks, nil
}

func (s *ProjectService) Get(ctx context.Context, slug string) (core.Project, error) {
	return getProject(ctx, s.projects, slug)
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
	name := cmp.Or(strings.TrimSpace(in.Name), in.Slug)
	port := cmp.Or(in.DefaultPort, 80)
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
	return updated, wrap("update project", err)
}

func (s *ProjectService) Delete(ctx context.Context, slug string) error {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	return wrap("delete project", s.projects.Delete(ctx, project.ID))
}

func (s *ProjectService) ListMembers(ctx context.Context, slug string) ([]core.ProjectMember, error) {
	project, err := s.Get(ctx, slug)
	if err != nil {
		return nil, err
	}
	members, err := s.projects.ListMembers(ctx, project.ID)
	return members, wrap("list members", err)
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
	return wrap("add member", s.projects.AddMember(ctx, core.ProjectMember{
		ProjectID: project.ID, UserID: userID, Role: role,
	}))
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
	owners := slices.DeleteFunc(members, func(m core.ProjectMember) bool { return m.Role != core.ProjectRoleOwner })
	if len(owners) == 1 && owners[0].UserID == userID {
		return fmt.Errorf("cannot remove the last owner of project %q: %w", slug, core.ErrConflict)
	}
	return wrap("remove member", s.projects.RemoveMember(ctx, project.ID, userID))
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
	acc.Role = acc.Role.Max(granted)
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
	return role, wrap("get task grant", err)
}

func (s *ProjectService) ListGrants(ctx context.Context, slug, taskName string) ([]core.TaskGrant, error) {
	task, _, err := getTask(ctx, s.projects, s.tasks, slug, taskName)
	if err != nil {
		return nil, err
	}
	grants, err := s.grants.ListForTask(ctx, task.ID)
	return grants, wrap("list task grants", err)
}

// Grant raises a user's role on one task; owner is rejected as it only means something at project scope.
func (s *ProjectService) Grant(ctx context.Context, slug, taskName string, userID uuid.UUID, role core.ProjectRole) error {
	if role.Rank() < core.ProjectRoleViewer.Rank() || role.Rank() > core.ProjectRoleMember.Rank() {
		return errors.Join(core.ErrInvalidInput, errors.New("role must be viewer, operator, or member"))
	}
	task, project, err := getTask(ctx, s.projects, s.tasks, slug, taskName)
	if err != nil {
		return err
	}
	if err := s.grants.Grant(ctx, core.TaskGrant{TaskID: task.ID, UserID: userID, Role: role}); err != nil {
		return fmt.Errorf("grant task: %w", err)
	}
	if s.Notify != nil {
		detail := string(role) + " access granted"
		if s.Users != nil {
			if user, err := s.Users.Get(ctx, userID); err == nil {
				detail = "assigned to " + user.Username + " (" + string(role) + ")"
			}
		}
		s.Notify(ctx, notification(project, taskName, core.NotificationTaskAssigned, "👤 Task Assigned", detail))
	}
	return nil
}

func (s *ProjectService) Revoke(ctx context.Context, slug, taskName string, userID uuid.UUID) error {
	task, _, err := getTask(ctx, s.projects, s.tasks, slug, taskName)
	if err != nil {
		return err
	}
	return wrap("revoke task grant", s.grants.Revoke(ctx, task.ID, userID))
}

func (s *ProjectService) ListCredentials(ctx context.Context) ([]core.RegistryCredential, error) {
	credentials, err := s.credentials.List(ctx)
	return credentials, wrap("list credentials", err)
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
	return credential, wrap("create credential", err)
}

func (s *ProjectService) DeleteCredential(ctx context.Context, id uuid.UUID) error {
	return wrap("delete credential", s.credentials.Delete(ctx, id))
}

func (s *ProjectService) checkCredential(ctx context.Context, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	_, err := s.credentials.Get(ctx, *id)
	return wrap("get registry credential", err)
}
