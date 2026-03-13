# Docker CC - Agent Instructions

## Project Overview

Docker CC is a lightweight Docker container dashboard with a Go backend and SvelteKit frontend. It provides container listing, inspection, real-time metrics, log viewing, and container control actions behind single-user authentication.

## Tech Stack

- **Backend:** Go 1.24+ with chi router, Docker SDK v25, gorilla/websocket
- **Frontend:** SvelteKit 2, Svelte 4, TypeScript, adapter-static (SPA mode)
- **Deployment:** Multi-stage Dockerfile, Docker Compose

## Project Structure

```
backend/
  main.go              # Server entrypoint, middleware, routes, graceful shutdown
  auth/
    auth.go            # Session management, auth middleware, CSRF, security headers
    handlers.go        # Login/logout/check HTTP handlers
  docker/
    client.go          # Docker SDK wrapper (list, inspect, start, stop, restart, logs)
    client_test.go     # Integration tests (build tag: integration)
    stats.go           # Stats parsing, concurrent stats fetching
  handlers/
    containers.go      # Container list/inspect/action handlers
    logs.go            # Log fetch and WebSocket streaming
    stats.go           # Stats one-shot and WebSocket streaming
    response.go        # JSON response helpers

frontend/
  src/
    lib/
      api.ts           # Typed API client (fetch wrapper)
      types.ts         # TypeScript interfaces (Container, ContainerDetail, Port)
      stores/
        auth.ts        # Auth store (login, logout, check)
        stats.ts       # WebSocket stats store (auto-reconnect)
        toast.ts       # Toast notification store
      components/
        ActionButton.svelte  # Start/stop/restart with confirmation
        ErrorState.svelte    # Error display with retry
        LogViewer.svelte     # Virtual-scrolled log viewer with search, filters, live streaming
        MetricChart.svelte   # Simple metric chart component
        MobileNav.svelte     # Mobile hamburger navigation
        Skeleton.svelte      # Loading skeleton placeholders
        Toast.svelte         # Toast notification display
      styles/
        global.css     # Global styles, CSS variables, dark theme
    routes/
      +layout.svelte   # App layout with sidebar, auth gate
      +layout.ts       # SSR disabled (SPA)
      +page.svelte     # Dashboard (container list with live stats)
      login/+page.svelte        # Login page
      logs/+page.svelte         # Dedicated log viewer page
      container/[id]/+page.svelte  # Container detail page
```

## Development Conventions

### Backend (Go)

- Use `chi` for routing; group API routes under `/api`
- Protected routes use `sm.AuthMiddleware`
- Container actions require `handlers.RequireActions` middleware (checks `ALLOW_ACTIONS` env)
- Docker client methods return SDK types; handlers transform to API responses
- Use context timeouts for all Docker operations (5s for list, 10s for inspect/actions)
- Integration tests use `//go:build integration` tag
- Run backend: `cd backend && go run .`
- Build: `go build -o server .`
- Test: `go test ./...` (unit), `go test -tags=integration ./...` (integration, needs Docker)

### Frontend (SvelteKit)

- SPA mode via `adapter-static` with `fallback: 'index.html'`
- TypeScript in all `.svelte` files via `<script lang="ts">` and `vitePreprocess()`
- Stores in `src/lib/stores/` using Svelte writable stores
- API calls go through `src/lib/api.ts` (centralized fetch wrapper)
- Components in `src/lib/components/`
- Avoid `catch(e: any)` in Svelte templates - use `catch(e)` with type assertion
- Avoid duplicate `<script>` or `<style>` blocks in components
- Use `$lib/` import alias for lib directory
- Run frontend: `cd frontend && npm run dev`
- Build: `npm run build` (outputs to `frontend/build/`)
- Lint: `npm run lint`
- Format: `npm run format`
- Type check: `npm run check`

### Environment Variables

Required for running:
- `ADMIN_PASSWORD` - plaintext admin password (hashed at startup)
- `SESSION_SECRET` - random string for session signing

Optional:
- `ADMIN_PASSWORD_HASH` - bcrypt hash (alternative to `ADMIN_PASSWORD`)
- `ADMIN_USER` (default: `admin`)
- `ALLOW_ACTIONS` (default: `false`)
- `SESSION_TTL` (default: `24h`)
- `STATIC_DIR` (default: `./static`)
- `DATA_DIR` (default: `./data`) - persistent data dir for 2FA secrets etc.

## Common Tasks

### Adding a new API endpoint

1. Add the handler method in `backend/handlers/`
2. Register the route in `backend/main.go` under the `/api` group
3. Add the API function in `frontend/src/lib/api.ts`
4. Use from components or pages

### Adding a new frontend page

1. Create `frontend/src/routes/your-page/+page.svelte`
2. Add navigation link in `+layout.svelte` sidebar and `MobileNav.svelte`

### Running tests

```sh
# Backend unit tests
cd backend && go test ./...

# Backend integration tests (requires Docker)
cd backend && go test -tags=integration ./...

# Frontend type checking
cd frontend && npm run check

# Frontend linting
cd frontend && npm run lint
```
