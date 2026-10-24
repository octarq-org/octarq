# SPEC: `internal/db`

## Purpose
SQLite/Postgres/MySQL connection helpers, AutoMigrate owner, backup/restore
utilities, DSN parsing, pool metrics, tracing hook. Backup/restore are the
only places mixing raw driver tooling (pg_dump/pg_restore, mysqldump).

## Connection Pool Architecture (Dual Pool)
To eliminate SQLite lock contention (`database is locked`, code 5 / `SQLITE_BUSY`)
under heavy multi-tenant, analytical queries, and agent trajectory workloads,
the database layer supports dedicated 1-writer and N-reader connection pools:

- **1-Writer Pool (`writeDB`)**:
  - `SetMaxOpenConns(1)` / `SetMaxIdleConns(1)`.
  - Serializes all write transactions through a single Go database handle mutex,
    preventing SQLite write lock collisions.
  - Used for schema migrations (`AutoMigrate`), data insertions, updates, and deletes.
- **N-Reader Pool (`readDB`)**:
  - `SetMaxOpenConns(max(4, runtime.NumCPU()))` / `SetMaxIdleConns(max(4, runtime.NumCPU()))`.
  - Allows concurrent read-only queries against SQLite WAL logs without blocking writers.
  - Used for background analytics, agent trajectory history inspection, and reads.
- **`OpenDualPool(cfg *config.Config)`**:
  - Exposes `(writeDB *gorm.DB, readDB *gorm.DB, err error)`.
  - For SQLite, instantiates dedicated writer (`1` conn) and reader (`max(4, runtime.NumCPU())` conns) pools.
  - For Postgres and MySQL, returns standard connection handles with zero regressions.

## SQLite WAL Pragmas
SQLite requires Write-Ahead Logging (WAL) and strict busy timeouts. To guarantee
every pooled connection inherits these pragmas across dynamic reconnects, pragmas
are enforced both in the DSN via `EnsureSQLiteWALDSN` and immediately on live handles:

- `_pragma=busy_timeout(5000)`: Wait up to 5,000ms on locks before failing.
- `_pragma=journal_mode(WAL)`: Write-Ahead Logging enables non-blocking concurrent readers.
- `_pragma=synchronous(NORMAL)`: Balances ACID persistence with high write throughput in WAL mode.
- `_pragma=foreign_keys(1)`: Strictly enforces relational integrity across all handles.

## Fail-Closed Notes
- `Restore` validates file existence/size before dispatch.
- `restoreSQLite` propagates `MkdirAll` failure instead of ignoring it.
- `backup`/`restore` cleanup `os.Remove` are best-effort and logged when needed.
- `OpenDualPool` fails closed on unsupported database drivers or connection configuration errors,
  releasing partially allocated pool handles before returning errors.

## Concurrency
- SQLite writers are strictly serialized (`SetMaxOpenConns(1)`) to eliminate writer-writer contention.
- SQLite readers run concurrently across `max(4, runtime.NumCPU())` connections in WAL mode.
- Readers never block writers and writers never block readers.
- Zero shared state outside connection pools managed by GORM / `database/sql`.

## Non-Goals
- No help docs (internal package).
