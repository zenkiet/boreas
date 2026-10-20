// Package core contains Boreas' domain types and infrastructure ports.
package core

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrTooMany       = errors.New("too many requests")
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

type DevStatus string

const (
	DevInProgress DevStatus = "in_progress"
	DevBlocked    DevStatus = "blocked"
	DevReady      DevStatus = "ready"
)

func (DevStatus) Enum() []any { return []any{DevInProgress, DevBlocked, DevReady} }

func ValidateDevStatus(status DevStatus) error {
	switch status {
	case DevInProgress, DevBlocked, DevReady:
		return nil
	}
	return errors.Join(ErrInvalidInput, errors.New("dev status must be in_progress, blocked, or ready"))
}

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

func (UserRole) Enum() []any { return []any{RoleAdmin, RoleUser} }

type ProjectRole string

const (
	ProjectRoleViewer   ProjectRole = "viewer"
	ProjectRoleOperator ProjectRole = "operator"
	ProjectRoleMember   ProjectRole = "member"
	ProjectRoleOwner    ProjectRole = "owner"
)

func (ProjectRole) Enum() []any {
	return []any{ProjectRoleViewer, ProjectRoleOperator, ProjectRoleMember, ProjectRoleOwner}
}

// Rank orders roles so authorization is one comparison; 0 marks an unknown or absent role.
func (r ProjectRole) Rank() int {
	return slices.Index([]ProjectRole{ProjectRoleViewer, ProjectRoleOperator, ProjectRoleMember, ProjectRoleOwner}, r) + 1
}

// Max keeps the higher role: a grant raises access and never lowers it.
func (r ProjectRole) Max(other ProjectRole) ProjectRole {
	if other.Rank() > r.Rank() {
		return other
	}
	return r
}

type RegistryKind string

const (
	RegistryGHCR      RegistryKind = "ghcr"
	RegistryDockerHub RegistryKind = "dockerhub"
)

func (RegistryKind) Enum() []any { return []any{RegistryGHCR, RegistryDockerHub} }

type NotificationStatus string

const (
	NotificationSuccess NotificationStatus = "success"
	NotificationFailure NotificationStatus = "failure"
	NotificationInfo    NotificationStatus = "info"
)

func (NotificationStatus) Enum() []any {
	return []any{NotificationSuccess, NotificationFailure, NotificationInfo}
}

type NotificationType string

const (
	NotificationDeployed      NotificationType = "deployed"
	NotificationDeployFailed  NotificationType = "deploy_failed"
	NotificationStatusChanged NotificationType = "status_changed"
	NotificationTaskCreated   NotificationType = "task_created"
	NotificationTaskAssigned  NotificationType = "task_assigned"
	NotificationBuildFailed   NotificationType = "build_failed"
)

func (NotificationType) Enum() []any {
	return []any{
		NotificationDeployed, NotificationDeployFailed, NotificationStatusChanged,
		NotificationTaskCreated, NotificationTaskAssigned, NotificationBuildFailed,
	}
}

// Status is the delivery severity: deploys succeed or fail, builds can fail, everything else informs.
func (t NotificationType) Status() NotificationStatus {
	switch t {
	case NotificationDeployed:
		return NotificationSuccess
	case NotificationDeployFailed, NotificationBuildFailed:
		return NotificationFailure
	}
	return NotificationInfo
}

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

func ValidatePushToken(token string) error {
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, ",/? \t\r\n") {
		return errors.Join(ErrInvalidInput,
			errors.New("push token must be 1-4096 bytes and contain no comma, slash, question mark, or whitespace"))
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
	Repositories         []string
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

// TaskGrant raises one user's role on a single task above whatever the project gives them.
type TaskGrant struct {
	TaskID    uuid.UUID
	UserID    uuid.UUID
	Username  string
	Role      ProjectRole
	CreatedAt time.Time
}

// ProjectAccess is one caller's effective reach in one project.
type ProjectAccess struct {
	Project  Project
	UserID   uuid.UUID
	Role     ProjectRole // effective role for the requested task, or for the project envelope
	AllTasks bool        // members and admins; grantees reach only what they were granted
}

type Task struct {
	ID              uuid.UUID
	ProjectID       uuid.UUID
	Name            string
	Description     string
	Note            string
	Image           string
	Status          TaskStatus
	DevStatus       DevStatus
	Port            int
	ContainerID     string
	ContainerIP     string
	Labels          map[string]string
	Env             map[string]string
	Volumes         map[string]string
	Routes          []Route
	Build           Build
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
		Volumes: maps.Clone(t.Volumes),
	}
}

