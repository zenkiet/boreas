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
	get    func(context.Context, string, string) (core.Task, error)
	create func(context.Context, string, service.CreateTaskInput) (core.Task, error)
	update func(context.Context, string, string, service.UpdateTaskInput, bool) (core.Task, error)
	logs   func(context.Context, string, string, core.LogOptions) (io.ReadCloser, error)
	stats  func(context.Context) (core.SystemStats, error)
}

func (stubTasks) List(context.Context, string) ([]core.Task, error) { return nil, nil }

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

func (stubTasks) Start(context.Context, string, string) (core.Task, error) { return core.Task{}, nil }
func (stubTasks) Stop(context.Context, string, string) (core.Task, error)  { return core.Task{}, nil }
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

func (s stubTasks) SystemStats(c context.Context) (core.SystemStats, error) {
	if s.stats != nil {
		return s.stats(c)
	}
	return core.SystemStats{}, nil
}

type stubAuth struct {
	user      core.User
	login     func(context.Context, string, string) (string, core.User, error)
	listUsers func(context.Context) ([]core.User, error)
	loggedOut []string
}

func (s *stubAuth) Login(c context.Context, username, password string) (string, core.User, error) {
	if s.login != nil {
		return s.login(c, username, password)
	}
	return testToken, s.user, nil
}

func (s *stubAuth) Authenticate(_ context.Context, token string) (core.User, error) {
	if token != testToken {
		return core.User{}, core.ErrUnauthorized
	}
	return s.user, nil
}

func (s *stubAuth) Logout(_ context.Context, token string) error {
	s.loggedOut = append(s.loggedOut, token)
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
	accessErr       error
	update          func(context.Context, string, service.UpdateProjectInput) (core.Project, error)
	listCredentials func(context.Context) ([]core.RegistryCredential, error)
}

func (s *stubProjects) Access(_ context.Context, actor core.User, _ string) (core.ProjectRole, error) {
	if s.accessErr != nil {
		return "", s.accessErr
	}
	if actor.IsAdmin() {
		return core.ProjectRoleOwner, nil
	}
	if s.role == "" {
		return core.ProjectRoleMember, nil
	}
	return s.role, nil
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
