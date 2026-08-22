// Package httptransport exposes the Boreas application service over HTTP.
package httptransport

import (
	"context"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/service"
)

type TaskService interface {
	List(ctx context.Context, acc core.ProjectAccess) ([]core.Task, error)
	Get(ctx context.Context, project, name string) (core.Task, error)
	Create(ctx context.Context, project string, in service.CreateTaskInput) (core.Task, error)
	Update(ctx context.Context, project, name string, in service.UpdateTaskInput, recreate bool) (core.Task, error)
	Deploy(ctx context.Context, project, name, image string) (core.Task, error)
	Start(ctx context.Context, project, name string) (core.Task, error)
	Stop(ctx context.Context, project, name string) (core.Task, error)
	Restart(ctx context.Context, project, name string) (core.Task, error)
	Delete(ctx context.Context, project, name string) error
	Logs(ctx context.Context, project, name string, opts core.LogOptions) (io.ReadCloser, error)
	SystemStats(context.Context) (core.SystemStats, error)
}

type AuthService interface {
	Login(ctx context.Context, username, password string) (string, core.User, error)
	Authenticate(ctx context.Context, token string) (core.User, core.TokenKind, error)
	Logout(ctx context.Context, token string) error
	CreateAPIToken(ctx context.Context, userID uuid.UUID, in service.CreateAPITokenInput) (string, core.AuthToken, error)
	ListAPITokens(ctx context.Context, userID uuid.UUID) ([]core.AuthToken, error)
	RevokeAPIToken(ctx context.Context, userID, tokenID uuid.UUID) error
	ListUsers(context.Context) ([]core.User, error)
	CreateUser(context.Context, service.CreateUserInput) (core.User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, in service.UpdateUserInput) (core.User, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

type ProjectService interface {
	List(ctx context.Context, actor core.User) ([]core.Project, error)
	Get(ctx context.Context, slug string) (core.Project, error)
	Create(ctx context.Context, actor core.User, in service.CreateProjectInput) (core.Project, error)
	Update(ctx context.Context, slug string, in service.UpdateProjectInput) (core.Project, error)
	Delete(ctx context.Context, slug string) error
	ListMembers(ctx context.Context, slug string) ([]core.ProjectMember, error)
	AddMember(ctx context.Context, slug string, userID uuid.UUID, role core.ProjectRole) error
	RemoveMember(ctx context.Context, slug string, userID uuid.UUID) error
	Notifications(ctx context.Context, acc core.ProjectAccess, limit int) ([]core.Notification, error)
	Access(ctx context.Context, actor core.User, slug, taskName string) (core.ProjectAccess, error)
	ListGrants(ctx context.Context, slug, taskName string) ([]core.TaskGrant, error)
	Grant(ctx context.Context, slug, taskName string, userID uuid.UUID, role core.ProjectRole) error
	Revoke(ctx context.Context, slug, taskName string, userID uuid.UUID) error
	ListCredentials(context.Context) ([]core.RegistryCredential, error)
	CreateCredential(ctx context.Context, actor core.User, in service.CreateCredentialInput) (core.RegistryCredential, error)
	DeleteCredential(ctx context.Context, id uuid.UUID) error
}

const maxRequestBytes = 1 << 20

// APIHandler serves the API and applies each route's declared access policy.
func APIHandler(tasks TaskService, auth AuthService, projects ProjectService, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	h := &Handler{tasks: tasks, auth: auth, projects: projects, logger: logger}

	mux := http.NewServeMux()
	for _, r := range routeTable {
		handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.handler(h, w, req) })
		if r.access != accessPublic {
			handler = h.authorize(r.access, handler)
		}
		mux.Handle(r.method+" "+r.path, handler)
	}
	mux.HandleFunc("GET /api/v1/openapi.json", h.openapiJSON)
	mux.HandleFunc("GET /api/v1/docs", h.docsPage)

	return cors(mux)
}
