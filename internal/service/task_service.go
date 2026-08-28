// Package service implements Boreas application use cases over core ports.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

type Config struct {
	DefaultPort      int
	ReadinessTimeout time.Duration
	PollInterval     time.Duration
	// Notify reports deploy outcomes. It defaults to a no-op so callers need no nil check.
	Notify func(context.Context, core.Notification)
}

type TaskService struct {
	runtime     core.ContainerRuntime
	tasks       core.TaskStore
	projects    core.ProjectStore
	credentials core.CredentialStore
	routes      core.RouteRegistry
	ready       func(context.Context, string) error
	cfg         Config
	locksMu     sync.Mutex
	locks       map[string]*sync.Mutex
}

func NewTaskService(
	runtime core.ContainerRuntime,
	tasks core.TaskStore,
	projects core.ProjectStore,
	credentials core.CredentialStore,
	routes core.RouteRegistry,
	ready func(context.Context, string) error,
	cfg Config,
) (*TaskService, error) {
	if runtime == nil || tasks == nil || projects == nil || credentials == nil || routes == nil {
		return nil, errors.Join(core.ErrInvalidInput,
			errors.New("runtime, task store, project store, credential store, and route registry are required"))
	}
	if cfg.DefaultPort == 0 {
		cfg.DefaultPort = 80
	}
	if cfg.ReadinessTimeout == 0 {
		cfg.ReadinessTimeout = 30 * time.Second
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	if cfg.Notify == nil {
		cfg.Notify = func(context.Context, core.Notification) {}
	}
	if cfg.DefaultPort < 1 || cfg.DefaultPort > 65535 || cfg.ReadinessTimeout < 0 || cfg.PollInterval <= 0 {
		return nil, errors.Join(core.ErrInvalidInput, errors.New("invalid service configuration"))
	}
	return &TaskService{
		runtime: runtime, tasks: tasks, projects: projects, credentials: credentials,
		routes: routes, ready: ready, cfg: cfg, locks: make(map[string]*sync.Mutex),
	}, nil
}

type CreateTaskInput struct {
	Name        string
	Description string
	Note        string
	Image       string
	Port        int
	Labels      map[string]string
	Env         map[string]string
}

func (s *TaskService) List(ctx context.Context, acc core.ProjectAccess) ([]core.Task, error) {
	tasks, err := s.tasks.List(ctx, acc.Project.ID, acc.UserID, acc.AllTasks)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	for i := range tasks {
		tasks[i] = tasks[i].Clone()
	}
	return tasks, nil
}

func (s *TaskService) Get(ctx context.Context, slug, name string) (core.Task, error) {
	task, _, err := s.get(ctx, slug, name)
	return task.Clone(), err
}

func (s *TaskService) get(ctx context.Context, slug, name string) (core.Task, core.Project, error) {
	if err := core.ValidateTaskName(name); err != nil {
		return core.Task{}, core.Project{}, err
	}
	project, err := s.project(ctx, slug)
	if err != nil {
		return core.Task{}, core.Project{}, err
	}
	task, err := s.tasks.GetByName(ctx, project.ID, name)
	if err != nil {
		return core.Task{}, core.Project{}, fmt.Errorf("get task %q: %w", name, err)
	}
	return task, project, nil
}

func (s *TaskService) credential(ctx context.Context, project core.Project) (*core.RegistryCredential, error) {
	if project.RegistryCredentialID == nil {
		return nil, nil
	}
	stored, err := s.credentials.Get(ctx, *project.RegistryCredentialID)
	if err != nil {
		return nil, fmt.Errorf("get registry credential: %w", err)
	}
	return &stored, nil
}

func (s *TaskService) project(ctx context.Context, slug string) (core.Project, error) {
	if err := core.ValidateProjectSlug(slug); err != nil {
		return core.Project{}, err
	}
	project, err := s.projects.GetBySlug(ctx, slug)
	if err != nil {
		return core.Project{}, fmt.Errorf("get project %q: %w", slug, err)
	}
	return project, nil
}

func (s *TaskService) Create(ctx context.Context, slug string, in CreateTaskInput) (core.Task, error) {
	unlock := s.lockTask(slug, in.Name)
	defer unlock()
	project, err := s.project(ctx, slug)
	if err != nil {
		return core.Task{}, err
	}
	port := in.Port
	if port == 0 {
		port = s.cfg.DefaultPort
	}
	spec := core.ContainerSpec{
		Project: project.Slug, Name: in.Name, Image: strings.TrimSpace(in.Image), Port: port,
		Labels: maps.Clone(in.Labels), Env: maps.Clone(in.Env),
	}
	if err := spec.Validate(); err != nil {
		return core.Task{}, err
	}

	task, err := s.tasks.Create(ctx, core.Task{
		ProjectID: project.ID, Name: in.Name, Description: in.Description, Note: in.Note, Image: spec.Image,
		Status: core.StatusCreating, DevStatus: core.DevInProgress, Port: port,
		Labels: spec.Labels, Env: spec.Env,
	})
	if err != nil {
		return core.Task{}, fmt.Errorf("reserve task %q: %w", in.Name, err)
	}
	detail := strings.TrimSpace(in.Description)
	if detail == "" {
		detail = spec.Image
	}
	s.cfg.Notify(ctx, notification(project, in.Name, core.NotificationInfo, "📋 Task Created", detail))

	credential, err := s.credential(ctx, project)
	if err != nil {
		return s.failTask(ctx, task, err)
	}
	if err := s.runtime.Pull(ctx, spec.Image, credential); err != nil {
		return s.failTask(ctx, task, fmt.Errorf("pull image: %w", err))
	}
	containerID, err := s.runtime.Create(ctx, spec)
	if err != nil {
		return s.failTask(ctx, task, fmt.Errorf("create container: %w", err))
	}
	task.ContainerID, task.Status = containerID, core.StatusStarting
	if task, err = s.tasks.Update(ctx, task); err != nil {
		_ = s.runtime.Remove(ctx, containerID)
		return core.Task{}, fmt.Errorf("persist created container: %w", err)
	}
	if err := s.runtime.Start(ctx, containerID); err != nil {
		return s.failTask(ctx, task, fmt.Errorf("start container: %w", err))
	}
	return s.finishStart(ctx, task, project.Slug)
}

func (s *TaskService) Start(ctx context.Context, slug, name string) (core.Task, error) {
	unlock := s.lockTask(slug, name)
	defer unlock()
	task, project, err := s.get(ctx, slug, name)
	if err != nil {
		return core.Task{}, err
	}
	if task.Status == core.StatusRunning || task.Status == core.StatusStarting || task.Status == core.StatusCreating {
		return core.Task{}, fmt.Errorf("task %q is %s: %w", name, task.Status, core.ErrConflict)
	}
	task.Status, task.Error = core.StatusStarting, ""
	if task, err = s.tasks.Update(ctx, task); err != nil {
		return core.Task{}, fmt.Errorf("mark starting: %w", err)
	}
	if task.PendingRecreate || task.ContainerID == "" {
		newID, recreateErr := s.runtime.Recreate(ctx, task.ContainerID, task.Spec(project.Slug))
		if recreateErr != nil {
			return s.failTask(ctx, task, fmt.Errorf("recreate container: %w", recreateErr))
		}
		task.ContainerID, task.PendingRecreate = newID, false
	}
	if err := s.runtime.Start(ctx, task.ContainerID); err != nil {
		return s.failTask(ctx, task, fmt.Errorf("start container: %w", err))
	}
	return s.finishStart(ctx, task, project.Slug)
}

func (s *TaskService) Stop(ctx context.Context, slug, name string) (core.Task, error) {
	unlock := s.lockTask(slug, name)
	defer unlock()
	task, project, err := s.get(ctx, slug, name)
	if err != nil {
		return core.Task{}, err
	}
	if task.Status == core.StatusStopped {
		return core.Task{}, fmt.Errorf("task %q is stopped: %w", name, core.ErrConflict)
	}
	if task.Status == core.StatusCreating || task.Status == core.StatusStarting {
		return core.Task{}, fmt.Errorf("task %q is transitioning: %w", name, core.ErrConflict)
	}
	if task.ContainerID != "" {
		if err := s.runtime.Stop(ctx, task.ContainerID); err != nil {
			return s.failTask(ctx, task, fmt.Errorf("stop container: %w", err))
		}
	}
	if err := s.routes.Unregister(ctx, project.Slug, name); err != nil && !errors.Is(err, core.ErrNotFound) {
		return core.Task{}, fmt.Errorf("unregister route: %w", err)
	}
	task.Status, task.ContainerIP, task.Error = core.StatusStopped, "", ""
	if task, err = s.tasks.Update(ctx, task); err != nil {
		return core.Task{}, fmt.Errorf("persist stopped task: %w", err)
	}
	return task.Clone(), nil
}

func (s *TaskService) Restart(ctx context.Context, slug, name string) (core.Task, error) {
	unlock := s.lockTask(slug, name)
	defer unlock()
	task, project, err := s.get(ctx, slug, name)
	if err != nil {
		return core.Task{}, err
	}
	if task.Status == core.StatusCreating || task.Status == core.StatusStarting {
		return core.Task{}, fmt.Errorf("task %q is transitioning: %w", name, core.ErrConflict)
	}
	credential, err := s.credential(ctx, project)
	if err != nil {
		return core.Task{}, err
	}
	if err := s.runtime.Pull(ctx, task.Image, credential); err != nil {
		return core.Task{}, fmt.Errorf("pull image: %w", err)
	}
	_ = s.routes.Unregister(ctx, project.Slug, name)
	newID, recreateErr := s.runtime.Recreate(ctx, task.ContainerID, task.Spec(project.Slug))
	if recreateErr != nil {
		return s.failTask(ctx, task, fmt.Errorf("recreate container: %w", recreateErr))
	}
	task.ContainerID, task.PendingRecreate = newID, false
	task.Status = core.StatusStarting
	if task, err = s.tasks.Update(ctx, task); err != nil {
		return core.Task{}, fmt.Errorf("mark restarting: %w", err)
	}
	if err := s.runtime.Start(ctx, task.ContainerID); err != nil {
		return s.failTask(ctx, task, fmt.Errorf("restart container: %w", err))
	}
	return s.finishStart(ctx, task, project.Slug)
}

func (s *TaskService) Delete(ctx context.Context, slug, name string) error {
	unlock := s.lockTask(slug, name)
	defer unlock()
	task, project, err := s.get(ctx, slug, name)
	if err != nil {
		return err
	}
	_ = s.routes.Unregister(ctx, project.Slug, name)
	if task.ContainerID != "" {
		state, inspectErr := s.runtime.Inspect(ctx, task.ContainerID)
		if inspectErr == nil && state.Exists && state.Status == core.StatusRunning {
			if err := s.runtime.Stop(ctx, task.ContainerID); err != nil {
				return fmt.Errorf("stop before delete: %w", err)
			}
		}
		if err := s.runtime.Remove(ctx, task.ContainerID); err != nil && !errors.Is(err, core.ErrNotFound) {
			return fmt.Errorf("remove container: %w", err)
		}
	}
	if err := s.tasks.Delete(ctx, task.ID); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

// UpdateTaskInput uses pointers to preserve omitted fields during partial updates.
type UpdateTaskInput struct {
	Description *string
	Note        *string
	DevStatus   *core.DevStatus
	Image       *string
	Port        *int
	Labels      *map[string]string
	Env         *map[string]string
}

// Update avoids container churn for metadata-only edits and can defer container changes until the next start.
func (s *TaskService) Update(ctx context.Context, slug, name string, in UpdateTaskInput, recreate bool) (core.Task, error) {
	unlock := s.lockTask(slug, name)
	defer unlock()
	task, project, err := s.get(ctx, slug, name)
	if err != nil {
		return core.Task{}, err
	}
	updated, err := s.update(ctx, task, project, in, recreate)
	if err == nil && updated.DevStatus != task.DevStatus {
		s.cfg.Notify(ctx, notification(project, name, core.NotificationInfo, "🔄 Status Changed",
			devLabel(task.DevStatus)+" ➔ "+devLabel(updated.DevStatus)))
	}
	return updated, err
}

var deployDigest = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)

// Deploy applies an immutable image and treats an already-deployed image as an idempotent retry.
func (s *TaskService) Deploy(ctx context.Context, slug, name, image string) (core.Task, error) {
	trimmed := strings.TrimSpace(image)
	if !deployDigest.MatchString(trimmed) {
		return core.Task{}, errors.Join(core.ErrInvalidInput,
			errors.New("deploy image must be immutable, of the form repository@sha256:<64 hex digits>"))
	}
	unlock := s.lockTask(slug, name)
	defer unlock()
	task, project, err := s.get(ctx, slug, name)
	if err != nil {
		return core.Task{}, err
	}
	// A retried callback for the image already running is not a new deploy, so it notifies nothing.
	if task.Image == trimmed && !task.PendingRecreate {
		return task.Clone(), nil
	}
	deployed, err := s.update(ctx, task, project, UpdateTaskInput{Image: &trimmed}, true)
	s.cfg.Notify(ctx, deployNotification(project, name, time.Now(), err))
	return deployed, err
}

func notification(project core.Project, task string, status core.NotificationStatus, event, detail string) core.Notification {
	return core.Notification{
		ProjectID: project.ID, TaskName: task, Status: status,
		Title: event + " • " + project.Name,
		Body:  task + ": " + detail,
	}
}

func deployNotification(project core.Project, name string, completedAt time.Time, cause error) core.Notification {
	completed := completedAt.Format("3:04PM")
	if cause != nil {
		return notification(project, name, core.NotificationFailure, "❌ Deploy Failed",
			fmt.Sprintf("Failed at %s: %v", completed, cause))
	}
	return notification(project, name, core.NotificationSuccess, "🚀 Deploy Succeeded", "Task completed at "+completed)
}

var devStatusLabels = map[core.DevStatus]string{
	core.DevInProgress: "In Progress",
	core.DevBlocked:    "Blocked",
	core.DevReady:      "Ready",
}

// devLabel falls back to the raw value so an unlabeled future status still renders.
func devLabel(status core.DevStatus) string {
	if label, ok := devStatusLabels[status]; ok {
		return label
	}
	return string(status)
}

func (s *TaskService) update(ctx context.Context, task core.Task, project core.Project, in UpdateTaskInput, recreate bool) (core.Task, error) {
	var err error

	spec := task.Spec(project.Slug)
	if in.Image != nil {
		spec.Image = strings.TrimSpace(*in.Image)
	}
	if in.Port != nil {
		spec.Port = *in.Port
	}
	if in.Labels != nil {
		spec.Labels = maps.Clone(*in.Labels)
	}
	if in.Env != nil {
		spec.Env = maps.Clone(*in.Env)
	}
	if err := spec.Validate(); err != nil {
		return core.Task{}, err
	}
	if in.DevStatus != nil {
		if err := core.ValidateDevStatus(*in.DevStatus); err != nil {
			return core.Task{}, err
		}
	}
	descriptionChanged := in.Description != nil && *in.Description != task.Description
	noteChanged := in.Note != nil && *in.Note != task.Note
	devStatusChanged := in.DevStatus != nil && *in.DevStatus != task.DevStatus
	containerChanged := spec.Image != task.Image || spec.Port != task.Port ||
		!maps.Equal(spec.Labels, task.Labels) || !maps.Equal(spec.Env, task.Env)
	if recreate && task.PendingRecreate {
		containerChanged = true
	}
	if !descriptionChanged && !noteChanged && !devStatusChanged && !containerChanged {
		return task.Clone(), nil
	}
	if descriptionChanged {
		task.Description = *in.Description
	}
	if noteChanged {
		task.Note = *in.Note
	}
	if devStatusChanged {
		task.DevStatus = *in.DevStatus
	}
	if !containerChanged {
		if task, err = s.tasks.Update(ctx, task); err != nil {
			return core.Task{}, fmt.Errorf("persist task: %w", err)
		}
		return task.Clone(), nil
	}

	if spec.Image != task.Image && recreate {
		credential, err := s.credential(ctx, project)
		if err != nil {
			return core.Task{}, err
		}
		if err := s.runtime.Pull(ctx, spec.Image, credential); err != nil {
			return core.Task{}, fmt.Errorf("pull image: %w", err)
		}
	}

	wasRunning := task.Status == core.StatusRunning
	if wasRunning {
		if err := s.runtime.Stop(ctx, task.ContainerID); err != nil {
			return s.failTask(ctx, task, fmt.Errorf("stop for update: %w", err))
		}
		_ = s.routes.Unregister(ctx, project.Slug, task.Name)
	}
	task.Image, task.Port, task.Labels, task.Env = spec.Image, spec.Port, spec.Labels, spec.Env
	// Clear stale failures because this configuration may fix them.
	task.PendingRecreate, task.Status, task.ContainerIP, task.Error = true, core.StatusStopped, "", ""
	if task, err = s.tasks.Update(ctx, task); err != nil {
		return core.Task{}, fmt.Errorf("persist task: %w", err)
	}
	if !recreate {
		return task.Clone(), nil
	}
	newID, err := s.runtime.Recreate(ctx, task.ContainerID, task.Spec(project.Slug))
	if err != nil {
		return s.failTask(ctx, task, fmt.Errorf("apply update: %w", err))
	}
	task.ContainerID, task.PendingRecreate = newID, false
	if !wasRunning {
		if task, err = s.tasks.Update(ctx, task); err != nil {
			return core.Task{}, fmt.Errorf("persist recreated container: %w", err)
		}
		return task.Clone(), nil
	}
	task.Status = core.StatusStarting
	if err := s.runtime.Start(ctx, newID); err != nil {
		return s.failTask(ctx, task, fmt.Errorf("start recreated container: %w", err))
	}
	return s.finishStart(ctx, task, project.Slug)
}

func (s *TaskService) Logs(ctx context.Context, slug, name string, opts core.LogOptions) (io.ReadCloser, error) {
	if opts.Tail < 0 {
		return nil, errors.Join(core.ErrInvalidInput, errors.New("log tail must not be negative"))
	}
	task, _, err := s.get(ctx, slug, name)
	if err != nil {
		return nil, err
	}
	if task.ContainerID == "" {
		return nil, fmt.Errorf("task has no container: %w", core.ErrConflict)
	}
	r, err := s.runtime.Logs(ctx, task.ContainerID, opts)
	if err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}
	return r, nil
}

