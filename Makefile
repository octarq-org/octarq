.PHONY: all web build run dev docker clean tidy

BINARY := led

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

# Run API + Vite dev server (frontend hot reload, proxies /api to :8080).
dev:
	cd web && pnpm dev

docker:
	docker build -t led:latest .

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) *.db *.db-*
	rm -rf web/node_modules webembed/dist/assets
