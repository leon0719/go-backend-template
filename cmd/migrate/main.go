// Command migrate applies (or rolls back) goose SQL migrations embedded from
// internal/db/migrations. It exists so the prod Docker image can run
// migrations without needing a shell, a separate goose binary fetched at
// build time, or the migrations directory mounted at runtime — the
// migration files are embedded straight into this binary.
//
// Usage:
//
//	migrate up            # apply all pending migrations (default)
//	migrate down           # roll back the most recent migration
//	migrate status          # print migration status
//
// Reads DATABASE_URL from the environment (same variable the api/worker
// binaries use).
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"go-backend-template/internal/config"
	"go-backend-template/internal/db/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}

func run() error {
	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	dbCfg, err := config.LoadDatabaseOnly()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	db, err := sql.Open("pgx", dbCfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	switch command {
	case "up":
		return goose.Up(db, ".")
	case "down":
		return goose.Down(db, ".")
	case "status":
		return goose.Status(db, ".")
	default:
		return fmt.Errorf("unknown command %q (expected up|down|status)", command)
	}
}
