package handlers

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxLongHistoryPoints is the number of coarse memory samples retained per
// container in the long-horizon buffer. At one sample per minute this gives
// ~24 hours of history — long enough to see a memory leak develop.
const MaxLongHistoryPoints = 1440

// LongSampleInterval is the minimum spacing between long-horizon samples for a
// single container. The existing 2s stats collector feeds this buffer but is
// gated to at most one sample per container per minute.
const LongSampleInterval = 60 * time.Second

// memSample is a single coarse memory reading for a container.
type memSample struct {
	T     time.Time `json:"t"`
	Mem   uint64    `json:"mem"`
	Limit uint64    `json:"limit"`
}

// MemHistory is a thread-safe, persisted, per-container long-horizon memory
// buffer. It is keyed by container name (stable across restarts, unlike short
// container IDs) and persisted to DATA_DIR so history survives restarts.
type MemHistory struct {
	mu         sync.RWMutex
	data       map[string][]memSample
	lastSample map[string]time.Time
	dir        string
}

// MemTrend is a computed summary of a container's long-horizon memory history,
// suitable for an LLM to reason about a memory leak in a single glance.
type MemTrend struct {
	Window            string      `json:"window"`
	Samples           int         `json:"samples"`
	FirstBytes        uint64      `json:"first_bytes"`
	LastBytes         uint64      `json:"last_bytes"`
	MinBytes          uint64      `json:"min_bytes"`
	MaxBytes          uint64      `json:"max_bytes"`
	SlopeBytesPerHour int64       `json:"slope_bytes_per_hour"`
	Points            []memSample `json:"points"`
}

// NewMemHistory creates a MemHistory rooted at dataDir and hydrates it from
// disk. A missing file is fine; a corrupt file is logged and skipped — startup
// never fails on it.
func NewMemHistory(dataDir string) *MemHistory {
	h := &MemHistory{
		data:       make(map[string][]memSample),
		lastSample: make(map[string]time.Time),
		dir:        dataDir,
	}
	if err := h.load(); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("mem-history: ignoring unreadable %s: %v", h.filePath(), err)
		}
	}
	return h
}

// filePath returns the on-disk location of the persisted buffer.
func (h *MemHistory) filePath() string {
	return filepath.Join(h.dir, "mem_history.json")
}

// Observe records a memory sample for a container, but only when at least
// LongSampleInterval has elapsed since the last recorded sample for that name.
// The oldest sample is evicted once the buffer exceeds MaxLongHistoryPoints.
func (h *MemHistory) Observe(name string, mem, limit uint64, now time.Time) {
	if name == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if last, ok := h.lastSample[name]; ok && now.Sub(last) < LongSampleInterval {
		return
	}

	buf := append(h.data[name], memSample{T: now, Mem: mem, Limit: limit})
	if len(buf) > MaxLongHistoryPoints {
		buf = buf[len(buf)-MaxLongHistoryPoints:]
	}
	h.data[name] = buf
	h.lastSample[name] = now
}

// load hydrates the buffer from disk. It also seeds lastSample from the newest
// retained sample per container so the interval gate survives a restart.
func (h *MemHistory) load() error {
	raw, err := os.ReadFile(h.filePath())
	if err != nil {
		return err
	}
	var data map[string][]memSample
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = data
	if h.data == nil {
		h.data = make(map[string][]memSample)
	}
	h.lastSample = make(map[string]time.Time, len(h.data))
	for name, buf := range h.data {
		if n := len(buf); n > 0 {
			h.lastSample[name] = buf[n-1].T
		}
	}
	return nil
}

// Save atomically persists the buffer to DATA_DIR. It writes a temp file then
// renames it into place, mirroring the write conventions in auth/totp.go.
func (h *MemHistory) Save() error {
	if err := os.MkdirAll(h.dir, 0700); err != nil {
		return err
	}

	h.mu.RLock()
	raw, err := json.Marshal(h.data)
	h.mu.RUnlock()
	if err != nil {
		return err
	}

	tmp := h.filePath() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, h.filePath())
}

// Trend computes a summary of the retained memory history for a container.
// It returns nil when fewer than two samples exist. slope_bytes_per_hour is the
// ordinary least-squares slope of memory against time (guarded for zero span).
func (h *MemHistory) Trend(name string) *MemTrend {
	h.mu.RLock()
	buf := h.data[name]
	samples := make([]memSample, len(buf))
	copy(samples, buf)
	h.mu.RUnlock()

	if len(samples) < 2 {
		return nil
	}

	first := samples[0]
	last := samples[len(samples)-1]

	minBytes := samples[0].Mem
	maxBytes := samples[0].Mem
	for _, s := range samples {
		if s.Mem < minBytes {
			minBytes = s.Mem
		}
		if s.Mem > maxBytes {
			maxBytes = s.Mem
		}
	}

	// Ordinary least-squares slope of mem (y) vs seconds-since-first (x).
	base := first.T
	var sumX, sumY, sumXY, sumXX float64
	n := float64(len(samples))
	for _, s := range samples {
		x := s.T.Sub(base).Seconds()
		y := float64(s.Mem)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	var slopePerHour int64
	if denom := n*sumXX - sumX*sumX; denom != 0 {
		slopePerSec := (n*sumXY - sumX*sumY) / denom
		slopePerHour = int64(slopePerSec * 3600)
	}

	return &MemTrend{
		Window:            "24h",
		Samples:           len(samples),
		FirstBytes:        first.Mem,
		LastBytes:         last.Mem,
		MinBytes:          minBytes,
		MaxBytes:          maxBytes,
		SlopeBytesPerHour: slopePerHour,
		Points:            downsample(samples, 20),
	}
}

// downsample returns at most maxPoints evenly-spaced samples, always including
// the first and last. It keeps the trend readable for an LLM without dumping
// the full 1440-point buffer.
func downsample(samples []memSample, maxPoints int) []memSample {
	n := len(samples)
	if maxPoints < 2 || n <= maxPoints {
		out := make([]memSample, n)
		copy(out, samples)
		return out
	}
	out := make([]memSample, 0, maxPoints)
	// Distribute maxPoints indices evenly across [0, n-1].
	for i := 0; i < maxPoints; i++ {
		idx := i * (n - 1) / (maxPoints - 1)
		out = append(out, samples[idx])
	}
	return out
}
