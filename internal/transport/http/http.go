// Package httptransport exposes the Boreas application service over HTTP.
package httptransport

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"github.com/zenkiet/boreas/internal/service"
)

type TaskService interface {
	List(ctx context.Context, project string) ([]core.Task, error)
	Get(ctx context.Context, project, name string) (core.Task, error)
	Create(ctx context.Context, project string, in service.CreateTaskInput) (core.Task, error)
	Update(ctx context.Context, project, name string, in service.UpdateTaskInput, recreate bool) (core.Task, error)
	Start(ctx context.Context, project, name string) (core.Task, error)
	Stop(ctx context.Context, project, name string) (core.Task, error)
	Restart(ctx context.Context, project, name string) (core.Task, error)
	Delete(ctx context.Context, project, name string) error
	GetEnv(ctx context.Context, project, name string) (map[string]string, error)
	UpdateEnv(ctx context.Context, project, name string, env map[string]string, recreate bool) (core.Task, error)
	Logs(ctx context.Context, project, name string, opts core.LogOptions) (io.ReadCloser, error)
	SystemStats(context.Context) (core.SystemStats, error)
}

type AuthService interface {
	Login(ctx context.Context, username, password string) (string, core.User, error)
	Authenticate(ctx context.Context, token string) (core.User, error)
	Logout(ctx context.Context, token string) error
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
	Access(ctx context.Context, actor core.User, slug string) (core.ProjectRole, error)
	ListCredentials(context.Context) ([]core.RegistryCredential, error)
	CreateCredential(ctx context.Context, actor core.User, in service.CreateCredentialInput) (core.RegistryCredential, error)
	DeleteCredential(ctx context.Context, id uuid.UUID) error
}

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedHeaders   []string
	AllowCredentials bool
}

type Options struct {
	Logger          *log.Logger
	MaxRequestBytes int64
	CORS            CORSConfig
	Heartbeat       time.Duration
	// Docs exposes public API documentation but no application data.
	Docs bool
}

func (o Options) defaults() Options {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	if o.MaxRequestBytes <= 0 {
		o.MaxRequestBytes = 1 << 20
	}
	if o.Heartbeat <= 0 {
		o.Heartbeat = 15 * time.Second
	}
	if len(o.CORS.AllowedHeaders) == 0 {
		o.CORS.AllowedHeaders = []string{"Authorization", "Content-Type"}
	}
	return o
}

// APIHandler protects every route except health and login; project routes also enforce membership.
func APIHandler(tasks TaskService, auth AuthService, projects ProjectService, options Options) http.Handler {
	o := options.defaults()
	h := &Handler{tasks: tasks, auth: auth, projects: projects, options: o}

	mux := http.NewServeMux()
	for _, r := range routeTable {
		handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.handler(h, w, req) })
		if r.access != accessPublic {
			handler = h.authorize(r.access, handler)
		}
		mux.Handle(r.method+" "+r.path, handler)
	}
	if o.Docs {
		mux.HandleFunc("GET /api/v1/openapi.json", h.openapiJSON)
		mux.HandleFunc("GET /api/v1/docs", h.docsPage)
	}

	return cors(o.CORS)(mux)
}
