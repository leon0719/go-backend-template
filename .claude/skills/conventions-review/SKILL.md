---
name: conventions-review
description: Review the current diff (or a named package) against THIS project's layering and backend conventions — IDOR, layering leaks, migration safety, task idempotency, logging, error format, rate limiting, generated-code drift. Use when the user asks to "review", "code review", "審查", "check conventions", or "review my change".
---

# Conventions review

Complements the general-purpose `/code-review` (which hunts bugs) by enforcing the rules this
specific project has decided on. Read the diff (`git diff`, or `git diff main...HEAD`), then
work the checklist. Report file:line for every finding and say plainly when something is fine.

Rank findings by what would actually go wrong. An IDOR is not the same kind of problem as an
inconsistent field name, and calling both "issues" wastes the reader's attention.

## Security

- [ ] **Every owner-scoped query carries `AND user_id = $n`.** Check each one individually —
      one missing clause is an IDOR the layers above cannot fix.
- [ ] **Not-found and not-owned are indistinguishable**: both 404, and a malformed UUID 404s
      too. A 403 or a 400 tells an attacker which ids exist.
- [ ] Request bodies are decoded via `request.DecodeAndValidate` (which imposes the size limit),
      not a bare `json.NewDecoder`.
- [ ] Nothing trusts `X-Forwarded-For` outside `middleware.RealIP`'s trusted-proxy check.
- [ ] No secret, token, password hash or full request body reaches a log line.

## Layering

- [ ] `handler.go` contains no business logic; `service.go` imports no HTTP types; nothing
      above `repository.go` mentions `pgtype` or `sqlc` params.
- [ ] The service depends on a **narrow interface** with a `var _ ... = (*Repository)(nil)`
      assertion, not on `*Repository` directly.
- [ ] Errors are translated at the repository boundary via `internal/db/dberr` — a raw pgx or
      driver error never escapes upward.
- [ ] Shared helpers are reused rather than re-implemented: `request.DecodeAndValidate`,
      `dberr.WrapNotFound`, `respond.Code*`, `textFromPtr`. A second copy drifts.

## Database

- [ ] Migrations are additive and numbered past the current highest. **An already-applied
      migration is never edited** — its checksum is recorded in `goose_db_version`.
- [ ] `NOT NULL` columns added to an existing table have a `DEFAULT`.
- [ ] Down migrations are honest: if Up destroys data, Down says it cannot restore it.
- [ ] Partial updates use `coalesce(sqlc.narg('col'), col)`, not a positional `$n` that blanks
      omitted fields.
- [ ] List and count queries share the same WHERE clause.
- [ ] Invariants the application relies on are enforced by constraints, not only in Go.
- [ ] New indexes match a query path that actually exists.

## Background tasks

- [ ] Handlers are idempotent: payloads carry IDs, everything else is re-derived. `acks_late`
      semantics mean a task can run twice.
- [ ] Permanent failures (malformed payload, 4xx from a webhook) are wrapped with
      `asynq.SkipRetry`; transient ones (network, 5xx) return a plain error so they retry.
- [ ] Outbound HTTP uses a client with an explicit timeout.
- [ ] Dual writes are acknowledged: if a task enqueue fails after the DB commit, the code says
      what happens rather than pretending the two are atomic.

## HTTP

- [ ] Errors use `respond.Error` with a `respond.Code*` constant — no bare strings.
- [ ] Pagination is clamped in the handler (page < 1, pageSize < 1, max cap). The service does
      not validate it and a negative OFFSET is a 500.
- [ ] Writes are rate-limited per-user; anonymous endpoints per-IP.
- [ ] New routes are mounted on the shared middleware chain, not on a bare router.

## Observability

- [ ] `log/slog` only. 4xx → Warn, 5xx → Error.
- [ ] No second access-log line per request; the chain already emits one.
- [ ] Config flows through `internal/config` — no `os.Getenv` outside it.

## Tests

- [ ] Tests assert behaviour, not that a mock was called. A test that cannot fail is worse
      than no test.
- [ ] Owner-scoped resources have a real cross-user test through **one** router instance.
- [ ] Integration tests build the schema with `db.MigrateUp`, never a hand-written
      `CREATE TABLE` that will drift from the migrations.
- [ ] Postgres containers pass `postgres.BasicWaitStrategies()`.
- [ ] Test output is clean — stray warnings are a finding.

## Generated code

- [ ] `internal/db/sqlc/` and `internal/httpserver/docs/` are regenerated and committed. Both
      are marked DO-NOT-EDIT and CI fails on drift.
- [ ] Nothing under either tree was hand-edited.
