# Agentic Monitoring — Phase 1: Long-Horizon Memory History (leak detector foundation)

**Why this phase exists.** Leak/trend detection needs history that a single snapshot cannot carry. The existing `StatsBuffer` (`backend/handlers/statsbuffer.go`) keeps only 150 points at 2 s cadence — ~5 minutes, far too short to see a memory leak that develops over hours or days. This phase adds a **second, coarse, long-horizon memory buffer** (one sample per minute, ~24 h) that is persisted to `DATA_DIR` so it survives container restarts. Phase 2's `/api/agent/snapshot` reads a computed trend summary from it, so a single agent poll carries enough signal for the judge to detect a real upward memory trend. The prod target (`eplan-backend`, the only container with a hard 768 MiB limit) is exactly the case this protects.

**Design decisions baked in (do not deviate):**
- Coarse cadence: at most one long-buffer sample per container per 60 s. Feed it from the existing 2 s collector loop by gating on elapsed time, so we do NOT add a second goroutine or a second Docker stats call.
- Retention: `MaxLongHistoryPoints = 1440` (24 h at 1/min). Ring buffer, oldest evicted.
- Persist to `DATA_DIR` (default `./data`, already a mounted volume in prod). Atomic write (temp file + `os.Rename`), same convention as `backend/auth/totp.go` (see its `os.MkdirAll(dir, 0700)` + `os.WriteFile(0600)` + write pattern). Load on startup; flush periodically (every 5 min) and once on graceful shutdown.
- Only memory is retained long-horizon (leak detection target). We do NOT persist CPU/net/IO long-horizon — keep the file small.

## Prerequisites
- Run `export PATH=$PATH:/usr/local/go/bin` before any `go` command.
- Match existing code conventions: typed structs with `json:` tags, `sync.RWMutex` guarding maps (see `statsbuffer.go`), context timeouts on Docker calls.

---

- [x] **Create `backend/handlers/memhistory.go` with a persisted long-horizon per-container memory buffer.** _(Done 2026-07-01: created `MemHistory` keyed by container name with 60s-gated `Observe`, ring eviction at 1440 points, atomic `Save()`/`load()` to `mem_history.json` (0700 dir / 0600 file, temp+rename), and `Trend()` computing OLS `slope_bytes_per_hour` + ≤20-point downsample. `lastSample` is re-seeded from disk on load so the interval gate survives restarts. `go build ./...` and `go vet ./handlers/...` pass.)_

  Add package `handlers`. Define:
  - `const MaxLongHistoryPoints = 1440` and `const LongSampleInterval = 60 * time.Second`.
  - `type memSample struct { T time.Time; Mem uint64; Limit uint64 }` (JSON: `t`, `mem`, `limit`).
  - `type MemHistory struct { mu sync.RWMutex; data map[string][]memSample; lastSample map[string]time.Time; dir string }` where `data` is keyed by container **name** (stable across restarts, unlike short container IDs) and `dir` is `DATA_DIR`.
  - `func NewMemHistory(dataDir string) *MemHistory` — initializes maps, sets `dir`, then calls `load()` to hydrate from disk (ignore a missing file; log and continue on a corrupt file — never fail startup).
  - `func (h *MemHistory) Observe(name string, mem, limit uint64, now time.Time)` — appends a `memSample` only when `now.Sub(lastSample[name]) >= LongSampleInterval` (or no prior sample). Evicts oldest beyond `MaxLongHistoryPoints`. Updates `lastSample[name]`.
  - `func (h *MemHistory) load() error` and `func (h *MemHistory) Save() error` — JSON at `filepath.Join(dir, "mem_history.json")`. `Save` must be atomic: write to `mem_history.json.tmp` then `os.Rename`. Create `dir` with `os.MkdirAll(dir, 0700)` and file mode `0600`, mirroring `backend/auth/totp.go`.
  - `type MemTrend struct` with JSON fields: `window` (string, e.g. `"24h"`), `samples` (int, count in window), `first_bytes`, `last_bytes`, `min_bytes`, `max_bytes` (uint64), `slope_bytes_per_hour` (int64, least-squares slope of mem vs time), `points` ([]memSample, downsampled to at most 20 evenly-spaced points for LLM readability).
  - `func (h *MemHistory) Trend(name string) *MemTrend` — computes the summary over all retained samples for that container; returns `nil` when fewer than 2 samples exist. Compute `slope_bytes_per_hour` via ordinary least squares on (seconds-since-first, mem); guard against a zero time span.

  Verification: `export PATH=$PATH:/usr/local/go/bin && cd /home/chris/code/docker-cc/backend && go build ./...` succeeds.

