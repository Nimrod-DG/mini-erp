package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/api"
	"github.com/DGosal/mini-erp/backend/internal/docnum"
	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// One tenant's business data: master data, opening stock, the procurement
// history, and the receipts that complete it.
//
// ALL OF IT IN ONE TRANSACTION, on the erp_app pool with tenant context set by
// db.WithTenant (I1). Not for speed — because a seed that half-succeeds leaves a
// demo database whose documents refer to master data that is not there, and the
// next person has to work out which half ran. This way there is no half.
//
// Running as erp_app also means the seed is held to the application's own
// grants: it cannot UPDATE the stock ledger, it cannot see the other tenant's
// rows, and if it tried, PostgreSQL would stop it. A seed run as the schema
// owner would be able to write a demo the application could not have produced.

// seedTenantData writes everything inside one tenant.
func seedTenantData(ctx context.Context, app *gorm.DB, tx *gorm.DB, t *seededTenant, now time.Time) error {
	// The 60-day window §15 asks for. Anchored once, so every date below is a
	// fixed offset from the same instant rather than from whenever that line
	// happened to run.
	windowStart := now.AddDate(0, 0, -60)

	warehouses, err := seedWarehouses(tx, t, windowStart)
	if err != nil {
		return err
	}
	products, err := seedProducts(tx, t, windowStart)
	if err != nil {
		return err
	}
	suppliers, err := seedSuppliers(tx, t, windowStart)
	if err != nil {
		return err
	}

	if err := seedOpeningStock(tx, t, products, warehouses, windowStart); err != nil {
		return err
	}
	if err := seedAdjustments(tx, t, products, warehouses, now); err != nil {
		return err
	}
	return seedProcurement(ctx, app, tx, t, products, warehouses, suppliers, now)
}

// --------------------------------------------------------------------------
// Master data.
// --------------------------------------------------------------------------

