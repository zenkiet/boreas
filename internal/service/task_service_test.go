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

func TestNewTaskServiceRequiresCredentialStore(t *testing.T) {
	_, err := NewTaskService(
		newFakeRuntime(), newFakeTaskStore(), newFakeProjectStore(), nil, newFakeRoutes(), nil,
		Config{DefaultPort: 80, PollInterval: time.Millisecond, ReadinessTimeout: time.Second},
	)
	if !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

func TestCreateLifecycleAndDefaults(t *testing.T) {
	h := newHarness(t)
	task, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "task-1", Image: " image "})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != core.StatusRunning || task.Port != 8080 || task.ContainerIP != "10.0.0.2" {
		t.Fatalf("unexpected task: %+v", task)
	}
	if task.Image != "image" {
		t.Fatalf("image was not trimmed: %q", task.Image)
	}
	if task.ProjectID != h.project.ID {
		t.Fatal("task was not scoped to the project")
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

	task, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "sent:image"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Image != "sent:image" || task.Port == 9999 || len(task.Env) != 0 {
		t.Fatalf("project form defaults leaked into the task: %+v", task)
	}
}

func TestCreateAllowsSameNameInDifferentProjects(t *testing.T) {
	h := newHarness(t)
	h.projects.add("other")
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Create(context.Background(), "other", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatalf("same task name in another project was rejected: %v", err)
	}
	if h.routes.registered["team/web"] == "" || h.routes.registered["other/web"] == "" {
		t.Fatalf("both routes should exist: %v", h.routes.registered)
	}
}

func TestCreateUnknownProject(t *testing.T) {
	h := newHarness(t)
	_, err := h.svc.Create(context.Background(), "missing", CreateTaskInput{Name: "x", Image: "img"})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestCreateUsesProjectRegistryCredential(t *testing.T) {
	h := newHarness(t)
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

	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "private", Image: "ghcr.io/org/app"}); err != nil {
		t.Fatal(err)
	}
	if len(h.runtime.pulled) != 1 || h.runtime.pulled[0] == nil || h.runtime.pulled[0].Token != "secret" {
		t.Fatalf("credential was not passed to Pull: %+v", h.runtime.pulled)
	}
}

func TestCreateWithoutCredentialPullsAnonymously(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "public", Image: "nginx"}); err != nil {
		t.Fatal(err)
	}
	if len(h.runtime.pulled) != 1 || h.runtime.pulled[0] != nil {
		t.Fatalf("expected an anonymous pull, got %+v", h.runtime.pulled)
	}
}

func TestStartStopRestartDelete(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "life", Image: "img"}); err != nil {
		t.Fatal(err)
	}
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

func TestUpdateTaskEnvDeferredAndImmediate(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "env", Image: "img", Env: map[string]string{"OLD": "1"}}); err != nil {
		t.Fatal(err)
	}
	deferredEnv := map[string]string{"NEW": "2"}
	deferred, err := h.svc.Update(context.Background(), "team", "env", UpdateTaskInput{Env: &deferredEnv}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !deferred.PendingRecreate || deferred.Status != core.StatusStopped {
		t.Fatalf("unexpected deferred task: %+v", deferred)
	}
	started, err := h.svc.Start(context.Background(), "team", "env")
	if err != nil {
		t.Fatal(err)
	}
	if started.PendingRecreate || len(h.runtime.recreated) != 1 || h.runtime.recreated[0].Env["NEW"] != "2" {
		t.Fatal("deferred recreate not applied")
	}
	if h.runtime.recreated[0].Project != "team" || h.runtime.recreated[0].Name != "env" {
		t.Fatalf("recreate spec lost its identity: %+v", h.runtime.recreated[0])
	}
	immediateEnv := map[string]string{"NOW": "3"}
	updated, err := h.svc.Update(context.Background(), "team", "env", UpdateTaskInput{Env: &immediateEnv}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != core.StatusRunning || len(h.runtime.recreated) != 2 || h.runtime.recreated[1].Env["NOW"] != "3" {
		t.Fatal("immediate recreate not applied")
	}
	task, _ := h.svc.Get(context.Background(), "team", "env")
	env := task.Env
	env["NOW"] = "changed"
	again, _ := h.svc.Get(context.Background(), "team", "env")
	if again.Env["NOW"] != "3" {
		t.Fatal("Get leaked env map")
	}
}

const (
	deployDigestA = "ghcr.io/acme/web@sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	deployDigestB = "ghcr.io/acme/web@sha256:" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestDeployPullsAndRestartsARunningTask(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA}); err != nil {
		t.Fatal(err)
	}
	h.runtime.calls = nil

	deployed, err := h.svc.Deploy(context.Background(), "team", "web", " "+deployDigestB+" ")
	if err != nil {
		t.Fatal(err)
	}
	if deployed.Image != deployDigestB || deployed.Status != core.StatusRunning {
		t.Fatalf("unexpected task: %+v", deployed)
	}
	pull, recreate := slices.Index(h.runtime.calls, "pull"), slices.Index(h.runtime.calls, "recreate")
	if pull < 0 || recreate < 0 || pull > recreate {
		t.Fatalf("pull must precede recreate, got %v", h.runtime.calls)
	}
	if last := h.runtime.pulledImages[len(h.runtime.pulledImages)-1]; last != deployDigestB {
		t.Fatalf("pulled %q", last)
	}
	if h.routes.registered["team/web"] != deployed.ContainerIP {
		t.Fatal("route was not restored after the deployment")
	}
}

