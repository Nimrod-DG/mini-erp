// Group C — procurement rules, Group E's HTTP half, and the Phase 5 half of
// Group G. Every test drives the real routes built by api.New, gated by the real
// RequireModule, so what is asserted is what ships.
//
// The two that would catch a silent design failure are H4 and C2. H4 is the only
// thing that proves the `SELECT … FOR UPDATE` in lockRequisition is load-bearing:
// without it, two managers tapping Approve on the same requisition produce two
// purchase orders from one requisition, and every sequential test still passes.
// C2 asserts segregation of duties against a *tenant admin* as well as a plain
// approver, because the rule is about the row and not about the role.
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
// Response shapes. Money and quantities are decoded as float64 in the test only;
// the *server* never sees one (I8), and comparing 4500 to 4500 is not where
// precision goes.
// --------------------------------------------------------------------------

type supplier struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ContactEmail *string `json:"contactEmail"`
	LeadTimeDays int     `json:"leadTimeDays"`
	PaymentTerms string  `json:"paymentTerms"`
	IsActive     bool    `json:"isActive"`
	DeletedAt    *string `json:"deletedAt"`
	OpenOrders   int     `json:"openOrders"`
}

type requisition struct {
	ID              string  `json:"id"`
	PRNumber        string  `json:"prNumber"`
	Status          string  `json:"status"`
	WarehouseID     string  `json:"warehouseId"`
	SupplierID      *string `json:"supplierId"`
	SupplierName    *string `json:"supplierName"`
	Notes           *string `json:"notes"`
	RequestedByID   string  `json:"requestedById"`
	RequestedByName string  `json:"requestedByName"`
	SubmittedAt     *string `json:"submittedAt"`
	DecidedByID     *string `json:"decidedById"`
	DecidedAt       *string `json:"decidedAt"`
	RejectReason    *string `json:"rejectReason"`
	CancelledByID   *string `json:"cancelledById"`
	CancelledAt     *string `json:"cancelledAt"`
	CancelReason    *string `json:"cancelReason"`
	LineCount       int     `json:"lineCount"`
	EstimatedTotal  float64 `json:"estimatedTotal"`

	PurchaseOrderID     *string `json:"purchaseOrderId"`
	PurchaseOrderNumber *string `json:"purchaseOrderNumber"`

	Lines []struct {
		ID          string  `json:"id"`
		LineNo      int     `json:"lineNo"`
		ProductID   string  `json:"productId"`
		SKU         string  `json:"sku"`
		ProductName string  `json:"productName"`
		Qty         float64 `json:"qty"`
		EstUnitCost float64 `json:"estUnitCost"`
		LineTotal   float64 `json:"lineTotal"`
	} `json:"lines"`
}

type purchaseOrder struct {
	ID                string  `json:"id"`
	PONumber          string  `json:"poNumber"`
	Status            string  `json:"status"`
	SupplierID        string  `json:"supplierId"`
	WarehouseID       string  `json:"warehouseId"`
	RequisitionID     *string `json:"requisitionId"`
	RequisitionNumber *string `json:"requisitionNumber"`
	TotalAmount       float64 `json:"totalAmount"`
	ExpectedAt        *string `json:"expectedAt"`
	CreatedByID       string  `json:"createdById"`
	CancelledByID     *string `json:"cancelledById"`
	CancelledAt       *string `json:"cancelledAt"`
	CancelReason      *string `json:"cancelReason"`
	LineCount         int     `json:"lineCount"`
	QtyOrdered        float64 `json:"qtyOrdered"`
	QtyReceived       float64 `json:"qtyReceived"`
	QtyOutstanding    float64 `json:"qtyOutstanding"`

	Lines []struct {
		ID             string  `json:"id"`
		LineNo         int     `json:"lineNo"`
		ProductID      string  `json:"productId"`
		SKU            string  `json:"sku"`
		ProductName    string  `json:"productName"`
		ProductDeleted bool    `json:"productDeleted"`
		QtyOrdered     float64 `json:"qtyOrdered"`
		UnitCost       float64 `json:"unitCost"`
		LineTotal      float64 `json:"lineTotal"`
		QtyReceived    float64 `json:"qtyReceived"`
		QtyOutstanding float64 `json:"qtyOutstanding"`
	} `json:"lines"`
}

// --------------------------------------------------------------------------
// Actors and helpers.
// --------------------------------------------------------------------------

// procurementUser makes a *staff* user holding one level in procurement and
// nothing else. Staff rather than a tenant admin on purpose: an admin resolves to
// `admin` everywhere implicitly, which would make these tests unable to tell a
// procurement permission from a workspace one.
func procurementUser(t *testing.T, f *testsupport.TenantFixture, level string) string {
	t.Helper()
	return f.NewUser(t, map[string]string{"procurement": level}).FirebaseUID
}

// line is one requisition line as the API takes it. Quantities and costs are
// strings, so nothing passes through a float on the way in either.
func line(productID uuid.UUID, qty, estUnitCost string) map[string]any {
	return map[string]any{
		"productId":   productID.String(),
		"qty":         qty,
		"estUnitCost": estUnitCost,
	}
}

func createRequisition(t *testing.T, h *testsupport.Harness, token string, body map[string]any) requisition {
	t.Helper()
	resp := h.Post(t, "/api/procurement/requisitions", token, body)
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	return testsupport.Decode[requisition](t, resp)
}

// draftWithLines is the common arrange step: a requisition with one line, raised
// by `token`, ready to submit.
func draftWithLines(t *testing.T, h *testsupport.Harness, token string, f *testsupport.TenantFixture, lines ...map[string]any) requisition {
	t.Helper()
	if len(lines) == 0 {
		lines = []map[string]any{line(f.ProductID, "10", "100.00")}
	}
	return createRequisition(t, h, token, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"supplierId":  f.SupplierID.String(),
		"lines":       lines,
	})
}

func submitRequisition(t *testing.T, h *testsupport.Harness, token, id string) requisition {
	t.Helper()
	resp := h.Post(t, "/api/procurement/requisitions/"+id+"/submit", token, nil)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	return testsupport.Decode[requisition](t, resp)
}

// submittedRequisition is a requisition raised by `author` and waiting for a
// decision — the state every approval test starts from.
func submittedRequisition(t *testing.T, h *testsupport.Harness, f *testsupport.TenantFixture, author string, lines ...map[string]any) requisition {
	t.Helper()
	draft := draftWithLines(t, h, author, f, lines...)
	return submitRequisition(t, h, author, draft.ID)
}

func getRequisition(t *testing.T, h *testsupport.Harness, token, id string) requisition {
	t.Helper()
	resp := h.Get(t, "/api/procurement/requisitions/"+id, token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	return testsupport.Decode[requisition](t, resp)
}

func getPurchaseOrder(t *testing.T, h *testsupport.Harness, token, id string) purchaseOrder {
	t.Helper()
	resp := h.Get(t, "/api/procurement/purchase-orders/"+id, token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	return testsupport.Decode[purchaseOrder](t, resp)
}

// --------------------------------------------------------------------------
// Group C — procurement rules.
// --------------------------------------------------------------------------

// C1 — submitting a requisition with zero lines is refused.
//
// A requisition with nothing on it asks for nothing, and approving it would
// generate a purchase order with no lines and a total of zero — which is a
// document that looks legitimate and orders air.
func TestC1SubmittingAnEmptyRequisitionIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Empty Ltd")
	author := procurementUser(t, f, "user")

	empty := createRequisition(t, h, author, map[string]any{
		"warehouseId": f.WarehouseID.String(),
	})
	if empty.Status != "draft" || empty.LineCount != 0 {
		t.Fatalf("created %s with %d lines, want a draft with none", empty.Status, empty.LineCount)
	}

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+empty.ID+"/submit", author, nil),
		http.StatusUnprocessableEntity, "empty_requisition")

	// Still a draft: a refused transition must not half-apply.
	if got := getRequisition(t, h, author, empty.ID); got.Status != "draft" {
		t.Errorf("status after the refusal = %s, want draft", got.Status)
	}

	// And it submits once it has a line, so the refusal is about the lines and
	// not about the requisition.
	resp := h.Patch(t, "/api/procurement/requisitions/"+empty.ID, author, map[string]any{
		"lines": []map[string]any{line(f.ProductID, "5", "10.00")},
	})
	testsupport.AssertStatus(t, resp, http.StatusOK)
	if got := submitRequisition(t, h, author, empty.ID); got.Status != "submitted" {
		t.Errorf("status = %s, want submitted", got.Status)
	}
}

