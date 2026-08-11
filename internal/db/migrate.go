package db

import (
	"database/sql"
	"fmt"

	// Registers the "pgx" database/sql driver that goose connects through.
	// pgxpool (used everywhere else) is not a database/sql pool, and goose
	// needs the database/sql interface.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"go-backend-template/internal/db/migrations"
)

// MigrateUp applies every pending goose migration embedded in
// internal/db/migrations against databaseURL.
//
// It backs both cmd/migrate (the one-off job the Compose stacks run before the
// API starts) and the integration tests, which apply the real migrations to
// their throwaway container rather than a hand-maintained copy of the schema.
// Sharing one implementation is the point: a test schema written out by hand
// drifts from production silently, and the drift only surfaces as a test that
// passes against a table shape that no longer exists.
func MigrateUp(databaseURL string) error {
	return withGoose(databaseURL, func(sqlDB *sql.DB) error {
		return goose.Up(sqlDB, ".")
	})
}

// MigrateDown rolls back the most recently applied migration.
func MigrateDown(databaseURL string) error {
	return withGoose(databaseURL, func(sqlDB *sql.DB) error {
		return goose.Down(sqlDB, ".")
	})
}

// MigrateStatus prints the applied/pending state of every migration.
func MigrateStatus(databaseURL string) error {
	return withGoose(databaseURL, func(sqlDB *sql.DB) error {
		return goose.Status(sqlDB, ".")
	})
}

func withGoose(databaseURL string, fn func(*sql.DB) error) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	return fn(sqlDB)
}