func TestDeployLeavesAStoppedTaskStopped(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Stop(context.Background(), "team", "web"); err != nil {
		t.Fatal(err)
	}
	starts := len(h.runtime.started)

	deployed, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestB)
	if err != nil {
		t.Fatal(err)
	}
	if deployed.Status != core.StatusStopped || len(h.runtime.started) != starts {
		t.Fatalf("a stopped task was started: status=%s starts=%d", deployed.Status, len(h.runtime.started))
	}
	if deployed.Image != deployDigestB || deployed.PendingRecreate {
		t.Fatalf("the new image was not applied to the container: %+v", deployed)
	}
}

func TestDeployOfTheSameImageIsANoOp(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA})
	if err != nil {
		t.Fatal(err)
	}
	h.runtime.calls = nil

	deployed, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestA)
	if err != nil {
		t.Fatal(err)
	}
	if deployed.ContainerID != created.ContainerID || deployed.Status != core.StatusRunning {
		t.Fatalf("the container was disturbed: %+v", deployed)
	}
	if len(h.runtime.calls) != 0 {
		t.Fatalf("redeploying the same image touched the runtime: %v", h.runtime.calls)
	}
}

func TestDeployRetriesPendingRecreate(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA})
	if err != nil {
		t.Fatal(err)
	}
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
	created, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA})
	if err != nil {
		t.Fatal(err)
	}
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
	if _, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA}); err != nil {
		t.Fatal(err)
	}
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

func TestDeployUnknownTask(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Deploy(context.Background(), "team", "absent", deployDigestA); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestDeployNotifiesOutcomeOnlyForRealDeployments(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: deployDigestA}); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 1 {
		t.Fatalf("creating a task should notify creation once: %+v", h.notified)
	}
	created := h.notified[0]
	if created.Status != core.NotificationInfo || created.Title != "📋 Task Created • team" ||
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
	if success.Status != core.NotificationSuccess || success.ProjectID != h.project.ID ||
		success.TaskName != "web" || success.Title != "🚀 Deploy Succeeded • team" ||
		!strings.HasPrefix(success.Body, "web: Task completed at ") {
		t.Fatalf("unexpected success notification: %+v", success)
	}

	// A retried callback for the running image is not a deployment.
	if _, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestB); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 2 {
		t.Fatalf("a redeploy of the same image notified: %+v", h.notified)
	}

	h.runtime.pullErr = errors.New("registry unavailable")
	if _, err := h.svc.Deploy(context.Background(), "team", "web", deployDigestA); err == nil {
		t.Fatal("expected pull failure")
	}
	if len(h.notified) != 3 {
		t.Fatalf("a failed deploy did not notify: %+v", h.notified)
	}
	failure := h.notified[2]
	if failure.Status != core.NotificationFailure || failure.Title != "❌ Deploy Failed • team" ||
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
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	h.notified = nil

	ready := core.DevReady
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &ready}, false); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 1 {
		t.Fatalf("want 1 notification, got %+v", h.notified)
	}
	change := h.notified[0]
	if change.Status != core.NotificationInfo || change.TaskName != "web" ||
		change.Title != "🔄 Status Changed • team" || change.Body != "web: In Progress ➔ Ready" {
		t.Fatalf("unexpected notification: %+v", change)
	}

	// Setting the same status again is not a change.
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &ready}, false); err != nil {
		t.Fatal(err)
	}
	if len(h.notified) != 1 {
		t.Fatalf("an unchanged dev status notified: %+v", h.notified)
	}
}

