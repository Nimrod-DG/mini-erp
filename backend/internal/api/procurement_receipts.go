// The goods receipt — the one business event that writes to three modules in a
// single transaction (§8.4). This is the most important handler in the codebase
// and the thing the project exists to demonstrate.
//
// FIVE THINGS TO KNOW BEFORE EDITING THIS FILE.
//
//  1. ONE TRANSACTION. `tx` is the transaction TenantTx opened for the request,
//     and every step below runs on it — the receipt, its lines, the purchase
//     order's new status, the [INVENTORY] stock ledger rows, and the [FINANCE]
//     journal entry. If the journal fails, the goods receipt never happened.
//     That claim is D8, and it is the project's main claim. Do not move a step
//     into a goroutine, a second request, or a trigger: atomicity is the whole
//     point, and hiding it in a trigger makes the story invisible in the code.
//
//  2. THE LOCKS COME BEFORE THE VALIDATION, and the order is header then lines
//     (§8.6.3). Two receipts posted at the same moment against the same order
//     would each read `qty_received = 0` from po_line_status and each write 25
//     against a line ordered at 40 — individually valid, jointly an over-receipt
//     (H5). Locking the header as well as the lines is what makes step 4
//     correct: without it, two receipts completing different lines both see the
//     other line outstanding and both leave the order `partially_received` when
//     it is in fact complete.
//
//  3. THE IDEMPOTENCY KEY IS THE CLIENT'S, generated when the form opens. A
//     receipt is posted from a phone on warehouse wifi, where a request that
//     times out client-side but succeeded server-side is a Tuesday (§8.6.1).
//     Never generate one server-side — that defeats the purpose entirely.
//
//  4. EVERY NUMBER IS COMPUTED BY POSTGRESQL. The over-receipt comparison, the
//     line totals, and the journal amount are all evaluated where both operands
//     are still NUMERIC (I8). httpx.Numeric has no arithmetic on purpose, and
//     adding a Compare method here would be the first step towards deciding a
//     business rule in float64.
//
//  5. THE TRIGGERS ARE BACKSTOPS, NOT THE MECHANISM. `grl_no_over_receipt` and
//     `jel_balanced` independently refuse what this handler refuses. A trigger
//     violation reaching a client is a bug to investigate, not a normal path —
//     the handler does the user-facing work and produces the clean error.
package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/internal/docnum"
	"github.com/DGosal/mini-erp/backend/internal/httpx"
)

// receiptKeyConstraint is the UNIQUE (tenant_id, idempotency_key) on
// `goods_receipts`, named by PostgreSQL from the table and its columns.
//
// The replay path matches on this name and not on bare SQLSTATE 23505, because
// a duplicate `gr_number` is also a unique violation — and that one is a real
// numbering bug. Answering it with a cheerful 200 would hide it forever.
// TestTheIdempotencyConstraintIsNamedWhatTheHandlerMatchesOn asserts the string.
const receiptKeyConstraint = "goods_receipts_tenant_id_idempotency_key_key"

// The two accounts a goods receipt posts against (§6.5). Dr Inventory, Cr Goods
// received not invoiced: the stock is on the shelf and the invoice has not
// arrived, so the liability is recognised now rather than when Finance opens the
// post.
const (
	accountInventory = "1300"
	accountGRNI      = "2150"
)

var goodsReceiptSortable = map[string]string{
	"grNumber":   "gr.gr_number",
	"receivedAt": "gr.received_at",
	"poNumber":   "po.po_number",
	"supplier":   "s.name",
	"receivedBy": "u.full_name",
}

// --------------------------------------------------------------------------
// Response shapes.
// --------------------------------------------------------------------------

