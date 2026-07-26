package testsupport

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seq keeps slugs, emails, SKUs, and document numbers unique across every
// fixture in a process without the tests having to coordinate.
var seq atomic.Int64

func next() int64 { return seq.Add(1) }

// TenantFixture is one tenant with all three modules entitled and enough
// master data to build a document against.
type TenantFixture struct {
	db *TestDB

	ID       uuid.UUID
	Name     string
	Slug     string
	Timezone string

	WarehouseID  uuid.UUID
	ProductID    uuid.UUID
	ProductAltID uuid.UUID // a second product, for tests that need two
	SupplierID   uuid.UUID

	InventoryAccountID uuid.UUID // 1300 asset
	GRNIAccountID      uuid.UUID // 2150 liability

	// User is a staff user with admin level in every module — the default
	// actor for created_by / requested_by. NewUser makes more.
	User *UserFixture
}

// UserFixture is one row in `users` plus its per-module levels.
type UserFixture struct {
	ID          uuid.UUID
	FirebaseUID string
	Email       string
	TenantRole  string
	Roles       map[string]string
}

// NewTenant creates a tenant, entitles all three modules, seeds the chart of
// accounts, master data, and a default user.
//
// Every test that touches tenant data creates TWO of these and asserts against
// both: a single-tenant test cannot detect an isolation failure (§12.2).
func (d *TestDB) NewTenant(t *testing.T, name string) *TenantFixture {
	t.Helper()
	return d.NewTenantInTZ(t, name, "Asia/Jakarta")
}

// NewTenantInTZ is NewTenant with an explicit business timezone, for the
// Group J tests where the tenant's zone is the thing under test.
func (d *TestDB) NewTenantInTZ(t *testing.T, name, timezone string) *TenantFixture {
	t.Helper()
	n := next()

	f := &TenantFixture{
		db:       d,
		ID:       uuid.New(),
		Name:     name,
		Slug:     fmt.Sprintf("%s-%d", "tenant", n),
		Timezone: timezone,
	}

	// Platform tables carry no RLS, so these go in as the owner with no tenant
	// context — exactly as the superadmin console would.
	mustExec(t, d.Owner, `
		INSERT INTO tenants (id, name, slug, timezone) VALUES (?, ?, ?, ?)`,
		f.ID, name, f.Slug, timezone)
	mustExec(t, d.Owner, `
		INSERT INTO tenant_modules (tenant_id, module_code, enabled)
		SELECT ?, code, true FROM modules`, f.ID)

	// Through the SECURITY DEFINER function (§4.2.1), from the admin pool —
	// which is revoked from `accounts` and could not do this directly.
	mustExec(t, d.Admin, `SELECT seed_tenant_accounts(?)`, f.ID)

	f.User = f.NewUser(t, map[string]string{
		"procurement": "admin", "inventory": "admin", "finance": "admin",
	})

	f.WarehouseID = uuid.New()
	f.ProductID = uuid.New()
	f.ProductAltID = uuid.New()
	f.SupplierID = uuid.New()

	d.MustAsTenant(t, f.ID, func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO warehouses (id, tenant_id, code, name)
			VALUES (?, ?, ?, ?)`,
			f.WarehouseID, f.ID, fmt.Sprintf("WH-%d", n), "Main warehouse").Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO products (id, tenant_id, sku, name, standard_cost)
			VALUES (?, ?, ?, ?, 10.00), (?, ?, ?, ?, 25.00)`,
			f.ProductID, f.ID, fmt.Sprintf("SKU-%d-A", n), "Product A",
			f.ProductAltID, f.ID, fmt.Sprintf("SKU-%d-B", n), "Product B").Error; err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO suppliers (id, tenant_id, code, name)
			VALUES (?, ?, ?, ?)`,
			f.SupplierID, f.ID, fmt.Sprintf("SUP-%d", n), "Supplier").Error; err != nil {
			return err
		}
		return tx.Raw(`
			SELECT (SELECT id FROM accounts WHERE code = '1300'),
			       (SELECT id FROM accounts WHERE code = '2150')`).
			Row().Scan(&f.InventoryAccountID, &f.GRNIAccountID)
	})

	return f
}

// NewUser creates a staff user in this tenant with the given per-module levels.
// An absent module means `none` — the level is the absence of a row (§5.3).
func (f *TenantFixture) NewUser(t *testing.T, roles map[string]string) *UserFixture {
	t.Helper()
	n := next()

	u := &UserFixture{
		ID:          uuid.New(),
		FirebaseUID: fmt.Sprintf("uid-%d", n),
		Email:       fmt.Sprintf("user-%d@example.test", n),
		TenantRole:  "staff",
		Roles:       roles,
	}
	mustExec(t, f.db.Owner, `
		INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role)
		VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, f.ID, u.FirebaseUID, u.Email, fmt.Sprintf("User %d", n), u.TenantRole)

	for module, level := range roles {
		mustExec(t, f.db.Owner, `
			INSERT INTO user_module_roles (user_id, module_code, role_level)
			VALUES (?, ?, ?)`, u.ID, module, level)
	}
	return u
}

