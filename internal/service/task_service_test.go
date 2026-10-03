package service

import (
	"context"
	"errors"
	"maps"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

type harness struct {
	svc         *TaskService
	tasks       *fakeTaskStore
	projects    *fakeProjectStore
	credentials *fakeCredentialStore
	runtime     *fakeRuntime
	routes      *fakeRoutes
	project     core.Project
	notified    []core.Notification
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		tasks:       newFakeTaskStore(),
		projects:    newFakeProjectStore(),
		credentials: newFakeCredentialStore(),
		runtime:     newFakeRuntime(),
		routes:      newFakeRoutes(),
	}
	h.project = h.projects.add("team")
	var err error
	h.svc, err = NewTaskService(h.runtime, h.tasks, h.projects, h.credentials, h.routes, nil,
		Config{
			DefaultPort: 8080, PollInterval: time.Millisecond, ReadinessTimeout: time.Second,
			Notify: func(_ context.Context, n core.Notification) {
				h.notified = append(h.notified, n)
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) create(t *testing.T, in CreateTaskInput) core.Task {
	t.Helper()
	task, err := h.svc.Create(context.Background(), "team", in)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestCreateLifecycleAndDefaults(t *testing.T) {
	h := newHarness(t)
	task := h.create(t, CreateTaskInput{Name: "task-1", Image: " image "})
	if task.Status != core.StatusRunning || task.Port != 8080 || task.ContainerIP != "10.0.0.2" ||
		task.Image != "image" || task.ProjectID != h.project.ID {
		t.Fatalf("unexpected task: %+v", task)
	}
	if len(h.runtime.created) != 1 || h.routes.registered["team/task-1"] != task.ContainerIP {
		t.Fatal("create side effects missing")
	}
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "task-1", Image: "x"}); !errors.Is(err, core.ErrAlreadyExists) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateIgnoresProjectFormDefaults(t *testing.T) {
	h := newHarness(t)
	project := h.project
	project.DefaultImage, project.DefaultPort = "preset:image", 9999
	project.DefaultEnv = map[string]string{"PRESET": "yes"}
	if _, err := h.projects.Update(context.Background(), project); err != nil {
		t.Fatal(err)
	}

	for name, in := range map[string]CreateTaskInput{
		"missing image": {Name: "no-image"},
		"blank image":   {Name: "blank-image", Image: "   "},
	} {
		if _, err := h.svc.Create(context.Background(), "team", in); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}

	task := h.create(t, CreateTaskInput{Name: "web", Image: "sent:image"})
	if task.Image != "sent:image" || task.Port == 9999 || len(task.Env) != 0 {
		t.Fatalf("project form defaults leaked into the task: %+v", task)
	}
}

func TestUnknownAndInvalidNamesAreRejected(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.Create(ctx, "missing", CreateTaskInput{Name: "x", Image: "img"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown project: got %v, want ErrNotFound", err)
	}
	if _, err := h.svc.Deploy(ctx, "team", "absent", deployDigestA); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown task: got %v, want ErrNotFound", err)
	}
	if _, err := h.svc.Create(ctx, "team", CreateTaskInput{Name: "bad name", Image: "img"}); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("bad task name: got %v, want ErrInvalidInput", err)
	}
	if _, err := h.svc.Get(ctx, "API", "task"); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("bad project slug: got %v, want ErrInvalidInput", err)
	}
}

func TestCreatePullsWithTheProjectRegistryCredential(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "public", Image: "nginx"})
	credential, err := h.credentials.Create(context.Background(), core.RegistryCredential{
		Name: "ghcr", Registry: core.RegistryGHCR, Username: "bot", Token: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	project := h.project
	project.RegistryCredentialID = &credential.ID
	if _, err := h.projects.Update(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	h.create(t, CreateTaskInput{Name: "private", Image: "ghcr.io/org/app"})
	if p := h.runtime.pulled; len(p) != 2 || p[0] != nil || p[1] == nil || p[1].Token != "secret" {
		t.Fatalf("want an anonymous pull, then one with the project credential: %+v", p)
	}
}

func TestStartStopRestartDelete(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "life", Image: "img"})
	if _, err := h.svc.Stop(context.Background(), "team", "life"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Stop(context.Background(), "team", "life"); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if _, err := h.svc.Start(context.Background(), "team", "life"); err != nil {
		t.Fatal(err)
	}
	restarted, err := h.svc.Restart(context.Background(), "team", "life")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.runtime.pulled) != 2 || len(h.runtime.recreated) != 1 || restarted.ContainerID != "recreated" {
		t.Fatalf("restart did not pull and recreate: pulls=%d recreates=%d task=%+v",
			len(h.runtime.pulled), len(h.runtime.recreated), restarted)
	}
	if len(h.runtime.stopped) < 1 || len(h.runtime.started) < 3 {
		t.Fatalf("stop/start calls: %v/%v", h.runtime.stopped, h.runtime.started)
	}
	if err := h.svc.Delete(context.Background(), "team", "life"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Get(context.Background(), "team", "life"); !errors.Is(err, core.ErrNotFound) {
		t.Fatal("task was not deleted")
	}
	if len(h.runtime.removed) == 0 {
		t.Fatal("container was not removed on delete")
	}
}

const (
	deployDigestA = "ghcr.io/acme/web@sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	deployDigestB = "ghcr.io/acme/web@sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestDeployPullsAndRestartsARunningTask(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: deployDigestA})
	blocked := core.DevBlocked
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &blocked}, true); err != nil {
		t.Fatal(err)
	}
	h.runtime.calls = nil

	deployed, err := h.svc.Deploy(context.Background(), "team", "web", " "+deployDigestB+" ")
	if err != nil {
		t.Fatal(err)
	}
	if deployed.Image != deployDigestB || deployed.Status != core.StatusRunning || deployed.DevStatus != core.DevBlocked {
		t.Fatalf("unexpected task: %+v", deployed)
	}
	pull, recreate := slices.Index(h.runtime.calls, "pull"), slices.Index(h.runtime.calls, "recreate")
	if pull < 0 || recreate < 0 || pull > recreate {
		t.Fatalf("pull must precede recreate, got %v", h.runtime.calls)
	}
	if last := h.runtime.pulledImages[len(h.runtime.pulledImages)-1]; last != deployDigestB ||
		h.runtime.recreated[len(h.runtime.recreated)-1].Image != deployDigestB {
		t.Fatalf("pulled %q, recreated %+v", last, h.runtime.recreated)
	}
	if h.routes.registered["team/web"] != deployed.ContainerIP {
		t.Fatal("route was not restored after the deployment")
	}
}

