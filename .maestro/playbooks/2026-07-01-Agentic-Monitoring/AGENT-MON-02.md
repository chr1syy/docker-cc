# Agentic Monitoring — Phase 2: `/api/agent/snapshot` endpoint

**Why this phase exists.** The agentic judge should make ONE call and receive everything it needs to reason about the fleet: per-container health, current CPU/mem, the long-horizon memory trend from Phase 1 (so leaks are visible from a single response), recent error log lines, and host memory/load. Today an agent must stitch together `/api/status`, `/api/stats/history`, and `/api/logs/digest`. This phase bundles them into one LLM-friendly document at `GET /api/agent/snapshot`.

**Auth.** Register the route inside the existing protected group in `backend/main.go` (the `r.Group` that applies `sm.AuthMiddleware`). That middleware already accepts BOTH a session cookie AND `Authorization: Bearer <API_TOKEN>` (see `backend/auth/auth.go`), and bearer requests are read-only because `RequireActions`/`RejectAgentMiddleware` gate the mutating routes. So the snapshot endpoint is automatically agent-reachable and read-only with no new auth code. Do NOT put it on a public/unauthenticated path.

**Reuse — do not duplicate.** This handler composes existing primitives:
- Container list + state counts + unhealthy/stopped/restarting detection: mirror the logic in `backend/handlers/status.go` (`StatusHandler.Get`, `containerDisplayName`, the state-count switch).
- Log errors per container: reuse `(*LogHandler).scanAllContainers(ctx, running, digestParams{...}, concurrency)` exactly as `status.go` does (`limit: 2`, default regexes).
- Current CPU/mem per container: read the short `StatsBuffer` (latest point per container) — the handler already holds `*StatsHandler`/`*StatsBuffer`. Do not open a new Docker stats stream.
- Long-horizon memory trend: `MemHistory.Trend(name)` from Phase 1.

## Response contract (`GET /api/agent/snapshot`)

```json
{
  "checked_at": "2026-07-01T13:00:00Z",
  "docker": "connected",
  "version": "0.7.0",
  "window": "1h",
  "host": {
    "load1": 0.11, "load5": 0.09, "load15": 0.05,
    "mem_total_bytes": 8253534208,
    "mem_available_bytes": 6442450944,
    "mem_used_percent": 21.9
  },
  "counts": { "total": 11, "running": 11, "stopped": 0, "unhealthy": 0, "restarting": 0 },
  "containers": [
    {
      "name": "eplan-backend",
      "id": "abc123def456",
      "state": "running",
      "status": "Up 6 weeks",
      "health": "healthy",
      "started_at": "2026-05-20T09:00:00Z",
      "uptime_seconds": 3628800,
      "restart_count": 0,
      "cpu_percent": 0.4,
      "mem_usage_bytes": 83886080,
      "mem_limit_bytes": 805306368,
      "mem_percent": 10.4,
      "mem_trend": {
        "window": "24h", "samples": 1440,
        "first_bytes": 61000000, "last_bytes": 83886080,
        "min_bytes": 60000000, "max_bytes": 84000000,
        "slope_bytes_per_hour": 380000,
        "points": [ { "t": "2026-06-30T13:00:00Z", "mem": 61000000, "limit": 805306368 } ]
      },
      "recent_errors": { "count": 3, "last_at": "2026-07-01T12:58:00Z", "samples": ["ERROR ...", "FATAL ..."] }
    }
  ]
}
```

Notes: `health` is `"healthy"`/`"unhealthy"`/`"starting"`/`"none"`. `mem_trend` is `null` when <2 long samples exist. `recent_errors.count` is 0 (with empty samples) for clean containers. `window` is the log-scan window (query param, default `1h`).

## Prerequisites
- `export PATH=$PATH:/usr/local/go/bin` before any `go` command.

---

