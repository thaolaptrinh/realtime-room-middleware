.PHONY: help tidy fmt vet lint test test-race \
       build build-gateway build-game-server \
       run-gateway run-game-server \
       config-check \
       smoke smoke-gateway smoke-kcp \
       bench-spatial bench-delta \
       loadtest-50 loadtest-100 loadtest-200 \
       dev-up dev-down dev-logs dev-restart dev-redis \
       clean

BINDIR  := bin
DEV_CFG := deployments/dev/config/dev.yaml

# ── help ──────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

# ── tooling ───────────────────────────────────────────────────────

tidy: ## Run go mod tidy
	go mod tidy

fmt: ## Run gofmt on all Go files
	gofmt -l -s -w .

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (requires install)
	golangci-lint run

# ── test ──────────────────────────────────────────────────────────

test: ## Run all tests
	go test ./...

test-race: ## Run all tests with race detector
	go test -race ./...

# ── build ─────────────────────────────────────────────────────────

build: build-gateway build-game-server ## Build all binaries

build-gateway: ## Build gateway binary
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/gateway ./cmd/gateway

build-game-server: ## Build game-server binary
	@mkdir -p $(BINDIR)
	go build -o $(BINDIR)/game-server ./cmd/game-server

# ── run (dev) ─────────────────────────────────────────────────────

run-gateway: ## Run gateway with dev config
	CONFIG_PATH=$(DEV_CFG) go run ./cmd/gateway

run-game-server: ## Run game-server with dev config
	CONFIG_PATH=$(DEV_CFG) go run ./cmd/game-server

# ── config-check ──────────────────────────────────────────────────

config-check: ## Validate all example/deployment config files
	go run ./cmd/config-check

# ── smoke tests ───────────────────────────────────────────────────

smoke: smoke-gateway smoke-kcp ## Run all smoke tests

smoke-gateway: ## Run gateway smoke test
	go test ./tests/integration -run TestGatewaySmoke -v

smoke-kcp: ## Run KCP smoke test
	go test ./tests/integration -run TestKCPSmoke -v

# ── benchmarks ────────────────────────────────────────────────────

bench-spatial: ## Benchmark spatial hash
	go test ./internal/game/spatial -bench=. -benchmem

bench-delta: ## Benchmark delta broadcast
	go test ./internal/game/delta -bench=. -benchmem

# ── load tests ────────────────────────────────────────────────────

loadtest-50: ## Run 50 CCU load test
	./loadtest/single-vps/run_50ccu.sh

loadtest-100: ## Run 100 CCU load test
	./loadtest/single-vps/run_100ccu.sh

loadtest-200: ## Run 200 CCU load test
	./loadtest/single-vps/run_200ccu.sh

# ── dev environment ───────────────────────────────────────────────

dev-up: ## Start dev Docker Compose
	docker compose -f deployments/dev/docker-compose.yml up --build

dev-down: ## Stop dev Docker Compose
	docker compose -f deployments/dev/docker-compose.yml down

dev-logs: ## Tail dev Docker Compose logs
	docker compose -f deployments/dev/docker-compose.yml logs -f

dev-restart: ## Restart dev Docker Compose
	docker compose -f deployments/dev/docker-compose.yml restart

dev-redis: ## Start Redis only via dev Docker Compose
	docker compose -f deployments/dev/docker-compose.yml --profile redis up redis

# ── clean ─────────────────────────────────────────────────────────

clean: ## Remove build artifacts and test cache
	rm -rf $(BINDIR)
	go clean -testcache
