// Group I — invariants the database enforces on its own, whatever the
// application does. Each of these is a rule that a handler also checks; the
// point of the test is that the rule survives a code path that forgets to.
package db_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// I1 — a journal entry with one line is rejected at commit. A posting needs at
// least two lines; one line cannot balance.
func TestI1_SingleLineJournalEntryIsRejected(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	err := f.AsTenant(t, func(tx *gorm.DB) error {
		entry := f.NewJournalEntry(t, tx)
		return addJournalLine(tx, f, entry, f.InventoryAccountID, 100, 0)
	})
	if !testsupport.IsPGCode(err, testsupport.CodeCheckViolation) {
		t.Fatalf("want check_violation at commit, got: %v", err)
	}
}

// I2 — an unbalanced entry raises at COMMIT, not at insert. This is what proves
// the trigger is DEFERRABLE INITIALLY DEFERRED: an immediate trigger would fail
// on the first line of every posting, since an entry is legitimately
// unbalanced until its last line lands.
func TestI2_UnbalancedEntryRaisesAtCommitNotAtInsert(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	var insertErrs []error
	err := f.AsTenant(t, func(tx *gorm.DB) error {
		entry := f.NewJournalEntry(t, tx)
		insertErrs = append(insertErrs,
			addJournalLine(tx, f, entry, f.InventoryAccountID, 100, 0),
			addJournalLine(tx, f, entry, f.GRNIAccountID, 0, 60))
		return nil
	})

	for i, insertErr := range insertErrs {
		if insertErr != nil {
			t.Fatalf("line %d failed at insert time — the trigger is not deferred: %v",
				i+1, insertErr)
		}
	}
	if !testsupport.IsPGCode(err, testsupport.CodeCheckViolation) {
		t.Fatalf("want check_violation at commit, got: %v", err)
	}
}

// I3 — a balanced two-line entry commits cleanly, and the intermediate
// single-line state does not fail on the way there.
func TestI3_BalancedEntryCommits(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	var entry uuid.UUID
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		entry = f.NewJournalEntry(t, tx)
		if err := addJournalLine(tx, f, entry, f.InventoryAccountID, 100, 0); err != nil {
			return err
		}
		return addJournalLine(tx, f, entry, f.GRNIAccountID, 0, 100)
	}); err != nil {
		t.Fatalf("a balanced entry was rejected: %v", err)
	}

	var lines int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM journal_entry_lines WHERE journal_entry_id = ?`,
			entry).Scan(&lines).Error
	})
	if lines != 2 {
		t.Fatalf("entry has %d lines, want 2", lines)
	}
}

// I4 — removing one line of a balanced pair raises at commit. Runs as the owner
// because erp_app has no DELETE on journal_entry_lines at all (§6.9.3) — the
// grant stops the application, the trigger stops everything else.
func TestI4_DeletingOneLineUnbalancesAtCommit(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	var entry, firstLine uuid.UUID
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		entry = f.NewJournalEntry(t, tx)
		if err := addJournalLine(tx, f, entry, f.InventoryAccountID, 100, 0); err != nil {
			return err
		}
		if err := addJournalLine(tx, f, entry, f.GRNIAccountID, 0, 100); err != nil {
			return err
		}
		return tx.Raw(`SELECT id FROM journal_entry_lines
		               WHERE journal_entry_id = ? AND debit > 0`, entry).
			Row().Scan(&firstLine)
	}); err != nil {
		t.Fatalf("arrange: %v", err)
	}

	// The application role cannot even try.
	appErr := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`DELETE FROM journal_entry_lines WHERE id = ?`, firstLine).Error
	})
	if !testsupport.IsPGCode(appErr, testsupport.CodeInsufficientPrivilege) {
		t.Fatalf("erp_app can DELETE from journal_entry_lines: %v", appErr)
	}

	ownerErr := f.AsOwner(t, func(tx *gorm.DB) error {
		return tx.Exec(`DELETE FROM journal_entry_lines WHERE id = ?`, firstLine).Error
	})
	if !testsupport.IsPGCode(ownerErr, testsupport.CodeCheckViolation) {
		t.Fatalf("want check_violation at commit, got: %v", ownerErr)
	}
}

// I5 — an approved requisition cannot be updated, even by raw SQL that never
// goes near the handler. The transition INTO the terminal state still works,
// because the trigger reads OLD.status.
func TestI5_ApprovedRequisitionIsImmutable(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	approved := f.NewRequisition(t, "approved")
	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE purchase_requisitions SET notes = 'rewritten' WHERE id = ?`,
			approved).Error
	})
	if !testsupport.IsPGCode(err, testsupport.CodeCheckViolation) {
		t.Fatalf("an approved requisition was modified: %v", err)
	}

	submitted := f.NewRequisition(t, "submitted")
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE purchase_requisitions
			SET status = 'approved', decided_by = ?, decided_at = now()
			WHERE id = ?`, f.User.ID, submitted).Error
	}); err != nil {
		t.Fatalf("submitted → approved was blocked: %v", err)
	}
}

// I6 — same for a received purchase order; open → partially_received still works.
func TestI6_ReceivedPurchaseOrderIsImmutable(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	po := f.NewPurchaseOrder(t)
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE purchase_orders SET status = 'partially_received' WHERE id = ?`,
			po).Error
	}); err != nil {
		t.Fatalf("open → partially_received was blocked: %v", err)
	}
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE purchase_orders SET status = 'received' WHERE id = ?`, po).Error
	}); err != nil {
		t.Fatalf("partially_received → received was blocked: %v", err)
	}

	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE purchase_orders SET total_amount = 999 WHERE id = ?`, po).Error
	})
	if !testsupport.IsPGCode(err, testsupport.CodeCheckViolation) {
		t.Fatalf("a received purchase order was modified: %v", err)
	}
}