func (t Task) Clone() Task {
	t.Labels = maps.Clone(t.Labels)
	t.Env = maps.Clone(t.Env)
	t.Volumes = maps.Clone(t.Volumes)
	t.Routes = slices.Clone(t.Routes)
	return t
}

type BuildState string

const (
	BuildRunning  BuildState = "running"
	BuildSuccess  BuildState = "success"
	BuildFailure  BuildState = "failure"
	BuildCanceled BuildState = "canceled"
)

func (BuildState) Enum() []any { return []any{BuildRunning, BuildSuccess, BuildFailure, BuildCanceled} }

// Build is the latest report a CI pipeline sent for a task; the zero value means none yet.
type Build struct {
	State    BuildState `json:"state"`
	Stage    string     `json:"stage,omitempty"`
	Progress int        `json:"progress,omitempty"`
	URL      string     `json:"url,omitempty"`
	At       time.Time  `json:"at"`
}

func (b Build) Validate() error {
	link, err := url.Parse(b.URL)
	switch {
	case !slices.Contains([]BuildState{BuildRunning, BuildSuccess, BuildFailure, BuildCanceled}, b.State):
		return errors.Join(ErrInvalidInput, errors.New("build state must be running, success, failure, or canceled"))
	case utf8.RuneCountInString(b.Stage) > 100 || strings.ContainsFunc(b.Stage, unicode.IsControl):
		return errors.Join(ErrInvalidInput, errors.New("build stage must be at most 100 printable characters"))
	case b.Progress < 0 || b.Progress > 100:
		return errors.Join(ErrInvalidInput, errors.New("build progress must be between 0 and 100"))
	case b.URL != "" && (err != nil || len(b.URL) > 2048 || link.Host == "" || (link.Scheme != "http" && link.Scheme != "https")):
		return errors.Join(ErrInvalidInput, errors.New("build url must be an absolute http or https URL"))
	}
	return nil
}

// FleetTask is a task as one caller sees it across every project they reach.
type FleetTask struct {
	Task
	Role       ProjectRole
	LastDeploy *Notification // nil until the first deploy
}

type Notification struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Project   string // slug, filled in on reads
	TaskName  string
	Type      NotificationType
	Status    NotificationStatus
	Title     string
	Body      string
	Seen      bool
	CreatedAt time.Time
}

type ContainerSpec struct {
	Project string
	Name    string
	Image   string
	Port    int
	Labels  map[string]string
	Env     map[string]string
	// Container path to a folder of the project in the shared volume.
	Volumes map[string]string
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
	return ValidateVolumes(s.Volumes)
}

func ValidateVolumes(volumes map[string]string) error {
	for target, folder := range volumes {
		if !path.IsAbs(target) || path.Clean(target) != target || target == "/" ||
			folder == "" || folder == "." || folder == ".." || strings.Contains(folder, "/") {
			return errors.Join(ErrInvalidInput, errors.New("volume must map a clean absolute path other than / to a folder name: "+target))
		}
	}
	return nil
}

// Route forwards a path under a task's App URL to an address the browser cannot reach itself.
type Route struct {
	Path    string `json:"path" example:"/api"`
	URL     string `json:"url" example:"http://10.20.0.15:8080"`
	AnyCert bool   `json:"any_cert,omitzero"`
}

// RouteCheck holds Status when the address answered and Error when it did not.
type RouteCheck struct {
	Status int
	Error  string
	Took   time.Duration
}

const badRouteURL = "address must be http(s)://host[:port][/path]"

var routePathPattern = regexp.MustCompile(`^(/[A-Za-z0-9_-]+)+$`)

// CleanRoutes sorts routes by path; each error starts with the path as sent so the app can mark its row.
func CleanRoutes(routes []Route) ([]Route, error) {
	if len(routes) > 20 {
		return nil, errors.Join(ErrInvalidInput, errors.New("at most 20 routes"))
	}
	clean := make([]Route, 0, len(routes))
	for _, r := range routes {
		p := strings.TrimSuffix(r.Path, "/")
		u, err := CleanRouteURL(r.URL)
		reason := ""
		switch {
		case r.Path == "/":
			reason = "/ is the task itself"
		case len(p) > 64 || !routePathPattern.MatchString(p):
			reason = "path must start with / and use letters, digits, - and _"
		case slices.ContainsFunc(clean, func(c Route) bool { return c.Path == p }):
			reason = "path is taken"
		case err != nil:
			reason = badRouteURL
		}
		if reason != "" {
			return nil, errors.Join(ErrInvalidInput, errors.New(r.Path+": "+reason))
		}
		clean = append(clean, Route{Path: p, URL: u, AnyCert: r.AnyCert})
	}
	slices.SortFunc(clean, func(a, b Route) int { return strings.Compare(a.Path, b.Path) })
	return clean, nil
}

