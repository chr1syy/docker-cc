# Docker CC

A lightweight Docker container dashboard with a Go backend and SvelteKit frontend. Monitor, inspect, and control your local Docker containers through a clean web interface.

## Features

- **Container Dashboard** - List all containers with status, image, ports, and resource usage at a glance
- **Container Detail View** - Deep inspection of individual containers including config, network, mounts, labels, and environment variables
- **Real-Time Metrics** - Live CPU, memory, network, and block I/O stats streamed via WebSocket
- **Log Viewer** - Searchable, filterable container logs with live streaming, time range selection, and virtual scrolling
- **Container Actions** - Start, stop, and restart containers directly from the UI (opt-in via `ALLOW_ACTIONS=true`)
- **Single-User Auth** - Session-based authentication with bcrypt password hashing, CSRF protection, and security headers
- **Responsive Design** - Works on desktop and mobile with a collapsible sidebar and mobile navigation

## Quick Start

### Docker (recommended)

1. Create an `app.env` file from the example:

```sh
cp app.env.example app.env
```

2. Edit `app.env` with your credentials:

```env
ADMIN_USER=admin
ADMIN_PASSWORD=your-secure-password
SESSION_SECRET=your-random-string  # generate with: openssl rand -hex 32
ALLOW_ACTIONS=false
```

3. Start the container:

```sh
docker compose up -d
```

4. Open `http://localhost:9090` and log in.

### Development

**Prerequisites:** Go 1.24+, Node.js 22+

```sh
# Backend
cd backend && go run .

# Frontend (separate terminal)
cd frontend && npm install && npm run dev
```

The frontend dev server proxies `/api` requests to `http://localhost:8080`.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `ADMIN_USER` | `admin` | Admin username |
| `ADMIN_PASSWORD` | *(required)* | Admin password (hashed at startup) |
| `ADMIN_PASSWORD_HASH` | | Bcrypt hash (alternative to `ADMIN_PASSWORD`, avoids `$` escaping issues in Compose) |
| `SESSION_SECRET` | *(required)* | Random string for signing session cookies |
| `SESSION_TTL` | `24h` | Session inactivity timeout (duration string or seconds) |
| `ALLOW_ACTIONS` | `false` | Enable start/stop/restart container actions |
| `STATIC_DIR` | `./static` | Directory for built frontend assets |

## Architecture

```
docker-cc/
  backend/           # Go HTTP server (chi router)
    auth/            # Session management, security middleware
    docker/          # Docker SDK client, stats parsing, log parsing
    handlers/        # HTTP handlers for containers, logs, stats
    main.go          # Server entrypoint with graceful shutdown
  frontend/          # SvelteKit SPA (adapter-static)
    src/lib/         # API client, stores, components
    src/routes/      # Pages (dashboard, container detail, logs, login)
  Dockerfile         # Multi-stage build (node + go + alpine runtime)
  docker-compose.yml # Production deployment config
```

## API Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/api/health` | No | Health check with Docker connectivity status |
| `POST` | `/api/login` | No | Authenticate with username/password |
| `POST` | `/api/logout` | No | Destroy session |
| `GET` | `/api/auth/check` | No | Check session validity |
| `GET` | `/api/containers` | Yes | List all containers |
| `GET` | `/api/containers/{id}` | Yes | Inspect a container |
| `GET` | `/api/containers/{id}/logs` | Yes | Fetch container logs |
| `GET` | `/api/containers/{id}/logs/stream` | Yes | WebSocket log streaming |
| `GET` | `/api/containers/{id}/stats` | Yes | One-shot container stats |
| `GET` | `/api/stats/stream` | Yes | WebSocket stats for all running containers |
| `POST` | `/api/containers/{id}/start` | Yes | Start a container (requires `ALLOW_ACTIONS=true`) |
| `POST` | `/api/containers/{id}/stop` | Yes | Stop a container (requires `ALLOW_ACTIONS=true`) |
| `POST` | `/api/containers/{id}/restart` | Yes | Restart a container (requires `ALLOW_ACTIONS=true`) |

## Security

