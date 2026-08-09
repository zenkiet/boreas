// Package docker implements the core container runtime port with Docker Engine.
package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/zenkiet/boreas/internal/core"
)

// Runtime permits concurrent Docker calls but does not serialize multi-step name reclamation.
type Runtime struct {
	client        *client.Client
	network       string
	restartPolicy string
}

func New(networkName, restartPolicy string) (*Runtime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Runtime{client: cli, network: networkName, restartPolicy: restartPolicy}, nil
}

func (r *Runtime) Close() error { return r.client.Close() }

func (r *Runtime) EnsureNetwork(ctx context.Context) error {
	if strings.TrimSpace(r.network) == "" {
		return errors.Join(core.ErrInvalidInput, errors.New("docker network is required"))
	}
	networks, err := r.client.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("name", r.network))})
	if err != nil {
		return fmt.Errorf("list docker networks: %w", err)
	}
	for _, n := range networks {
		if n.Name == r.network {
			return nil
		}
	}
	_, err = r.client.NetworkCreate(ctx, r.network, network.CreateOptions{Driver: "bridge", Labels: map[string]string{"managed-by": "boreas"}})
	if errdefs.IsConflict(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create docker network %q: %w", r.network, err)
	}
	return nil
}

func (r *Runtime) Pull(ctx context.Context, imageName string, credential *core.RegistryCredential) error {
	if strings.TrimSpace(imageName) == "" {
		return errors.Join(core.ErrInvalidInput, errors.New("image is required"))
	}
	opts := image.PullOptions{RegistryAuth: registryAuth(credential)}
	stream, err := r.client.ImagePull(ctx, imageName, opts)
	if err != nil {
		return mapError("pull image "+imageName, err)
	}
	defer stream.Close()
	// The pull finishes only when the stream is consumed, which also exposes delayed daemon errors.
	decoder := json.NewDecoder(stream)
	for {
		var event struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&event); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("read pull response for %q: %w", imageName, err)
		}
		if event.Error != "" {
			return fmt.Errorf("pull image %q: %s", imageName, event.Error)
		}
	}
}

func (r *Runtime) Create(ctx context.Context, spec core.ContainerSpec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", err
	}
	labels := map[string]string{"managed-by": "boreas", "project": spec.Project, "task": spec.Name}
	for k, v := range spec.Labels {
		labels[k] = v
	}
	port := nat.Port(strconv.Itoa(spec.Port) + "/tcp")
	basePath := "/" + spec.Project + "/" + spec.Name + "/"
	env := make([]string, 0, len(spec.Env)+4)
	env = append(env,
		"BOREAS_PROJECT="+spec.Project,
		"BOREAS_TASK="+spec.Name,
		"BOREAS_PORT="+strconv.Itoa(spec.Port),
		"BASE_HREF="+basePath,
	)
	for key, value := range spec.Env {
		env = append(env, key+"="+value)
	}

	name := containerName(spec)
	create := func() (container.CreateResponse, error) {
		return r.client.ContainerCreate(ctx, &container.Config{
			Image: spec.Image, ExposedPorts: nat.PortSet{port: struct{}{}}, Labels: labels, Env: env,
		}, &container.HostConfig{
			NetworkMode:   container.NetworkMode(r.network),
			PortBindings:  nat.PortMap{port: []nat.PortBinding{}},
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(r.restartPolicy)},
		}, &network.NetworkingConfig{}, nil, name)
	}

	response, err := create()
	if errdefs.IsConflict(err) {
		if removeErr := r.removeSupersededContainer(ctx, name, spec); removeErr != nil {
			return "", removeErr
		}
		response, err = create()
	}
	if err != nil {
		return "", mapError("create container for task "+spec.Project+"/"+spec.Name, err)
	}
	return response.ID, nil
}

// containerName keeps Docker identity stable when Boreas loses its database record.
func containerName(spec core.ContainerSpec) string {
	return "boreas-" + spec.Project + "-" + spec.Name
}

