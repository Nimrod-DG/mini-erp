// Group L — the dashboard (§9.7, §10.2).
//
// The dashboard is the only endpoint that spans modules, so the interesting
// claim is not "the numbers are right" but "each widget is present exactly when
// the caller may read the module behind it, and absent otherwise". L1 is that
// claim; the rest check that a widget's number means what its label says.
//
// L4 is the one worth reading. It asserts that the low-stock widget's count and
// GET /inventory/stock/low's total are the same number — which is a claim about
// two queries not drifting, and the reason they share `lowStockFrom` rather than
// each carrying a copy of "below reorder point".
package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// --------------------------------------------------------------------------
// Response shapes. Money and quantities decode as float64 in the test only —
// the server never sees one (I8), and comparing 4500 to 4500 is not where
// precision goes.
// --------------------------------------------------------------------------

type dashboard struct {
	// Pointers, because the whole point of the endpoint is that a widget the
	// caller cannot read is *absent*. A non-pointer field would decode a missing
	// widget into a zero value and make L1 unable to fail.
	OpenOrders *struct {
		Count      int     `json:"count"`
		TotalValue float64 `json:"totalValue"`
	} `json:"openOrders"`

	PendingApprovals *struct {
		Count      int           `json:"count"`
		CanApprove bool          `json:"canApprove"`
		Queue      []requisition `json:"queue"`
	} `json:"pendingApprovals"`

	LowStock *struct {
		Count    int        `json:"count"`
		Products []lowStock `json:"products"`
	} `json:"lowStock"`

	RecentActivity *struct {
		Entries []ledgerEntry `json:"entries"`
	} `json:"recentActivity"`
}

func getDashboard(t *testing.T, h *testsupport.Harness, token string) dashboard {
	t.Helper()
	resp := h.Get(t, "/api/dashboard/summary", token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	return testsupport.Decode[dashboard](t, resp)
}

// --------------------------------------------------------------------------
// L1 — which widgets a caller gets.
// --------------------------------------------------------------------------

// L1 — the four widgets are filtered by module, individually.
//
// The four cases below are the whole contract. A procurement-only user must get
// the two procurement widgets and *not* an empty stock panel: `{"count": 0}`
// reads as "nothing is low", and telling somebody who cannot see Inventory that
// nothing is low is a lie the nav does not tell.
func TestDashboardShowsOnlyWidgetsTheCallerCanRead(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Widget Works")

	both := f.NewUser(t, map[string]string{
		"procurement": "viewer", "inventory": "viewer"}).FirebaseUID
	procurementOnly := f.NewUser(t, map[string]string{"procurement": "viewer"}).FirebaseUID
	inventoryOnly := f.NewUser(t, map[string]string{"inventory": "viewer"}).FirebaseUID
	// Entitled to nothing at all. Not a superadmin — an ordinary employee whose
	// administrator has given them no module. The dashboard is the first screen
	// they land on and it must render, empty, rather than 403 or 500.
	neither := f.NewUser(t, map[string]string{}).FirebaseUID

	cases := []struct {
		name                       string
		token                      string
		wantOrders, wantApprovals  bool
		wantLowStock, wantActivity bool
	}{
		{"both modules", both, true, true, true, true},
		{"procurement only", procurementOnly, true, true, false, false},
		{"inventory only", inventoryOnly, false, false, true, true},
		{"no modules", neither, false, false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getDashboard(t, h, tc.token)

			if (got.OpenOrders != nil) != tc.wantOrders {
				t.Errorf("openOrders present = %t, want %t", got.OpenOrders != nil, tc.wantOrders)
			}
			if (got.PendingApprovals != nil) != tc.wantApprovals {
				t.Errorf("pendingApprovals present = %t, want %t",
					got.PendingApprovals != nil, tc.wantApprovals)
			}
			if (got.LowStock != nil) != tc.wantLowStock {
				t.Errorf("lowStock present = %t, want %t", got.LowStock != nil, tc.wantLowStock)
			}
			if (got.RecentActivity != nil) != tc.wantActivity {
				t.Errorf("recentActivity present = %t, want %t",
					got.RecentActivity != nil, tc.wantActivity)
			}
		})
	}
}

