package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for OpenSQL
)

// migrationsTable records what has already run, which is what makes `make
// migrate` safe to run twice in a row.
const migrationsTable = "schema_migrations"

// OpenSQL dials a database with database/sql rather than GORM. Migrations are
// plain DDL and want nothing GORM provides.
func OpenSQL(url string) (*sql.DB, error) {
	return sql.Open("pgx", url)
}

// Apply runs every pending versioned migration in fsys, in filename order,
// each inside its own transaction — PostgreSQL DDL is transactional, so a
// migration either lands whole or not at all.
//
// Files named 000_* are skipped: 000_roles.sql is applied outside the
// versioned sequence, before the tables it grants on exist. See
// migrations.RolesFile.
//
// Returns the versions applied by this call, which is empty on a second run.
func Apply(ctx context.Context, sqlDB *sql.DB, fsys fs.FS) ([]string, error) {
	if _, err := sqlDB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+migrationsTable+` (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, fmt.Errorf("migrate: create %s: %w", migrationsTable, err)
	}

	done, err := appliedVersions(ctx, sqlDB)
	if err != nil {
		return nil, err
	}

	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("migrate: list migrations: %w", err)
	}
	sort.Strings(names)

	var applied []string
	for _, name := range names {
		if strings.HasPrefix(name, "000_") || done[name] {
			continue
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return applied, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		if err := applyOne(ctx, sqlDB, name, string(body)); err != nil {
			return applied, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}

// ExecFile runs one SQL file from fsys outside the versioned sequence, every
// time it is called. Only 000_roles.sql is applied this way: it has to run
// once before the tables exist (to create the roles) and again afterwards (to
// grant on the platform tables), so it is written to be idempotent.
func ExecFile(ctx context.Context, sqlDB *sql.DB, fsys fs.FS, name string) error {
	body, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("migrate: read %s: %w", name, err)
	}
	if _, err := sqlDB.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("migrate: %s: %w", name, err)
	}
	return nil
}

func applyOne(ctx context.Context, sqlDB *sql.DB, name, body string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: begin %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("migrate: %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO `+migrationsTable+` (version) VALUES ($1)`, name); err != nil {
		return fmt.Errorf("migrate: record %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migrate: commit %s: %w", name, err)
	}
	return nil
}

func appliedVersions(ctx context.Context, sqlDB *sql.DB) (map[string]bool, error) {
	rows, err := sqlDB.QueryContext(ctx, `SELECT version FROM `+migrationsTable)
	if err != nil {
		return nil, fmt.Errorf("migrate: read %s: %w", migrationsTable, err)
	}
	defer func() { _ = rows.Close() }()

	done := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		done[v] = true
	}
	return done, rows.Err()
}
