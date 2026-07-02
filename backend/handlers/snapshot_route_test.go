package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	authpkg "backend/auth"
)

// buildSnapshotTestRouter mirrors the protected route wiring from main.go: the
// snapshot endpoint sits behind the security-headers and origin-check
// middleware inside an AuthMiddleware-protected group. It returns the router
// plus the session manager so a test can mint a valid session cookie. This
// exercises the actual auth gate rather than calling the handler directly.
func buildSnapshotTestRouter(t *testing.T) (*chi.Mux, *authpkg.SessionManager) {
	t.Helper()
	sm := authpkg.NewSessionManager(time.Hour)
	snap := newTestSnapshotHandler(t, "test")

	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Use(authpkg.SecurityHeadersMiddleware)
		r.Use(authpkg.OriginCheckMiddleware)
		r.Group(func(r chi.Router) {
			r.Use(sm.AuthMiddleware)
			r.Get("/agent/snapshot", snap.Get)
		})
	})
	return r, sm
}

// TestSnapshotRoute_Unauthenticated401 verifies the endpoint is gated: a
// request with no session cookie is rejected by AuthMiddleware with 401 and
// never reaches the handler.
func TestSnapshotRoute_Unauthenticated401(t *testing.T) {
	r, _ := buildSnapshotTestRouter(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/snapshot", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a session, got %d (body=%q)", rr.Code, rr.Body.String())
	}
}

// TestSnapshotRoute_ValidSession200 verifies that a request carrying a valid
// session cookie passes AuthMiddleware and gets a 200 JSON snapshot.
func TestSnapshotRoute_ValidSession200(t *testing.T) {
	r, sm := buildSnapshotTestRouter(t)

	id, err := sm.CreateSession("admin")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/snapshot", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: id})
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with a valid session, got %d (body=%q)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}
}

// TestSnapshotRoute_InvalidSession401 verifies a cookie with an unknown
// session id is rejected — the gate validates the session, not just its
// presence.
func TestSnapshotRoute_InvalidSession401(t *testing.T) {
	r, _ := buildSnapshotTestRouter(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/snapshot", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "deadbeef-not-a-real-session"})
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an unknown session id, got %d (body=%q)", rr.Code, rr.Body.String())
	}
}
