# Go Backend Template

A production-ready Go backend REST API template with an asynq/Redis task queue, PostgreSQL, and Docker-based development.

## Stack

| Layer         | Technology                                    |
| ------------- | ---------------------------------------------- |
| Web / API     | net/http + chi                                 |
| DB access     | sqlc + pgx                                     |
| Database      | PostgreSQL 18                                  |
| Migrations    | goose (embedded into a `cmd/migrate` binary in prod) |
| Async tasks   | asynq + Redis                                  |
| Auth          | JWT access token + DB-backed refresh token     |
| API docs      | swaggo/swag (Swagger UI at `/api/docs`)        |
| Logging       | log/slog                                       |
| Tooling       | gofmt, golangci-lint, go vet, go test          |

## Quick Start (Docker)

```bash
cp .env.local.example .env.local        # then edit JWT_SECRET etc. (optional — dev compose has defaults)
make up                                  # api, worker, postgres, redis
```

The dev compose file (`docker/docker-compose.dev.yml`) ships with working defaults for `DATABASE_URL`, `REDIS_URL`, and `JWT_SECRET`, so `make up` works out of the box even without a `.env.local`. Create one anyway to override values (e.g. a real `JWT_SECRET`) — it's loaded automatically if present.

- API: http://localhost:8000
- API docs (Swagger): http://localhost:8000/api/docs
- Liveness probe: http://localhost:8000/health/live
- Readiness probe: http://localhost:8000/health/ready

Full guide: [docs/local-development.md](docs/local-development.md)

## Common Commands

| Command                  | Description                              |
| ------------------------- | ----------------------------------------- |
| `make up` / `make down`   | Start/stop dev containers                 |
| `make rebuild`             | Rebuild after Dockerfile/dep changes      |
| `make logs-api`            | Tail the `api` container's logs           |
| `make format`               | `gofmt -l -w .`                           |
| `make lint`                  | `golangci-lint run`                       |
| `make vet`                    | `go vet ./...`                            |
| `make test`                    | Unit tests (no external services needed)  |
| `make test-integration`         | Integration tests (needs a local Docker daemon; uses testcontainers) |
| `make migrate` / `make migrate-down` | Apply/roll back DB migrations via the `goose` CLI against `$DATABASE_URL` |
| `make docs`                       | Regenerate the Swagger spec (`swag init`) |

Note: `make migrate` shells out to the `goose` CLI (must be installed locally, or run inside the `api`/`worker` dev container). Production deploys don't use `make migrate` at all — they run the `cmd/migrate` binary (goose migrations embedded via `embed.FS`) as a one-off Compose service; see [docs/deployment.md](docs/deployment.md).

## Project Layout

```
cmd/
  api/          # HTTP server entrypoint (also implements -healthcheck probe mode)
  worker/       # asynq task worker entrypoint
  migrate/      # standalone migration binary (embeds goose SQL files; used in prod)
internal/
  config/       # env-based Config struct (+ narrow ServerConfig/DatabaseConfig loaders)
  httpserver/   # router, middleware, response envelope, generated Swagger docs
  jwtutil/      # JWT access-token primitives shared by accounts + realtime
  db/           # migrations, sqlc queries + generated code, pool
  accounts/     # example app: JWT auth (register/login/refresh/logout/me)
  articles/     # example app: CRUD + ownership + pagination + throttle + task
  health/       # /health/live, /health/ready
  realtime/     # SSE demo endpoint
  tasks/        # asynq task type/payload definitions
  logging/      # slog setup
docker/         # Dockerfiles + compose files (dev/prod) + Caddyfile
```

## Example Endpoints

| Method | Path                             | Auth   | Notes                                     |
| ------ | ---------------------------------- | ------ | ------------------------------------------ |
| POST   | `/api/v1/auth/register`            | —      | Returns access+refresh tokens; IP rate-limited |
| POST   | `/api/v1/auth/login`               | —      | Returns access+refresh tokens; IP rate-limited |
| POST   | `/api/v1/auth/refresh`             | —      | Rotates refresh token                      |
| POST   | `/api/v1/auth/logout`              | Bearer | Revokes all of the user's refresh tokens   |
| GET    | `/api/v1/auth/me`                  | Bearer | Current user profile                       |
| GET    | `/api/v1/articles`                 | Bearer | `?page=&page_size=` (page_size capped at 100) |
| POST   | `/api/v1/articles`                 | Bearer | Create; rate-limited per user              |
| GET    | `/api/v1/articles/{id}`            | Bearer | Owner-scoped (404 if not owner)            |
| PATCH  | `/api/v1/articles/{id}`            | Bearer | Partial update; rate-limited per user      |
| DELETE | `/api/v1/articles/{id}`            | Bearer | 204 on success; rate-limited per user      |
| POST   | `/api/v1/articles/{id}/publish`    | Bearer | Publishes + enqueues webhook task; rate-limited per user |
| GET    | `/api/v1/realtime/sse`             | Bearer | SSE demo stream                            |
| GET    | `/health/live`                     | —      | Liveness probe; unversioned, excluded from Swagger spec |
| GET    | `/health/ready`                    | —      | Readiness probe (checks DB + Redis); unversioned, excluded from Swagger spec |

See [docs/backend-standards.md](docs/backend-standards.md) for conventions.

## Scope decisions (this template does not include)

- No Django-admin equivalent management UI
- No WebSocket support (SSE only)
- No cross-worker SSE broadcast (single-instance only in this version)
- No session-count limit on concurrent logins per user
- No idempotency-key support on writes
- CORS middleware exists but is **disabled by default** (`CORS_ALLOWED_ORIGINS` is empty) — opt in per origin
- No alerting/error-tracking integration (see [docs/alerting.md](docs/alerting.md))
