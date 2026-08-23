package docker

import (
	"cmp"
	"context"
	"encoding/json"

	"github.com/docker/docker/api/types/container"
	"github.com/zenkiet/boreas/internal/core"
)

// Stats streams samples for one container, closing the channel when ctx is done or the
// container stops. Docker fills PreCPUStats itself, so callers keep no state.
func (r *Runtime) Stats(ctx context.Context, containerID string) (<-chan core.TaskMetric, error) {
	response, err := r.client.ContainerStats(ctx, containerID, true)
	if err != nil {
		return nil, mapError("stream container stats", err)
	}
	out := make(chan core.TaskMetric)
	go func() {
		defer close(out)
		defer response.Body.Close()
		decoder := json.NewDecoder(response.Body)
		for {
			var s container.StatsResponse
			// Any decode error means the container stopped or the caller left.
			if err := decoder.Decode(&s); err != nil {
				return
			}
			select {
			case out <- metricFrom(s):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Kept separate as the only seam that tests the mapping without a live daemon.
func metricFrom(s container.StatsResponse) core.TaskMetric {
	var rx, tx int64
	for _, n := range s.Networks {
		rx += int64(n.RxBytes)
		tx += int64(n.TxBytes)
	}
	return core.TaskMetric{
		CPUPercent: cpuPercent(s), MemoryBytes: memoryBytes(s),
		MemoryLimit:    int64(s.MemoryStats.Limit),
		NetworkRXBytes: rx, NetworkTXBytes: tx,
		ObservedAt: s.Read,
	}
}

// Docker reports cumulative counters, so a percentage only exists as a delta. The daemon
// zeroes PreCPUStats on the first frame, where a delta would be lifetime usage, not a rate.
func cpuPercent(s container.StatsResponse) float64 {
	if s.PreRead.IsZero() {
		return 0
	}
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if cpuDelta <= 0 || systemDelta <= 0 {
		return 0
	}
	cpus := cmp.Or(float64(s.CPUStats.OnlineCPUs), float64(len(s.CPUStats.CPUUsage.PercpuUsage)), 1)
	return cpuDelta / systemDelta * cpus * 100
}

// Usage counts reclaimable page cache, so an idle container looks like it is leaking;
// `docker stats` subtracts inactive_file and so do we.
func memoryBytes(s container.StatsResponse) int64 {
	usage := int64(s.MemoryStats.Usage)
	if inactive, ok := s.MemoryStats.Stats["inactive_file"]; ok {
		usage -= int64(inactive)
	}
	return max(usage, 0)
}
