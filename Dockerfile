# ---- Stage 1: build the React dashboard ----
FROM node:22-alpine AS web
RUN corepack enable
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml* web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile || pnpm install
COPY web/ ./
RUN pnpm build

# ---- Stage 2: build the Go binary (embeds the dashboard) ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Bring in the freshly built dashboard so go:embed picks it up.
COPY --from=web /app/webembed/dist ./webembed/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /led .

# ---- Stage 3: minimal runtime ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /led /led
EXPOSE 8080
VOLUME ["/data"]
ENV LED_DB_DSN=/data/led.db
ENTRYPOINT ["/led"]