- [x] **Add a host-info helper `backend/handlers/hostinfo.go` reading `/proc` (host-visible from inside the container).**

  Done. Added `HostInfo` struct + `ReadHostInfo()` (returns a partial struct with a logged `hostinfo:` warning when `/proc/loadavg` or `/proc/meminfo` is unreadable, e.g. non-Linux dev host, so the caller can set `host` to null without failing the snapshot). Parsing is split into unexported `parseLoadAvg(string)` / `parseMemInfo(string)` helpers so tests use fixture strings and don't touch the runner's real `/proc`. `parseMemInfo` guards against divide-by-zero when `MemTotal` is absent. `hostinfo_test.go` has 4 `TestHostInfo_*` cases (well-formed + malformed loadavg, well-formed meminfo + missing-total). Verified: `go build ./...`, `go test ./handlers/ -run HostInfo` (4 passed), `go vet ./handlers/` clean.

  `/proc/loadavg` and `/proc/meminfo` are NOT namespaced — inside a standard container they report the host kernel's values (this matches the live check: host ~6 GB free, load 0.11). Implement:
  - `type HostInfo struct` with the JSON fields under `host` above.
  - `func ReadHostInfo() (*HostInfo, error)` — parse `/proc/loadavg` (first three fields → `load1/5/15`) and `/proc/meminfo` (`MemTotal`, `MemAvailable` in kB → bytes; compute `mem_used_percent = (1 - available/total) * 100`). Return a partial struct with a logged warning if a file is unreadable (e.g. non-Linux dev host) rather than failing the whole snapshot — the caller sets `host` to `null` on error.
  - Add `backend/handlers/hostinfo_test.go` that parses fixture strings (inject the raw text via small unexported parse helpers, e.g. `parseLoadAvg(string)` / `parseMemInfo(string)`, so the test does not depend on the runner's real `/proc`).

  Verification: `go build ./... && go test ./handlers/ -run HostInfo` passes.

- [ ] **Create `backend/handlers/snapshot.go` with `SnapshotHandler` and `GET /api/agent/snapshot`.**

  - `type SnapshotHandler struct { dclient *docker.Client; version string; logh *LogHandler; stats *StatsHandler; mem *MemHistory }` and `func NewSnapshotHandler(d *docker.Client, version string, logh *LogHandler, sh *StatsHandler, mh *MemHistory) *SnapshotHandler`. Expose whatever accessor you need on `StatsHandler`/`StatsBuffer` to fetch the latest point per container (e.g. add `(*StatsBuffer).Latest(id string) (docker.ContainerMetrics, bool)` returning the last element).
  - `Get(w, r)`:
    1. Parse `window` (log scan, default `1h`, clamp `[1m, 24h]`) like `status.go` parses `since`.
    2. Ping Docker (2 s). On disconnect: return `docker:"disconnected"`, `host` from `ReadHostInfo()`, empty `containers`, zeroed `counts`, HTTP 200.
    3. List containers; build `counts` and the per-container slice. For running containers, inspect (budget the inspects, e.g. cap at the fleet size — 11 here is fine, use a 5 s per-inspect timeout) to fill `health` (from `State.Health.Status`), `started_at` (`State.StartedAt`), `uptime_seconds` (now − started), and `restart_count` (`RestartCount`). Fall back gracefully when an inspect fails (leave fields zero/empty; never abort the whole response).
    4. Current stats: from the short `StatsBuffer` latest point → `cpu_percent`, `mem_usage_bytes`, `mem_limit_bytes`, `mem_percent`.
    5. `mem_trend`: `h.mem.Trend(name)` (may be `null`).
    6. `recent_errors`: run `scanAllContainers` over running containers once (`limit: 2`, default regexes, `window`), map results by container name; attach `count`/`last_at`/`samples`.
    7. `host`: `ReadHostInfo()`.
    8. Write the typed struct as JSON (Content-Type `application/json`), field order matching the contract.
  - Keep total work bounded: single list, ≤fleet inspects, one `scanAllContainers` fan-out. Target well under a couple seconds for 11 containers.

  Verification: `go build ./...` succeeds and `go vet ./...` is clean.

- [ ] **Wire the route in `backend/main.go`.**

  Next to the other handler constructors, add `snap := handlers.NewSnapshotHandler(dclient, Version, lh, sh, mh)` (reuse the existing `lh`, `sh`, and the `mh` from Phase 1). Inside the protected `r.Group` that uses `sm.AuthMiddleware` (the same group holding `/status`), register:

  ```go
  r.Get("/agent/snapshot", snap.Get)
  ```

  Do NOT add it under `RequireActions`. Confirm the route sits alongside `r.Get("/status", stath.Get)`.

  Verification: `go build ./...` succeeds. Then smoke-run without Docker to confirm the disconnected path returns 200: start the server (`ADMIN_PASSWORD=x SESSION_SECRET=y API_TOKEN=t go run .`) and `curl -s -H "Authorization: Bearer t" localhost:8080/api/agent/snapshot | head -c 400` returns JSON with `"docker"` present. Stop the server afterward.