type goodsReceiptLine struct {
	ID       uuid.UUID `json:"id"`
	POLineID uuid.UUID `json:"poLineId"`
	LineNo   int       `json:"lineNo"`

	ProductID   uuid.UUID `json:"productId"`
	SKU         string    `json:"sku"`
	ProductName string    `json:"productName"`
	UOM         string    `json:"uom"`
	// ProductDeleted marks a line whose product has since been deleted. The
	// receipt still resolves the name — a receipt is a historical record, and the
	// join below carries no deleted filter on purpose (§6.9.1, Trap 3).
	ProductDeleted bool `json:"productDeleted"`

	QtyReceived httpx.Numeric `json:"qtyReceived"`
	UnitCost    httpx.Numeric `json:"unitCost"`
	LineTotal   httpx.Numeric `json:"lineTotal"`
}

type goodsReceiptRow struct {
	ID       uuid.UUID `json:"id"`
	GRNumber string    `json:"grNumber"`

	POID         uuid.UUID `json:"poId"`
	PONumber     string    `json:"poNumber"`
	POStatus     string    `json:"poStatus"`
	SupplierID   uuid.UUID `json:"supplierId"`
	SupplierName string    `json:"supplierName"`

	WarehouseID   uuid.UUID `json:"warehouseId"`
	WarehouseCode string    `json:"warehouseCode"`
	WarehouseName string    `json:"warehouseName"`

	ReceivedByID   uuid.UUID `json:"receivedById"`
	ReceivedByName string    `json:"receivedByName"`
	ReceivedAt     time.Time `json:"receivedAt"`
	Note           *string   `json:"note"`

	LineCount   int           `json:"lineCount"`
	QtyReceived httpx.Numeric `json:"qtyReceived"`
	// TotalValue is SUM(qty_received × unit_cost) — the same expression the
	// journal entry is posted for, evaluated in the same place.
	TotalValue httpx.Numeric `json:"totalValue"`
}

type goodsReceiptDetail struct {
	goodsReceiptRow
	Lines []goodsReceiptLine `json:"lines"`
}

// receiptOrderState is what the confirmation panel needs to say about the order:
// where it is now, having been moved by this receipt.
type receiptOrderState struct {
	ID       uuid.UUID `json:"id"`
	PONumber string    `json:"poNumber"`
	Status   string    `json:"status"`
}

// receiptInventoryResult names what the [INVENTORY] step wrote, so the panel can
// link straight to the rows rather than telling the user to go and look.
type receiptInventoryResult struct {
	LedgerEntryIDs []uuid.UUID `json:"ledgerEntryIds"`
	EntryCount     int         `json:"entryCount"`
}

// receiptFinanceResult is the same for [FINANCE]: one entry, two lines, and the
// two account codes spelled out, because "Dr Inventory / Cr GRNI" is the sentence
// the panel exists to be able to write.
type receiptFinanceResult struct {
	JournalEntryID   uuid.UUID     `json:"journalEntryId"`
	EntryNumber      string        `json:"entryNumber"`
	Amount           httpx.Numeric `json:"amount"`
	DebitAccountID   uuid.UUID     `json:"debitAccountId"`
	DebitAccountCode string        `json:"debitAccountCode"`
	DebitAccountName string        `json:"debitAccountName"`

	CreditAccountID   uuid.UUID `json:"creditAccountId"`
	CreditAccountCode string    `json:"creditAccountCode"`
	CreditAccountName string    `json:"creditAccountName"`
}

// receiptResult is §8.4's response: the receipt, where the order is now, and the
// IDs of everything the other two modules wrote.
type receiptResult struct {
	Receipt       goodsReceiptDetail     `json:"receipt"`
	PurchaseOrder receiptOrderState      `json:"purchaseOrder"`
	Inventory     receiptInventoryResult `json:"inventory"`
	Finance       receiptFinanceResult   `json:"finance"`
	// Replayed marks a response rebuilt for a repeated Idempotency-Key. The body
	// is otherwise identical to the first call's, which is the point — but a
	// client that wants to say "already posted" rather than "posted" can.
	Replayed bool `json:"replayed"`
}

// --------------------------------------------------------------------------
// Request shapes.
// --------------------------------------------------------------------------

