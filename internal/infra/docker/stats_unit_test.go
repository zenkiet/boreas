package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

// Marks a frame as having a predecessor; the daemon zeroes PreRead only on the first.
var midStream = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestCPUPercent(t *testing.T) {
	cases := []struct {
		name string
		in   container.StatsResponse
		want float64
	}{
		{
			// Without the PreRead guard this reports 40%.
			name: "first frame reports zero, not lifetime usage",
			in: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 1_000}, SystemUsage: 10_000, OnlineCPUs: 4,
				},
			},
			want: 0,
		},
		{
			name: "half of one cpu on a two cpu host",
			in: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 500}, SystemUsage: 2_000, OnlineCPUs: 2,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 0}, SystemUsage: 0,
				},
				PreRead: midStream,
			},
			want: 50,
		},
		{
			name: "OnlineCPUs unset falls back to the percpu slice",
			in: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage:    container.CPUUsage{TotalUsage: 500, PercpuUsage: []uint64{1, 2, 3, 4}},
					SystemUsage: 2_000,
				},
				PreCPUStats: container.CPUStats{CPUUsage: container.CPUUsage{TotalUsage: 0}},
				PreRead:     midStream,
			},
			want: 100,
		},
		{
			name: "no cpu count at all still reports a usable number",
			in: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 500}, SystemUsage: 1_000,
				},
				PreRead: midStream,
			},
			want: 50,
		},
		{
			name: "counter reset must not produce a negative percentage",
			in: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 100}, SystemUsage: 5_000, OnlineCPUs: 2,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 900}, SystemUsage: 1_000,
				},
				PreRead: midStream,
			},
			want: 0,
		},
		{
			name: "idle container reports zero without dividing by zero",
			in: container.StatsResponse{
				CPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 500}, SystemUsage: 2_000, OnlineCPUs: 2,
				},
				PreCPUStats: container.CPUStats{
					CPUUsage: container.CPUUsage{TotalUsage: 500}, SystemUsage: 2_000,
				},
				PreRead: midStream,
			},
			want: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cpuPercent(c.in); got != c.want {
				t.Fatalf("cpuPercent = %v, want %v", got, c.want)
			}
		})
	}
}

func TestMemoryBytes(t *testing.T) {
	cases := []struct {
		name string
		in   container.MemoryStats
		want int64
	}{
		{
			name: "page cache is excluded like docker stats does",
			in: container.MemoryStats{
				Usage: 200 << 20,
				Stats: map[string]uint64{"inactive_file": 150 << 20},
			},
			want: 50 << 20,
		},
		{
			name: "missing inactive_file leaves usage untouched",
			in:   container.MemoryStats{Usage: 100 << 20},
			want: 100 << 20,
		},
		{
			name: "cache larger than usage clamps to zero rather than going negative",
			in: container.MemoryStats{
				Usage: 10 << 20,
				Stats: map[string]uint64{"inactive_file": 40 << 20},
			},
			want: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := memoryBytes(container.StatsResponse{MemoryStats: c.in}); got != c.want {
				t.Fatalf("memoryBytes = %d, want %d", got, c.want)
			}
		})
	}
}

func TestMetricFromSumsEveryInterface(t *testing.T) {
	got := metricFrom(container.StatsResponse{
		MemoryStats: container.MemoryStats{Usage: 1 << 20, Limit: 8 << 20},
		Networks: map[string]container.NetworkStats{
			"eth0": {RxBytes: 100, TxBytes: 200},
			"eth1": {RxBytes: 5, TxBytes: 7},
		},
	})
	if got.NetworkRXBytes != 105 || got.NetworkTXBytes != 207 {
		t.Fatalf("network totals = rx %d tx %d", got.NetworkRXBytes, got.NetworkTXBytes)
	}
	if got.MemoryBytes != 1<<20 || got.MemoryLimit != 8<<20 {
		t.Fatalf("memory = %d of %d", got.MemoryBytes, got.MemoryLimit)
	}
}