// C2 — a user may not approve their own requisition (§8.2).
//
// Segregation of duties, and a record-level rule: it is asserted here against a
// plain approver AND against a tenant admin, who holds `admin` in every module
// implicitly. A rule implemented in the middleware would let the admin through,
// because no role level distinguishes "you are the person who asked for this".
func TestC2SelfApprovalIsForbidden(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Duties Ltd")

	t.Run("an approver cannot approve their own", func(t *testing.T) {
		author := procurementUser(t, f, "approver")
		pr := submittedRequisition(t, h, f, author)

		testsupport.AssertErrorCode(t,
			h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", author, nil),
			http.StatusForbidden, "self_approval_forbidden")

		if got := getRequisition(t, h, author, pr.ID); got.Status != "submitted" {
			t.Errorf("status = %s, want submitted", got.Status)
		}
		assertPurchaseOrderCount(t, f, 0)
	})

	t.Run("a tenant admin cannot either", func(t *testing.T) {
		admin := f.NewAdmin(t).FirebaseUID
		pr := submittedRequisition(t, h, f, admin)

		testsupport.AssertErrorCode(t,
			h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", admin, nil),
			http.StatusForbidden, "self_approval_forbidden")
	})

	t.Run("somebody else can", func(t *testing.T) {
		author := procurementUser(t, f, "user")
		other := procurementUser(t, f, "approver")
		pr := submittedRequisition(t, h, f, author)

		resp := h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", other, nil)
		testsupport.AssertStatus(t, resp, http.StatusOK)
		if got := testsupport.Decode[requisition](t, resp); got.Status != "approved" {
			t.Errorf("status = %s, want approved", got.Status)
		}
	})
}

// C3 — rejecting without a reason is refused.
//
// The person who raised the requisition has to know what to change, so a
// rejection with no reason is not a decision, it is a dead end. The
// `pr_reject_needs_reason` CHECK says the same thing at the database level (G13).
func TestC3RejectingWithoutAReasonIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Reasons Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	pr := submittedRequisition(t, h, f, author)

	for _, body := range []any{
		nil,
		map[string]any{},
		map[string]any{"reason": ""},
		map[string]any{"reason": "   "},
	} {
		testsupport.AssertErrorCode(t,
			h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/reject", approver, body),
			http.StatusUnprocessableEntity, "reason_required")
	}

	if got := getRequisition(t, h, author, pr.ID); got.Status != "submitted" {
		t.Errorf("status = %s, want submitted", got.Status)
	}

	resp := h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/reject", approver,
		map[string]any{"reason": "over budget this quarter"})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	rejected := testsupport.Decode[requisition](t, resp)
	if rejected.Status != "rejected" {
		t.Errorf("status = %s, want rejected", rejected.Status)
	}
	if rejected.RejectReason == nil || *rejected.RejectReason != "over budget this quarter" {
		t.Errorf("rejectReason = %v, want the reason given", rejected.RejectReason)
	}
	if rejected.DecidedByID == nil || rejected.DecidedAt == nil {
		t.Error("decidedBy and decidedAt must both be recorded — pr_decided_fields_together")
	}
}

// C4 — approving an already-approved requisition is a 409, and does not produce a
// second purchase order.
func TestC4ApprovingTwiceIsAConflict(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Twice Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	pr := submittedRequisition(t, h, f, author)

	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", approver, nil),
		http.StatusOK)

	body := testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", approver, nil),
		http.StatusConflict, "state_conflict")
	if body.Details["status"] != "approved" {
		t.Errorf("details.status = %v, want approved — the client cannot refresh "+
			"without being told where the document actually is", body.Details["status"])
	}

	// Rejecting and cancelling are refused for the same reason.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/reject", approver,
			map[string]any{"reason": "changed my mind"}),
		http.StatusConflict, "state_conflict")

	assertPurchaseOrderCount(t, f, 1)
}

// C5 — editing a submitted requisition is a 409.
//
// A submitted requisition is in somebody else's hands. Letting its author quietly
// double a quantity while an approver is reading it is how an approval means
// something other than what was approved.
func TestC5EditingASubmittedRequisitionIsAConflict(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Frozen Ltd")
	author := procurementUser(t, f, "user")
	pr := submittedRequisition(t, h, f, author)

	body := testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/requisitions/"+pr.ID, author,
			map[string]any{"notes": "actually, make it 20"}),
		http.StatusConflict, "state_conflict")
	if body.Details["status"] != "submitted" {
		t.Errorf("details.status = %v, want submitted", body.Details["status"])
	}

	// Lines too — the field most worth changing behind an approver's back.
	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/requisitions/"+pr.ID, author,
			map[string]any{"lines": []map[string]any{line(f.ProductID, "999", "1.00")}}),
		http.StatusConflict, "state_conflict")

	after := getRequisition(t, h, author, pr.ID)
	if after.Lines[0].Qty != 10 {
		t.Errorf("qty = %v, want 10 — the refused edit was applied anyway", after.Lines[0].Qty)
	}
}

