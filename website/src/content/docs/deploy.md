---
title: Deploying Octarq
description: Production deployment guide for Octarq using Docker, binary releases, or cloud providers.
---

Octarq is designed as a single zero-dependency Go binary with an embedded SQLite database and frontend dashboard, making it lightweight and straightforward to deploy.

## 1. Docker Compose (Recommended)

The fastest way to deploy Octarq in production is with Docker Compose:

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
      - OCTARQ_PORT=8080
    volumes:
      - ./data:/app/data
```

Start the container:

```bash
docker compose up -d
```

---

## 2. Standalone Binary Deployment

Build a production binary with embedded dashboard assets:

```bash
make release
```

This outputs a compiled binary without CGO dependencies (~19MB). Run it as a systemd service:

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
| `OCTARQ_PORT` | No | HTTP listening port (default: `8080`) |
| `OCTARQ_MAXMIND_LICENSE_KEY` | No | MaxMind GeoIP license key for auto-downloading GeoIP DB |
