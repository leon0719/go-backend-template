# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A production-ready Go backend REST API template: `net/http` + chi, sqlc + pgx over PostgreSQL, asynq + Redis for async/scheduled tasks, JWT auth, Docker-based dev and prod. `internal/accounts` and `internal/articles` are example apps meant to be copied when adding a new one — see "Adding a new app" below.

## Commands

```bash
make up                              # start dev stack (postgres, redis, migrate one-off, api, worker, scheduler; hot reload via air)
make down                            # stop it
make rebuild                         # rebuild images after Dockerfile/dependency changes, then up
make logs-api                        # tail api logs (also logs-worker / logs-scheduler / logs-db / logs)
make docker-clean                    # down -v — wipes pgdata; next up re-migrates from empty

make all                             # format + vet + lint + test — run before committing
make format                          # gofmt -l -w .
make vet                             # go vet ./...
make lint                            # golangci-lint run (config: .golangci.yml, version pinned in .github/workflows/ci.yml)
make test                            # go test -race ./...            (unit only, no Docker needed)
make test-integration                # go test -race -tags=integration ./...   (testcontainers, needs local Docker)
govulncheck ./...                    # CI also runs this; not wired into `make all`

go test -race ./internal/articles/... -run TestName   # single package / single test
go vet -tags=integration ./...       # integration files are invisible to the untagged vet run — CI checks both

make migrate / make migrate-down     # goose CLI against $DATABASE_URL (host-side; export DATABASE_URL=postgres://postgres:postgres@localhost:5432/go_backend_template?sslmode=disable for the dev stack)
make docs                            # regenerate Swagger spec after changing handler @... annotations (swag init -g cmd/api/main.go -o internal/httpserver/docs)

sqlc generate                        # after editing internal/db/queries/*.sql; commit the regenerated internal/db/sqlc/
```

CI enforces gofmt, `go vet` (both build tags), lint, unit + integration tests (`-race`), govulncheck, and drift checks on `sqlc generate` / `swag init` output — regenerate and commit both before pushing if you touched queries or handler annotations.

On colima, integration tests need `export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"` and `export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock` first.

## Architecture

**Layering** (`internal/accounts`, `internal/articles` are the canonical examples — kept structurally parallel on purpose):

```
handler.go     HTTP only: decode/validate, call service, map result/error to response
service.go     business logic; depends on a narrow repository interface, not sqlc types
repository.go  wraps sqlc-generated Queries; translates pgx/Postgres errors
schema.go      request/response DTOs
```

