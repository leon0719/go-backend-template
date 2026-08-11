---
name: add-resource
description: Add a CRUD resource (table + repository + service + endpoints) to an EXISTING app package, following this project's layering, ownership, pagination and rate-limit conventions. Use when the user asks to "add an endpoint", "add a CRUD resource", "加端點", "新增 API", or "expose <thing> via the API".
---

# Add a CRUD resource to an existing app

Use when the target package under `internal/` already exists; otherwise use **scaffold-app**.

`internal/articles` is the canonical reference — it exercises every convention this project
has (ownership scoping, pagination, filtering, per-user rate limiting, a background task on a
state transition). Copy its shape rather than inventing one.

## Steps

1. **Migration** — `internal/db/migrations/NNNNN_create_<table>.sql`, goose format with both
   directions. Owner-scoped tables need `user_id UUID NOT NULL REFERENCES users(id) ON DELETE
   CASCADE` and an index on the column the list query actually filters by. Enforce real
   invariants in the schema (`CHECK (status IN (...))`, unique indexes) — a rule that lives
   only in Go survives exactly as long as every future write path remembers it.

2. **Queries** (`internal/db/queries/<table>.sql`) — hand-written SQL, compiled by sqlc.
   - **Every owner-scoped query carries `AND user_id = $n`.** No exceptions: a missing one is
     an IDOR, and the repository cannot add it back.
   - List and count must use the *same* WHERE clause, or pagination totals disagree with the
     rows returned.
   - Partial updates: `col = coalesce(sqlc.narg('col'), col)`, never a positional `$n`.
   - Conditional state transitions: a single `UPDATE ... WHERE status = 'draft'` marked
     `:execrows`, so rows-affected tells you whether *this* caller made the transition. That
     is what makes a side effect fire exactly once under concurrency.

   Then `sqlc generate`, and read the generated params structs before writing Go against them.

3. **Repository** (`repository.go`) — wrap `*sqlc.Queries`. Public methods take domain types
   (`uuid.UUID`, `string`, `*string`); convert to `pgtype` internally. Translate errors with
   `internal/db/dberr`: `dberr.WrapNotFound` for `:one` queries, `dberr.IsUniqueViolation` for
   constraint conflicts. `:execrows` deletes return `ErrNotFound` on 0 rows.

4. **Service** (`service.go`) — declare a **narrow interface** listing only the repository
   methods this service uses, and assert it:

   ```go
   var _ <name>Repository = (*Repository)(nil)
   ```

   That line is what turns a signature drift into a build failure instead of a runtime
   surprise. Business rules live here; no HTTP types, no `pgtype`.

5. **Schemas** (`schema.go`) — `*CreateRequest` (value fields, `validate:"required"` etc.),
   `*UpdateRequest` (pointer fields; `omitempty` in front of every constraint so absence isn't
   rejected — otherwise PATCH can blank a field POST refuses to create empty), `*Response`,
   and a `List*Response` carrying `items`/`total`/`page`/`page_size`.

6. **Handler** (`handler.go`) — HTTP only.
   - Decode with `request.DecodeAndValidate` (it applies the body-size limit and validation);
     do not hand-roll `json.NewDecoder`.
   - Reply with `respond.JSON` / `respond.Error` and the `respond.Code*` constants — never a
     bare error-code string.
   - Read the caller from `middleware.UserIDFromContext`; mount `middleware.JWTAuth` in
     `RegisterRoutes`.
   - **Not found and not owned must be indistinguishable** — both 404, and a malformed UUID
     404s too. Anything else tells an attacker which ids exist.
   - Clamp pagination defensively (`page < 1 → 1`, `pageSize < 1 → default`, cap the max).
     The service does not validate these, and a negative OFFSET is a 500.
   - Rate-limit writes per-user when a limiter is supplied; leave routes unwrapped when it is
     nil so tests can construct the router without Redis.

7. **Mount** — add the route group in `internal/httpserver/router.go`, and construct the
   repository/service in `cmd/api/main.go`.

8. **Tests**
   - Unit: hand-written fake implementing the narrow interface (no mocking framework), covering
     the business rules — especially "a non-owner gets not-found".
   - Handler: `httptest` through one `chi.Router`, with a real IDOR test — create as user A,
     fetch as user B, assert 404. Two separate routers prove nothing.
   - Integration (`_integration_test.go`, `//go:build integration`): build the schema with
     `db.MigrateUp(connStr)`. Never hand-write `CREATE TABLE` in a test — a second copy of the
     schema drifts silently and the test then passes against a shape production no longer has.
     Postgres containers need `postgres.BasicWaitStrategies()` or they fail with
     "connection reset by peer".

9. **Docs** — annotate handlers for swag, then `make docs`.

## Verify

Use the **check** skill. Both generated trees must be committed or CI's drift check fails.
