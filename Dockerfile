# ---- Stage 1: build the React dashboard ----
# The dashboard's pnpm workspace (web/pnpm-workspace.yaml) includes the sibling
# @octarq-org/plugin-sdk at ../packages/*, so both trees must be present for the
# workspace dependency to resolve. Manifests first for layer caching.
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /app
COPY web/package.json web/pnpm-lock.yaml* web/pnpm-workspace.yaml ./web/
COPY packages/plugin-sdk/package.json ./packages/plugin-sdk/
WORKDIR /app/web
RUN pnpm install --frozen-lockfile || pnpm install
WORKDIR /app
COPY packages/ ./packages/
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

# ---- Stage 3: minimal runtime ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /octarq /octarq
EXPOSE 8080
VOLUME ["/data"]
ENV OCTARQ_DB_DSN=/data/octarq.db
USER 65532:65532
ENTRYPOINT ["/octarq"]
