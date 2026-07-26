// Group D — the goods receipt, which is the critical path — and Group H's
// receipt half. Every test drives the real route built by api.New.
//
// D8 IS THE MOST VALUABLE TEST IN THE SUITE. It injects a failure at the
// journal-posting step and asserts that the goods receipt, its lines, the
// purchase order's status change, and the stock ledger entries are *all* absent.
// Everything else here checks that the handler does the right thing; D8 checks
// that when it cannot, it does nothing at all — which is the project's main
// claim, and the only one that cannot be demonstrated by a screenshot.
//
// The failure it injects is a real one rather than a test hook: the tenant's GRNI
// account is soft-deleted, so step 6 cannot resolve the account it must credit.
// No seam exists in the production code for the test to reach through, which
// means what rolls back is exactly what would roll back in production.
package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/internal/db"
	"github.com/DGosal/mini-erp/backend/testsupport"
)

// --------------------------------------------------------------------------
// Response shapes. Quantities and money decode as float64 in the test only —
// the server never sees one (I8), and comparing 40 to 40 is not where precision
// goes.
// --------------------------------------------------------------------------

type goodsReceiptLine struct {
	ID             string  `json:"id"`
	POLineID       string  `json:"poLineId"`
	LineNo         int     `json:"lineNo"`
	ProductID      string  `json:"productId"`
	SKU            string  `json:"sku"`
	ProductName    string  `json:"productName"`
	ProductDeleted bool    `json:"productDeleted"`
	QtyReceived    float64 `json:"qtyReceived"`
	UnitCost       float64 `json:"unitCost"`
	LineTotal      float64 `json:"lineTotal"`
}

type goodsReceipt struct {
	ID             string             `json:"id"`
	GRNumber       string             `json:"grNumber"`
	POID           string             `json:"poId"`
	PONumber       string             `json:"poNumber"`
	POStatus       string             `json:"poStatus"`
	SupplierName   string             `json:"supplierName"`
	WarehouseID    string             `json:"warehouseId"`
	ReceivedByID   string             `json:"receivedById"`
	ReceivedByName string             `json:"receivedByName"`
	ReceivedAt     string             `json:"receivedAt"`
	Note           *string            `json:"note"`
	LineCount      int                `json:"lineCount"`
	QtyReceived    float64            `json:"qtyReceived"`
	TotalValue     float64            `json:"totalValue"`
	Lines          []goodsReceiptLine `json:"lines"`
}

type receiptResult struct {
	Receipt       goodsReceipt `json:"receipt"`
	PurchaseOrder struct {
		ID       string `json:"id"`
		PONumber string `json:"poNumber"`
		Status   string `json:"status"`
	} `json:"purchaseOrder"`
	Inventory struct {
		LedgerEntryIDs []string `json:"ledgerEntryIds"`
		EntryCount     int      `json:"entryCount"`
	} `json:"inventory"`
	Finance struct {
		JournalEntryID    string  `json:"journalEntryId"`
		EntryNumber       string  `json:"entryNumber"`
		Amount            float64 `json:"amount"`
		DebitAccountCode  string  `json:"debitAccountCode"`
		DebitAccountName  string  `json:"debitAccountName"`
		CreditAccountCode string  `json:"creditAccountCode"`
		CreditAccountName string  `json:"creditAccountName"`
	} `json:"finance"`
	Replayed bool `json:"replayed"`
}

// --------------------------------------------------------------------------
// Arrange helpers.
// --------------------------------------------------------------------------

// orderWithLines raises a requisition, submits it, and has somebody else approve
// it — which is how a purchase order comes into existence (§8.3). Going through
// the real lifecycle rather than inserting a PO means these tests are receiving
// against an order the application itself built, lines, costs, and all.
func orderWithLines(t *testing.T, h *testsupport.Harness, f *testsupport.TenantFixture, lines ...map[string]any) purchaseOrder {
	t.Helper()
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")

	pr := submittedRequisition(t, h, f, author, lines...)
	resp := h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", approver, nil)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	decided := testsupport.Decode[requisition](t, resp)
	if decided.PurchaseOrderID == nil {
		t.Fatalf("approval produced no purchase order")
	}
	return getPurchaseOrder(t, h, approver, *decided.PurchaseOrderID)
}

// receiptLine is one line of a receipt request. The quantity is a string, so
// nothing passes through a float on the way in either.
func receiptLine(poLineID, qty string) map[string]any {
	return map[string]any{"poLineId": poLineID, "qtyReceived": qty}
}

// postReceipt issues one receipt request with an explicit idempotency key.
func postReceipt(t *testing.T, h *testsupport.Harness, token, poID, key string, body map[string]any) *http.Response {
	t.Helper()
	return h.Request(t, http.MethodPost,
		"/api/procurement/purchase-orders/"+poID+"/receipts", token, body,
		[2]string{"Idempotency-Key", key})
}

// receive posts a receipt that is expected to succeed, and returns the §8.4
// result. The key is fresh per call, which is what a client does when the form is
// reopened.
func receive(t *testing.T, h *testsupport.Harness, token, poID string, lines ...map[string]any) receiptResult {
	t.Helper()
	resp := postReceipt(t, h, token, poID, uuid.NewString(), map[string]any{
		"lines": lines,
	})
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	return testsupport.Decode[receiptResult](t, resp)
}

// counts is what the rollback and idempotency tests compare: every table the
// receipt handler writes to, counted in one place so a test cannot check three of
// them and forget the fourth.
type counts struct {
	Receipts     int64
	ReceiptLines int64
	LedgerRows   int64
	Journals     int64
	JournalLines int64
}

func tableCounts(t *testing.T, f *testsupport.TenantFixture) counts {
	t.Helper()
	var got counts
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT (SELECT count(*) FROM goods_receipts)      AS receipts,
			       (SELECT count(*) FROM goods_receipt_lines) AS receipt_lines,
			       (SELECT count(*) FROM stock_ledger)        AS ledger_rows,
			       (SELECT count(*) FROM journal_entries)     AS journals,
			       (SELECT count(*) FROM journal_entry_lines) AS journal_lines`).
			Scan(&got).Error
	})
	return got
}

func poStatus(t *testing.T, f *testsupport.TenantFixture, poID string) string {
	t.Helper()
	var status string
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT status FROM purchase_orders WHERE id = ?`, poID).
			Scan(&status).Error
	})
	return status
}

