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

Clone the repository and spin up the complete Octarq stack with Docker Compose:

```bash
git clone https://github.com/octarq-org/octarq.git && cd octarq
cp .env.example .env          # set OCTARQ_SECRET_KEY and OCTARQ_ADMIN_PASSWORD
docker compose up -d
```

<Aside type="tip" title="Success check">
Run `docker compose logs -f` and verify the output:
- The log displays `Octarq server started on :8080`.
- SQLite database migrations run successfully with `AutoMigrate finished`.
- Running `docker compose ps` shows the container state as `Up` (healthy).
</Aside>

Prefer a minimal image (`deploy/Dockerfile.binary`) or from-source build? See `make release` and `deploy/`.

---

## 2. Environment Setup

Create `.env` based on `.env.example` and customize your configuration:

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

- **Database Options**:
  - **SQLite (Default)**: Automatically stored as `octarq.db` (zero external dependencies).
  - **PostgreSQL**: Set `OCTARQ_DB_DRIVER=postgres` and `OCTARQ_DB_DSN` for multi-instance or high-concurrency production deployments.
- **Authentication**: Admin account configured via `OCTARQ_ADMIN_PASSWORD`.

<Aside type="tip" title="Success check">
Verify your database and secret initialization:
- When using SQLite: An `octarq.db` file is created automatically in your working directory or container volume.
- When using PostgreSQL: Connection to `OCTARQ_DB_DSN` establishes without connection pool or SSL handshake errors.
- `OCTARQ_SECRET_KEY` is loaded (must be 32 bytes for AES-256-GCM encryption).
</Aside>

---

## 3. First Login & Dashboard Verification

Open your browser and navigate to `http://localhost:8080`:

1. Enter your administrator username (`admin` by default) and the password set in `OCTARQ_ADMIN_PASSWORD`.
2. Click **Sign In** to access the backoffice.

<Aside type="tip" title="Success check">
Upon successful authentication:
- You are redirected to the `/admin` shell.
- The **Overview** page renders with metric summary cards (Short Links, Mailboxes, Domains, System Health).
- The sidebar displays active workspace areas and configured plugins without 404 or 403 errors in the browser console.
</Aside>

---

## 4. Consuming `@octarq/plugin-sdk`

If you are developing plugins or extensions, install `@octarq/plugin-sdk` directly from npm:

```bash
pnpm add @octarq/plugin-sdk
```

<Aside type="tip" title="Success check">
Verify the SDK installation:
- `@octarq/plugin-sdk` appears in your project's `package.json` under `dependencies`.
- In your plugin source file, `import { type UIPlugin } from "@octarq/plugin-sdk"` resolves cleanly with full TypeScript type definitions and no missing module warnings.
</Aside>