// C6 — approval generates a purchase order whose lines and total match the
// requisition.
//
// Three lines, two of them with different unit costs, because a total computed
// from one line is a total that could be anything. The expected value is written
// out in the test rather than derived, so a change in how the sum is computed has
// to be justified against a number a person chose.
func TestC6ApprovalGeneratesAMatchingPurchaseOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Ordering Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	third := f.NewProduct(t, "Product C", "0")

	pr := submittedRequisition(t, h, f, author,
		line(f.ProductID, "10", "1500.00"),    // 15 000.00
		line(f.ProductAltID, "2.5", "400.00"), //  1 000.00
		line(third, "3", "0.50"),              //      1.50
	)
	if pr.EstimatedTotal != 16001.50 {
		t.Fatalf("requisition estimatedTotal = %v, want 16001.50", pr.EstimatedTotal)
	}

	resp := h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", approver, nil)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	approved := testsupport.Decode[requisition](t, resp)

	if approved.Status != "approved" {
		t.Fatalf("status = %s, want approved", approved.Status)
	}
	if approved.PurchaseOrderID == nil || approved.PurchaseOrderNumber == nil {
		t.Fatal("the approved requisition does not name its purchase order")
	}
	if !strings.HasPrefix(*approved.PurchaseOrderNumber, "PO-") {
		t.Errorf("po number = %s, want the PO-<YYYYMM>-<SEQ4> shape", *approved.PurchaseOrderNumber)
	}

	po := getPurchaseOrder(t, h, approver, *approved.PurchaseOrderID)
	if po.Status != "open" {
		t.Errorf("po status = %s, want open", po.Status)
	}
	if po.TotalAmount != 16001.50 {
		t.Errorf("po totalAmount = %v, want 16001.50", po.TotalAmount)
	}
	if po.SupplierID != f.SupplierID.String() || po.WarehouseID != f.WarehouseID.String() {
		t.Error("the order did not copy the requisition's supplier and warehouse")
	}
	if po.RequisitionID == nil || *po.RequisitionID != pr.ID {
		t.Error("the order does not point back at its requisition")
	}
	if po.RequisitionNumber == nil || *po.RequisitionNumber != pr.PRNumber {
		t.Errorf("requisitionNumber = %v, want %s", po.RequisitionNumber, pr.PRNumber)
	}
	if po.ExpectedAt == nil {
		t.Error("expectedAt is null; it is the supplier's lead time from today")
	}

	// Lines: same products, same quantities, est_unit_cost becomes unit_cost, and
	// line_no is preserved so "line 3" means the same line on both documents.
	if len(po.Lines) != 3 {
		t.Fatalf("po has %d lines, want 3", len(po.Lines))
	}
	for i, want := range pr.Lines {
		got := po.Lines[i]
		if got.LineNo != want.LineNo {
			t.Errorf("line %d: lineNo = %d, want %d", i, got.LineNo, want.LineNo)
		}
		if got.ProductID != want.ProductID {
			t.Errorf("line %d: productId = %s, want %s", i, got.ProductID, want.ProductID)
		}
		if got.QtyOrdered != want.Qty {
			t.Errorf("line %d: qtyOrdered = %v, want %v", i, got.QtyOrdered, want.Qty)
		}
		if got.UnitCost != want.EstUnitCost {
			t.Errorf("line %d: unitCost = %v, want the estimate %v", i, got.UnitCost, want.EstUnitCost)
		}
		// Nothing was initialised for received quantity: it is derived (I6).
		if got.QtyReceived != 0 || got.QtyOutstanding != want.Qty {
			t.Errorf("line %d: received %v / outstanding %v, want 0 / %v",
				i, got.QtyReceived, got.QtyOutstanding, want.Qty)
		}
	}

	// And the totals the list shows come from po_line_status, not from a column.
	if po.QtyOrdered != 15.5 || po.QtyReceived != 0 || po.QtyOutstanding != 15.5 {
		t.Errorf("order progress = %v ordered / %v received / %v outstanding, want 15.5 / 0 / 15.5",
			po.QtyOrdered, po.QtyReceived, po.QtyOutstanding)
	}
}

// Approval requires a supplier, and takes one in the request when the requisition
// does not name one (§8.3).
func TestApprovalRequiresASupplier(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Supplierless Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")

	draft := createRequisition(t, h, author, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"lines":       []map[string]any{line(f.ProductID, "4", "25.00")},
	})
	if draft.SupplierID != nil {
		t.Fatalf("supplierId = %v, want null", draft.SupplierID)
	}
	submitRequisition(t, h, author, draft.ID)

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/approve", approver, nil),
		http.StatusUnprocessableEntity, "supplier_required")
	assertPurchaseOrderCount(t, f, 0)

	// A supplier that does not exist is a 404, indistinguishable from another
	// tenant's — and still no order.
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/approve", approver,
			map[string]any{"supplierId": uuid.New().String()}),
		http.StatusNotFound)
	assertPurchaseOrderCount(t, f, 0)

	resp := h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/approve", approver,
		map[string]any{"supplierId": f.SupplierID.String()})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	approved := testsupport.Decode[requisition](t, resp)
	if approved.SupplierID == nil || *approved.SupplierID != f.SupplierID.String() {
		t.Errorf("supplierId = %v, want the one given at approval", approved.SupplierID)
	}
	assertPurchaseOrderCount(t, f, 1)
}

// --------------------------------------------------------------------------
// H4 — concurrent approval (§8.6.2). Listed under Group H, written here because
// this is the phase that builds the lock it is about.
// --------------------------------------------------------------------------

