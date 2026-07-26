// Group A — tenant isolation. The highest-priority tests in the suite: they
// are what make "the database itself refuses to return another tenant's rows"
// a fact rather than a claim.
//
// Every test here creates TWO tenants and asserts against both. A
// single-tenant test cannot detect an isolation failure.
package db_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// A1 — rows created under tenant A are invisible when the session is tenant B.
func TestA1_RowsAreInvisibleToOtherTenants(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	b := d.NewTenant(t, "Tenant B")

	// Each fixture seeded two products of its own.
	assertProductCount(t, a, 2)
	assertProductCount(t, b, 2)

	// And neither can see the other's, by ID.
	var found int64
	b.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM products WHERE id = ?`, a.ProductID).
			Scan(&found).Error
	})
	if found != 0 {
		t.Fatalf("tenant B sees tenant A's product by id (%d rows)", found)
	}
}

// A2 — a query with no tenant context set returns zero rows, not all rows.
//
// Both shapes matter: a connection that never had context, and one that had it
// and committed. The second resets the GUC to the empty string rather than to
// NULL, which is why the policy maps ” to NULL (see 005_rls_grants.up.sql).
func TestA2_NoTenantContextReturnsZeroRows(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	_ = d.NewTenant(t, "Tenant B")

	var count int64
	if err := d.App.Raw(`SELECT count(*) FROM products`).Scan(&count).Error; err != nil {
		t.Fatalf("query without tenant context errored instead of returning nothing: %v", err)
	}
	if count != 0 {
		t.Fatalf("query without tenant context returned %d rows", count)
	}

	// Same again on a connection that has carried tenant context before.
	pool := d.NewAppPool(t, 1)
	if err := commitTenantTx(t, pool, a.ID); err != nil {
		t.Fatalf("seed transaction: %v", err)
	}
	if err := pool.Raw(`SELECT count(*) FROM products`).Scan(&count).Error; err != nil {
		t.Fatalf("query after a committed tenant transaction errored: %v", err)
	}
	if count != 0 {
		t.Fatalf("tenant context survived the transaction: %d rows visible", count)
	}
}

// A3 — an INSERT carrying tenant B's tenant_id while the session is tenant A is
// rejected by WITH CHECK. Without WITH CHECK the row would be written and
// become permanently invisible to the tenant that owns it.
func TestA3_InsertWithForeignTenantIDIsRejected(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	b := d.NewTenant(t, "Tenant B")

	err := a.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO products (tenant_id, sku, name) VALUES (?, 'SMUGGLED', 'x')`,
			b.ID).Error
	})
	if err == nil {
		t.Fatal("insert tagged with another tenant's id succeeded")
	}
	if got := testsupport.PGCode(err); got != testsupport.CodeInsufficientPrivilege {
		t.Fatalf("want a row-security violation (42501), got %s: %v", got, err)
	}
}

// A4 — UPDATE and DELETE cannot reach another tenant's rows even by primary key.
func TestA4_UpdateAndDeleteCannotReachOtherTenants(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	b := d.NewTenant(t, "Tenant B")

	var updated, deleted int64
	b.Must(t, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE products SET name = 'hijacked' WHERE id = ?`, a.ProductID)
		if res.Error != nil {
			return res.Error
		}
		updated = res.RowsAffected

		res = tx.Exec(`DELETE FROM products WHERE id = ?`, a.ProductID)
		deleted = res.RowsAffected
		return res.Error
	})
	if updated != 0 || deleted != 0 {
		t.Fatalf("cross-tenant reach by primary key: %d updated, %d deleted", updated, deleted)
	}

	// The row is untouched and still tenant A's.
	var name string
	a.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT name FROM products WHERE id = ?`, a.ProductID).Scan(&name).Error
	})
	if name != "Product A" {
		t.Fatalf("tenant A's product was modified: name = %q", name)
	}
}

