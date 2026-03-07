package handlers

import (
    "context"
    "os"
    "fmt"
    "net/http"
    "strings"
    "time"

    "backend/docker"
)

type ContainerHandler struct {
    d *docker.Client
}

func NewContainerHandler(d *docker.Client) *ContainerHandler {
    return &ContainerHandler{d: d}
}

// RequireActions is middleware that blocks container action routes unless
// the ALLOW_ACTIONS env var is set to "true".
func RequireActions(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if os.Getenv("ALLOW_ACTIONS") != "true" {
            writeError(w, http.StatusForbidden, "container actions are disabled")
            return
        }
        next.ServeHTTP(w, r)
    })
}

// ContainerResponse is a simplified representation returned by the API
type ContainerResponse struct {
    ID      string   `json:"id"`
    Name    string   `json:"name"`
    Image   string   `json:"image"`
    State   string   `json:"state"`
    Status  string   `json:"status"`
    Ports   []string `json:"ports"`
    Created int64    `json:"created"`
}

// List handles GET /api/containers
func (h *ContainerHandler) List(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.d == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
        return
    }

    // limit list operation to 5s to avoid hanging requests
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    ctrs, err := h.d.ListContainers(ctx)
    if err != nil {
        writeError(w, http.StatusServiceUnavailable, fmt.Sprintf("failed to list containers: %v", err))
        return
    }

    resp := make([]ContainerResponse, 0, len(ctrs))
    for _, c := range ctrs {
        name := ""
        if len(c.Names) > 0 {
            name = strings.TrimPrefix(c.Names[0], "/")
        }

        ports := make([]string, 0, len(c.Ports))
        for _, p := range c.Ports {
            if p.PublicPort != 0 {
                // e.g. 0.0.0.0:8080->80/tcp
                ports = append(ports, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
            } else {
                ports = append(ports, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
            }
        }

        resp = append(resp, ContainerResponse{
            ID:      c.ID,
            Name:    name,
            Image:   c.Image,
            State:   c.State,
            Status:  c.Status,
            Ports:   ports,
            Created: c.Created,
        })
    }

    writeJSON(w, http.StatusOK, resp)
}

// Inspect handles GET /api/containers/{id}
func (h *ContainerHandler) Inspect(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.d == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
        return
    }

    id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/containers/"))
    if id == "" {
        writeError(w, http.StatusBadRequest, "missing container id")
        return
    }

    // Inspect can take longer; use a 10s timeout
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    info, err := h.d.InspectContainer(ctx, id)
    if err != nil {
        // Docker SDK returns a plain error; try to map to 404 when appropriate
        writeError(w, http.StatusNotFound, fmt.Sprintf("container not found: %v", err))
        return
    }

    // Build a reduced response with selected fields
    resp := map[string]interface{}{
        "id":   info.ID,
        "name": strings.TrimPrefix(info.Name, "/"),
        "image": info.Config.Image,
        "state": info.State,
        "config": map[string]interface{}{
            "env":      redactEnv(info.Config.Env),
            "exposed":  info.Config.ExposedPorts,
            "cmd":      info.Config.Cmd,
            "entrypoint": info.Config.Entrypoint,
            "labels":   info.Config.Labels,
        },
        "networkSettings": info.NetworkSettings,
        "mounts":          info.Mounts,
        "restartCount":    info.RestartCount,
        "platform":        info.Platform,
    }

    writeJSON(w, http.StatusOK, resp)
}

// redactEnv returns env vars with values redacted (key only)
func redactEnv(env []string) []string {
    out := make([]string, 0, len(env))
    for _, e := range env {
        if i := strings.Index(e, "="); i > 0 {
            out = append(out, e[:i]+"=REDACTED")
        } else {
            out = append(out, e)
        }
    }
    return out
}

// Start handles POST /api/containers/{id}/start
func (h *ContainerHandler) Start(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.d == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
        return
    }
    id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/containers/"))
    id = strings.TrimSuffix(id, "/start")
    if id == "" {
        writeError(w, http.StatusBadRequest, "missing container id")
        return
    }
    // use 10s timeout for inspect before attempting start
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    info, err := h.d.InspectContainer(ctx, id)
    if err != nil {
        writeError(w, http.StatusNotFound, fmt.Sprintf("container not found: %v", err))
        return
    }
    name := strings.TrimPrefix(info.Name, "/")
    if info.State != nil && info.State.Running {
        writeError(w, http.StatusConflict, "container already running")
        return
    }
    if err := h.d.StartContainer(ctx, id); err != nil {
        writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start container: %v", err))
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "action": "started", "container": name})
}

// Stop handles POST /api/containers/{id}/stop
func (h *ContainerHandler) Stop(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.d == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
        return
    }
    id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/containers/"))
    id = strings.TrimSuffix(id, "/stop")
    if id == "" {
        writeError(w, http.StatusBadRequest, "missing container id")
        return
    }
    // use 10s timeout for inspect before attempting stop
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    info, err := h.d.InspectContainer(ctx, id)
    if err != nil {
        writeError(w, http.StatusNotFound, fmt.Sprintf("container not found: %v", err))
        return
    }
    name := strings.TrimPrefix(info.Name, "/")
    if info.State != nil && !info.State.Running {
        writeError(w, http.StatusConflict, "container not running")
        return
    }
    // 10-second graceful timeout
    if err := h.d.StopContainer(ctx, id, 10*time.Second); err != nil {
        writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to stop container: %v", err))
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "action": "stopped", "container": name})
}

// Restart handles POST /api/containers/{id}/restart
func (h *ContainerHandler) Restart(w http.ResponseWriter, r *http.Request) {
    if h == nil || h.d == nil {
        writeError(w, http.StatusServiceUnavailable, "docker daemon unreachable")
        return
    }
    id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/containers/"))
    id = strings.TrimSuffix(id, "/restart")
    if id == "" {
        writeError(w, http.StatusBadRequest, "missing container id")
        return
    }
    // use 10s timeout for inspect before attempting restart
    ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
    defer cancel()
    info, err := h.d.InspectContainer(ctx, id)
    if err != nil {
        writeError(w, http.StatusNotFound, fmt.Sprintf("container not found: %v", err))
        return
    }
    name := strings.TrimPrefix(info.Name, "/")
    // 10-second graceful timeout for restart
    if err := h.d.RestartContainer(ctx, id, 10*time.Second); err != nil {
        writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to restart container: %v", err))
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "action": "restarted", "container": name})
}
