---
title: Backup & Restore
description: Backing up and restoring an Octarq instance with the `octarq backup` / `octarq restore` commands.
sidebar:
  order: 6
  group:
    label: "Start"
---

Octarq ships a built-in `backup` and `restore` subcommand for the database.
Read the warnings below before relying on them in production.

## Two facts that shape every backup

### 1. Back up the whole data directory — a database backup alone is not enough

When Octarq boots with no configuration (the zero-config `docker run` path), it
generates a secret key and an initial admin password and persists them **next to
the database**, as `octarq.secret` and `octarq-admin-password.txt` (in the Docker
image: `/data/octarq.secret` and `/data/octarq-admin-password.txt`).

- `octarq.secret` is the only thing that can decrypt stored credentials (TOTP
  seeds, plugin and provider secrets) — it is the key-encryption key.
- `octarq-admin-password.txt` is the only copy of the initial admin login.

Restoring **only the database file** on a different machine therefore leaves that
machine unable to decrypt any stored credential and without a working admin
login. **Back up and restore the whole data directory** — the database *and* the
two key files together. If you manage `OCTARQ_SECRET_KEY` / `OCTARQ_ADMIN_PASSWORD`
yourself via the environment, the equivalent of the key files lives in your
secret manager instead and must be backed up there.

### 2. Postgres backup/restore needs host tools — it cannot run inside the container

The Postgres path shells out to `pg_dump` / `pg_restore` / `psql` from the
PostgreSQL client tools ([`internal/db/backup.go`](https://github.com/octarq-org/octarq/blob/main/internal/db/backup.go)).
The published Octarq images (scratch / distroless) are minimal and do **not**
contain those binaries, so `octarq backup` / `octarq restore` against Postgres
fail inside a stock container with a "pg_dump command not found" error. Run the
command on a host (or sidecar) that has `postgresql-client` installed and can
reach the Postgres server. SQLite has no such dependency — it is a pure-Go
`VACUUM INTO` snapshot.

## Taking a backup

```bash
octarq backup [-o <output-path>]
```

| Driver   | Mechanism | While the server runs? |
| --- | --- | --- |
| `sqlite`   | Online, non-locking consistent snapshot via `VACUUM INTO` | ✅ safe |
| `postgres` | Plain `pg_dump` SQL dump | ✅ safe (needs `pg_dump` on the host, see above) |

Default output filename is `octarq-backup-<timestamp>.db` (SQLite) or
`octarq-backup-<timestamp>.sql` (Postgres) in the current directory; override
with `-o` / `--out`.

## Restoring

```bash
octarq restore --in <backup-file> [-y | --yes]
```

- The command refuses to proceed until you type `yes` (or pass `-y` / `--yes`
  / `--confirm`).
- Before restoring, it automatically creates a **safety backup of the current
  database** (`octarq-backup-before-restore-<timestamp>.db`) — a bad restore is
  never irreversible.
- **SQLite**: the input file is integrity-checked (`PRAGMA integrity_check`)
  before it replaces the live database, then swapped in atomically. If the Octarq
  server is running, **restart it** afterwards so it reloads the restored
  database.
- **Postgres**: restores either a `pg_restore` binary dump (`.dump` / `.tar`) or
  a plain SQL dump via `psql`, both with `--clean` semantics on the target. As
  above, these two tools must exist on the host that runs the command.

## Scheduling

`backup` is on-demand only — there is no built-in scheduler and nothing runs on
a timer. Backups happen when you make them happen: wire `octarq backup` (plus
the key files, §1) into your cron, Kubernetes CronJob, or backup agent, and test
a restore before you need one. The pre-launch checklist
([`docs/PRE-LAUNCH.md`](https://github.com/octarq-org/octarq/blob/main/docs/PRE-LAUNCH.md))
treats this as a checklist item for a reason.