// A5 — every tenant-scoped table has RLS enabled AND forced, with a policy.
//
// Driven from the catalog rather than a hand-written list, so a table added in
// a later phase without a policy fails this test automatically.
func TestA5_EveryTenantTableHasForcedRLS(t *testing.T) {
	d := testsupport.NewTestDB(t)

	// The five platform tables carry no RLS by design (§6.8): they are read
	// during identity resolution, before tenant context exists, and are scoped
	// in application code. Two of them carry a tenant_id.
	exempt := map[string]bool{"users": true, "tenant_modules": true}

	type table struct {
		Name     string
		Enabled  bool
		Forced   bool
		Policies int64
	}
	var tables []table
	if err := d.App.Raw(`
		SELECT c.relname                                                   AS name,
		       c.relrowsecurity                                            AS enabled,
		       c.relforcerowsecurity                                       AS forced,
		       (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid)  AS policies
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND EXISTS (
		        SELECT 1 FROM information_schema.columns col
		        WHERE col.table_schema = 'public'
		          AND col.table_name   = c.relname
		          AND col.column_name  = 'tenant_id')
		ORDER BY c.relname`).Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}

	// The §6.8 list, minus audit_log which is Phase 11.
	want := map[string]bool{
		"warehouses": true, "products": true, "stock_ledger": true,
		"suppliers": true, "purchase_requisitions": true,
		"purchase_requisition_lines": true, "purchase_orders": true,
		"purchase_order_lines": true, "goods_receipts": true,
		"goods_receipt_lines": true, "accounts": true, "journal_entries": true,
		"journal_entry_lines": true, "document_sequences": true,
	}

	seen := map[string]bool{}
	for _, tbl := range tables {
		if exempt[tbl.Name] {
			continue
		}
		seen[tbl.Name] = true
		if !tbl.Enabled || !tbl.Forced || tbl.Policies == 0 {
			t.Errorf("%s: rls enabled=%v forced=%v policies=%d — want true/true/>0",
				tbl.Name, tbl.Enabled, tbl.Forced, tbl.Policies)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("%s is missing from the schema or lost its tenant_id", name)
		}
	}
}

// A6 — WithTenant uses a transaction-local set: after commit, a fresh query on
// the same pooled connection sees no tenant context. A session-scoped SET here
// would leak one request's tenant onto the next request that reuses the
// connection, which is the failure mode this whole design exists to avoid.
func TestA6_TenantContextDoesNotSurviveCommit(t *testing.T) {
	d := testsupport.NewTestDB(t)

	// Pool of exactly one, so the follow-up query is guaranteed to reuse the
	// connection the transaction ran on.
	pool := d.NewAppPool(t, 1)
	if err := commitTenantTx(t, pool, uuid.New()); err != nil {
		t.Fatalf("WithTenant: %v", err)
	}

	var after string
	if err := pool.Raw(`SELECT current_setting('app.current_tenant', true)`).
		Scan(&after).Error; err != nil {
		t.Fatalf("read setting: %v", err)
	}
	if after != "" {
		t.Fatalf("app.current_tenant leaked out of the transaction: %q", after)
	}
}

// A7 — isolation holds through stock_balances, not just stock_ledger.
//
// The view is where a missing security_invoker leaks everything while the base
// table stays correctly isolated, so a base-table-only test would pass.
func TestA7_IsolationHoldsThroughTheBalancesView(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	b := d.NewTenant(t, "Tenant B")

	addStock(t, a, 40)
	addStock(t, b, 7)

	for _, tc := range []struct {
		tenant *testsupport.TenantFixture
		want   float64
	}{{a, 40}, {b, 7}} {
		var rows int64
		var onHand float64
		tc.tenant.Must(t, func(tx *gorm.DB) error {
			if err := tx.Raw(`SELECT count(*) FROM stock_balances`).Scan(&rows).Error; err != nil {
				return err
			}
			return tx.Raw(`SELECT COALESCE(sum(qty_on_hand), 0) FROM stock_balances`).
				Scan(&onHand).Error
		})
		if rows != 1 || onHand != tc.want {
			t.Fatalf("stock_balances leaked: %d rows, %v on hand, want 1 row / %v",
				rows, onHand, tc.want)
		}
	}
}

// A8 — every view is security_invoker. Without it the view runs as its owner,
// and an owner is not subject to its own tables' policies.
func TestA8_EveryViewIsSecurityInvoker(t *testing.T) {
	d := testsupport.NewTestDB(t)

	type view struct {
		Name    string
		Options string
	}
	var views []view
	if err := d.App.Raw(`
		SELECT c.relname                                        AS name,
		       COALESCE(array_to_string(c.reloptions, ','), '')  AS options
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'v'
		ORDER BY c.relname`).Scan(&views).Error; err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 {
		t.Fatalf("expected the two derived views, found %d: %v", len(views), views)
	}
	for _, v := range views {
		if v.Options != "security_invoker=true" {
			t.Errorf("view %s has reloptions %q — want security_invoker=true", v.Name, v.Options)
		}
	}
}