type receiptLineInput struct {
	POLineID    string        `json:"poLineId"`
	QtyReceived httpx.Numeric `json:"qtyReceived"`
}

type receiptRequest struct {
	Note  string             `json:"note"`
	Lines []receiptLineInput `json:"lines"`
}

// resolvedReceiptLine is one validated line: which order line, and how much of
// it arrived. The product is deliberately absent — it comes from the order line,
// never from the client, so a receipt cannot book stock of something that was
// not ordered.
type resolvedReceiptLine struct {
	POLineID uuid.UUID
	Qty      httpx.Numeric
}

// --------------------------------------------------------------------------
// POST /api/procurement/purchase-orders/:id/receipts — §8.4.
// --------------------------------------------------------------------------

func (s *server) createGoodsReceipt(c *fiber.Ctx) error {
	caller, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	poID, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That purchase order")
	}

	key, err := idempotencyKey(c)
	if err != nil {
		return malformed(c, "%s", err)
	}

	var req receiptRequest
	if err := c.BodyParser(&req); err != nil {
		return malformed(c, "The request body is not valid JSON.")
	}

	// The ordinary replay: the first request succeeded and committed, and the
	// phone asked again because it never saw the answer. Answered with a read, so
	// the common case never takes a lock and never touches a constraint.
	//
	// It has to come before the status check below, not after: a retry of a receipt
	// that *completed* its order would otherwise be told the order is `received`,
	// which is a 409 for the person whose receipt worked.
	replay, err := receiptByKey(tx, key)
	if err != nil {
		return err
	}
	if replay != nil {
		return s.replayGoodsReceipt(c, tx, poID, *replay)
	}

	// ---- Step 1: lock, then validate (§8.4 step 1, §8.6.3). ----------------

	var orders []struct {
		Status      string
		WarehouseID uuid.UUID
	}
	if err := tx.Raw(`
		SELECT status, warehouse_id FROM purchase_orders WHERE id = ? FOR UPDATE`, poID).
		Scan(&orders).Error; err != nil {
		return err
	}
	if len(orders) == 0 {
		return notFound(c, "That purchase order")
	}

	// ASK ABOUT THE KEY AGAIN, now that the lock has been taken. The read above
	// could see nothing and still be a replay: a twin retry that was in flight at
	// that moment commits while this request waits for the lock. Everything below
	// would then judge this receipt against quantities its own twin has already
	// booked, and the honest-looking answer is the worst one available — `422
	// over_receipt`, telling somebody to correct a quantity for a delivery that was
	// recorded correctly a millisecond ago.
	//
	// TestConcurrentRetriesOfOneReceiptPostItOnce is this exact case, and it failed
	// on that 422 before this second read existed.
	replay, err = receiptByKey(tx, key)
	if err != nil {
		return err
	}
	if replay != nil {
		return s.replayGoodsReceipt(c, tx, poID, *replay)
	}

	if orders[0].Status != "open" && orders[0].Status != "partially_received" {
		return stateConflict(c, "This purchase order", orders[0].Status,
			"open", "partially_received")
	}

	lines, err := resolveReceiptLines(req.Lines)
	if err != nil {
		return malformed(c, "%s", err)
	}
	if len(lines) == 0 {
		return unprocessable(c, "empty_receipt",
			"Say how much of at least one line arrived — a receipt of nothing is not a receipt.")
	}

	// The lock of §8.6.3, taken in a deterministic order so two receipts touching
	// overlapping subsets of the same order cannot deadlock each other. It is a
	// separate statement from the po_line_status read below because FOR UPDATE
	// cannot be used with an aggregate, and po_line_status is a GROUP BY.
	ids := make([]uuid.UUID, 0, len(lines))
	for _, line := range lines {
		ids = append(ids, line.POLineID)
	}
	var locked []uuid.UUID
	if err := tx.Raw(`
		SELECT id FROM purchase_order_lines
		WHERE po_id = ? AND id IN ?
		ORDER BY id
		FOR UPDATE`, poID, ids).Scan(&locked).Error; err != nil {
		return err
	}
	if len(locked) != len(ids) {
		// A line that does not exist and a line belonging to another order are the
		// same 404, for the same reason every other miss is (§9.8).
		return notFound(c, "One of those order lines")
	}

	over, err := overReceiptLines(tx, lines)
	if err != nil {
		return err
	}
	if len(over) > 0 {
		return refuseOverReceipt(c, over)
	}

	// ---- Step 2: the receipt header, under a savepoint. --------------------
	//
	// The two reads above catch every replay that can be caught by reading. This is
	// the last, narrow window: a twin that commits between the second read and this
	// insert. The savepoint covers the number allocation as well as the insert, so
	// a replay detected here gives the number back rather than leaving a gap in the
	// tenant's GR sequence.
	//
	// A savepoint rather than §8.6.1's "roll back and open a second transaction":
	// see the note on replayGoodsReceipt.
	if err := tx.SavePoint("goods_receipt_header").Error; err != nil {
		return err
	}

	grNumber, err := docnum.Allocate(tx, caller.TenantID, docnum.GR)
	if err != nil {
		return err
	}
	grID := uuid.New()
	if err := tx.Exec(`
		INSERT INTO goods_receipts
		  (id, tenant_id, gr_number, po_id, received_by, note, idempotency_key)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		grID, caller.TenantID, grNumber, poID, caller.UserID,
		nullIfEmpty(trimmed(req.Note)), key).Error; err != nil {

		if db.IsUniqueViolation(err) && db.ConstraintName(err) == receiptKeyConstraint {
			if err := tx.RollbackTo("goods_receipt_header").Error; err != nil {
				return err
			}
			raced, err := receiptByKey(tx, key)
			if err != nil {
				return err
			}
			if raced == nil {
				// The row that just collided is not there on a fresh read. Nothing
				// deletes a goods receipt — erp_app has DELETE revoked — so this
				// cannot happen, and guessing at it would return a wrong answer.
				return fmt.Errorf(
					"api: idempotency key %s collided but no receipt carries it", key)
			}
			return s.replayGoodsReceipt(c, tx, poID, *raced)
		}
		return err
	}

	// ---- Step 3: the receipt lines. ----------------------------------------
	//
	// One statement, and `product_id` comes from the order line rather than from
	// the request. The grl_no_over_receipt constraint trigger fires per row here;
	// reaching it means the validation above missed something.
	values, args := receiptValues(lines)
	args = append([]any{caller.TenantID, grID}, args...)
	if err := tx.Exec(`
		INSERT INTO goods_receipt_lines
		  (id, tenant_id, gr_id, po_line_id, product_id, qty_received)
		SELECT gen_random_uuid(), ?, ?, r.po_line_id, pol.product_id, r.qty
		FROM (VALUES `+values+`) AS r(po_line_id, qty)
		JOIN purchase_order_lines pol ON pol.id = r.po_line_id`, args...).Error; err != nil {
		return err
	}

	// ---- Step 4: the order's new status. -----------------------------------
	//
	// Re-read from po_line_status, which now includes the lines just written.
	// There is no per-line quantity to update: received quantity is derived (I6).
	var complete bool
	if err := tx.Raw(`
		SELECT COALESCE(bool_and(qty_received >= qty_ordered), false)
		FROM po_line_status WHERE po_id = ?`, poID).Scan(&complete).Error; err != nil {
		return err
	}
	status := "partially_received"
	if complete {
		status = "received"
	}
	if err := tx.Exec(`
		UPDATE purchase_orders SET status = ? WHERE id = ?`, status, poID).Error; err != nil {
		return err
	}

	// ---- Step 5: [INVENTORY]. ----------------------------------------------
	ledgerIDs, err := postReceiptStockLedger(tx, caller, grID, orders[0].WarehouseID)
	if err != nil {
		return err
	}

	// ---- Step 6: [FINANCE]. ------------------------------------------------
	journal, err := postReceiptJournal(tx, caller, grID, grNumber)
	if err != nil {
		return err
	}

	// ---- Step 7: audit. ----------------------------------------------------
	// TODO(post-mvp): audit gr.posted

	// ---- Step 8: commit, which TenantTx does on the way out. ---------------
	result, err := s.receiptResult(tx, grID)
	if err != nil {
		return err
	}
	result.Inventory = receiptInventoryResult{
		LedgerEntryIDs: ledgerIDs,
		EntryCount:     len(ledgerIDs),
	}
	result.Finance = *journal
	return c.Status(fiber.StatusCreated).JSON(result)
}

// replayGoodsReceipt answers a repeated Idempotency-Key with the receipt the
// first call created, rebuilt from the database.
//
// §8.6.1 says the replay lookup needs a *second* transaction, because a unique
// violation aborts the current one. That is true of a bare INSERT, and it is why
// the insert above sits under a SAVEPOINT: ROLLBACK TO releases the failure and
// leaves this transaction usable, so the lookup happens on the connection that
// already has tenant context, no second connection is held while the first is
// still open, and TenantTx's COMMIT is a real commit rather than a commit issued
// against an aborted transaction. Under READ COMMITTED the read below takes a
// fresh snapshot, so it sees the row the racing transaction committed.
//
// The body is rebuilt rather than remembered, which is what makes it *the same*
// body: there is one function that renders a receipt result and both paths call
// it.
func (s *server) replayGoodsReceipt(c *fiber.Ctx, tx *gorm.DB, poID uuid.UUID, replay receiptRef) error {
	// A key belonging to a receipt against a different order is a client bug, and
	// the friendly reading of it — hand back that other order's receipt — is the
	// dangerous one: the phone would report goods arriving against an order nobody
	// touched.
	if replay.POID != poID {
		return unprocessable(c, "idempotency_key_reused",
			"That Idempotency-Key already belongs to a receipt against a different "+
				"purchase order. Reopen the form to get a new one.")
	}

	result, err := s.receiptResult(tx, replay.ID)
	if err != nil {
		return err
	}
	result.Replayed = true
	return c.JSON(result)
}

// --------------------------------------------------------------------------
// Reads — GET /goods-receipts and /goods-receipts/:id (§9.4).
// --------------------------------------------------------------------------

// listGoodsReceipts is the receipt history, filterable by order — which is how
// the order detail screen renders "what has arrived so far" (§10.3).
func (s *server) listGoodsReceipts(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	params, err := httpx.ParseList(c, goodsReceiptSortable, "-receivedAt")
	if err != nil {
		return malformed(c, "%s", err)
	}
	poID, ok := optionalUUID(c, "poId")
	if !ok {
		return malformed(c, "poId is not a valid id.")
	}

	where := `
		WHERE (?::uuid IS NULL OR gr.po_id = ?)
		  AND (gr.gr_number ILIKE ? OR po.po_number ILIKE ? OR s.name ILIKE ?)`
	args := []any{poID, poID, params.Like(), params.Like(), params.Like()}

	var total int64
	if err := tx.Raw(`
		SELECT count(*)
		FROM goods_receipts gr
		JOIN purchase_orders po ON po.id = gr.po_id
		JOIN suppliers       s  ON s.id = po.supplier_id`+where, args...).
		Scan(&total).Error; err != nil {
		return err
	}

	page := append(append([]any{}, args...), params.PageSize, params.Offset())
	rows, err := goodsReceiptRows(tx, where+fmt.Sprintf(`
		ORDER BY %s
		LIMIT ? OFFSET ?`, params.OrderBy("gr.id")), page...)
	if err != nil {
		return err
	}
	return c.JSON(httpx.NewListResponse(rows, params, total))
}

// getGoodsReceipt is GET /api/procurement/goods-receipts/:id.
func (s *server) getGoodsReceipt(c *fiber.Ctx) error {
	_, tx, err := tenantScope(c)
	if err != nil {
		return err
	}
	id, ok := pathUUID(c, "id")
	if !ok {
		return notFound(c, "That goods receipt")
	}

	detail, err := goodsReceiptDetailFor(tx, id)
	if err != nil {
		return err
	}
	if detail == nil {
		return notFound(c, "That goods receipt")
	}
	return c.JSON(detail)
}

// goodsReceiptRows runs the receipt projection with a caller-supplied tail.
//
// Suppliers, warehouses, and products are joined WITHOUT a deleted filter: a
// receipt is a historical document and still names who delivered what, wherever
// the master data has got to since (§6.9.1, Trap 3).
func goodsReceiptRows(tx *gorm.DB, tail string, args ...any) ([]goodsReceiptRow, error) {
	var rows []goodsReceiptRow
	err := tx.Raw(`
		SELECT gr.id, gr.gr_number,
		       gr.po_id, po.po_number, po.status AS po_status,
		       po.supplier_id, s.name AS supplier_name,
		       po.warehouse_id, w.code AS warehouse_code, w.name AS warehouse_name,
		       gr.received_by AS received_by_id, u.full_name AS received_by_name,
		       gr.received_at, gr.note,
		       COALESCE(l.line_count, 0)         AS line_count,
		       COALESCE(l.qty_received, 0)::text AS qty_received,
		       COALESCE(l.total_value, 0)::text  AS total_value
		FROM goods_receipts gr
		JOIN purchase_orders po ON po.id = gr.po_id
		JOIN suppliers       s  ON s.id = po.supplier_id
		JOIN warehouses      w  ON w.id = po.warehouse_id
		JOIN users           u  ON u.id = gr.received_by
		LEFT JOIN (
		  SELECT grl.gr_id,
		         count(*)                                              AS line_count,
		         SUM(grl.qty_received)                                 AS qty_received,
		         SUM(grl.qty_received * pol.unit_cost)::numeric(18,2)  AS total_value
		  FROM goods_receipt_lines grl
		  JOIN purchase_order_lines pol ON pol.id = grl.po_line_id
		  GROUP BY grl.gr_id
		) l ON l.gr_id = gr.id
		`+tail, args...).Scan(&rows).Error
	return rows, err
}

// goodsReceiptDetailFor reads one receipt with its lines, or nil when there is
// no such receipt in this tenant. RLS is the tenant filter.
func goodsReceiptDetailFor(tx *gorm.DB, id uuid.UUID) (*goodsReceiptDetail, error) {
	rows, err := goodsReceiptRows(tx, `WHERE gr.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	var lines []goodsReceiptLine
	if err := tx.Raw(`
		SELECT grl.id, grl.po_line_id, pol.line_no,
		       grl.product_id, p.sku, p.name AS product_name, p.uom,
		       (p.deleted_at IS NOT NULL) AS product_deleted,
		       grl.qty_received::text AS qty_received,
		       pol.unit_cost::text    AS unit_cost,
		       (grl.qty_received * pol.unit_cost)::numeric(18,2)::text AS line_total
		FROM goods_receipt_lines grl
		JOIN purchase_order_lines pol ON pol.id = grl.po_line_id
		JOIN products p ON p.id = grl.product_id
		WHERE grl.gr_id = ?
		ORDER BY pol.line_no`, id).Scan(&lines).Error; err != nil {
		return nil, err
	}
	if lines == nil {
		lines = []goodsReceiptLine{}
	}
	return &goodsReceiptDetail{goodsReceiptRow: rows[0], Lines: lines}, nil
}

