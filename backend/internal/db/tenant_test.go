package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// The helper itself: WithTenant must actually put the tenant where the
// policies look for it. Everything in Group A rests on this one statement.
func TestWithTenantSetsCurrentTenant(t *testing.T) {
	d := testsupport.NewTestDB(t)
	tenantID := uuid.New()

	var seen string
	err := db.WithTenant(context.Background(), d.App, tenantID, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT current_setting('app.current_tenant', true)`).Scan(&seen).Error
	})
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
	if seen != tenantID.String() {
		t.Fatalf("app.current_tenant = %q, want %q", seen, tenantID)
	}
}

// A rolled-back transaction must not leave the context behind either.
func TestWithTenantClearsContextOnRollback(t *testing.T) {
	d := testsupport.NewTestDB(t)
	pool := d.NewAppPool(t, 1)

	wantErr := context.Canceled
	if err := db.WithTenant(context.Background(), pool, uuid.New(),
		func(tx *gorm.DB) error { return wantErr }); err != wantErr {
		t.Fatalf("WithTenant swallowed the error: %v", err)
	}

	var after string
	if err := pool.Raw(`SELECT current_setting('app.current_tenant', true)`).
		Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != "" {
		t.Fatalf("app.current_tenant survived a rollback: %q", after)
	}
}

// commitTenantTx runs an empty tenant transaction to completion, so a test can
// then look at what the connection carries afterwards.
func commitTenantTx(t *testing.T, g *gorm.DB, tenantID uuid.UUID) error {
	t.Helper()
	return db.WithTenant(context.Background(), g, tenantID,
		func(tx *gorm.DB) error { return nil })
}
