package handlers

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestStatus_NilClient_DockerDisconnected(t *testing.T) {
    h := NewStatusHandler(nil, "test", NewLogHandler(nil))
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/status", nil)

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
    if resp["healthy"] != false {
        t.Errorf("expected healthy=false, got %v", resp["healthy"])
    }

    counts, ok := resp["containers"].(map[string]interface{})
    if !ok {
        t.Fatalf("expected containers to be an object, got %T", resp["containers"])
    }
    if total, _ := counts["total"].(float64); total != 0 {
        t.Errorf("expected containers.total=0, got %v", counts["total"])
    }
}

func TestStatus_InvalidSince_400(t *testing.T) {
    h := NewStatusHandler(nil, "test", NewLogHandler(nil))
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/status?since=not-a-duration", nil)

    h.Get(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 for invalid since, got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestStatus_HealthyResponseShape(t *testing.T) {
    h := NewStatusHandler(nil, "test-version", NewLogHandler(nil))
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/status", nil)

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

    requiredKeys := []string{"healthy", "checked_at", "window", "docker", "version", "containers", "issues"}
    for _, k := range requiredKeys {
        if _, ok := resp[k]; !ok {
            t.Errorf("response is missing required key %q", k)
        }
    }

    if v, _ := resp["version"].(string); v != "test-version" {
        t.Errorf("expected version=test-version, got %v", resp["version"])
    }
    if v, _ := resp["window"].(string); v == "" {
        t.Errorf("expected non-empty window, got %v", resp["window"])
    }
    if v, _ := resp["checked_at"].(string); v == "" {
        t.Errorf("expected non-empty checked_at, got %v", resp["checked_at"])
    }
}
