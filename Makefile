.PHONY: all web build run dev docker clean tidy plugin-build

BINARY := octarq
AIR    := $(shell go env GOPATH)/bin/air

all: web build

# Build the React dashboard into webembed/dist (embedded by the Go binary).
web:
	cd web && pnpm install && pnpm build

# Build the single binary (assumes web is already built).
build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BINARY) .

# Build everything from scratch.
release: web build

run: build
	./$(BINARY)

# Hot-reload dev mode:
#   - air     → watches *.go, rebuilds & restarts the API (port from .env / OCTARQ_LISTEN)
#   - vite    → serves the frontend on :5173 with HMR, proxies /api → backend
# Open http://localhost:5173/admin/
# Override port:  OCTARQ_PORT=9000 make dev
# Ctrl-C kills both processes.
dev:
	@echo "Starting backend (air) + frontend (vite) with hot reload..."
	@export OCTARQ_PORT=$${OCTARQ_PORT:-8680}; \
	  trap 'kill 0' INT; \
	  $(AIR) & \
	  (cd web && OCTARQ_PORT=$$OCTARQ_PORT pnpm dev) & \
	  wait

docker:
	docker build -t octarq:latest .

# Build a custom binary with third-party plugins composed in (xcaddy-style).
# Set OCTARQ_PLUGINS to a JSON array of {go, gomod, npm} entries, e.g.
#   OCTARQ_PLUGINS='[{"go":"github.com/you/octarq-plugin-foo","gomod":"github.com/you/octarq-plugin-foo@v1.0.0","npm":"@you/octarq-plugin-foo"}]' make plugin-build
# cmd/octarq-build regenerates custom_plugins.go (backend) + .octarq-frontend-plugins.json (frontend);
# the frontend build then composes the UI halves via its own OCTARQ_PLUGINS manifest.
# Reset afterwards: git checkout custom_plugins.go go.mod go.sum && rm -f .octarq-frontend-plugins.json
plugin-build:
	@test -n "$(OCTARQ_PLUGINS)" || { echo "set OCTARQ_PLUGINS to a JSON array of plugin entries"; exit 1; }
	go run ./cmd/octarq-build
	cd web && OCTARQ_PLUGINS="$$(cat ../.octarq-frontend-plugins.json)" pnpm install && pnpm build
	$(MAKE) build
	@echo "Built ./$(BINARY) with custom plugins. Reset: git checkout custom_plugins.go go.mod go.sum && rm -f .octarq-frontend-plugins.json"

tidy:
	go mod tidy

vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

openapi:
	go run cmd/openapi-gen/main.go


clean:
	rm -f $(BINARY) *.db *.db-*
	rm -rf web/node_modules webembed/dist/assets .air
