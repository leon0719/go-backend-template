---
name: add-column
description: Add a database column to an existing table and thread it through every layer (migration, sqlc, repository, service, schema, handler, tests, OpenAPI). Use when the user asks to "add a column", "add a field", "加欄位", "新增欄位", or "articles 要多一個 X".
---

# Add a column

The mechanical half of this is what makes it tedious by hand: one column touches ~14 places
across six files plus two sets of generated code. Do all of it, then hand back a diff to review.

`internal/db/migrations/00005_add_articles_summary.sql` and the commit that added it are the
worked reference — match that shape.

## Before starting

Ask only if genuinely ambiguous, otherwise pick and state your choice:

- **Nullable or `NOT NULL DEFAULT`?** Prefer `NOT NULL DEFAULT ''` / `DEFAULT 0` — sqlc then
  generates a plain `string`/`int32` instead of `pgtype.Text`, and no caller has to check
  `.Valid` for a field that just means "not written yet". Use nullable only when "unset" is
  genuinely distinct from the zero value.
- **Writable through the API, or derived/internal?** A read-only column skips the request
  schemas and the INSERT/UPDATE queries entirely.

## Steps

1. **Migration** — `internal/db/migrations/NNNNN_<verb>_<table>_<column>.sql`, numbering one
   past the current highest. goose format, both directions:

   ```sql
   -- +goose Up
   ALTER TABLE articles ADD COLUMN summary TEXT NOT NULL DEFAULT '';

   -- +goose Down
   ALTER TABLE articles DROP COLUMN summary;
   ```

   `NOT NULL` without a `DEFAULT` fails on any non-empty table — say why in a comment when the
   choice isn't obvious. If the Up statement contains a semicolon inside a function body or a
   multi-statement block, wrap it in `-- +goose StatementBegin` / `-- +goose StatementEnd`.
   If Up destroys data (folding duplicates, dropping rows), say so in the Down comment rather
   than implying the rollback restores it.

2. **Queries** (`internal/db/queries/<table>.sql`) — **only if the column is writable.**
   Reads need no change: every SELECT is `SELECT *`, so the column flows into the generated
   model for free.
   - `INSERT`: add the column and a positional param.
   - `UPDATE`: add `col = coalesce(sqlc.narg('col'), col)` so a PATCH that omits the field
     leaves the stored value alone instead of blanking it. Do not use a positional `$n` here —
     it breaks partial updates.

3. **Regenerate** — `sqlc generate`. Then READ the generated params struct: sqlc decides
   whether the update param is `pgtype.Text`, `*string` or `string`, and the repository must
   match what it actually produced, not what you expected.

4. **Repository** (`internal/<app>/repository.go`) — extend the `Create`/`Update` signatures.
   Keep the public API in domain types (`summary string`, `summary *string`) and convert
   inside with the existing `textFromPtr` helper. Do not leak `pgtype` upward.

5. **Service** (`internal/<app>/service.go`) — update BOTH the narrow `<app>Repository`
   interface and the methods. The `var _ <app>Repository = (*Repository)(nil)` assertion will
   fail the build until they agree; that is the point.

6. **Schemas** (`internal/<app>/schema.go`) — add the field to the request/response structs
   that should carry it:
   - `*CreateRequest`: value type, with `validate:"..."` constraints.
   - `*UpdateRequest`: pointer type (nil = leave unchanged), and `omitempty` in front of any
     constraint so absence isn't rejected — `validate:"omitempty,max=280"`.
   - `*Response`: value type.

7. **Handler** (`internal/<app>/handler.go`) — pass the field in the `create`/`update` calls
   and add it to `toResponse`. No swag annotation edits: the spec is derived from the structs.

8. **Tests** — the compiler will point at every fake that implements the narrow interface
   (`service_test.go`, `handler_test.go`); update them and any call site. Then add a real
   assertion, not just a compile fix:
   - an integration test that the value round-trips, and
   - that a partial update omitting the field preserves it (this is what the `sqlc.narg`
     form buys, and it silently regresses if someone switches to `$n`).

9. **Regenerate docs** — `make docs`.

## Verify

```bash
gofmt -l .            # must print nothing
go build ./...
go vet ./... && go vet -tags=integration ./...
golangci-lint run
go test -race ./...
go test -race -tags=integration ./...   # see docs/local-development.md for DOCKER_HOST on colima
```

CI fails on stale generated code, so both must be committed:

```bash
sqlc generate && git diff --exit-code -- internal/db/sqlc/
swag init -g cmd/api/main.go -o internal/httpserver/docs && git diff --exit-code -- internal/httpserver/docs/
```

Run those AFTER committing — against uncommitted work they always report a diff and tell you
nothing.

## Applying it to a database

`sqlc generate` updates Go types; it does not touch any database. These are separate:

- `make up` — the Compose `migrate` service applies pending migrations (easiest in dev)
- `make migrate` — runs the goose CLI **on your host**, so it needs goose installed and
  `DATABASE_URL` pointing at `localhost`

## Never

- Hand-edit anything under `internal/db/sqlc/` or `internal/httpserver/docs/`. Both say
  "DO NOT EDIT" and are overwritten on the next generate; CI's drift check catches it anyway.
- Edit an already-applied migration. Add a new one — the old file's checksum is recorded in
  `goose_db_version` on every database that ran it.
