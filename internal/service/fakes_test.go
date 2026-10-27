package service

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

// grantKey mirrors the task_grants primary key.
type grantKey struct {
	taskID uuid.UUID
	userID uuid.UUID
}

type fakeTaskStore struct {
	tasks map[uuid.UUID]core.Task
	// roles stands in for task_grants: a key is granted exactly when it holds a role.
	roles map[grantKey]core.ProjectRole
	now   time.Time
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{
		tasks: map[uuid.UUID]core.Task{}, roles: map[grantKey]core.ProjectRole{}, now: time.Unix(1000, 0).UTC(),
	}
}

func (f *fakeTaskStore) tick() time.Time {
	f.now = f.now.Add(time.Second)
	return f.now
}

func (f *fakeTaskStore) List(
	_ context.Context, projectID, userID uuid.UUID, allTasks bool,
) ([]core.Task, error) {
	result := make([]core.Task, 0, len(f.tasks))
	for _, task := range f.tasks {
		if task.ProjectID != projectID {
			continue
		}
		if !allTasks && f.roles[grantKey{task.ID, userID}] == "" {
			continue
		}
		result = append(result, task.Clone())
	}
	return result, nil
}

func (f *fakeTaskStore) ListAll(context.Context) ([]core.Task, error) {
	result := make([]core.Task, 0, len(f.tasks))
	for _, task := range f.tasks {
		result = append(result, task.Clone())
	}
	return result, nil
}

func (f *fakeTaskStore) GetByName(_ context.Context, projectID uuid.UUID, name string) (core.Task, error) {
	for _, task := range f.tasks {
		if task.ProjectID == projectID && task.Name == name {
			return task.Clone(), nil
		}
	}
	return core.Task{}, core.ErrNotFound
}

func (f *fakeTaskStore) Create(_ context.Context, task core.Task) (core.Task, error) {
	for _, existing := range f.tasks {
		if existing.ProjectID == task.ProjectID && existing.Name == task.Name {
			return core.Task{}, core.ErrAlreadyExists
		}
	}
	task.ID = uuid.New()
	task.CreatedAt, task.UpdatedAt = f.tick(), f.now
	f.tasks[task.ID] = task.Clone()
	return task.Clone(), nil
}

func (f *fakeTaskStore) Update(_ context.Context, task core.Task) (core.Task, error) {
	existing := f.tasks[task.ID]
	task.CreatedAt, task.Build, task.UpdatedAt = existing.CreatedAt, existing.Build, f.tick()
	f.tasks[task.ID] = task.Clone()
	return task.Clone(), nil
}

func (f *fakeTaskStore) SetBuild(_ context.Context, id uuid.UUID, build core.Build) error {
	task := f.tasks[id]
	task.Build = build
	f.tasks[id] = task
	return nil
}

func (f *fakeTaskStore) Delete(_ context.Context, id uuid.UUID) error {
	delete(f.tasks, id)
	return nil
}

type fakeProjectStore struct {
	projects map[uuid.UUID]core.Project
	members  map[uuid.UUID]map[uuid.UUID]core.ProjectMember
	tasks    *fakeTaskStore // resolves task grants, as the SQL query's second EXISTS does
}

func newFakeProjectStore() *fakeProjectStore {
	return &fakeProjectStore{
		projects: map[uuid.UUID]core.Project{},
		members:  map[uuid.UUID]map[uuid.UUID]core.ProjectMember{},
	}
}