// --------------------------------------------------------------------------
// Group D — the goods receipt.
// --------------------------------------------------------------------------

// D1 — receiving everything ordered closes the order.
func TestD1FullReceiptSetsTheOrderReceived(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Full Receipt Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	result := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "40"))

	if result.PurchaseOrder.Status != "received" {
		t.Errorf("order status = %s, want received", result.PurchaseOrder.Status)
	}
	// Asserted from the database as well as from the response: the response is the
	// handler's opinion, the row is the fact.
	if got := poStatus(t, f, po.ID); got != "received" {
		t.Errorf("stored status = %s, want received", got)
	}
	if !strings.HasPrefix(result.Receipt.GRNumber, "GR-") {
		t.Errorf("gr number = %s, want a GR- number", result.Receipt.GRNumber)
	}

	after := getPurchaseOrder(t, h, receiver, po.ID)
	if after.Lines[0].QtyReceived != 40 || after.Lines[0].QtyOutstanding != 0 {
		t.Errorf("line reads %v received / %v outstanding, want 40 / 0",
			after.Lines[0].QtyReceived, after.Lines[0].QtyOutstanding)
	}
}

// D2 — a partial receipt leaves the order open for the rest, and the derived
// quantities say exactly how much is still coming.
func TestD2PartialReceiptSetsPartiallyReceived(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Partial Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	result := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "25"))

	if result.PurchaseOrder.Status != "partially_received" {
		t.Errorf("order status = %s, want partially_received", result.PurchaseOrder.Status)
	}
	if got := poStatus(t, f, po.ID); got != "partially_received" {
		t.Errorf("stored status = %s, want partially_received", got)
	}

	after := getPurchaseOrder(t, h, receiver, po.ID)
	if after.Lines[0].QtyReceived != 25 || after.Lines[0].QtyOutstanding != 15 {
		t.Errorf("line reads %v received / %v outstanding, want 25 / 15",
			after.Lines[0].QtyReceived, after.Lines[0].QtyOutstanding)
	}
	if after.QtyReceived != 25 || after.QtyOutstanding != 15 {
		t.Errorf("order header reads %v / %v, want 25 / 15",
			after.QtyReceived, after.QtyOutstanding)
	}
}

// D3 — two partial receipts that together complete the order close it.
//
// The second delivery is what a stored counter would get wrong: nothing
// increments here, the view sums both receipts, and the status follows from the
// sum rather than from what the last request happened to know.
func TestD3TwoPartialReceiptsCompleteTheOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Sequential Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))

	first := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "25"))
	if first.PurchaseOrder.Status != "partially_received" {
		t.Fatalf("after the first receipt: %s, want partially_received",
			first.PurchaseOrder.Status)
	}
	second := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "15"))
	if second.PurchaseOrder.Status != "received" {
		t.Errorf("after the second receipt: %s, want received",
			second.PurchaseOrder.Status)
	}

	after := getPurchaseOrder(t, h, receiver, po.ID)
	if after.Lines[0].QtyReceived != 40 || after.Lines[0].QtyOutstanding != 0 {
		t.Errorf("line reads %v / %v, want 40 / 0",
			after.Lines[0].QtyReceived, after.Lines[0].QtyOutstanding)
	}
	if after.Status != "received" {
		t.Errorf("order status = %s, want received", after.Status)
	}

	// Two receipts, two ledger rows, one journal entry each — the second delivery
	// is its own event, not an edit of the first.
	got := tableCounts(t, f)
	if got.Receipts != 2 || got.LedgerRows != 2 || got.Journals != 2 {
		t.Errorf("%d receipts, %d ledger rows, %d journals; want 2, 2, 2",
			got.Receipts, got.LedgerRows, got.Journals)
	}
}

// D4 — over-receipt by any amount is refused, and NOTHING is written.
//
// The "nothing" half is the point. A handler that inserted the receipt and then
// noticed the over-receipt would leave stock credited for goods it had just
// refused, and every count below is a place that could have happened.
func TestD4OverReceiptIsRefusedAndWritesNothing(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Too Much Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	before := tableCounts(t, f)

	// By one unit, which is the amount a stored counter would round away.
	body := testsupport.AssertErrorCode(t,
		postReceipt(t, h, receiver, po.ID, uuid.NewString(), map[string]any{
			"lines": []map[string]any{receiptLine(po.Lines[0].ID, "41")},
		}),
		http.StatusUnprocessableEntity, "over_receipt")

	// The refusal names the line, so the form can put it next to the box.
	detail, ok := body.Details["lines"].([]any)
	if !ok || len(detail) != 1 {
		t.Fatalf("details.lines = %v, want one entry", body.Details["lines"])
	}
	if got := detail[0].(map[string]any)["lineNo"]; got != float64(1) {
		t.Errorf("details names line %v, want 1", got)
	}

	if got := tableCounts(t, f); got != before {
		t.Errorf("the refusal wrote something: %+v, want %+v", got, before)
	}
	if got := poStatus(t, f, po.ID); got != "open" {
		t.Errorf("order status = %s, want open", got)
	}

	// And over-receipt is judged against what has *already* arrived, not against
	// the order in isolation: 25 of 40 is fine, then 16 is one too many.
	receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "25"))
	testsupport.AssertErrorCode(t,
		postReceipt(t, h, receiver, po.ID, uuid.NewString(), map[string]any{
			"lines": []map[string]any{receiptLine(po.Lines[0].ID, "16")},
		}),
		http.StatusUnprocessableEntity, "over_receipt")
	if got := poStatus(t, f, po.ID); got != "partially_received" {
		t.Errorf("order status = %s, want partially_received", got)
	}
}

