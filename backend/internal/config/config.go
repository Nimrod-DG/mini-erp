// Package config reads process configuration from the environment. Nothing
// here reaches out to a network or opens a connection — see internal/db.
package config

import (
	"fmt"
	"os"
	"strings"

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

// Load reads backend/.env when present, then the environment, which wins.
// A missing .env is not an error: in Cloud Run there is no such file.
func Load() (*Config, error) {
	_ = godotenv.Load()

	c := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		AdminDatabaseURL:   os.Getenv("ADMIN_DATABASE_URL"),
		MigrateDatabaseURL: os.Getenv("MIGRATE_DATABASE_URL"),
		FirebaseProjectID:  os.Getenv("FIREBASE_PROJECT_ID"),
		Port:               envOr("PORT", "8080"),
		CORSOrigins:        splitCSV(envOr("CORS_ORIGINS", "http://localhost:5173")),
	}

	for _, missing := range []struct {
		key   string
		value string
	}{
		{"DATABASE_URL", c.DatabaseURL},
		{"ADMIN_DATABASE_URL", c.AdminDatabaseURL},
		{"MIGRATE_DATABASE_URL", c.MigrateDatabaseURL},
	} {
		if missing.value == "" {
			return nil, fmt.Errorf("config: %s is required (see backend/.env.example)", missing.key)
		}
	}

	return c, nil
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
