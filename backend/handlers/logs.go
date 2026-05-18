package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "regexp"
    "sort"
    "strings"
    "sync"
    "time"

    dockertypes "github.com/docker/docker/api/types"
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

// digestParams holds the parsed common query parameters shared by the
// per-container Digest and the bulk BulkDigest endpoints.
type digestParams struct {
    since      time.Duration
    limit      int
    maxScan    int
    errorRegex *regexp.Regexp
    warnRegex  *regexp.Regexp
}

// parseDigestParams parses common digest query parameters. Returned errors
// describe malformed input and are suitable for an HTTP 400 response.
func parseDigestParams(r *http.Request) (digestParams, error) {
    p := digestParams{
        since:      24 * time.Hour,
        limit:      50,
        maxScan:    50000,
        errorRegex: docker.DefaultErrorRegex,
        warnRegex:  docker.DefaultWarnRegex,
    }
    q := r.URL.Query()

    if v := q.Get("since"); v != "" {
        parsed, err := time.ParseDuration(v)
        if err != nil {
            return p, fmt.Errorf("invalid since: %s", err.Error())
        }
        p.since = parsed
    }
    const minSince = 1 * time.Minute
    const maxSince = 7 * 24 * time.Hour
    if p.since < minSince {
        p.since = minSince
    }
    if p.since > maxSince {
        p.since = maxSince
    }

    if v := q.Get("limit"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            p.limit = n
        }
    }
    if p.limit < 1 {
        p.limit = 1
    }
    if p.limit > 200 {
        p.limit = 200
    }

    if v := q.Get("max_scan"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            p.maxScan = n
        }
    }
    if p.maxScan < 100 {
        p.maxScan = 100
    }
    if p.maxScan > 200000 {
        p.maxScan = 200000
    }

    if v := q.Get("error_patterns"); v != "" {
        re, err := compileORRegex(v)
        if err != nil {
            return p, fmt.Errorf("invalid regex: %s", err.Error())
        }
        p.errorRegex = re
    }

    if q.Has("warn_patterns") {
        v := q.Get("warn_patterns")
        switch {
        case v == "":
            // keep default
        case v == "none":
            p.warnRegex = nil
        default:
            re, err := compileORRegex(v)
            if err != nil {
                return p, fmt.Errorf("invalid regex: %s", err.Error())
            }
            p.warnRegex = re
        }
    }

    return p, nil
}

// GET /api/containers/{id}/logs/digest
func (h *LogHandler) Digest(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    if id == "" {
        writeError(w, http.StatusBadRequest, "missing container id")
        return
    }

    params, err := parseDigestParams(r)
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }

    if h == nil || h.dclient == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
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

    rc, err := h.dclient.LogsSince(ctx, id, params.since)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to fetch logs")
        return
    }
    defer rc.Close()

    result := docker.ScanLogs(rc, docker.DigestOptions{
        Since:       params.since,
        ErrorRegex:  params.errorRegex,
        WarnRegex:   params.warnRegex,
        SampleLimit: params.limit,
        MaxScan:     params.maxScan,
    })

    resp := map[string]interface{}{
        "container":      name,
        "container_id":   id,
        "window":         params.since.String(),
        "checked_at":     time.Now().UTC().Format(time.RFC3339),
        "error_count":    result.ErrorCount,
        "warn_count":     result.WarnCount,
        "first_error_at": result.FirstErrorAt,
        "last_error_at":  result.LastErrorAt,
        "lines_scanned":  result.Scanned,
        "truncated":      result.Truncated,
        "samples":        result.Samples,
    }

    writeJSON(w, http.StatusOK, resp)
}

// containerDigest is a single per-container entry in a bulk digest response.
type containerDigest struct {
    Container    string              `json:"container"`
    ContainerID  string              `json:"container_id"`
    ErrorCount   int                 `json:"error_count"`
    WarnCount    int                 `json:"warn_count"`
    FirstErrorAt string              `json:"first_error_at,omitempty"`
    LastErrorAt  string              `json:"last_error_at,omitempty"`
    Samples      []docker.DigestLine `json:"samples"`
    Truncated    bool                `json:"truncated"`
}

// scanAllContainers fans out the per-container log scanner across the
// provided containers using the given concurrency. Returns a containerDigest
// for each input container, in the same order as the input. Used by both
// BulkDigest and the StatusHandler so neither needs its own goroutine
// plumbing.
func (h *LogHandler) scanAllContainers(ctx context.Context, containers []dockertypes.Container, params digestParams, concurrency int) []containerDigest {
    if concurrency < 1 {
        concurrency = 1
    }
    sem := make(chan struct{}, concurrency)
    var wg sync.WaitGroup
    results := make([]containerDigest, len(containers))
    for i, c := range containers {
        wg.Add(1)
        go func(i int, c dockertypes.Container) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()

            name := ""
            if len(c.Names) > 0 {
                name = strings.TrimPrefix(c.Names[0], "/")
            }
            if name == "" {
                name = c.ID
            }

            cd := containerDigest{
                Container:   name,
                ContainerID: c.ID,
                Samples:     []docker.DigestLine{},
            }

            scanCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
            defer cancel()

            rc, err := h.dclient.LogsSince(scanCtx, c.ID, params.since)
            if err != nil {
                results[i] = cd
                return
            }
            defer rc.Close()

            res := docker.ScanLogs(rc, docker.DigestOptions{
                Since:       params.since,
                ErrorRegex:  params.errorRegex,
                WarnRegex:   params.warnRegex,
                SampleLimit: params.limit,
                MaxScan:     params.maxScan,
            })

            cd.ErrorCount = res.ErrorCount
            cd.WarnCount = res.WarnCount
            cd.FirstErrorAt = res.FirstErrorAt
            cd.LastErrorAt = res.LastErrorAt
            cd.Truncated = res.Truncated
            cd.Samples = res.Samples
            results[i] = cd
        }(i, c)
    }
    wg.Wait()
    return results
}

