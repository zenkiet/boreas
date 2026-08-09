package docker

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/zenkiet/boreas/internal/core"
)

const testImage = "busybox:latest"

func newRuntime(t *testing.T) *Runtime {
	t.Helper()
	if os.Getenv("BOREAS_TEST_DOCKER") == "" {
		t.Skip("set BOREAS_TEST_DOCKER=1 to run the Docker runtime tests")
	}
	r, err := New("boreas-test-net", "no")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := r.EnsureNetwork(ctx); err != nil {
		t.Fatalf("network: %v", err)
	}
	if err := r.Pull(ctx, testImage, nil); err != nil {
		t.Fatalf("pull %s: %v", testImage, err)
	}
	return r
}

func testSpec(name string) core.ContainerSpec {
	return core.ContainerSpec{Project: "boreastest", Name: name, Image: testImage, Port: 80}
}

// Re-creation must reclaim an orphan left after Boreas loses its database record.
func TestCreateReplacesAnOrphanedContainerOfTheSameTask(t *testing.T) {
	r := newRuntime(t)
	ctx := context.Background()
	spec := testSpec("orphan")

	first, err := r.Create(ctx, spec)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), first) })

	second, err := r.Create(ctx, spec)
	if err != nil {
		t.Fatalf("second create must reclaim the name, got %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), second) })

	if second == first {
		t.Fatal("expected a new container")
	}
	state, err := r.Inspect(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if state.Exists {
		t.Fatal("the superseded container was not removed")
	}
}

// Foreign ownership labels must prevent destructive name reclamation.
func TestCreateRefusesToTakeAForeignContainerName(t *testing.T) {
	r := newRuntime(t)
	ctx := context.Background()
	spec := testSpec("foreign")
	name := containerName(spec)

	created, err := r.client.ContainerCreate(ctx,
		&container.Config{Image: testImage, Labels: map[string]string{"managed-by": "someone-else"}},
		&container.HostConfig{}, &network.NetworkingConfig{}, nil, name)
	if err != nil {
		t.Fatalf("seed foreign container: %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), created.ID) })

	if _, err := r.Create(ctx, spec); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	state, err := r.Inspect(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Exists {
		t.Fatal("a container Boreas does not manage was removed")
	}
}

func TestCreateRefusesToTakeAnotherTasksContainer(t *testing.T) {
	r := newRuntime(t)
	ctx := context.Background()
	spec := testSpec("mine")
	name := containerName(spec)

	created, err := r.client.ContainerCreate(ctx,
		&container.Config{Image: testImage, Labels: map[string]string{
			"managed-by": "boreas", "project": spec.Project, "task": "someone-elses-task",
		}},
		&container.HostConfig{}, &network.NetworkingConfig{}, nil, name)
	if err != nil {
		t.Fatalf("seed container: %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), created.ID) })

	if _, err := r.Create(ctx, spec); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestRecreateSurvivesAStaleContainerID(t *testing.T) {
	r := newRuntime(t)
	ctx := context.Background()
	spec := testSpec("stale")

	existing, err := r.Create(ctx, spec)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), existing) })

	// An ID Boreas recorded before the container was replaced out of band.
	id, err := r.Recreate(ctx, "0000000000000000000000000000000000000000000000000000000000000000", spec)
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), id) })
	if id == existing {
		t.Fatal("expected a new container")
	}
}
