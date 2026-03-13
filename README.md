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

1. Generate a bcrypt password hash:

```sh
# Using htpasswd
htpasswd -nbBC 10 "" your-password | cut -d: -f2

# Or using Python
python3 -c "import bcrypt; print(bcrypt.hashpw(b'your-password', bcrypt.gensalt()).decode())"
```

2. Set your environment variables in `docker-compose.yml` under `environment:`. Bcrypt hashes contain `$` signs, so you must double them (`$$`) in the compose file:

```yaml
environment:
  - ADMIN_PASSWORD_HASH=$$2a$$10$$your-hash-here
  - SESSION_SECRET=some-random-string-here
```

3. Start the container:

```sh
docker compose up -d
```

5. Open `http://localhost:9090` and log in.

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
| `ADMIN_PASSWORD_HASH` | *(required)* | Bcrypt hash of the admin password |
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

## License

MIT
