package handlers

import (
	"sync"

	"backend/docker"
)

// MaxHistoryPoints is the number of stats snapshots retained per container.
// At a 2-second collection interval this gives ~5 minutes of history.
const MaxHistoryPoints = 150

// StatsBuffer is a thread-safe, per-container ring buffer of metrics snapshots.
type StatsBuffer struct {
	mu   sync.RWMutex
	data map[string][]docker.ContainerMetrics // container_id → ring buffer
}

// NewStatsBuffer creates an empty stats buffer.
func NewStatsBuffer() *StatsBuffer {
	return &StatsBuffer{
		data: make(map[string][]docker.ContainerMetrics),
	}
}

// Push appends a metrics snapshot for a container, evicting the oldest entry
// when the buffer is full.
func (b *StatsBuffer) Push(m docker.ContainerMetrics) {
	b.mu.Lock()
	defer b.mu.Unlock()
	buf := b.data[m.ContainerID]
	buf = append(buf, m)
	if len(buf) > MaxHistoryPoints {
		buf = buf[len(buf)-MaxHistoryPoints:]
	}
	b.data[m.ContainerID] = buf
}

// PushAll stores a batch of metrics (one tick's worth).
func (b *StatsBuffer) PushAll(metrics []docker.ContainerMetrics) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range metrics {
		buf := b.data[m.ContainerID]
		buf = append(buf, m)
		if len(buf) > MaxHistoryPoints {
			buf = buf[len(buf)-MaxHistoryPoints:]
		}
		b.data[m.ContainerID] = buf
	}
}

// All returns a copy of the full history map.
func (b *StatsBuffer) All() map[string][]docker.ContainerMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string][]docker.ContainerMetrics, len(b.data))
	for id, buf := range b.data {
		cp := make([]docker.ContainerMetrics, len(buf))
		copy(cp, buf)
		out[id] = cp
	}
	return out
}

// Get returns the history for a single container.
func (b *StatsBuffer) Get(containerID string) []docker.ContainerMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()
	buf := b.data[containerID]
	cp := make([]docker.ContainerMetrics, len(buf))
	copy(cp, buf)
	return cp
}
