# ---- Stage 1: build the React dashboard ----
# The dashboard's pnpm workspace (web/pnpm-workspace.yaml) includes the sibling
# @octarq/plugin-sdk at ../packages/* and the example plugin
# @acme/octarq-plugin-hello at ../examples/plugin-hello/web (the OSS default
# manifest composes it), so all three trees must be present for the workspace
# dependencies to resolve. Manifests first for layer caching.
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /app
COPY web/package.json web/pnpm-lock.yaml* web/pnpm-workspace.yaml ./web/
COPY packages/plugin-sdk/package.json ./packages/plugin-sdk/
COPY examples/plugin-hello/web/package.json ./examples/plugin-hello/web/
WORKDIR /app/web
RUN pnpm install --frozen-lockfile || pnpm install
WORKDIR /app
COPY packages/ ./packages/
COPY examples/ ./examples/
COPY web/ ./web/
WORKDIR /app/web
RUN pnpm build

# ---- Stage 2: build the Go binary (embeds the dashboard) ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the freshly built dashboard so go:embed picks it up.
COPY --from=web /app/webembed/dist ./webembed/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /octarq .
# Runtime data dir, owned by the container UID 65532. Created in this
# shell-capable stage — the distroless runtime has no shell — then copied into
# the final image (below) so a Docker-created volume over /data inherits the
# ownership on first boot instead of being root:root.
#
# The .keep placeholder is load-bearing: COPY copies a directory's *contents*,
# not the directory itself, so copying an empty /data would leave whether the
# destination gets created (and whether --chown applies to it) up to the
# BuildKit version. One file in it makes the directory materialise with the
# right ownership on every builder.
RUN mkdir -p /data && touch /data/.keep && chown -R 65532:65532 /data

# ---- Stage 3: minimal runtime ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /octarq /octarq
# Bake /data (owned 65532:65532) in BEFORE declaring VOLUME — build-time
# changes to a path after its VOLUME declaration are discarded, so the COPY
# must precede it.
COPY --from=build --chown=65532:65532 /data /data
EXPOSE 8080
VOLUME ["/data"]
ENV OCTARQ_DB_DSN=/data/octarq.db
USER 65532:65532
ENTRYPOINT ["/octarq"]
