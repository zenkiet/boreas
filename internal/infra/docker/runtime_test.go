package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// Re-creation must reclaim an orphan left after Boreas loses its database record,
// even when the container ID it recorded no longer exists.
func TestRecreateReclaimsAnOrphanBehindAStaleID(t *testing.T) {
	r := newRuntime(t)
	ctx := context.Background()
	spec := testSpec("orphan")

	orphan, err := r.Create(ctx, spec)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), orphan) })

	id, err := r.Recreate(ctx, "0000000000000000000000000000000000000000000000000000000000000000", spec)
	if err != nil {
		t.Fatalf("recreate must reclaim the name, got %v", err)
	}
	t.Cleanup(func() { _ = r.Remove(context.Background(), id) })
	if state, err := r.Inspect(ctx, orphan); err != nil || id == orphan || state.Exists {
		t.Fatalf("the orphan was not replaced: id=%s state=%+v err=%v", id, state, err)
	}
}

// Ownership labels must prevent destructive name reclamation.
func TestCreateRefusesToTakeAContainerItDoesNotOwn(t *testing.T) {
	r := newRuntime(t)
	ctx := context.Background()
	spec := testSpec("foreign")
	for name, labels := range map[string]map[string]string{
		"foreign manager": {"managed-by": "someone-else"},
		"another task":    {"managed-by": "boreas", "project": spec.Project, "task": "someone-elses-task"},
	} {
		t.Run(name, func(t *testing.T) {
			created, err := r.client.ContainerCreate(ctx, &container.Config{Image: testImage, Labels: labels},
				&container.HostConfig{}, &network.NetworkingConfig{}, nil, containerName(spec))
			if err != nil {
				t.Fatalf("seed container: %v", err)
			}
			t.Cleanup(func() { _ = r.Remove(context.Background(), created.ID) })
			if _, err := r.Create(ctx, spec); !errors.Is(err, core.ErrConflict) {
				t.Fatalf("want ErrConflict, got %v", err)
			}
			if state, err := r.Inspect(ctx, created.ID); err != nil || !state.Exists {
				t.Fatalf("a container Boreas does not own was removed: %+v, %v", state, err)
			}
		})
	}
}

func TestMounts(t *testing.T) {
	got := mounts(core.ContainerSpec{Project: "shop", Volumes: map[string]string{"/app/certs": "certs"}})
	if len(got) != 1 || got[0].Source != appdataVolume || got[0].Target != "/app/certs" || !got[0].ReadOnly ||
		got[0].VolumeOptions.Subpath != "shop/certs" {
		t.Fatalf("mounts = %+v", got)
	}
}

func TestFolders(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"certs", "init", "_shared"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("_shared", filepath.Join(dir, "tls")); err != nil {
		t.Fatal(err)
	}
	got, err := folders(dir)
	if err != nil || strings.Join(got, ",") != "_shared,certs,init,tls" {
		t.Fatalf("folders = %v, %v", got, err)
	}
	if missing, err := folders(filepath.Join(dir, "absent")); err != nil || len(missing) != 0 {
		t.Fatalf("a project without a folder must list none: %v, %v", missing, err)
	}
}