// D5 — one ledger row per receipt line, with the right sign, cost, and warehouse.
//
// The cost is the one worth checking: it comes from the *order* line, not from
// the product's current standard cost, because what the goods are worth on
// arrival is what was agreed with the supplier.
func TestD5AReceiptWritesOneLedgerRowPerLine(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Ledger Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f,
		line(f.ProductID, "10", "1500.00"),
		line(f.ProductAltID, "4", "250.50"))

	result := receive(t, h, receiver, po.ID,
		receiptLine(po.Lines[0].ID, "10"),
		receiptLine(po.Lines[1].ID, "4"))

	if result.Inventory.EntryCount != 2 || len(result.Inventory.LedgerEntryIDs) != 2 {
		t.Fatalf("reported %d ledger entries, want 2", result.Inventory.EntryCount)
	}

	type row struct {
		ProductID   string
		WarehouseID string
		EntryType   string
		SourceType  string
		SourceID    string
		QtyDelta    string
		UnitCost    string
	}
	var rows []row
	f.Must(t, func(tx *gorm.DB) error {
		// Ordered by `l.unit_cost` rather than by the bare name: PostgreSQL
		// resolves an unqualified ORDER BY against the *output* columns first, and
		// those are ::text here — which sorts '250.50' above '1500.00'.
		return tx.Raw(`
			SELECT product_id, warehouse_id, entry_type, source_type, source_id,
			       qty_delta::text AS qty_delta, unit_cost::text AS unit_cost
			FROM stock_ledger l ORDER BY l.unit_cost DESC`).Scan(&rows).Error
	})
	if len(rows) != 2 {
		t.Fatalf("%d ledger rows, want 2", len(rows))
	}
	for _, got := range rows {
		if got.EntryType != "receipt" || got.SourceType != "goods_receipt" {
			t.Errorf("row is %s/%s, want receipt/goods_receipt", got.EntryType, got.SourceType)
		}
		if got.SourceID != result.Receipt.ID {
			t.Errorf("source_id = %s, want the receipt %s", got.SourceID, result.Receipt.ID)
		}
		if got.WarehouseID != f.WarehouseID.String() {
			t.Errorf("warehouse = %s, want the order's %s", got.WarehouseID, f.WarehouseID)
		}
		if strings.HasPrefix(got.QtyDelta, "-") {
			t.Errorf("qty_delta = %s, want a positive delta — goods arrived", got.QtyDelta)
		}
	}
	if rows[0].QtyDelta != "10.0000" || rows[0].UnitCost != "1500.00" {
		t.Errorf("first row = %s @ %s, want 10.0000 @ 1500.00", rows[0].QtyDelta, rows[0].UnitCost)
	}
	if rows[1].QtyDelta != "4.0000" || rows[1].UnitCost != "250.50" {
		t.Errorf("second row = %s @ %s, want 4.0000 @ 250.50", rows[1].QtyDelta, rows[1].UnitCost)
	}
}

// D6 — after a partial receipt, po_line_status reports the right quantities with
// no stored column anywhere.
//
// Asserted against the view directly as well as through the endpoint, and the
// schema is asserted too: the day somebody adds `qty_received` to
// purchase_order_lines "for performance", this fails and says why.
func TestD6PoLineStatusIsDerivedNotStored(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Derived Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "25"))

	var view []struct {
		QtyOrdered     string
		QtyReceived    string
		QtyOutstanding string
	}
	var stored int64
	f.Must(t, func(tx *gorm.DB) error {
		if err := tx.Raw(`
			SELECT qty_ordered::text, qty_received::text, qty_outstanding::text
			FROM po_line_status WHERE po_line_id = ?`, po.Lines[0].ID).
			Scan(&view).Error; err != nil {
			return err
		}
		return tx.Raw(`
			SELECT count(*) FROM information_schema.columns
			WHERE table_name = 'purchase_order_lines' AND column_name = 'qty_received'`).
			Scan(&stored).Error
	})

	if len(view) != 1 {
		t.Fatalf("%d rows in po_line_status, want 1", len(view))
	}
	if view[0].QtyReceived != "25.0000" || view[0].QtyOutstanding != "15.0000" {
		t.Errorf("view reads %s received / %s outstanding, want 25.0000 / 15.0000",
			view[0].QtyReceived, view[0].QtyOutstanding)
	}
	if stored != 0 {
		t.Error("purchase_order_lines has a qty_received column — received quantity " +
			"must stay derived (I6), or it can drift from the receipts it counts")
	}
}

