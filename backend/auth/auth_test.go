package auth

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
)

func TestCreateAndValidateSession(t *testing.T) {
    sm := NewSessionManager(1 * time.Hour)
    sid, err := sm.CreateSession("admin")
    if err != nil {
        t.Fatalf("CreateSession failed: %v", err)
    }
    if sid == "" {
        t.Fatal("expected non-empty session ID")
    }

    username, err := sm.ValidateSession(sid)
    if err != nil {
        t.Fatalf("ValidateSession failed: %v", err)
    }
    if username != "admin" {
        t.Errorf("expected username 'admin', got %q", username)
    }
}

func TestValidateSession_NotFound(t *testing.T) {
    sm := NewSessionManager(1 * time.Hour)
    _, err := sm.ValidateSession("nonexistent")
    if err == nil {
        t.Error("expected error for nonexistent session")
    }
}

func TestValidateSession_Expired(t *testing.T) {
    sm := NewSessionManager(1 * time.Millisecond)
    sid, _ := sm.CreateSession("admin")
    time.Sleep(5 * time.Millisecond)
    _, err := sm.ValidateSession(sid)
    if err == nil {
        t.Error("expected error for expired session")
    }
}

func TestDestroySession(t *testing.T) {
    sm := NewSessionManager(1 * time.Hour)
    sid, _ := sm.CreateSession("admin")
    sm.DestroySession(sid)
    _, err := sm.ValidateSession(sid)
    if err == nil {
        t.Error("expected error after session destroy")
    }
}

func TestCleanExpired(t *testing.T) {
    sm := NewSessionManager(1 * time.Millisecond)
    sm.CreateSession("user1")
    sm.CreateSession("user2")
    time.Sleep(5 * time.Millisecond)
    sm.CleanExpired()

    // Both should be cleaned
    count := 0
    sm.sessions.Range(func(_, _ interface{}) bool {
        count++
        return true
    })
    if count != 0 {
        t.Errorf("expected 0 sessions after cleanup, got %d", count)
    }
}

func TestUsernameFromContext(t *testing.T) {
    ctx := context.WithValue(context.Background(), usernameContextKey, "testuser")
    username, ok := UsernameFromContext(ctx)
    if !ok || username != "testuser" {
        t.Errorf("expected 'testuser', got %q ok=%v", username, ok)
    }
}

func TestUsernameFromContext_Missing(t *testing.T) {
    _, ok := UsernameFromContext(context.Background())
    if ok {
        t.Error("expected ok=false for empty context")
    }
}

func TestAuthMiddleware_NoCookie(t *testing.T) {
    sm := NewSessionManager(1 * time.Hour)
    handler := sm.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Error("handler should not be called")
    }))

    req := httptest.NewRequest("GET", "/api/test", nil)
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", w.Code)
    }
}

func TestAuthMiddleware_InvalidSession(t *testing.T) {
    sm := NewSessionManager(1 * time.Hour)
    handler := sm.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Error("handler should not be called")
    }))

    req := httptest.NewRequest("GET", "/api/test", nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: "invalid"})
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", w.Code)
    }
}

func TestAuthMiddleware_ValidSession(t *testing.T) {
    sm := NewSessionManager(1 * time.Hour)
    sid, _ := sm.CreateSession("admin")

    var ctxUsername string
    handler := sm.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctxUsername, _ = UsernameFromContext(r.Context())
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest("GET", "/api/test", nil)
    req.AddCookie(&http.Cookie{Name: "session", Value: sid})
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }
    if ctxUsername != "admin" {
        t.Errorf("expected context username 'admin', got %q", ctxUsername)
    }
}

func TestSecurityHeadersMiddleware(t *testing.T) {
    handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest("GET", "/", nil)
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    headers := map[string]string{
        "X-Content-Type-Options": "nosniff",
        "X-Frame-Options":       "DENY",
        "X-XSS-Protection":      "1; mode=block",
        "Referrer-Policy":        "same-origin",
    }
    for key, expected := range headers {
        if got := w.Header().Get(key); got != expected {
            t.Errorf("expected %s=%q, got %q", key, expected, got)
        }
    }
}

func TestOriginCheckMiddleware_GET_NoCheck(t *testing.T) {
    handler := OriginCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest("GET", "/api/test", nil)
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("GET should pass without origin check, got %d", w.Code)
    }
}

func TestOriginCheckMiddleware_POST_ValidOrigin(t *testing.T) {
    handler := OriginCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest("POST", "/api/login", nil)
    req.Host = "localhost:8080"
    req.Header.Set("Origin", "http://localhost:8080")
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200 with valid origin, got %d", w.Code)
    }
}

func TestOriginCheckMiddleware_POST_BadOrigin(t *testing.T) {
    handler := OriginCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Error("handler should not be called")
    }))

    req := httptest.NewRequest("POST", "/api/login", nil)
    req.Host = "localhost:8080"
    req.Header.Set("Origin", "http://evil.com")
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusForbidden {
        t.Errorf("expected 403 with bad origin, got %d", w.Code)
    }
}

func TestOriginCheckMiddleware_POST_RefererFallback(t *testing.T) {
    handler := OriginCheckMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    req := httptest.NewRequest("POST", "/api/login", nil)
    req.Host = "localhost:8080"
    req.Header.Set("Referer", "http://localhost:8080/login")
    w := httptest.NewRecorder()
    handler.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("expected 200 with valid referer, got %d", w.Code)
    }
}