// CleanRouteURL drops dot segments and a trailing slash from the base path.
func CleanRouteURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.Join(ErrInvalidInput, errors.New(badRouteURL))
	}
	u.Path, u.RawPath = strings.TrimSuffix(path.Clean("/"+u.Path), "/"), ""
	return u.String(), nil
}

type ContainerState struct {
	Exists bool
	Status TaskStatus
	IP     string
	Error  string
}

type LogOptions struct {
	Tail   int
	Follow bool
	Since  time.Time // lines from then on; Tail still caps them unless Follow
}

type SystemStats struct {
	TotalTasks       int
	RunningTasks     int
	StoppedTasks     int
	TotalProjects    int
	TotalMemoryBytes int64
}

// TaskMetric is one live sample. CPUPercent is a rate, so the first sample of a stream
// reports 0; MemoryBytes excludes page cache to match `docker stats`.
type TaskMetric struct {
	TaskName       string    `json:"task"`
	CPUPercent     float64   `json:"cpu_percent"`
	MemoryBytes    int64     `json:"memory_bytes"`
	MemoryLimit    int64     `json:"memory_limit"`
	NetworkRXBytes int64     `json:"network_rx_bytes"`
	NetworkTXBytes int64     `json:"network_tx_bytes"`
	ObservedAt     time.Time `json:"observed_at"`
}

// CodeIndex reads indexed source code; every call is confined to the repos it is given.
type CodeIndex interface {
	Repos(ctx context.Context) ([]string, error)
	Search(ctx context.Context, repos []string, query string) ([]CodeMatch, error)
	Read(ctx context.Context, repos []string, repo, path string) (CodeFile, error)
}

type CodeMatch struct {
	Repo, Path string
	Line       int
	Snippet    string
}

type CodeFile struct {
	Repo, Path, Content, URL string
}

type ChatRole string

const (
	ChatUser      ChatRole = "user"
	ChatAssistant ChatRole = "assistant"
)

func (ChatRole) Enum() []any { return []any{ChatUser, ChatAssistant} }

type CodeSource struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
	URL  string `json:"url,omitempty"`
}

type ChatMessage struct {
	Role    ChatRole
	Content string
}

// Tool is one thing the assistant may call; Run gets the model's JSON arguments and returns plain text for it.
type Tool struct {
	Name, Description string
	Params            map[string]any // JSON Schema of the arguments object
	Run               func(ctx context.Context, args string) string
}

// Ask is one question; Data is text others wrote, kept out of the system message, and Delta streams the answer.
type Ask struct {
	System, Brief  string
	Clarify        bool
	Tools          []Tool
	Budget         int
	History        []ChatMessage
	Question, Data string
	// ClarifyData replaces Data for screening, which has no tools to read with.
	ClarifyData string
	Delta       func(string)
	// Reset says the text streamed since the last reset led into a tool call.
	Reset func()
}

// secrets catches values hardcoded in ordinary source and logs, which the Sourcebot blocklist of file names misses.
var secrets = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), "[redacted]"},
	// A key whose BEGIN line a log tail cut off; log lines may start with a timestamp.
	{regexp.MustCompile(`(?m)(?:^(?:\S+ )?[A-Za-z0-9+/=]+\r?\n)+(?:\S+ )?-----END [A-Z ]*PRIVATE KEY-----`), "[redacted]"},
	{regexp.MustCompile(`(?i)([\w.-]*(?:password|passwd|pwd|secret|token|key|credential)[\w.-]*["']?\s*(?::=|=>|[:=])\s*)(?:"[^"\n]*"|'[^'\n]*')`), `${1}"[redacted]"`},
	{regexp.MustCompile(`(?i)\b([\w.-]*(?:password|passwd|pwd|secret|token|key)[\w.-]*(?:[ \t]*=[ \t]*|:[ \t]*))[^\s"'=][^\s,;]*`), "${1}[redacted]"},
	{regexp.MustCompile(`(?i)(--?[\w.-]*(?:password|passwd|pwd|secret|token|key)[\w.-]*(?:[ \t]+|["']\s*,\s*))(?:"[^"\n]*"|'[^'\n]*'|[^\s"'-]\S*)`), "${1}[redacted]"},
	{regexp.MustCompile(`(?i)(authorization["']?\s*[:=]\s*["']?(?:(?:basic|bearer|digest|token)\s+)?)[^\s"',;]+`), "${1}[redacted]"},
	{regexp.MustCompile(`(?i)\b(bearer\s+)[\w.~+/=-]{16,}`), "${1}[redacted]"},
	{regexp.MustCompile(`(?i)([;"']\s*(?:password|pwd)\s*=\s*)[^;"'\n]+`), "${1}[redacted]"},
	{regexp.MustCompile(`(://[^:/\s"'@]*:)[^@/\s"']+@`), "${1}[redacted]@"},
	{regexp.MustCompile(`\b(?:AKIA[0-9A-Z]{16}|[sr]k_(?:live|test)_[0-9A-Za-z]{16,}|sk-[\w-]{20,}|gh[pousr]_[A-Za-z0-9]{36,}|xox[abprs]-[\w-]{10,}|AIza[\w-]{35}|eyJ[\w-]{10,}\.eyJ[\w-]{10,}\.[\w-]{10,})`), "[redacted]"},
}

