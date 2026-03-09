package auth

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "net/http"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"
)

type sessionData struct {
    Username     string
    CreatedAt    time.Time
    LastActivity time.Time
}

type SessionManager struct {
    sessions sync.Map // map[string]sessionData
    ttl      time.Duration
    totp     *TOTPManager
}

func NewSessionManager(ttl time.Duration) *SessionManager {
    // If ttl is zero, attempt to read SESSION_TTL from environment (supports time.ParseDuration or integer seconds).
    if ttl == 0 {
        if v := os.Getenv("SESSION_TTL"); v != "" {
            if d, err := time.ParseDuration(v); err == nil {
                ttl = d
            } else if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
                ttl = time.Duration(secs) * time.Second
            }
        }
        if ttl == 0 {
            ttl = 24 * time.Hour
        }
    }

    sm := &SessionManager{ttl: ttl}
    go sm.cleanupLoop()
    return sm
}

// SetTOTP attaches a TOTPManager to enable 2FA support.
func (s *SessionManager) SetTOTP(tm *TOTPManager) {
    s.totp = tm
}

func (s *SessionManager) CreateSession(username string) (string, error) {
    b := make([]byte, 32)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    id := hex.EncodeToString(b)
    s.sessions.Store(id, sessionData{Username: username, CreatedAt: time.Now(), LastActivity: time.Now()})
    return id, nil
}

func (s *SessionManager) ValidateSession(id string) (string, error) {
    v, ok := s.sessions.Load(id)
    if !ok {
        return "", errors.New("session not found")
    }
    sd := v.(sessionData)
    if time.Since(sd.LastActivity) > s.ttl {
        s.sessions.Delete(id)
        return "", errors.New("session expired")
    }
    sd.LastActivity = time.Now()
    s.sessions.Store(id, sd)
    return sd.Username, nil
}

func (s *SessionManager) DestroySession(id string) {
    s.sessions.Delete(id)
}

func (s *SessionManager) cleanupLoop() {
    ticker := time.NewTicker(10 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        s.CleanExpired()
    }
}

func (s *SessionManager) CleanExpired() {
    s.sessions.Range(func(key, value interface{}) bool {
        sd := value.(sessionData)
        if time.Since(sd.LastActivity) > s.ttl {
            s.sessions.Delete(key)
        }
        return true
    })
}

// contextKey is an unexported type for keys defined in this package.
type contextKey string

const usernameContextKey contextKey = "auth_username"

// AuthMiddleware enforces session-based authentication. It expects a cookie named
// "session" containing a session ID. On success it stores the username in the
// request context under the package key and calls the next handler. On failure
// it responds with 401.
func (s *SessionManager) AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        c, err := r.Cookie("session")
        if err != nil || c.Value == "" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        username, err := s.ValidateSession(c.Value)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        // Attach username to context and continue
        ctx := context.WithValue(r.Context(), usernameContextKey, username)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// SecurityHeadersMiddleware applies strict security-related headers to every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Referrer-Policy", "same-origin")
        next.ServeHTTP(w, r)
    })
}

// OriginCheckMiddleware enforces that state-changing requests (POST, PUT, DELETE)
// include an Origin or Referer matching the expected host. It respects
// X-Forwarded-Host when present.
func OriginCheckMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        method := r.Method
        if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete || method == http.MethodPatch {
            // determine expected host
            expectedHost := r.Host
            if xf := r.Header.Get("X-Forwarded-Host"); xf != "" {
                expectedHost = strings.Split(xf, ",")[0]
            }

            origin := r.Header.Get("Origin")
            ref := r.Header.Get("Referer")
            ok := false
            if origin != "" {
                // Origin contains scheme://host
                if strings.Contains(origin, expectedHost) {
                    ok = true
                }
            } else if ref != "" {
                if strings.Contains(ref, expectedHost) {
                    ok = true
                }
            }
            if !ok {
                http.Error(w, "forbidden - bad origin", http.StatusForbidden)
                return
            }
        }
        next.ServeHTTP(w, r)
    })
}

// UsernameFromContext extracts the authenticated username from the request context.
func UsernameFromContext(ctx context.Context) (string, bool) {
    v := ctx.Value(usernameContextKey)
    if v == nil {
        return "", false
    }
    u, ok := v.(string)
    return u, ok
}