// GET /api/logs/digest — fans out the per-container scanner across all
// running containers (or all containers when include_stopped=true) and
// returns a bounded aggregate (256 KB cap).
func (h *LogHandler) BulkDigest(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.dclient == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
        return
    }

    params, err := parseDigestParams(r)
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }

    q := r.URL.Query()

    concurrency := 5
    if v := q.Get("concurrency"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            concurrency = n
        }
    }
    if concurrency < 1 {
        concurrency = 1
    }
    if concurrency > 16 {
        concurrency = 16
    }

    includeStopped := false
    if v := q.Get("include_stopped"); v != "" {
        if b, err := strconv.ParseBool(v); err == nil {
            includeStopped = b
        }
    }

    perContainerSamples := 5
    if v := q.Get("per_container_samples"); v != "" {
        if n, err := strconv.Atoi(v); err == nil {
            perContainerSamples = n
        }
    }
    if perContainerSamples < 1 {
        perContainerSamples = 1
    }
    if perContainerSamples > 50 {
        perContainerSamples = 50
    }

    listCtx, listCancel := context.WithTimeout(r.Context(), 5*time.Second)
    containers, err := h.dclient.ListContainers(listCtx)
    listCancel()
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to list containers")
        return
    }

    filtered := make([]dockertypes.Container, 0, len(containers))
    for _, c := range containers {
        if !includeStopped && c.State != "running" {
            continue
        }
        filtered = append(filtered, c)
    }

    // Bound overall fan-out latency: each container scan also has its own 20s
    // timeout, but without this wrapper a large fleet could still serially
    // chew through ~(n/concurrency)*20s before the handler returns.
    scanCtx, scanCancel := context.WithTimeout(r.Context(), 60*time.Second)
    defer scanCancel()
    results := h.scanAllContainers(scanCtx, filtered, params, concurrency)

    totalErrors := 0
    totalWarns := 0
    containersWithErrors := 0
    for _, cd := range results {
        totalErrors += cd.ErrorCount
        totalWarns += cd.WarnCount
        if cd.ErrorCount > 0 {
            containersWithErrors++
        }
    }

    sort.SliceStable(results, func(i, j int) bool {
        a, b := results[i], results[j]
        if a.ErrorCount != b.ErrorCount {
            return a.ErrorCount > b.ErrorCount
        }
        if a.WarnCount != b.WarnCount {
            return a.WarnCount > b.WarnCount
        }
        return a.Container < b.Container
    })

    for i := range results {
        if len(results[i].Samples) > perContainerSamples {
            results[i].Samples = results[i].Samples[:perContainerSamples]
        }
    }

    resp := map[string]interface{}{
        "window":     params.since.String(),
        "checked_at": time.Now().UTC().Format(time.RFC3339),
        "summary": map[string]int{
            "containers_scanned":     len(results),
            "containers_with_errors": containersWithErrors,
            "total_errors":           totalErrors,
            "total_warns":            totalWarns,
        },
        "containers": results,
    }

    const maxBytes = 256 * 1024
    body, _ := json.Marshal(resp)
    truncated := false

    if len(body) > maxBytes {
        // Drop samples starting from the container with the lowest error
        // count (clean ones first). After all samples are gone, drop the
        // containers themselves, lowest counts first.
        order := make([]int, len(results))
        for i := range order {
            order[i] = i
        }
        sort.SliceStable(order, func(i, j int) bool {
            a, b := results[order[i]], results[order[j]]
            if a.ErrorCount != b.ErrorCount {
                return a.ErrorCount < b.ErrorCount
            }
            return a.WarnCount < b.WarnCount
        })
        for _, idx := range order {
            if len(results[idx].Samples) == 0 {
                continue
            }
            results[idx].Samples = []docker.DigestLine{}
            truncated = true
            resp["containers"] = results
            body, _ = json.Marshal(resp)
            if len(body) <= maxBytes {
                break
            }
        }

        if len(body) > maxBytes {
            // Drop containers, lowest priority first (kept sorted from the
            // earlier descending sort, so trim from the tail).
            for len(results) > 0 && len(body) > maxBytes {
                results = results[:len(results)-1]
                resp["containers"] = results
                body, _ = json.Marshal(resp)
            }
            truncated = true
        }
    }

    if truncated {
        resp["response_truncated"] = true
        body, _ = json.Marshal(resp)
    }

    w.Header().Set("Content-Type", "application/json")
    _, _ = w.Write(body)
}

// Caps on user-supplied regex input to keep compileORRegex bounded.
// A large or pathological pattern list can otherwise burn CPU/memory at
// compile time (and at match time across long log streams).
const (
    maxRegexInputBytes = 1024
    maxRegexPatterns   = 32
)

// compileORRegex takes a comma-separated list of regex fragments and
// compiles them into a single case-insensitive alternation pattern.
func compileORRegex(commaList string) (*regexp.Regexp, error) {
    if len(commaList) > maxRegexInputBytes {
        return nil, fmt.Errorf("pattern list too long (max %d bytes)", maxRegexInputBytes)
    }
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
    if len(cleaned) > maxRegexPatterns {
        return nil, fmt.Errorf("too many patterns (max %d)", maxRegexPatterns)
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
