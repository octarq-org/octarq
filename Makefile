.PHONY: all web build run dev docker clean tidy plugin-build plugin-build-reset test lint vulncheck openapi release

BINARY := octarq
AIR    := $(shell go env GOPATH)/bin/air

all: web build

# Build the React dashboard into webembed/dist (embedded by the Go binary).
web:
	cd web && pnpm install && pnpm build

# Build metadata injected via ldflags. Version prefers an exact tag and falls
# back to the short commit hash; commit is always the short hash. Outside a git
# checkout (e.g. building from a release tarball) both degrade to explicit
# placeholders — a build must never fail, or ship empty strings, for lack of git.
GIT_COMMIT  := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo unknown)
GIT_VERSION := $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short=8 HEAD 2>/dev/null || echo dev)
BUILD_TIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS     := -s -w \
	-X github.com/octarq-org/octarq/server/internal/buildinfo.Version=$(GIT_VERSION) \
	-X github.com/octarq-org/octarq/server/internal/buildinfo.Commit=$(GIT_COMMIT) \
	-X github.com/octarq-org/octarq/server/internal/buildinfo.BuiltAt=$(BUILD_TIME)

# Build the single binary (assumes web is already built).
build:
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o ../$(BINARY) .

# Build everything from scratch.
release: web build

run: build
	./$(BINARY)

# Hot-reload dev mode:
#   - air     → watches *.go, rebuilds & restarts the API (listening on :8680)
#   - vite    → serves the frontend on :5173 with HMR
#   - backend → transparently proxies /admin & SPA requests to :5173 (OCTARQ_DEV_WEB_PROXY)
# Open http://localhost:8680/admin/ (or http://localhost:5173/admin/)
# Override port:  OCTARQ_PORT=9000 make dev
# Ctrl-C kills both processes.
#
# OCTARQ_PORT is the one knob, and it has to reach both halves: vite reads it to
# aim its /api proxy, the backend reads OCTARQ_LISTEN (config.go). Deriving the
# second from the first is what makes the documented override work — setting only
# OCTARQ_PORT moved the proxy target while the backend stayed on .env's port, so
# vite proxied to a port with nothing behind it. Explicit env beats .env in
# loadDotEnv, so this wins over a checked-in OCTARQ_LISTEN.
dev:
	@echo "Starting backend (air) + frontend (vite) with hot reload..."
	@export OCTARQ_PORT=$${OCTARQ_PORT:-8680}; \
	  export OCTARQ_LISTEN=":$$OCTARQ_PORT"; \
	  export OCTARQ_DEV_WEB_PROXY="http://localhost:5173"; \
	  trap 'kill 0' INT; \
	  (cd server && $(AIR)) & \
	  (cd web && OCTARQ_PORT=$$OCTARQ_PORT pnpm dev) & \
	  wait

docker:
	docker build -t octarq:latest .

# Build a custom binary with third-party plugins composed in (xcaddy-style).
# Set OCTARQ_PLUGINS to a JSON array of {go, gomod, npm} entries, e.g.
#   OCTARQ_PLUGINS='[{"go":"github.com/you/octarq-plugin-foo","gomod":"github.com/you/octarq-plugin-foo@v1.0.0","npm":"@you/octarq-plugin-foo"}]' make plugin-build
# cmd/octarq-build regenerates custom_plugins.go (backend) + .octarq-frontend-plugins.json (frontend);
# the frontend build then composes the UI halves via its own OCTARQ_PLUGINS manifest.
# The frontend list (bare npm names) is what the web build consumes. Export it
# for BOTH install and build: an inline assignment before one command does not
# reach the next, and the raw OCTARQ_PLUGINS entries ({go, gomod, npm}) are not
# a manifest the frontend can read.
plugin-build:
	@test -n "$(OCTARQ_PLUGINS)" || { echo "set OCTARQ_PLUGINS to a JSON array of plugin entries"; exit 1; }
	cd server && go run ./cmd/octarq-build
	cd web && front="$$(cat ../server/.octarq-frontend-plugins.json 2>/dev/null || cat ../.octarq-frontend-plugins.json)" && OCTARQ_PLUGINS="$$front" pnpm install && OCTARQ_PLUGINS="$$front" pnpm build
	$(MAKE) build
	@echo "Built ./$(BINARY) with custom plugins. Reset: make plugin-build-reset"

# Undo what plugin-build wrote into tracked files: the generated backend wiring,
# the go.mod/go.sum rows it added, and the composed dashboard in webembed/dist
# (checked in, and refreshed only by CI on main). The binary keeps the composed
# build; only the source tree goes back to the plain checkout.
plugin-build-reset:
	git checkout -- server/custom_plugins.go server/go.mod server/go.sum server/webembed/dist
	git clean -fdq server/webembed/dist
	rm -f server/.octarq-frontend-plugins.json .octarq-frontend-plugins.json

test:
	cd server && go test ./... -race

lint:
	cd server && golangci-lint run ./...

tidy:
	cd server && go mod tidy

vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	cd server && govulncheck ./...

# Prints to stdout; redirect it where you want it. Writing a file here would
# leave an untracked openapi.json in the repo root on every run.
openapi:
	cd server && go run cmd/openapi-gen/main.go

clean:
	rm -f $(BINARY) *.db *.db-*
	rm -rf web/node_modules server/webembed/dist/assets server/.air .air