func TestDeployRetriesPendingRecreate(t *testing.T) {
	h := newHarness(t)
	created := h.create(t, CreateTaskInput{Name: "web", Image: deployDigestA})
	created.PendingRecreate = true
	if _, err := h.tasks.Update(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	h.runtime.calls = nil

	deployed, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestA)
	if err != nil {
		t.Fatal(err)
	}
	if deployed.PendingRecreate || !slices.Contains(h.runtime.calls, "recreate") {
		t.Fatalf("pending deployment was not retried: task=%+v calls=%v", deployed, h.runtime.calls)
	}
}

func TestDeployPullFailureLeavesRunningTaskUntouched(t *testing.T) {
	h := newHarness(t)
	created := h.create(t, CreateTaskInput{Name: "web", Image: deployDigestA})
	h.runtime.calls = nil
	h.runtime.pullErr = errors.New("registry unavailable")

	if _, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestB); err == nil {
		t.Fatal("expected pull failure")
	}
	stored, err := h.svc.Get(context.Background(), "team", "web")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Image != created.Image || stored.Status != core.StatusRunning ||
		stored.ContainerID != created.ContainerID || stored.PendingRecreate {
		t.Fatalf("pull failure changed the task: before=%+v after=%+v", created, stored)
	}
	if len(h.runtime.stopped) != 0 || slices.Contains(h.runtime.calls, "recreate") {
		t.Fatalf("pull failure touched the container: calls=%v stops=%v", h.runtime.calls, h.runtime.stopped)
	}
}

func TestDeployRejectsMutableAndMalformedImages(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: deployDigestA})
	h.runtime.calls = nil

	for name, image := range map[string]string{
		"tag":          "ghcr.io/acme/web:staging",
		"bare digest":  "sha256:" + strings.Repeat("a", 64),
		"short digest": "ghcr.io/acme/web@sha256:abc",
		"other digest": "ghcr.io/acme/web@sha512:" + strings.Repeat("a", 64),
		"empty":        "",
	} {
		if _, err := h.svc.Deploy(context.Background(), "team", "web", image); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("%s: got %v, want ErrInvalidInput", name, err)
		}
	}
	if len(h.runtime.calls) != 0 {
		t.Fatalf("a rejected deployment touched the runtime: %v", h.runtime.calls)
	}
	task, err := h.svc.Get(context.Background(), "team", "web")
	if err != nil {
		t.Fatal(err)
	}
	if task.Image != deployDigestA {
		t.Fatalf("a rejected deployment changed the image: %q", task.Image)
	}
}

