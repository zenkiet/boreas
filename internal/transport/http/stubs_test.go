package httptransport

import (
	"context"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/service"
)

const testToken = "test-token"

var (
	testAdmin  = core.User{ID: uuid.New(), Username: "admin", Email: "a@example.com", Role: core.RoleAdmin}
	testMember = core.User{ID: uuid.New(), Username: "member", Email: "m@example.com", Role: core.RoleUser}
)

type stubTasks struct {
	get     func(context.Context, string, string) (core.Task, error)
	create  func(context.Context, string, service.CreateTaskInput) (core.Task, error)
	update  func(context.Context, string, string, service.UpdateTaskInput, bool) (core.Task, error)
	deploy  func(context.Context, string, string, string) (core.Task, error)
	logs    func(context.Context, string, string, core.LogOptions) (io.ReadCloser, error)
	stats   func(context.Context) (core.SystemStats, error)
	list    func(context.Context, core.ProjectAccess) ([]core.Task, error)
	metrics func(context.Context, core.ProjectAccess, string) (<-chan core.TaskMetric, error)
}

func (s stubTasks) List(c context.Context, acc core.ProjectAccess) ([]core.Task, error) {
	if s.list != nil {
		return s.list(c, acc)
	}
	return nil, nil
}

func (s stubTasks) Get(c context.Context, project, name string) (core.Task, error) {
	if s.get != nil {
		return s.get(c, project, name)
	}
	return core.Task{}, nil
}

func (s stubTasks) Create(c context.Context, project string, in service.CreateTaskInput) (core.Task, error) {
	if s.create != nil {
		return s.create(c, project, in)
	}
	return core.Task{}, nil
}

func (s stubTasks) Update(c context.Context, project, name string, in service.UpdateTaskInput, recreate bool) (core.Task, error) {
	if s.update != nil {
		return s.update(c, project, name, in, recreate)
	}
	return core.Task{}, nil
}

func (s stubTasks) Deploy(c context.Context, project, name, image string) (core.Task, error) {
	if s.deploy != nil {
		return s.deploy(c, project, name, image)
	}
	return core.Task{}, nil
}

func (stubTasks) Start(context.Context, string, string) (core.Task, error) { return core.Task{}, nil }

func (stubTasks) Stop(context.Context, string, string) (core.Task, error) { return core.Task{}, nil }

func (stubTasks) Restart(context.Context, string, string) (core.Task, error) {
	return core.Task{}, nil
}
func (stubTasks) Delete(context.Context, string, string) error { return nil }