// AsTenant runs fn against this tenant on the app pool, with RLS in force.
func (f *TenantFixture) AsTenant(t *testing.T, fn func(tx *gorm.DB) error) error {
	t.Helper()
	return f.db.AsTenant(t, f.ID, fn)
}

// Must is AsTenant for arrange steps, where a failure is a broken fixture.
func (f *TenantFixture) Must(t *testing.T, fn func(tx *gorm.DB) error) {
	t.Helper()
	f.db.MustAsTenant(t, f.ID, fn)
}

// AsOwner runs fn against this tenant as erp_migrate. See TestDB.AsOwner for
// when that is legitimate.
func (f *TenantFixture) AsOwner(t *testing.T, fn func(tx *gorm.DB) error) error {
	t.Helper()
	return f.db.AsOwner(t, f.ID, fn)
}

// NewRequisition creates a requisition in the given status, filling whichever
// conditional fields that status requires (§6.10.3).
func (f *TenantFixture) NewRequisition(t *testing.T, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var submittedAt, decidedAt *time.Time
	var decidedBy *uuid.UUID
	var rejectReason *string

	now := time.Now().UTC()
	if status != "draft" {
		submittedAt = &now
	}
	if status == "approved" || status == "rejected" {
		decidedAt, decidedBy = &now, &f.User.ID
	}
	if status == "rejected" {
		reason := "not budgeted"
		rejectReason = &reason
	}

	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO purchase_requisitions
			  (id, tenant_id, pr_number, warehouse_id, status, requested_by,
			   submitted_at, decided_by, decided_at, reject_reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, f.ID, fmt.Sprintf("PR-202607-%04d", next()), f.WarehouseID, status,
			f.User.ID, submittedAt, decidedBy, decidedAt, rejectReason).Error
	})
	return id
}

// NewPurchaseOrder creates an `open` purchase order with no lines.
func (f *TenantFixture) NewPurchaseOrder(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO purchase_orders
			  (id, tenant_id, po_number, supplier_id, warehouse_id, created_by)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, f.ID, fmt.Sprintf("PO-202607-%04d", next()),
			f.SupplierID, f.WarehouseID, f.User.ID).Error
	})
	return id
}

// NewPOLine adds a line to a purchase order. Line numbers come from the shared
// counter, so callers never collide on pol_line_no_uq.
func (f *TenantFixture) NewPOLine(t *testing.T, poID, productID uuid.UUID, qty float64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO purchase_order_lines
			  (id, tenant_id, po_id, product_id, qty_ordered, unit_cost, line_no)
			VALUES (?, ?, ?, ?, ?, 10.00, ?)`,
			id, f.ID, poID, productID, qty, next()).Error
	})
	return id
}

// NewGoodsReceipt creates a receipt header with no lines.
func (f *TenantFixture) NewGoodsReceipt(t *testing.T, poID uuid.UUID) uuid.UUID {
	t.Helper()
	n := next()
	id := uuid.New()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO goods_receipts
			  (id, tenant_id, gr_number, po_id, received_by, idempotency_key)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, f.ID, fmt.Sprintf("GR-202607-%04d", n), poID, f.User.ID,
			fmt.Sprintf("test-key-%d", n)).Error
	})
	return id
}

// NewJournalEntry creates a journal entry header with no lines. The balance
// trigger is deferred, so an entry with no lines is legal until commit.
func (f *TenantFixture) NewJournalEntry(t *testing.T, tx *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := tx.Exec(`
		INSERT INTO journal_entries
		  (id, tenant_id, entry_number, source_type, description, created_by)
		VALUES (?, ?, ?, 'manual', 'test entry', ?)`,
		id, f.ID, fmt.Sprintf("JE-202607-%04d", next()), f.User.ID).Error; err != nil {
		t.Fatalf("new journal entry: %v", err)
	}
	return id
}

func mustExec(t *testing.T, g *gorm.DB, query string, args ...any) {
	t.Helper()
	if err := g.Exec(query, args...).Error; err != nil {
		t.Fatalf("fixture: %v", err)
	}
}
