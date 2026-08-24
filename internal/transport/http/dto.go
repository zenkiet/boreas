package httptransport

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/swaggest/jsonschema-go"
	"github.com/zenkiet/boreas/internal/core"
)

type errorResponse struct {
	Error string `json:"error" example:"not found"`
}

type successResponse struct {
	Success bool `json:"success" example:"true"`
}

type apiTokenStatus string

const (
	apiTokenScheduled apiTokenStatus = "scheduled"
	apiTokenActive    apiTokenStatus = "active"
	apiTokenExpired   apiTokenStatus = "expired"
	apiTokenRevoked   apiTokenStatus = "revoked"
)

func (apiTokenStatus) Enum() []any {
	return []any{apiTokenScheduled, apiTokenActive, apiTokenExpired, apiTokenRevoked}
}

type apiTokenDTO struct {
	ID        uuid.UUID      `json:"id"`
	Name      string         `json:"name" example:"staging-deployer"`
	ValidFrom time.Time      `json:"valid_from"`
	ValidTo   time.Time      `json:"valid_to"`
	CreatedAt time.Time      `json:"created_at"`
	RevokedAt *time.Time     `json:"revoked_at"`
	Status    apiTokenStatus `json:"status"`
}

func apiTokenFromCore(token core.AuthToken, now time.Time) apiTokenDTO {
	status := apiTokenActive
	switch {
	case token.RevokedAt != nil:
		status = apiTokenRevoked
	case !token.ExpiresAt.After(now):
		status = apiTokenExpired
	case token.ValidFrom.After(now):
		status = apiTokenScheduled
	}
	return apiTokenDTO{
		ID: token.ID, Name: token.Name, ValidFrom: token.ValidFrom,
		ValidTo: token.ExpiresAt, CreatedAt: token.CreatedAt,
		RevokedAt: token.RevokedAt, Status: status,
	}
}

type createAPITokenRequest struct {
	Name      string    `json:"name" example:"staging-deployer"`
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`
}

type createAPITokenResponse struct {
	Token    string      `json:"token" format:"password"`
	APIToken apiTokenDTO `json:"api_token"`
}

type apiTokensResponse struct {
	APITokens []apiTokenDTO `json:"api_tokens"`
	Total     int           `json:"total"`
}

type apiTokenPath struct {
	ID uuid.UUID `json:"-" path:"id"`
}

type nullableUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (n *nullableUUID) UnmarshalJSON(data []byte) error {
	n.Set = true
	return json.Unmarshal(data, &n.Value)
}

func (nullableUUID) JSONSchema() (jsonschema.Schema, error) {
	schema := jsonschema.Schema{}
	schema.AddType(jsonschema.String)
	schema.AddType(jsonschema.Null)
	schema.WithFormat("uuid")
	return schema, nil
}

type taskDTO struct {
	ID              uuid.UUID         `json:"id"`
	ProjectID       uuid.UUID         `json:"project_id"`
	Name            string            `json:"name" example:"web"`
	Description     string            `json:"description,omitempty"`
	Image           string            `json:"image" example:"nginx:alpine"`
	Status          core.TaskStatus   `json:"status"`
	Port            int               `json:"port" example:"80"`
	ContainerID     string            `json:"container_id,omitempty"`
	ContainerIP     string            `json:"container_ip,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Labels          map[string]string `json:"labels,omitempty"`
	Env             map[string]string `json:"env"`
	Error           string            `json:"error,omitempty"`
	PendingRecreate bool              `json:"pending_recreate,omitempty"`
}

func taskFromCore(t core.Task) taskDTO {
	if t.Env == nil {
		t.Env = map[string]string{}
	}
	return taskDTO{
		ID: t.ID, ProjectID: t.ProjectID, Name: t.Name, Description: t.Description,
		Image: t.Image, Status: t.Status, Port: t.Port,
		ContainerID: t.ContainerID, ContainerIP: t.ContainerIP,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		Labels: t.Labels, Env: t.Env, Error: t.Error, PendingRecreate: t.PendingRecreate,
	}
}

type projectPath struct {
	Project string `json:"-" path:"project" example:"demo"`
}

type taskPath struct {
	Project string `json:"-" path:"project" example:"demo"`
	Name    string `json:"-" path:"name" example:"web"`
}