func TestDeployNotifiesOutcomeOnlyForRealDeployments(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: deployDigestA})
	if len(h.notified) != 1 {
		t.Fatalf("creating a task should notify creation once: %+v", h.notified)
	}
	created := h.notified[0]
	if created.Type != core.NotificationTaskCreated || created.Title != "📋 Task Created • team" ||
		created.Body != "web: "+deployDigestA {
		t.Fatalf("unexpected creation notification: %+v", created)
	}

	if _, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestB); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 2 {
		t.Fatalf("want 2 notifications, got %+v", h.notified)
	}
	success := h.notified[1]
	if success.Type != core.NotificationDeployed || success.ProjectID != h.project.ID ||
		success.TaskName != "web" || success.Title != "🚀 Deploy Succeeded • team" ||
		!strings.HasPrefix(success.Body, "web: Task completed at ") {
		t.Fatalf("unexpected success notification: %+v", success)
	}

	// A retried callback for the running image is not a deployment.
	h.runtime.calls = nil
	if _, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestB); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 2 || len(h.runtime.calls) != 0 {
		t.Fatalf("a redeploy of the same image did work: notified=%+v calls=%v", h.notified, h.runtime.calls)
	}

	h.runtime.pullErr = errors.New("registry unavailable")
	if _, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestA); err == nil {
		t.Fatal("expected pull failure")
	}
	if len(h.notified) != 3 {
		t.Fatalf("a failed deploy did not notify: %+v", h.notified)
	}
	failure := h.notified[2]
	if failure.Type != core.NotificationDeployFailed || failure.Title != "❌ Deploy Failed • team" ||
		!strings.HasPrefix(failure.Body, "web: Failed at ") ||
		!strings.Contains(failure.Body, "registry unavailable") || strings.Contains(failure.Body, deployDigestA) {
		t.Fatalf("unexpected failure notification: %+v", failure)
	}
}

func TestDeployNotificationMessages(t *testing.T) {
	project := core.Project{ID: uuid.New(), Name: "Online Ordering", Slug: "online-ordering"}

	completedAt := time.Date(2026, time.August, 25, 23, 0, 0, 0, time.UTC)

	success := deployNotification(project, "wo-705", completedAt, nil)
	if success.Status != core.NotificationSuccess ||
		success.Title != "🚀 Deploy Succeeded • Online Ordering" ||
		success.Body != "wo-705: Task completed at 11:00PM" {
		t.Fatalf("unexpected success message: %+v", success)
	}

	failure := deployNotification(project, "wo-705", completedAt, errors.New("registry unavailable"))
	if failure.Status != core.NotificationFailure ||
		failure.Title != "❌ Deploy Failed • Online Ordering" ||
		failure.Body != "wo-705: Failed at 11:00PM: registry unavailable" {
		t.Fatalf("unexpected failure message: %+v", failure)
	}
}

func TestUpdateDevStatusNotifiesChange(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: "img"})
	h.notified = nil

	ready := core.DevReady
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &ready}, false); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 1 {
		t.Fatalf("want 1 notification, got %+v", h.notified)
	}
	change := h.notified[0]
	if change.Type != core.NotificationStatusChanged || change.TaskName != "web" ||
		change.Title != "🔄 Status Changed • team" || change.Body != "web: In Progress ➔ Ready" {
		t.Fatalf("unexpected notification: %+v", change)
	}

	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &ready}, false); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 1 {
		t.Fatalf("an unchanged dev status notified: %+v", h.notified)
	}
}