// Two approvers tap Approve on the same requisition at the same moment. Exactly
// one purchase order may exist afterwards, and the loser must get a clean 409
// rather than a constraint error.
//
// This is the test the `FOR UPDATE` in lockRequisition exists to pass. Without
// the lock both transactions read `status = 'submitted'`, both pass the check, and
// one requisition becomes two orders — the supplier is sent the same order twice
// and nothing in the data says which is real.
func TestH4ConcurrentApprovalsProduceOneOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Race Ltd")
	author := procurementUser(t, f, "user")
	first := procurementUser(t, f, "approver")
	second := procurementUser(t, f, "approver")
	pr := submittedRequisition(t, h, f, author)

	type outcome struct {
		status int
		code   string
		err    error
	}
	results := make(chan outcome, 2)

	for _, token := range []string{first, second} {
		// Deliberately not using the harness helpers: they call t.Fatalf and
		// t.Cleanup, neither of which is safe from a non-test goroutine.
		go func(actor string) {
			req := httptest.NewRequest(http.MethodPost,
				"/api/procurement/requisitions/"+pr.ID+"/approve", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+actor)

			resp, err := h.App.Test(req, -1)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

			var envelope testsupport.ErrorBody
			_ = json.NewDecoder(resp.Body).Decode(&envelope)
			results <- outcome{status: resp.StatusCode, code: envelope.Code}
		}(token)
	}

	var approved, refused int
	for range 2 {
		got := <-results
		switch {
		case got.err != nil:
			t.Fatalf("request failed: %v", got.err)
		case got.status == http.StatusOK:
			approved++
		case got.status == http.StatusConflict && got.code == "state_conflict":
			refused++
		default:
			t.Fatalf("unexpected outcome: %d %q", got.status, got.code)
		}
	}
	if approved != 1 || refused != 1 {
		t.Fatalf("%d approved and %d were refused; want exactly one of each",
			approved, refused)
	}

	// The invariant, read from the database rather than from the responses.
	assertPurchaseOrderCount(t, f, 1)

	var lines int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM purchase_order_lines`).Scan(&lines).Error
	})
	if lines != 1 {
		t.Errorf("%d purchase order lines, want 1", lines)
	}
}

// --------------------------------------------------------------------------
// Draft editing and cancellation.
// --------------------------------------------------------------------------

// A draft belongs to its author: nobody else may edit or submit it, whatever
// level they hold. `approver` is used as the intruder precisely because it is
// *higher* than the level the route requires — the rule is about the row.
func TestOnlyTheAuthorMayEditOrSubmitTheirDraft(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Ownership Ltd")
	author := procurementUser(t, f, "user")
	intruder := procurementUser(t, f, "approver")
	draft := draftWithLines(t, h, author, f)

	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/requisitions/"+draft.ID, intruder,
			map[string]any{"notes": "mine now"}),
		http.StatusForbidden, "forbidden")
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/submit", intruder, nil),
		http.StatusForbidden, "forbidden")

	// The author can do both.
	testsupport.AssertStatus(t,
		h.Patch(t, "/api/procurement/requisitions/"+draft.ID, author,
			map[string]any{"notes": "for the Jakarta site"}),
		http.StatusOK)
	submitRequisition(t, h, author, draft.ID)
}

// Editing a draft's lines replaces the whole set — which is the one DELETE in
// this module, and why it is scoped to drafts only.
func TestEditingADraftReplacesItsLines(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Redraft Ltd")
	author := procurementUser(t, f, "user")

	draft := draftWithLines(t, h, author, f,
		line(f.ProductID, "10", "5.00"),
		line(f.ProductAltID, "1", "5.00"))
	if draft.LineCount != 2 {
		t.Fatalf("lineCount = %d, want 2", draft.LineCount)
	}

	// The second product is dropped and the first one's quantity changed.
	resp := h.Patch(t, "/api/procurement/requisitions/"+draft.ID, author,
		map[string]any{"lines": []map[string]any{line(f.ProductID, "3", "5.00")}})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	edited := testsupport.Decode[requisition](t, resp)
	if edited.LineCount != 1 || len(edited.Lines) != 1 {
		t.Fatalf("lineCount = %d, want 1", edited.LineCount)
	}
	if edited.Lines[0].ProductID != f.ProductID.String() || edited.Lines[0].Qty != 3 {
		t.Errorf("line = %s × %v, want the first product × 3",
			edited.Lines[0].ProductID, edited.Lines[0].Qty)
	}
	if edited.EstimatedTotal != 15 {
		t.Errorf("estimatedTotal = %v, want 15", edited.EstimatedTotal)
	}
	// Renumbered from 1, so the surviving line is line 1 rather than a gap.
	if edited.Lines[0].LineNo != 1 {
		t.Errorf("lineNo = %d, want 1", edited.Lines[0].LineNo)
	}
}

// A draft may be cancelled by its creator without ever having been submitted
// (§6.9.2) — which is what migration 006 makes possible. The constraint as §6.10.3
// wrote it required `submitted_at` on every non-draft status, so this transition
// failed at the database with a check violation.
func TestADraftCanBeCancelledByItsCreator(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Withdrawn Ltd")
	author := procurementUser(t, f, "user")
	other := procurementUser(t, f, "approver")
	draft := draftWithLines(t, h, author, f)

	// Not even an approver may cancel somebody else's draft.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/cancel", other,
			map[string]any{"reason": "tidying up"}),
		http.StatusForbidden, "forbidden")

	// A reason is required: cancelling records who, when, and why.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/cancel", author, nil),
		http.StatusUnprocessableEntity, "reason_required")

	resp := h.Post(t, "/api/procurement/requisitions/"+draft.ID+"/cancel", author,
		map[string]any{"reason": "ordered elsewhere"})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	cancelled := testsupport.Decode[requisition](t, resp)
	if cancelled.Status != "cancelled" {
		t.Fatalf("status = %s, want cancelled", cancelled.Status)
	}
	if cancelled.CancelledByID == nil || *cancelled.CancelledByID == "" ||
		cancelled.CancelledAt == nil || cancelled.CancelReason == nil {
		t.Error("cancelling must record who, when, and why")
	}
	// It was never submitted, and cancelling must not invent a submission.
	if cancelled.SubmittedAt != nil {
		t.Errorf("submittedAt = %v, want null — this draft was never submitted",
			*cancelled.SubmittedAt)
	}
}

// A submitted requisition may be cancelled by its creator or by an approver
// (§6.9.2).
func TestASubmittedRequisitionCanBeCancelledByAnApprover(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Withdraw Later Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	pr := submittedRequisition(t, h, f, author)

	resp := h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/cancel", approver,
		map[string]any{"reason": "budget frozen"})
	testsupport.AssertStatus(t, resp, http.StatusOK)
	if got := testsupport.Decode[requisition](t, resp); got.Status != "cancelled" {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
}

// A user with only `user` in procurement and no relationship to the requisition
// cannot cancel someone else's submitted one either.
func TestAStrangerCannotCancelASubmittedRequisition(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Strangers Ltd")
	author := procurementUser(t, f, "user")
	stranger := procurementUser(t, f, "user")
	pr := submittedRequisition(t, h, f, author)

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/cancel", stranger,
			map[string]any{"reason": "no reason at all"}),
		http.StatusForbidden, "forbidden")
}

// --------------------------------------------------------------------------
// Group G — the Phase 5 half of the deletion policy.
// --------------------------------------------------------------------------

// G4 — deleting a supplier with an open PO returns 409 in_use; one with only
// closed POs succeeds.
//
// Two suppliers, so the refusal is a property of the *blocked* one rather than of
// deletion generally, and both PO states are exercised: `open` and
// `partially_received` block, `received` and `cancelled` do not.
func TestG4SupplierWithOpenOrdersCannotBeDeleted(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Suppliers Ltd")
	admin := procurementUser(t, f, "admin")

	blocked := f.SupplierID
	free := f.NewSupplier(t, "Unencumbered Supplies")

	openPO := f.NewPurchaseOrderFor(t, blocked)
	closedPO := f.NewPurchaseOrderFor(t, free)
	f.SetPOStatus(t, closedPO, "received")

	body := testsupport.AssertErrorCode(t,
		h.Delete(t, "/api/procurement/suppliers/"+blocked.String(), admin),
		http.StatusConflict, "in_use")
	if body.Details["openPurchaseOrders"] != float64(1) {
		t.Errorf("details.openPurchaseOrders = %v, want 1 — the refusal has to say "+
			"how much is in the way", body.Details["openPurchaseOrders"])
	}

	// A partially received order blocks too: goods are still expected.
	f.SetPOStatus(t, openPO, "partially_received")
	testsupport.AssertErrorCode(t,
		h.Delete(t, "/api/procurement/suppliers/"+blocked.String(), admin),
		http.StatusConflict, "in_use")

	// The supplier whose only order is closed deletes cleanly — a historical
	// reference does not block, which is the whole point of soft delete.
	resp := h.Delete(t, "/api/procurement/suppliers/"+free.String(), admin)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	if got := testsupport.Decode[supplier](t, resp); got.DeletedAt == nil {
		t.Error("deletedAt is null after a delete")
	}

	// And the received order still resolves its now-deleted supplier's name.
	po := getPurchaseOrder(t, h, admin, closedPO.String())
	if po.SupplierID != free.String() {
		t.Error("the order lost its supplier when the supplier was deleted")
	}

	// Once the blocking order is closed, the first supplier deletes too.
	f.SetPOStatus(t, openPO, "cancelled")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/procurement/suppliers/"+blocked.String(), admin),
		http.StatusOK)
}

// G7 — cancelling a `received` PO is a 409; cancelling an `open` one succeeds and
// records the actor and the timestamp.
//
// The refusal is not squeamishness: goods have physically arrived and the stock
// ledger has already recorded them, so there is nothing to cancel — cancellation
// is constrained by what happened in the real world (§6.9.2).
func TestG7OnlyOpenPurchaseOrdersCanBeCancelled(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Cancels Ltd")
	approver := procurementUser(t, f, "approver")

	for _, status := range []string{"received", "partially_received"} {
		po := f.NewPurchaseOrder(t)
		f.SetPOStatus(t, po, status)

		body := testsupport.AssertErrorCode(t,
			h.Post(t, "/api/procurement/purchase-orders/"+po.String()+"/cancel", approver,
				map[string]any{"reason": "too late"}),
			http.StatusConflict, "state_conflict")
		if body.Details["status"] != status {
			t.Errorf("details.status = %v, want %s", body.Details["status"], status)
		}
	}

	open := f.NewPurchaseOrder(t)
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/purchase-orders/"+open.String()+"/cancel", approver, nil),
		http.StatusUnprocessableEntity, "reason_required")

	resp := h.Post(t, "/api/procurement/purchase-orders/"+open.String()+"/cancel", approver,
		map[string]any{"reason": "supplier cannot fill it"})
	testsupport.AssertStatus(t, resp, http.StatusOK)

	cancelled := testsupport.Decode[purchaseOrder](t, resp)
	if cancelled.Status != "cancelled" {
		t.Fatalf("status = %s, want cancelled", cancelled.Status)
	}
	if cancelled.CancelledByID == nil || cancelled.CancelledAt == nil ||
		cancelled.CancelReason == nil {
		t.Error("cancelling must record who, when, and why")
	}

	// Cancelling twice is a conflict, not a second cancellation — and the
	// po_terminal_immutable trigger would refuse the UPDATE regardless.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/purchase-orders/"+open.String()+"/cancel", approver,
			map[string]any{"reason": "again"}),
		http.StatusConflict, "state_conflict")
}

// G8 — cancelling an approved requisition is a 409: the purchase order must be
// cancelled instead.
//
// The response says so, and the pr_terminal_immutable trigger enforces it
// independently — asserted here at both levels, because a handler check that
// happened to be removed would otherwise silently start working.
func TestG8AnApprovedRequisitionCannotBeCancelled(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Committed Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	pr := submittedRequisition(t, h, f, author)

	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", approver, nil),
		http.StatusOK)

	body := testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/cancel", approver,
			map[string]any{"reason": "no longer needed"}),
		http.StatusConflict, "state_conflict")
	if body.Details["status"] != "approved" {
		t.Errorf("details.status = %v, want approved", body.Details["status"])
	}

	// The database refuses it too, past the handler.
	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE purchase_requisitions SET status = 'cancelled' WHERE id = ?`, pr.ID).Error
	})
	if db.SQLState(err) != db.SQLStateCheckViolation {
		t.Errorf("raw update of an approved requisition gave %v, want a check violation "+
			"from pr_terminal_immutable", err)
	}

	// The PO is the thing to cancel, and it can be.
	approved := getRequisition(t, h, approver, pr.ID)
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/purchase-orders/"+*approved.PurchaseOrderID+"/cancel",
			approver, map[string]any{"reason": "no longer needed"}),
		http.StatusOK)
}

