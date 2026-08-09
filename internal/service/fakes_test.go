package service

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

type fakeTaskStore struct {
	tasks map[uuid.UUID]core.Task
	now   time.Time
}

func newFakeTaskStore() *fakeTaskStore {
	return &fakeTaskStore{tasks: map[uuid.UUID]core.Task{}, now: time.Unix(1000, 0).UTC()}
}

func (f *fakeTaskStore) tick() time.Time {
	f.now = f.now.Add(time.Second)
	return f.now
}

func (f *fakeTaskStore) List(_ context.Context, projectID uuid.UUID) ([]core.Task, error) {
	result := make([]core.Task, 0, len(f.tasks))
	for _, task := range f.tasks {
		if task.ProjectID == projectID {
			result = append(result, task.Clone())
		}
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
	existing, ok := f.tasks[task.ID]
	if !ok {
		return core.Task{}, core.ErrNotFound
	}
	task.CreatedAt = existing.CreatedAt
	task.UpdatedAt = f.tick()
	f.tasks[task.ID] = task.Clone()
	return task.Clone(), nil
}

func (f *fakeTaskStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.tasks[id]; !ok {
		return core.ErrNotFound
	}
	delete(f.tasks, id)
	return nil
}

type fakeProjectStore struct {
	projects map[uuid.UUID]core.Project
	members  map[uuid.UUID]map[uuid.UUID]core.ProjectMember
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

func (f *fakeProjectStore) ListForUser(_ context.Context, userID uuid.UUID) ([]core.Project, error) {
	result := make([]core.Project, 0)
	for id, members := range f.members {
		if _, ok := members[userID]; ok {
			result = append(result, f.projects[id])
		}
	}
	return result, nil
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
	for _, existing := range f.projects {
		if existing.Slug == project.Slug {
			return core.Project{}, core.ErrAlreadyExists
		}
	}
	project.ID = uuid.New()
	project.CreatedAt, project.UpdatedAt = time.Now(), time.Now()
	f.projects[project.ID] = project
	return project, nil
}

func (f *fakeProjectStore) Update(_ context.Context, project core.Project) (core.Project, error) {
	if _, ok := f.projects[project.ID]; !ok {
		return core.Project{}, core.ErrNotFound
	}
	project.UpdatedAt = time.Now()
	f.projects[project.ID] = project
	return project, nil
}

func (f *fakeProjectStore) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.projects[id]; !ok {
		return core.ErrNotFound
	}
	delete(f.projects, id)
	delete(f.members, id)
	return nil
}

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
	if _, ok := f.members[projectID][userID]; !ok {
		return core.ErrNotFound
	}
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

type fakeUserStore struct{ users map[uuid.UUID]core.User }

func newFakeUserStore() *fakeUserStore { return &fakeUserStore{users: map[uuid.UUID]core.User{}} }

func (f *fakeUserStore) List(context.Context) ([]core.User, error) {
	result := make([]core.User, 0, len(f.users))
	for _, user := range f.users {
		result = append(result, user)
	}
	return result, nil
}

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
	for _, existing := range f.users {
		if strings.EqualFold(existing.Username, user.Username) {
			return core.User{}, core.ErrAlreadyExists
		}
	}
	user.ID = uuid.New()
	user.CreatedAt, user.UpdatedAt = time.Now(), time.Now()
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeUserStore) Update(_ context.Context, user core.User) (core.User, error) {
	if _, ok := f.users[user.ID]; !ok {
		return core.User{}, core.ErrNotFound
	}
	user.UpdatedAt = time.Now()
	f.users[user.ID] = user
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

func (f *fakeTokenStore) Create(_ context.Context, token core.AuthToken) error {
	f.tokens[token.TokenHash] = token
	return nil
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
	nextID                    string
	created, recreated        []core.ContainerSpec
	pulled                    []*core.RegistryCredential
	pulledImages              []string
	started, stopped, removed []string
	totalMemory               int64
	// calls verifies that changed images are pulled before container recreation.
	calls []string
}

func newFakeRuntime() *fakeRuntime {
	return &fakeRuntime{states: map[string]core.ContainerState{}, nextID: "container-1"}
}

func (f *fakeRuntime) Pull(_ context.Context, image string, credential *core.RegistryCredential) error {
	f.pulled = append(f.pulled, credential)
	f.pulledImages = append(f.pulledImages, image)
	f.calls = append(f.calls, "pull")
	return nil
}

func (f *fakeRuntime) Create(_ context.Context, s core.ContainerSpec) (string, error) {
	f.created = append(f.created, s)
	f.calls = append(f.calls, "create")
	id := f.nextID
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

func (f *fakeRuntime) Inspect(_ context.Context, id string) (core.ContainerState, error) {
	state, ok := f.states[id]
	if !ok {
		return core.ContainerState{}, core.ErrNotFound
	}
	return state, nil
}

func (f *fakeRuntime) Logs(context.Context, string, core.LogOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("log")), nil
}

func (f *fakeRuntime) TotalMemory(context.Context) (int64, error) { return f.totalMemory, nil }

type fakeRoutes struct {
	registered   map[string]string
	unregistered []string
}

func newFakeRoutes() *fakeRoutes { return &fakeRoutes{registered: map[string]string{}} }

func (f *fakeRoutes) Register(_ context.Context, project, task, ip string, _ int) error {
	f.registered[project+"/"+task] = ip
	return nil
}

func (f *fakeRoutes) Unregister(_ context.Context, project, task string) error {
	key := project + "/" + task
	delete(f.registered, key)
	f.unregistered = append(f.unregistered, key)
	return nil
}
