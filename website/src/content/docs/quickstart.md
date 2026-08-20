---
title: Quickstart
description: Get up and running with Octarq in minutes.
sidebar:
  order: 2
  group:
    label: "Start"
---

Run Octarq on your server or local environment using Docker Compose or single binary builds.

## 1. Quick Start with Docker Compose

```bash
git clone https://github.com/octarq-org/octarq.git && cd octarq
cp .env.example .env          # set OCTARQ_SECRET_KEY and OCTARQ_ADMIN_PASSWORD
docker compose up -d
```

Open `http://localhost:8080` and log in. The container runs the dashboard, API, redirector, and MCP server using embedded SQLite by default.

Prefer a minimal image (`deploy/Dockerfile.binary`) or from-source build? See `make release` and `deploy/`.

---

## 2. Environment Setup

Create `.env` based on `.env.example`:

```ini
OCTARQ_SECRET_KEY=your-32-byte-secret-key
OCTARQ_ADMIN_PASSWORD=your-secure-admin-password
OCTARQ_LISTEN=:8080

# Database engine: sqlite (default, zero-config) or postgres (production recommended)
OCTARQ_DB_DRIVER=sqlite
# When using postgres:
# OCTARQ_DB_DRIVER=postgres
# OCTARQ_DB_DSN=postgres://user:password@localhost:5432/octarq?sslmode=disable
```

- **Database**:
  - **SQLite (Default)**: Stored automatically as `octarq.db` (zero external dependencies).
  - **PostgreSQL**: Set `OCTARQ_DB_DRIVER=postgres` and `OCTARQ_DB_DSN` for multi-instance or high-concurrency production deployments.
- **Authentication**: Admin account configured via environment variables.

---

## 3. Consuming `@octarq/plugin-sdk`

If you are developing plugins or extensions, install `@octarq/plugin-sdk` directly from npm:

```bash
pnpm add @octarq/plugin-sdk
```
