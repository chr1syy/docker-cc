package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/docker"
)

// newTestSnapshotHandler builds a SnapshotHandler wired against a nil Docker
// client but with real (empty) LogHandler/StatsHandler/MemHistory instances,
// mirroring the disconnected server state. The StatsHandler's background
// collector exits immediately on a nil client (see collect()), so no goroutine
// leaks or panics from spinning it up in a test.
func newTestSnapshotHandler(t *testing.T, version string) *SnapshotHandler {
	t.Helper()
	mh := NewMemHistory(t.TempDir())
	sh := NewStatsHandler(nil, mh)
	return NewSnapshotHandler(nil, version, NewLogHandler(nil), sh, mh)
}

func TestSnapshot_NilClient_Disconnected(t *testing.T) {
	h := newTestSnapshotHandler(t, "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/snapshot", nil)

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["docker"] != "disconnected" {
		t.Errorf("expected docker=disconnected, got %v", resp["docker"])
	}

	counts, ok := resp["counts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected counts to be an object, got %T", resp["counts"])
	}
	if total, _ := counts["total"].(float64); total != 0 {
		t.Errorf("expected counts.total=0, got %v", counts["total"])
	}

	// host must be present (either a populated object or explicitly null) and
	// must not have panicked. json.Unmarshal into map[string]interface{} keeps
	// an explicit null as a present key with a nil value.
	if _, present := resp["host"]; !present {
		t.Errorf("expected host key to be present in the response")
	}
}

func TestSnapshot_InvalidWindow_400(t *testing.T) {
	h := newTestSnapshotHandler(t, "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/snapshot?window=not-a-duration", nil)

	h.Get(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid window, got %d (body=%q)", rr.Code, rr.Body.String())
	}
}

func TestSnapshot_ResponseShape(t *testing.T) {
	h := newTestSnapshotHandler(t, "test-version")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/snapshot", nil)

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rr.Code, rr.Body.String())
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	requiredKeys := []string{"checked_at", "docker", "version", "window", "host", "counts", "containers"}
	for _, k := range requiredKeys {
		if _, ok := resp[k]; !ok {
			t.Errorf("response is missing required key %q", k)
		}
	}

	if v, _ := resp["version"].(string); v != "test-version" {
		t.Errorf("expected version to round-trip as test-version, got %v", resp["version"])
	}
	if v, _ := resp["window"].(string); v == "" {
		t.Errorf("expected non-empty window, got %v", resp["window"])
	}
	if v, _ := resp["checked_at"].(string); v == "" {
		t.Errorf("expected non-empty checked_at, got %v", resp["checked_at"])
	}
}

// TestMapDigestsToErrors_ScanFailureNotClean is the regression test for the
// Codex-major finding: a failed log scan must NOT be reported as a clean/
// healthy container. It must instead surface scan_ok:false + scan_error and
// count toward scan_failures, so downstream monitoring never acts on a
// falsely-healthy status.
func TestMapDigestsToErrors_ScanFailureNotClean(t *testing.T) {
	digests := []containerDigest{
		{Container: "healthy-app", ErrorCount: 0, Samples: []docker.DigestLine{}},
		{Container: "noisy-app", ErrorCount: 3, LastErrorAt: "2026-07-02T10:00:00Z",
			Samples: []docker.DigestLine{{Text: "boom"}}},
		{Container: "unscannable", ScanError: "failed to fetch logs: connection refused"},
	}

	byName, scanFailures := mapDigestsToErrors(digests)

	if scanFailures != 1 {
		t.Fatalf("expected exactly 1 scan failure, got %d", scanFailures)
	}

	// Genuinely clean: scan ran, found nothing → scan_ok true, no error.
	clean := byName["healthy-app"]
	if !clean.ScanOK || clean.ScanError != "" || clean.Count != 0 {
		t.Errorf("healthy-app: expected clean scanned container, got %+v", clean)
	}

	// Real errors: scan succeeded, counts surfaced, still scan_ok.
	noisy := byName["noisy-app"]
	if !noisy.ScanOK || noisy.Count != 3 {
		t.Errorf("noisy-app: expected ScanOK=true and Count=3, got %+v", noisy)
	}

	// Failed scan: the core fix. count stays 0 (unknown, not fabricated) but
	// the container must read as NOT clean: scan_ok=false with the reason.
	failed := byName["unscannable"]
	if failed.ScanOK {
		t.Errorf("unscannable: a failed scan must not read as clean (want scan_ok=false)")
	}
	if failed.ScanError == "" {
		t.Errorf("unscannable: expected the scan error to be surfaced")
	}
	if failed.Count != 0 {
		t.Errorf("unscannable: expected Count=0, got %d", failed.Count)
	}

	// And it must serialize as scan_ok:false for the LLM/automation consumer.
	raw, err := json.Marshal(failed)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(raw), `"scan_ok":false`) {
		t.Errorf("expected scan_ok:false in JSON, got %s", raw)
	}
}

// TestSnapshot_MemTrendNullWhenSparse verifies that a container with only a
// single long-horizon sample serializes mem_trend as JSON null (a nil *MemTrend
// pointer), not a zero-value struct. This guards the leak-signal field's
// "not enough data yet" contract for the LLM consumer.
func TestSnapshot_MemTrendNullWhenSparse(t *testing.T) {
	mh := NewMemHistory(t.TempDir())
	mh.Observe("solo", 100, 768, time.Now())

	if tr := mh.Trend("solo"); tr != nil {
		t.Fatalf("expected nil Trend for a single sample, got %+v", tr)
	}

	// The snapshot embeds the trend as a *MemTrend, which must serialize to
	// null rather than an empty object when Trend returns nil.
	sc := snapshotContainer{
		Name:         "solo",
		MemTrend:     mh.Trend("solo"),
		RecentErrors: snapshotErrors{Samples: []string{}},
	}
	raw, err := json.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(raw), `"mem_trend":null`) {
		t.Errorf("expected mem_trend to serialize as null, got %s", raw)
	}
}