// Metrics streams samples for one task, or for every running task when name is empty.
// The channel closes once ctx is done and every source has drained.
func (s *TaskService) Metrics(ctx context.Context, acc core.ProjectAccess, name string) (<-chan core.TaskMetric, error) {
	var sources []core.Task
	if name != "" {
		task, _, err := s.get(ctx, acc.Project.Slug, name)
		if err != nil {
			return nil, err
		}
		if task.ContainerID == "" {
			return nil, fmt.Errorf("task has no container: %w", core.ErrConflict)
		}
		sources = []core.Task{task}
	} else {
		tasks, err := s.tasks.List(ctx, acc.Project.ID, acc.UserID, acc.AllTasks)
		if err != nil {
			return nil, fmt.Errorf("list tasks for metrics: %w", err)
		}
		// Skipped rather than refused, so a project streams while some tasks are down.
		sources = slices.DeleteFunc(tasks, func(t core.Task) bool {
			return t.ContainerID == "" || t.Status != core.StatusRunning
		})
	}

	out := make(chan core.TaskMetric)
	var wg sync.WaitGroup
	for _, task := range sources {
		samples, err := s.runtime.Stats(ctx, task.ContainerID)
		if err != nil {
			// One unreadable container must not sink a whole project's stream.
			if name == "" {
				continue
			}
			return nil, fmt.Errorf("stream metrics: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sample := range samples {
				sample.TaskName = task.Name
				select {
				case out <- sample:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out, nil
}

// Reconcile repairs state and routes because Docker may change while Boreas is offline.
func (s *TaskService) Reconcile(ctx context.Context) error {
	projects, err := s.projects.List(ctx)
	if err != nil {
		return fmt.Errorf("list projects for reconcile: %w", err)
	}
	slugs := make(map[uuid.UUID]string, len(projects))
	for _, project := range projects {
		slugs[project.ID] = project.Slug
	}

	tasks, err := s.tasks.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list for reconcile: %w", err)
	}
	var joined error
	for _, task := range tasks {
		slug, ok := slugs[task.ProjectID]
		if !ok {
			joined = errors.Join(joined, fmt.Errorf("task %q references unknown project", task.Name))
			continue
		}
		state, inspectErr := s.runtime.Inspect(ctx, task.ContainerID)
		if inspectErr != nil || !state.Exists {
			task.Status, task.ContainerIP = core.StatusUnknown, ""
			if inspectErr != nil {
				task.Error = inspectErr.Error()
			}
			_ = s.routes.Unregister(ctx, slug, task.Name)
		} else {
			task.Status, task.ContainerIP, task.Error = state.Status, state.IP, state.Error
			if state.Status == core.StatusRunning && state.IP != "" {
				if routeErr := s.routes.Register(ctx, slug, task.Name, state.IP, task.Port); routeErr != nil {
					joined = errors.Join(joined, fmt.Errorf("route %q/%q: %w", slug, task.Name, routeErr))
				}
			} else {
				_ = s.routes.Unregister(ctx, slug, task.Name)
			}
		}
		if _, updateErr := s.tasks.Update(ctx, task); updateErr != nil {
			joined = errors.Join(joined, fmt.Errorf("update %q: %w", task.Name, updateErr))
		}
	}
	return joined
}

func (s *TaskService) SystemStats(ctx context.Context) (core.SystemStats, error) {
	tasks, err := s.tasks.ListAll(ctx)
	if err != nil {
		return core.SystemStats{}, fmt.Errorf("list for stats: %w", err)
	}
	projectCount, err := s.projects.Count(ctx)
	if err != nil {
		return core.SystemStats{}, fmt.Errorf("count projects: %w", err)
	}
	totalMemory, err := s.runtime.TotalMemory(ctx)
	if err != nil {
		return core.SystemStats{}, fmt.Errorf("runtime stats: %w", err)
	}
	stats := core.SystemStats{
		TotalTasks: len(tasks), TotalProjects: projectCount, TotalMemoryBytes: totalMemory,
	}
	for _, task := range tasks {
		switch task.Status {
		case core.StatusRunning:
			stats.RunningTasks++
		case core.StatusStopped, core.StatusError:
			stats.StoppedTasks++
		}
	}
	return stats, nil
}

func (s *TaskService) finishStart(ctx context.Context, task core.Task, slug string) (core.Task, error) {
	state, err := s.waitReady(ctx, task.ContainerID, task.Port)
	if err != nil {
		return s.failTask(ctx, task, err)
	}
	task.Status, task.ContainerIP, task.Error, task.PendingRecreate = core.StatusRunning, state.IP, "", false
	updated, err := s.tasks.Update(ctx, task)
	if err != nil {
		return core.Task{}, fmt.Errorf("persist running task: %w", err)
	}
	if err := s.routes.Register(ctx, slug, updated.Name, updated.ContainerIP, updated.Port); err != nil {
		return core.Task{}, fmt.Errorf("register route: %w", err)
	}
	return updated.Clone(), nil
}

func (s *TaskService) waitReady(ctx context.Context, containerID string, port int) (core.ContainerState, error) {
	readyCtx, cancel := context.WithTimeout(ctx, s.cfg.ReadinessTimeout)
	defer cancel()
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	for {
		state, err := s.runtime.Inspect(readyCtx, containerID)
		if err == nil {
			if state.Status == core.StatusError || state.Status == core.StatusStopped {
				return state, fmt.Errorf("container became %s: %w", state.Status, core.ErrConflict)
			}
			if state.Status == core.StatusRunning && state.IP != "" {
				if s.ready == nil || s.ready(readyCtx, net.JoinHostPort(state.IP, strconv.Itoa(port))) == nil {
					return state, nil
				}
			}
		}
		select {
		case <-readyCtx.Done():
			return core.ContainerState{}, fmt.Errorf("container readiness: %w", readyCtx.Err())
		case <-ticker.C:
		}
	}
}

func (s *TaskService) failTask(ctx context.Context, task core.Task, cause error) (core.Task, error) {
	task.Status, task.Error = core.StatusError, cause.Error()
	if updated, err := s.tasks.Update(ctx, task); err == nil {
		task = updated
	}
	return task.Clone(), cause
}

func (s *TaskService) lockTask(slug, name string) func() {
	key := slug + "/" + name
	s.locksMu.Lock()
	lock := s.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[key] = lock
	}
	s.locksMu.Unlock()
	lock.Lock()
	return lock.Unlock
}
