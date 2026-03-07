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
    _ "golang.org/x/crypto/bcrypt"

    "backend/docker"
    "backend/handlers"
    authpkg "backend/auth"
)

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

    // Authentication setup
    // SESSION_TTL can be set as a duration string (eg. "24h") or seconds
    sm := authpkg.NewSessionManager(0)

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
            _ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "docker": dockerState})
        })

        // public auth endpoints
        r.Post("/login", sm.LoginHandler)
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