// createdAt is backdated to the start of the window for every master-data row.
// A catalogue whose every product was created this morning, next to a ledger
// going back two months, is a database that visibly did not happen.
func seedWarehouses(tx *gorm.DB, t *seededTenant, at time.Time) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(t.spec.Warehouses))
	for _, w := range t.spec.Warehouses {
		id := seedID("warehouse", t.spec.Slug, w.Code)
		if err := tx.Exec(`
			INSERT INTO warehouses (id, tenant_id, code, name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name`,
			id, t.id, w.Code, w.Name, at, at).Error; err != nil {
			return nil, fmt.Errorf("warehouse %s: %w", w.Code, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func seedProducts(tx *gorm.DB, t *seededTenant, at time.Time) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(t.spec.Products))
	for _, p := range t.spec.Products {
		id := seedID("product", t.spec.Slug, p.SKU)

		// Deleted 20 days into the window rather than at creation, so the row
		// has a plausible life before somebody tidied it away — and `deleted_by`
		// names who, because a recycle bin that cannot say who put something in
		// it is not much of an audit trail.
		var deletedAt *time.Time
		var deletedBy *uuid.UUID
		if p.Deleted {
			when := at.AddDate(0, 0, 20)
			deletedAt, deletedBy = &when, &t.approver
		}

		if err := tx.Exec(`
			INSERT INTO products
			  (id, tenant_id, sku, name, uom, reorder_point, standard_cost,
			   is_active, created_at, updated_at, deleted_at, deleted_by)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name, uom = EXCLUDED.uom,
			    reorder_point = EXCLUDED.reorder_point,
			    standard_cost = EXCLUDED.standard_cost,
			    is_active = EXCLUDED.is_active,
			    deleted_at = EXCLUDED.deleted_at, deleted_by = EXCLUDED.deleted_by`,
			id, t.id, p.SKU, p.Name, p.UOM, p.ReorderPoint, p.StandardCost,
			!p.Discontinued, at, at, deletedAt, deletedBy).Error; err != nil {
			return nil, fmt.Errorf("product %s: %w", p.SKU, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func seedSuppliers(tx *gorm.DB, t *seededTenant, at time.Time) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(t.spec.Suppliers))
	for _, s := range t.spec.Suppliers {
		id := seedID("supplier", t.spec.Slug, s.Code)
		if err := tx.Exec(`
			INSERT INTO suppliers
			  (id, tenant_id, code, name, lead_time_days, payment_terms,
			   is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE
			SET name = EXCLUDED.name, lead_time_days = EXCLUDED.lead_time_days,
			    payment_terms = EXCLUDED.payment_terms, is_active = EXCLUDED.is_active`,
			id, t.id, s.Code, s.Name, s.LeadTimeDays, s.PaymentTerms,
			!s.Inactive, at, at).Error; err != nil {
			return nil, fmt.Errorf("supplier %s: %w", s.Code, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// --------------------------------------------------------------------------
// The stock ledger.
// --------------------------------------------------------------------------

// seedOpeningStock writes §15's "opening adjustment per product, dated ~60 days
// ago" — one row per product per warehouse that holds any.
//
// The rows are staggered an hour apart rather than sharing one timestamp. Not
// cosmetics: `stock_ledger` has no natural ordering beyond `occurred_at`, and
// twenty rows at the same instant make the ledger's default sort arbitrary and
// the recent-activity widget shuffle between two reloads of identical data.
func seedOpeningStock(tx *gorm.DB, t *seededTenant, products, warehouses []uuid.UUID, at time.Time) error {
	hour := 0
	for i, p := range t.spec.Products {
		for w, qty := range p.Opening {
			if qty == "" || w >= len(warehouses) {
				continue
			}
			// `ledger_qty_nonzero` refuses a movement of nothing, and rightly:
			// "we hold none of this here" is the absence of a row.
			if qty == "0" {
				continue
			}
			hour++
			if err := insertLedger(tx, t,
				seedID("opening", t.spec.Slug, p.SKU, t.spec.Warehouses[w].Code),
				products[i], warehouses[w], "adjustment", qty, p.StandardCost,
				"Opening balance carried in at go-live.",
				at.Add(time.Duration(hour)*time.Hour), t.approver); err != nil {
				return fmt.Errorf("opening stock %s: %w", p.SKU, err)
			}
		}
	}
	return nil
}

// seedAdjustments writes the manual corrections of documents.go — the §6.3 case
// where there is no document behind the movement and the person is the source.
func seedAdjustments(tx *gorm.DB, t *seededTenant, products, warehouses []uuid.UUID, now time.Time) error {
	for _, plan := range adjustmentPlans {
		by := t.raiser
		if plan.By == actorApprover {
			by = t.approver
		}
		if err := insertLedger(tx, t,
			seedID("adjustment", t.spec.Slug, fmt.Sprint(plan.Seq)),
			products[plan.Product], warehouses[plan.Warehouse],
			"adjustment", plan.Qty, t.spec.Products[plan.Product].StandardCost,
			plan.Note, now.AddDate(0, 0, -plan.DaysAgo), by); err != nil {
			return fmt.Errorf("adjustment %d: %w", plan.Seq, err)
		}
	}
	return nil
}

// insertLedger appends one row. `ON CONFLICT DO NOTHING` and never DO UPDATE:
// the ledger is append-only (§6.9.3) and erp_app has UPDATE revoked, so an
// upsert here would fail at the database — which is the invariant doing its job,
// and the reason a reseed *skips* rather than rewrites.
func insertLedger(tx *gorm.DB, t *seededTenant, id, productID, warehouseID uuid.UUID,
	entryType, qty, unitCost, note string, at time.Time, by uuid.UUID) error {

	return tx.Exec(`
		INSERT INTO stock_ledger
		  (id, tenant_id, product_id, warehouse_id, entry_type, qty_delta,
		   unit_cost, source_type, source_id, note, occurred_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'manual_adjustment', NULL, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`,
		id, t.id, productID, warehouseID, entryType, qty, unitCost, note, at, by).Error
}

// --------------------------------------------------------------------------
// Procurement, and the cross-module receipts that finish it.
// --------------------------------------------------------------------------

// seedProcurement writes the thirteen requisitions, the six orders their
// approvals produced, and the receipts against three of those orders.
//
// WHY THE REQUISITIONS AND ORDERS ARE WRITTEN AS SQL AND THE RECEIPTS ARE NOT.
//
// §15 is specific about the receipts and silent about the rest, and the
// difference is not an oversight. A receipt is the one event that writes to
// three modules at once; a second implementation of it would be a second place
// the atomicity could be wrong, and the demo database would be the one place
// nobody checked. A requisition is a document with no consequences beyond
// itself.
//
// The other half of the reason is dates. §15 wants orders spread across sixty
// days with `expected_at` "a mix of past and future", and approval computes
// `expected_at` as *today* plus the supplier's lead time (§8.3) — so every
// seeded order would be expected next week, and "overdue" could not be shown at
// all. Writing the orders directly is what buys the history a shape.
//
// Document numbers still come from `docnum.AllocateAt`, dated at the document's
// own instant, so a requisition raised in May is numbered PR-2025xx and not
// filed under this month.
func seedProcurement(ctx context.Context, app, tx *gorm.DB, t *seededTenant,
	products, warehouses, suppliers []uuid.UUID, now time.Time) error {

	// The receipts are posted as a real resolved identity rather than a struct
	// assembled here. `identity.Resolve` is the same function the middleware
	// calls on every request (I9), so the seed's caller comes out of the database
	// exactly as a signed-in user's would — entitlements, levels, and all.
	receiver, err := identity.Resolve(ctx, app, t.approverUID)
	if err != nil {
		return fmt.Errorf("resolve the receiving user %s: %w", t.approverUID, err)
	}

	for _, plan := range requisitionPlans {
		requisitionID := seedID("requisition", t.spec.Slug, fmt.Sprint(plan.Seq))
		raisedAt := now.AddDate(0, 0, -plan.DaysAgo)

		// Existence is checked once, here, rather than relied on per statement.
		// A requisition is several rows across three tables plus two allocated
		// document numbers, and `docnum` has no upsert: re-running it would burn
		// numbers and leave gaps in the tenant's sequence for documents that
		// already exist.
		exists, err := rowExists(tx, "purchase_requisitions", requisitionID)
		if err != nil {
			return err
		}
		if !exists {
			if err := insertRequisition(tx, t, plan, requisitionID, products, warehouses, suppliers, raisedAt); err != nil {
				return fmt.Errorf("requisition %d: %w", plan.Seq, err)
			}
		}

		if plan.Order == nil {
			continue
		}
		poID := seedID("order", t.spec.Slug, fmt.Sprint(plan.Seq))
		exists, err = rowExists(tx, "purchase_orders", poID)
		if err != nil {
			return err
		}
		if !exists {
			if err := insertPurchaseOrder(tx, t, plan, requisitionID, poID, suppliers, warehouses, raisedAt, now); err != nil {
				return fmt.Errorf("order for requisition %d: %w", plan.Seq, err)
			}
		}

		if err := postReceipts(tx, t, plan, poID, receiver); err != nil {
			return fmt.Errorf("receipts for requisition %d: %w", plan.Seq, err)
		}
	}
	return nil
}

func insertRequisition(tx *gorm.DB, t *seededTenant, plan requisitionPlan, id uuid.UUID,
	products, warehouses, suppliers []uuid.UUID, at time.Time) error {

	number, err := docnum.AllocateAt(tx, t.id, docnum.PR, &at)
	if err != nil {
		return err
	}

	var supplierID *uuid.UUID
	if plan.Supplier >= 0 {
		supplierID = &suppliers[plan.Supplier]
	}

	// The conditional columns of §6.10.3, filled per status. A draft carries
	// none of them; anything past draft carries `submitted_at`; a decision
	// carries `decided_by` and `decided_at` together (`pr_decided_fields_together`);
	// a rejection carries a reason (`pr_reject_needs_reason`).
	var submittedAt, decidedAt, cancelledAt *time.Time
	var decidedBy, cancelledBy *uuid.UUID
	var rejectReason, cancelReason *string

	if plan.Status != "draft" {
		when := at.Add(4 * time.Hour)
		submittedAt = &when
	}
	if plan.Status == "approved" || plan.Status == "rejected" {
		when := at.Add(28 * time.Hour)
		decidedAt, decidedBy = &when, &t.approver
	}
	if plan.Status == "rejected" {
		rejectReason = &plan.Reason
	}
	if plan.Status == "cancelled" {
		when := at.Add(30 * time.Hour)
		cancelledAt, cancelledBy = &when, &t.raiser
		cancelReason = &plan.Reason
	}

	requestedBy := t.raiser
	if plan.Raiser == actorApprover {
		requestedBy = t.approver
	}

	if err := tx.Exec(`
		INSERT INTO purchase_requisitions
		  (id, tenant_id, pr_number, warehouse_id, supplier_id, status, notes,
		   requested_by, submitted_at, decided_by, decided_at, reject_reason,
		   cancelled_by, cancelled_at, cancel_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, t.id, number, warehouses[plan.Warehouse], supplierID, plan.Status,
		nullIfEmpty(plan.Notes), requestedBy, submittedAt, decidedBy, decidedAt,
		rejectReason, cancelledBy, cancelledAt, cancelReason, at, at).Error; err != nil {
		return err
	}

	for i, line := range plan.Lines {
		if err := tx.Exec(`
			INSERT INTO purchase_requisition_lines
			  (id, tenant_id, requisition_id, product_id, qty, est_unit_cost, line_no)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			seedID("requisition-line", t.spec.Slug, fmt.Sprint(plan.Seq), fmt.Sprint(i)),
			t.id, id, products[line.Product], line.Qty,
			// The estimate defaults to the product's standard cost, exactly as
			// resolveLines does. It is not decoration: this number is copied to
			// the PO line as `unit_cost` and from there values the receipt's
			// journal entry, so a zero here would post Dr 0 / Cr 0 — a balanced
			// entry for nothing, which looks fine and is worse than an error.
			t.spec.Products[line.Product].StandardCost, i+1).Error; err != nil {
			return err
		}
	}
	return nil
}

func insertPurchaseOrder(tx *gorm.DB, t *seededTenant, plan requisitionPlan,
	requisitionID, poID uuid.UUID, suppliers, warehouses []uuid.UUID, raisedAt, now time.Time) error {

	// Ordered a day after the requisition was decided, which is when approval
	// would have generated it.
	orderedAt := raisedAt.Add(28 * time.Hour)
	number, err := docnum.AllocateAt(tx, t.id, docnum.PO, &orderedAt)
	if err != nil {
		return err
	}

	// A DATE, not an instant: `expected_at` is a business date and crosses the
	// wire as YYYY-MM-DD, so no browser can render it a day early (§2.5.3).
	expected := now.AddDate(0, 0, plan.Order.ExpectedInDays).Format("2006-01-02")

	status := "open"
	var cancelledAt *time.Time
	var cancelledBy *uuid.UUID
	var cancelReason *string
	if plan.Order.Cancelled {
		// Inserted cancelled rather than transitioned: `po_terminal_immutable`
		// refuses to modify a row that is already terminal, and there is no
		// value in faking a transition nobody will see.
		status = "cancelled"
		when := orderedAt.Add(72 * time.Hour)
		cancelledAt, cancelledBy = &when, &t.approver
		cancelReason = &plan.Order.Reason
	}

	if err := tx.Exec(`
		INSERT INTO purchase_orders
		  (id, tenant_id, po_number, requisition_id, supplier_id, warehouse_id,
		   status, total_amount, ordered_at, expected_at, created_by,
		   cancelled_by, cancelled_at, cancel_reason, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?,
		       COALESCE((SELECT SUM(l.qty * l.est_unit_cost)
		                 FROM purchase_requisition_lines l
		                 WHERE l.requisition_id = ?), 0)::numeric(18,2),
		       ?, ?::date, ?, ?, ?, ?, ?`,
		poID, t.id, number, requisitionID, suppliers[plan.Supplier],
		warehouses[plan.Warehouse], status, requisitionID,
		orderedAt, expected, t.approver,
		cancelledBy, cancelledAt, cancelReason, orderedAt).Error; err != nil {
		return err
	}

	// `line_no` is preserved rather than renumbered, so "line 3" means the same
	// line on the requisition and on the order — the same thing
	// generatePurchaseOrder does, and the reason the receipt form can be read
	// alongside the requisition it came from.
	return tx.Exec(`
		INSERT INTO purchase_order_lines
		  (id, tenant_id, po_id, product_id, qty_ordered, unit_cost, line_no)
		SELECT gen_random_uuid(), l.tenant_id, ?, l.product_id,
		       l.qty, l.est_unit_cost, l.line_no
		FROM purchase_requisition_lines l
		WHERE l.requisition_id = ?
		ORDER BY l.line_no`, poID, requisitionID).Error
}

// postReceipts runs each planned delivery through the real §8.4 transaction.
//
// This is the part of the seed §15 is specific about, and the reason it matters
// is worth restating: these calls write the goods receipt, its lines, the
// order's new status, the stock ledger rows, AND the balanced journal entry, all
// on `tx` — the same transaction everything else in this tenant is being written
// in. If any of it were wrong, `jel_balanced` or `grl_no_over_receipt` would
// refuse the commit and the seed would fail loudly rather than produce a demo
// database that quietly disagrees with the application.
//
// The idempotency keys are deterministic, per §15: without that a second `make
// seed` would post every delivery twice and double the stock.
func postReceipts(tx *gorm.DB, t *seededTenant, plan requisitionPlan, poID uuid.UUID,
	receiver *identity.Identity) error {

	if plan.Order == nil || len(plan.Order.Receipts) == 0 {
		return nil
	}

	// The order's lines, in line_no order, so a receiptPlan's map keys line up
	// with the positions in plan.Lines.
	var poLines []uuid.UUID
	if err := tx.Raw(`
		SELECT id FROM purchase_order_lines WHERE po_id = ? ORDER BY line_no`, poID).
		Scan(&poLines).Error; err != nil {
		return err
	}

	for i, receipt := range plan.Order.Receipts {
		lines := make([]api.ReceiptLine, 0, len(receipt.Lines))
		// Iterated by position rather than over the map, because Go randomises
		// map order and the receipt's lines would otherwise be written in a
		// different order on every run — which is not a difference the database
		// would notice, but it is one a reviewer diffing two seeds would.
		for position := range plan.Lines {
			qty, ok := receipt.Lines[position]
			if !ok {
				continue
			}
			if position >= len(poLines) {
				return fmt.Errorf("receipt names line %d of an order with %d lines",
					position, len(poLines))
			}
			lines = append(lines, api.ReceiptLine{POLineID: poLines[position], Qty: qty})
		}
		if len(lines) == 0 {
			continue
		}

		// §15's shape exactly: seed-gr-<tenant-slug>-<n>. It is not a UUID, which
		// the endpoint would refuse — and that is fine, because this is not the
		// endpoint. The property the key needs is that it is the same string on
		// every run, and this one is.
		key := fmt.Sprintf("seed-gr-%s-%d-%d", t.spec.Slug, plan.Seq, i+1)

		summary, err := api.PostGoodsReceipt(tx, receiver, poID, key, receipt.Note, lines)
		if err != nil {
			return err
		}
		if !summary.Replayed {
			log.Printf("seed:   %s → %s posted %s and %d ledger %s",
				t.spec.Slug, summary.GRNumber, summary.EntryNumber,
				summary.LedgerEntries, plural(summary.LedgerEntries, "entry", "entries"))
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Small helpers.
// --------------------------------------------------------------------------

// rowExists asks whether a seeded row is already there. The table name is a
// literal at every call site — there is no user input anywhere near it.
func rowExists(tx *gorm.DB, table string, id uuid.UUID) (bool, error) {
	var found []int
	if err := tx.Raw(`SELECT 1 FROM `+table+` WHERE id = ? LIMIT 1`, id).
		Scan(&found).Error; err != nil {
		return false, err
	}
	return len(found) > 0, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
