package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DGosal/mini-erp/backend/internal/config"
)

// setAPIEnv sets exactly what a deployed API service is given: two connection
// strings and a Firebase project. No MIGRATE_DATABASE_URL — the deployed
// service must not hold the schema owner's credential.
func setAPIEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://erp_app:x@localhost:5432/erp")
	t.Setenv("ADMIN_DATABASE_URL", "postgres://erp_admin:x@localhost:5432/erp")
	t.Setenv("FIREBASE_PROJECT_ID", "erp-project-b66ce")
	t.Setenv("MIGRATE_DATABASE_URL", "")
}

// The API boots without the schema owner's connection string. This is the
// deployment shape: Cloud Run gets DATABASE_URL and ADMIN_DATABASE_URL from
// Secret Manager, and MIGRATE_DATABASE_URL is never attached to the service.
func TestLoadDoesNotRequireTheMigrateURL(t *testing.T) {
	setAPIEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() with no MIGRATE_DATABASE_URL: %v", err)
	}
	if cfg.MigrateDatabaseURL != "" {
		t.Errorf("MigrateDatabaseURL = %q, want empty", cfg.MigrateDatabaseURL)
	}
}

// …and cmd/migrate is refused, by name, rather than dialling an empty string
// and reporting a driver error.
func TestRequireMigrateURLRefusesWhenUnset(t *testing.T) {
	setAPIEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.RequireMigrateURL(); err == nil {
		t.Fatal("RequireMigrateURL() succeeded with MIGRATE_DATABASE_URL unset")
	} else if !strings.Contains(err.Error(), "MIGRATE_DATABASE_URL") {
		t.Errorf("error does not name the variable: %v", err)
	}
}

func TestRequireMigrateURLReturnsItWhenSet(t *testing.T) {
	setAPIEnv(t)
	const want = "postgres://erp_migrate:x@localhost:5432/erp"
	t.Setenv("MIGRATE_DATABASE_URL", want)

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.RequireMigrateURL()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("RequireMigrateURL() = %q, want %q", got, want)
	}
}

// The deployment commands point at a deployed database with an env file, and
// its values must beat whatever is already exported — otherwise a shell that
// still has the local DATABASE_URL in it would migrate the wrong database while
// reporting the right filename.
func TestLoadFromOverridesTheEnvironment(t *testing.T) {
	setAPIEnv(t)

	path := filepath.Join(t.TempDir(), ".env.production")
	if err := os.WriteFile(path, []byte(
		"DATABASE_URL=postgres://erp_app:x@neon/erp?sslmode=require\n"+
			"MIGRATE_DATABASE_URL=postgres://erp_migrate:x@neon/erp?sslmode=require\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.DatabaseURL, "neon") {
		t.Errorf("DatabaseURL = %q, want the file's value", cfg.DatabaseURL)
	}
	if _, err := cfg.RequireMigrateURL(); err != nil {
		t.Errorf("RequireMigrateURL() after LoadFrom: %v", err)
	}
	// Untouched by the file, so it still comes from the environment.
	if cfg.AdminDatabaseURL == "" {
		t.Error("AdminDatabaseURL was dropped by LoadFrom")
	}
}

func TestLoadFromNamesAMissingFile(t *testing.T) {
	setAPIEnv(t)

	_, err := config.LoadFrom(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil {
		t.Fatal("LoadFrom succeeded on a file that does not exist")
	}
	if !strings.Contains(err.Error(), "nope.env") {
		t.Errorf("error does not name the file: %v", err)
	}
}

// The two the API genuinely cannot start without are still refused by name.
func TestLoadStillRequiresTheAppCredentials(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "ADMIN_DATABASE_URL", "FIREBASE_PROJECT_ID"} {
		t.Run(key, func(t *testing.T) {
			setAPIEnv(t)
			t.Setenv(key, "")

			if _, err := config.Load(); err == nil {
				t.Fatalf("Load() succeeded with %s unset", key)
			} else if !strings.Contains(err.Error(), key) {
				t.Errorf("error does not name %s: %v", key, err)
			}
		})
	}
}

// CORS_ORIGINS is a comma-separated list at Phase 9: the deployed API allows
// the two Firebase Hosting origins and nothing else. No wildcard (§2.3).
func TestCORSOriginsSplitsAndTrims(t *testing.T) {
	setAPIEnv(t)
	t.Setenv("CORS_ORIGINS", "https://erp-project-b66ce.web.app, https://erp-project-b66ce.firebaseapp.com ,")

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://erp-project-b66ce.web.app",
		"https://erp-project-b66ce.firebaseapp.com",
	}
	if len(cfg.CORSOrigins) != len(want) {
		t.Fatalf("CORSOrigins = %q, want %q", cfg.CORSOrigins, want)
	}
	for i := range want {
		if cfg.CORSOrigins[i] != want[i] {
			t.Errorf("CORSOrigins[%d] = %q, want %q", i, cfg.CORSOrigins[i], want[i])
		}
	}
}
