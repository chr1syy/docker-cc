package auth

import (
	"crypto/rand"
	"encoding/hex"
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

// pendingTOTP stores temporary session tokens for users who passed password
// but still need to provide a TOTP code.
var pendingTOTP sync.Map // map[string]*pendingTOTPSession

type pendingTOTPSession struct {
	Username  string
	CreatedAt time.Time
}

// pendingSetup stores temporary TOTP secrets during the setup flow
// (between generating the QR and confirming the code).
var pendingSetup sync.Map // map[sessionID]string (the TOTP secret)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type totpVerifyRequest struct {
	Token string `json:"token"` // pending TOTP token (from phase 1)
	Code  string `json:"code"`  // 6-digit TOTP code
}

type loginResponse struct {
	Ok           bool   `json:"ok"`
	Username     string `json:"username,omitempty"`
	Error        string `json:"error,omitempty"`
	RequiresTOTP bool   `json:"requires_totp,omitempty"`
	TOTPToken    string `json:"totp_token,omitempty"`
}

type setupRequest struct {
	Code string `json:"code"`
}

type setupResponse struct {
	Ok     bool   `json:"ok"`
	URI    string `json:"uri,omitempty"`
	Secret string `json:"secret,omitempty"`
	Error  string `json:"error,omitempty"`
}

type statusResponse struct {
	Enabled bool `json:"enabled"`
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

// LoginHandler handles phase 1 of login: password check.
// If 2FA is enabled, returns a temporary token for phase 2.
func (s *SessionManager) LoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

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

	// Password correct — clear failed attempts
	failedAttempts.Delete(ip)

	// Check if 2FA is enabled
	if s.totp != nil && s.totp.IsEnabled() {
		// Issue a temporary pending-TOTP token
		token, err := generateToken()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "internal error"})
			return
		}
		pendingTOTP.Store(token, &pendingTOTPSession{
			Username:  req.Username,
			CreatedAt: time.Now(),
		})
		_ = json.NewEncoder(w).Encode(loginResponse{
			Ok:           false,
			RequiresTOTP: true,
			TOTPToken:    token,
		})
		return
	}

	// No 2FA — create session directly
	s.createSessionAndRespond(w, r, req.Username)
}

// TOTPVerifyHandler handles phase 2: validating the TOTP code after password success.
func (s *SessionManager) TOTPVerifyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req totpVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "invalid request"})
		return
	}

	if req.Token == "" || req.Code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "token and code are required"})
		return
	}

	// Look up pending session
	v, ok := pendingTOTP.Load(req.Token)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "invalid or expired token"})
		return
	}
	pending := v.(*pendingTOTPSession)

	// Expire tokens after 5 minutes
	if time.Since(pending.CreatedAt) > 5*time.Minute {
		pendingTOTP.Delete(req.Token)
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: "token expired, please login again"})
		return
	}

	// Validate TOTP code
	if err := s.totp.Validate(req.Code); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(loginResponse{Ok: false, Error: err.Error()})
		return
	}

	// TOTP valid — clean up and create session
	pendingTOTP.Delete(req.Token)
	s.createSessionAndRespond(w, r, pending.Username)
}

// TwoFAStatusHandler returns whether 2FA is enabled.
func (s *SessionManager) TwoFAStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	enabled := s.totp != nil && s.totp.IsEnabled()
	_ = json.NewEncoder(w).Encode(statusResponse{Enabled: enabled})
}

// TwoFASetupHandler generates a new TOTP secret and returns the otpauth URI.
func (s *SessionManager) TwoFASetupHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.totp == nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "2FA not available"})
		return
	}

	if s.totp.IsEnabled() {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "2FA is already enabled. Disable it first."})
		return
	}

	adminUser := os.Getenv("ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}

	key, err := s.totp.GenerateSecret("Docker CC", adminUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "failed to generate secret"})
		return
	}

	// Store the secret temporarily keyed by the session cookie
	c, _ := r.Cookie("session")
	if c != nil {
		pendingSetup.Store(c.Value, key.Secret())
	}

	_ = json.NewEncoder(w).Encode(setupResponse{
		Ok:     true,
		URI:    key.URL(),
		Secret: key.Secret(),
	})
}

// TwoFAConfirmHandler validates the TOTP code and enables 2FA.
func (s *SessionManager) TwoFAConfirmHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "invalid request"})
		return
	}

	// Get the pending secret
	c, _ := r.Cookie("session")
	if c == nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "not authenticated"})
		return
	}

	v, ok := pendingSetup.Load(c.Value)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "no pending setup. Call setup first."})
		return
	}
	secret := v.(string)

	if err := s.totp.ConfirmSetup(secret, req.Code); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: err.Error()})
		return
	}

	pendingSetup.Delete(c.Value)
	_ = json.NewEncoder(w).Encode(setupResponse{Ok: true})
}

// TwoFADisableHandler disables 2FA after validating a TOTP code.
func (s *SessionManager) TwoFADisableHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "invalid request"})
		return
	}

	if s.totp == nil || !s.totp.IsEnabled() {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: "2FA is not enabled"})
		return
	}

	if err := s.totp.Disable(req.Code); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(setupResponse{Ok: false, Error: err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(setupResponse{Ok: true})
}

func (s *SessionManager) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("session")
	if err == nil && c.Value != "" {
		s.DestroySession(c.Value)
	}
	cookie := &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1}
	http.SetCookie(w, cookie)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *SessionManager) CheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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
	twoFAEnabled := s.totp != nil && s.totp.IsEnabled()
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"username":     username,
		"totp_enabled": twoFAEnabled,
	})
}

// createSessionAndRespond creates a session cookie and writes a success response.
func (s *SessionManager) createSessionAndRespond(w http.ResponseWriter, r *http.Request, username string) {
	sid, err := s.CreateSession(username)
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
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
	_ = json.NewEncoder(w).Encode(loginResponse{Ok: true, Username: username})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
