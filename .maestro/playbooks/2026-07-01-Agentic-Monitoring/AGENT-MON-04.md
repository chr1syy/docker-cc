# Agentic Monitoring — Phase 4: CI, version bump, and verification

**Why this phase exists.** Ties off the backlog item "docker-cc agentic api — build image for agentic API, add CI build & test." The agentic API (the `/api/agent/snapshot` endpoint + memory-history buffer) ships **inside the existing docker-cc image** — no separate image is required. docker-cc's job ends at *exposing* the data; all scheduling, judging, alerting, and remediation live in a separate future Maestro Cue pipeline (see the folder README) and are NOT part of this playbook. So there are no shell scripts to lint and no auto-deploy step here — this phase only ensures CI covers the new Go code, bumps the version, and lays out a manual smoke test that Chris runs before deploying.

**CI today** (`.github/workflows/ci.yml`): on `v*` tags — `test-backend` runs `go test ./...` (already covers the new `snapshot`/`memhistory`/`hostinfo` tests automatically), `test-frontend` lints + type-checks, then `build-and-push` builds and pushes the image to GHCR. No CI changes are required for the new endpoint beyond confirming the existing Go test job exercises it. **No auto-deploy** — Chris deploys manually after review, pulling the same image as today.

**Versioning** (per project convention, see memory `feedback_version_bumps`): a NEW endpoint + new env/data behavior is *new functionality* → **minor** bump (0.6.1 → **0.7.0**), not a patch.

## Prerequisites
- `export PATH=$PATH:/usr/local/go/bin` before any `go` command.

---

- [x] **Confirm CI already gates the new backend code (no workflow edits needed).**

  Inspect `.github/workflows/ci.yml` and confirm the `test-backend` job runs `go test ./...` (or `go test ./backend/...`) across the whole module, so the new `snapshot`, `memhistory`, and `hostinfo` tests are picked up automatically with no new job. Confirm `build-and-push` still `needs:` the test jobs so a failing test blocks a release. Do NOT add a `shellcheck`/`lint-scripts` job — there are no shell scripts in this playbook. Do NOT add any deploy step; the existing build-and-push-to-GHCR is the end of CI, and deployment stays a manual human step.

  Verification: `grep -n "go test" .github/workflows/ci.yml` shows the module-wide test invocation; `grep -n "shellcheck\|lint-scripts\|deploy" .github/workflows/ci.yml` returns NO new agentic-monitoring matches (only pre-existing lines, if any).

  **Done (2026-07-01):** Confirmed — no workflow edits made or needed.
  - `test-backend` runs `go test ./...` at `ci.yml:24` with `working-directory: backend`, so the whole backend Go module is tested. New `snapshot`/`memhistory`/`hostinfo` test files are picked up automatically; no new job added.
  - `grep -n "shellcheck\|lint-scripts\|deploy"` returns **zero** matches — no shell-script lint job, no auto-deploy step. Deployment stays a manual human step.
  - `build-and-push` at `ci.yml:46` still `needs: [test-backend, test-frontend]`, so a failing test blocks the GHCR release.

- [ ] **Confirm the snapshot endpoint is present in a built binary and the full backend test suite is green.**

  - `export PATH=$PATH:/usr/local/go/bin && cd /home/chris/code/docker-cc/backend && go build -o /tmp/dcc-server . && go test ./...` — build and ALL tests pass (snapshot, memhistory, hostinfo, plus existing).
  - Sanity: start `/tmp/dcc-server` with `ADMIN_PASSWORD=x SESSION_SECRET=y API_TOKEN=t STATIC_DIR=/tmp` (no Docker needed for the disconnected path), then `curl -s -H "Authorization: Bearer t" localhost:8080/api/agent/snapshot` returns 200 JSON containing `"docker"` and `"counts"`; `curl -s localhost:8080/api/agent/snapshot` (no token) returns 401. Stop the server.

  Verification: both curl assertions hold; test suite exits 0.

- [ ] **Bump the version to 0.7.0 across the repo.**

  Update the version wherever it is declared for a release (grep for `0.6.1`: at minimum `frontend/package.json`, and any `Version` default / VERSION references — do NOT change the CI `-ldflags` injection mechanism, only version strings/manifests). Update `CLAUDE.md`'s "Current version:" line to `0.7.0`. Do NOT create the git tag or push in this task (release is a human-triggered step below).

  Verification: `grep -rn "0.7.0" frontend/package.json CLAUDE.md` shows the bump; `grep -rn "0.6.1" .` returns no stale release-version references (ignore this playbook folder and changelog history).

## Human-only verification (run after the playbook completes — NOT agent checkbox tasks)

- Review the diff, then deploy the 0.7.0 image to prod manually: `git tag v0.7.0 && git push --tags`, wait for CI/GHCR, then `docker compose pull && docker compose up -d` on the box (same image and flow as today — no automated deploy).
- Confirm `docker-cc` restarts cleanly and `DATA_DIR` contains `mem_history.json` after ~a few minutes.
- Ensure `API_TOKEN` is set in the prod `app.env`, then hit the endpoint from the box and confirm real fleet data:
  `curl -s -H "Authorization: Bearer $API_TOKEN" localhost:8080/api/agent/snapshot | jq '.counts, .host'`.
- (Later, separately) stand up the Maestro Cue pipeline that consumes this endpoint — that is a distinct effort outside docker-cc and outside this playbook.
