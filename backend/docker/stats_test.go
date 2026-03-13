package docker

import (
    "encoding/json"
    "io"
    "strings"
    "testing"

    "github.com/docker/docker/api/types"
)

func makeStatsReader(s types.StatsJSON) io.ReadCloser {
    b, _ := json.Marshal(s)
    return io.NopCloser(strings.NewReader(string(b)))
}

func TestParseStats_CPUCalculation(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{
            CPUStats: types.CPUStats{
                CPUUsage:    types.CPUUsage{TotalUsage: 200},
                SystemUsage: 1000,
                OnlineCPUs:  2,
            },
            PreCPUStats: types.CPUStats{
                CPUUsage:    types.CPUUsage{TotalUsage: 100},
                SystemUsage: 500,
            },
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // cpuDelta=100, systemDelta=500, numCPUs=2 => (100/500)*2*100 = 40%
    expected := 40.0
    if m.CPUPercent != expected {
        t.Errorf("expected CPU %.1f%%, got %.1f%%", expected, m.CPUPercent)
    }
}

func TestParseStats_CPUZeroDelta(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{
            CPUStats: types.CPUStats{
                CPUUsage:    types.CPUUsage{TotalUsage: 100},
                SystemUsage: 500,
                OnlineCPUs:  1,
            },
            PreCPUStats: types.CPUStats{
                CPUUsage:    types.CPUUsage{TotalUsage: 100},
                SystemUsage: 500,
            },
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if m.CPUPercent != 0 {
        t.Errorf("expected 0%% CPU, got %.1f%%", m.CPUPercent)
    }
}

func TestParseStats_Memory(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{
            MemoryStats: types.MemoryStats{
                Usage: 256 * 1024 * 1024,
                Limit: 1024 * 1024 * 1024,
            },
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if m.MemoryUsage != 256*1024*1024 {
        t.Errorf("unexpected memory usage: %d", m.MemoryUsage)
    }
    if m.MemoryLimit != 1024*1024*1024 {
        t.Errorf("unexpected memory limit: %d", m.MemoryLimit)
    }
    expected := 25.0
    if m.MemoryPercent != expected {
        t.Errorf("expected %.1f%% memory, got %.1f%%", expected, m.MemoryPercent)
    }
}

func TestParseStats_MemoryZeroLimit(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{
            MemoryStats: types.MemoryStats{
                Usage: 100,
                Limit: 0,
            },
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if m.MemoryPercent != 0 {
        t.Errorf("expected 0%% memory with zero limit, got %.1f%%", m.MemoryPercent)
    }
}

func TestParseStats_Network(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{},
        Networks: map[string]types.NetworkStats{
            "eth0": {RxBytes: 1000, TxBytes: 2000},
            "eth1": {RxBytes: 500, TxBytes: 300},
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if m.NetworkRxBytes != 1500 {
        t.Errorf("expected rx=1500, got %d", m.NetworkRxBytes)
    }
    if m.NetworkTxBytes != 2300 {
        t.Errorf("expected tx=2300, got %d", m.NetworkTxBytes)
    }
}

func TestParseStats_NoNetworks(t *testing.T) {
    s := types.StatsJSON{Stats: types.Stats{}}

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if m.NetworkRxBytes != 0 || m.NetworkTxBytes != 0 {
        t.Errorf("expected zero network bytes, got rx=%d tx=%d", m.NetworkRxBytes, m.NetworkTxBytes)
    }
}

func TestParseStats_BlockIO(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{
            BlkioStats: types.BlkioStats{
                IoServiceBytesRecursive: []types.BlkioStatEntry{
                    {Op: "Read", Value: 4096},
                    {Op: "Write", Value: 8192},
                    {Op: "Read", Value: 1024},
                },
            },
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if m.BlockReadBytes != 5120 {
        t.Errorf("expected block read=5120, got %d", m.BlockReadBytes)
    }
    if m.BlockWriteBytes != 8192 {
        t.Errorf("expected block write=8192, got %d", m.BlockWriteBytes)
    }
}

func TestParseStats_CPUFallbackToPercpu(t *testing.T) {
    s := types.StatsJSON{
        Stats: types.Stats{
            CPUStats: types.CPUStats{
                CPUUsage: types.CPUUsage{
                    TotalUsage:  200,
                    PercpuUsage: []uint64{100, 100, 0, 0},
                },
                SystemUsage: 1000,
                OnlineCPUs:  0, // zero — should fall back to len(PercpuUsage)
            },
            PreCPUStats: types.CPUStats{
                CPUUsage:    types.CPUUsage{TotalUsage: 100},
                SystemUsage: 500,
            },
        },
    }

    m, err := ParseStats(makeStatsReader(s))
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // cpuDelta=100, systemDelta=500, numCPUs=4 => (100/500)*4*100 = 80%
    expected := 80.0
    if m.CPUPercent != expected {
        t.Errorf("expected CPU %.1f%%, got %.1f%%", expected, m.CPUPercent)
    }
}

func TestParseStats_InvalidJSON(t *testing.T) {
    reader := io.NopCloser(strings.NewReader("not json"))
    _, err := ParseStats(reader)
    if err == nil {
        t.Error("expected error for invalid JSON")
    }
}
