package docker

import (
    "context"
    "encoding/json"
    "io"
    "sync"
    "time"

    "github.com/docker/docker/api/types"
    "golang.org/x/sync/errgroup"
)

// ContainerMetrics represents a normalized snapshot of container resource usage.
type ContainerMetrics struct {
    ContainerID     string    `json:"container_id"`
    ContainerName   string    `json:"container_name"`
    CPUPercent      float64   `json:"cpu_percent"`
    MemoryUsage     uint64    `json:"memory_usage"`
    MemoryLimit     uint64    `json:"memory_limit"`
    MemoryPercent   float64   `json:"memory_percent"`
    NetworkRxBytes  uint64    `json:"network_rx_bytes"`
    NetworkTxBytes  uint64    `json:"network_tx_bytes"`
    BlockReadBytes  uint64    `json:"block_read_bytes"`
    BlockWriteBytes uint64    `json:"block_write_bytes"`
    Timestamp       time.Time `json:"timestamp"`
}

// ParseStats parses a single Docker stats JSON payload (stream element)
// and returns a ContainerMetrics instance (ContainerID/Name left empty
// — they're populated by callers that have container metadata).
func ParseStats(jsonBody io.ReadCloser) (*ContainerMetrics, error) {
    defer jsonBody.Close()
    var s types.StatsJSON
    dec := json.NewDecoder(jsonBody)
    if err := dec.Decode(&s); err != nil {
        return nil, err
    }

    // CPU calculation using Docker formula
    cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
    systemDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)

    var numCPUs float64 = 1
    if s.CPUStats.OnlineCPUs > 0 {
        numCPUs = float64(s.CPUStats.OnlineCPUs)
    } else if len(s.CPUStats.CPUUsage.PercpuUsage) > 0 {
        numCPUs = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
    }

    var cpuPercent float64
    if systemDelta > 0 && cpuDelta > 0 {
        cpuPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
    }

    memUsage := s.MemoryStats.Usage
    memLimit := s.MemoryStats.Limit
    var memPercent float64
    if memLimit > 0 {
        memPercent = float64(memUsage) / float64(memLimit) * 100.0
    }

    var rx, tx uint64
    if s.Networks != nil {
        for _, net := range s.Networks {
            rx += net.RxBytes
            tx += net.TxBytes
        }
    }

    var readBytes, writeBytes uint64
    for _, entry := range s.BlkioStats.IoServiceBytesRecursive {
        switch entry.Op {
        case "Read":
            readBytes += entry.Value
        case "Write":
            writeBytes += entry.Value
        }
    }

    return &ContainerMetrics{
        CPUPercent:      cpuPercent,
        MemoryUsage:     memUsage,
        MemoryLimit:     memLimit,
        MemoryPercent:   memPercent,
        NetworkRxBytes:  rx,
        NetworkTxBytes:  tx,
        BlockReadBytes:  readBytes,
        BlockWriteBytes: writeBytes,
        Timestamp:       s.Read,
    }, nil
}

// GetAllContainerStats fetches stats for all running containers concurrently.
// Each container call has a 3 second timeout. If any fetch fails the error is
// returned (using errgroup semantics).
func (d *Client) GetAllContainerStats(ctx context.Context) ([]ContainerMetrics, error) {
    containers, err := d.ListContainers(ctx)
    if err != nil {
        return nil, err
    }

    var mu sync.Mutex
    results := make([]ContainerMetrics, 0, len(containers))

    eg, egCtx := errgroup.WithContext(ctx)
    for _, ctr := range containers {
        // only consider running containers
        if ctr.State != "running" {
            continue
        }
        c := ctr
        eg.Go(func() error {
            tctx, cancel := context.WithTimeout(egCtx, 3*time.Second)
            defer cancel()

            stats, err := d.ContainerStats(tctx, c.ID)
            if err != nil {
                return err
            }
            // ContainerStats returns a Body that must be closed by caller
            m, err := ParseStats(stats.Body)
            if err != nil {
                return err
            }

            m.ContainerID = c.ID
            if len(c.Names) > 0 {
                name := c.Names[0]
                if len(name) > 0 && name[0] == '/' {
                    name = name[1:]
                }
                m.ContainerName = name
            }

            mu.Lock()
            results = append(results, *m)
            mu.Unlock()
            return nil
        })
    }

    if err := eg.Wait(); err != nil {
        return nil, err
    }
    return results, nil
}