// A9 — composite FK: a purchase_order_lines row tagged tenant A but pointing at
// tenant B's product is rejected by the foreign key.
//
// Run as the owner, so RLS is demonstrably not what blocks it: FK checks run
// with the table owner's privileges and bypass policies entirely, which is why
// the tenant has to be part of the reference.
func TestA9_CompositeFKRejectsCrossTenantReference(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	b := d.NewTenant(t, "Tenant B")

	poID := a.NewPurchaseOrder(t)

	err := a.AsOwner(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO purchase_order_lines
			  (tenant_id, po_id, product_id, qty_ordered, unit_cost, line_no)
			VALUES (?, ?, ?, 1, 1.00, 9999)`,
			a.ID, poID, b.ProductID).Error
	})
	if err == nil {
		t.Fatal("a PO line referencing another tenant's product was accepted")
	}
	if got := testsupport.PGCode(err); got != testsupport.CodeForeignKeyViolation {
		t.Fatalf("want a foreign key violation (%s), got %s: %v",
			testsupport.CodeForeignKeyViolation, got, err)
	}
}

// A10 — neither application role may hold BYPASSRLS or SUPERUSER (I3). Catches
// a role provisioned by hand through a managed-provider console.
func TestA10_ApplicationRolesCannotBypassRLS(t *testing.T) {
	d := testsupport.NewTestDB(t)

	type role struct {
		Rolname      string
		Rolbypassrls bool
		Rolsuper     bool
	}
	var roles []role
	// rolsuper, not rolsuperuser: pg_roles has no such column, and the query
	// as written in the phase brief errors out.
	if err := d.App.Raw(`
		SELECT rolname, rolbypassrls, rolsuper
		FROM pg_roles WHERE rolname IN ('erp_app','erp_admin')
		ORDER BY rolname`).Scan(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected erp_app and erp_admin, found %v", roles)
	}
	for _, r := range roles {
		if r.Rolbypassrls || r.Rolsuper {
			t.Errorf("%s: bypassrls=%v superuser=%v — both must be false",
				r.Rolname, r.Rolbypassrls, r.Rolsuper)
		}
	}
}

// A11 — a SELECT on tenant business data as erp_admin raises a permission
// error, not an empty result. Zero rows would mean the REVOKE never applied and
// only the absence of tenant context was hiding the data.
func TestA11_AdminRoleIsRevokedFromTenantTables(t *testing.T) {
	d := testsupport.NewTestDB(t)
	a := d.NewTenant(t, "Tenant A")
	_ = a

	var count int64
	err := d.Admin.Raw(`SELECT count(*) FROM purchase_orders`).Scan(&count).Error
	if err == nil {
		t.Fatalf("erp_admin read purchase_orders and got %d rows; the REVOKE did not apply", count)
	}
	if got := testsupport.PGCode(err); got != testsupport.CodeInsufficientPrivilege {
		t.Fatalf("want permission denied (%s), got %s: %v",
			testsupport.CodeInsufficientPrivilege, got, err)
	}

	// ...while the platform tables it does own remain readable.
	if err := d.Admin.Raw(`SELECT count(*) FROM tenants`).Scan(&count).Error; err != nil {
		t.Fatalf("erp_admin cannot read tenants: %v", err)
	}
}

// --- helpers ---------------------------------------------------------------

func assertProductCount(t *testing.T, f *testsupport.TenantFixture, want int64) {
	t.Helper()
	var got int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM products`).Scan(&got).Error
	})
	if got != want {
		t.Fatalf("tenant %s sees %d products, want %d", f.Name, got, want)
	}
}

func addStock(t *testing.T, f *testsupport.TenantFixture, qty float64) {
	t.Helper()
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO stock_ledger
			  (tenant_id, product_id, warehouse_id, entry_type, qty_delta,
			   unit_cost, source_type, created_by)
			VALUES (?, ?, ?, 'adjustment', ?, 10.00, 'manual_adjustment', ?)`,
			f.ID, f.ProductID, f.WarehouseID, qty, f.User.ID).Error
	})
}
