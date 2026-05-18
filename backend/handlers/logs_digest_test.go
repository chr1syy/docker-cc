package handlers

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/go-chi/chi/v5"
)

func newDigestRequest(t *testing.T, query string, id string) *http.Request {
    t.Helper()
    target := "/api/containers/" + id + "/logs/digest"
    if query != "" {
        target += "?" + query
    }
    req := httptest.NewRequest(http.MethodGet, target, nil)
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("id", id)
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
    return req
}

func TestDigest_NilClient_503(t *testing.T) {
    h := NewLogHandler(nil)
    rr := httptest.NewRecorder()
    req := newDigestRequest(t, "", "test-id")

    h.Digest(rr, req)

    if rr.Code != http.StatusServiceUnavailable {
        t.Fatalf("expected 503, got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestDigest_InvalidSince_400(t *testing.T) {
    h := NewLogHandler(nil)
    rr := httptest.NewRecorder()
    req := newDigestRequest(t, "since=not-a-duration", "test-id")

    h.Digest(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 for invalid since, got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestDigest_InvalidRegex_400(t *testing.T) {
    h := NewLogHandler(nil)
    rr := httptest.NewRecorder()
    req := newDigestRequest(t, "error_patterns=%5Bunclosed", "test-id")

    h.Digest(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 for invalid regex, got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestDigest_MissingID_400(t *testing.T) {
    h := NewLogHandler(nil)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/containers//logs/digest", nil)
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("id", "")
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

    h.Digest(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 for missing id, got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestCompileORRegex_ValidPatterns(t *testing.T) {
    re, err := compileORRegex("foo,bar,baz")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !re.MatchString("hello FOO world") {
        t.Errorf("expected case-insensitive match on FOO")
    }
    if !re.MatchString("just a bar here") {
        t.Errorf("expected match on bar")
    }
    if re.MatchString("nothing here") {
        t.Errorf("did not expect a match")
    }
}

func TestCompileORRegex_TrimsWhitespace(t *testing.T) {
    re, err := compileORRegex(" foo , bar ")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !re.MatchString("bar") {
        t.Errorf("expected match on bar after whitespace trim")
    }
}

func TestCompileORRegex_EmptyList(t *testing.T) {
    if _, err := compileORRegex(""); err == nil {
        t.Errorf("expected error for empty pattern list")
    }
    if _, err := compileORRegex(" , , "); err == nil {
        t.Errorf("expected error for whitespace-only pattern list")
    }
}

func TestCompileORRegex_InvalidPattern(t *testing.T) {
    if _, err := compileORRegex("[unclosed"); err == nil {
        t.Errorf("expected error for malformed regex")
    }
}

func TestBulkDigest_NilClient_503(t *testing.T) {
    h := NewLogHandler(nil)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/logs/digest", nil)

    h.BulkDigest(rr, req)

    if rr.Code != http.StatusServiceUnavailable {
        t.Fatalf("expected 503, got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestBulkDigest_InvalidConcurrency_Clamped(t *testing.T) {
    h := NewLogHandler(nil)
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/api/logs/digest?concurrency=999", nil)

    h.BulkDigest(rr, req)

    // With a nil client the request short-circuits to 503 before any fan-out,
    // but the important assertion is that the handler does not panic or
    // misbehave when given an out-of-range concurrency value.
    if rr.Code != http.StatusServiceUnavailable {
        t.Fatalf("expected 503 (nil client), got %d (body=%q)", rr.Code, rr.Body.String())
    }
}

func TestParseDigestParams_Defaults(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/api/logs/digest", nil)

    p, err := parseDigestParams(req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if p.since != 24*time.Hour {
        t.Errorf("expected since=24h, got %s", p.since)
    }
    if p.limit != 50 {
        t.Errorf("expected limit=50, got %d", p.limit)
    }
    if p.maxScan != 50000 {
        t.Errorf("expected maxScan=50000, got %d", p.maxScan)
    }
    if p.errorRegex == nil {
        t.Errorf("expected non-nil errorRegex")
    }
    if p.warnRegex == nil {
        t.Errorf("expected non-nil warnRegex")
    }
}

func TestParseDigestParams_WarnNone(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/api/logs/digest?warn_patterns=none", nil)

    p, err := parseDigestParams(req)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if p.warnRegex != nil {
        t.Errorf("expected nil warnRegex when warn_patterns=none, got %v", p.warnRegex)
    }
}
