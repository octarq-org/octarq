# FIX-docs Report

Branch: `fix/ship-docs` — fixes audited release/documentation blockers. No
re-investigation done; all changes applied directly per the audit findings.

## Changes

### 1. README.md, README_ZH.md — docker image name pointed at a 404 image (blocking)

Line 66 in both files:

```diff
-docker run -p 8080:8080 -v octarq-data:/data octarq/octarq
+docker run -p 8080:8080 -v octarq-data:/data ghcr.io/octarq-org/octarq:latest
```

`octarq/octarq` is 404 on Docker Hub; the actual publish location is GHCR.
Now consistent with `docker-compose.yml`, `website/src/content/docs/deploy.md`
and `.github/workflows/release.yml`.

### 2. .github/workflows/release.yml — `latest` tag permanently stale (blocking)

The docker job runs only on tag events
(`if: startsWith(github.ref, 'refs/tags/v')`, line 60), so
`enable={{is_default_branch}}` was always false and `latest` had not been
refreshed since 2026-07-25. Changed the metadata-action `tags` block so tag
releases also produce `latest` — in a tag-only job this equals "the newest
release". The job was NOT changed back to main-push trigger (that guard is
intentional).

Changed line (release.yml, line 88, in the `Docker metadata` step's `tags:`):

```diff
-            type=raw,value=latest,enable={{is_default_branch}}
+            type=raw,value=latest
```

Current full `tags:` block:

```yaml
          tags: |
            type=ref,event=branch
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha,format=short
            type=raw,value=latest
```

### 3. Dockerfile — `/data` not writable by UID 65532 (blocking, defensive)

The distroless/static-debian12 runtime declares `VOLUME ["/data"]` +
`ENV OCTARQ_DB_DSN=/data/octarq.db` + `USER 65532:65532`, but `/data` never
existed in the image. Docker creates fresh volume mount points as root:root, so
first-boot writes (`/data/octarq.secret`, the SQLite DB) would EACCES.

Approach used:

```dockerfile
# Stage 2 (golang:1.25-alpine, has shell) — after the Go build:
RUN mkdir -p /data && chown 65532:65532 /data

# Stage 3 (distroless):
COPY --from=build --chown=65532:65532 /data /data
EXPOSE 8080
VOLUME ["/data"]
```

Rationale:

- distroless/static has no shell, so `RUN mkdir` cannot run in the final stage;
  the directory is created in the shell-capable `build` stage.
- `COPY --from=build --chown=65532:65532 /data /data` transfers both the
  directory and its 65532:65532 ownership into the final image. When Docker
  creates an anonymous/named volume over `/data` at runtime, it initializes the
  new volume with the image directory's ownership, so the volume is writable by
  the container UID from first boot.
- The COPY is deliberately placed **before** the `VOLUME` declaration:
  build-time changes to a path after its `VOLUME` declaration are discarded
  (Docker documented behavior), so a COPY placed after it would not survive
  into the image.

### 4. website/src/content/docs/deploy.md — data volume mounted at wrong path (severe)

Line 32:

```diff
-      - ./data:/app/data
+      - ./data:/data
```

The in-image DSN is `/data/octarq.db` (Dockerfile), so the old mount left the
DB in an anonymous volume — users would lose data on machine change. Now the
bind mount covers the real data path.

### 5. README.md, README_ZH.md — MCP example used a non-existent env var (severe)

Line 126 in both files:

```diff
-  "env": { "OCTARQ_DB_PATH": "/path/to/octarq.db" }
+  "env": { "OCTARQ_DB_DSN": "/path/to/octarq.db" }
```

No Go code reads `OCTARQ_DB_PATH`; `octarq mcp` goes through `config.Load()`,
which reads `OCTARQ_DB_DSN` (config/config.go:151). Users following the old
example would connect to an empty DB.

### 6. .env.example — missing `OCTARQ_CORS_ORIGINS` + `OCTARQ_PORT` clarification (minor)

Added (server section):

```
# CORS allowlist for the public API (comma-separated exact origins, e.g.
# https://app.example.com). Empty = CORS disabled; never "*". Only used as a
# startup fallback when the instance setting has not been written yet.
# OCTARQ_CORS_ORIGINS=
```

Read by config/config.go:165 (`PublicCORSOrigins`); runtime setting
`public_cors_origins` wins once written (internal/api/settings.go).

`OCTARQ_PORT` comment now reads:

```
# Docker compose only: host port published by docker-compose. Change if 8080 is
# taken. To change the binary's own listen port use OCTARQ_LISTEN instead.
OCTARQ_PORT=8080
```

## Verification

- `go build ./...` — passed.
- `gofmt -l .` — no output.
- `grep -rn 'octarq/octarq' README.md README_ZH.md` — no matches (exit 1).
- `grep -rn 'OCTARQ_DB_PATH' .` — no matches (exit 1).

## Not verified (explicit)

- **Dockerfile changes not tested locally** — no Docker daemon on this machine.
  Needs a real `docker build` + `docker run` to confirm `/data` is writable by
  65532 and first-boot succeeds. Do not treat this as verified.
- **release.yml `latest` refresh** — only verifiable on the next tagged release
  run; the workflow change itself is a one-line config diff.
