package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "regexp"
    "strings"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/gorilla/websocket"
    "strconv"

    "backend/docker"
)

type LogHandler struct {
    dclient *docker.Client
}

func NewLogHandler(d *docker.Client) *LogHandler {
    return &LogHandler{dclient: d}
}

var logUpgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

// GET /api/containers/{id}/logs
func (h *LogHandler) Get(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.dclient == nil {
        http.Error(w, "docker daemon unreachable", http.StatusServiceUnavailable)
        return
    }
    id := chi.URLParam(r, "id")
    if id == "" {
        http.Error(w, "missing container id", http.StatusBadRequest)
        return
    }

    // parse query params
    q := r.URL.Query()
    since := q.Get("since")
    until := q.Get("until")
    tail := 500
    if t := q.Get("tail"); t != "" {
        if n, err := strconv.Atoi(t); err == nil {
            tail = n
        }
    }
    filter := q.Get("filter")

    tsPtr := new(bool)
    *tsPtr = true
    opts := docker.LogOptions{Since: since, Until: until, Tail: tail, Follow: false, Timestamps: tsPtr}

    // log queries may be larger; allow a longer timeout (30s)
    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    rc, err := h.dclient.GetContainerLogs(ctx, id, opts)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rc.Close()

    lines := []map[string]string{}

    // read whole stream into memory (bounded by tail) — simple approach
    buf := make([]byte, 0)
    tmp := make([]byte, 4096)
    for {
        n, err := rc.Read(tmp)
        if n > 0 {
            buf = append(buf, tmp[:n]...)
        }
        if err != nil {
            if err == io.EOF {
                break
            }
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
    }

    // split by newline and parse lines
    for _, raw := range strings.Split(string(buf), "\n") {
        if raw == "" {
            continue
        }
        _, stream, msg := docker.ParseLogLine([]byte(raw))
        // naive timestamp extraction again
        ts := ""
        if i := strings.IndexByte(raw, ' '); i > 0 {
            ts = raw[:i]
        }
        if filter != "" && !strings.Contains(strings.ToLower(msg), strings.ToLower(filter)) {
            continue
        }
        lines = append(lines, map[string]string{"timestamp": ts, "stream": stream, "message": msg})
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]interface{}{"lines": lines})
}

// GET /api/containers/{id}/logs/digest
func (h *LogHandler) Digest(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    if id == "" {
        http.Error(w, "missing container id", http.StatusBadRequest)
        return
    }

    q := r.URL.Query()

    since := 24 * time.Hour
    if v := q.Get("since"); v != "" {
        parsed, err := time.ParseDuration(v)
        if err != nil {
            http.Error(w, fmt.Sprintf(`{"error":"invalid since: %s"}`, err.Error()), http.StatusBadRequest)
            return
        }
        since = parsed
    }
    const minSince = 1 * time.Minute
    const maxSince = 7 * 24 * time.Hour
    if since < minSince {
        since = minSince
    }
    if since > maxSince {
        since = maxSince
    }

    limit := 50
    if v := q.Get("limit"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            limit = n
        }
    }
    if limit < 1 {
        limit = 1
    }
    if limit > 200 {
        limit = 200
    }

    maxScan := 50000
    if v := q.Get("max_scan"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            maxScan = n
        }
    }
    if maxScan < 100 {
        maxScan = 100
    }
    if maxScan > 200000 {
        maxScan = 200000
    }

    errorRegex := docker.DefaultErrorRegex
    if v := q.Get("error_patterns"); v != "" {
        re, err := compileORRegex(v)
        if err != nil {
            http.Error(w, fmt.Sprintf(`{"error":"invalid regex: %s"}`, err.Error()), http.StatusBadRequest)
            return
        }
        errorRegex = re
    }

    warnRegex := docker.DefaultWarnRegex
    if q.Has("warn_patterns") {
        v := q.Get("warn_patterns")
        switch {
        case v == "":
            warnRegex = docker.DefaultWarnRegex
        case v == "none":
            warnRegex = nil
        default:
            re, err := compileORRegex(v)
            if err != nil {
                http.Error(w, fmt.Sprintf(`{"error":"invalid regex: %s"}`, err.Error()), http.StatusBadRequest)
                return
            }
            warnRegex = re
        }
    }

    if h == nil || h.dclient == nil {
        http.Error(w, "docker daemon unreachable", http.StatusServiceUnavailable)
        return
    }

    name := id
    inspectCtx, inspectCancel := context.WithTimeout(r.Context(), 5*time.Second)
    if info, err := h.dclient.InspectContainer(inspectCtx, id); err != nil {
        log.Printf("warning: inspect container %q failed: %v", id, err)
    } else {
        if info.Name != "" {
            name = strings.TrimPrefix(info.Name, "/")
        }
    }
    inspectCancel()

    ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
    defer cancel()

    rc, err := h.dclient.LogsSince(ctx, id, since)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rc.Close()

    result := docker.ScanLogs(rc, docker.DigestOptions{
        Since:       since,
        ErrorRegex:  errorRegex,
        WarnRegex:   warnRegex,
        SampleLimit: limit,
        MaxScan:     maxScan,
    })

    resp := map[string]interface{}{
        "container":      name,
        "container_id":   id,
        "window":         since.String(),
        "checked_at":     time.Now().UTC().Format(time.RFC3339),
        "error_count":    result.ErrorCount,
        "warn_count":     result.WarnCount,
        "first_error_at": result.FirstErrorAt,
        "last_error_at":  result.LastErrorAt,
        "lines_scanned":  result.Scanned,
        "truncated":      result.Truncated,
        "samples":        result.Samples,
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(resp)
}