type createTaskRequest struct {
	Project     string            `json:"-" path:"project" example:"demo"`
	Name        string            `json:"name" example:"web"`
	Description string            `json:"description,omitempty"`
	Image       string            `json:"image" example:"nginx:alpine"`
	Port        int               `json:"port,omitempty" example:"80"`
	Labels      map[string]string `json:"labels,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

type updateTaskRequest struct {
	Project     string             `json:"-" path:"project" example:"demo"`
	Name        string             `json:"-" path:"name" example:"web"`
	Description *string            `json:"description,omitempty"`
	Image       *string            `json:"image,omitempty" example:"nginx:1.27-alpine"`
	Port        *int               `json:"port,omitempty" example:"80"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Env         *map[string]string `json:"env,omitempty"`
	AutoRestart *bool              `json:"auto_restart,omitempty"`
}

type deployTaskRequest struct {
	Project string `json:"-" path:"project" example:"demo"`
	Name    string `json:"-" path:"name" example:"web"`
	Image   string `json:"image" example:"ghcr.io/acme/web@sha256:0000000000000000000000000000000000000000000000000000000000000000"`
}

type updateStateRequest struct {
	Project string `json:"-" path:"project" example:"demo"`
	Name    string `json:"-" path:"name" example:"web"`
	Action  string `json:"action" enum:"start,stop,restart" example:"start"`
}

type logsRequest struct {
	Project  string `json:"-" path:"project" example:"demo"`
	Name     string `json:"-" path:"name" example:"web"`
	Tail     int    `query:"tail" minimum:"0" default:"100"`
	Download bool   `query:"download"`
}

type streamLogsRequest struct {
	Project string `json:"-" path:"project" example:"demo"`
	Name    string `json:"-" path:"name" example:"web"`
	Tail    int    `query:"tail" minimum:"0" default:"100"`
}

type taskResponse struct {
	Task taskDTO `json:"task"`
}

type tasksResponse struct {
	Tasks []taskDTO `json:"tasks"`
	Total int       `json:"total"`
}

type taskStateResponse struct {
	Success bool    `json:"success"`
	Task    taskDTO `json:"task"`
}

type taskDeletedResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message" example:"task deleted"`
}

type projectDTO struct {
	ID                   uuid.UUID         `json:"id"`
	Slug                 string            `json:"slug" example:"demo"`
	Name                 string            `json:"name" example:"Demo"`
	RegistryCredentialID *uuid.UUID        `json:"registry_credential_id,omitempty"`
	DefaultImage         string            `json:"default_image" example:"nginx:alpine"`
	DefaultPort          int               `json:"default_port" example:"80"`
	DefaultEnv           map[string]string `json:"default_env"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

func projectFromCore(p core.Project) projectDTO {
	if p.DefaultEnv == nil {
		p.DefaultEnv = map[string]string{}
	}
	return projectDTO{
		ID: p.ID, Slug: p.Slug, Name: p.Name,
		RegistryCredentialID: p.RegistryCredentialID,
		DefaultImage:         p.DefaultImage, DefaultPort: p.DefaultPort, DefaultEnv: p.DefaultEnv,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

type createProjectRequest struct {
	Slug                 string            `json:"slug" example:"demo"`
	Name                 string            `json:"name,omitempty" example:"Demo"`
	RegistryCredentialID *uuid.UUID        `json:"registry_credential_id,omitempty"`
	DefaultImage         string            `json:"default_image,omitempty" example:"nginx:alpine"`
	DefaultPort          int               `json:"default_port,omitempty" example:"80"`
	DefaultEnv           map[string]string `json:"default_env,omitempty"`
}

type updateProjectRequest struct {
	Project              string             `json:"-" path:"project" example:"demo"`
	Name                 *string            `json:"name,omitempty"`
	RegistryCredentialID nullableUUID       `json:"registry_credential_id,omitempty"`
	DefaultImage         *string            `json:"default_image,omitempty" example:"nginx:alpine"`
	DefaultPort          *int               `json:"default_port,omitempty" example:"80"`
	DefaultEnv           *map[string]string `json:"default_env,omitempty"`
}

type addMemberRequest struct {
	Project string           `json:"-" path:"project" example:"demo"`
	UserID  uuid.UUID        `json:"user_id"`
	Role    core.ProjectRole `json:"role,omitempty"`
}

type memberPath struct {
	Project string    `json:"-" path:"project" example:"demo"`
	UserID  uuid.UUID `json:"-" path:"userID"`
}

type memberDTO struct {
	UserID    uuid.UUID        `json:"user_id"`
	Username  string           `json:"username"`
	Role      core.ProjectRole `json:"role"`
	CreatedAt time.Time        `json:"created_at"`
}

type projectResponse struct {
	Project projectDTO `json:"project"`
}

type projectsResponse struct {
	Projects []projectDTO `json:"projects"`
	Total    int          `json:"total"`
}

type membersResponse struct {
	Members []memberDTO `json:"members"`
	Total   int         `json:"total"`
}

type grantRequest struct {
	Project string           `json:"-" path:"project" example:"demo"`
	Name    string           `json:"-" path:"name" example:"web"`
	UserID  uuid.UUID        `json:"user_id"`
	Role    core.ProjectRole `json:"role,omitempty"`
}

type grantPath struct {
	Project string    `json:"-" path:"project" example:"demo"`
	Name    string    `json:"-" path:"name" example:"web"`
	UserID  uuid.UUID `json:"-" path:"userID"`
}

type grantDTO struct {
	UserID    uuid.UUID        `json:"user_id"`
	Username  string           `json:"username"`
	Role      core.ProjectRole `json:"role"`
	CreatedAt time.Time        `json:"created_at"`
}

type grantsResponse struct {
	Grants []grantDTO `json:"grants"`
	Total  int        `json:"total"`
}

type notificationsRequest struct {
	Project string `json:"-" path:"project" example:"demo"`
	Limit   int    `query:"limit" minimum:"1" maximum:"200" default:"50"`
}

type notificationDTO struct {
	ID        uuid.UUID               `json:"id"`
	TaskName  string                  `json:"task_name" example:"web"`
	Status    core.NotificationStatus `json:"status"`
	Title     string                  `json:"title" example:"Deployed: demo/web"`
	Body      string                  `json:"body,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
}

