.PHONY: help
.PHONY: up down build rebuild logs logs-api logs-worker logs-scheduler logs-db docker-clean
.PHONY: prod-build prod-up prod-down prod-logs
.PHONY: all format lint vet test test-integration
.PHONY: migrate migrate-down docs

COMPOSE_DEV = docker compose -f docker/docker-compose.dev.yml

# --env-file lets compose interpolate ${...} in the prod file from .env.prod.
# Omitted when the file is absent so `make prod-build` still works in CI, which
# builds the image without ever holding production secrets.
COMPOSE_PROD = docker compose $(if $(wildcard .env.prod),--env-file .env.prod) -f docker/docker-compose.prod.yml

# Baked into the prod image as a build arg, which Dockerfile.prod turns into
# -ldflags "-X main.GitCommitSHA=..." and cmd/api logs at startup. Without this
# export the compose default takes over and every image reports "unknown", so
# you cannot tell which commit a running container is.
export GIT_COMMIT_SHA ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)

# Default target: running a bare `make` should explain itself, not do something.
help:
	@echo "========================================"
	@echo "  go-backend-template"
	@echo "========================================"
	@echo ""
	@echo "開發環境 (Docker):"
	@echo "  make up               - 啟動開發堆疊 (背景執行)"
	@echo "  make down             - 停止開發堆疊"
	@echo "  make build            - 僅建構開發映像 (不啟動)"
	@echo "  make rebuild          - 重新建構並啟動 (改 Dockerfile 或相依後用)"
	@echo "  make logs             - 查看所有服務日誌"
	@echo "  make logs-api         - 查看 API 日誌 (另有 logs-worker / logs-scheduler / logs-db)"
	@echo "  make docker-clean     - 停止並刪除開發資料卷 (資料庫會清空)"
	@echo ""
	@echo "生產環境 (Docker):"
	@echo "  make prod-build       - 建構生產映像 (自動帶入 GIT_COMMIT_SHA)"
	@echo "  make prod-up          - 啟動生產堆疊"
	@echo "  make prod-down        - 停止生產堆疊"
	@echo "  make prod-logs        - 查看生產日誌"
	@echo ""
	@echo "程式碼品質:"
	@echo "  make all              - format + vet + lint + test"
	@echo "  make format           - gofmt -l -w ."
	@echo "  make lint             - golangci-lint run (設定: .golangci.yml)"
	@echo "  make vet              - go vet ./..."
	@echo "  make test             - 單元測試"
	@echo "  make test-integration - 整合測試 (需要 Docker，用 testcontainers)"
	@echo ""
	@echo "資料庫與文件:"
	@echo "  make migrate          - 套用 goose migration (需 host 有 goose 且設定 DATABASE_URL)"
	@echo "  make migrate-down     - 回滾一個 migration"
	@echo "  make docs             - 重新產生 Swagger spec"

# ===================
# Development (Docker)
# ===================

up:
	$(COMPOSE_DEV) up -d
	@echo ""
	@echo "開發環境已啟動 (migration 由 migrate service 自動套用)"
	@echo "  API:      http://localhost:8000"
	@echo "  API Docs: http://localhost:8000/api/docs"
	@echo ""
	@echo "  日誌: make logs"

down:
	$(COMPOSE_DEV) down

build:
	$(COMPOSE_DEV) build

rebuild:
	$(COMPOSE_DEV) up -d --build

logs:
	$(COMPOSE_DEV) logs -f

logs-api:
	$(COMPOSE_DEV) logs -f api

logs-worker:
	$(COMPOSE_DEV) logs -f worker

logs-scheduler:
	$(COMPOSE_DEV) logs -f scheduler

logs-db:
	$(COMPOSE_DEV) logs -f postgres

docker-clean:
	$(COMPOSE_DEV) down -v
	@echo "開發資料卷已刪除 (pgdata / 建置快取)；下次 make up 會重新 migrate"

# ===================
# Production (Docker)
# ===================

prod-build:
	$(COMPOSE_PROD) build

prod-up:
	$(COMPOSE_PROD) up -d
	@echo ""
	@echo "生產堆疊已啟動 (migrate 先跑完，api/worker/scheduler 才會起來)"

prod-down:
	$(COMPOSE_PROD) down

prod-logs:
	$(COMPOSE_PROD) logs -f

# ===================
# Code Quality
# ===================

all:
	@$(MAKE) format
	@$(MAKE) vet
	@$(MAKE) lint
	@$(MAKE) test
	@echo ""
	@echo "所有檢查通過"

format:
	gofmt -l -w .

lint:
	golangci-lint run  # config: .golangci.yml (version pinned in .github/workflows/ci.yml)

vet:
	go vet ./...

# -race matches CI. The SSE hub, asynq workers and the Redis rate limiter are
# all concurrent code; running the suite without the detector hides exactly
# the class of bug that only shows up under production load.
test:
	go test -race ./...

test-integration:
	go test -race -tags=integration ./...

# ===================
# Database / Docs
# ===================

migrate:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" down

docs:
	swag init -g cmd/api/main.go -o internal/httpserver/docs