// compileORRegex takes a comma-separated list of regex fragments and
// compiles them into a single case-insensitive alternation pattern.
func compileORRegex(commaList string) (*regexp.Regexp, error) {
    parts := strings.Split(commaList, ",")
    cleaned := make([]string, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p == "" {
            continue
        }
        cleaned = append(cleaned, p)
    }
    if len(cleaned) == 0 {
        return nil, fmt.Errorf("no patterns provided")
    }
    pattern := "(?i)(" + strings.Join(cleaned, "|") + ")"
    return regexp.Compile(pattern)
}

// WS /api/containers/{id}/logs/stream
func (h *LogHandler) WS(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.dclient == nil {
        http.Error(w, "docker daemon unreachable", http.StatusServiceUnavailable)
        return
    }
    id := chi.URLParam(r, "id")
    if id == "" {
        http.Error(w, "missing container id", http.StatusBadRequest)
        return
    }

    conn, err := logUpgrader.Upgrade(w, r, nil)
    if err != nil {
        http.Error(w, "failed to upgrade websocket", http.StatusBadRequest)
        return
    }
    defer conn.Close()

    // Request logs with follow=true
    tsPtr := new(bool)
    *tsPtr = true
    opts := docker.LogOptions{Tail: 100, Follow: true, Timestamps: tsPtr}
    ctx := r.Context()
    rc, err := h.dclient.GetContainerLogs(ctx, id, opts)
    if err != nil {
        _ = conn.WriteJSON(map[string]string{"error": err.Error()})
        return
    }
    defer rc.Close()

    // Read pump: detect client disconnect
    done := make(chan struct{})
    go func() {
        defer close(done)
        for {
            if _, _, err := conn.ReadMessage(); err != nil {
                return
            }
        }
    }()

    // Stream lines as they arrive
    reader := make([]byte, 4096)
    partial := make([]byte, 0)
    for {
        // Check if client disconnected
        select {
        case <-done:
            return
        default:
        }

        n, err := rc.Read(reader)
        if n > 0 {
            partial = append(partial, reader[:n]...)
            // split into lines
            parts := strings.Split(string(partial), "\n")
            // last part may be incomplete
            for i := 0; i < len(parts)-1; i++ {
                raw := parts[i]
                ts, stream, msg := docker.ParseLogLine([]byte(raw))
                _ = conn.WriteJSON(map[string]string{"timestamp": ts, "stream": stream, "message": msg})
            }
            partial = []byte(parts[len(parts)-1])
        }
        if err != nil {
            if err == io.EOF {
                return
            }
            _ = conn.WriteJSON(map[string]string{"error": err.Error()})
            return
        }
    }
}