// A module the *tenant* is not entitled to hides its widgets too, even from a
// tenant admin — who resolves to `admin` in every entitled module and to nothing
// at all in the others. This is Agus from the seed (§15): the admin shortcut
// sits below the entitlement ceiling, and the dashboard has to say so.
func TestDashboardHidesWidgetsOfAnUnentitledModule(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "No Stock Ltd")
	admin := f.NewAdmin(t).FirebaseUID

	before := getDashboard(t, h, admin)
	if before.LowStock == nil {
		t.Fatal("a tenant admin of an entitled workspace should see the stock widgets")
	}

	f.DisableModule(t, "inventory")

	after := getDashboard(t, h, admin)
	if after.LowStock != nil || after.RecentActivity != nil {
		t.Error("revoking the inventory entitlement must hide both inventory widgets")
	}
	if after.OpenOrders == nil {
		t.Error("revoking inventory must not affect the procurement widgets")
	}
}

// A superadmin has no tenant, so TenantTx opens no transaction for them. The
// dashboard must answer with an empty summary rather than a 500 — they reach it
// because it carries no RequireModule, and §5.5 says they read no tenant
// business data anyway.
func TestDashboardGivesASuperadminNothingRatherThanAnError(t *testing.T) {
	h := testsupport.NewHarness(t)
	h.DB.NewTenant(t, "Somebody Else's Data")
	super := h.DB.NewSuperadmin(t).FirebaseUID

	got := getDashboard(t, h, super)
	if got.OpenOrders != nil || got.PendingApprovals != nil ||
		got.LowStock != nil || got.RecentActivity != nil {
		t.Errorf("a superadmin's dashboard should be empty, got %+v", got)
	}
}

// --------------------------------------------------------------------------
// L2 — the approval queue.
// --------------------------------------------------------------------------

// L2 — the count is everybody's, the queue is the approver's.
//
// The count is deliberately the same for both: "3 requisitions are waiting" is
// true whoever is asking, and a viewer who cannot see the queue can still see
// that the backlog exists. Only the inline decision is gated.
func TestApprovalQueueIsPopulatedOnlyForAnApprover(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Queue Co")

	for i := 0; i < 3; i++ {
		requisitionID := f.NewRequisition(t, "submitted")
		f.NewRequisitionLine(t, requisitionID, f.ProductID, "2", "10.00", i+1)
	}
	// A draft and an approved one, neither of which is waiting for anybody.
	f.NewRequisition(t, "draft")
	f.NewRequisition(t, "approved")

	viewer := f.NewUser(t, map[string]string{"procurement": "viewer"}).FirebaseUID
	approver := f.NewUser(t, map[string]string{"procurement": "approver"}).FirebaseUID

	asViewer := getDashboard(t, h, viewer).PendingApprovals
	if asViewer.Count != 3 {
		t.Errorf("viewer count = %d, want 3", asViewer.Count)
	}
	if asViewer.CanApprove {
		t.Error("a viewer must not be told they can approve")
	}
	if len(asViewer.Queue) != 0 {
		t.Errorf("viewer queue = %d rows, want 0", len(asViewer.Queue))
	}

	asApprover := getDashboard(t, h, approver).PendingApprovals
	if asApprover.Count != 3 {
		t.Errorf("approver count = %d, want 3", asApprover.Count)
	}
	if !asApprover.CanApprove {
		t.Error("an approver must be told they can approve")
	}
	if len(asApprover.Queue) != 3 {
		t.Fatalf("approver queue = %d rows, want 3", len(asApprover.Queue))
	}
	for _, row := range asApprover.Queue {
		if row.Status != "submitted" {
			t.Errorf("queue holds a %s requisition", row.Status)
		}
	}
}

// The queue keeps the caller's own submissions, and the count includes them.
//
// Filtering them out would be the tempting thing — the caller cannot approve
// their own (C2) — and it is wrong: the count above the list would then disagree
// with the list itself. The row carries `requestedById` so the screen can
// disable its own buttons and say why, and the server refuses regardless (I12).
func TestApprovalQueueKeepsTheApproversOwnRequisitions(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Self Serve Ltd")

	approver := f.NewUser(t, map[string]string{"procurement": "approver"})

	// Raised by the approver themselves, through the real endpoint so
	// `requested_by` is genuinely theirs.
	own := createRequisition(t, h, approver.FirebaseUID, map[string]any{
		"warehouseId": f.WarehouseID,
		"supplierId":  f.SupplierID,
		"lines": []map[string]any{
			{"productId": f.ProductID, "qty": "5", "estUnitCost": "10.00"},
		},
	})
	testsupport.AssertStatus(t,
		h.Post(t, "/api/procurement/requisitions/"+own.ID+"/submit", approver.FirebaseUID, nil),
		http.StatusOK)

	widget := getDashboard(t, h, approver.FirebaseUID).PendingApprovals
	if widget.Count != 1 || len(widget.Queue) != 1 {
		t.Fatalf("count = %d, queue = %d rows, want 1 and 1", widget.Count, len(widget.Queue))
	}
	if widget.Queue[0].RequestedByID != approver.ID.String() {
		t.Errorf("requestedById = %s, want the approver's own id %s",
			widget.Queue[0].RequestedByID, approver.ID)
	}

	// And the server still refuses it, which is what makes the row's presence
	// cosmetic rather than a hole.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/requisitions/"+own.ID+"/approve", approver.FirebaseUID, nil),
		http.StatusForbidden, "self_approval_forbidden")
}