// I7 — a receipt line whose product differs from its PO line's product is
// rejected by the (po_line_id, product_id) composite FK. Declarative, no
// trigger: the receipt line simply cannot name a product the order did not.
func TestI7_ReceiptLineMustMatchItsPOLineProduct(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	po := f.NewPurchaseOrder(t)
	line := f.NewPOLine(t, po, f.ProductID, 10)
	gr := f.NewGoodsReceipt(t, po)

	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO goods_receipt_lines (tenant_id, gr_id, po_line_id, product_id, qty_received)
			VALUES (?, ?, ?, ?, 1)`, f.ID, gr, line, f.ProductAltID).Error
	})
	if !testsupport.IsPGCode(err, testsupport.CodeForeignKeyViolation) {
		t.Fatalf("want a foreign key violation, got: %v", err)
	}

	// The matching product is accepted.
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO goods_receipt_lines (tenant_id, gr_id, po_line_id, product_id, qty_received)
			VALUES (?, ?, ?, ?, 1)`, f.ID, gr, line, f.ProductID).Error
	}); err != nil {
		t.Fatalf("a matching receipt line was rejected: %v", err)
	}
}

// I8 — every mutable table's updated_at advances on update.
//
// The insert and the update run in separate transactions on purpose: now() is
// fixed for the life of a transaction, so a same-transaction update would show
// no movement no matter what the trigger did.
func TestI8_UpdatedAtAdvancesOnEveryMutableTable(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	po := f.NewPurchaseOrder(t)
	pr := f.NewRequisition(t, "draft")

	platform := []struct {
		table, set, where string
		arg               any
	}{
		{"tenants", `name = name || '.'`, `id = ?`, f.ID},
		{"users", `full_name = full_name || '.'`, `id = ?`, f.User.ID},
		{"tenant_modules", `enabled = enabled`, `tenant_id = ?`, f.ID},
		{"user_module_roles", `role_level = role_level`, `user_id = ?`, f.User.ID},
	}
	for _, tc := range platform {
		t.Run(tc.table, func(t *testing.T) {
			before := maxUpdatedAt(t, d.Owner, tc.table, tc.where, tc.arg)
			if err := d.Owner.Exec(
				`UPDATE `+tc.table+` SET `+tc.set+` WHERE `+tc.where, tc.arg).Error; err != nil {
				t.Fatal(err)
			}
			assertAdvanced(t, before, maxUpdatedAt(t, d.Owner, tc.table, tc.where, tc.arg), tc.table)
		})
	}

	tenantScoped := []struct {
		table, set string
		id         uuid.UUID
	}{
		{"products", `name = name || '.'`, f.ProductID},
		{"suppliers", `name = name || '.'`, f.SupplierID},
		{"warehouses", `name = name || '.'`, f.WarehouseID},
		{"accounts", `name = name || '.'`, f.InventoryAccountID},
		{"purchase_requisitions", `notes = 'touched'`, pr},
		{"purchase_orders", `total_amount = 1`, po},
	}
	for _, tc := range tenantScoped {
		t.Run(tc.table, func(t *testing.T) {
			var before, after string
			f.Must(t, func(tx *gorm.DB) error {
				return tx.Raw(`SELECT max(updated_at)::text FROM `+tc.table+` WHERE id = ?`,
					tc.id).Scan(&before).Error
			})
			f.Must(t, func(tx *gorm.DB) error {
				return tx.Exec(`UPDATE `+tc.table+` SET `+tc.set+` WHERE id = ?`, tc.id).Error
			})
			f.Must(t, func(tx *gorm.DB) error {
				return tx.Raw(`SELECT max(updated_at)::text FROM `+tc.table+` WHERE id = ?`,
					tc.id).Scan(&after).Error
			})
			assertAdvanced(t, before, after, tc.table)
		})
	}
}

