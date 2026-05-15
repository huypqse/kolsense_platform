.PHONY: all build run ingest dev deps up down migrate pull-models test lint clean

BINARY_API    := bin/api
BINARY_INGEST := bin/ingest
GO            := go
DOCKER        := docker compose

# ─── Build ─────────────────────────────────────────────────────────────────────
all: build

build:
	@echo "→ Building API server..."
	$(GO) build -o $(BINARY_API) ./cmd/api
	@echo "→ Building ingest CLI..."
	$(GO) build -o $(BINARY_INGEST) ./cmd/ingest

# ─── Run ───────────────────────────────────────────────────────────────────────
run: build
	./$(BINARY_API)

ingest: build
	./$(BINARY_INGEST) --dir=./data

# ─── Dev (auto-rebuild) ────────────────────────────────────────────────────────
dev:
	@which air > /dev/null || $(GO) install github.com/air-verse/air@latest
	air -c .air.toml

# ─── Dependencies ──────────────────────────────────────────────────────────────
deps:
	$(GO) mod tidy
	$(GO) mod download

# ─── Docker infra ──────────────────────────────────────────────────────────────
up:
	$(DOCKER) up -d postgres ollama

down:
	$(DOCKER) down

# ─── DB migration (runs automatically via docker-entrypoint-initdb.d) ──────────
migrate:
	@echo "→ Running migrations against DATABASE_URL..."
	psql $${DATABASE_URL} -f internal/vectorstore/migrations/001_init.sql

# ─── Pull Ollama models ────────────────────────────────────────────────────────
pull-models:
	@echo "→ Pulling bge-m3 embedding model..."
	docker exec kolsense-ollama ollama pull bge-m3
	@echo "→ Pulling qwen2.5:14b LLM model..."
	docker exec kolsense-ollama ollama pull qwen2.5:14b

# ─── Test ──────────────────────────────────────────────────────────────────────
test:
	$(GO) test ./... -v -race -timeout 120s

test-short:
	$(GO) test ./... -short -timeout 30s

# ─── Lint ──────────────────────────────────────────────────────────────────────
lint:
	@which golangci-lint > /dev/null || (echo "Install golangci-lint first" && exit 1)
	golangci-lint run ./...

# ─── Clean ─────────────────────────────────────────────────────────────────────
clean:
	rm -rf bin/
	$(GO) clean -cache