// G6, through the endpoint that renders it: a purchase order line whose product
// has since been deleted still resolves the product's name.
//
// Trap 3 in PROGRESS.md, and the mutation a later phase makes by reflex — adding
// `AND p.deleted_at IS NULL` to the products join, which would delete last
// quarter's orders from the screen. G6 already asserts this at the query level;
// this asserts it at the only place a user would notice.
func TestG6OrderLinesStillResolveADeletedProductsName(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "History Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	inventoryToken := f.NewUser(t, map[string]string{"inventory": "admin"}).FirebaseUID

	pr := submittedRequisition(t, h, f, author, line(f.ProductAltID, "7", "12.00"))
	resp := h.Post(t, "/api/procurement/requisitions/"+pr.ID+"/approve", approver, nil)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	approved := testsupport.Decode[requisition](t, resp)

	// The order is closed first, or the in-use check would refuse the delete —
	// which is the *other* half of the policy, and not what this is about.
	f.SetPOStatus(t, uuid.MustParse(*approved.PurchaseOrderID), "received")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductAltID.String(), inventoryToken),
		http.StatusOK)

	po := getPurchaseOrder(t, h, approver, *approved.PurchaseOrderID)
	if len(po.Lines) != 1 {
		t.Fatalf("the order has %d lines, want 1 — deleting a product deleted "+
			"its order line", len(po.Lines))
	}
	if po.Lines[0].ProductName != "Product B" {
		t.Errorf("productName = %q, want Product B", po.Lines[0].ProductName)
	}
	if !po.Lines[0].ProductDeleted {
		t.Error("productDeleted is false; the screen has no way to say why this " +
			"product is not in the catalogue")
	}

	// The requisition behind it, too.
	if got := getRequisition(t, h, approver, pr.ID); len(got.Lines) != 1 ||
		got.Lines[0].ProductName != "Product B" {
		t.Error("the requisition lost its line when the product was deleted")
	}
}

// G12 — an invalid status string is rejected by the CHECK constraint, not merely
// by the handler. `'recieved'` inserts cleanly without one, and then fails every
// status filter in the application for ever.
func TestG12InvalidStatusStringsAreRejectedByTheDatabase(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Spelling Ltd")

	po := f.NewPurchaseOrder(t)
	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE purchase_orders SET status = 'recieved' WHERE id = ?`, po).Error
	})
	if db.SQLState(err) != db.SQLStateCheckViolation {
		t.Errorf("po_status_valid accepted 'recieved': %v", err)
	}

	pr := f.NewRequisition(t, "draft")
	err = f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE purchase_requisitions SET status = 'aproved' WHERE id = ?`, pr).Error
	})
	if db.SQLState(err) != db.SQLStateCheckViolation {
		t.Errorf("pr_status_valid accepted 'aproved': %v", err)
	}

	// The API's own answer for the same typo is a 400 on the filter, so a
	// mistyped status never silently returns everything.
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/procurement/purchase-orders?status=recieved",
			procurementUser(t, f, "viewer")),
		http.StatusBadRequest, "malformed")
}

