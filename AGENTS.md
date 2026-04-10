# Docker CC - Agent Instructions

## Project Overview

Docker CC is a lightweight, self-hosted Docker container dashboard with a Go backend and SvelteKit frontend. It provides real-time container monitoring, log viewing, and container lifecycle management behind single-user authentication with optional 2FA (TOTP).

**Repository:** `ghcr.io/chr1syy/docker-cc`
**Current version:** 0.1.0
**License:** MIT

## Tech Stack

- **Backend:** Go 1.24+ with chi router, Docker SDK v25, gorilla/websocket, bcrypt auth
- **Frontend:** SvelteKit 2, Svelte 4, TypeScript, adapter-static (SPA mode)
- **Deployment:** Multi-stage Dockerfile, Docker Compose, GHCR
- **CI/CD:** GitHub Actions — tests + build on version tags (`v*`)

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Browser (SPA)                                  │
│  SvelteKit static build served by Go backend    │
└──────────────┬──────────────────────────────────┘
               │ HTTP/WS on :8080
┌──────────────▼──────────────────────────────────┐
│  Go Backend (chi router)                        │
│  ├─ /api/auth/*        Auth + 2FA endpoints     │
│  ├─ /api/containers/*  CRUD + actions           │
│  ├─ /api/stats/stream  WebSocket live stats     │
│  ├─ /api/stats/history Buffered stats history   │
│  ├─ /api/*/logs/stream WebSocket log streaming  │
│  └─ /*                 Static file server (SPA) │
└──────────────┬──────────────────────────────────┘
               │ Docker SDK via unix socket
┌──────────────▼──────────────────────────────────┐
│  Docker Engine (/var/run/docker.sock)           │
└─────────────────────────────────────────────────┘
```

## Project Structure

```
backend/
  main.go                # Server entrypoint, middleware, routes, graceful shutdown
  auth/
    auth.go              # Session management, auth middleware, CSRF, security headers
    handlers.go          # Login/logout/check HTTP handlers, 2FA setup/verify/disable
    totp.go              # TOTP manager with encrypted storage and lockout
  docker/
    client.go            # Docker SDK wrapper (list, inspect, start, stop, restart, remove, logs, ping)
    client_test.go       # Integration tests (build tag: integration)
    client_unit_test.go  # Unit tests
    stats.go             # Stats parsing, concurrent stats fetching
  handlers/
    containers.go        # Container list/inspect/action handlers (start, stop, restart, remove)
    logs.go              # Log fetch and WebSocket streaming
    stats.go             # Stats one-shot, WebSocket streaming, history endpoint, background collector
    statsbuffer.go       # Thread-safe per-container ring buffer for metrics history (150 points / ~5 min)
    response.go          # JSON response helpers

frontend/
  src/
    lib/
      api.ts             # Typed API client (fetch wrapper with auth)
      types.ts           # TypeScript interfaces (Container, ContainerDetail, Port, NetworkInfo)
      stores/
        auth.ts          # Auth store (login, logout, check)
        stats.ts         # WebSocket stats store (auto-reconnect, 150-point history, hydration from backend)
        toast.ts         # Toast notification store
      components/
        ActionButton.svelte  # Start/stop/restart/remove with confirmation
        ErrorState.svelte    # Error display with retry
        LogViewer.svelte     # Virtual-scrolled log viewer with search, filters, live streaming
        MetricChart.svelte   # Metric chart component
        MobileNav.svelte     # Mobile hamburger navigation
        Skeleton.svelte      # Loading skeleton placeholders
        Sparkline.svelte     # Inline sparkline chart for dashboard
        Toast.svelte         # Toast notification display
      styles/
        global.css       # Global styles, CSS variables, dark theme
    routes/
      +layout.svelte     # App layout with sidebar, auth gate, navigation
      +layout.ts         # SSR disabled (SPA)
      +page.svelte       # Dashboard (container list with live stats + sparklines)
      login/+page.svelte            # Login page
      logs/+page.svelte             # Dedicated log viewer page
      settings/+page.svelte         # Settings page (2FA setup)
      container/[id]/+page.svelte   # Container detail page (inspect, metrics, logs)
```

## API Endpoints

### Public
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Health check with Docker connectivity + version |
| GET | `/api/version` | Build version |
| POST | `/api/login` | Username/password login |
| POST | `/api/auth/totp/verify` | TOTP verification (2FA second step) |
| POST | `/api/logout` | Session logout |
| GET | `/api/auth/check` | Session validity check |

### Protected (requires auth session)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/containers` | List all containers |
| GET | `/api/containers/{id}` | Inspect container |
| GET | `/api/containers/{id}/logs` | Fetch container logs |
| GET | `/api/containers/{id}/logs/stream` | WebSocket log streaming |
| GET | `/api/containers/{id}/stats` | One-shot container stats |
| GET | `/api/stats/stream` | WebSocket live stats for all containers |
| GET | `/api/stats/history` | Buffered stats history (~5 min ring buffer) |

### Protected + Actions enabled (`ALLOW_ACTIONS=true`)
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/containers/{id}/start` | Start container |
| POST | `/api/containers/{id}/stop` | Stop container |
| POST | `/api/containers/{id}/restart` | Restart container |
| DELETE | `/api/containers/{id}` | Remove stopped container |

### Protected (2FA management)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/auth/2fa/status` | Check 2FA enrollment status |
| POST | `/api/auth/2fa/setup` | Generate TOTP secret + QR URI |
| POST | `/api/auth/2fa/confirm` | Confirm 2FA setup with code |
| POST | `/api/auth/2fa/disable` | Disable 2FA |

## Development Conventions

### Backend (Go)

- Use `chi` for routing; group API routes under `/api`
- Protected routes use `sm.AuthMiddleware`
- Container actions require `handlers.RequireActions` middleware (checks `ALLOW_ACTIONS` env)
- Docker client methods return SDK types; handlers transform to API responses
- Use context timeouts for all Docker operations (5s for list, 10s for inspect/actions)
- Integration tests use `//go:build integration` tag
- Structured JSON logging middleware for all requests
- 1MB request body size limit on all API endpoints
- Security headers and origin checking on all API routes
- Version injected at build time via `-ldflags "-X main.Version=..."`
- Stats are collected by a background goroutine every 2s from server start, stored in an in-memory ring buffer (150 points per container, ~5 min). Frontend hydrates from `/api/stats/history` on auth, then continues via WebSocket for real-time updates.

### Frontend (SvelteKit)

- SPA mode via `adapter-static` with `fallback: 'index.html'`
- TypeScript in all `.svelte` files via `<script lang="ts">` and `vitePreprocess()`
- Stores in `src/lib/stores/` using Svelte writable stores
- API calls go through `src/lib/api.ts` (centralized fetch wrapper)
- Components in `src/lib/components/`
- Avoid `catch(e: any)` in Svelte templates — use `catch(e)` with type assertion
- Avoid duplicate `<script>` or `<style>` blocks in components
- Use `$lib/` import alias for lib directory
- Dev server proxies `/api` and WebSocket to backend via Vite config

### Security Model

- Single-user auth: one admin user configured via environment variables
- Session-based authentication with signed cookies
- CSRF protection via origin checking middleware
- Optional TOTP 2FA with encrypted secret storage on disk (`DATA_DIR`)
- Container actions gated behind `ALLOW_ACTIONS=true` (off by default)
- bcrypt password hashing (plaintext `ADMIN_PASSWORD` hashed at startup)
- Security headers on all API responses

## Environment Variables

### Required
| Variable | Description |
|----------|-------------|
| `ADMIN_PASSWORD` | Plaintext admin password (hashed at startup via bcrypt) |
| `SESSION_SECRET` | Random string for session cookie signing |

### Optional
| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_PASSWORD_HASH` | — | Pre-hashed bcrypt password (alternative to `ADMIN_PASSWORD`) |
| `ADMIN_USER` | `admin` | Admin username |
| `ALLOW_ACTIONS` | `false` | Enable container start/stop/restart/remove actions |
| `SESSION_TTL` | `24h` | Session duration |
| `STATIC_DIR` | `./static` | Path to frontend static build |
| `DATA_DIR` | `./data` | Persistent data directory (2FA secrets, etc.) |

## Development Commands

```sh
# Backend
cd backend && go run .                         # Run dev server
cd backend && go build -o server .             # Build binary
cd backend && go test ./...                    # Unit tests
cd backend && go test -tags=integration ./...  # Integration tests (needs Docker)

# Frontend
cd frontend && npm run dev       # Dev server with HMR
cd frontend && npm run build     # Production build → frontend/build/
cd frontend && npm run check     # TypeScript type checking
cd frontend && npm run lint      # ESLint
cd frontend && npm run format    # Prettier

# Docker
docker compose up -d             # Run from GHCR image
docker compose down              # Stop

# Make shortcuts
make dev-backend                 # go run
make dev-frontend                # npm run dev
make test                        # backend unit tests
make test-integration            # backend integration tests
make lint                        # frontend lint
make check                       # frontend type check
make build                       # docker compose build
make up / make down              # docker compose up/down
```

## Common Tasks

### Adding a new API endpoint

1. Add handler method in `backend/handlers/` (or `backend/auth/` for auth-related)
2. Register the route in `backend/main.go` under the `/api` group
3. Add the API function in `frontend/src/lib/api.ts`
4. Use from components or pages

### Adding a new frontend page

1. Create `frontend/src/routes/your-page/+page.svelte`
2. Add navigation link in `+layout.svelte` sidebar and `MobileNav.svelte`

### Adding a new container action

1. Add method to `backend/docker/client.go`
2. Add handler in `backend/handlers/containers.go`
3. Register route with `handlers.RequireActions` middleware in `main.go`
4. Add API function in `frontend/src/lib/api.ts`
5. Wire up in `ActionButton.svelte` or relevant UI component

### Deployment

- Push a version tag (`git tag v0.2.0 && git push --tags`) to trigger CI
- CI runs backend + frontend tests, then builds and pushes to GHCR
- Image tagged as both `:<version>` and `:latest`
- Deploy via `docker-compose.yml` with `app.env` for secrets