// D7 — one journal entry per receipt, and it balances.
func TestD7AReceiptPostsOneBalancedJournalEntry(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Balanced Ltd")
	receiver := procurementUser(t, f, "approver")

	// 10 × 1500.00 + 4 × 250.50 = 16002.00
	po := orderWithLines(t, h, f,
		line(f.ProductID, "10", "1500.00"),
		line(f.ProductAltID, "4", "250.50"))
	result := receive(t, h, receiver, po.ID,
		receiptLine(po.Lines[0].ID, "10"),
		receiptLine(po.Lines[1].ID, "4"))

	if result.Finance.Amount != 16002 {
		t.Errorf("posted amount = %v, want 16002", result.Finance.Amount)
	}
	if result.Finance.DebitAccountCode != "1300" || result.Finance.CreditAccountCode != "2150" {
		t.Errorf("posted Dr %s / Cr %s, want Dr 1300 / Cr 2150",
			result.Finance.DebitAccountCode, result.Finance.CreditAccountCode)
	}
	if !strings.HasPrefix(result.Finance.EntryNumber, "JE-") {
		t.Errorf("entry number = %s, want a JE- number", result.Finance.EntryNumber)
	}

	var entries []struct {
		EntryNumber string
		SourceType  string
		SourceID    string
		Lines       int
		Debits      string
		Credits     string
	}
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT je.entry_number, je.source_type, je.source_id,
			       count(jel.id)          AS lines,
			       SUM(jel.debit)::text   AS debits,
			       SUM(jel.credit)::text  AS credits
			FROM journal_entries je
			JOIN journal_entry_lines jel ON jel.journal_entry_id = je.id
			GROUP BY je.id, je.entry_number, je.source_type, je.source_id`).
			Scan(&entries).Error
	})
	if len(entries) != 1 {
		t.Fatalf("%d journal entries, want exactly 1", len(entries))
	}
	got := entries[0]
	if got.Lines != 2 {
		t.Errorf("%d journal lines, want 2", got.Lines)
	}
	if got.Debits != got.Credits {
		t.Errorf("debits %s, credits %s — an unbalanced entry is a corrupt ledger",
			got.Debits, got.Credits)
	}
	if got.Debits != "16002.00" {
		t.Errorf("debits = %s, want 16002.00", got.Debits)
	}
	if got.SourceType != "goods_receipt" || got.SourceID != result.Receipt.ID {
		t.Errorf("entry is sourced %s/%s, want goods_receipt/%s",
			got.SourceType, got.SourceID, result.Receipt.ID)
	}
}

// D8 — THE ROLLBACK TEST. A failure at the journal-posting step leaves nothing
// behind: no receipt, no receipt lines, no PO status change, no stock ledger
// rows.
//
// This is the whole claim of the project. The failure is injected by soft-
// deleting the tenant's GRNI account, so step 6 cannot resolve the account it
// must credit — a real failure on the real code path, with no test hook in the
// handler for it to reach through.
//
// Note what has *already* happened by the time the failure lands: the receipt is
// inserted, its lines are inserted, the order has been moved to `received`, and
// two stock ledger rows exist. Every one of those is asserted absent afterwards.
func TestD8AFailedJournalRollsBackTheWholeReceipt(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Atomic Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f,
		line(f.ProductID, "10", "1500.00"),
		line(f.ProductAltID, "4", "250.50"))
	before := tableCounts(t, f)

	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE accounts SET deleted_at = now() WHERE code = '2150'`).Error
	})

	resp := postReceipt(t, h, receiver, po.ID, uuid.NewString(), map[string]any{
		"lines": []map[string]any{
			receiptLine(po.Lines[0].ID, "10"),
			receiptLine(po.Lines[1].ID, "4"),
		},
	})
	testsupport.AssertErrorCode(t, resp, http.StatusInternalServerError, "internal_error")

	after := tableCounts(t, f)
	if after != before {
		t.Errorf("the failed receipt left rows behind: %+v, want %+v", after, before)
	}
	if after.Receipts != 0 {
		t.Error("the goods receipt survived a failure in the journal it produced")
	}
	if after.LedgerRows != 0 {
		t.Error("stock was credited for a receipt that did not happen — this is the " +
			"failure the single-transaction design exists to prevent")
	}
	if got := poStatus(t, f, po.ID); got != "open" {
		t.Errorf("order status = %s, want open — the status change must roll back too", got)
	}

	// The number is not consumed either: the whole transaction went back,
	// including the document_sequences upsert (E4, through this path).
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE accounts SET deleted_at = NULL WHERE code = '2150'`).Error
	})
	repaired := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "10"))
	if !strings.HasSuffix(repaired.Receipt.GRNumber, "-0001") {
		t.Errorf("the first successful GR number is %s, want -0001 — the failed "+
			"receipt consumed one", repaired.Receipt.GRNumber)
	}
}

// D9 — stock balances after a receipt equal the sum of the ledger deltas.
//
// Not a tautology: the balance comes from the stock_balances view and the sum
// from the ledger table, and the view is the thing every screen reads. A receipt
// that wrote the ledger without the view agreeing would mean the goods are in the
// history and not on the shelf.
func TestD9BalancesAfterAReceiptEqualTheLedgerSum(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Balances Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "25"))

	var balance, ledgerSum string
	f.Must(t, func(tx *gorm.DB) error {
		if err := tx.Raw(`
			SELECT qty_on_hand::text FROM stock_balances
			WHERE product_id = ? AND warehouse_id = ?`, f.ProductID, f.WarehouseID).
			Scan(&balance).Error; err != nil {
			return err
		}
		return tx.Raw(`
			SELECT COALESCE(SUM(qty_delta), 0)::text FROM stock_ledger
			WHERE product_id = ? AND warehouse_id = ?`, f.ProductID, f.WarehouseID).
			Scan(&ledgerSum).Error
	})
	if balance != ledgerSum {
		t.Errorf("stock_balances says %s, the ledger sums to %s", balance, ledgerSum)
	}
	if balance != "25.0000" {
		t.Errorf("balance = %s, want 25.0000", balance)
	}
}

// --------------------------------------------------------------------------
// Group H — idempotency and concurrency.
// --------------------------------------------------------------------------

// H1 — a replayed Idempotency-Key returns the original receipt and writes nothing
// twice.
//
// This is the loading-dock case: the request succeeded, the phone never saw the
// answer, and the user tapped Post again. Without this, stock is credited twice
// with two journal entries to match, and nothing in the schema would flag it —
// because a second partial receipt is a legitimate operation.
func TestH1ReplayingAReceiptReturnsTheOriginal(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Retry Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	key := uuid.NewString()
	body := map[string]any{"lines": []map[string]any{receiptLine(po.Lines[0].ID, "25")}}

	first := postReceipt(t, h, receiver, po.ID, key, body)
	testsupport.AssertStatus(t, first, http.StatusCreated)
	original := testsupport.Decode[receiptResult](t, first)
	if original.Replayed {
		t.Error("the first call reported itself as a replay")
	}
	after := tableCounts(t, f)

	// The same key, twice more — a flaky connection retries more than once.
	for attempt := range 2 {
		resp := postReceipt(t, h, receiver, po.ID, key, body)
		testsupport.AssertStatus(t, resp, http.StatusOK)
		replay := testsupport.Decode[receiptResult](t, resp)

		if !replay.Replayed {
			t.Errorf("attempt %d: replayed = false, want true", attempt+2)
		}
		if replay.Receipt.ID != original.Receipt.ID ||
			replay.Receipt.GRNumber != original.Receipt.GRNumber {
			t.Errorf("attempt %d returned receipt %s/%s, want the original %s/%s",
				attempt+2, replay.Receipt.ID, replay.Receipt.GRNumber,
				original.Receipt.ID, original.Receipt.GRNumber)
		}
		if replay.Finance.JournalEntryID != original.Finance.JournalEntryID ||
			replay.Finance.EntryNumber != original.Finance.EntryNumber {
			t.Errorf("attempt %d reported journal entry %s, want the original %s",
				attempt+2, replay.Finance.EntryNumber, original.Finance.EntryNumber)
		}
		if len(replay.Inventory.LedgerEntryIDs) != len(original.Inventory.LedgerEntryIDs) {
			t.Errorf("attempt %d reported %d ledger entries, want %d",
				attempt+2, len(replay.Inventory.LedgerEntryIDs),
				len(original.Inventory.LedgerEntryIDs))
		}
	}

	if got := tableCounts(t, f); got != after {
		t.Errorf("replays wrote rows: %+v, want %+v", got, after)
	}
	if got := poStatus(t, f, po.ID); got != "partially_received" {
		t.Errorf("order status = %s, want partially_received", got)
	}
}

// The other half of H1: two retries genuinely in flight at once, with one key
// between them.
//
// H1's replays are answered by the read at the top of the handler, because the
// first request had committed before the retry arrived. Here neither has: both pass
// that read, one takes the order lock and posts, and the other is still waiting for
// the lock when it does.
//
// THIS TEST FOUND A REAL BUG. With only the one read at the top of the handler, the
// loser woke up holding the lock and judged its own twin's already-booked 25
// against the 40 ordered — and answered `422 over_receipt`. So the ordinary
// flaky-wifi retry, the exact case the key exists for, told the user that too much
// had arrived and asked them to correct a receipt that had posted correctly a
// millisecond earlier. The fix is the second `receiptByKey` read, taken after the
// lock and before any quantity is judged; the savepoint below it is now only the
// last narrow window rather than the mechanism.
func TestConcurrentRetriesOfOneReceiptPostItOnce(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Flaky Wifi Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	key := uuid.NewString()

	type outcome struct {
		status   int
		grNumber string
		replayed bool
		err      error
	}
	results := make(chan outcome, 2)

	for range 2 {
		go func() {
			payload := fmt.Sprintf(`{"lines":[{"poLineId":%q,"qtyReceived":"25"}]}`,
				po.Lines[0].ID)
			req := httptest.NewRequest(http.MethodPost,
				"/api/procurement/purchase-orders/"+po.ID+"/receipts",
				strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+receiver)
			// The same key on both, which is exactly what a retry sends.
			req.Header.Set("Idempotency-Key", key)

			resp, err := h.App.Test(req, -1)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

			var body struct {
				Receipt  struct{ GRNumber string } `json:"receipt"`
				Replayed bool                      `json:"replayed"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			results <- outcome{
				status:   resp.StatusCode,
				grNumber: body.Receipt.GRNumber,
				replayed: body.Replayed,
			}
		}()
	}

	var posted, replayed int
	numbers := map[string]bool{}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("request failed: %v", got.err)
		}
		switch {
		case got.status == http.StatusCreated && !got.replayed:
			posted++
		case got.status == http.StatusOK && got.replayed:
			replayed++
		default:
			t.Fatalf("unexpected outcome: %d replayed=%v — a 500 here means the "+
				"savepoint did not leave the transaction usable",
				got.status, got.replayed)
		}
		numbers[got.grNumber] = true
	}
	if posted != 1 || replayed != 1 {
		t.Fatalf("%d posted and %d replayed; want exactly one of each", posted, replayed)
	}
	if len(numbers) != 1 {
		t.Errorf("the two responses named different receipts: %v — a replay must "+
			"return the original", numbers)
	}

	// One receipt, one ledger row, one journal entry. Two would be stock credited
	// twice for one delivery, which is the whole reason the key exists.
	got := tableCounts(t, f)
	if got.Receipts != 1 || got.ReceiptLines != 1 || got.LedgerRows != 1 ||
		got.Journals != 1 || got.JournalLines != 2 {
		t.Errorf("wrote %+v; want 1 receipt, 1 line, 1 ledger row, 1 journal, 2 journal lines",
			got)
	}
	// And the loser's rolled-back allocation did not eat a number.
	next := receive(t, h, receiver, po.ID, receiptLine(po.Lines[0].ID, "15"))
	if !strings.HasSuffix(next.Receipt.GRNumber, "-0002") {
		t.Errorf("the next GR number is %s, want -0002 — the refused allocation "+
			"consumed one", next.Receipt.GRNumber)
	}
}