// --------------------------------------------------------------------------
// L3 — open purchase orders.
// --------------------------------------------------------------------------

// L3 — "open" means outstanding: `open` and `partially_received` both count.
//
// An order half of which has arrived is still an order somebody is waiting on. A
// widget that dropped it the moment the first box landed would count down to
// zero while goods were still in transit, which is the opposite of what the
// number is for.
func TestOpenOrdersCountsPartiallyReceivedOrdersToo(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Orders Ltd")
	viewer := f.NewUser(t, map[string]string{"procurement": "viewer"}).FirebaseUID

	// Three lines of 10 at 10.00 each: 100.00 per order.
	for _, status := range []string{"open", "partially_received", "received", "cancelled"} {
		poID := f.NewPurchaseOrder(t)
		f.NewPOLine(t, poID, f.ProductID, 10)
		if status != "open" {
			f.SetPOStatus(t, poID, status)
		}
	}

	widget := getDashboard(t, h, viewer).OpenOrders
	if widget.Count != 2 {
		t.Errorf("count = %d, want 2 (open + partially_received)", widget.Count)
	}
	if widget.TotalValue != 200 {
		t.Errorf("totalValue = %v, want 200 (2 orders × 10 × 10.00)", widget.TotalValue)
	}
}

// --------------------------------------------------------------------------
// L4 — low stock.
// --------------------------------------------------------------------------

// L4 — the widget's count and the list's total are the same number.
//
// This is the test the shared `lowStockFrom` fragment exists for. The widget
// says "4 products are low" and links to a list that has to show four rows; two
// copies of "below reorder point" is exactly how one of them acquires an extra
// clause and the pair silently stops agreeing.
//
// The scenario deliberately includes all three edge cases of the rule: a product
// exactly at its point (not low — the comparison is strict), a product with a
// reorder point of zero (never low, or every new product would be), and a
// product with no ledger rows at all (low, because COALESCE makes it zero).
func TestLowStockWidgetAgreesWithTheLowStockList(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Reorder Ltd")
	viewer := f.NewUser(t, map[string]string{"inventory": "viewer"}).FirebaseUID

	below := f.NewProduct(t, "Below the line", "50")
	f.PostLedger(t, below, f.WarehouseID, "10", "adjustment")

	exactly := f.NewProduct(t, "Exactly at the line", "20")
	f.PostLedger(t, exactly, f.WarehouseID, "20", "adjustment")

	noPoint := f.NewProduct(t, "No reorder point set", "0")

	neverReceived := f.NewProduct(t, "Never received", "5")

	widget := getDashboard(t, h, viewer).LowStock

	resp := h.Get(t, "/api/inventory/stock/low?pageSize=100", viewer)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	fromList := testsupport.Decode[list[lowStock]](t, resp)

	if widget.Count != fromList.TotalItems {
		t.Errorf("widget count = %d, list total = %d — the two definitions have drifted",
			widget.Count, fromList.TotalItems)
	}
	if widget.Count != 2 {
		t.Errorf("count = %d, want 2 (below + neverReceived)", widget.Count)
	}

	low := map[string]bool{}
	for _, row := range widget.Products {
		low[row.ProductID] = true
	}
	if !low[below.String()] {
		t.Error("a product under its reorder point should be low")
	}
	if !low[neverReceived.String()] {
		t.Error("a product with no ledger rows at all should be low")
	}
	if low[exactly.String()] {
		t.Error("a product exactly at its reorder point is not yet below it")
	}
	if low[noPoint.String()] {
		t.Error("a product with no reorder point set can never be low")
	}
}

// The preview is capped, and the count is not. A widget that says "5" and lists
// five when there are eleven is worse than one that says eleven and lists five,
// because only the second sends the reader to the full list.
func TestLowStockPreviewIsCappedButTheCountIsNot(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Very Low Ltd")
	viewer := f.NewUser(t, map[string]string{"inventory": "viewer"}).FirebaseUID

	for i := 0; i < 8; i++ {
		f.NewProduct(t, fmt.Sprintf("Low product %d", i), "10")
	}

	widget := getDashboard(t, h, viewer).LowStock
	if widget.Count != 8 {
		t.Errorf("count = %d, want 8", widget.Count)
	}
	if len(widget.Products) != 5 {
		t.Errorf("preview = %d rows, want 5", len(widget.Products))
	}
}

