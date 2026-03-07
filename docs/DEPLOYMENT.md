# Deployment Guide

## Docker Compose (recommended)

The simplest way to deploy Docker CC is with Docker Compose. The included `docker-compose.yml` builds a multi-stage image containing both the Go backend and the pre-built SvelteKit frontend.

### Prerequisites

- Docker Engine 20.10+
- Docker Compose v2

### Steps

1. **Clone the repository:**

```sh
git clone https://github.com/chr1syy/docker-cc.git
cd docker-cc
```

2. **Configure environment:**

```sh
cp .env.example .env
```

Edit `.env` and set the required values:

```env
ADMIN_USER=admin
ADMIN_PASSWORD_HASH=<bcrypt hash>
SESSION_SECRET=<random string>
ALLOW_ACTIONS=false
```

Generate a bcrypt hash for your password:

```sh
# Python
python3 -c "import bcrypt; print(bcrypt.hashpw(b'YOUR_PASSWORD', bcrypt.gensalt()).decode())"

# htpasswd (Apache utils)
htpasswd -nbBC 10 "" YOUR_PASSWORD | cut -d: -f2

# Node.js
node -e "const b=require('bcryptjs');console.log(b.hashSync('YOUR_PASSWORD',10))"
```

Generate a random session secret:

```sh
openssl rand -hex 32
```

3. **Build and start:**

```sh
docker compose up -d
```

4. **Access the dashboard** at `http://localhost:9090`

### Customizing the Port

Edit `docker-compose.yml` to change the published port:

```yaml
ports:
  - "3000:8080"  # Change 3000 to your preferred port
```

### Enabling Container Actions

To allow starting, stopping, and restarting containers from the UI:

```env
ALLOW_ACTIONS=true
```

This is disabled by default as a safety measure.

### Health Check

The built-in health check verifies both the HTTP server and Docker daemon connectivity:

```sh
curl http://localhost:9090/api/health
# {"status":"ok","docker":"connected"}
```

---

## Reverse Proxy

### Nginx

```nginx
server {
    listen 443 ssl;
    server_name docker.example.com;

    ssl_certificate     /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:9090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
    }

    # WebSocket support for live logs and stats
    location ~ ^/api/(containers/.*/logs/stream|stats/stream) {
        proxy_pass http://127.0.0.1:9090;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Host $host;
        proxy_read_timeout 86400;
    }
}
```

### Caddy

```
docker.example.com {
    reverse_proxy localhost:9090
}
```

Caddy handles WebSocket upgrades and TLS automatically.

### Traefik (Docker labels)

```yaml
services:
  docker-cc:
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.docker-cc.rule=Host(`docker.example.com`)"
      - "traefik.http.routers.docker-cc.tls.certresolver=letsencrypt"
      - "traefik.http.services.docker-cc.loadbalancer.server.port=8080"
```

---

## Manual Build (without Docker)

### Prerequisites

- Go 1.24+
- Node.js 22+

### Build Steps

```sh
# Build frontend
cd frontend
npm ci
npm run build
cd ..

# Build backend
cd backend
go build -o ../docker-cc-server .
cd ..

# Copy frontend build to static directory
mkdir -p static
cp -r frontend/build/* static/

# Run
ADMIN_USER=admin \
ADMIN_PASSWORD_HASH='$2b$10$...' \
SESSION_SECRET='your-secret' \
STATIC_DIR=./static \
./docker-cc-server
```

The server starts on port 8080 by default.

---

## Security Considerations

- **Docker socket access:** The container needs access to the Docker socket (`/var/run/docker.sock`). This grants significant privileges. Mount it read-only (`:ro`) as shown in the default config.
- **Network exposure:** Do not expose Docker CC directly to the internet without a reverse proxy with TLS.
- **Password strength:** Use a strong admin password. The bcrypt hash cost is configurable via the hash generation tool.
- **Session secret:** Use a cryptographically random string of at least 32 characters.
- **Container actions:** Keep `ALLOW_ACTIONS=false` (default) unless you need to control containers from the UI.