// Redact hides what looks like a secret.
func Redact(text string) string {
	for _, s := range secrets {
		text = s.re.ReplaceAllString(text, s.with)
	}
	return text
}

type BoraOp string

// BoraOps are the changes Bora may propose, each with the role the route that runs it requires.
var BoraOps = map[BoraOp]ProjectRole{
	"task.start": ProjectRoleOperator, "task.stop": ProjectRoleOperator, "task.restart": ProjectRoleOperator,
	"task.create": ProjectRoleMember, "task.delete": ProjectRoleMember, "task.update": ProjectRoleMember, "task.env": ProjectRoleMember,
	"member.add": ProjectRoleOwner, "member.role": ProjectRoleOwner, "member.remove": ProjectRoleOwner,
	"grant.add": ProjectRoleOwner, "grant.remove": ProjectRoleOwner, "project.update": ProjectRoleOwner, "project.delete": ProjectRoleOwner,
}

func (BoraOp) Enum() []any {
	var ops []any
	for _, op := range slices.Sorted(maps.Keys(BoraOps)) {
		ops = append(ops, op)
	}
	return ops
}

// BoraAction is a change Bora proposes; only the user's tap runs it, through the route that checks their role.
type BoraAction struct {
	Op                   BoraOp            `json:"op"`
	Project              string            `json:"project"`
	Task                 string            `json:"task,omitempty"`
	Reason               string            `json:"reason"`
	UserID               uuid.UUID         `json:"user_id,omitzero"`
	Username             string            `json:"username,omitempty"`
	Role                 ProjectRole       `json:"role,omitempty"`
	Name                 string            `json:"name,omitempty"`
	Description          string            `json:"description,omitempty"`
	Note                 string            `json:"note,omitempty"`
	DevStatus            DevStatus         `json:"dev_status,omitempty"`
	Image                string            `json:"image,omitempty"`
	Port                 int               `json:"port,omitempty"`
	AutoRestart          *bool             `json:"auto_restart,omitempty"`
	Volumes              map[string]string `json:"volumes,omitempty"`
	Set                  map[string]string `json:"set,omitempty"`
	Unset                []string          `json:"unset,omitempty"`
	DefaultImage         string            `json:"default_image,omitempty"`
	DefaultPort          int               `json:"default_port,omitempty"`
	DefaultEnvSet        map[string]string `json:"default_env_set,omitempty"`
	DefaultEnvUnset      []string          `json:"default_env_unset,omitempty"`
	RegistryCredentialID *uuid.UUID        `json:"registry_credential_id,omitempty"`
}

// BoraAbout is what a conversation is about; it never changes.
type BoraAbout struct {
	Kind    string    `json:"kind" enum:"fleet,project,task,alert,build,log"`
	Project string    `json:"project,omitempty" example:"demo"`
	Task    string    `json:"task,omitempty" example:"qa-1"`
	ID      uuid.UUID `json:"id,omitzero"`
	At      string    `json:"at,omitempty"`
	Line    string    `json:"line,omitempty" maxLength:"1000"`
}

type BoraMessage struct {
	Index   int          `json:"index"`
	Role    ChatRole     `json:"role"`
	Content string       `json:"content"`
	Say     string       `json:"say"`
	Actions []BoraAction `json:"actions"`
	Choices []string     `json:"choices"`
	Sources []CodeSource `json:"sources"`
	Ran     []string     `json:"ran"`
	At      time.Time    `json:"at"`
}

