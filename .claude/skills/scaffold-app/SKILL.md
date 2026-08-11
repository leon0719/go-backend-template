---
name: scaffold-app
description: Scaffold a new app package under internal/ following this project's layered conventions, register its routes, and wire it into cmd/api. Use when the user asks to "create a new app", "開新 app", "scaffold app", "add a module", or "新增模組".
---

# Scaffold a new app package

Creates `internal/<name>/` with the layer set this project uses everywhere, mounts it, and
leaves it building and tested. For adding a resource to a package that already exists, use
**add-resource** instead.

`internal/articles` is the reference implementation; `internal/accounts` shows the same
conventions with different subject matter. Read one before generating.

## Layout

| File | Responsibility |
|---|---|
| `handler.go` | HTTP only: decode, call the service, map errors to responses. `RegisterRoutes(r chi.Router, svc *Service, jwtSecret string, rl *middleware.RateLimiter)` |
| `schema.go` | Request/response structs with `json` + `validate` tags. No domain logic |
| `service.go` | Business rules. Depends on a **narrow repository interface**, never on HTTP or `pgtype` |
| `repository.go` | Wraps `*sqlc.Queries`; converts domain types ↔ `pgtype`; translates errors via `internal/db/dberr` |
| `external.go` | *Only if* the app calls a third-party HTTP API. Use a client with an explicit timeout — never `http.DefaultClient`, which can hang a worker forever |
| `tasks.go` | *Only if* the app enqueues background work. asynq handlers; payloads carry IDs, handlers re-derive everything else so they are safe to run twice |

Omit files the app does not need. A package with no database has no `repository.go`; do not
create empty ones.

## Steps

1. **Create the package** with only the layers it needs.

2. **Repository interface + assertion** in `service.go`:

   ```go
   type <name>Repository interface { /* only what this service calls */ }

   var _ <name>Repository = (*Repository)(nil)
   ```

   Narrow, and defined by the consumer — that is what lets unit tests use a small hand-written
   fake, and what turns a repository signature change into a build failure.

3. **Database** (if the app owns tables) — migration in `internal/db/migrations/`, queries in
   `internal/db/queries/<name>.sql`, then `sqlc generate`. Every owner-scoped query carries
   `AND user_id = $n`. See **add-resource** for the query conventions.

4. **Mount** — in `internal/httpserver/router.go`:

   ```go
   r.Route("/api/v1/<name>", func(nr chi.Router) {
       <name>.RegisterRoutes(nr, deps.<Name>Svc, deps.Config.JWTSecret, deps.WriteRateLimit)
   })
   ```

   Add the service to `Deps`, and construct it in `cmd/api/main.go`. If it enqueues tasks,
   register the handler in `cmd/worker/main.go` too.

5. **Tests** — unit tests with a fake repository; handler tests through a real `chi.Router`
   including an ownership check if the resource is owner-scoped; an integration test using
   `db.MigrateUp` if it touches Postgres.

6. **Docs** — swag annotations on each handler, then `make docs`.

## Conventions that are not negotiable

- Config comes from `internal/config` — no `os.Getenv` outside that package.
- Logging is `log/slog`. 4xx → Warn, 5xx → Error. The global chain already logs one record per
  request; do not add a second.
- Errors reach clients as `respond.Error(...)` with a `respond.Code*` constant, never a bare
  string and never a raw driver error.
- Unversioned routes (infrastructure probes) stay out of the OpenAPI spec — see the comments in
  `internal/health/handler.go` for why.

## Verify

Use the **check** skill.
