// Package config reads process configuration from the environment. Nothing
// here reaches out to a network or opens a connection — see internal/db.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the whole of this service's configuration. Three database URLs,
// because the application never connects as the schema owner: erp_app for
// requests, erp_admin for platform administration, erp_migrate for migrations
// only.
type Config struct {
	DatabaseURL        string // erp_app  — RLS applies
	AdminDatabaseURL   string // erp_admin
	MigrateDatabaseURL string // erp_migrate — migrations only

	FirebaseProjectID string

	Port        string
	CORSOrigins []string
}

// PinUTC forces the process's local timezone to UTC (Decision 003 §2.5.2).
//
// The deployment sets TZ=UTC, but a developer's laptop does not, and every
// place an instant becomes a business date — document number periods above
// all — reads the local zone. Pinning it in code means the laptop and the
// deployment cannot disagree. Test J2 asserts it.
func PinUTC() { time.Local = time.UTC }

// Load reads backend/.env when present, then the environment, which wins.
// A missing .env is not an error: in Cloud Run there is no such file.
func Load() (*Config, error) {
	PinUTC()
	_ = godotenv.Load()

	c := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		AdminDatabaseURL:   os.Getenv("ADMIN_DATABASE_URL"),
		MigrateDatabaseURL: os.Getenv("MIGRATE_DATABASE_URL"),
		FirebaseProjectID:  os.Getenv("FIREBASE_PROJECT_ID"),
		Port:               envOr("PORT", "8080"),
		CORSOrigins:        splitCSV(envOr("CORS_ORIGINS", "http://localhost:5173")),
	}

	// MIGRATE_DATABASE_URL is deliberately absent from this list: it is the
	// schema owner's credential, and only cmd/migrate has any use for it.
	// Requiring it here would mean the deployed API service carried a
	// connection string that can DROP its own tables in order to boot — see
	// requireMigrateURL, which is how cmd/migrate demands it instead.
	for _, missing := range []struct {
		key   string
		value string
	}{
		{"DATABASE_URL", c.DatabaseURL},
		{"ADMIN_DATABASE_URL", c.AdminDatabaseURL},
		// Required from Phase 2: without it the Admin SDK cannot check a
		// token's audience, and the API cannot serve one authenticated request.
		{"FIREBASE_PROJECT_ID", c.FirebaseProjectID},
	} {
		if missing.value == "" {
			return nil, fmt.Errorf("config: %s is required (see backend/.env.example)", missing.key)
		}
	}

	return c, nil
}

// LoadFrom reads an extra env file first, overriding the process environment,
// and then Load.
//
// The deployment commands — migrate, seed, dbverify — take a path as an
// optional argument so that pointing one at the deployed database is
// `go run ./cmd/migrate .env.production` rather than five exported variables
// in whichever shell the operator happens to be in. An empty path is Load.
func LoadFrom(path string) (*Config, error) {
	if path != "" {
		if err := godotenv.Overload(path); err != nil {
			return nil, fmt.Errorf("config: read %s: %w", path, err)
		}
	}
	return Load()
}

// LoadCLI is LoadFrom with the path taken from the command line. The three
// deployment commands accept an optional env file as their only argument:
//
//	go run ./cmd/migrate                  # backend/.env, the local database
//	go run ./cmd/migrate .env.production  # the deployed one
//
// The API deliberately does not: a server takes its configuration from its
// environment, and Cloud Run has no files to point at.
func LoadCLI() (*Config, error) {
	var path string
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	return LoadFrom(path)
}

// RequireMigrateURL returns the schema owner's connection string, or an error
// naming it. cmd/migrate calls this; nothing else may.
func (c *Config) RequireMigrateURL() (string, error) {
	if c.MigrateDatabaseURL == "" {
		return "", fmt.Errorf("config: MIGRATE_DATABASE_URL is required to run migrations (see backend/.env.example)")
	}
	return c.MigrateDatabaseURL, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
