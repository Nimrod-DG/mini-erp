// Package testsupport starts a real PostgreSQL for the test suite and hands
// back one connection per database role.
//
// RLS cannot be tested against a mock, an in-memory fake, or SQLite: the
// isolation guarantee is a property of PostgreSQL policy evaluation, and a
// suite that stubs the database proves nothing about the mechanism this
// project exists to demonstrate (§12.2).
//
// Tests run as erp_app, never as the owner. A suite connected as the owner
// silently bypasses policies and passes while production leaks.
package testsupport

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/migrations"
)

// TestDB is one migrated database with a connection per role.
type TestDB struct {
	// Owner is erp_migrate: the schema owner, and the container's superuser.
	// Use it only where a test must not be blocked by RLS — A9's composite-FK
	// check, and the ledger tables erp_app is revoked from.
	Owner *gorm.DB
	// App is erp_app, the role every request uses. RLS applies. Default choice.
	App *gorm.DB
	// Admin is erp_admin, the superadmin console role: platform tables only.
	Admin *gorm.DB

	OwnerURL, AppURL, AdminURL string
}

var (
	once      sync.Once
	shared    *TestDB
	sharedErr error
	container testcontainers.Container
)

// NewTestDB returns the package's database, starting and migrating it on first
// call. One container per test process: starting Postgres per test would
// dominate the runtime and prove nothing extra.
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()
	once.Do(func() { shared, sharedErr = start(context.Background()) })
	if sharedErr != nil {
		t.Fatalf("testsupport: %v", sharedErr)
	}
	return shared
}

// Shutdown terminates the container. Call it from TestMain.
func Shutdown() {
	if container != nil {
		_ = container.Terminate(context.Background())
	}
}

func start(ctx context.Context) (*TestDB, error) {
	pg, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("erp"),
		tcpostgres.WithUsername("erp_migrate"),
		tcpostgres.WithPassword("localdev"),
		// Mirrors docker-compose.yml. The role-level `ALTER ROLE ... SET
		// timezone` in 000_roles.sql is the belt to this braces (J1).
		testcontainers.WithEnv(map[string]string{"TZ": "UTC", "PGTZ": "UTC"}),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres (is Docker running?): %w", err)
	}
	container = pg

	host, err := pg.Host(ctx)
	if err != nil {
		return nil, err
	}
	port, err := pg.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, err
	}

	url := func(role string) string {
		return fmt.Sprintf("postgres://%s:localdev@%s:%s/erp?sslmode=disable",
			role, host, port.Port())
	}
	tdb := &TestDB{
		OwnerURL: url("erp_migrate"),
		AppURL:   url("erp_app"),
		AdminURL: url("erp_admin"),
	}

	sqlDB, err := db.OpenSQL(tdb.OwnerURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sqlDB.Close() }()

	// Exactly the sequence `make migrate` performs against a fresh container:
	// roles first (its grants no-op while the tables are absent), then the
	// schema, then roles again so the platform-table grants land.
	if err := db.ExecFile(ctx, sqlDB, migrations.FS, migrations.RolesFile); err != nil {
		return nil, err
	}
	if _, err := db.Apply(ctx, sqlDB, migrations.FS); err != nil {
		return nil, err
	}
	if err := db.ExecFile(ctx, sqlDB, migrations.FS, migrations.RolesFile); err != nil {
		return nil, err
	}

	for _, conn := range []struct {
		url string
		out **gorm.DB
	}{
		{tdb.OwnerURL, &tdb.Owner},
		{tdb.AppURL, &tdb.App},
		{tdb.AdminURL, &tdb.Admin},
	} {
		g, err := gorm.Open(gormpostgres.Open(conn.url), &gorm.Config{
			// Expected failures are the point of most of these tests; GORM's
			// default logger would print each one as an error.
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return nil, err
		}
		*conn.out = g
	}
	return tdb, nil
}

// NewAppPool opens a private erp_app pool with a fixed connection limit, for
// the tests that need to reason about which physical connection a query lands
// on. Capping the shared pool instead would leak into every later test.
func (d *TestDB) NewAppPool(t *testing.T, maxConns int) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(gormpostgres.Open(d.AppURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testsupport: open app pool: %v", err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatalf("testsupport: %v", err)
	}
	sqlDB.SetMaxOpenConns(maxConns)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return g
}

// AsTenant runs fn on the app pool inside a transaction with app.current_tenant
// set — the only way tenant data is ever touched (I1).
func (d *TestDB) AsTenant(t *testing.T, tenantID uuid.UUID, fn func(tx *gorm.DB) error) error {
	t.Helper()
	return db.WithTenant(context.Background(), d.App, tenantID, fn)
}

// MustAsTenant is AsTenant for the arrange half of a test, where a failure is
// a broken fixture rather than a result.
func (d *TestDB) MustAsTenant(t *testing.T, tenantID uuid.UUID, fn func(tx *gorm.DB) error) {
	t.Helper()
	if err := d.AsTenant(t, tenantID, fn); err != nil {
		t.Fatalf("as tenant %s: %v", tenantID, err)
	}
}

// AsOwner runs fn as erp_migrate with tenant context set. Reserved for the two
// cases the app role cannot express: proving a constraint rather than a policy
// is what rejects a write, and touching the ledger tables erp_app is revoked
// from.
func (d *TestDB) AsOwner(t *testing.T, tenantID uuid.UUID, fn func(tx *gorm.DB) error) error {
	t.Helper()
	return db.WithTenant(context.Background(), d.Owner, tenantID, fn)
}