// H2 — no Idempotency-Key header is a 400, and the server does not invent one.
func TestH2AReceiptWithoutAnIdempotencyKeyIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Keyless Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	body := map[string]any{"lines": []map[string]any{receiptLine(po.Lines[0].ID, "1")}}

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/purchase-orders/"+po.ID+"/receipts", receiver, body),
		http.StatusBadRequest, "malformed")

	// Malformed counts too: a key that is not a UUID is a key that might repeat
	// across forms, which is the one property it must not have.
	testsupport.AssertErrorCode(t,
		postReceipt(t, h, receiver, po.ID, "form-42", body),
		http.StatusBadRequest, "malformed")

	if got := tableCounts(t, f); got.Receipts != 0 {
		t.Errorf("%d receipts were written by refused requests", got.Receipts)
	}
}

// H3 — two *different* keys against the same order both post. Idempotency must
// not turn a genuine second delivery into a silent no-op.
func TestH3TwoDifferentKeysBothPost(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Two Deliveries Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	body := map[string]any{"lines": []map[string]any{receiptLine(po.Lines[0].ID, "20")}}

	first := postReceipt(t, h, receiver, po.ID, uuid.NewString(), body)
	testsupport.AssertStatus(t, first, http.StatusCreated)
	one := testsupport.Decode[receiptResult](t, first)

	second := postReceipt(t, h, receiver, po.ID, uuid.NewString(), body)
	testsupport.AssertStatus(t, second, http.StatusCreated)
	two := testsupport.Decode[receiptResult](t, second)

	if one.Receipt.ID == two.Receipt.ID || one.Receipt.GRNumber == two.Receipt.GRNumber {
		t.Fatalf("both deliveries came back as %s — the second was swallowed",
			one.Receipt.GRNumber)
	}
	if two.PurchaseOrder.Status != "received" {
		t.Errorf("order status = %s, want received", two.PurchaseOrder.Status)
	}

	got := tableCounts(t, f)
	if got.Receipts != 2 || got.LedgerRows != 2 || got.Journals != 2 {
		t.Errorf("%d receipts, %d ledger rows, %d journals; want 2, 2, 2",
			got.Receipts, got.LedgerRows, got.Journals)
	}
}

