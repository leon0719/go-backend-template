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

`DATABASE_URL`/`REDIS_URL`/`JWT_SECRET` are marked required by struct tags (`env:"...,required"`) — `config.Load()` returns an error if they're unset.

**`.env.local` and the Docker stack configure different things.** Keeping this straight avoids a nasty class of confusion:

| | Configured by | Reaches Postgres/Redis at |
|---|---|---|
| `make up` (containers) | the `environment:` block in `docker/docker-compose.dev.yml` | `postgres:5432` / `redis:6379` (service names) |
| Host-side runs — `make migrate`, `go run ./cmd/api` | `.env.local`, loaded by `godotenv` at startup | `localhost:5432` / `localhost:6380` |

So `make up` needs no `.env.local` at all: Compose carries working defaults. And `.env.local` is deliberately *not* wired into Compose as an `env_file` — Compose ranks `environment:` above `env_file:`, so the values in the compose file would win regardless, and a `.env.local` listed there would be read and silently discarded. Someone putting a real `JWT_SECRET` in it would keep running on the dev default while believing they'd changed it. The two also disagree by construction, per the table above: the same `DATABASE_URL` cannot be right for both.

To override a value for the containers, export it in your shell — the compose file interpolates each one with a default:

```bash
JWT_SECRET=something-else LOG_LEVEL=warn make up
```

## Daily commands

- `make up` — start the whole dev stack via `docker/docker-compose.dev.yml`: `postgres` and `redis` (both health-gated), a one-off `migrate` job that applies the goose migrations and exits, then `api`, `worker` and `scheduler` with hot reload (air). Because `api`/`worker`/`scheduler` wait on `service_healthy` / `service_completed_successfully`, a cold `make up` from an empty volume works with no manual migration step
- `make down` — stop the dev stack
- `make rebuild` — rebuild images after Dockerfile/dependency changes, then `up`
- `make logs-api` — tail the `api` container's logs
- `make migrate` / `make migrate-down` — apply/roll back goose migrations via the `goose` CLI (must be installed on your host) against `$DATABASE_URL`; when running against the dev stack's Postgres, export `DATABASE_URL=postgres://postgres:postgres@localhost:5432/go_backend_template?sslmode=disable` first (the dev stack maps Postgres to host port 5432)
- `make test` — unit tests (no external services needed)
- `make test-integration` — integration tests; spins up Postgres/Redis via testcontainers, requires a local Docker daemon
- `make format` — `gofmt -l -w .`
- `make lint` — `golangci-lint run`, using the checked-in `.golangci.yml` (gosec, errorlint, bodyclose, revive, …). The version CI uses is pinned in `.github/workflows/ci.yml`; install the same one locally with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- `make vet` — `go vet ./...`
- `make docs` — regenerate the Swagger spec (`swag init -g cmd/api/main.go -o internal/httpserver/docs`) after changing handler annotations

## Adding a new app

1. Create `internal/<name>/` with `handler.go`, `service.go`, `repository.go`, `schema.go` as needed
2. Add SQL queries under `internal/db/queries/<name>.sql`, run `sqlc generate`
3. Add a migration under `internal/db/migrations/`, run `make migrate`
4. Decode request bodies with `request.DecodeAndValidate` and translate DB errors with `internal/db/dberr` — do not hand-roll either (see [backend-standards.md](backend-standards.md))
5. Mount routes in `internal/httpserver/router.go` (via a `RegisterRoutes(r chi.Router, ...)` function, mirroring `accounts`/`articles`/`realtime`)
6. Add unit tests (fakes implementing narrow repository interfaces) and, if the code touches Postgres/Redis directly, an integration test (`_integration_test.go`, `//go:build integration`)

## Troubleshooting

- `worker`/`scheduler` logging `compile: signal: killed` on the first `make up`: the Go compiler was OOM-killed. `api`, `worker` and `scheduler` each build the same module, and a cold build of all three at once needs more memory than a small Docker VM has. The compose file shares the Go build/module caches (`gobuild`/`gomod` volumes) between them so only the first build is cold — but give Docker Desktop/colima at least 4 GB of RAM (`colima start --memory 4`). air keeps retrying, so the containers recover on the next rebuild.
- Edits not triggering a rebuild (hot reload appears dead): both `.air.toml` and `.air.worker.toml` set `poll = true` precisely to avoid this. Docker on macOS and Windows runs in a VM, and inotify filesystem events do not cross that boundary through a bind mount — with event-based watching, air sits there seeing nothing while you edit. If you turn polling off for the lower CPU use (fine on native Linux), expect hot reload to stop working the moment someone runs the stack on a Mac.
- `make up` failing with "port is already allocated": something on your host already owns 5432, 6380 or 8000 — most often a locally-installed Postgres or Redis. Repoint the host side of the `ports:` mapping in `docker/docker-compose.dev.yml` (e.g. `"5433:5432"`); only `api` needs a host port for local use, so you can also just delete the mapping. Redis is already mapped to **6380** rather than 6379 for exactly this reason, since a local `redis-server` on the default port is common.
- `go test -tags=integration ./...` failing with connection errors: verify your local Docker daemon is running (testcontainers needs it to spin up Postgres/Redis)
- Swagger page at `/api/docs` stale: run `make docs` after changing handler `@...` annotations, then rebuild/restart the api container
- `make migrate` failing with "goose: command not found": `make migrate`/`make migrate-down` run the `goose` CLI on your host, not inside a container — install it with `go install github.com/pressly/goose/v3/cmd/goose@latest` (the dev containers do not have goose preinstalled, only `air` for hot reload)
- Health endpoints (`/health/live`, `/health/ready`) don't show up in Swagger UI: this is intentional, not a bug — see [docs/api-standards.md](api-standards.md)
