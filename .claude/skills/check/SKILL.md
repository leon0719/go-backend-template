---
name: check
description: Run the full local quality gate for this project — gofmt, go vet, golangci-lint, unit tests, integration tests, and the generated-code drift checks CI enforces. Use when the user asks to "run checks", "check the code", "品質檢查", "跑測試", "make sure it passes", or before committing/finishing a change.
---

# Quality gate

Run these in order and stop at the first failure — a vet error usually explains a confusing
test failure further down.

## 1. Format, build, vet

```bash
gofmt -l .        # prints offending files; must be empty. `make format` fixes them.
go build ./...
go vet ./...
go vet -tags=integration ./...   # integration files are invisible to the untagged run
```

The second vet matters: integration tests only compile under their build tag, so a broken one
sails through `go vet ./...` and fails in CI instead.

## 2. Lint

```bash
golangci-lint run
```

Uses the checked-in `.golangci.yml` (gosec, errorlint, bodyclose, revive, …). The version CI
pins is in `.github/workflows/ci.yml`; a different local version can legitimately disagree, so
match it before concluding CI is wrong.

## 3. Tests

```bash
go test ./...                    # unit; no Docker needed
go test -tags=integration ./...  # spins up Postgres/Redis via testcontainers
```

On colima the integration run needs the daemon pointed out explicitly, or it fails with
`rootless Docker not found`:

```bash
export DOCKER_HOST="unix://$HOME/.colima/default/docker.sock"
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
```

## 4. Generated-code drift

CI regenerates and fails if the committed output differs:

```bash
sqlc generate && git diff --exit-code -- internal/db/sqlc/
swag init -g cmd/api/main.go -o internal/httpserver/docs && git diff --exit-code -- internal/httpserver/docs/
```

**Only meaningful against committed work.** Run before committing and they always report a
diff — that is your own uncommitted change, not drift.

## Reporting

Say plainly what ran and what happened. If something failed, quote the actual output rather
than summarizing it, and do not describe the gate as passing because the remaining steps would
have passed. If you skipped a step (no Docker, lint not installed), say which and why — a
silent skip reads as a pass.
