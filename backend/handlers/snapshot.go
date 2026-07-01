package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"

	"backend/docker"
)

// SnapshotHandler exposes GET /api/agent/snapshot: a single LLM-friendly
// document bundling everything an agentic judge needs to reason about the
// fleet in one call — per-container health/state/uptime/restarts, current
// CPU/mem from the short stats buffer, the 24h memory trend (leak signal),
// recent error log lines, and host memory/load.
//
// It composes existing primitives rather than duplicating them: the state
// rollup mirrors StatusHandler, log errors reuse LogHandler.scanAllContainers,
// current stats come from StatsHandler's buffer, and the long-horizon trend is
// MemHistory.Trend.
type SnapshotHandler struct {
	dclient *docker.Client
	version string
	logh    *LogHandler
	stats   *StatsHandler
	mem     *MemHistory
}

// NewSnapshotHandler constructs a SnapshotHandler. logh, sh and mh are the same
// instances wired elsewhere so the snapshot shares their buffers/plumbing.
func NewSnapshotHandler(d *docker.Client, version string, logh *LogHandler, sh *StatsHandler, mh *MemHistory) *SnapshotHandler {
	return &SnapshotHandler{dclient: d, version: version, logh: logh, stats: sh, mem: mh}
}

type snapshotCounts struct {
	Total      int `json:"total"`
	Running    int `json:"running"`
	Stopped    int `json:"stopped"`
	Unhealthy  int `json:"unhealthy"`
	Restarting int `json:"restarting"`
}

// snapshotErrors is the recent_errors block per container. Samples are the raw
// log line texts (not structured DigestLine objects) to stay compact and
// readable for an LLM. count is 0 with an empty samples slice for clean
// containers.
type snapshotErrors struct {
	Count   int      `json:"count"`
	LastAt  string   `json:"last_at,omitempty"`
	Samples []string `json:"samples"`
}