func TestUpdateDescriptionLeavesContainerAlone(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team",
		CreateTaskInput{Name: "web", Image: "img", Description: "before"})
	if err != nil {
		t.Fatal(err)
	}
	stops, recreates := len(h.runtime.stopped), len(h.runtime.recreated)

	description := "after"
	updated, err := h.svc.Update(context.Background(), "team", "web",
		UpdateTaskInput{Description: &description}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "after" {
		t.Fatalf("description = %q", updated.Description)
	}
	if updated.Status != core.StatusRunning || updated.ContainerID != created.ContainerID {
		t.Fatalf("running container was disturbed: %+v", updated)
	}
	if updated.PendingRecreate {
		t.Fatal("a description change must not schedule a recreate")
	}
	if len(h.runtime.stopped) != stops || len(h.runtime.recreated) != recreates {
		t.Fatalf("container was touched: stops=%d recreates=%d", len(h.runtime.stopped), len(h.runtime.recreated))
	}
	if h.routes.registered["team/web"] != created.ContainerIP {
		t.Fatal("route was disturbed")
	}
}

func TestDevStatusStartsInProgressAndMovesWithoutTouchingTheContainer(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	if created.DevStatus != core.DevInProgress {
		t.Fatalf("dev status = %q, want in_progress", created.DevStatus)
	}
	stops, recreates := len(h.runtime.stopped), len(h.runtime.recreated)

	for _, want := range []core.DevStatus{core.DevReady, core.DevBlocked, core.DevInProgress} {
		updated, err := h.svc.Update(context.Background(), "team", "web",
			UpdateTaskInput{DevStatus: &want}, true)
		if err != nil {
			t.Fatal(err)
		}
		if updated.DevStatus != want {
			t.Fatalf("dev status = %q, want %q", updated.DevStatus, want)
		}
		if updated.Status != core.StatusRunning || updated.ContainerID != created.ContainerID {
			t.Fatalf("running container was disturbed: %+v", updated)
		}
		if updated.PendingRecreate {
			t.Fatal("a dev status change must not schedule a recreate")
		}
	}
	if len(h.runtime.stopped) != stops || len(h.runtime.recreated) != recreates {
		t.Fatalf("container was touched: stops=%d recreates=%d", len(h.runtime.stopped), len(h.runtime.recreated))
	}
	if h.routes.registered["team/web"] != created.ContainerIP {
		t.Fatal("route was disturbed")
	}
}

func TestDevStatusRejectsUnknownValue(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
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

func TestDevStatusUnchangedIsANoOp(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	same := core.DevInProgress
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &same}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatal("an unchanged dev status wrote to the store")
	}
}

func TestDeployLeavesDevStatusAlone(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	blocked := core.DevBlocked
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{DevStatus: &blocked}, true); err != nil {
		t.Fatal(err)
	}
	deployed, err := h.svc.Deploy(context.Background(), "team", "web",
		"registry/app@sha256:"+strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if deployed.DevStatus != core.DevBlocked {
		t.Fatalf("deploy changed dev status to %q", deployed.DevStatus)
	}
}

func TestUpdateImagePullsBeforeRecreating(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "old"}); err != nil {
		t.Fatal(err)
	}
	h.runtime.calls = nil

	image := "new:tag"
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Image: &image}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Image != "new:tag" || updated.Status != core.StatusRunning {
		t.Fatalf("unexpected task: %+v", updated)
	}
	pull, recreate := slices.Index(h.runtime.calls, "pull"), slices.Index(h.runtime.calls, "recreate")
	if pull < 0 || recreate < 0 || pull > recreate {
		t.Fatalf("pull must precede recreate, got %v", h.runtime.calls)
	}
	if last := h.runtime.pulledImages[len(h.runtime.pulledImages)-1]; last != "new:tag" {
		t.Fatalf("pulled %q", last)
	}
	if spec := h.runtime.recreated[len(h.runtime.recreated)-1]; spec.Image != "new:tag" {
		t.Fatalf("recreated with %q", spec.Image)
	}
}

func TestUpdateWithoutImageChangeSkipsPull(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "old"}); err != nil {
		t.Fatal(err)
	}
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
	if started.PendingRecreate || h.runtime.recreated[len(h.runtime.recreated)-1].Image != "new" {
		t.Fatalf("deferred update was not applied on start: %+v", started)
	}
}

