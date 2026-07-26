package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
)

// Skips rather than fails when the database is not up: `go test ./...` should
// still be runnable without Docker. The full testcontainers harness arrives
// with the schema it needs to test.
func appDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://erp_app:localdev@localhost:5432/erp?sslmode=disable"
	}
	pools := db.NewPools(url, url)
	g, err := pools.App()
	if err != nil {
		t.Skipf("no database available: %v", err)
	}
	t.Cleanup(func() { _ = pools.Close() })
	return g
}

func TestWithTenantSetsCurrentTenant(t *testing.T) {
	g := appDB(t)
	tenantID := uuid.New()

	var seen string
	err := db.WithTenant(context.Background(), g, tenantID, func(tx *gorm.DB) error {
		return tx.Raw("SELECT current_setting('app.current_tenant', true)").Scan(&seen).Error
	})
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
	if seen != tenantID.String() {
		t.Fatalf("app.current_tenant = %q, want %q", seen, tenantID)
	}
}

// I2: the setting must not survive the transaction. If it does, a pooled
// connection carries one tenant's context into the next request.
func TestWithTenantDoesNotLeakAfterCommit(t *testing.T) {
	g := appDB(t)

	if err := db.WithTenant(context.Background(), g, uuid.New(), func(tx *gorm.DB) error {
		return nil
	}); err != nil {
		t.Fatalf("WithTenant: %v", err)
	}

	// Pool size is 1 for this check, so the follow-up query is guaranteed to
	// reuse the same connection the transaction ran on.
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	var after string
	if err := g.Raw("SELECT current_setting('app.current_tenant', true)").Scan(&after).Error; err != nil {
		t.Fatalf("read setting: %v", err)
	}
	if after != "" {
		t.Fatalf("app.current_tenant leaked out of the transaction: %q", after)
	}
}