// --------------------------------------------------------------------------
// L5 — recent activity.
// --------------------------------------------------------------------------

// L5 — the last fifteen movements, newest first, each naming its source.
//
// The source number is the point: §10.2 says each entry links to its source
// document, and a link needs the GR number rather than the UUID in
// `stock_ledger.source_id`. This receives goods through the real endpoint so the
// row under test was written by the application, not by a fixture.
func TestRecentActivityIsTheLastFifteenEntriesNewestFirst(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Busy Warehouse")
	actor := f.User.FirebaseUID

	for i := 0; i < 18; i++ {
		f.PostLedger(t, f.ProductID, f.WarehouseID, "1", "adjustment")
	}

	widget := getDashboard(t, h, actor).RecentActivity
	if len(widget.Entries) != 15 {
		t.Fatalf("entries = %d, want 15", len(widget.Entries))
	}

	// Now a real goods receipt, which must appear at the top with its GR number
	// resolved.
	poID := f.NewPurchaseOrder(t)
	lineID := f.NewPOLine(t, poID, f.ProductID, 10)
	receipt := postReceipt(t, h, actor, poID.String(), uuid.NewString(), map[string]any{
		"lines": []map[string]any{{"poLineId": lineID, "qtyReceived": "4"}},
	})
	testsupport.AssertStatus(t, receipt, http.StatusCreated)
	posted := testsupport.Decode[receiptResult](t, receipt)

	top := getDashboard(t, h, actor).RecentActivity.Entries[0]
	if top.SourceType != "goods_receipt" {
		t.Fatalf("newest entry sourceType = %q, want goods_receipt", top.SourceType)
	}
	if top.SourceNumber == nil || *top.SourceNumber != posted.Receipt.GRNumber {
		t.Errorf("sourceNumber = %v, want %q — without it the link is a UUID",
			top.SourceNumber, posted.Receipt.GRNumber)
	}
	if top.SourcePOID == nil || *top.SourcePOID != poID.String() {
		t.Errorf("sourcePoId = %v, want %s", top.SourcePOID, poID)
	}
}

// --------------------------------------------------------------------------
// L6 — isolation.
// --------------------------------------------------------------------------

// L6 — one workspace's dashboard counts nothing of the other's.
//
// Every widget is a bare aggregate with no tenant predicate in it, which is
// correct only because RLS supplies one. This is the test that says so: build
// the same documents in two tenants and check neither total includes the other.
func TestDashboardCountsOnlyTheCallersOwnWorkspace(t *testing.T) {
	h := testsupport.NewHarness(t)
	ours := h.DB.NewTenant(t, "Ours")
	theirs := h.DB.NewTenant(t, "Theirs")

	// Twice as much of everything next door, so a leak cannot be mistaken for a
	// coincidence.
	for i := 0; i < 2; i++ {
		poID := theirs.NewPurchaseOrder(t)
		theirs.NewPOLine(t, poID, theirs.ProductID, 10)
		requisitionID := theirs.NewRequisition(t, "submitted")
		theirs.NewRequisitionLine(t, requisitionID, theirs.ProductID, "1", "10.00", i+1)
		theirs.PostLedger(t, theirs.ProductID, theirs.WarehouseID, "5", "adjustment")
		theirs.NewProduct(t, fmt.Sprintf("Their low product %d", i), "99")
	}
	poID := ours.NewPurchaseOrder(t)
	ours.NewPOLine(t, poID, ours.ProductID, 10)
	requisitionID := ours.NewRequisition(t, "submitted")
	ours.NewRequisitionLine(t, requisitionID, ours.ProductID, "1", "10.00", 1)
	ours.PostLedger(t, ours.ProductID, ours.WarehouseID, "5", "adjustment")
	ours.NewProduct(t, "Our low product", "99")

	got := getDashboard(t, h, ours.User.FirebaseUID)
	if got.OpenOrders.Count != 1 {
		t.Errorf("openOrders = %d, want 1 — the other workspace's orders are showing",
			got.OpenOrders.Count)
	}
	if got.PendingApprovals.Count != 1 {
		t.Errorf("pendingApprovals = %d, want 1", got.PendingApprovals.Count)
	}
	if got.LowStock.Count != 1 {
		t.Errorf("lowStock = %d, want 1", got.LowStock.Count)
	}
	if len(got.RecentActivity.Entries) != 1 {
		t.Errorf("recentActivity = %d entries, want 1", len(got.RecentActivity.Entries))
	}
}