- [x] **Feed the long buffer from the existing stats collector and wire persistence into the server lifecycle.** _(Done 2026-07-01: `StatsHandler` gained a `memHistory *MemHistory` field; `NewStatsHandler(d, mh)` now takes it and `collect()` calls `Observe(ContainerName, MemoryUsage, MemoryLimit, now)` for each non-empty-named metric after `PushAll`, sharing one `time.Now()` per batch. In `main.go`, `dataDir` resolution was hoisted above the stats handler (and the later duplicate removed), `mh := handlers.NewMemHistory(dataDir)` is constructed and passed to `NewStatsHandler`, a 5-min flush ticker goroutine calls `mh.Save()` (logs errors), and the graceful-shutdown path calls `mh.Save()` once after `srv.Shutdown`. `go build ./...` + `go vet ./...` clean; `go test ./handlers/...` = 26 passed.)_

  In `backend/handlers/stats.go`:
  - Add a `memHistory *MemHistory` field to `StatsHandler` and a constructor param, OR (cleaner) pass an existing `*MemHistory` in. Update `NewStatsHandler` to accept `mh *MemHistory` and store it. Inside `collect()`, after `s.buffer.PushAll(metrics)`, loop the batch and call `s.memHistory.Observe(m.ContainerName, m.MemoryUsage, m.MemoryLimit, time.Now())` for each metric that has a non-empty `ContainerName`.

  In `backend/main.go`:
  - Construct `mh := handlers.NewMemHistory(dataDir)` where `dataDir` is the resolved `DATA_DIR` value already used elsewhere (grep for how `DATA_DIR` / the TOTP manager gets its dir; reuse the same variable). Pass `mh` into `handlers.NewStatsHandler(dclient, mh)`.
  - Start a periodic flush goroutine: a 5-minute ticker calling `mh.Save()` (log errors, never crash).
  - In the existing graceful-shutdown path (there is already a `srv.Shutdown` block near the bottom of `main.go`), call `mh.Save()` once after the server stops so the newest samples are persisted.

  Verification: `go build ./...` succeeds; `go vet ./...` is clean. Run `go run .` locally is NOT required for this task (no Docker assumptions) — the build + vet gate is sufficient.

- [x] **Add unit tests in `backend/handlers/memhistory_test.go`.** _(Done 2026-07-01: added 8 tests using `t.TempDir()` — interval-gate (1 sample from two calls 10s apart, 2 after a +90s call), empty-name ignore, ring eviction (retains exactly 1440, oldest 5 dropped), `Trend` nil for <2 samples, increasing series → positive slope with ≤20 endpoint-preserving downsampled points and first<last, persistence round-trip (reload matches counts/last sample and re-seeds the interval gate), atomic Save leaves no `.tmp`, and corrupt-file tolerance (no panic, starts empty, stays usable). `go test ./handlers/...` = 34 passed; `go vet ./handlers/...` clean.)_

  Cover, using a `t.TempDir()` for `DATA_DIR`:
  - `Observe` respects the 60 s gate: two `Observe` calls 10 s apart for the same name produce exactly one sample; a third at +90 s produces two.
  - Ring eviction: pushing `MaxLongHistoryPoints + 5` samples (advance `now` by 60 s each) retains exactly `MaxLongHistoryPoints`, oldest dropped.
  - `Trend` returns `nil` for <2 samples; for a strictly increasing memory series it returns `slope_bytes_per_hour > 0` and `points` length ≤ 20 with `first_bytes < last_bytes`.
  - Persistence round-trip: `Save()` then a fresh `NewMemHistory(sameDir)` reloads the same samples (compare counts and last sample). Corrupt-file tolerance: write garbage to `mem_history.json`, construct `NewMemHistory` — it must not panic and must start empty.

  Verification: `export PATH=$PATH:/usr/local/go/bin && cd /home/chris/code/docker-cc/backend && go test ./handlers/...` — all new tests pass and no existing handler test regresses.
