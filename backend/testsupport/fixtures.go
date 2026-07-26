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
	return f.NewUserAs(t, "staff", roles)
}

// NewAdmin creates a tenant admin with **no** user_module_roles rows.
//
// That absence is the correct shape, not a shortcut: an admin holds `admin`
// implicitly in every entitled module, and seeding rows for them would make a
// later demotion restore levels they never chose (§5.4). B7 exists to check
// that this shape resolves to full access, and B8 that it still stops at the
// tenant's entitlements.
func (f *TenantFixture) NewAdmin(t *testing.T) *UserFixture {
	t.Helper()
	return f.NewUserAs(t, "admin", nil)
}

// NewUserAs creates a user with an explicit tenant role and per-module levels.
func (f *TenantFixture) NewUserAs(t *testing.T, tenantRole string, roles map[string]string) *UserFixture {
	t.Helper()
	n := next()

	if roles == nil {
		roles = map[string]string{}
	}
	u := &UserFixture{
		ID:          uuid.New(),
		FirebaseUID: fmt.Sprintf("uid-%d", n),
		Email:       fmt.Sprintf("user-%d@example.test", n),
		TenantRole:  tenantRole,
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

// NewSuperadmin creates a platform superadmin: tenant_role 'superadmin' and no
// tenant, which the users_superadmin_has_no_tenant CHECK requires to go
// together. It belongs to no TenantFixture, hence the method on TestDB.
func (d *TestDB) NewSuperadmin(t *testing.T) *UserFixture {
	t.Helper()
	n := next()

	u := &UserFixture{
		ID:          uuid.New(),
		FirebaseUID: fmt.Sprintf("uid-super-%d", n),
		Email:       fmt.Sprintf("super-%d@example.test", n),
		TenantRole:  "superadmin",
		Roles:       map[string]string{},
	}
	mustExec(t, d.Owner, `
		INSERT INTO users (id, tenant_id, firebase_uid, email, full_name, tenant_role)
		VALUES (?, NULL, ?, ?, ?, 'superadmin')`,
		u.ID, u.FirebaseUID, u.Email, fmt.Sprintf("Superadmin %d", n))
	return u
}

// Deactivate flips is_active off — the only way a user ever leaves this system
// (§6.9.4). Their Firebase account still authenticates; identity resolution is
// what stops them.
func (d *TestDB) Deactivate(t *testing.T, userID uuid.UUID) {
	t.Helper()
	mustExec(t, d.Owner, `UPDATE users SET is_active = false WHERE id = ?`, userID)
}

// Suspend marks the tenant suspended. Its users then reach identity resolution
// and are turned away with 403 tenant_suspended, rather than signing in to an
// application that is inexplicably empty (§9.x).
func (f *TenantFixture) Suspend(t *testing.T) {
	t.Helper()
	mustExec(t, f.db.Owner, `UPDATE tenants SET status = 'suspended' WHERE id = ?`, f.ID)
}

// DisableModule revokes the tenant's entitlement to a module, without touching
// anyone's role level. The two are independent, and /api/me must reflect the
// entitlement, not just the role.
func (f *TenantFixture) DisableModule(t *testing.T, moduleCode string) {
	t.Helper()
	f.SetModule(t, moduleCode, false)
}

// SetModule toggles one entitlement, standing in for the superadmin console's
// PUT /admin/tenants/:id/modules/:code. Used by B5, where the point is that the
// change lands on the next request with no restart and nothing to invalidate.
func (f *TenantFixture) SetModule(t *testing.T, moduleCode string, enabled bool) {
	t.Helper()
	mustExec(t, f.db.Owner, `
		INSERT INTO tenant_modules (tenant_id, module_code, enabled)
		VALUES (?, ?, ?)
		ON CONFLICT (tenant_id, module_code) DO UPDATE SET enabled = EXCLUDED.enabled`,
		f.ID, moduleCode, enabled)
}

// SetTenantRole promotes or demotes a user directly in the database, for the
// arrange half of a test that is about something else.
func (d *TestDB) SetTenantRole(t *testing.T, userID uuid.UUID, tenantRole string) {
	t.Helper()
	mustExec(t, d.Owner, `UPDATE users SET tenant_role = ? WHERE id = ?`, tenantRole, userID)
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

// NewProduct creates a product with an explicit reorder point, for the tests
// where the reorder threshold is the thing under test (F2).
func (f *TenantFixture) NewProduct(t *testing.T, name, reorderPoint string) uuid.UUID {
	t.Helper()
	n := next()
	id := uuid.New()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO products (id, tenant_id, sku, name, reorder_point, standard_cost)
			VALUES (?, ?, ?, ?, ?, 10.00)`,
			id, f.ID, fmt.Sprintf("SKU-%d-F", n), name, reorderPoint).Error
	})
	return id
}

// NewWarehouse creates an extra warehouse.
func (f *TenantFixture) NewWarehouse(t *testing.T, name string) uuid.UUID {
	t.Helper()
	n := next()
	id := uuid.New()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO warehouses (id, tenant_id, code, name)
			VALUES (?, ?, ?, ?)`, id, f.ID, fmt.Sprintf("WH-%d", n), name).Error
	})
	return id
}

// PostLedger appends one stock ledger row directly, standing in for whatever
// document would have written it. `qtyDelta` is signed decimal text — never a
// float, in the fixtures either (I8).
//
// It is an INSERT and nothing else: the ledger is append-only (§6.9.3), so
// there is no fixture that edits one, deliberately. erp_app holds INSERT and
// has UPDATE and DELETE revoked, so this runs on the same grant the application
// does.
func (f *TenantFixture) PostLedger(t *testing.T, productID, warehouseID uuid.UUID, qtyDelta, entryType string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	sourceType := "manual_adjustment"
	if entryType == "receipt" {
		sourceType = "goods_receipt"
	}
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO stock_ledger
			  (id, tenant_id, product_id, warehouse_id, entry_type, qty_delta,
			   unit_cost, source_type, created_by)
			VALUES (?, ?, ?, ?, ?, ?, 10.00, ?, ?)`,
			id, f.ID, productID, warehouseID, entryType, qtyDelta,
			sourceType, f.User.ID).Error
	})
	return id
}

// SetPOStatus moves a purchase order to a status directly, for the arrange half
// of a test about something else — the in-use checks care only whether a PO is
// still open.
func (f *TenantFixture) SetPOStatus(t *testing.T, poID uuid.UUID, status string) {
	t.Helper()
	// As the owner: the terminal-state trigger refuses an update to a `received`
	// PO, and these fixtures are setting up the state rather than transitioning
	// through it.
	mustExec(t, f.db.Owner, `UPDATE purchase_orders SET status = ? WHERE id = ?`,
		status, poID)
}

// NewGoodsReceiptLine adds a receipt line, which is what po_line_status derives
// qty_received from (G11).
func (f *TenantFixture) NewGoodsReceiptLine(t *testing.T, grID, poLineID, productID uuid.UUID, qty string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO goods_receipt_lines
			  (id, tenant_id, gr_id, po_line_id, product_id, qty_received)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, f.ID, grID, poLineID, productID, qty).Error
	})
	return id
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
