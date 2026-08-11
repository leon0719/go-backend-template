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
	"fmt"
	"log"
	"os"

	"go-backend-template/internal/config"
	"go-backend-template/internal/db"
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

	switch command {
	case "up":
		return db.MigrateUp(dbCfg.DatabaseURL)
	case "down":
		return db.MigrateDown(dbCfg.DatabaseURL)
	case "status":
		return db.MigrateStatus(dbCfg.DatabaseURL)
	default:
		return fmt.Errorf("unknown command %q (expected up|down|status)", command)
	}
}
