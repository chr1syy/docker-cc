package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "strings"
    "time"

    dockertypes "github.com/docker/docker/api/types"

    "backend/docker"
)

// StatusHandler exposes the aggregated agent-facing health summary at
// GET /api/status. It composes the bulk log scanner used by LogHandler
// with a container state rollup so an agent can poll a single endpoint.
type StatusHandler struct {
    dclient *docker.Client
    version string
    logh    *LogHandler
}

// NewStatusHandler constructs a StatusHandler. logh is reused to share the
// scanAllContainers fan-out plumbing.
func NewStatusHandler(d *docker.Client, version string, logh *LogHandler) *StatusHandler {
    return &StatusHandler{dclient: d, version: version, logh: logh}
}

type statusContainerCounts struct {
    Total      int `json:"total"`
    Running    int `json:"running"`
    Stopped    int `json:"stopped"`
    Unhealthy  int `json:"unhealthy"`
    Restarting int `json:"restarting"`
}

type statusIssue struct {
    Container   string              `json:"container"`
    Kind        string              `json:"kind"`
    ErrorCount  int                 `json:"error_count,omitempty"`
    LastErrorAt string              `json:"last_error_at,omitempty"`
    Since       string              `json:"since,omitempty"`
    Samples     []docker.DigestLine `json:"samples,omitempty"`
}

type statusResponse struct {
    Healthy    bool                  `json:"healthy"`
    CheckedAt  string                `json:"checked_at"`
    Window     string                `json:"window"`
    Docker     string                `json:"docker"`
    Version    string                `json:"version"`
    Containers statusContainerCounts `json:"containers"`
    Issues     []statusIssue         `json:"issues"`
}

// Get implements GET /api/status.
func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
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
    if since < 1*time.Minute {
        since = 1 * time.Minute
    }
    if since > 7*24*time.Hour {
        since = 7 * 24 * time.Hour
    }

    includeSamples := true
    if v := q.Get("include_samples"); v != "" {
        if b, err := strconv.ParseBool(v); err == nil {
            includeSamples = b
        }
    }

    dockerState := "disconnected"
    if h.dclient != nil {
        pingCtx, pingCancel := context.WithTimeout(r.Context(), 2*time.Second)
        if err := h.dclient.Ping(pingCtx); err == nil {
            dockerState = "connected"
        }
        pingCancel()
    }

    counts := statusContainerCounts{}
    issues := []statusIssue{}

    if dockerState == "connected" {
        listCtx, listCancel := context.WithTimeout(r.Context(), 5*time.Second)
        containers, err := h.dclient.ListContainers(listCtx)
        listCancel()
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        var (
            unhealthyCandidates  []dockertypes.Container
            stoppedCandidates    []dockertypes.Container
            restartingCandidates []dockertypes.Container
            runningContainers    []dockertypes.Container
        )
        for _, c := range containers {
            counts.Total++
            switch c.State {
            case "running":
                counts.Running++
                runningContainers = append(runningContainers, c)
                if strings.Contains(strings.ToLower(c.Status), "unhealthy") {
                    counts.Unhealthy++
                    unhealthyCandidates = append(unhealthyCandidates, c)
                }
            case "exited":
                counts.Stopped++
                stoppedCandidates = append(stoppedCandidates, c)
            case "restarting":
                counts.Restarting++
                restartingCandidates = append(restartingCandidates, c)
            }
        }

        // Issues for container state. Cap inspects across all kinds to bound
        // latency — fall back to Container.Created when the budget is spent.
        inspectBudget := 5

        appendStateIssue := func(c dockertypes.Container, kind string, useFinishedAt bool) {
            issue := statusIssue{Container: containerDisplayName(c), Kind: kind}
            inspected := false
            if inspectBudget > 0 {
                inspectCtx, inspectCancel := context.WithTimeout(r.Context(), 5*time.Second)
                if info, err := h.dclient.InspectContainer(inspectCtx, c.ID); err == nil {
                    if info.State != nil {
                        if useFinishedAt && info.State.FinishedAt != "" {
                            issue.Since = info.State.FinishedAt
                            inspected = true
                        } else if !useFinishedAt && info.State.StartedAt != "" {
                            issue.Since = info.State.StartedAt
                            inspected = true
                        }
                    }
                }
                inspectCancel()
                inspectBudget--
            }
            if !inspected && c.Created > 0 {
                issue.Since = time.Unix(c.Created, 0).UTC().Format(time.RFC3339)
            }
            issues = append(issues, issue)
        }

        for _, c := range unhealthyCandidates {
            appendStateIssue(c, "unhealthy", false)
        }
        for _, c := range stoppedCandidates {
            appendStateIssue(c, "stopped", true)
        }
        for _, c := range restartingCandidates {
            appendStateIssue(c, "restarting", false)
        }

        // Log error issues across running containers.
        if h.logh != nil && len(runningContainers) > 0 {
            scanCtx, scanCancel := context.WithTimeout(r.Context(), 30*time.Second)
            digests := h.logh.scanAllContainers(scanCtx, runningContainers, digestParams{
                since:      since,
                limit:      2,
                maxScan:    50000,
                errorRegex: docker.DefaultErrorRegex,
                warnRegex:  docker.DefaultWarnRegex,
            }, 5)
            scanCancel()
            for _, d := range digests {
                if d.ErrorCount == 0 {
                    continue
                }
                issue := statusIssue{
                    Container:   d.Container,
                    Kind:        "log_errors",
                    ErrorCount:  d.ErrorCount,
                    LastErrorAt: d.LastErrorAt,
                }
                if includeSamples {
                    if len(d.Samples) > 2 {
                        issue.Samples = d.Samples[:2]
                    } else {
                        issue.Samples = d.Samples
                    }
                }
                issues = append(issues, issue)
            }
        }
    }

    healthy := dockerState == "connected" &&
        counts.Stopped == 0 &&
        counts.Unhealthy == 0 &&
        counts.Restarting == 0
    if healthy {
        for _, i := range issues {
            if i.Kind == "log_errors" && i.ErrorCount > 0 {
                healthy = false
                break
            }
        }
    }

    resp := statusResponse{
        Healthy:    healthy,
        CheckedAt:  time.Now().UTC().Format(time.RFC3339),
        Window:     since.String(),
        Docker:     dockerState,
        Version:    h.version,
        Containers: counts,
        Issues:     issues,
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(resp)
}

func containerDisplayName(c dockertypes.Container) string {
    if len(c.Names) > 0 {
        return strings.TrimPrefix(c.Names[0], "/")
    }
    return c.ID
}