// The SECURITY DEFINER escape hatch of §4.2.1: erp_admin is revoked from
// `accounts` and still has to be able to seed a new tenant's chart of accounts
// in the same transaction that creates the tenant. Two rows wide, and no
// table-level grant.
func TestSeedTenantAccountsCrossesTheRevoke(t *testing.T) {
	d := testsupport.NewTestDB(t)
	f := d.NewTenant(t, "Tenant A")

	// Direct access is still refused...
	err := d.Admin.Exec(`INSERT INTO accounts (tenant_id, code, name, type)
	                     VALUES (?, '9999', 'x', 'asset')`, f.ID).Error
	if !testsupport.IsPGCode(err, testsupport.CodeInsufficientPrivilege) {
		t.Fatalf("erp_admin can write accounts directly: %v", err)
	}

	// ...while the two seeded accounts the fixture asked for are present, and
	// visible only to their own tenant.
	var codes []string
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT code FROM accounts ORDER BY code`).Scan(&codes).Error
	})
	if len(codes) != 2 || codes[0] != "1300" || codes[1] != "2150" {
		t.Fatalf("chart of accounts = %v, want [1300 2150]", codes)
	}

	// Idempotent: a second call adds nothing.
	if err := d.Admin.Exec(`SELECT seed_tenant_accounts(?)`, f.ID).Error; err != nil {
		t.Fatalf("second seed: %v", err)
	}
	var count int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM accounts`).Scan(&count).Error
	})
	if count != 2 {
		t.Fatalf("re-seeding produced %d accounts, want 2", count)
	}
}

// --- helpers ---------------------------------------------------------------

func addJournalLine(tx *gorm.DB, f *testsupport.TenantFixture,
	entry, account uuid.UUID, debit, credit float64) error {
	return tx.Exec(`
		INSERT INTO journal_entry_lines
		  (tenant_id, journal_entry_id, account_id, debit, credit)
		VALUES (?, ?, ?, ?, ?)`, f.ID, entry, account, debit, credit).Error
}

func maxUpdatedAt(t *testing.T, g *gorm.DB, table, where string, arg any) string {
	t.Helper()
	var ts string
	if err := g.Raw(`SELECT max(updated_at)::text FROM `+table+` WHERE `+where, arg).
		Scan(&ts).Error; err != nil {
		t.Fatal(err)
	}
	return ts
}

func assertAdvanced(t *testing.T, before, after, table string) {
	t.Helper()
	if before == "" || after == "" {
		t.Fatalf("%s: no updated_at to compare (%q → %q)", table, before, after)
	}
	if after <= before {
		t.Fatalf("%s: updated_at did not advance (%s → %s)", table, before, after)
	}
}