func TestUpdateDescriptionLeavesContainerAlone(t *testing.T) {
	h := newHarness(t)
	created := h.create(t, CreateTaskInput{Name: "web", Image: "img", Description: "before"})
	stops, recreates := len(h.runtime.stopped), len(h.runtime.recreated)

	description := "after"
	updated, err := h.svc.Update(context.Background(), "team", "web",
		UpdateTaskInput{Description: &description}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "after" || updated.Status != core.StatusRunning ||
		updated.ContainerID != created.ContainerID || updated.PendingRecreate {
		t.Fatalf("a description change disturbed the container: %+v", updated)
	}
	if len(h.runtime.stopped) != stops || len(h.runtime.recreated) != recreates {
		t.Fatalf("container was touched: stops=%d recreates=%d", len(h.runtime.stopped), len(h.runtime.recreated))
	}
	if h.routes.registered["team/web"] != created.ContainerIP {
		t.Fatal("route was disturbed")
	}

	note, blocked := "## Context\n- seed the staging DB\n\n`make db`", core.DevBlocked
	if _, err := h.svc.Update(context.Background(), "team", "web",
		UpdateTaskInput{Note: &note, DevStatus: &blocked}, true); err != nil {
		t.Fatal(err)
	}
	stored, err := h.svc.Get(context.Background(), "team", "web")
	if err != nil || stored.Note != note || stored.DevStatus != core.DevBlocked || stored.Description != "after" {
		t.Fatalf("metadata round trip lost data: %+v, %v", stored, err)
	}
	if len(h.runtime.stopped) != stops || len(h.runtime.recreated) != recreates {
		t.Fatalf("a metadata change touched the container: stops=%d recreates=%d", len(h.runtime.stopped), len(h.runtime.recreated))
	}
}

func TestDevStatusRejectsUnknownValue(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: "img"})
	bogus := core.DevStatus("done")
	if _, err := h.svc.Update(context.Background(), "team", "web",
		UpdateTaskInput{DevStatus: &bogus}, true); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
	task, err := h.svc.Get(context.Background(), "team", "web")
	if err != nil {
		t.Fatal(err)
	}
	if task.DevStatus != core.DevInProgress {
		t.Fatalf("a rejected update changed dev status to %q", task.DevStatus)
	}
}

func TestUpdateWithoutImageChangeSkipsPull(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: "img"})
	h.runtime.calls = nil

	port := 9090
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Port: &port}, true); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(h.runtime.calls, "pull") {
		t.Fatalf("unchanged image was pulled again: %v", h.runtime.calls)
	}
	if h.routes.registered["team/web"] == "" {
		t.Fatal("route was not restored after the port change")
	}
}

func TestUpdateDeferredUntilNextStart(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: "old"})
	recreates := len(h.runtime.recreated)

	image := "new"
	deferred, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Image: &image}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !deferred.PendingRecreate || deferred.Status != core.StatusStopped {
		t.Fatalf("unexpected deferred task: %+v", deferred)
	}
	if len(h.runtime.recreated) != recreates {
		t.Fatal("deferred update recreated the container immediately")
	}
	started, err := h.svc.Start(context.Background(), "team", "web")
	if err != nil {
		t.Fatal(err)
	}
	if spec := h.runtime.recreated[len(h.runtime.recreated)-1]; started.PendingRecreate ||
		spec.Image != "new" || spec.Project != "team" || spec.Name != "web" {
		t.Fatalf("deferred update was not applied on start: task=%+v spec=%+v", started, spec)
	}
}

func TestUpdateDoesNotStartAStoppedTask(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: "img"})
	if _, err := h.svc.Stop(context.Background(), "team", "web"); err != nil {
		t.Fatal(err)
	}
	starts := len(h.runtime.started)

	image := "new"
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Image: &image}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != core.StatusStopped || updated.Image != "new" || len(h.runtime.started) != starts {
		t.Fatalf("stopped task was started: status=%s starts=%d", updated.Status, len(h.runtime.started))
	}
	if updated.PendingRecreate {
		t.Fatal("the container was recreated, so nothing should still be pending")
	}
}

