package auth

import (
    "encoding/json"
    "net"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    "golang.org/x/crypto/bcrypt"
)

var failedAttempts sync.Map // map[string][]time.Time

type loginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type loginResponse struct {
    Ok       bool   `json:"ok"`
    Username string `json:"username,omitempty"`
    Error    string `json:"error,omitempty"`
}

// maxFailed and window are configurable via env vars
func getRateLimitConfig() (int, time.Duration) {
    max := 5
    window := 10 * time.Minute
    if v := os.Getenv("AUTH_MAX_FAILED"); v != "" {
        // ignore parse errors, keep defaults
    }
    if v := os.Getenv("AUTH_WINDOW_SECONDS"); v != "" {
        if secs, err := time.ParseDuration(v + "s"); err == nil {
            window = secs
        }
    }
    return max, window
}

func getIP(r *http.Request) string {
    // try X-Forwarded-For first
    if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
        parts := strings.Split(xf, ",")
        return strings.TrimSpace(parts[0])
    }
    host, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return host
}

func (s *SessionManager) LoginHandler(w http.ResponseWriter, r *http.Request) {
    var req loginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        _ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "invalid request"})
        return
    }

    ip := getIP(r)
    maxFailed, window := getRateLimitConfig()
    now := time.Now()

    // prune old attempts
    v, _ := failedAttempts.LoadOrStore(ip, []time.Time{})
    attempts := v.([]time.Time)
    var recent []time.Time
    for _, ts := range attempts {
        if now.Sub(ts) <= window {
            recent = append(recent, ts)
        }
    }
    if len(recent) >= maxFailed {
        w.WriteHeader(http.StatusTooManyRequests)
        _ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "too many failed attempts"})
        return
    }

    adminUser := os.Getenv("ADMIN_USER")
    adminHash := os.Getenv("ADMIN_PASSWORD_HASH")
    if adminUser == "" || adminHash == "" {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "server misconfigured"})
        return
    }

    if req.Username != adminUser {
        // record failed attempt
        recent = append(recent, now)
        failedAttempts.Store(ip, recent)
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "invalid credentials"})
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(adminHash), []byte(req.Password)); err != nil {
        recent = append(recent, now)
        failedAttempts.Store(ip, recent)
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "invalid credentials"})
        return
    }

    // success — clear failed attempts for ip
    failedAttempts.Delete(ip)

    sid, err := s.CreateSession(req.Username)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        _ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "failed to create session"})
        return
    }

    cookie := &http.Cookie{
        Name:     "session",
        Value:    sid,
        Path:     "/",
        HttpOnly: true,
        SameSite: http.SameSiteStrictMode,
        MaxAge:   int(s.ttl.Seconds()),
    }
    // Only set Secure flag when behind TLS (forwarded proto or direct TLS)
    if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
        cookie.Secure = true
    }
    http.SetCookie(w, cookie)

    _ = json.NewEncoder(w).Encode(loginResponse{Ok: true, Username: req.Username})
}

func (s *SessionManager) LogoutHandler(w http.ResponseWriter, r *http.Request) {
    c, err := r.Cookie("session")
    if err == nil && c.Value != "" {
        s.DestroySession(c.Value)
    }
    // clear cookie
    cookie := &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1}
    http.SetCookie(w, cookie)
    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *SessionManager) CheckHandler(w http.ResponseWriter, r *http.Request) {
    c, err := r.Cookie("session")
    if err != nil || c.Value == "" {
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated"})
        return
    }
    username, err := s.ValidateSession(c.Value)
    if err != nil {
        w.WriteHeader(http.StatusUnauthorized)
        _ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated"})
        return
    }
    _ = json.NewEncoder(w).Encode(map[string]string{"username": username})
}
