package handlers

import (
    "context"
    "encoding/json"
    "io"
    "net/http"
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

var logUpgrader = websocket.Upgrader{}

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

    // Stream lines as they arrive
    reader := make([]byte, 4096)
    partial := make([]byte, 0)
    for {
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
        // check for client close (non-blocking)
        conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
        if _, _, err := conn.ReadMessage(); err != nil {
            // assume closed
            return
        }
    }
}