- **Request decoding**: always `request.DecodeAndValidate` (`internal/httpserver/request`) — never `json.NewDecoder` directly. Applies a 1 MiB body cap and `validator` struct-tag validation, writing 422/413 on failure.
- **Repository errors**: the `ErrNotFound` sentinel and pgx/Postgres error translation live once in `internal/db/dberr` (`WrapNotFound`, `IsUniqueViolation`, `IsForeignKeyViolation`); each app re-exports `ErrNotFound`. Enforce uniqueness with a DB constraint + map SQLSTATE `23505` in the repository — never a SELECT-then-INSERT pre-check (TOCTOU race).
- **Handler error mapping**: never hand-roll `errors.Is`/status codes in a handler. Call `respond.MapError(w, err, respond.ErrMapping{Target: ..., Status: ..., Code: ..., Message: ...})` (`internal/httpserver/respond`), one mapping per sentinel the service returns. Anything unmatched becomes a generic 500 so handlers never leak internals.
- **Pagination**: compute SQL `OFFSET` in `int64` and cap it — `(page-1)*pageSize` in `int32` overflows negative on large page numbers. See `articles.maxOffset`.
- **DB/migrations**: schema changes go through goose migrations in `internal/db/migrations/`; queries are hand-written SQL in `internal/db/queries/`, compiled by sqlc into `internal/db/sqlc/`. No ORM, no query builder. `cmd/migrate` embeds the migration files (`embed.FS`) for prod.
- **Config**: everything flows through `internal/config.Config` (`env:"...,required"` tags; loaded via `caarlos0/env` + `godotenv`). Narrower `LoadServerOnly()`/`LoadDatabaseOnly()` loaders exist for tools that don't need the full set (`-healthcheck` probe, `cmd/migrate`). Never call `os.Getenv` outside that package.
- **JWT**: token creation/parsing lives in `internal/jwtutil`, pulled out of `internal/accounts` to avoid an import cycle with `internal/realtime` (which also verifies access tokens).
- **Periodic tasks**: `cmd/scheduler` runs `asynq.Scheduler` (Celery Beat equivalent) as a separate process from `cmd/worker` — one scheduler, N workers. Register new periodic tasks there; put the handler with its app (`internal/<app>/tasks.go`) or in `internal/tasks` for pure infra (e.g. the heartbeat).
- **Task idempotency**: asynq handlers must be safe to run twice — re-derive everything from the payload's ID rather than trusting local state (see `internal/articles/tasks.go`).
- **Atomic multi-write transactions**: for two+ writes to the *same* database that must succeed/fail together (the Django `transaction.atomic()` equivalent), use `db.WithinTx(ctx, pool, func(qtx *sqlc.Queries) error { ... })` (`internal/db/tx.go`) — shared across every app, commits on nil / rolls back on error or panic via `pgx.BeginFunc`. See `articles.Repository.ArchiveWithEvent` (archives + writes an audit row in one transaction) and its rollback test `TestRepository_ArchiveWithEvent_RollsBackStatusWhenEventInsertFails`.
- **Dual writes**: `articles.Service.Publish` commits the status change then enqueues a webhook task — not atomic. An enqueue failure is logged but the request still returns success (the state change did happen; a 500 would trigger a retry that silently no-ops). This is a known accepted gap, not a pattern to copy — the real fix is a transactional outbox. See `docs/backend-standards.md` before touching this path.
- **Testing**: unit tests use hand-written fakes implementing the narrow repository interfaces (no mocking framework). Integration tests are gated behind `//go:build integration` and use `testcontainers-go`, building schema via `db.MigrateUp` (the same embedded migrations prod runs) — never hand-write `CREATE TABLE` in a test.

**Middleware chain** (`internal/httpserver/router.go`): `RequestID → RealIP → SlogLogger → Recoverer → CORS`, then route-specific `RateLimit`/`JWTAuth` per `RegisterRoutes`. Order is deliberate: `SlogLogger` sits outside `Recoverer` so a panic's 500 still produces an access-log line (chi runs first-registered middleware outermost).

**API conventions** (`docs/api-standards.md`):
- Error envelope: `{"error": {"code": "...", "message": "..."}}` on every non-2xx response; panics are recovered into the same shape. Not RFC 7807 — deliberately simpler.
- Rate limiting: Redis-backed fixed-window via a single atomic Lua script (INCR + conditional EXPIRE). IP-keyed on `/auth/register`/`/auth/login` (10/min, via `TRUSTED_PROXIES`-aware `RealIP`); user-keyed on article writes (30/min). **Fails open** if Redis is unreachable — treated as abuse-prevention, not a hard dependency.
- CORS is disabled by default (`CORS_ALLOWED_ORIGINS` empty); `*` is not special-cased because responses set `Access-Control-Allow-Credentials: true`.
- No idempotency-key support on writes (documented gap, not implemented).

**Adding a new app** (see `docs/local-development.md` for the full walkthrough, or use the `scaffold-app`/`add-resource`/`add-column` skills):
1. `internal/<name>/` with `handler.go`, `service.go`, `repository.go`, `schema.go`
2. SQL under `internal/db/queries/<name>.sql`, then `sqlc generate`
3. Migration under `internal/db/migrations/`, then `make migrate`
4. Decode with `request.DecodeAndValidate`, translate DB errors via `internal/db/dberr` — don't hand-roll either
5. Mount routes in `internal/httpserver/router.go` via a `RegisterRoutes(r chi.Router, ...)` function, mirroring `accounts`/`articles`
6. Unit tests (fakes) + integration test if touching Postgres/Redis directly (`_integration_test.go`, `//go:build integration`)