func (f *fakeProjectStore) add(slug string) core.Project {
	project := core.Project{ID: uuid.New(), Slug: slug, Name: slug, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.projects[project.ID] = project
	return project
}

func (f *fakeProjectStore) List(context.Context) ([]core.Project, error) {
	result := make([]core.Project, 0, len(f.projects))
	for _, project := range f.projects {
		result = append(result, project)
	}
	return result, nil
}

// ListForUser mirrors the SQL query: memberships and task grants both reveal a project,
// but only a membership carries its role.
func (f *fakeProjectStore) ListForUser(_ context.Context, userID uuid.UUID) ([]core.ProjectAccess, error) {
	result := make([]core.ProjectAccess, 0)
	for id, project := range f.projects {
		if member, ok := f.members[id][userID]; ok {
			result = append(result, core.ProjectAccess{Project: project, UserID: userID, Role: member.Role, AllTasks: true})
			continue
		}
		if f.tasks != nil && f.tasks.anyGrantIn(id, userID) {
			result = append(result, core.ProjectAccess{Project: project, UserID: userID, Role: core.ProjectRoleViewer})
		}
	}
	return result, nil
}

func (f *fakeTaskStore) anyGrantIn(projectID, userID uuid.UUID) bool {
	for _, task := range f.tasks {
		if task.ProjectID == projectID && f.roles[grantKey{task.ID, userID}] != "" {
			return true
		}
	}
	return false
}

func (f *fakeProjectStore) GetBySlug(_ context.Context, slug string) (core.Project, error) {
	for _, project := range f.projects {
		if project.Slug == slug {
			return project, nil
		}
	}
	return core.Project{}, core.ErrNotFound
}

func (f *fakeProjectStore) Create(_ context.Context, project core.Project) (core.Project, error) {
	project.ID = uuid.New()
	project.CreatedAt, project.UpdatedAt = time.Now(), time.Now()
	f.projects[project.ID] = project
	return project, nil
}

func (f *fakeProjectStore) Update(_ context.Context, project core.Project) (core.Project, error) {
	project.UpdatedAt = time.Now()
	f.projects[project.ID] = project
	return project, nil
}

func (*fakeProjectStore) Delete(context.Context, uuid.UUID) error { return nil }

func (f *fakeProjectStore) Count(context.Context) (int, error) { return len(f.projects), nil }

func (f *fakeProjectStore) ListMembers(_ context.Context, projectID uuid.UUID) ([]core.ProjectMember, error) {
	result := make([]core.ProjectMember, 0)
	for _, member := range f.members[projectID] {
		result = append(result, member)
	}
	return result, nil
}

func (f *fakeProjectStore) GetMember(_ context.Context, projectID, userID uuid.UUID) (core.ProjectMember, error) {
	member, ok := f.members[projectID][userID]
	if !ok {
		return core.ProjectMember{}, core.ErrNotFound
	}
	return member, nil
}

func (f *fakeProjectStore) AddMember(_ context.Context, member core.ProjectMember) error {
	if f.members[member.ProjectID] == nil {
		f.members[member.ProjectID] = map[uuid.UUID]core.ProjectMember{}
	}
	member.CreatedAt = time.Now()
	f.members[member.ProjectID][member.UserID] = member
	return nil
}

func (f *fakeProjectStore) RemoveMember(_ context.Context, projectID, userID uuid.UUID) error {
	delete(f.members[projectID], userID)
	return nil
}

type fakeCredentialStore struct {
	credentials map[uuid.UUID]core.RegistryCredential
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{credentials: map[uuid.UUID]core.RegistryCredential{}}
}

func (f *fakeCredentialStore) List(context.Context) ([]core.RegistryCredential, error) {
	result := make([]core.RegistryCredential, 0, len(f.credentials))
	for _, credential := range f.credentials {
		result = append(result, credential)
	}
	return result, nil
}

func (f *fakeCredentialStore) Get(_ context.Context, id uuid.UUID) (core.RegistryCredential, error) {
	credential, ok := f.credentials[id]
	if !ok {
		return core.RegistryCredential{}, core.ErrNotFound
	}
	return credential, nil
}

func (f *fakeCredentialStore) Create(_ context.Context, credential core.RegistryCredential) (core.RegistryCredential, error) {
	credential.ID = uuid.New()
	credential.CreatedAt = time.Now()
	f.credentials[credential.ID] = credential
	return credential, nil
}

func (f *fakeCredentialStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.credentials[id]; !ok {
		return core.ErrNotFound
	}
	delete(f.credentials, id)
	return nil
}

type fakeNotificationStore struct {
	notifications []core.Notification
	tasks         *fakeTaskStore // resolves task names, as the SQL join does
}

func newFakeNotificationStore(tasks *fakeTaskStore) *fakeNotificationStore {
	return &fakeNotificationStore{tasks: tasks}
}

func (f *fakeNotificationStore) Create(_ context.Context, n core.Notification) (core.Notification, error) {
	n.ID, n.CreatedAt = uuid.New(), f.tasks.tick()
	f.notifications = append(f.notifications, n)
	return n, nil
}

// List ignores visibility but keeps the SQL's newest-first order and limit.
func (f *fakeNotificationStore) List(_ context.Context, _ uuid.UUID, _ bool, projectID, _ *uuid.UUID, limit int) ([]core.Notification, error) {
	var out []core.Notification
	for _, n := range slices.Backward(f.notifications) {
		if len(out) < limit && (projectID == nil || n.ProjectID == *projectID) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (f *fakeNotificationStore) Get(_ context.Context, _ uuid.UUID, _ bool, id uuid.UUID) (core.Notification, error) {
	if i := slices.IndexFunc(f.notifications, func(n core.Notification) bool { return n.ID == id }); i >= 0 {
		return f.notifications[i], nil
	}
	return core.Notification{}, core.ErrNotFound
}

func (*fakeNotificationStore) MarkSeen(context.Context, uuid.UUID, bool, []uuid.UUID) error {
	return nil
}

func (*fakeNotificationStore) MarkUnseen(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (f *fakeNotificationStore) LastDeploys(context.Context) (map[uuid.UUID]core.Notification, error) {
	deploys := map[uuid.UUID]core.Notification{}
	for id, task := range f.tasks.tasks {
		for _, n := range f.notifications {
			if n.ProjectID == task.ProjectID && n.TaskName == task.Name && (n.Type == core.NotificationDeployed || n.Type == core.NotificationDeployFailed) {
				deploys[id] = n // oldest first, so the newest wins
			}
		}
	}
	return deploys, nil
}

type fakeGrantStore struct{ tasks *fakeTaskStore }

func newFakeGrantStore(tasks *fakeTaskStore) *fakeGrantStore { return &fakeGrantStore{tasks: tasks} }

func (f *fakeGrantStore) Role(
	_ context.Context, projectID, userID uuid.UUID, taskName string,
) (core.ProjectRole, error) {
	for _, task := range f.tasks.tasks {
		if task.ProjectID == projectID && task.Name == taskName {
			return f.tasks.roles[grantKey{task.ID, userID}], nil
		}
	}
	return "", nil
}

func (f *fakeGrantStore) AnyInProject(_ context.Context, projectID, userID uuid.UUID) (bool, error) {
	return f.tasks.anyGrantIn(projectID, userID), nil
}

func (f *fakeGrantStore) ForUser(_ context.Context, userID uuid.UUID) (map[uuid.UUID]core.ProjectRole, error) {
	roles := map[uuid.UUID]core.ProjectRole{}
	for key, role := range f.tasks.roles {
		if key.userID == userID {
			roles[key.taskID] = role
		}
	}
	return roles, nil
}

func (f *fakeGrantStore) ListForTask(_ context.Context, taskID uuid.UUID) ([]core.TaskGrant, error) {
	var grants []core.TaskGrant
	for key, role := range f.tasks.roles {
		if key.taskID == taskID {
			grants = append(grants, core.TaskGrant{TaskID: taskID, UserID: key.userID, Role: role})
		}
	}
	return grants, nil
}

func (f *fakeGrantStore) Grant(_ context.Context, grant core.TaskGrant) error {
	f.tasks.roles[grantKey{grant.TaskID, grant.UserID}] = grant.Role
	return nil
}

func (*fakeGrantStore) Revoke(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type fakeUserStore struct{ users map[uuid.UUID]core.User }

func newFakeUserStore() *fakeUserStore { return &fakeUserStore{users: map[uuid.UUID]core.User{}} }

func (*fakeUserStore) List(context.Context) ([]core.User, error) { return nil, nil }

func (f *fakeUserStore) Get(_ context.Context, id uuid.UUID) (core.User, error) {
	user, ok := f.users[id]
	if !ok {
		return core.User{}, core.ErrNotFound
	}
	return user, nil
}

func (f *fakeUserStore) GetByUsername(_ context.Context, username string) (core.User, error) {
	for _, user := range f.users {
		if strings.EqualFold(user.Username, username) {
			return user, nil
		}
	}
	return core.User{}, core.ErrNotFound
}

func (f *fakeUserStore) Count(context.Context) (int, error) { return len(f.users), nil }

func (f *fakeUserStore) Create(_ context.Context, user core.User) (core.User, error) {
	user.ID = uuid.New()
	user.CreatedAt, user.UpdatedAt = time.Now(), time.Now()
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeUserStore) Update(_ context.Context, user core.User) (core.User, error) {
	user.UpdatedAt = time.Now()
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeUserStore) SetAvatar(_ context.Context, id uuid.UUID) (core.User, error) {
	user, ok := f.users[id]
	if !ok {
		return core.User{}, core.ErrNotFound
	}
	user.AvatarAt = new(time.Now())
	f.users[id] = user
	return user, nil
}

func (f *fakeUserStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.users[id]; !ok {
		return core.ErrNotFound
	}
	delete(f.users, id)
	return nil
}

type fakeTokenStore struct{ tokens map[string]core.AuthToken }

func newFakeTokenStore() *fakeTokenStore {
	return &fakeTokenStore{tokens: map[string]core.AuthToken{}}
}

func (f *fakeTokenStore) Create(_ context.Context, token core.AuthToken) (core.AuthToken, error) {
	token.ID = uuid.New()
	f.tokens[token.TokenHash] = token
	return token, nil
}

func (f *fakeTokenStore) ListAPITokens(_ context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
	result := make([]core.AuthToken, 0)
	for _, token := range f.tokens {
		if token.UserID == userID && token.Kind == core.TokenKindAPI {
			result = append(result, token)
		}
	}
	return result, nil
}

func (f *fakeTokenStore) GetByHash(_ context.Context, hash string) (core.AuthToken, error) {
	token, ok := f.tokens[hash]
	if !ok {
		return core.AuthToken{}, core.ErrNotFound
	}
	return token, nil
}

func (f *fakeTokenStore) Revoke(_ context.Context, hash string) error {
	token, ok := f.tokens[hash]
	if !ok {
		return nil
	}
	now := time.Now()
	token.RevokedAt = &now
	f.tokens[hash] = token
	return nil
}

func (f *fakeTokenStore) RevokeByID(_ context.Context, userID, tokenID uuid.UUID) error {
	for hash, token := range f.tokens {
		if token.ID == tokenID && token.UserID == userID && token.Kind == core.TokenKindAPI {
			now := time.Now()
			token.RevokedAt = &now
			f.tokens[hash] = token
			return nil
		}
	}
	return core.ErrNotFound
}

func (f *fakeTokenStore) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	now := time.Now()
	for hash, token := range f.tokens {
		if token.UserID == userID && token.RevokedAt == nil {
			token.RevokedAt = &now
			f.tokens[hash] = token
		}
	}
	return nil
}

type fakeRuntime struct {
	states                    map[string]core.ContainerState
	created, recreated        []core.ContainerSpec
	pulled                    []*core.RegistryCredential
	pulledImages              []string
	started, stopped, removed []string
	pullErr                   error
	totalMemory               int64
	calls                     []string
	// One scripted stream per container id; a missing id makes Stats fail.
	statsFor map[string][]core.TaskMetric
	folders  []string
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{states: map[string]core.ContainerState{}}
}

func (f *fakeRuntime) Pull(_ context.Context, image string, credential *core.RegistryCredential) error {
	f.pulled = append(f.pulled, credential)
	f.pulledImages = append(f.pulledImages, image)
	f.calls = append(f.calls, "pull")
	return f.pullErr
}

func (f *fakeRuntime) Create(_ context.Context, s core.ContainerSpec) (string, error) {
	f.created = append(f.created, s)
	f.calls = append(f.calls, "create")
	const id = "container-1"
	f.states[id] = core.ContainerState{Exists: true, Status: core.StatusStopped}
	return id, nil
}

func (f *fakeRuntime) Recreate(_ context.Context, _ string, s core.ContainerSpec) (string, error) {
	f.recreated = append(f.recreated, s)
	f.calls = append(f.calls, "recreate")
	id := "recreated"
	f.states[id] = core.ContainerState{Exists: true, Status: core.StatusStopped}
	return id, nil
}

func (f *fakeRuntime) Start(_ context.Context, id string) error {
	f.started = append(f.started, id)
	state := f.states[id]
	state.Exists, state.Status, state.IP = true, core.StatusRunning, "10.0.0.2"
	f.states[id] = state
	return nil
}

func (f *fakeRuntime) Stop(_ context.Context, id string) error {
	f.stopped = append(f.stopped, id)
	state := f.states[id]
	state.Status = core.StatusStopped
	f.states[id] = state
	return nil
}

func (f *fakeRuntime) Remove(_ context.Context, id string) error {
	f.removed = append(f.removed, id)
	delete(f.states, id)
	return nil
}

func (f *fakeRuntime) Folders(context.Context, string) ([]string, error) { return f.folders, nil }

func (f *fakeRuntime) Inspect(_ context.Context, id string) (core.ContainerState, error) {
	return f.states[id], nil
}

func (f *fakeRuntime) Logs(context.Context, string, core.LogOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("log")), nil
}

func (f *fakeRuntime) TotalMemory(context.Context) (int64, error) { return f.totalMemory, nil }

func (f *fakeRuntime) Stats(ctx context.Context, containerID string) (<-chan core.TaskMetric, error) {
	samples, ok := f.statsFor[containerID]
	if !ok {
		return nil, errors.New("no stats for " + containerID)
	}
	// Unbuffered like the real stream, so an abandoned consumer parks this goroutine.
	out := make(chan core.TaskMetric)
	go func() {
		defer close(out)
		for _, s := range samples {
			select {
			case out <- s:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

type fakeRoutes struct {
	registered map[string]string
	routes     map[string][]core.Route
}

func newFakeRoutes() *fakeRoutes {
	return &fakeRoutes{registered: map[string]string{}, routes: map[string][]core.Route{}}
}

func (f *fakeRoutes) SetRoutes(project, task string, routes []core.Route) {
	f.routes[project+"/"+task] = routes
}

func (f *fakeRoutes) Check(context.Context, core.Route) core.RouteCheck {
	return core.RouteCheck{Status: 200}
}

func (f *fakeRoutes) Register(_ context.Context, project, task, ip string, _ int) error {
	f.registered[project+"/"+task] = ip
	return nil
}

func (f *fakeRoutes) Unregister(_ context.Context, project, task string) error {
	delete(f.registered, project+"/"+task)
	return nil
}

// fakeBoraStore ignores visibility, which the Postgres store tests cover.
type fakeBoraStore struct {
	mu    sync.Mutex
	convs map[uuid.UUID]core.BoraConversation
}

func newFakeBoraStore() *fakeBoraStore {
	return &fakeBoraStore{convs: map[uuid.UUID]core.BoraConversation{}}
}

func (f *fakeBoraStore) Save(_ context.Context, c core.BoraConversation, turn []core.BoraMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c.Messages = append(slices.Clone(f.convs[c.ID].Messages), turn...)
	f.convs[c.ID] = c
	return nil
}

func (f *fakeBoraStore) Get(_ context.Context, userID uuid.UUID, _ bool, id uuid.UUID) (core.BoraConversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.convs[id]
	if !ok || c.UserID != userID {
		return core.BoraConversation{}, core.ErrNotFound
	}
	c.Messages = slices.Clone(c.Messages)
	return c, nil
}

func (f *fakeBoraStore) List(_ context.Context, userID uuid.UUID, _ bool, _ string, _ core.BoraCursor, limit int) ([]core.BoraConversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []core.BoraConversation
	for _, c := range f.convs {
		if c.UserID == userID && len(out) < limit {
			c.Messages = nil
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeBoraStore) AddRan(_ context.Context, id uuid.UUID, index int, line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := f.convs[id].Messages
	if index < 0 || index >= len(messages) || messages[index].Role != core.ChatAssistant || len(messages[index].Ran) >= 10 {
		return core.ErrInvalidInput
	}
	messages[index].Ran = append(messages[index].Ran, line)
	return nil
}

func (f *fakeBoraStore) Delete(_ context.Context, userID uuid.UUID, _ bool, id *uuid.UUID) ([]uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	found := false
	var images []uuid.UUID
	for cid, c := range f.convs {
		if c.UserID == userID && (id == nil || cid == *id) {
			for _, m := range c.Messages {
				images = append(images, m.Images...)
			}
			delete(f.convs, cid)
			found = true
		}
	}
	if id != nil && !found {
		return nil, core.ErrNotFound
	}
	return images, nil
}

// fakeObjects keeps objects in memory and records every deleted key.
type fakeObjects struct {
	mu      sync.Mutex
	objects map[string]core.Image
	deleted []string
}

func newFakeObjects() *fakeObjects { return &fakeObjects{objects: map[string]core.Image{}} }

func (f *fakeObjects) Put(_ context.Context, key, contentType string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = core.Image{MIME: contentType, Data: body}
	return nil
}

func (f *fakeObjects) Get(_ context.Context, key string) ([]byte, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.objects[key]
	if !ok {
		return nil, "", core.ErrNotFound
	}
	return o.Data, o.MIME, nil
}

func (f *fakeObjects) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}
