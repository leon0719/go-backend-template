.PHONY: up down rebuild logs-api format lint vet test test-integration migrate migrate-down docs

up:
	docker compose -f docker/docker-compose.dev.yml up

down:
	docker compose -f docker/docker-compose.dev.yml down

rebuild:
	docker compose -f docker/docker-compose.dev.yml up --build

logs-api:
	docker compose -f docker/docker-compose.dev.yml logs -f api

format:
	gofmt -l -w .

lint:
	golangci-lint run  # config: .golangci.yml (version pinned in .github/workflows/ci.yml)

vet:
	go vet ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

migrate:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" down

docs:
	swag init -g cmd/api/main.go -o internal/httpserver/docs
