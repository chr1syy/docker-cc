package handlers

import (
    "encoding/json"
    "net/http/httptest"
    "testing"
)

func TestWriteJSON(t *testing.T) {
    w := httptest.NewRecorder()
    data := map[string]string{"hello": "world"}
    writeJSON(w, 200, data)

    if w.Code != 200 {
        t.Errorf("expected status 200, got %d", w.Code)
    }
    if ct := w.Header().Get("Content-Type"); ct != "application/json" {
        t.Errorf("expected application/json, got %q", ct)
    }

    var body map[string]string
    if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
        t.Fatalf("invalid JSON: %v", err)
    }
    if body["hello"] != "world" {
        t.Errorf("expected hello=world, got %q", body["hello"])
    }
}

func TestWriteJSON_CustomStatus(t *testing.T) {
    w := httptest.NewRecorder()
    writeJSON(w, 201, map[string]bool{"ok": true})
    if w.Code != 201 {
        t.Errorf("expected status 201, got %d", w.Code)
    }
}

func TestWriteError(t *testing.T) {
    w := httptest.NewRecorder()
    writeError(w, 404, "not found")

    if w.Code != 404 {
        t.Errorf("expected status 404, got %d", w.Code)
    }

    var body map[string]string
    if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
        t.Fatalf("invalid JSON: %v", err)
    }
    if body["error"] != "not found" {
        t.Errorf("expected error='not found', got %q", body["error"])
    }
}