func TestUpdateDoesNotStartAStoppedTask(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Stop(context.Background(), "team", "web"); err != nil {
		t.Fatal(err)
	}
	starts := len(h.runtime.started)

	image := "new"
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Image: &image}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != core.StatusStopped || len(h.runtime.started) != starts {
		t.Fatalf("stopped task was started: status=%s starts=%d", updated.Status, len(h.runtime.started))
	}
	if updated.PendingRecreate {
		t.Fatal("the container was recreated, so nothing should still be pending")
	}
}

func TestUpdateRejectsInvalidInput(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
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

func TestUpdateWithNoFieldsIsANoOp(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	recreates, updatedAt := len(h.runtime.recreated), created.UpdatedAt
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Image != created.Image || updated.Status != core.StatusRunning {
		t.Fatalf("unexpected task: %+v", updated)
	}
	if len(h.runtime.recreated) != recreates {
		t.Fatal("an empty update touched the container")
	}
	if !updated.UpdatedAt.Equal(updatedAt) {
		t.Fatal("an empty update wrote to the store")
	}
}

func TestUpdateWithUnchangedValuesIsANoOp(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team", CreateTaskInput{
		Name: "web", Image: "img", Port: 80,
		Labels: map[string]string{"tier": "web"}, Env: map[string]string{"A": "B"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.runtime.calls = nil
	description, image, port := created.Description, created.Image, created.Port
	labels, env := maps.Clone(created.Labels), maps.Clone(created.Env)
	updated, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{
		Description: &description, Image: &image, Port: &port, Labels: &labels, Env: &env,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.UpdatedAt.Equal(created.UpdatedAt) || len(h.runtime.calls) != 0 {
		t.Fatalf("unchanged values caused work: task=%+v calls=%v", updated, h.runtime.calls)
	}
}

func TestUpdateClonesSuppliedMaps(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "img"}); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"KEY": "value"}
	if _, err := h.svc.Update(context.Background(), "team", "web", UpdateTaskInput{Env: &env}, true); err != nil {
		t.Fatal(err)
	}
	env["KEY"] = "mutated"
	stored, err := h.svc.Get(context.Background(), "team", "web")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Env["KEY"] != "value" {
		t.Fatalf("service kept the caller's map: %v", stored.Env)
	}
}

func TestUpdateClearsAPreviousError(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "web", Image: "broken"})
	if err != nil {
		t.Fatal(err)
	}
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

func TestUpdatedAtComesFromStore(t *testing.T) {
	h := newHarness(t)
	created, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "clock", Image: "img"})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := h.svc.Stop(context.Background(), "team", "clock")
	if err != nil {
		t.Fatal(err)
	}
	if !stopped.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("updated_at did not advance: %v -> %v", created.UpdatedAt, stopped.UpdatedAt)
	}
	if !stopped.CreatedAt.Equal(created.CreatedAt) {
		t.Fatal("created_at must not change on update")
	}
}

