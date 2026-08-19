// Package core contains Boreas' domain types and infrastructure ports.
package core

import (
	"context"
	"errors"
	"io"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
)

type TaskStatus string

const (
	StatusCreating TaskStatus = "creating"
	StatusRunning  TaskStatus = "running"
	StatusStopped  TaskStatus = "stopped"
	StatusError    TaskStatus = "error"
	StatusStarting TaskStatus = "starting"
	StatusUnknown  TaskStatus = "unknown"
)

// Enum publishes the allowed values to JSON Schema reflection.
func (TaskStatus) Enum() []any {
	return []any{StatusCreating, StatusStarting, StatusRunning, StatusStopped, StatusError, StatusUnknown}
}

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

func (UserRole) Enum() []any { return []any{RoleAdmin, RoleUser} }

type ProjectRole string

const (
	ProjectRoleOwner  ProjectRole = "owner"
	ProjectRoleMember ProjectRole = "member"
)

func (ProjectRole) Enum() []any { return []any{ProjectRoleOwner, ProjectRoleMember} }

type RegistryKind string

const (
	RegistryGHCR      RegistryKind = "ghcr"
	RegistryDockerHub RegistryKind = "dockerhub"
)

func (RegistryKind) Enum() []any { return []any{RegistryGHCR, RegistryDockerHub} }

var (
	taskNamePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	projectSlugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

func ValidateTaskName(name string) error {
	if !taskNamePattern.MatchString(name) {
		return errors.Join(ErrInvalidInput, errors.New("task name must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$"))
	}
	return nil
}

// ValidateEnv rejects names Docker cannot express and those Boreas injects itself.
func ValidateEnv(env map[string]string) error {
	for key := range env {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return errors.Join(ErrInvalidInput, errors.New("environment variable names must be non-empty and contain neither '=' nor NUL"))
		}
		switch key {
		case "BOREAS_PROJECT", "BOREAS_TASK", "BOREAS_PORT", "BASE_HREF":
			return errors.Join(ErrInvalidInput, errors.New("environment variable is reserved by Boreas: "+key))
		}
	}
	return nil
}

func ValidateProjectSlug(slug string) error {
	if !projectSlugRegexp.MatchString(slug) {
		return errors.Join(ErrInvalidInput, errors.New("project slug must match ^[a-z0-9][a-z0-9-]{0,62}$"))
	}
	switch slug {
	case "api", "health", "metrics", "static", "admin":
		return errors.Join(ErrInvalidInput, errors.New("project slug is reserved: "+slug))
	}
	return nil
}