- Passwords are stored as bcrypt hashes (never plaintext)
- Sessions use cryptographically random 256-bit tokens
- CSRF protection via Origin/Referer header validation on state-changing requests
- Security headers: `X-Content-Type-Options`, `X-Frame-Options`, `X-XSS-Protection`, `Referrer-Policy`
- Request body size limited to 1MB
- Environment variables are redacted in container inspection responses
- Docker socket access required for container management

## Agent / Programmatic Access

Docker CC exposes a small read-only API for external agents (Claude, cron jobs, uptime monitors, custom scripts). Enable it by setting `API_TOKEN`:

    API_TOKEN=$(openssl rand -hex 32)

Then call the API with a bearer header:

    curl -H "Authorization: Bearer $API_TOKEN" https://your-host/api/status

### Endpoints

- `GET /api/agent/snapshot` — one-call agent snapshot: per-container health/stats/mem-trend + recent errors + host mem/load (see [Agentic Monitoring](#agentic-monitoring)).
- `GET /api/status` — one-call health summary (container counts, recent log errors, healthy flag). Use this for morning checks.
- `GET /api/logs/digest?since=24h` — aggregated error/warn scan across all running containers, bounded to 256 KB.
- `GET /api/containers/{id}/logs/digest?since=24h` — drill-down on a single container with more samples and configurable regex patterns.

### Security Notes

- The token grants read-only access. Container start/stop/restart/remove are rejected even when `ALLOW_ACTIONS=true`.
- Rotate the token periodically. To invalidate, change `API_TOKEN` and restart the container.
- Always serve over HTTPS (the token is sent in cleartext in the `Authorization` header).
- Bearer auth is OFF by default — leaving `API_TOKEN` unset preserves session+2FA-only access.

### Suggested Agent Workflow

1. Morning poll: `GET /api/status` → check `healthy` field.
2. If unhealthy: read the `issues` array. Each issue has a `container` name and `kind` (`log_errors`, `stopped`, `unhealthy`, `restarting`).
3. For log issues, drill down: `GET /api/containers/<name>/logs/digest?since=24h&limit=100`.
4. Summarize and report.

## Agentic Monitoring

`GET /api/agent/snapshot` returns a single LLM-friendly document bundling everything a health-judging agent needs in one call — per-container state/uptime/restarts, current CPU/memory from the live buffer, a 24 h memory-leak trend, recent error log lines, and host memory/load. It replaces stitching together `/api/status`, `/api/stats/history`, and `/api/logs/digest`.

    curl -H "Authorization: Bearer $API_TOKEN" https://your-host/api/agent/snapshot

Optional `?window=<duration>` sets the recent-error scan window (default `1h`, clamped to `[1m, 24h]`). The endpoint is read-only and always returns HTTP 200 when reachable — if the Docker socket is down it still responds with `docker: "disconnected"`, host metrics, and an empty container list rather than failing.

Abridged response:

```json
{
  "checked_at": "2026-07-01T15:04:05Z",
  "docker": "connected",
  "version": "0.6.1",
  "window": "1h",
  "host": { "load1": 0.42, "mem_total_bytes": 8319737856, "mem_used_percent": 61.3 },
  "counts": { "total": 11, "running": 10, "stopped": 1, "unhealthy": 0, "restarting": 0 },
  "containers": [
    {
      "name": "web", "state": "running", "health": "healthy",
      "uptime_seconds": 86400, "restart_count": 0,
      "cpu_percent": 2.1, "mem_usage_bytes": 134217728, "mem_percent": 3.2,
      "mem_trend": { "window": "24h", "samples": 1440, "slope_bytes_per_hour": 1048576 },
      "recent_errors": { "count": 0, "samples": [] }
    }
  ]
}
```

`mem_trend` is `null` until a container has accumulated at least two long-horizon samples (sampled 1/min), so trends only become meaningful after roughly an hour of uptime.

docker-cc only *exposes* this data — scheduling the polls, judging health, alerting, and any remediation are intended to live in a separate consumer (e.g. a Maestro Cue pipeline) that reads this endpoint, not inside docker-cc.

## License

MIT
