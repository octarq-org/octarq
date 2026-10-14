# SPEC: `internal/db`

## Purpose
SQLite/Postgres/MySQL connection helpers, AutoMigrate owner, backup/restore
utilities, DSN parsing, pool metrics, tracing hook. Backup/restore are the
only places mixing raw driver tooling (pg_dump/pg_restore, mysqldump).

## Fail-Closed Notes
- `Restore` validates file existence/size before dispatch.
- `restoreSQLite` now propagates `MkdirAll` failure instead of ignoring it.
- `backup`/`restore` cleanup `os.Remove` are best-effort and logged when needed.

## Concurrency
No shared state beyond the global `db`; connection pools owned by GORM.

## Non-Goals
- No help docs (internal package).
EOF
echo done