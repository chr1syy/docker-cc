package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