// receiptRef is the minimum an idempotent replay needs to decide what to do.
type receiptRef struct {
	ID   uuid.UUID
	POID uuid.UUID
}

// receiptByKey finds the receipt carrying this idempotency key, or nil.
//
// Pure: it returns a real error and never writes a response. `goods_receipts` is
// RLS-forced, so the (tenant_id, idempotency_key) uniqueness is per tenant and
// this read cannot see another workspace's key.
func receiptByKey(tx *gorm.DB, key string) (*receiptRef, error) {
	var rows []receiptRef
	if err := tx.Raw(`
		SELECT id, po_id FROM goods_receipts WHERE idempotency_key = ?`, key).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// receiptResult rebuilds the whole §8.4 response from the database.
//
// Both the fresh post and the replay go through this, which is what makes the
// replay's body "the same body the first call returned" by construction rather
// than by two pieces of code agreeing.
func (s *server) receiptResult(tx *gorm.DB, grID uuid.UUID) (*receiptResult, error) {
	detail, err := goodsReceiptDetailFor(tx, grID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, fmt.Errorf("api: goods receipt %s was written but could not be read back", grID)
	}

	result := &receiptResult{
		Receipt: *detail,
		PurchaseOrder: receiptOrderState{
			ID:       detail.POID,
			PONumber: detail.PONumber,
			Status:   detail.POStatus,
		},
	}

	// Both of these are read back by source rather than remembered from the
	// insert, so a replay reports exactly what the first call wrote.
	var ledgerIDs []uuid.UUID
	if err := tx.Raw(`
		SELECT id FROM stock_ledger
		WHERE source_type = 'goods_receipt' AND source_id = ?
		ORDER BY id`, grID).Scan(&ledgerIDs).Error; err != nil {
		return nil, err
	}
	result.Inventory = receiptInventoryResult{
		LedgerEntryIDs: ledgerIDs,
		EntryCount:     len(ledgerIDs),
	}

	journal, err := receiptJournalFor(tx, grID)
	if err != nil {
		return nil, err
	}
	if journal != nil {
		result.Finance = *journal
	}
	return result, nil
}

// --------------------------------------------------------------------------
// Validation. All pure — none of these writes a response except the one that
// says so in its name.
// --------------------------------------------------------------------------

// idempotencyKey reads the header §8.6.1 requires.
//
// A UUID is what the contract says the client sends, and holding it to that is
// what makes "malformed" mean something: a key that is a timestamp or a form id
// repeats across forms, which is the one property this header must not have.
// Nothing is generated server-side — a key the server invents is a new key on
// every retry, which is exactly the failure it exists to prevent.
func idempotencyKey(c *fiber.Ctx) (string, error) {
	raw := trimmed(c.Get("Idempotency-Key"))
	if raw == "" {
		return "", fmt.Errorf("an Idempotency-Key header is required: generate a UUID " +
			"when the form opens and send the same one on every retry")
	}
	if _, err := uuid.Parse(raw); err != nil {
		return "", fmt.Errorf("Idempotency-Key must be a UUID")
	}
	return raw, nil
}

// resolveReceiptLines validates the submitted lines. It does not look anything
// up: which order the lines belong to is checked under the lock, afterwards.
func resolveReceiptLines(inputs []receiptLineInput) ([]resolvedReceiptLine, error) {
	resolved := make([]resolvedReceiptLine, 0, len(inputs))
	seen := map[uuid.UUID]int{}

	for i, in := range inputs {
		lineNo := i + 1

		poLineID, err := uuid.Parse(trimmed(in.POLineID))
		if err != nil {
			return nil, fmt.Errorf("line %d: poLineId is required and must be an id", lineNo)
		}
		if first, dup := seen[poLineID]; dup {
			return nil, fmt.Errorf("line %d repeats the order line on line %d; "+
				"put the whole quantity on one line", lineNo, first)
		}
		seen[poLineID] = lineNo

		qty, err := httpx.ParseNumeric(in.QtyReceived.String())
		if err != nil {
			return nil, fmt.Errorf("line %d: qtyReceived: %w", lineNo, err)
		}
		if qty.IsZero() || qty.IsNegative() {
			return nil, fmt.Errorf("line %d: qtyReceived must be greater than zero — "+
				"a return is a separate movement, not a negative receipt", lineNo)
		}

		resolved = append(resolved, resolvedReceiptLine{POLineID: poLineID, Qty: qty})
	}
	return resolved, nil
}

// overReceiptRow is one line that would receive more than was ordered, with
// everything the refusal has to be able to say about it.
type overReceiptRow struct {
	POLineID       uuid.UUID
	LineNo         int
	SKU            string
	QtyOrdered     httpx.Numeric
	QtyReceived    httpx.Numeric
	QtyOutstanding httpx.Numeric
	QtyRequested   httpx.Numeric
}

// overReceiptLines returns the lines of §8.4 step 1 that would over-receive.
//
// The comparison is `qty_received + r.qty > qty_ordered`, evaluated by
// PostgreSQL against po_line_status with the lines already locked. Doing it in
// Go would mean either arithmetic on httpx.Numeric — which does not have any,
// deliberately — or a float64 round trip that decides a business rule in a type
// that cannot represent 0.1.
//
// Pure: it returns a real error and never writes a response, so the caller's
// `if err != nil` means what it says (Trap 1).
func overReceiptLines(tx *gorm.DB, lines []resolvedReceiptLine) ([]overReceiptRow, error) {
	values, args := receiptValues(lines)

	var offending []overReceiptRow
	if err := tx.Raw(`
		SELECT st.po_line_id, pol.line_no, p.sku,
		       st.qty_ordered::text     AS qty_ordered,
		       st.qty_received::text    AS qty_received,
		       st.qty_outstanding::text AS qty_outstanding,
		       r.qty::text              AS qty_requested
		FROM (VALUES `+values+`) AS r(po_line_id, qty)
		JOIN po_line_status        st  ON st.po_line_id = r.po_line_id
		JOIN purchase_order_lines  pol ON pol.id = r.po_line_id
		JOIN products              p   ON p.id = pol.product_id
		WHERE st.qty_received + r.qty > st.qty_ordered
		ORDER BY pol.line_no`, args...).Scan(&offending).Error; err != nil {
		return nil, err
	}
	return offending, nil
}

// refuseOverReceipt is the 422 of §8.4 step 1, naming every offending line.
//
// One code for the rule and `details.lines` for the arithmetic, so the receipt
// form can put the refusal next to the box that is wrong rather than in a banner
// the user has to map back onto eight rows themselves.
func refuseOverReceipt(c *fiber.Ctx, offending []overReceiptRow) error {
	details := make([]map[string]any, 0, len(offending))
	names := make([]string, 0, len(offending))
	for _, row := range offending {
		details = append(details, map[string]any{
			"poLineId":       row.POLineID,
			"lineNo":         row.LineNo,
			"sku":            row.SKU,
			"qtyOrdered":     row.QtyOrdered,
			"qtyReceived":    row.QtyReceived,
			"qtyOutstanding": row.QtyOutstanding,
			"qtyRequested":   row.QtyRequested,
		})
		names = append(names, fmt.Sprintf("line %d (%s) has %s outstanding of %s ordered, "+
			"and %s was entered", row.LineNo, row.SKU,
			row.QtyOutstanding, row.QtyOrdered, row.QtyRequested))
	}

	return httpx.FailWith(c, fiber.StatusUnprocessableEntity, "over_receipt",
		fmt.Sprintf("More arrived than was ordered: %s. Correct the quantities, or "+
			"raise a second order for the excess.", strings.Join(names, "; ")),
		map[string]any{"lines": details})
}

// receiptValues renders the submitted lines as a SQL VALUES list, so the
// comparison and the insert can both be one statement over the whole set rather
// than a loop that asks the same question N times.
//
// The casts matter: without them PostgreSQL types a bare VALUES column as
// `text`, and `qty_received + r.qty` then fails to resolve an operator.
func receiptValues(lines []resolvedReceiptLine) (string, []any) {
	fragments := make([]string, 0, len(lines))
	args := make([]any, 0, len(lines)*2)
	for _, line := range lines {
		fragments = append(fragments, "(?::uuid, ?::numeric)")
		args = append(args, line.POLineID, line.Qty)
	}
	return strings.Join(fragments, ", "), args
}