// snapshotContainer field order matches the response contract in the playbook.
type snapshotContainer struct {
	Name          string         `json:"name"`
	ID            string         `json:"id"`
	State         string         `json:"state"`
	Status        string         `json:"status"`
	Health        string         `json:"health"`
	StartedAt     string         `json:"started_at"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	RestartCount  int            `json:"restart_count"`
	CPUPercent    float64        `json:"cpu_percent"`
	MemUsageBytes uint64         `json:"mem_usage_bytes"`
	MemLimitBytes uint64         `json:"mem_limit_bytes"`
	MemPercent    float64        `json:"mem_percent"`
	MemTrend      *MemTrend      `json:"mem_trend"`
	RecentErrors  snapshotErrors `json:"recent_errors"`
}

type snapshotResponse struct {
	CheckedAt  string              `json:"checked_at"`
	Docker     string              `json:"docker"`
	Version    string              `json:"version"`
	Window     string              `json:"window"`
	Host       *HostInfo           `json:"host"`
	Counts     snapshotCounts      `json:"counts"`
	Containers []snapshotContainer `json:"containers"`
}

// Get implements GET /api/agent/snapshot.
func (h *SnapshotHandler) Get(w http.ResponseWriter, r *http.Request) {
	// 1. Parse the log-scan window (default 1h, clamp [1m, 24h]).
	window := 1 * time.Hour
	if v := r.URL.Query().Get("window"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid window: "+err.Error())
			return
		}
		window = parsed
	}
	if window < 1*time.Minute {
		window = 1 * time.Minute
	}
	if window > 24*time.Hour {
		window = 24 * time.Hour
	}

	// host is best-effort: a partial/nil read must not fail the snapshot.
	host, hostErr := ReadHostInfo()
	if hostErr != nil {
		host = nil
	}

	// 2. Ping Docker. On disconnect, still serve a valid 200 with host info,
	// empty containers and zeroed counts.
	dockerState := "disconnected"
	if h.dclient != nil {
		pingCtx, pingCancel := context.WithTimeout(r.Context(), 2*time.Second)
		if err := h.dclient.Ping(pingCtx); err == nil {
			dockerState = "connected"
		}
		pingCancel()
	}

	counts := snapshotCounts{}
	containersOut := []snapshotContainer{}

	if dockerState == "connected" {
		listCtx, listCancel := context.WithTimeout(r.Context(), 5*time.Second)
		containers, err := h.dclient.ListContainers(listCtx)
		listCancel()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list containers")
			return
		}

		// Budget inspects at the fleet size so total work stays bounded.
		inspectBudget := len(containers)
		var running []dockertypes.Container

		for _, c := range containers {
			counts.Total++
			name := containerDisplayName(c)
			sc := snapshotContainer{
				Name:         name,
				ID:           c.ID,
				State:        c.State,
				Status:       c.Status,
				Health:       "none",
				RecentErrors: snapshotErrors{Samples: []string{}},
			}

			switch c.State {
			case "running":
				counts.Running++
				running = append(running, c)
				if strings.Contains(strings.ToLower(c.Status), "unhealthy") {
					counts.Unhealthy++
				}
			case "exited":
				counts.Stopped++
			case "restarting":
				counts.Restarting++
			}

			// 4. Current stats from the short ring buffer (no new stats stream).
			if h.stats != nil {
				if m, ok := h.stats.Latest(c.ID); ok {
					sc.CPUPercent = m.CPUPercent
					sc.MemUsageBytes = m.MemoryUsage
					sc.MemLimitBytes = m.MemoryLimit
					sc.MemPercent = m.MemoryPercent
				}
			}

			// 5. Long-horizon memory trend (nil when <2 long samples).
			if h.mem != nil {
				sc.MemTrend = h.mem.Trend(name)
			}

			// 3. Inspect running containers for health/uptime/restart_count.
			// A failed inspect leaves fields at their zero values — never abort.
			if c.State == "running" && inspectBudget > 0 {
				inspectBudget--
				inspectCtx, inspectCancel := context.WithTimeout(r.Context(), 5*time.Second)
				info, ierr := h.dclient.InspectContainer(inspectCtx, c.ID)
				inspectCancel()
				if ierr == nil {
					sc.RestartCount = info.RestartCount
					if info.State != nil {
						if info.State.Health != nil && info.State.Health.Status != "" {
							sc.Health = info.State.Health.Status
						}
						if info.State.StartedAt != "" {
							sc.StartedAt = info.State.StartedAt
							if t, perr := time.Parse(time.RFC3339Nano, info.State.StartedAt); perr == nil {
								if secs := int64(time.Since(t).Seconds()); secs > 0 {
									sc.UptimeSeconds = secs
								}
							}
						}
					}
				}
			}

			containersOut = append(containersOut, sc)
		}

		// 6. Recent error digest across running containers, reusing the shared
		// scanAllContainers fan-out exactly as status.go does.
		if h.logh != nil && len(running) > 0 {
			scanCtx, scanCancel := context.WithTimeout(r.Context(), 30*time.Second)
			digests := h.logh.scanAllContainers(scanCtx, running, digestParams{
				since:      window,
				limit:      2,
				maxScan:    50000,
				errorRegex: docker.DefaultErrorRegex,
				warnRegex:  docker.DefaultWarnRegex,
			}, 5)
			scanCancel()

			byName := make(map[string]snapshotErrors, len(digests))
			for _, d := range digests {
				se := snapshotErrors{Count: d.ErrorCount, LastAt: d.LastErrorAt, Samples: []string{}}
				for _, s := range d.Samples {
					se.Samples = append(se.Samples, s.Text)
				}
				byName[d.Container] = se
			}
			for i := range containersOut {
				if se, ok := byName[containersOut[i].Name]; ok {
					containersOut[i].RecentErrors = se
				}
			}
		}
	}

	resp := snapshotResponse{
		CheckedAt:  time.Now().UTC().Format(time.RFC3339),
		Docker:     dockerState,
		Version:    h.version,
		Window:     window.String(),
		Host:       host,
		Counts:     counts,
		Containers: containersOut,
	}

	writeJSON(w, http.StatusOK, resp)
}