func TestReconcileLogsAndStats(t *testing.T) {
	h := newHarness(t)
	running, err := h.tasks.Create(context.Background(), core.Task{
		ProjectID: h.project.ID, Name: "run", Image: "i", Port: 80,
		ContainerID: "c", Status: core.StatusUnknown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.tasks.Create(context.Background(), core.Task{
		ProjectID: h.project.ID, Name: "stop", Image: "i", Port: 80,
		ContainerID: "s", Status: core.StatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	h.runtime.states["c"] = core.ContainerState{Exists: true, Status: core.StatusRunning, IP: "10.0.0.4"}
	h.runtime.states["s"] = core.ContainerState{Exists: true, Status: core.StatusStopped}

	if err := h.svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.routes.registered["team/run"] != "10.0.0.4" {
		t.Fatalf("route not restored: %v", h.routes.registered)
	}
	stopped, err := h.svc.Get(context.Background(), "team", "stop")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != core.StatusStopped {
		t.Fatalf("reconcile mismatch: %+v", stopped)
	}
	if h.tasks.tasks[running.ID].Status != core.StatusRunning {
		t.Fatal("running task was not reconciled")
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
	if _, err := h.tasks.Create(context.Background(), core.Task{
		ProjectID: uuid.New(), Name: "orphan", Image: "img", Port: 80, Status: core.StatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Reconcile(context.Background()); err == nil {
		t.Fatal("expected a warning for a task with an unknown project")
	}
	if len(h.routes.registered) != 0 {
		t.Fatalf("orphaned route was registered: %v", h.routes.registered)
	}
}

func TestInvalidNamesRejected(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.Create(context.Background(), "team", CreateTaskInput{Name: "bad name", Image: "img"}); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
	if _, err := h.svc.Get(context.Background(), "API", "task"); !errors.Is(err, core.ErrInvalidInput) {
		t.Fatalf("got %v, want ErrInvalidInput", err)
	}
}

// Drains a stream into a per-task sample count.
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

func TestMetricsSkipsTasksThatAreNotRunning(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.tasks.Create(ctx, core.Task{
		ProjectID: h.project.ID, Name: "web", Image: "img", Port: 80,
		Status: core.StatusRunning, ContainerID: "c-web",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.tasks.Create(ctx, core.Task{
		ProjectID: h.project.ID, Name: "db", Image: "img", Port: 80,
		Status: core.StatusStopped, ContainerID: "c-db",
	}); err != nil {
		t.Fatal(err)
	}
	// A stopped container has no scripted stream, so asking for it would fail.
	h.runtime.statsFor = map[string][]core.TaskMetric{
		"c-web": {{CPUPercent: 1}, {CPUPercent: 2}},
	}

	seen := metricsFor(t, h, core.ProjectAccess{Project: h.project, AllTasks: true}, "")
	if seen["web"] != 2 || len(seen) != 1 {
		t.Fatalf("project stream = %v, want two samples for web only", seen)
	}
}

func TestMetricsFansInEveryRunningTask(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, name := range []string{"web", "api"} {
		if _, err := h.tasks.Create(ctx, core.Task{
			ProjectID: h.project.ID, Name: name, Image: "img", Port: 80,
			Status: core.StatusRunning, ContainerID: "c-" + name,
		}); err != nil {
			t.Fatal(err)
		}
	}
	h.runtime.statsFor = map[string][]core.TaskMetric{
		"c-web": {{CPUPercent: 1}},
		"c-api": {{CPUPercent: 2}, {CPUPercent: 3}},
	}

	seen := metricsFor(t, h, core.ProjectAccess{Project: h.project, AllTasks: true}, "")
	if seen["web"] != 1 || seen["api"] != 2 {
		t.Fatalf("fan-in = %v, want web 1 and api 2", seen)
	}
}

func TestMetricsForOneTaskRequiresAContainer(t *testing.T) {
	h := newHarness(t)
	if _, err := h.tasks.Create(context.Background(), core.Task{
		ProjectID: h.project.ID, Name: "web", Image: "img", Port: 80, Status: core.StatusStopped,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.Metrics(context.Background(),
		core.ProjectAccess{Project: h.project, AllTasks: true}, "web")
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
}

func TestMetricsRespectsTaskGrants(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	grantee := uuid.New()
	for _, name := range []string{"web", "secret"} {
		task, err := h.tasks.Create(ctx, core.Task{
			ProjectID: h.project.ID, Name: name, Image: "img", Port: 80,
			Status: core.StatusRunning, ContainerID: "c-" + name,
		})
		if err != nil {
			t.Fatal(err)
		}
		if name == "web" {
			h.tasks.granted[grantKey{taskID: task.ID, userID: grantee}] = true
		}
	}
	h.runtime.statsFor = map[string][]core.TaskMetric{
		"c-web": {{CPUPercent: 1}}, "c-secret": {{CPUPercent: 9}},
	}

	seen := metricsFor(t, h,
		core.ProjectAccess{Project: h.project, UserID: grantee, AllTasks: false}, "")
	if seen["web"] != 1 || len(seen) != 1 {
		t.Fatalf("grantee stream = %v, want web only", seen)
	}
}

func TestMetricsStopsWhenTheClientLeaves(t *testing.T) {
	h := newHarness(t)
	if _, err := h.tasks.Create(context.Background(), core.Task{
		ProjectID: h.project.ID, Name: "web", Image: "img", Port: 80,
		Status: core.StatusRunning, ContainerID: "c-web",
	}); err != nil {
		t.Fatal(err)
	}
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
	<-stream // read one, abandon the rest

	// Wait for the sender to park, or the assertion races the goroutines into existence.
	waitUntil(func() bool { return runtime.NumGoroutine() > before })
	cancel()

	// Without the ctx.Done() escape the sender and the closer never return.
	if !waitUntil(func() bool { return runtime.NumGoroutine() <= before }) {
		t.Fatalf("abandoning the stream leaked %d goroutines", runtime.NumGoroutine()-before)
	}
}

// Polls cond for up to two seconds, reporting whether it ever held.
func waitUntil(cond func() bool) bool {
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
