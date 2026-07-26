// Command migrate applies the schema migrations as erp_migrate, the only role
// that owns DDL. It is a separate process from the API on purpose: a running
// service must never be able to change its own schema.
package main

import (
	"context"
	"log"

	"github.com/DGosal/mini-erp/backend/internal/config"
	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/migrations"
)

func main() {
	// An optional env file, so pointing this at a deployed database is
	// `go run ./cmd/migrate .env.production` rather than three exported
	// variables. See config.LoadFrom.
	cfg, err := config.LoadCLI()
	if err != nil {
		log.Fatal(err)
	}

	migrateURL, err := cfg.RequireMigrateURL()
	if err != nil {
		log.Fatal(err)
	}

	sqlDB, err := db.OpenSQL(migrateURL)
	if err != nil {
		log.Fatalf("migrate: open: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	ctx := context.Background()

	applied, err := db.Apply(ctx, sqlDB, migrations.FS)
	if err != nil {
		log.Fatal(err)
	}
	for _, name := range applied {
		log.Printf("migrate: applied %s", name)
	}

	// Re-applied on every run. Its platform-table grants cannot be applied on
	// the container's first boot, when it runs from docker-entrypoint-initdb.d
	// and the tables do not exist yet. The file is idempotent.
	if err := db.ExecFile(ctx, sqlDB, migrations.FS, migrations.RolesFile); err != nil {
		log.Fatal(err)
	}
	log.Printf("migrate: re-applied %s", migrations.RolesFile)

	if len(applied) == 0 {
		log.Print("migrate: schema already up to date")
	}
}