// G13 — rejecting without a reason is refused by `pr_reject_needs_reason` at the
// database level, not only by the handler (C3 is the handler's half).
//
// Two rules from one file: a handler check is a promise, a constraint is a
// guarantee. This is the one that still holds when somebody writes a second code
// path in six months.
func TestG13RejectionWithoutAReasonIsRefusedByTheDatabase(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Constraint Ltd")
	pr := f.NewRequisition(t, "submitted")

	for _, reason := range []any{nil, ""} {
		err := f.AsTenant(t, func(tx *gorm.DB) error {
			return tx.Exec(`
				UPDATE purchase_requisitions
				SET status = 'rejected', decided_by = ?, decided_at = now(), reject_reason = ?
				WHERE id = ?`, f.User.ID, reason, pr).Error
		})
		if db.SQLState(err) != db.SQLStateCheckViolation {
			t.Errorf("reject_reason %v was accepted: %v", reason, err)
		}
	}

	// With a reason it commits, so the constraint is refusing the emptiness and
	// not the transition.
	if err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE purchase_requisitions
			SET status = 'rejected', decided_by = ?, decided_at = now(), reject_reason = 'too dear'
			WHERE id = ?`, f.User.ID, pr).Error
	}); err != nil {
		t.Errorf("a rejection with a reason was refused: %v", err)
	}
}

// G14 — the same product twice on one purchase order violates
// pol_one_line_per_product. Two lines for one product make receipt quantities
// ambiguous: a receipt of 5 against "the product" has no line to belong to.
//
// The API refuses the duplicate a step earlier, when the requisition is written,
// so both halves are asserted: the constraint, and the sentence.
func TestG14OneLinePerProductOnAnOrder(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Duplicates Ltd")
	po := f.NewPurchaseOrder(t)
	f.NewPOLine(t, po, f.ProductID, 10)

	err := f.AsTenant(t, func(tx *gorm.DB) error {
		return tx.Exec(`
			INSERT INTO purchase_order_lines
			  (id, tenant_id, po_id, product_id, qty_ordered, unit_cost, line_no)
			VALUES (?, ?, ?, ?, 5, 10.00, 99)`,
			uuid.New(), f.ID, po, f.ProductID).Error
	})
	if !db.IsUniqueViolation(err) {
		t.Errorf("a second line for the same product was accepted: %v", err)
	}
	if got := db.ConstraintName(err); got != "pol_one_line_per_product" {
		t.Errorf("constraint = %q, want pol_one_line_per_product", got)
	}

	// The requisition endpoint refuses it with a sentence, so nobody reaches the
	// constraint by ordinary use.
	author := procurementUser(t, f, "user")
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions", author, map[string]any{
			"warehouseId": f.WarehouseID.String(),
			"lines": []map[string]any{
				line(f.ProductID, "1", "10.00"),
				line(f.ProductID, "2", "10.00"),
			},
		}),
		http.StatusBadRequest, "malformed")
}

// --------------------------------------------------------------------------
// Supplier master data.
// --------------------------------------------------------------------------

func TestSupplierCRUDAndRestore(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Vendors Ltd")
	admin := procurementUser(t, f, "admin")

	resp := h.Post(t, "/api/procurement/suppliers", admin, map[string]any{
		"code":         "SUP-NEW",
		"name":         "Sumber Makmur",
		"contactEmail": "sales@sumber.test",
		"leadTimeDays": 14,
	})
	testsupport.AssertStatus(t, resp, http.StatusCreated)

	created := testsupport.Decode[supplier](t, resp)
	if created.LeadTimeDays != 14 || created.PaymentTerms != "NET30" {
		t.Errorf("leadTimeDays/%d paymentTerms/%s, want 14 and the NET30 default",
			created.LeadTimeDays, created.PaymentTerms)
	}
	if created.ContactEmail == nil || *created.ContactEmail != "sales@sumber.test" {
		t.Errorf("contactEmail = %v", created.ContactEmail)
	}

	// The code is unique among live suppliers.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/suppliers", admin,
			map[string]any{"code": "SUP-NEW", "name": "Impostor"}),
		http.StatusConflict, "in_use")

	// An unparseable contact address is refused rather than stored.
	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/suppliers/"+created.ID, admin,
			map[string]any{"contactEmail": "not an address"}),
		http.StatusBadRequest, "malformed")

	// Editing, including clearing a nullable field with an empty string.
	resp = h.Patch(t, "/api/procurement/suppliers/"+created.ID, admin, map[string]any{
		"name":         "Sumber Makmur Jaya",
		"contactEmail": "",
		"paymentTerms": "NET14",
	})
	testsupport.AssertStatus(t, resp, http.StatusOK)
	edited := testsupport.Decode[supplier](t, resp)
	if edited.Name != "Sumber Makmur Jaya" || edited.PaymentTerms != "NET14" {
		t.Errorf("edit did not apply: %+v", edited)
	}
	if edited.ContactEmail != nil {
		t.Errorf("contactEmail = %v, want null after being cleared", *edited.ContactEmail)
	}

	// Deleted: gone from the list, still resolvable by id, and the code is free
	// again for a replacement (the partial unique index over live rows).
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/procurement/suppliers/"+created.ID, admin), http.StatusOK)

	if supplierListContains(t, h, admin, "/api/procurement/suppliers?pageSize=100", created.ID) {
		t.Error("a deleted supplier is still in the default list")
	}
	if !supplierListContains(t, h, admin,
		"/api/procurement/suppliers?pageSize=100&includeDeleted=true", created.ID) {
		t.Error("a deleted supplier is missing from the recycle-bin view")
	}
	testsupport.AssertStatus(t,
		h.Get(t, "/api/procurement/suppliers/"+created.ID, admin), http.StatusOK)

	replacement := testsupport.Decode[supplier](t,
		h.Post(t, "/api/procurement/suppliers", admin,
			map[string]any{"code": "SUP-NEW", "name": "Replacement"}))

	// Restoring is now refused: the code has been taken in the meantime, and the
	// replacement was legal to create.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/suppliers/"+created.ID+"/restore", admin, nil),
		http.StatusConflict, "in_use")

	// Free the code and the restore succeeds. Restoring something that is not
	// deleted is a no-op rather than an error.
	testsupport.AssertStatus(t,
		h.Patch(t, "/api/procurement/suppliers/"+replacement.ID, admin,
			map[string]any{"code": "SUP-OTHER"}), http.StatusOK)
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/suppliers/"+created.ID+"/restore", admin, nil),
		http.StatusOK)
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/suppliers/"+created.ID+"/restore", admin, nil),
		http.StatusOK)
}

// The recycle bin is module `admin` only (§9.0), and the refusal carries the
// level that would have worked — the same shape RequireModule's does, so a client
// cannot tell the two apart and does not have to.
func TestIncludeDeletedSuppliersRequiresProcurementAdmin(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Bins Ltd")
	viewer := procurementUser(t, f, "viewer")

	body := testsupport.AssertErrorCode(t,
		h.Get(t, "/api/procurement/suppliers?includeDeleted=true", viewer),
		http.StatusForbidden, "insufficient_module_role")
	if body.Details["required"] != "admin" || body.Details["module"] != "procurement" {
		t.Errorf("details = %v, want procurement/admin", body.Details)
	}

	// Without the parameter the same viewer reads the list.
	testsupport.AssertStatus(t,
		h.Get(t, "/api/procurement/suppliers", viewer), http.StatusOK)
}

// A requisition may not name a deleted product or a deleted warehouse: this is
// the picker case, not the historical-reference one. A *discontinued* product is
// allowed — ordering the last of something being wound down is ordinary (§6.9.1).
func TestRequisitionLinesRefuseDeletedMasterData(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Pickers Ltd")
	author := procurementUser(t, f, "user")
	inventoryAdminToken := f.NewUser(t, map[string]string{
		"procurement": "user", "inventory": "admin",
	}).FirebaseUID

	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductAltID.String(), inventoryAdminToken),
		http.StatusOK)

	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions", author, map[string]any{
			"warehouseId": f.WarehouseID.String(),
			"lines":       []map[string]any{line(f.ProductAltID, "1", "10.00")},
		}),
		http.StatusNotFound)

	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions", author, map[string]any{
			"warehouseId": uuid.New().String(),
			"lines":       []map[string]any{line(f.ProductID, "1", "10.00")},
		}),
		http.StatusNotFound)

	// Discontinued, not deleted: allowed.
	testsupport.AssertStatus(t,
		h.Patch(t, "/api/inventory/products/"+f.ProductID.String(), inventoryAdminToken,
			map[string]any{"isActive": false}),
		http.StatusOK)
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions", author, map[string]any{
			"warehouseId": f.WarehouseID.String(),
			"lines":       []map[string]any{line(f.ProductID, "1", "10.00")},
		}),
		http.StatusCreated)
}

// A line's estimated unit cost defaults to the product's standard cost.
//
// The column defaults to 0 and nothing would complain — but that zero is copied
// to the PO line as `unit_cost`, and from there to the goods receipt's journal
// entry, which would post Dr 0 / Cr 0. A balanced entry for nothing is worse than
// an error, because it looks fine.
func TestLineCostDefaultsToTheProductsStandardCost(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Costing Ltd")
	author := procurementUser(t, f, "user")

	pr := createRequisition(t, h, author, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"lines": []map[string]any{
			{"productId": f.ProductID.String(), "qty": "3"},
			{"productId": f.ProductAltID.String(), "qty": "1", "estUnitCost": "99.00"},
		},
	})
	// The fixture's products cost 10.00 and 25.00.
	if pr.Lines[0].EstUnitCost != 10 {
		t.Errorf("estUnitCost = %v, want the product's standard cost 10", pr.Lines[0].EstUnitCost)
	}
	if pr.Lines[1].EstUnitCost != 99 {
		t.Errorf("an explicit estUnitCost was overridden: %v", pr.Lines[1].EstUnitCost)
	}
	if pr.EstimatedTotal != 129 {
		t.Errorf("estimatedTotal = %v, want 129 (3 × 10 + 1 × 99)", pr.EstimatedTotal)
	}
}

// Quantities are refused when they are not positive, by the handler and by the
// `qty > 0` CHECK behind it.
func TestRequisitionLineQuantitiesMustBePositive(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Quantities Ltd")
	author := procurementUser(t, f, "user")

	for _, qty := range []string{"0", "-5", "0.0000"} {
		testsupport.AssertErrorCode(t,
			h.Post(t, "/api/procurement/requisitions", author, map[string]any{
				"warehouseId": f.WarehouseID.String(),
				"lines":       []map[string]any{line(f.ProductID, qty, "10.00")},
			}),
			http.StatusBadRequest, "malformed")
	}

	err := f.AsTenant(t, func(tx *gorm.DB) error {
		pr := f.NewRequisition(t, "draft")
		return tx.Exec(`
			INSERT INTO purchase_requisition_lines
			  (id, tenant_id, requisition_id, product_id, qty, est_unit_cost, line_no)
			VALUES (?, ?, ?, ?, 0, 10.00, 1)`,
			uuid.New(), f.ID, pr, f.ProductID).Error
	})
	if db.SQLState(err) != db.SQLStateCheckViolation {
		t.Errorf("a zero-quantity line was accepted by the database: %v", err)
	}
}

// --------------------------------------------------------------------------
// The module gate, on the routes that ship.
// --------------------------------------------------------------------------

// Every §9.4 route refuses a caller below its level, and the refusal names the
// level that would have worked.
//
// Group B already asserts RequireModule's behaviour against the probe routes in
// testsupport. This asserts the *levels in the route table*: reads at `viewer`,
// raising and editing at `user`, deciding at `approver`, master data at `admin`. A
// probe route cannot catch a real route registered at the wrong level.
func TestProcurementRoutesCarryTheLevelsFromTheSpec(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Levels Ltd")

	viewer := procurementUser(t, f, "viewer")
	user := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")
	// A level in another module entirely, to check the gate is per module rather
	// than per "has any level at all".
	elsewhere := f.NewUser(t, map[string]string{"inventory": "admin"}).FirebaseUID

	pr := draftWithLines(t, h, user, f)
	po := f.NewPurchaseOrder(t)

	reads := []string{
		"/api/procurement/suppliers",
		"/api/procurement/suppliers/" + f.SupplierID.String(),
		"/api/procurement/requisitions",
		"/api/procurement/requisitions/" + pr.ID,
		"/api/procurement/purchase-orders",
		"/api/procurement/purchase-orders/" + po.String(),
	}
	for _, path := range reads {
		testsupport.AssertStatus(t, h.Get(t, path, viewer), http.StatusOK)
		testsupport.AssertErrorCode(t, h.Get(t, path, elsewhere),
			http.StatusForbidden, "insufficient_module_role")
	}

	// Raising and editing require `user`: a viewer cannot.
	atUser := []struct{ method, path string }{
		{http.MethodPost, "/api/procurement/requisitions"},
		{http.MethodPatch, "/api/procurement/requisitions/" + pr.ID},
		{http.MethodPost, "/api/procurement/requisitions/" + pr.ID + "/submit"},
		{http.MethodPost, "/api/procurement/requisitions/" + pr.ID + "/cancel"},
	}
	for _, route := range atUser {
		body := testsupport.AssertErrorCode(t,
			h.Request(t, route.method, route.path, viewer, map[string]any{}),
			http.StatusForbidden, "insufficient_module_role")
		if body.Details["required"] != "user" {
			t.Errorf("%s %s: required = %v, want user",
				route.method, route.path, body.Details["required"])
		}
	}

	// Deciding requires `approver`: a `user` cannot, even on their own document.
	atApprover := []string{
		"/api/procurement/requisitions/" + pr.ID + "/approve",
		"/api/procurement/requisitions/" + pr.ID + "/reject",
		"/api/procurement/purchase-orders/" + po.String() + "/cancel",
	}
	for _, path := range atApprover {
		body := testsupport.AssertErrorCode(t,
			h.Post(t, path, user, map[string]any{"reason": "x"}),
			http.StatusForbidden, "insufficient_module_role")
		if body.Details["required"] != "approver" {
			t.Errorf("%s: required = %v, want approver", path, body.Details["required"])
		}
	}

	// Supplier master data requires `admin`: an approver cannot.
	atAdmin := []struct{ method, path string }{
		{http.MethodPost, "/api/procurement/suppliers"},
		{http.MethodPatch, "/api/procurement/suppliers/" + f.SupplierID.String()},
		{http.MethodDelete, "/api/procurement/suppliers/" + f.SupplierID.String()},
		{http.MethodPost, "/api/procurement/suppliers/" + f.SupplierID.String() + "/restore"},
	}
	for _, route := range atAdmin {
		body := testsupport.AssertErrorCode(t,
			h.Request(t, route.method, route.path, approver, map[string]any{}),
			http.StatusForbidden, "insufficient_module_role")
		if body.Details["required"] != "admin" {
			t.Errorf("%s %s: required = %v, want admin",
				route.method, route.path, body.Details["required"])
		}
	}
}

// There is no DELETE on either document, and adding one "for symmetry" is called
// out by name in §9.6.1. The route not existing is the enforcement.
func TestNoDeleteRouteForRequisitionsOrOrders(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "No Deletes Ltd")
	admin := f.NewAdmin(t).FirebaseUID
	pr := f.NewRequisition(t, "draft")
	po := f.NewPurchaseOrder(t)

	for _, path := range []string{
		"/api/procurement/requisitions/" + pr.String(),
		"/api/procurement/purchase-orders/" + po.String(),
	} {
		if got := h.Delete(t, path, admin); got.StatusCode != http.StatusMethodNotAllowed &&
			got.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE %s = %d, want 404 or 405 — the route must not exist",
				path, got.StatusCode)
		}
	}
}

// A tenant without the procurement entitlement gets module_not_enabled — the
// superadmin's problem, not the tenant admin's (§5.7).
func TestProcurementRoutesRefuseAnUnentitledTenant(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "No Procurement Ltd")
	token := procurementUser(t, f, "admin")
	f.DisableModule(t, "procurement")

	testsupport.AssertErrorCode(t, h.Get(t, "/api/procurement/requisitions", token),
		http.StatusForbidden, "module_not_enabled")
}

// --------------------------------------------------------------------------
// Isolation and the list contract.
// --------------------------------------------------------------------------

// Isolation, asserted from two tenants. A single-tenant test cannot detect an
// isolation failure (§12.2), and every procurement table is RLS-forced — so this
// is really a test that the handlers stayed on the transaction TenantTx opened.
func TestProcurementIsNotVisibleAcrossTenants(t *testing.T) {
	h := testsupport.NewHarness(t)
	a := h.DB.NewTenant(t, "Tenant A")
	b := h.DB.NewTenant(t, "Tenant B")
	tokenA := procurementUser(t, a, "admin")
	tokenB := procurementUser(t, b, "admin")

	theirs := submittedRequisition(t, h, b, procurementUser(t, b, "user"))
	theirPO := b.NewPurchaseOrder(t)

	// Lists.
	page := testsupport.Decode[list[requisition]](t,
		h.Get(t, "/api/procurement/requisitions?pageSize=100", tokenA))
	if len(page.Data) != 0 {
		t.Errorf("tenant A sees %d of tenant B's requisitions", len(page.Data))
	}
	orders := testsupport.Decode[list[purchaseOrder]](t,
		h.Get(t, "/api/procurement/purchase-orders?pageSize=100", tokenA))
	if len(orders.Data) != 0 {
		t.Errorf("tenant A sees %d of tenant B's orders", len(orders.Data))
	}
	if supplierListContains(t, h, tokenA, "/api/procurement/suppliers?pageSize=100",
		b.SupplierID.String()) {
		t.Error("tenant A can see tenant B's supplier")
	}

	// By ID, which is the path RLS has to hold on rather than a WHERE clause.
	for _, path := range []string{
		"/api/procurement/requisitions/" + theirs.ID,
		"/api/procurement/purchase-orders/" + theirPO.String(),
		"/api/procurement/suppliers/" + b.SupplierID.String(),
	} {
		testsupport.AssertStatus(t, h.Get(t, path, tokenA), http.StatusNotFound)
	}

	// And writes cannot reach across either — the same 404 an unknown ID gets,
	// so nothing distinguishes "exists elsewhere" from "never existed".
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions/"+theirs.ID+"/approve", tokenA, nil),
		http.StatusNotFound)
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/purchase-orders/"+theirPO.String()+"/cancel", tokenA,
			map[string]any{"reason": "not mine"}),
		http.StatusNotFound)
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/procurement/suppliers/"+b.SupplierID.String(), tokenA),
		http.StatusNotFound)

	// B's own data is intact, which is what makes the emptiness above a filtering
	// result rather than a broken fixture.
	if got := getRequisition(t, h, tokenB, theirs.ID); got.Status != "submitted" {
		t.Errorf("tenant B's own requisition reads %s, want submitted", got.Status)
	}
	mine := testsupport.Decode[list[purchaseOrder]](t,
		h.Get(t, "/api/procurement/purchase-orders?pageSize=100", tokenB))
	if len(mine.Data) != 1 {
		t.Errorf("tenant B sees %d of their own orders, want 1", len(mine.Data))
	}
}

// The §9.0 list contract on the procurement lists: an unknown sort field is an
// error rather than a silent fallback, the status filter filters, and sorting is
// over the whole result set rather than per page.
func TestProcurementListsFollowTheListContract(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Paging Ltd")
	author := procurementUser(t, f, "user")
	approver := procurementUser(t, f, "approver")

	var drafts []requisition
	for i := range 5 {
		pr := draftWithLines(t, h, author, f, line(f.ProductID, fmt.Sprint(i+1), "10.00"))
		drafts = append(drafts, pr)
	}
	// Two of them move on, so the status filter has something to exclude.
	submitRequisition(t, h, author, drafts[0].ID)
	submitRequisition(t, h, author, drafts[1].ID)
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions/"+drafts[1].ID+"/approve", approver, nil),
		http.StatusOK)

	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/procurement/requisitions?sort=nonsense", author),
		http.StatusBadRequest, "malformed")
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/procurement/requisitions?status=pending", author),
		http.StatusBadRequest, "malformed")

	byStatus := map[string]int{"draft": 3, "submitted": 1, "approved": 1, "rejected": 0}
	for status, want := range byStatus {
		page := testsupport.Decode[list[requisition]](t,
			h.Get(t, "/api/procurement/requisitions?pageSize=100&status="+status, author))
		if page.TotalItems != want {
			t.Errorf("status=%s: totalItems = %d, want %d", status, page.TotalItems, want)
		}
		for _, row := range page.Data {
			if row.Status != status {
				t.Errorf("status=%s returned a %s requisition", status, row.Status)
			}
		}
	}

	// Page 2 of a descending sort holds items from the whole set, not a sorted
	// slice of one page.
	page := testsupport.Decode[list[requisition]](t,
		h.Get(t, "/api/procurement/requisitions?sort=-prNumber&pageSize=2&page=2", author))
	if page.TotalItems != 5 {
		t.Fatalf("totalItems = %d, want 5", page.TotalItems)
	}
	all := testsupport.Decode[list[requisition]](t,
		h.Get(t, "/api/procurement/requisitions?sort=-prNumber&pageSize=100", author))
	if all.Data[2].PRNumber != page.Data[0].PRNumber {
		t.Errorf("page 2 starts at %s, but the whole set's third item is %s — the "+
			"sort is being applied per page", page.Data[0].PRNumber, all.Data[2].PRNumber)
	}

	// The supplier filter, which the requisition list gained when the status
	// chips became a dropdown and a second dropdown fitted beside them. Last,
	// because it adds a sixth requisition and the counts above are over five.
	// The five so far all name the fixture's supplier; this one names another.
	other := f.NewSupplier(t, "Other Supplies")
	elsewhere := createRequisition(t, h, author, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"supplierId":  other.String(),
		"lines":       []map[string]any{line(f.ProductID, "4", "10.00")},
	})

	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/procurement/requisitions?supplierId=not-a-uuid", author),
		http.StatusBadRequest, "malformed")

	forOther := testsupport.Decode[list[requisition]](t,
		h.Get(t, "/api/procurement/requisitions?pageSize=100&supplierId="+other.String(), author))
	if forOther.TotalItems != 1 || forOther.Data[0].ID != elsewhere.ID {
		t.Errorf("supplierId returned %d rows, want just %s",
			forOther.TotalItems, elsewhere.PRNumber)
	}

	// Status and supplier narrow together rather than one replacing the other.
	both := testsupport.Decode[list[requisition]](t,
		h.Get(t, "/api/procurement/requisitions?pageSize=100&status=draft&supplierId="+
			f.SupplierID.String(), author))
	if both.TotalItems != 3 {
		t.Errorf("status+supplier: totalItems = %d, want 3", both.TotalItems)
	}
}

// Requisition numbers are allocated per tenant and per month, in the
// PR-<YYYYMM>-<SEQ4> shape (§8.1). Group E covers the mechanism; this is the
// claim as a caller sees it.
func TestRequisitionNumbersRunInSequencePerTenant(t *testing.T) {
	h := testsupport.NewHarness(t)
	a := h.DB.NewTenant(t, "Numbering A")
	b := h.DB.NewTenant(t, "Numbering B")
	authorA := procurementUser(t, a, "user")
	authorB := procurementUser(t, b, "user")

	first := draftWithLines(t, h, authorA, a).PRNumber
	second := draftWithLines(t, h, authorA, a).PRNumber
	other := draftWithLines(t, h, authorB, b).PRNumber

	if !strings.HasSuffix(first, "-0001") || !strings.HasSuffix(second, "-0002") {
		t.Errorf("tenant A's numbers = %s then %s, want -0001 then -0002", first, second)
	}
	if !strings.HasSuffix(other, "-0001") {
		t.Errorf("tenant B's first number = %s, want -0001 — the counter is shared", other)
	}
	// Same month, so the period matches; the counters do not.
	if first[:10] != other[:10] {
		t.Errorf("periods differ: %s and %s", first, other)
	}
}

// A refused requisition does not consume a document number (E4, through HTTP).
// TenantTx rolls back on any status ≥ 400, so the number allocated before the
// refusal goes back with it.
func TestARefusedRequisitionDoesNotConsumeANumber(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Frugal Ltd")
	author := procurementUser(t, f, "user")

	// Refused after the warehouse check but before any insert: a line naming a
	// product that does not exist.
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions", author, map[string]any{
			"warehouseId": f.WarehouseID.String(),
			"lines":       []map[string]any{line(uuid.New(), "1", "10.00")},
		}),
		http.StatusNotFound)

	if got := draftWithLines(t, h, author, f).PRNumber; !strings.HasSuffix(got, "-0001") {
		t.Errorf("first successful number = %s, want -0001 — the refused request "+
			"consumed one", got)
	}
}

// --------------------------------------------------------------------------
// Helpers.
// --------------------------------------------------------------------------

func assertPurchaseOrderCount(t *testing.T, f *testsupport.TenantFixture, want int64) {
	t.Helper()
	var got int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM purchase_orders`).Scan(&got).Error
	})
	if got != want {
		t.Errorf("purchase_orders = %d, want %d", got, want)
	}
}

func supplierListContains(t *testing.T, h *testsupport.Harness, token, path, id string) bool {
	t.Helper()
	page := testsupport.Decode[list[supplier]](t, h.Get(t, path, token))
	for _, row := range page.Data {
		if row.ID == id {
			return true
		}
	}
	return false
}
