package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"go-backend-template/internal/db/sqlc"
)

// WithinTx runs fn inside a single Postgres transaction: every query fn
// issues through the *sqlc.Queries it receives commits together, or -- if fn
// returns a non-nil error or panics -- rolls back together. This is the
// sqlc/pgx equivalent of Django's `with transaction.atomic()`, and it is
// shared across every app's repository.go so none of them hand-roll
// Begin/defer-Rollback/Commit bookkeeping themselves.
//
// pgx.BeginFunc does that bookkeeping: commit on a nil return, rollback
// (and re-panic) otherwise. See articles.Repository.ArchiveWithEvent for a
// caller, and docs/backend-standards.md ("Atomic multi-write transactions")
// for when to reach for this versus accepting a dual-write gap.
func WithinTx(ctx context.Context, pool *pgxpool.Pool, fn func(q *sqlc.Queries) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return fn(sqlc.New(tx))
	})
}
