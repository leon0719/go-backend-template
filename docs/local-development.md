# Local Development

## Environment variables

Read by `internal/config.Config` (full app: `cmd/api`, `cmd/worker`). Two narrower loaders exist for tools that shouldn't need the whole set: `config.LoadServerOnly()` (just `ENV`/`PORT`, used by the `-healthcheck` probe) and `config.LoadDatabaseOnly()` (just `DATABASE_URL`, used by `cmd/migrate`).

| Var | Required | Default | Description |
|---|---|---|---|
| `ENV` | no | `local` | Selects which dotenv file to load: `.env.{ENV}` (e.g. `.env.local`, `.env.prod`) |
| `PORT` | no | `8000` | HTTP listen port |
| `DATABASE_URL` | yes | — | Postgres connection string |
| `REDIS_URL` | yes | — | Redis connection string (rate limiting + asynq) |
| `JWT_SECRET` | yes | — | HMAC secret for access tokens |
| `LOG_LEVEL` | no | `info` | `debug`/`info`/`warn`/`error` |
| `CORS_ALLOWED_ORIGINS` | no | `""` | Comma-separated exact-match browser origins. Empty = CORS disabled (no headers emitted) |
| `TRUSTED_PROXIES` | no | `""` | Comma-separated CIDRs allowed to set `X-Forwarded-For`. Empty = header ignored, `RemoteAddr` used |
| `ARTICLE_PUBLISHED_WEBHOOK_URL` | no | `""` | If empty, the publish task is a no-op |

`DATABASE_URL`/`REDIS_URL`/`JWT_SECRET` are marked required by struct tags (`env:"...,required"`) — `config.Load()` returns an error if they're unset. In the dev Compose stack you don't need to set these yourself: `docker/docker-compose.dev.yml` supplies working defaults directly in its `environment:` block, and `.env.local` (if present) is loaded on top and can override them.

## Daily commands

- `make up` — start api, worker, postgres, redis with hot reload (air), using `docker/docker-compose.dev.yml`
- `make down` — stop the dev stack
- `make rebuild` — rebuild images after Dockerfile/dependency changes, then `up`
- `make logs-api` — tail the `api` container's logs
- `make migrate` / `make migrate-down` — apply/roll back goose migrations via the `goose` CLI (must be installed on your host) against `$DATABASE_URL`; when running against the dev stack's Postgres, export `DATABASE_URL=postgres://postgres:postgres@localhost:5432/go_backend_template?sslmode=disable` first (the container maps port 5432 to the host)
- `make test` — unit tests (no external services needed)
- `make test-integration` — integration tests; spins up Postgres/Redis via testcontainers, requires a local Docker daemon
- `make format` — `gofmt -l -w .`
- `make lint` — `golangci-lint run`
- `make vet` — `go vet ./...`
- `make docs` — regenerate the Swagger spec (`swag init -g cmd/api/main.go -o internal/httpserver/docs`) after changing handler annotations

## Adding a new app

1. Create `internal/<name>/` with `handler.go`, `service.go`, `repository.go`, `schema.go` as needed
2. Add SQL queries under `internal/db/queries/<name>.sql`, run `sqlc generate`
3. Add a migration under `internal/db/migrations/`, run `make migrate`
4. Mount routes in `internal/httpserver/router.go` (via a `RegisterRoutes(r chi.Router, ...)` function, mirroring `accounts`/`articles`/`realtime`)
5. Add unit tests (fakes implementing narrow repository interfaces) and, if the code touches Postgres/Redis directly, an integration test (`_integration_test.go`, `//go:build integration`)

## Troubleshooting

- `go test -tags=integration ./...` failing with connection errors: verify your local Docker daemon is running (testcontainers needs it to spin up Postgres/Redis)
- Swagger page at `/api/docs` stale: run `make docs` after changing handler `@...` annotations, then rebuild/restart the api container
- `make migrate` failing with "goose: command not found": `make migrate`/`make migrate-down` run the `goose` CLI on your host, not inside a container — install it with `go install github.com/pressly/goose/v3/cmd/goose@latest` (the dev containers do not have goose preinstalled, only `air` for hot reload)
- Health endpoints (`/health/live`, `/health/ready`) don't show up in Swagger UI: this is intentional, not a bug — see [docs/api-standards.md](api-standards.md)
