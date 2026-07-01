# Agentic Monitoring — Phase 3: Snapshot tests + documentation

**Why this phase exists.** The snapshot endpoint is docker-cc's contract with any external consumer — including the future Maestro Cue pipeline that will poll it, judge health, and intervene (that pipeline is out of scope here; see the folder README). It must be tested for shape stability so future changes don't silently break that consumer, and documented so operators know what the endpoint returns. Tests here also make the endpoint part of the CI gate (`go test ./...` already runs in `.github/workflows/ci.yml`).

## Prerequisites
- `export PATH=$PATH:/usr/local/go/bin` before any `go` command.

---

- [x] **Add `backend/handlers/snapshot_test.go` covering the disconnected path and response shape.**

  > **Done (2026-07-01).** Added `backend/handlers/snapshot_test.go` with the four tests: `TestSnapshot_NilClient_Disconnected` (200, `docker=disconnected`, `counts.total=0`, `host` key present), `TestSnapshot_InvalidWindow_400`, `TestSnapshot_ResponseShape` (all seven top-level keys + version round-trip), and `TestSnapshot_MemTrendNullWhenSparse` (single sample → `Trend` nil → `"mem_trend":null` on serialization, not a zero struct). Guarded the 2 s collector against a nil Docker client: added an early `if s.dclient == nil { return }` at the top of `StatsHandler.collect()` in `backend/handlers/stats.go`, so constructing a `StatsHandler` over a nil client in tests spins no panicking goroutine. Verified: `go test ./handlers/...` → 42 tests pass (up from 34), no goroutine leak/panic.

  Follow the style of `backend/handlers/status_test.go`. Cover:
  - `TestSnapshot_NilClient_Disconnected` — construct `NewSnapshotHandler(nil, "test", NewLogHandler(nil), <stats handler with a nil docker client but a real MemHistory over t.TempDir()>, mh)`; assert HTTP 200, `docker == "disconnected"`, `counts.total == 0`, and that `host` is still populated OR explicitly `null` (assert it does not panic and the key is present).
  - `TestSnapshot_InvalidWindow_400` — `?window=not-a-duration` returns 400.
  - `TestSnapshot_ResponseShape` — assert `Content-Type: application/json`, all top-level keys present (`checked_at`, `docker`, `version`, `window`, `host`, `counts`, `containers`), and the injected `version` round-trips.
  - `TestSnapshot_MemTrendNullWhenSparse` — with a `MemHistory` holding a single sample for a name, `Trend` (and thus the serialized `mem_trend`) is `null`, not a zero-value struct.

  If constructing `StatsHandler` in a test spins the 2 s collector goroutine against a nil Docker client, guard the collector so a nil `dclient` makes `collect()` return immediately (add an early `if s.dclient == nil { return }` at the top of `collect()`), and note that change here. Verify no goroutine leak/panic in tests.

  Verification: `export PATH=$PATH:/usr/local/go/bin && cd /home/chris/code/docker-cc/backend && go test ./handlers/...` — all snapshot + memhistory + existing handler tests pass.

- [ ] **Document the endpoint and its env in `CLAUDE.md` (symlinked to `AGENTS.md`).**

  - In the "Protected (requires auth session)" endpoint table, add at the top (it is the marquee agent endpoint):

    ```
    | GET | `/api/agent/snapshot` | One-call agent snapshot: per-container health/stats/mem-trend + recent errors + host mem/load |
    ```
  - In the architecture ASCII diagram's route list, add a `/api/agent/snapshot   One-call agentic monitoring digest` line near `/api/status`.
  - Under "Backend (Go)" development conventions, add one bullet: long-horizon memory history is sampled at 1/min for 24 h and persisted to `DATA_DIR/mem_history.json` (atomic write, loaded on startup, flushed every 5 min and on shutdown) to make memory-leak trends detectable from a single `/api/agent/snapshot` call.
  - Confirm the bearer-auth note already present (agent API accepts `Authorization: Bearer <API_TOKEN>`, read-only) also lists `/api/agent/snapshot`.

  Verification: `grep -n "agent/snapshot" /home/chris/code/docker-cc/AGENTS.md` shows the new rows.

- [ ] **Add a "Agentic monitoring" section to `README.md` describing the snapshot endpoint and how the pipeline consumes it.**

  Add a concise section documenting: the endpoint path + auth (`Authorization: Bearer $API_TOKEN`), an abridged example response, the `window` query param, and that memory-trend fields require ~an hour of uptime to be meaningful. Add one sentence noting that docker-cc only *exposes* this data — scheduling, health-judging, alerting, and any remediation are intended to live in a separate consumer (a Maestro Cue pipeline) that polls this endpoint, not inside docker-cc. If `README.md` has no such top-level anchor yet, place the section after the existing API/usage documentation. Keep it under ~40 lines.

  Verification: `grep -n "agent/snapshot" /home/chris/code/docker-cc/README.md` shows the section.