// removeSupersededContainer reclaims only containers whose ownership labels match the task.
func (r *Runtime) removeSupersededContainer(ctx context.Context, name string, spec core.ContainerSpec) error {
	info, err := r.client.ContainerInspect(ctx, name)
	if err != nil {
		return mapError("inspect container "+name, err)
	}
	if info.Config == nil || info.Config.Labels["managed-by"] != "boreas" ||
		info.Config.Labels["project"] != spec.Project || info.Config.Labels["task"] != spec.Name {
		return errors.Join(core.ErrConflict,
			fmt.Errorf("container %q already exists and is not managed by Boreas", name))
	}
	if err := r.Remove(ctx, info.ID); err != nil && !errors.Is(err, core.ErrNotFound) {
		return err
	}
	return nil
}

func (r *Runtime) Recreate(ctx context.Context, oldID string, spec core.ContainerSpec) (string, error) {
	if oldID != "" {
		if err := r.Stop(ctx, oldID); err != nil && !errors.Is(err, core.ErrNotFound) {
			return "", err
		}
		if err := r.Remove(ctx, oldID); err != nil && !errors.Is(err, core.ErrNotFound) {
			return "", err
		}
	}
	id, err := r.Create(ctx, spec)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *Runtime) Start(ctx context.Context, id string) error {
	return mapError("start container "+id, r.client.ContainerStart(ctx, id, container.StartOptions{}))
}

func (r *Runtime) Stop(ctx context.Context, id string) error {
	timeout := 10
	err := r.client.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
	if errdefs.IsNotModified(err) {
		return nil
	}
	return mapError("stop container "+id, err)
}

func (r *Runtime) Remove(ctx context.Context, id string) error {
	return mapError("remove container "+id, r.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}))
}

func (r *Runtime) Inspect(ctx context.Context, id string) (core.ContainerState, error) {
	info, err := r.client.ContainerInspect(ctx, id)
	if errdefs.IsNotFound(err) {
		return core.ContainerState{Exists: false}, nil
	}
	if err != nil {
		return core.ContainerState{}, mapError("inspect container "+id, err)
	}
	result := core.ContainerState{Exists: true, Status: core.StatusUnknown}
	if info.State != nil {
		result.Error = info.State.Error
		switch {
		case info.State.Running:
			result.Status = core.StatusRunning
		case info.State.Status == container.StateCreated || info.State.Restarting:
			result.Status = core.StatusStarting
		case info.State.Status == container.StateExited || info.State.Status == container.StateDead:
			result.Status = core.StatusStopped
		}
	}
	if info.NetworkSettings != nil {
		if endpoint := info.NetworkSettings.Networks[r.network]; endpoint != nil {
			result.IP = endpoint.IPAddress
		}
		if result.IP == "" {
			for _, endpoint := range info.NetworkSettings.Networks {
				if endpoint != nil && endpoint.IPAddress != "" {
					result.IP = endpoint.IPAddress
					break
				}
			}
		}
	}
	return result, nil
}

func (r *Runtime) Logs(ctx context.Context, id string, options core.LogOptions) (io.ReadCloser, error) {
	tail := strconv.Itoa(options.Tail)
	since := ""
	if !options.Since.IsZero() {
		since = options.Since.Format(time.RFC3339Nano)
	}
	stream, err := r.client.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: options.Follow, Tail: tail,
		Timestamps: options.Timestamps, Since: since,
	})
	if err != nil {
		return nil, mapError("read logs for container "+id, err)
	}
	return stream, nil
}

func (r *Runtime) TotalMemory(ctx context.Context) (int64, error) {
	info, err := r.client.Info(ctx)
	if err != nil {
		return 0, fmt.Errorf("docker info: %w", err)
	}
	return info.MemTotal, nil
}

func registryAuth(credential *core.RegistryCredential) string {
	if credential == nil {
		return ""
	}
	server := "https://index.docker.io/v1/"
	if credential.Registry == core.RegistryGHCR {
		server = "https://ghcr.io"
	}
	payload, _ := json.Marshal(registry.AuthConfig{
		Username: credential.Username, Password: credential.Token, ServerAddress: server,
	})
	return base64.URLEncoding.EncodeToString(payload)
}

func mapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errdefs.IsNotFound(err):
		return errors.Join(core.ErrNotFound, fmt.Errorf("%s: %w", operation, err))
	case errdefs.IsConflict(err):
		return errors.Join(core.ErrConflict, fmt.Errorf("%s: %w", operation, err))
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}
