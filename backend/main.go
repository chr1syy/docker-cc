package main

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    _ "github.com/docker/docker/client"
    _ "github.com/gorilla/websocket"
    "golang.org/x/crypto/bcrypt"

    "backend/docker"
    "backend/handlers"
    authpkg "backend/auth"
)

// Version is set at build time via -ldflags "-X main.Version=..."
var Version = "dev"

func main() {
    r := chi.NewRouter()
    // request id, real ip and default logger (text). We also add a small
    // structured logger middleware below for JSON-like entries.
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(StructuredLogger)

    // Respect STATIC_DIR env var for static files (default ./static)
    staticDir := os.Getenv("STATIC_DIR")
    if staticDir == "" {
        staticDir = "./static"
    }

    // Initialize Docker client (if available) and register API routes
    dclient, err := docker.New()
    if err != nil {
        log.Printf("warning: docker client unavailable: %v", err)
    } else {
        defer dclient.Close()
    }
    ch := handlers.NewContainerHandler(dclient)
    // Log handlers
    lh := handlers.NewLogHandler(dclient)
    // Stats routes
    sh := handlers.NewStatsHandler(dclient)

    // If ADMIN_PASSWORD is set (plaintext), hash it and set ADMIN_PASSWORD_HASH
    // so the login handler can use it. This avoids bcrypt $ escaping issues in Docker Compose.
    if plainPw := os.Getenv("ADMIN_PASSWORD"); plainPw != "" {
        hash, err := bcrypt.GenerateFromPassword([]byte(plainPw), bcrypt.DefaultCost)
        if err != nil {
            log.Fatalf("failed to hash ADMIN_PASSWORD: %v", err)
        }
        os.Setenv("ADMIN_PASSWORD_HASH", string(hash))
        log.Println("ADMIN_PASSWORD hashed and set as ADMIN_PASSWORD_HASH")
    }

    // Authentication setup
    // SESSION_TTL can be set as a duration string (eg. "24h") or seconds
    sm := authpkg.NewSessionManager(0)

    // Initialize 2FA (TOTP) support
    dataDir := os.Getenv("DATA_DIR")
    if dataDir == "" {
        dataDir = "./data"
    }
    totpMgr, totpErr := authpkg.NewTOTPManager(dataDir)
    if totpErr != nil {
        log.Printf("warning: 2FA unavailable: %v", totpErr)
    } else {
        sm.SetTOTP(totpMgr)
        log.Println("2FA (TOTP) support enabled")
    }

    // Mount all /api routes within a single group so we can apply origin checks
    // and security headers to every API endpoint (including login/logout).
    r.Route("/api", func(r chi.Router) {
        // Limit request body sizes and apply security headers for all API
        // endpoints.
        r.Use(MaxBodySizeMiddleware(1 << 20)) // 1MB
        r.Use(authpkg.SecurityHeadersMiddleware)
        r.Use(authpkg.OriginCheckMiddleware)

        r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            // Check Docker connectivity
            dockerState := "disconnected"
            if dclient != nil {
                ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
                defer cancel()
                if err := dclient.Ping(ctx); err == nil {
                    dockerState = "connected"
                }
            }
            _ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "docker": dockerState, "version": Version})
        })

        r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            _ = json.NewEncoder(w).Encode(map[string]string{"version": Version})
        })

        // public auth endpoints
        r.Post("/login", sm.LoginHandler)
        r.Post("/auth/totp/verify", sm.TOTPVerifyHandler)
        r.Post("/logout", sm.LogoutHandler)
        r.Get("/auth/check", sm.CheckHandler)

        // Protected API endpoints
            r.Group(func(r chi.Router) {
                r.Use(sm.AuthMiddleware)
                r.Get("/containers", ch.List)
                r.Get("/containers/{id}", ch.Inspect)
                r.Get("/containers/{id}/logs", lh.Get)
                r.Get("/containers/{id}/logs/stream", lh.WS)
                r.Get("/stats/stream", sh.WS)
                r.Get("/containers/{id}/stats", sh.OneShot)
                // Container action routes (require explicit allow)
                r.With(handlers.RequireActions).Post("/containers/{id}/start", ch.Start)
                r.With(handlers.RequireActions).Post("/containers/{id}/stop", ch.Stop)
                r.With(handlers.RequireActions).Post("/containers/{id}/restart", ch.Restart)
                r.With(handlers.RequireActions).Delete("/containers/{id}", ch.Remove)

                // 2FA management (requires active session)
                r.Get("/auth/2fa/status", sm.TwoFAStatusHandler)
                r.Post("/auth/2fa/setup", sm.TwoFASetupHandler)
                r.Post("/auth/2fa/confirm", sm.TwoFAConfirmHandler)
                r.Post("/auth/2fa/disable", sm.TwoFADisableHandler)
            })
    })

    // Graceful shutdown
    srv := &http.Server{Addr: ":8080", Handler: r}

    // Serve static files (SPA fallback handled by frontend in production build)
    fs := http.FileServer(http.Dir(staticDir))
    r.Handle("/", fs)
    r.Handle("/*", fs)

    // start server with graceful shutdown on SIGINT/SIGTERM
    go func() {
        log.Println("Starting server on :8080")
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("server failed: %v", err)
        }
    }()

    // wait for signal
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
    // handle shutdown
    <-sig
    log.Println("Shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := srv.Shutdown(ctx); err != nil {
        log.Fatalf("shutdown failed: %v", err)
    }
    log.Println("server stopped")
}

// StructuredLogger is a lightweight middleware that logs method, path,
// status and duration in a simple JSON-ish line so it's easier to parse by
// log collectors.
func StructuredLogger(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        rw := &responseWriter{ResponseWriter: w, status: 200}
        next.ServeHTTP(rw, r)
        dur := time.Since(start)
        log.Printf("{\"method\":\"%s\",\"path\":\"%s\",\"status\":%d,\"duration_ms\":%d}", r.Method, r.URL.Path, rw.status, dur.Milliseconds())
    })
}

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.status = code
    rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker so WebSocket upgrades work through the
// structured logger middleware.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
    if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
        return hj.Hijack()
    }
    return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// Flush implements http.Flusher for streaming responses.
func (rw *responseWriter) Flush() {
    if f, ok := rw.ResponseWriter.(http.Flusher); ok {
        f.Flush()
    }
}

// MaxBodySizeMiddleware limits the size of request bodies to maxBytes.
func MaxBodySizeMiddleware(maxBytes int64) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
            next.ServeHTTP(w, r)
        })
    }
}