## Environment variables (`internal/config.Config`)

`DATABASE_URL`, `REDIS_URL`, `JWT_SECRET` are required (struct-tag enforced, `config.Load()` errors if unset). Others: `ENV` (selects `.env.{ENV}`), `PORT`, `LOG_LEVEL`, `CORS_ALLOWED_ORIGINS`, `TRUSTED_PROXIES`, `ARTICLE_PUBLISHED_WEBHOOK_URL`. Full table in `docs/local-development.md`.

**`.env.local` and the Docker stack configure different things and are not wired together** — `docker-compose.dev.yml`'s `environment:` block always wins over any `env_file`, so a `.env.local` value silently has no effect on containerized services. `make up` needs no `.env.local` at all (compose ships working defaults); host-side runs (`go run ./cmd/api`, `make migrate`) read `.env.local` via `godotenv`. To override a containerized value, export it in the shell before `make up` (the compose file interpolates `${VAR}` with a default).

## Project layout

```
cmd/api/          HTTP server entrypoint (also implements -healthcheck probe mode)
cmd/worker/        asynq task consumer
cmd/scheduler/     asynq.Scheduler (run exactly one instance — see docs/deployment.md)
cmd/migrate/       standalone migration binary, goose files embedded via embed.FS (used in prod)
internal/config/   env-based Config (+ narrow ServerConfig/DatabaseConfig loaders)
internal/httpserver/  router, middleware chain, request decoding, respond envelope, Swagger docs
internal/jwtutil/  shared JWT primitives (accounts + realtime)
internal/db/       migrations, sqlc queries + generated code, pool, dberr
internal/accounts/ example app: JWT auth (register/login/refresh/logout/me)
internal/articles/ example app: CRUD + ownership + pagination + rate limit + async task
internal/health/   /health/live, /health/ready
internal/realtime/ SSE demo endpoint
internal/tasks/    asynq task type/payload definitions (infra-level, e.g. heartbeat)
docker/            Dockerfiles (dev/prod) + compose files + Caddyfile
```

## Deployment notes (`docs/deployment.md`, `docs/caddy.md`)

Single-host Docker Compose: one built image (`docker/Dockerfile.prod`, distroless final stage) contains `/api`, `/worker`, `/scheduler`, `/migrate`; `api`/`worker` wait on `migrate` exiting 0. Behind the bundled Caddy, `TRUSTED_PROXIES` must be set (Caddy's container CIDR) or IP-based rate limiting collapses every client into one bucket — see the table in `docs/deployment.md` before touching `RealIP`/rate-limit code. `cmd/api` sets explicit server timeouts; `WriteTimeout` is intentionally `0` to not sever SSE streams.

## Scope decisions (intentionally not implemented)

No admin UI; no WebSocket (SSE only, single-instance, no cross-worker broadcast); no transactional outbox (see dual-writes above); no session-count limits; no idempotency keys; no alerting/error-tracking integration (`docs/alerting.md` is a TODO list, not documentation of existing behavior). Don't assume any of these exist when reasoning about behavior — check the relevant `docs/*.md` first, since they call out gaps explicitly rather than staying silent.

## Project skills (`.claude/skills/`)

Prefer these over hand-editing for routine changes — they encode the multi-file conventions above so the result comes back as a reviewable diff:

| Skill | Use for |
|---|---|
| `add-column` | DB column threaded through migration → sqlc → repository → service → schema → handler → tests → OpenAPI (~14 places) |
| `add-resource` | New CRUD resource inside an existing `internal/<app>` package |
| `scaffold-app` | New `internal/<app>` package, mounted into the router |
| `check` | Full local gate: gofmt, vet (both build tags), lint, unit + integration tests, govulncheck, generated-code drift |
| `conventions-review` | Review a diff against this project's rules — IDOR, layering leaks, migration safety, task idempotency |