func TestUpdateRejectsInvalidInput(t *testing.T) {
	h := newHarness(t)
	h.create(t, CreateTaskInput{Name: "web", Image: "img"})
	reserved := map[string]string{"BOREAS_PORT": "1"}
	badPort := 0
	blank := "   "
	for name, in := range map[string]UpdateTaskInput{
		"reserved env": {Env: &reserved},
		"port":         {Port: &badPort},
		"empty image":  {Image: &blank},
	} {
		if _, err := h.svc.Update(context.Background(), "team", "web", in, true); !errors.Is(err, core.ErrInvalidInput) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
	task, err := h.svc.Get(context.Background(), "team", "web")
	if err != nil {
		t.Fatal(err)
	}
	if task.Image != "img" || task.Status != core.StatusRunning {
		t.Fatalf("a rejected update changed the task: %+v", task)
	}
}

func TestUpdateWithUnchangedValuesIsANoOp(t *testing.T) {
	h := newHarness(t)
	created := h.create(t, CreateTaskInput{
		Name: "web", Image: "img", Port: 80,
		Labels: map[string]string{"tier": "web"}, Env: map[string]string{"A": "B"},
	})
	h.runtime.calls = nil
	description, note, dev, image, port := created.Description, created.Note, created.DevStatus, created.Image, created.Port
	labels, env := maps.Clone(created.Labels), maps.Clone(created.Env)
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{
		Description: &description, Note: &note, DevStatus: &dev, Image: &image, Port: &port, Labels: &labels, Env: &env,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.Equal(created.UpdatedAt) || len(h.runtime.calls) != 0 {
		t.Fatalf("unchanged values caused work: task=%+v calls=%v", updated, h.runtime.calls)
	}
}

func TestUpdateClearsAPreviousError(t *testing.T) {
	h := newHarness(t)
	created := h.create(t, CreateTaskInput{Name: "web", Image: "broken"})
	created.Status, created.Error = core.StatusError, "container readiness: context deadline exceeded"
	if _, err := h.tasks.Update(context.Background(), created); err != nil {
		t.Fatal(err)
	}

	image := "fixed"
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Image: &image}, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Error != "" {
		t.Fatalf("stale error survived the update: %q", updated.Error)
	}
}

func TestReconcileLogsAndStats(t *testing.T) {
	h := newHarness(t)
	running := seedTask(t, h.tasks, core.Task{ProjectID: h.project.ID, Name: "run", ContainerID: "c", Status: core.StatusUnknown})
	stopped := seedTask(t, h.tasks, core.Task{ProjectID: h.project.ID, Name: "stop", ContainerID: "s", Status: core.StatusRunning})
	h.runtime.states["c"] = core.ContainerState{Exists: true, Status: core.StatusRunning, IP: "10.0.0.4"}
	h.runtime.states["s"] = core.ContainerState{Exists: true, Status: core.StatusStopped}

	if err := h.svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.routes.registered["team/run"] != "10.0.0.4" {
		t.Fatalf("route not restored: %v", h.routes.registered)
	}
	if h.tasks.tasks[running.ID].Status != core.StatusRunning || h.tasks.tasks[stopped.ID].Status != core.StatusStopped {
		t.Fatalf("reconcile mismatch: run=%+v stop=%+v", h.tasks.tasks[running.ID], h.tasks.tasks[stopped.ID])
	}

	reader, err := h.svc.Logs(context.Background(), "team", "run", core.LogOptions{Tail: 1})
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()
	if _, err := h.svc.Logs(context.Background(), "team", "run", core.LogOptions{Tail: -1}); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}

	h.runtime.totalMemory = 1000
	stats, err := h.svc.SystemStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTasks != 2 || stats.RunningTasks != 1 || stats.StoppedTasks != 1 ||
		stats.TotalProjects != 1 || stats.TotalMemoryBytes != 1000 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestReconcileReportsOrphanedProject(t *testing.T) {
	h := newHarness(t)
	seedTask(t, h.tasks, core.Task{ProjectID: uuid.New(), Name: "orphan", Status: core.StatusRunning})
	if err := h.svc.Reconcile(context.Background()); err == nil {
		t.Fatal("expected a warning for a task with an unknown project")
	}
	if len(h.routes.registered) != 0 {
		t.Fatalf("orphaned route was registered: %v", h.routes.registered)
	}
}

func metricsFor(t *testing.T, h *harness, acc core.ProjectAccess, name string) map[string]int {
	t.Helper()
	stream, err := h.svc.Metrics(context.Background(), acc, name)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	seen := map[string]int{}
	for sample := range stream {
		seen[sample.TaskName]++
	}
	return seen
}

func TestMetricsFansInRunningTasksTheCallerReaches(t *testing.T) {
	h := newHarness(t)
	grantee := uuid.New()
	for name, status := range map[string]core.TaskStatus{
		"web": core.StatusRunning, "api": core.StatusRunning, "db": core.StatusStopped,
	} {
		task := seedTask(t, h.tasks, core.Task{ProjectID: h.project.ID, Name: name, Status: status, ContainerID: "c-" + name})
		if name == "web" {
			h.tasks.roles[grantKey{taskID: task.ID, userID: grantee}] = core.ProjectRoleViewer
		}
	}
	// db has a stream too, so only the status filter can keep a stopped task out.
	h.runtime.statsFor = map[string][]core.TaskMetric{
		"c-web": {{CPUPercent: 1}}, "c-api": {{CPUPercent: 2}, {CPUPercent: 3}}, "c-db": {{CPUPercent: 9}},
	}

	full := metricsFor(t, h, core.ProjectAccess{Project: h.project, AllTasks: true}, "")
	if !maps.Equal(full, map[string]int{"web": 1, "api": 2}) {
		t.Fatalf("member stream = %v, want every running task", full)
	}
	scoped := metricsFor(t, h, core.ProjectAccess{Project: h.project, UserID: grantee}, "")
	if !maps.Equal(scoped, map[string]int{"web": 1}) {
		t.Fatalf("grantee stream = %v, want the granted task only", scoped)
	}
}

func TestMetricsForOneTaskRequiresAContainer(t *testing.T) {
	h := newHarness(t)
	seedTask(t, h.tasks, core.Task{ProjectID: h.project.ID, Name: "web", Status: core.StatusStopped})
	_, err := h.svc.Metrics(context.Background(),
		core.ProjectAccess{Project: h.project, AllTasks: true}, "web")
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

func TestMetricsStopsWhenTheClientLeaves(t *testing.T) {
	h := newHarness(t)
	seedTask(t, h.tasks, core.Task{ProjectID: h.project.ID, Name: "web", Status: core.StatusRunning, ContainerID: "c-web"})
	// More samples than the consumer reads, so the fan-in goroutine is mid-send.
	h.runtime.statsFor = map[string][]core.TaskMetric{
		"c-web": {{CPUPercent: 1}, {CPUPercent: 2}, {CPUPercent: 3}, {CPUPercent: 4}},
	}

	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := h.svc.Metrics(ctx, core.ProjectAccess{Project: h.project, AllTasks: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	<-stream

	// Wait for the sender to park, or the assertion races the goroutines into existence.
	waitUntil(func() bool { return runtime.NumGoroutine() > before })
	cancel()

	// Without the ctx.Done() escape the sender and the closer never return.
	if !waitUntil(func() bool { return runtime.NumGoroutine() <= before }) {
		t.Fatalf("abandoning the stream leaked %d goroutines", runtime.NumGoroutine()-before)
	}
}

func waitUntil(cond func() bool) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// A missing folder must fail the save, before a task row or container exists.
func TestVolumesNeedAnExistingFolder(t *testing.T) {
	h := newHarness(t)
	in := CreateTaskInput{Name: "web", Image: "img", Volumes: map[string]string{"/app/certs": "certs"}}
	if _, err := h.svc.Create(context.Background(), "team", in); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("missing folder: got %v, want ErrInvalidInput", err)
	}
	h.runtime.folders = []string{"certs"}
	if _, err := h.svc.Create(context.Background(), "team", in); err != nil || h.runtime.created[0].Volumes["/app/certs"] != "certs" {
		t.Fatalf("existing folder: %v, %+v", err, h.runtime.created)
	}
}

// A post hook reports only the outcome: it must keep the stage and notify a failure once.
func TestReportBuild(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	h.notified, h.runtime.calls = nil, nil
	running := core.Build{State: core.BuildRunning, Stage: "Test", Progress: 40, URL: "https://ci.example.com/42"}
	for _, b := range []core.Build{running, {State: core.BuildFailure}, {State: core.BuildFailure}} {
		if err := h.svc.ReportBuild(context.Background(), "team", "web", b); err != nil {
			t.Fatal(err)
		}
	}
	task, _ := h.svc.Get(context.Background(), "team", "web")
	if b := task.Build; b.State != core.BuildFailure || b.Stage != "Test" || b.Progress != 40 || b.URL != running.URL || b.At.IsZero() {
		t.Fatalf("build = %+v", b)
	}
	if len(h.notified) != 1 || h.notified[0].Type != core.NotificationBuildFailed || h.notified[0].Body != "web: failed at Test" {
		t.Fatalf("notified %+v, want one build_failed", h.notified)
	}
	nextRun := core.Build{State: core.BuildFailure, URL: "https://ci.example.com/43"}
	if err := h.svc.ReportBuild(context.Background(), "team", "web", nextRun); err != nil || len(h.notified) != 2 {
		t.Fatalf("another run failing: err %v, notified %d, want 2", err, len(h.notified))
	}
	if len(h.runtime.calls) != 0 {
		t.Fatalf("a build report touched the runtime: %v", h.runtime.calls)
	}
	if err := h.svc.ReportBuild(context.Background(), "team", "absent", running); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown task: got %v, want ErrNotFound", err)
	}
}