type BoraPending struct {
	Question string    `json:"question"`
	Since    time.Time `json:"since"`
}

// BoraConversation is one user's, hidden while a project its answers read is out of their reach.
type BoraConversation struct {
	ID, UserID           uuid.UUID
	ProjectID            *uuid.UUID
	About                BoraAbout
	Projects             []uuid.UUID
	Title                string
	Snippet              string        // set by a search
	Messages             []BoraMessage // set by Get
	Answering            *BoraPending
	CreatedAt, UpdatedAt time.Time
}

// BoraCursor pages conversations newest first; the zero value starts at the newest.
type BoraCursor struct {
	At time.Time
	ID uuid.UUID
}

// BoraStore sees only the user's own visible conversations, unless noted.
type BoraStore interface {
	// Save inserts a conversation with its first turn, or appends a later one.
	Save(ctx context.Context, c BoraConversation, turn []BoraMessage) error
	Get(ctx context.Context, userID uuid.UUID, admin bool, id uuid.UUID) (BoraConversation, error)
	// List with a query keeps conversations with a message holding each of its words as a prefix.
	List(ctx context.Context, userID uuid.UUID, admin bool, query string, before BoraCursor, limit int) ([]BoraConversation, error)
	// AddRan expects a checked conversation, and refuses an index that is not an answer with room left.
	AddRan(ctx context.Context, id uuid.UUID, index int, line string) error
	// Delete with a nil id removes every conversation of the user, hidden ones included.
	Delete(ctx context.Context, userID uuid.UUID, admin bool, id *uuid.UUID) error
}

type ContainerRuntime interface {
	Pull(context.Context, string, *RegistryCredential) error
	Create(context.Context, ContainerSpec) (string, error)
	Recreate(context.Context, string, ContainerSpec) (string, error)
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Remove(context.Context, string) error
	Folders(ctx context.Context, project string) ([]string, error)
	Inspect(context.Context, string) (ContainerState, error)
	Logs(context.Context, string, LogOptions) (io.ReadCloser, error)
	TotalMemory(context.Context) (int64, error)
	Stats(ctx context.Context, containerID string) (<-chan TaskMetric, error)
}

// TaskStore persists tasks whose names are unique only within a project.
type TaskStore interface {
	List(ctx context.Context, projectID, userID uuid.UUID, allTasks bool) ([]Task, error)
	ListAll(ctx context.Context) ([]Task, error)
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (Task, error)
	Create(context.Context, Task) (Task, error)
	Update(context.Context, Task) (Task, error)
	SetBuild(ctx context.Context, id uuid.UUID, build Build) error
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
	ListForUser(ctx context.Context, userID uuid.UUID) ([]ProjectAccess, error)
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

type NotificationStore interface {
	Create(context.Context, Notification) (Notification, error)
	List(ctx context.Context, userID uuid.UUID, admin bool, projectID, before *uuid.UUID, limit int) ([]Notification, error)
	Get(ctx context.Context, userID uuid.UUID, admin bool, id uuid.UUID) (Notification, error)
	MarkSeen(ctx context.Context, userID uuid.UUID, admin bool, ids []uuid.UUID) error
	MarkUnseen(ctx context.Context, id, userID uuid.UUID) error
	LastDeploys(context.Context) (map[uuid.UUID]Notification, error)
}

type GrantStore interface {
	Role(ctx context.Context, projectID, userID uuid.UUID, taskName string) (ProjectRole, error)
	AnyInProject(ctx context.Context, projectID, userID uuid.UUID) (bool, error)
	ForUser(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]ProjectRole, error)
	ListForTask(ctx context.Context, taskID uuid.UUID) ([]TaskGrant, error)
	Grant(context.Context, TaskGrant) error
	Revoke(ctx context.Context, taskID, userID uuid.UUID) error
}

type CredentialStore interface {
	List(context.Context) ([]RegistryCredential, error)
	Get(ctx context.Context, id uuid.UUID) (RegistryCredential, error)
	Create(context.Context, RegistryCredential) (RegistryCredential, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// RouteRegistry serves a task's container while it runs and its routes whether or not it does.
type RouteRegistry interface {
	Register(ctx context.Context, project, task, host string, port int) error
	Unregister(ctx context.Context, project, task string) error
	SetRoutes(project, task string, routes []Route)
	Check(context.Context, Route) RouteCheck
}