// H5 — two receipts racing to over-receive one line: one wins, one is refused,
// and the derived quantity never exceeds what was ordered.
//
// This is why step 1 locks the order and its lines before reading po_line_status.
// Without the lock both requests read `qty_received = 0`, both find 25 of 40
// acceptable, and the line ends up at 50 received against 40 ordered — with the
// grl_no_over_receipt trigger turning the second one into a 500 rather than
// corrupt data.
func TestH5ConcurrentReceiptsCannotJointlyOverReceive(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Race Dock Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	poLineID := po.Lines[0].ID

	type outcome struct {
		status int
		code   string
		err    error
	}
	results := make(chan outcome, 2)

	for range 2 {
		// Deliberately not using the harness helpers: they call t.Fatalf and
		// t.Cleanup, neither of which is safe from a non-test goroutine.
		go func() {
			payload := fmt.Sprintf(`{"lines":[{"poLineId":%q,"qtyReceived":"25"}]}`, poLineID)
			req := httptest.NewRequest(http.MethodPost,
				"/api/procurement/purchase-orders/"+po.ID+"/receipts",
				strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+receiver)
			// Different keys: these are two people posting two receipts, not one
			// person retrying. Idempotency must not be what saves this.
			req.Header.Set("Idempotency-Key", uuid.NewString())

			resp, err := h.App.Test(req, -1)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

			var envelope testsupport.ErrorBody
			_ = json.NewDecoder(resp.Body).Decode(&envelope)
			results <- outcome{status: resp.StatusCode, code: envelope.Code}
		}()
	}

	var posted, refused int
	for range 2 {
		got := <-results
		switch {
		case got.err != nil:
			t.Fatalf("request failed: %v", got.err)
		case got.status == http.StatusCreated:
			posted++
		case got.status == http.StatusUnprocessableEntity && got.code == "over_receipt":
			refused++
		default:
			t.Fatalf("unexpected outcome: %d %q — a trigger violation reaching the "+
				"client means the handler lock was missed", got.status, got.code)
		}
	}
	if posted != 1 || refused != 1 {
		t.Fatalf("%d posted and %d were refused; want exactly one of each", posted, refused)
	}

	// The invariant, read from the view rather than from the responses.
	var received, ordered string
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT qty_received::text, qty_ordered::text
			FROM po_line_status WHERE po_line_id = ?`, poLineID).
			Row().Scan(&received, &ordered)
	})
	if received != "25.0000" {
		t.Errorf("qty_received = %s of %s ordered, want 25.0000", received, ordered)
	}
}

// Two receipts arriving at once against *different* lines of one order, each
// completing its own line. Both must post, and the order must end up `received`.
//
// This is why step 1 locks the purchase order header and not only the lines. With
// only the line locks, neither transaction blocks: each re-reads po_line_status,
// sees its own line complete and the *other* line still outstanding — because the
// other transaction has not committed — and each writes `partially_received`. The
// order then sits half-received forever with nothing outstanding on it, which no
// screen can explain and no later receipt can fix.
//
// HONEST NOTE, MEASURED RATHER THAN ASSUMED: removing the header lock does not
// currently make this test fail, and neither does removing the line lock. The
// reason is `docnum.Allocate`: every receipt in a tenant upserts the same
// `document_sequences` row for the GR sequence, and that row lock serialises all
// of them for the rest of the transaction, long before either reaches step 4. Give
// each request its own sequence row and the bug appears immediately — 10 attempts
// out of 10 left the order `partially_received` with nothing outstanding, and
// restoring the header lock alone fixed all 10. So the lock is load-bearing and
// this test cannot see it, because a numbering detail is standing in front of it.
// The locks stay explicit for exactly that reason: the serialisation this handler
// needs must not depend on how documents happen to be numbered.
func TestConcurrentReceiptsOnDifferentLinesCloseTheOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Two Docks Ltd")
	receiver := procurementUser(t, f, "approver")

	po := orderWithLines(t, h, f,
		line(f.ProductID, "10", "100.00"),
		line(f.ProductAltID, "4", "50.00"))

	type outcome struct {
		status int
		err    error
	}
	results := make(chan outcome, 2)

	for _, spec := range []struct{ poLineID, qty string }{
		{po.Lines[0].ID, "10"},
		{po.Lines[1].ID, "4"},
	} {
		go func(poLineID, qty string) {
			payload := fmt.Sprintf(`{"lines":[{"poLineId":%q,"qtyReceived":%q}]}`, poLineID, qty)
			req := httptest.NewRequest(http.MethodPost,
				"/api/procurement/purchase-orders/"+po.ID+"/receipts",
				strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+receiver)
			req.Header.Set("Idempotency-Key", uuid.NewString())

			resp, err := h.App.Test(req, -1)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			results <- outcome{status: resp.StatusCode}
		}(spec.poLineID, spec.qty)
	}

	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("request failed: %v", got.err)
		}
		if got.status != http.StatusCreated {
			t.Fatalf("status = %d, want 201 — both lines were within their order",
				got.status)
		}
	}

	if got := poStatus(t, f, po.ID); got != "received" {
		t.Errorf("order status = %s, want received — everything ordered has arrived, "+
			"so the two receipts have to agree about that", got)
	}
}

// H6 — the trigger backstop. A raw INSERT past the service layer, in an
// over-receiving amount, must still be refused.
//
// This is what makes the guard a property of the database rather than a promise
// of the handler: `grl_no_over_receipt` fires on any insert from any code path,
// including a psql session (§6.10.6).
func TestH6TheOverReceiptTriggerRefusesRawInserts(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Backstop Ltd")

	po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	poLineID := uuid.MustParse(po.Lines[0].ID)
	grID := f.NewGoodsReceipt(t, uuid.MustParse(po.ID))

	// 41 against 40 ordered, inserted as erp_app with the same grants the
	// application has, bypassing every check in the handler.
	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO goods_receipt_lines
			  (id, tenant_id, gr_id, po_line_id, product_id, qty_received)
			VALUES (?, ?, ?, ?, ?, 41)`,
			uuid.New(), f.ID, grID, poLineID, f.ProductID).Error
	})
	if err == nil {
		t.Fatal("the raw over-receiving insert was accepted — grl_no_over_receipt " +
			"is the guarantee behind the handler's check, and it is not holding")
	}
	if !testsupport.IsPGCode(err, testsupport.CodeCheckViolation) {
		t.Errorf("SQLSTATE = %s, want %s (check_violation): %v",
			testsupport.PGCode(err), testsupport.CodeCheckViolation, err)
	}

	// And the same insert at exactly the ordered quantity is accepted, so the
	// refusal is about the amount rather than about the trigger refusing
	// everything.
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO goods_receipt_lines
			  (id, tenant_id, gr_id, po_line_id, product_id, qty_received)
			VALUES (?, ?, ?, ?, ?, 40)`,
			uuid.New(), f.ID, grID, poLineID, f.ProductID).Error
	}); err != nil {
		t.Errorf("receiving exactly the ordered quantity was refused: %v", err)
	}
}

// --------------------------------------------------------------------------
// The rest of the receipt contract.
// --------------------------------------------------------------------------

// The constraint the replay path matches on has to be spelled exactly right, and
// a rename in a migration would otherwise turn every replay into a 500 that only
// shows up on warehouse wifi.
//
// Matching the *name* rather than bare SQLSTATE 23505 is the point: a duplicate
// gr_number is also a unique violation and is a real numbering bug, so answering
// it with a 200 would hide it.
func TestTheIdempotencyConstraintIsNamedWhatTheHandlerMatchesOn(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Constraint Ltd")
	po := f.NewPurchaseOrder(t)

	err := f.AsTenant(t, func(tx *gorm.DB) error {
		for range 2 {
			if err := tx.Exec(`
				INSERT INTO goods_receipts
				  (id, tenant_id, gr_number, po_id, received_by, idempotency_key)
				VALUES (gen_random_uuid(), ?, ?, ?, ?, 'the-same-key')`,
				f.ID, "GR-202607-"+uuid.NewString()[:4], po, f.User.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		t.Fatal("two receipts with one idempotency key were accepted")
	}
	// The handler branches on this exact string; see receiptKeyConstraint.
	const want = "goods_receipts_tenant_id_idempotency_key_key"
	if got := db.ConstraintName(err); got != want {
		t.Errorf("constraint name = %q, want %q — the replay path in "+
			"procurement_receipts.go matches on the latter", got, want)
	}
}

// Receiving against an order that has moved on is a 409 carrying where it went,
// so the screen can refresh rather than re-ask.
func TestReceivingAgainstAClosedOrderIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Closed Ltd")
	receiver := procurementUser(t, f, "approver")

	for _, status := range []string{"received", "cancelled"} {
		po := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
		f.SetPOStatus(t, uuid.MustParse(po.ID), status)

		body := testsupport.AssertErrorCode(t,
			postReceipt(t, h, receiver, po.ID, uuid.NewString(), map[string]any{
				"lines": []map[string]any{receiptLine(po.Lines[0].ID, "1")},
			}),
			http.StatusConflict, "state_conflict")
		if body.Details["status"] != status {
			t.Errorf("details.status = %v, want %s", body.Details["status"], status)
		}
	}
}

// A line belonging to another order is the same 404 an unknown line gets: which
// order a line sits on is not something a caller may probe.
func TestAReceiptCannotNameAnotherOrdersLine(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Wrong Line Ltd")
	receiver := procurementUser(t, f, "approver")

	mine := orderWithLines(t, h, f, line(f.ProductID, "10", "100.00"))
	theirs := orderWithLines(t, h, f, line(f.ProductID, "10", "100.00"))

	for _, poLineID := range []string{theirs.Lines[0].ID, uuid.NewString()} {
		testsupport.AssertStatus(t,
			postReceipt(t, h, receiver, mine.ID, uuid.NewString(), map[string]any{
				"lines": []map[string]any{receiptLine(poLineID, "1")},
			}),
			http.StatusNotFound)
	}
	if got := tableCounts(t, f); got.Receipts != 0 {
		t.Errorf("%d receipts were written by refused requests", got.Receipts)
	}
}

// Empty and malformed receipt bodies, which are refusals rather than 500s.
func TestAReceiptNeedsAtLeastOneUsableLine(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Nothing Arrived Ltd")
	receiver := procurementUser(t, f, "approver")
	po := orderWithLines(t, h, f, line(f.ProductID, "10", "100.00"))

	testsupport.AssertErrorCode(t,
		postReceipt(t, h, receiver, po.ID, uuid.NewString(),
			map[string]any{"lines": []map[string]any{}}),
		http.StatusUnprocessableEntity, "empty_receipt")

	for _, qty := range []string{"0", "-5"} {
		testsupport.AssertErrorCode(t,
			postReceipt(t, h, receiver, po.ID, uuid.NewString(), map[string]any{
				"lines": []map[string]any{receiptLine(po.Lines[0].ID, qty)},
			}),
			http.StatusBadRequest, "malformed")
	}

	// The same line twice in one request: legal in the schema, and refused here so
	// the over-receipt check has one number per line to compare.
	testsupport.AssertErrorCode(t,
		postReceipt(t, h, receiver, po.ID, uuid.NewString(), map[string]any{
			"lines": []map[string]any{
				receiptLine(po.Lines[0].ID, "4"),
				receiptLine(po.Lines[0].ID, "6"),
			},
		}),
		http.StatusBadRequest, "malformed")
}

// A key already used against a different order is refused rather than answered
// with that other order's receipt — which is the friendly reading, and the
// dangerous one: the phone would report goods arriving against an order nobody
// touched.
func TestAnIdempotencyKeyCannotBeReusedAcrossOrders(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Reused Key Ltd")
	receiver := procurementUser(t, f, "approver")

	first := orderWithLines(t, h, f, line(f.ProductID, "10", "100.00"))
	second := orderWithLines(t, h, f, line(f.ProductID, "10", "100.00"))

	key := uuid.NewString()
	testsupport.AssertStatus(t,
		postReceipt(t, h, receiver, first.ID, key, map[string]any{
			"lines": []map[string]any{receiptLine(first.Lines[0].ID, "10")},
		}),
		http.StatusCreated)

	testsupport.AssertErrorCode(t,
		postReceipt(t, h, receiver, second.ID, key, map[string]any{
			"lines": []map[string]any{receiptLine(second.Lines[0].ID, "10")},
		}),
		http.StatusUnprocessableEntity, "idempotency_key_reused")

	if got := poStatus(t, f, second.ID); got != "open" {
		t.Errorf("the second order is %s, want open", got)
	}
}

// The receipt history, which is what the order detail screen reads (§10.3).
func TestGoodsReceiptsAreReadableAndFilterableByOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "History Ltd")
	receiver := procurementUser(t, f, "approver")
	viewer := procurementUser(t, f, "viewer")

	first := orderWithLines(t, h, f, line(f.ProductID, "40", "1000.00"))
	second := orderWithLines(t, h, f, line(f.ProductAltID, "5", "200.00"))

	one := receive(t, h, receiver, first.ID, receiptLine(first.Lines[0].ID, "25"))
	two := receive(t, h, receiver, first.ID, receiptLine(first.Lines[0].ID, "15"))
	other := receive(t, h, receiver, second.ID, receiptLine(second.Lines[0].ID, "5"))

	all := testsupport.Decode[list[goodsReceipt]](t,
		h.Get(t, "/api/procurement/goods-receipts?pageSize=100", viewer))
	if all.TotalItems != 3 {
		t.Errorf("totalItems = %d, want 3", all.TotalItems)
	}

	// Filtered to one order, which is the query the order screen makes.
	mine := testsupport.Decode[list[goodsReceipt]](t,
		h.Get(t, "/api/procurement/goods-receipts?pageSize=100&poId="+first.ID, viewer))
	if mine.TotalItems != 2 {
		t.Fatalf("poId filter returned %d, want 2", mine.TotalItems)
	}
	for _, row := range mine.Data {
		if row.POID != first.ID {
			t.Errorf("the filter returned a receipt against %s", row.POID)
		}
		if row.ID != one.Receipt.ID && row.ID != two.Receipt.ID {
			t.Errorf("unexpected receipt %s in the filtered list", row.ID)
		}
	}

	// And by id, with its lines and the value the journal was posted for.
	resp := h.Get(t, "/api/procurement/goods-receipts/"+other.Receipt.ID, viewer)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	detail := testsupport.Decode[goodsReceipt](t, resp)
	if len(detail.Lines) != 1 || detail.Lines[0].QtyReceived != 5 {
		t.Fatalf("detail has %d lines, first qty %v", len(detail.Lines),
			detail.Lines[0].QtyReceived)
	}
	if detail.TotalValue != 1000 {
		t.Errorf("total value = %v, want 1000 (5 × 200.00)", detail.TotalValue)
	}
	if detail.POStatus != "received" {
		t.Errorf("the receipt reports its order as %s, want received", detail.POStatus)
	}
}

// The receipt's ledger rows are findable by the document that wrote them, which
// is what the confirmation panel links to. Without the filter, "2 stock ledger
// entries created" is a claim the reader cannot check.
func TestAReceiptsLedgerRowsAreFindableByItsSourceId(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Traceable Ltd")
	receiver := procurementUser(t, f, "approver")
	inventory := f.NewUser(t, map[string]string{"inventory": "viewer"}).FirebaseUID

	po := orderWithLines(t, h, f,
		line(f.ProductID, "10", "1500.00"),
		line(f.ProductAltID, "4", "250.50"))
	result := receive(t, h, receiver, po.ID,
		receiptLine(po.Lines[0].ID, "10"),
		receiptLine(po.Lines[1].ID, "4"))

	page := testsupport.Decode[list[ledgerEntry]](t,
		h.Get(t, "/api/inventory/ledger?pageSize=100&sourceId="+result.Receipt.ID, inventory))
	if page.TotalItems != 2 {
		t.Fatalf("sourceId filter returned %d rows, want 2", page.TotalItems)
	}
	for _, row := range page.Data {
		if row.SourceType != "goods_receipt" || row.SourceID == nil ||
			*row.SourceID != result.Receipt.ID {
			t.Errorf("row is sourced %s/%v, want goods_receipt/%s",
				row.SourceType, row.SourceID, result.Receipt.ID)
		}
		// The number resolved from the document, so a ledger row can name where it
		// came from rather than showing a UUID.
		if row.SourceNumber == nil || *row.SourceNumber != result.Receipt.GRNumber {
			t.Errorf("sourceNumber = %v, want %s", row.SourceNumber, result.Receipt.GRNumber)
		}
	}
}

// Isolation, asserted from two tenants. Every table the receipt writes is
// RLS-forced, so this is really a test that the handler stayed on the transaction
// TenantTx opened.
func TestReceiptsAreNotVisibleOrWritableAcrossTenants(t *testing.T) {
	h := testsupport.NewHarness(t)
	a := h.DB.NewTenant(t, "Dock A")
	b := h.DB.NewTenant(t, "Dock B")
	tokenA := procurementUser(t, a, "approver")
	tokenB := procurementUser(t, b, "approver")

	theirOrder := orderWithLines(t, h, b, line(b.ProductID, "10", "100.00"))
	theirs := receive(t, h, tokenB, theirOrder.ID, receiptLine(theirOrder.Lines[0].ID, "4"))

	// A cannot see it, in the list or by id.
	page := testsupport.Decode[list[goodsReceipt]](t,
		h.Get(t, "/api/procurement/goods-receipts?pageSize=100", tokenA))
	if page.TotalItems != 0 {
		t.Errorf("tenant A sees %d of tenant B's receipts", page.TotalItems)
	}
	testsupport.AssertStatus(t,
		h.Get(t, "/api/procurement/goods-receipts/"+theirs.Receipt.ID, tokenA),
		http.StatusNotFound)

	// Nor receive against their order — the same 404 an unknown order gets.
	testsupport.AssertStatus(t,
		postReceipt(t, h, tokenA, theirOrder.ID, uuid.NewString(), map[string]any{
			"lines": []map[string]any{receiptLine(theirOrder.Lines[0].ID, "1")},
		}),
		http.StatusNotFound)

	// B's own data is intact, which is what makes the emptiness above a filtering
	// result rather than a broken fixture.
	mine := testsupport.Decode[list[goodsReceipt]](t,
		h.Get(t, "/api/procurement/goods-receipts?pageSize=100", tokenB))
	if mine.TotalItems != 1 {
		t.Errorf("tenant B sees %d of their own receipts, want 1", mine.TotalItems)
	}
	if got := poStatus(t, b, theirOrder.ID); got != "partially_received" {
		t.Errorf("tenant B's order is %s, want partially_received", got)
	}
}

// The three routes §9.4 adds, at the levels it gives them. The receipt is
// `approver` and the reads are `viewer`; a route registered one level too low
// would pass every other test in this file.
func TestReceiptRoutesCarryTheLevelsFromTheSpec(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Receipt Levels Ltd")
	viewer := procurementUser(t, f, "viewer")
	user := procurementUser(t, f, "user")
	elsewhere := f.NewUser(t, map[string]string{"inventory": "admin"}).FirebaseUID

	po := orderWithLines(t, h, f, line(f.ProductID, "10", "100.00"))
	receipt := receive(t, h, procurementUser(t, f, "approver"), po.ID,
		receiptLine(po.Lines[0].ID, "4"))

	for _, path := range []string{
		"/api/procurement/goods-receipts",
		"/api/procurement/goods-receipts/" + receipt.Receipt.ID,
	} {
		testsupport.AssertStatus(t, h.Get(t, path, viewer), http.StatusOK)
		testsupport.AssertErrorCode(t, h.Get(t, path, elsewhere),
			http.StatusForbidden, "insufficient_module_role")
	}

	body := testsupport.AssertErrorCode(t,
		postReceipt(t, h, user, po.ID, uuid.NewString(), map[string]any{
			"lines": []map[string]any{receiptLine(po.Lines[0].ID, "1")},
		}),
		http.StatusForbidden, "insufficient_module_role")
	if body.Details["required"] != "approver" {
		t.Errorf("required = %v, want approver", body.Details["required"])
	}
}