func (s stubTasks) Logs(c context.Context, project, name string, opts core.LogOptions) (io.ReadCloser, error) {
	if s.logs != nil {
		return s.logs(c, project, name, opts)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (s stubTasks) Metrics(c context.Context, acc core.ProjectAccess, name string) (<-chan core.TaskMetric, error) {
	if s.metrics != nil {
		return s.metrics(c, acc, name)
	}
	closed := make(chan core.TaskMetric)
	close(closed)
	return closed, nil
}

func (s stubTasks) SystemStats(c context.Context) (core.SystemStats, error) {
	if s.stats != nil {
		return s.stats(c)
	}
	return core.SystemStats{}, nil
}

type stubAuth struct {
	user      core.User
	login     func(context.Context, string, string) (string, core.User, error)
	auth      func(context.Context, string) (core.User, core.TokenKind, error)
	createAPI func(context.Context, uuid.UUID, service.CreateAPITokenInput) (string, core.AuthToken, error)
	listAPI   func(context.Context, uuid.UUID) ([]core.AuthToken, error)
	revokeAPI func(context.Context, uuid.UUID, uuid.UUID) error
	listUsers func(context.Context) ([]core.User, error)
	loggedOut []string
}

func (s *stubAuth) Login(c context.Context, username, password string) (string, core.User, error) {
	if s.login != nil {
		return s.login(c, username, password)
	}
	return testToken, s.user, nil
}

func (s *stubAuth) Authenticate(c context.Context, token string) (core.User, core.TokenKind, error) {
	if s.auth != nil {
		return s.auth(c, token)
	}
	if token != testToken {
		return core.User{}, "", core.ErrUnauthorized
	}
	return s.user, core.TokenKindSession, nil
}

func (s *stubAuth) Logout(_ context.Context, token string) error {
	s.loggedOut = append(s.loggedOut, token)
	return nil
}

func (s *stubAuth) CreateAPIToken(c context.Context, userID uuid.UUID, in service.CreateAPITokenInput) (string, core.AuthToken, error) {
	if s.createAPI != nil {
		return s.createAPI(c, userID, in)
	}
	return "", core.AuthToken{}, nil
}

func (s *stubAuth) ListAPITokens(c context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
	if s.listAPI != nil {
		return s.listAPI(c, userID)
	}
	return nil, nil
}

func (s *stubAuth) RevokeAPIToken(c context.Context, userID, tokenID uuid.UUID) error {
	if s.revokeAPI != nil {
		return s.revokeAPI(c, userID, tokenID)
	}
	return nil
}

func (s *stubAuth) ListUsers(c context.Context) ([]core.User, error) {
	if s.listUsers != nil {
		return s.listUsers(c)
	}
	return nil, nil
}

func (*stubAuth) CreateUser(context.Context, service.CreateUserInput) (core.User, error) {
	return core.User{}, nil
}

func (*stubAuth) UpdateUser(context.Context, uuid.UUID, service.UpdateUserInput) (core.User, error) {
	return core.User{}, nil
}
func (*stubAuth) DeleteUser(context.Context, uuid.UUID) error { return nil }

type stubProjects struct {
	role            core.ProjectRole
	project         core.Project
	allTasks        *bool
	accessErr       error
	update          func(context.Context, string, service.UpdateProjectInput) (core.Project, error)
	listCredentials func(context.Context) ([]core.RegistryCredential, error)
	notifications   func(context.Context, core.ProjectAccess, int) ([]core.Notification, error)
	markSeen        func(context.Context, core.ProjectAccess, uuid.UUID) error
	grant           func(context.Context, string, string, uuid.UUID, core.ProjectRole) error
	listGrants      func(context.Context, string, string) ([]core.TaskGrant, error)
	revoke          func(context.Context, string, string, uuid.UUID) error
}

func (s *stubProjects) Access(_ context.Context, actor core.User, _, _ string) (core.ProjectAccess, error) {
	if s.accessErr != nil {
		return core.ProjectAccess{}, s.accessErr
	}
	acc := core.ProjectAccess{Project: s.project, UserID: actor.ID, AllTasks: true}
	switch {
	case actor.IsAdmin():
		acc.Role = core.ProjectRoleOwner
	case s.role == "":
		acc.Role = core.ProjectRoleMember
	default:
		acc.Role = s.role
	}
	if s.allTasks != nil {
		acc.AllTasks = *s.allTasks
	}
	return acc, nil
}

func (*stubProjects) List(context.Context, core.User) ([]core.Project, error) { return nil, nil }
func (*stubProjects) Get(context.Context, string) (core.Project, error)       { return core.Project{}, nil }

func (*stubProjects) Create(context.Context, core.User, service.CreateProjectInput) (core.Project, error) {
	return core.Project{}, nil
}

func (s *stubProjects) Update(c context.Context, slug string, in service.UpdateProjectInput) (core.Project, error) {
	if s.update != nil {
		return s.update(c, slug, in)
	}
	return core.Project{}, nil
}

func (s *stubProjects) Notifications(c context.Context, acc core.ProjectAccess, limit int) ([]core.Notification, error) {
	if s.notifications != nil {
		return s.notifications(c, acc, limit)
	}
	return nil, nil
}

func (s *stubProjects) MarkNotificationSeen(c context.Context, acc core.ProjectAccess, id uuid.UUID) error {
	if s.markSeen != nil {
		return s.markSeen(c, acc, id)
	}
	return nil
}

func (*stubProjects) MarkNotificationUnseen(context.Context, core.ProjectAccess, uuid.UUID) error {
	return nil
}

func (s *stubProjects) ListGrants(c context.Context, slug, name string) ([]core.TaskGrant, error) {
	if s.listGrants != nil {
		return s.listGrants(c, slug, name)
	}
	return nil, nil
}

func (s *stubProjects) Grant(c context.Context, slug, name string, userID uuid.UUID, role core.ProjectRole) error {
	if s.grant != nil {
		return s.grant(c, slug, name, userID, role)
	}
	return nil
}

func (s *stubProjects) Revoke(c context.Context, slug, name string, userID uuid.UUID) error {
	if s.revoke != nil {
		return s.revoke(c, slug, name, userID)
	}
	return nil
}

func (*stubProjects) Delete(context.Context, string) error { return nil }
func (*stubProjects) ListMembers(context.Context, string) ([]core.ProjectMember, error) {
	return nil, nil
}

func (*stubProjects) AddMember(context.Context, string, uuid.UUID, core.ProjectRole) error {
	return nil
}
func (*stubProjects) RemoveMember(context.Context, string, uuid.UUID) error { return nil }

func (s *stubProjects) ListCredentials(c context.Context) ([]core.RegistryCredential, error) {
	if s.listCredentials != nil {
		return s.listCredentials(c)
	}
	return nil, nil
}

func (*stubProjects) CreateCredential(context.Context, core.User, service.CreateCredentialInput) (core.RegistryCredential, error) {
	return core.RegistryCredential{}, nil
}

func (*stubProjects) DeleteCredential(context.Context, uuid.UUID) error { return nil }

type stubPush struct {
	create func(context.Context, uuid.UUID, string) error
	delete func(context.Context, uuid.UUID, string) error
}

func (s *stubPush) Create(c context.Context, userID uuid.UUID, token string) error {
	if s.create != nil {
		return s.create(c, userID, token)
	}
	return nil
}

func (s *stubPush) Delete(c context.Context, userID uuid.UUID, token string) error {
	if s.delete != nil {
		return s.delete(c, userID, token)
	}
	return nil
}
