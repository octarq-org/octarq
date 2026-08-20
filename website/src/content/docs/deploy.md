---
title: Deploying Octarq
description: Production deployment guide for Octarq using Docker, binary releases, or cloud providers with SQLite or PostgreSQL.
sidebar:
  order: 5
  group:
    label: "Start"
---

Octarq is a single Go binary with an embedded frontend dashboard and support for both embedded SQLite and external PostgreSQL databases.

## 1. Docker Compose Deployments

### Option A: Standard Deployment (SQLite)

The fastest way to deploy Octarq for single-node setups:

```yaml
version: '3.8'

services:
  octarq:
    image: ghcr.io/octarq-org/octarq:latest
    container_name: octarq
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - OCTARQ_SECRET_KEY=change-this-to-a-random-32-byte-string
      - OCTARQ_ADMIN_PASSWORD=change-this-admin-password
      - OCTARQ_LISTEN=:8080
      - OCTARQ_DB_DRIVER=sqlite
    volumes:
      - ./data:/data
```

### Option B: Scalable Production Deployment (PostgreSQL + Redis)

For enterprise high availability, clustered setups, and high-concurrency workloads:

```yaml
version: '3.8'

services:
  octarq:
    image: ghcr.io/octarq-org/octarq:latest
    container_name: octarq
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    ports:
      - "8080:8080"
    environment:
      - OCTARQ_SECRET_KEY=change-this-to-a-random-32-byte-string
      - OCTARQ_ADMIN_PASSWORD=change-this-admin-password
      - OCTARQ_LISTEN=:8080
      - OCTARQ_DB_DRIVER=postgres
      - OCTARQ_DB_DSN=postgres://octarq:securepassword@postgres:5432/octarq?sslmode=disable
      - OCTARQ_REDIS_URL=redis://redis:6379

  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: octarq
      POSTGRES_PASSWORD: securepassword
      POSTGRES_DB: octarq
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U octarq -d octarq"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pgdata:
```

Start the stack:

```bash
docker compose up -d
```

---

## 2. Standalone Binary Deployment

Build a production binary with embedded dashboard assets:

```bash
make release
```

This outputs a compiled binary without CGO dependencies. Run it as a systemd service:

```ini
[Unit]
Description=Octarq Backend
After=network.target

[Service]
Type=simple
User=octarq
WorkingDirectory=/opt/octarq
ExecStart=/opt/octarq/octarq
Restart=always
Environment=OCTARQ_SECRET_KEY=your-secret-key
Environment=OCTARQ_ADMIN_PASSWORD=your-admin-password
# Optional: Use PostgreSQL instead of SQLite
# Environment=OCTARQ_DB_DRIVER=postgres
# Environment=OCTARQ_DB_DSN=postgres://user:password@localhost:5432/octarq?sslmode=disable

[Install]
WantedBy=multi-user.target
```

---

## 3. Reverse Proxy Configuration

Put Caddy or Nginx in front of Octarq for HTTPS termination.

### Caddyfile Example

```caddy
app.yourdomain.com {
    reverse_proxy localhost:8080
}
```

### Nginx Example

```nginx
server {
    server_name app.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 4. Environment Variables Reference

| Variable | Required | Description |
| --- | --- | --- |
| `OCTARQ_SECRET_KEY` | Yes | 32-byte secret key for token hashing & encryption |
| `OCTARQ_ADMIN_PASSWORD` | Yes | Initial administrator account password |
| `OCTARQ_LISTEN` | No | Listen address (default: `:8080`) |
| `OCTARQ_DB_DRIVER` | No | `sqlite` (default), `postgres`, or `mysql` |
| `OCTARQ_DB_DSN` | No | DSN when using PostgreSQL (e.g. `postgres://user:pass@host:5432/db`) or MySQL 8 (e.g. `user:pass@tcp(host:3306)/db?charset=utf8mb4&parseTime=True&loc=Local`) |
| `OCTARQ_REDIS_URL` | No | Redis connection URL for distributed rate limiting & caching |
| `OCTARQ_MAXMIND_LICENSE_KEY` | No | MaxMind GeoIP license key for auto-downloading GeoIP DB |

---

## 5. Backup & Restore

Octarq ships `octarq backup` and `octarq restore` subcommands supporting SQLite, PostgreSQL, and MySQL 8, plus on-demand backups from the dashboard (Settings → Instance). Read the full walkthrough: **[Backup & Restore](/backup-restore/)**.
