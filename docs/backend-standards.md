# Backend Internal Standards

- **Layering**: `handler.go` (HTTP only) → `service.go` (business logic, depends on a narrow repository interface) → `repository.go` (wraps sqlc `Queries`). See `internal/accounts` or `internal/articles`.
- **DB/migrations**: all schema changes go through goose migrations in `internal/db/migrations/`; all queries are hand-written SQL in `internal/db/queries/`, compiled by sqlc into `internal/db/sqlc/` — no ORM, no query builder.
- **Secrets/config**: all config flows through `internal/config.Config` (or its narrower `ServerConfig`/`DatabaseConfig` siblings for tools that only need part of it); never call `os.Getenv` outside that package.
- **Shared JWT primitives**: token creation/parsing lives in `internal/jwtutil` (extracted out of `internal/accounts` to avoid an import cycle with `internal/realtime`, which also needs to verify access tokens).
- **Observability**: structured logging via `log/slog` (JSON in prod, via `internal/logging`); `X-Request-ID` is generated/propagated by `internal/httpserver/middleware.RequestID`. No distributed tracing in this version.
- **Testing**: unit tests use hand-written fakes implementing narrow repository interfaces (no mocking framework); integration tests use `testcontainers-go` and are gated behind `-tags=integration` so `go test ./...` never needs Docker.
- **Task idempotency**: asynq task handlers (`internal/articles/tasks.go`) must be safe to run twice — this template's example re-derives everything from the task payload's ID rather than trusting local state.
