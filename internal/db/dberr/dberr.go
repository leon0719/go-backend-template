// Package dberr holds the database-error translation shared by every app's
// repository.go: the sentinel "not found" domain error, and helpers for
// recognising Postgres constraint violations. Defining these once (rather
// than copy-pasting an ErrNotFound + wrapNotFound pair into each app) keeps
// the repository layer consistent as new apps are added.
package dberr

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound is the domain-level "row does not exist" error. Repositories
// translate pgx.ErrNoRows into it so services and handlers never import pgx.
var ErrNotFound = errors.New("not found")

// Postgres SQLSTATE codes worth recognising in repositories.
// See https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
)

// WrapNotFound converts pgx.ErrNoRows into ErrNotFound and passes anything
// else (including nil) through unchanged.
func WrapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// IsUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505). Letting the database enforce uniqueness and
// mapping the resulting error is race-free; a SELECT-then-INSERT pre-check is
// not, and surfaces as a generic 500 under concurrency.
func IsUniqueViolation(err error) bool {
	return hasSQLState(err, codeUniqueViolation)
}

// IsForeignKeyViolation reports whether err is a Postgres foreign-key
// violation (SQLSTATE 23503).
func IsForeignKeyViolation(err error) bool {
	return hasSQLState(err, codeForeignKeyViolation)
}

func hasSQLState(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