type notificationsResponse struct {
	Notifications []notificationDTO `json:"notifications"`
	Total         int               `json:"total"`
}

type pushSubscriptionRequest struct {
	Token string `json:"token" example:"fJ9x2Qk7RtqB:APA91bH-x9Kd2QwErTyUiOp"`
}

type pushSubscriptionPath struct {
	Token string `json:"-" path:"token" example:"fJ9x2Qk7RtqB:APA91bH-x9Kd2QwErTyUiOp"`
}

type userDTO struct {
	ID        uuid.UUID     `json:"id"`
	Username  string        `json:"username" example:"admin"`
	Email     string        `json:"email" format:"email"`
	Role      core.UserRole `json:"role"`
	Disabled  bool          `json:"disabled"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func userFromCore(u core.User) userDTO {
	return userDTO{
		ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role,
		Disabled: u.Disabled(), CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

type loginRequest struct {
	Username string `json:"username" example:"admin"`
	Password string `json:"password" format:"password"`
}

type createUserRequest struct {
	Username string        `json:"username"`
	Email    string        `json:"email" format:"email"`
	Password string        `json:"password" format:"password" minLength:"8"`
	Role     core.UserRole `json:"role,omitempty"`
}

type userPath struct {
	ID uuid.UUID `json:"-" path:"id"`
}

type updateUserRequest struct {
	ID       uuid.UUID      `json:"-" path:"id"`
	Email    *string        `json:"email,omitempty" format:"email"`
	Password *string        `json:"password,omitempty" format:"password" minLength:"8"`
	Role     *core.UserRole `json:"role,omitempty"`
	Disabled *bool          `json:"disabled,omitempty"`
}

type loginResponse struct {
	Token string  `json:"token"`
	User  userDTO `json:"user"`
}

type userResponse struct {
	User userDTO `json:"user"`
}

type usersResponse struct {
	Users []userDTO `json:"users"`
	Total int       `json:"total"`
}

type credentialDTO struct {
	ID        uuid.UUID         `json:"id"`
	Name      string            `json:"name" example:"ghcr"`
	Registry  core.RegistryKind `json:"registry"`
	Username  string            `json:"username"`
	CreatedAt time.Time         `json:"created_at"`
}

func credentialFromCore(c core.RegistryCredential) credentialDTO {
	return credentialDTO{
		ID: c.ID, Name: c.Name, Registry: c.Registry,
		Username: c.Username, CreatedAt: c.CreatedAt,
	}
}

type createCredentialRequest struct {
	Name     string            `json:"name" example:"ghcr"`
	Registry core.RegistryKind `json:"registry"`
	Username string            `json:"username"`
	Token    string            `json:"token" format:"password"`
}

type credentialPath struct {
	ID uuid.UUID `json:"-" path:"id"`
}

type credentialResponse struct {
	Credential credentialDTO `json:"credential"`
}

type credentialsResponse struct {
	Credentials []credentialDTO `json:"credentials"`
	Total       int             `json:"total"`
}

type healthResponse struct {
	Service string `json:"service" example:"boreas"`
	Status  string `json:"status" example:"healthy"`
}

type systemStatsDTO struct {
	TotalTasks    int     `json:"total_tasks"`
	RunningTasks  int     `json:"running_tasks"`
	StoppedTasks  int     `json:"stopped_tasks"`
	TotalProjects int     `json:"total_projects"`
	TotalMemoryMB float64 `json:"total_memory_mb"`
}
