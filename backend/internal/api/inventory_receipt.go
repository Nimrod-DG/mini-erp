// [INVENTORY] — the inventory module's half of the goods receipt (§8.4 step 5).
//
// This is one of the two functions the cross-module transaction reaches into,
// and it is in its own file for the same reason the finance one is: the claim
// the project makes is that a single business event writes to three modules
// atomically, and that claim is easier to believe when you can see the three
// pieces and see that they all take the same `tx`.
//
// It appends to the ledger and nothing else. There is no counter to bump: stock
// on hand is SUM(qty_delta) read through stock_balances on every request (I6),
// so writing these rows *is* receiving the goods.
package api

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/identity"
)

// postReceiptStockLedger writes one `receipt` ledger row per goods receipt line
// and returns their IDs, so the confirmation panel can link to them.
//
// It runs on the caller's transaction — the same one the receipt was written in.
// A separate transaction here would be the bug the whole phase is about: stock
// credited for a receipt that later rolled back.
//
// The rows are derived in SQL from the receipt lines that were just inserted,
// rather than from the request:
//
//   - `qty_delta` is the quantity that was actually written, so the ledger cannot
//     disagree with the receipt it came from;
//   - `unit_cost` is the *order* line's cost, which is what the goods are worth
//     on arrival — the invoice may say something else later, and that is a
//     separate event;
//   - `warehouse_id` is the order's delivery warehouse, not a field the client
//     may set, because goods arrive where the order said they would.
func postReceiptStockLedger(tx *gorm.DB, caller *identity.Identity, grID, warehouseID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := tx.Raw(`
		INSERT INTO stock_ledger
		  (id, tenant_id, product_id, warehouse_id, entry_type, qty_delta,
		   unit_cost, source_type, source_id, created_by)
		SELECT gen_random_uuid(), ?, grl.product_id, ?, 'receipt', grl.qty_received,
		       pol.unit_cost, 'goods_receipt', ?, ?
		FROM goods_receipt_lines grl
		JOIN purchase_order_lines pol ON pol.id = grl.po_line_id
		WHERE grl.gr_id = ?
		RETURNING id`,
		caller.TenantID, warehouseID, grID, caller.UserID, grID).
		Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
