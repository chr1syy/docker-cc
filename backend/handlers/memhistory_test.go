package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestObserveIntervalGate verifies that Observe respects the 60s spacing gate:
// two calls 10s apart yield one sample; a third at +90s yields a second.
func TestObserveIntervalGate(t *testing.T) {
	h := NewMemHistory(t.TempDir())
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	h.Observe("eplan-backend", 100, 768, base)
	h.Observe("eplan-backend", 110, 768, base.Add(10*time.Second))

	if got := len(h.data["eplan-backend"]); got != 1 {
		t.Fatalf("after two calls 10s apart, expected 1 sample, got %d", got)
	}

	h.Observe("eplan-backend", 120, 768, base.Add(90*time.Second))
	if got := len(h.data["eplan-backend"]); got != 2 {
		t.Fatalf("after a third call at +90s, expected 2 samples, got %d", got)
	}
}

// TestObserveEmptyName confirms an empty container name is ignored.
func TestObserveEmptyName(t *testing.T) {
	h := NewMemHistory(t.TempDir())
	h.Observe("", 100, 768, time.Now())
	if len(h.data) != 0 {
		t.Fatalf("empty name should be ignored, got %d entries", len(h.data))
	}
}

// TestObserveRingEviction verifies the buffer retains exactly
// MaxLongHistoryPoints, dropping the oldest samples.
func TestObserveRingEviction(t *testing.T) {
	h := NewMemHistory(t.TempDir())
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	total := MaxLongHistoryPoints + 5
	for i := 0; i < total; i++ {
		// Advance by the full interval each step so every call records.
		h.Observe("app", uint64(i), 768, base.Add(time.Duration(i)*LongSampleInterval))
	}

	buf := h.data["app"]
	if len(buf) != MaxLongHistoryPoints {
		t.Fatalf("expected %d retained, got %d", MaxLongHistoryPoints, len(buf))
	}
	// Oldest 5 dropped, so the first retained sample's Mem is 5.
	if buf[0].Mem != 5 {
		t.Errorf("expected oldest retained Mem=5, got %d", buf[0].Mem)
	}
	if buf[len(buf)-1].Mem != uint64(total-1) {
		t.Errorf("expected newest Mem=%d, got %d", total-1, buf[len(buf)-1].Mem)
	}
}

// TestTrendTooFewSamples verifies Trend returns nil for fewer than 2 samples.
func TestTrendTooFewSamples(t *testing.T) {
	h := NewMemHistory(t.TempDir())
	if tr := h.Trend("missing"); tr != nil {
		t.Errorf("expected nil trend for unknown container, got %+v", tr)
	}
	h.Observe("app", 100, 768, time.Now())
	if tr := h.Trend("app"); tr != nil {
		t.Errorf("expected nil trend for single sample, got %+v", tr)
	}
}

// TestTrendIncreasing verifies a strictly increasing series yields a positive
// slope, a bounded downsampled point set, and first_bytes < last_bytes.
func TestTrendIncreasing(t *testing.T) {
	h := NewMemHistory(t.TempDir())
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// 100 minute-spaced samples climbing by 1 MiB each minute.
	const n = 100
	const step = 1024 * 1024
	for i := 0; i < n; i++ {
		mem := uint64(step * (i + 1))
		h.Observe("leaky", mem, 768*1024*1024, base.Add(time.Duration(i)*LongSampleInterval))
	}

	tr := h.Trend("leaky")
	if tr == nil {
		t.Fatal("expected a trend, got nil")
	}
	if tr.Samples != n {
		t.Errorf("expected %d samples, got %d", n, tr.Samples)
	}
	if tr.SlopeBytesPerHour <= 0 {
		t.Errorf("expected positive slope for increasing series, got %d", tr.SlopeBytesPerHour)
	}
	if tr.FirstBytes >= tr.LastBytes {
		t.Errorf("expected first_bytes < last_bytes, got %d >= %d", tr.FirstBytes, tr.LastBytes)
	}
	if tr.MinBytes != step || tr.MaxBytes != uint64(step*n) {
		t.Errorf("expected min=%d max=%d, got min=%d max=%d", step, step*n, tr.MinBytes, tr.MaxBytes)
	}
	if len(tr.Points) > 20 {
		t.Errorf("expected at most 20 downsampled points, got %d", len(tr.Points))
	}
	if len(tr.Points) < 2 {
		t.Errorf("expected at least 2 points, got %d", len(tr.Points))
	}
	// Downsampling must preserve endpoints.
	if tr.Points[0].Mem != tr.FirstBytes || tr.Points[len(tr.Points)-1].Mem != tr.LastBytes {
		t.Errorf("downsample must retain endpoints: got first=%d last=%d", tr.Points[0].Mem, tr.Points[len(tr.Points)-1].Mem)
	}
}

// TestPersistenceRoundTrip verifies Save followed by a fresh NewMemHistory on
// the same dir reloads the same samples and re-seeds the interval gate.
func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	h := NewMemHistory(dir)
	const n = 5
	for i := 0; i < n; i++ {
		h.Observe("app", uint64(100+i), 768, base.Add(time.Duration(i)*LongSampleInterval))
	}
	if err := h.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded := NewMemHistory(dir)
	buf := reloaded.data["app"]
	if len(buf) != n {
		t.Fatalf("expected %d reloaded samples, got %d", n, len(buf))
	}
	last := buf[len(buf)-1]
	if last.Mem != uint64(100+n-1) {
		t.Errorf("expected last reloaded Mem=%d, got %d", 100+n-1, last.Mem)
	}

	// The interval gate must be re-seeded from the newest reloaded sample: an
	// Observe within LongSampleInterval of it is a no-op.
	reloaded.Observe("app", 999, 768, last.T.Add(10*time.Second))
	if got := len(reloaded.data["app"]); got != n {
		t.Errorf("interval gate should survive reload, expected %d samples, got %d", n, got)
	}
	// A sample past the interval is recorded.
	reloaded.Observe("app", 999, 768, last.T.Add(LongSampleInterval))
	if got := len(reloaded.data["app"]); got != n+1 {
		t.Errorf("expected %d samples after interval elapsed, got %d", n+1, got)
	}
}

// TestSaveAtomicNoTempLeft confirms Save leaves no lingering temp file.
func TestSaveAtomicNoTempLeft(t *testing.T) {
	dir := t.TempDir()
	h := NewMemHistory(dir)
	h.Observe("app", 100, 768, time.Now())
	h.Observe("app", 200, 768, time.Now().Add(2*LongSampleInterval))
	if err := h.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mem_history.json.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected no lingering temp file, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mem_history.json")); err != nil {
		t.Errorf("expected persisted file to exist, stat err=%v", err)
	}
}

// TestCorruptFileTolerated verifies a garbage on-disk file does not panic and
// yields an empty (but usable) buffer.
func TestCorruptFileTolerated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mem_history.json"), []byte("{not valid json"), 0600); err != nil {
		t.Fatalf("setup write failed: %v", err)
	}

	h := NewMemHistory(dir) // must not panic
	if len(h.data) != 0 {
		t.Errorf("expected empty buffer after corrupt load, got %d entries", len(h.data))
	}
	// Buffer must remain usable.
	h.Observe("app", 100, 768, time.Now())
	if len(h.data["app"]) != 1 {
		t.Errorf("expected buffer usable after corrupt load, got %d samples", len(h.data["app"]))
	}
}