type User struct {
	ID           uuid.UUID
	Username     string
	Email        string
	PasswordHash string
	Role         UserRole
	DisabledAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (u User) IsAdmin() bool  { return u.Role == RoleAdmin }
func (u User) Disabled() bool { return u.DisabledAt != nil }

type AuthToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Kind      TokenKind
	TokenHash string
	ValidFrom time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

type TokenKind string

const (
	TokenKindSession TokenKind = "session"
	TokenKindAPI     TokenKind = "api"
)

type RegistryCredential struct {
	ID        uuid.UUID
	Name      string
	Registry  RegistryKind
	Username  string
	Token     string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

type Project struct {
	ID                   uuid.UUID
	Slug                 string
	Name                 string
	RegistryCredentialID *uuid.UUID
	DefaultImage         string
	DefaultPort          int
	DefaultEnv           map[string]string
	CreatedBy            *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type ProjectMember struct {
	ProjectID uuid.UUID
	UserID    uuid.UUID
	Username  string
	Role      ProjectRole
	CreatedAt time.Time
}

type Task struct {
	ID              uuid.UUID
	ProjectID       uuid.UUID
	Name            string
	Description     string
	Image           string
	Status          TaskStatus
	Port            int
	ContainerID     string
	ContainerIP     string
	Labels          map[string]string
	Env             map[string]string
	PendingRecreate bool
	Error           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Spec requires projectSlug separately because it belongs to the project, not the task.
func (t Task) Spec(projectSlug string) ContainerSpec {
	return ContainerSpec{
		Project: projectSlug,
		Name:    t.Name,
		Image:   t.Image,
		Port:    t.Port,
		Labels:  maps.Clone(t.Labels),
		Env:     maps.Clone(t.Env),
	}
}

func (t Task) Clone() Task {
	t.Labels = maps.Clone(t.Labels)
	t.Env = maps.Clone(t.Env)
	return t
}

type ContainerSpec struct {
	Project string
	Name    string
	Image   string
	Port    int
	Labels  map[string]string
	Env     map[string]string
}

func (s ContainerSpec) Validate() error {
	if err := ValidateProjectSlug(s.Project); err != nil {
		return err
	}
	if err := ValidateTaskName(s.Name); err != nil {
		return err
	}
	if strings.TrimSpace(s.Image) == "" {
		return errors.Join(ErrInvalidInput, errors.New("image is required"))
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.Join(ErrInvalidInput, errors.New("port must be between 1 and 65535"))
	}
	if err := ValidateEnv(s.Env); err != nil {
		return err
	}
	for key := range s.Labels {
		switch key {
		case "managed-by", "project", "task":
			return errors.Join(ErrInvalidInput, errors.New("container label is reserved by Boreas: "+key))
		}
	}
	return nil
}

type ContainerState struct {
	Exists bool
	Status TaskStatus
	IP     string
	Error  string
}

type LogOptions struct {
	Tail       int
	Follow     bool
	Timestamps bool
}

type SystemStats struct {
	TotalTasks       int
	RunningTasks     int
	StoppedTasks     int
	TotalProjects    int
	TotalMemoryBytes int64
}

type ContainerRuntime interface {
	Pull(context.Context, string, *RegistryCredential) error
	Create(context.Context, ContainerSpec) (string, error)
	Recreate(context.Context, string, ContainerSpec) (string, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Remove(context.Context, string) error
	Inspect(context.Context, string) (ContainerState, error)
	Logs(context.Context, string, LogOptions) (io.ReadCloser, error)
	TotalMemory(context.Context) (int64, error)
}

// TaskStore persists tasks whose names are unique only within a project.
type TaskStore interface {
	List(ctx context.Context, projectID uuid.UUID) ([]Task, error)
	ListAll(ctx context.Context) ([]Task, error)
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (Task, error)
	Create(context.Context, Task) (Task, error)
	Update(context.Context, Task) (Task, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type UserStore interface {
	List(context.Context) ([]User, error)
	Get(ctx context.Context, id uuid.UUID) (User, error)
	GetByUsername(ctx context.Context, username string) (User, error)
	Count(context.Context) (int, error)
	Create(context.Context, User) (User, error)
	Update(context.Context, User) (User, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type TokenStore interface {
	Create(context.Context, AuthToken) (AuthToken, error)
	ListAPITokens(ctx context.Context, userID uuid.UUID) ([]AuthToken, error)
	GetByHash(ctx context.Context, hash string) (AuthToken, error)
	Revoke(ctx context.Context, hash string) error
	RevokeByID(ctx context.Context, userID, tokenID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

type ProjectStore interface {
	List(context.Context) ([]Project, error)
	ListForUser(ctx context.Context, userID uuid.UUID) ([]Project, error)
	GetBySlug(ctx context.Context, slug string) (Project, error)
	Create(context.Context, Project) (Project, error)
	Update(context.Context, Project) (Project, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Count(context.Context) (int, error)

	ListMembers(ctx context.Context, projectID uuid.UUID) ([]ProjectMember, error)
	GetMember(ctx context.Context, projectID, userID uuid.UUID) (ProjectMember, error)
	AddMember(context.Context, ProjectMember) error
	RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error
}

type CredentialStore interface {
	List(context.Context) ([]RegistryCredential, error)
	Get(ctx context.Context, id uuid.UUID) (RegistryCredential, error)
	Create(context.Context, RegistryCredential) (RegistryCredential, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type RouteRegistry interface {
	Register(ctx context.Context, project, task, host string, port int) error
	Unregister(ctx context.Context, project, task string) error
}
