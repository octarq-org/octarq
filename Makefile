.PHONY: all web build run dev docker clean tidy

BINARY := led
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
#   - air     → watches *.go, rebuilds & restarts the API (port from .env / LED_LISTEN)
#   - vite    → serves the frontend on :5173 with HMR, proxies /api → backend
# Open http://localhost:5173/admin/
# Override port:  LED_PORT=9000 make dev
# Ctrl-C kills both processes.
dev:
	@echo "Starting backend (air) + frontend (vite) with hot reload..."
	@export LED_PORT=$${LED_PORT:-8680}; \
	  trap 'kill 0' INT; \
	  $(AIR) & \
	  (cd web && LED_PORT=$$LED_PORT pnpm dev) & \
	  wait

docker:
	docker build -t led:latest .

tidy:
	go mod tidy

vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

clean:
	rm -f $(BINARY) *.db *.db-*
	rm -rf web/node_modules webembed/dist/assets .air
