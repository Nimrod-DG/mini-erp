// Group F — Inventory, and the Phase 4 half of Group G — Deletion policy
// (§12.3). Every test drives the real routes built by api.New, gated by the real
// RequireModule, so what is asserted is what ships.
//
// The two that would catch a silent design failure are G2 and G5. G2 is the only
// thing that proves products_sku_active is a *partial* index — build this whole
// phase without it and nothing complains until someone deletes a product and
// tries to reuse the code. G5 is the check that a warehouse's emptiness is asked
// per product rather than as one total, which a single-product test cannot see.
package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// --------------------------------------------------------------------------
// Response shapes. Quantities are decoded as json.Number-free strings would be
// awkward here, so they arrive as float64 in the test only — the *server* never
// sees one (I8), and a test comparing 47 to 47 is not where precision is lost.
// --------------------------------------------------------------------------

type product struct {
	ID                string  `json:"id"`
	SKU               string  `json:"sku"`
	Name              string  `json:"name"`
	UOM               string  `json:"uom"`
	ReorderPoint      float64 `json:"reorderPoint"`
	StandardCost      float64 `json:"standardCost"`
	IsActive          bool    `json:"isActive"`
	QtyOnHand         float64 `json:"qtyOnHand"`
	BelowReorderPoint bool    `json:"belowReorderPoint"`
	DeletedAt         *string `json:"deletedAt"`
	Balances          []struct {
		WarehouseID   string  `json:"warehouseId"`
		WarehouseCode string  `json:"warehouseCode"`
		QtyOnHand     float64 `json:"qtyOnHand"`
	} `json:"balances"`
}

type warehouse struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	QtyOnHand    float64 `json:"qtyOnHand"`
	ProductCount int     `json:"productCount"`
	DeletedAt    *string `json:"deletedAt"`
}

type stockCell struct {
	ProductID      string  `json:"productId"`
	SKU            string  `json:"sku"`
	ProductDeleted bool    `json:"productDeleted"`
	WarehouseID    string  `json:"warehouseId"`
	WarehouseCode  string  `json:"warehouseCode"`
	QtyOnHand      float64 `json:"qtyOnHand"`
}

type lowStock struct {
	ProductID    string  `json:"productId"`
	SKU          string  `json:"sku"`
	QtyOnHand    float64 `json:"qtyOnHand"`
	ReorderPoint float64 `json:"reorderPoint"`
	Shortfall    float64 `json:"shortfall"`
}

type ledgerEntry struct {
	ID         string  `json:"id"`
	EntryType  string  `json:"entryType"`
	QtyDelta   float64 `json:"qtyDelta"`
	UnitCost   float64 `json:"unitCost"`
	SourceType string  `json:"sourceType"`
	SourceID   *string `json:"sourceId"`
	// Resolved from the source document, so a receipt's ledger row can name where
	// it came from. Null for a manual adjustment, which has no document.
	SourceNumber   *string `json:"sourceNumber"`
	SourcePOID     *string `json:"sourcePoId"`
	Note           *string `json:"note"`
	ProductID      string  `json:"productId"`
	SKU            string  `json:"sku"`
	ProductName    string  `json:"productName"`
	ProductDeleted bool    `json:"productDeleted"`
	WarehouseCode  string  `json:"warehouseCode"`
	CreatedByID    string  `json:"createdById"`
	CreatedByName  string  `json:"createdByName"`
}

type list[T any] struct {
	Data       []T `json:"data"`
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

type adjustmentResult struct {
	Entry     ledgerEntry `json:"entry"`
	QtyOnHand float64     `json:"qtyOnHand"`
}

// inventoryAdmin is a staff user with `admin` in inventory and nothing else —
// the level §9.5 requires for every master-data write. Staff rather than a
// tenant admin on purpose: a tenant admin resolves to `admin` everywhere
// implicitly, which would make these tests unable to tell an inventory
// permission from a workspace one.
func inventoryAdmin(t *testing.T, f *testsupport.TenantFixture) string {
	t.Helper()
	return f.NewUser(t, map[string]string{"inventory": "admin"}).FirebaseUID
}

func postAdjustment(t *testing.T, h *testsupport.Harness, token string, productID, warehouseID uuid.UUID, qty string, note string) *http.Response {
	t.Helper()
	return h.Post(t, "/api/inventory/adjustments", token, map[string]any{
		"productId":   productID.String(),
		"warehouseId": warehouseID.String(),
		// A string, so the quantity never passes through a float on its way to
		// the server either. The endpoint accepts both forms.
		"qtyDelta": qty,
		"note":     note,
	})
}

// --------------------------------------------------------------------------
// Group F — Inventory.
// --------------------------------------------------------------------------

// F1 — the balance view equals the ledger sum for a product/warehouse pair.
//
// Asserted through the API and again straight against the ledger, because the
// claim is that the two agree. Reading only the view would pass even if the view
// were a stored column, which is the design decision I6 exists to defend.
func TestF1BalanceViewEqualsTheLedgerSum(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Balances Ltd")
	token := inventoryAdmin(t, f)

	for _, qty := range []string{"10", "5.5", "-3.25"} {
		f.PostLedger(t, f.ProductID, f.WarehouseID, qty, "adjustment")
	}
	// A second warehouse, so a query that forgets to scope by warehouse is
	// visibly wrong rather than accidentally right.
	other := f.NewWarehouse(t, "Overflow")
	f.PostLedger(t, f.ProductID, other, "100", "adjustment")

	resp := h.Get(t, fmt.Sprintf("/api/inventory/stock?productId=%s&warehouseId=%s",
		f.ProductID, f.WarehouseID), token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	page := testsupport.Decode[list[stockCell]](t, resp)

	if len(page.Data) != 1 {
		t.Fatalf("stock rows = %d, want 1", len(page.Data))
	}
	if got := page.Data[0].QtyOnHand; got != 12.25 {
		t.Errorf("qtyOnHand = %v, want 12.25 (10 + 5.5 - 3.25)", got)
	}

	// The same number, computed by summing the ledger directly. If these ever
	// disagree, something is storing a total.
	var direct float64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT COALESCE(SUM(qty_delta), 0) FROM stock_ledger
			WHERE product_id = ? AND warehouse_id = ?`,
			f.ProductID, f.WarehouseID).Scan(&direct).Error
	})
	if direct != page.Data[0].QtyOnHand {
		t.Errorf("view says %v, SUM(qty_delta) says %v — a balance is being stored somewhere",
			page.Data[0].QtyOnHand, direct)
	}
}

// F2 — the low-stock query returns exactly the products under their reorder
// point. "Exactly" is the assertion: the products that must NOT appear are
// listed here as deliberately as the one that must.
func TestF2LowStockReturnsExactlyTheProductsUnderTheirPoint(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Reorder Ltd")
	token := inventoryAdmin(t, f)

	below := f.NewProduct(t, "Below the point", "20")   // 5 on hand, low
	exactly := f.NewProduct(t, "Exactly at it", "20")   // 20 on hand, NOT low
	above := f.NewProduct(t, "Comfortably above", "20") // 50 on hand, not low
	never := f.NewProduct(t, "Never received", "20")    // no ledger rows, low
	noPoint := f.NewProduct(t, "No reorder point", "0") // 0 on hand, not low

	f.PostLedger(t, below, f.WarehouseID, "5", "adjustment")
	f.PostLedger(t, exactly, f.WarehouseID, "20", "adjustment")
	f.PostLedger(t, above, f.WarehouseID, "50", "adjustment")

	resp := h.Get(t, "/api/inventory/stock/low?pageSize=100", token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	page := testsupport.Decode[list[lowStock]](t, resp)

	got := map[string]lowStock{}
	for _, row := range page.Data {
		got[row.ProductID] = row
	}

	for _, want := range []struct {
		id    uuid.UUID
		label string
	}{{below, "5 on hand against a point of 20"}, {never, "nothing ever received"}} {
		if _, ok := got[want.id.String()]; !ok {
			t.Errorf("product with %s is missing from the low-stock list", want.label)
		}
	}
	for _, unwanted := range []struct {
		id    uuid.UUID
		label string
	}{
		{exactly, "at the reorder point, which is not below it"},
		{above, "above the reorder point"},
		{noPoint, "no reorder point set, so nothing to be below"},
	} {
		if _, ok := got[unwanted.id.String()]; ok {
			t.Errorf("product %s should not be low", unwanted.label)
		}
	}

	if row := got[below.String()]; row.Shortfall != 15 {
		t.Errorf("shortfall = %v, want 15 (20 - 5)", row.Shortfall)
	}
	if row := got[never.String()]; row.QtyOnHand != 0 || row.Shortfall != 20 {
		t.Errorf("never-received product = %+v, want 0 on hand and a shortfall of 20", row)
	}
}

// F3 — a manual adjustment with a negative delta reduces the balance, and the
// row it writes carries the entry and source types from the naming contract.
func TestF3NegativeAdjustmentReducesTheBalance(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Shrinkage Ltd")
	token := inventoryAdmin(t, f)

	f.PostLedger(t, f.ProductID, f.WarehouseID, "40", "receipt")

	resp := postAdjustment(t, h, token, f.ProductID, f.WarehouseID, "-12.5", "annual count")
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	result := testsupport.Decode[adjustmentResult](t, resp)

	if result.QtyOnHand != 27.5 {
		t.Errorf("balance after = %v, want 27.5 (40 - 12.5)", result.QtyOnHand)
	}
	if result.Entry.EntryType != "adjustment" || result.Entry.SourceType != "manual_adjustment" {
		t.Errorf("entry = %s/%s, want adjustment/manual_adjustment",
			result.Entry.EntryType, result.Entry.SourceType)
	}
	if result.Entry.SourceID != nil {
		t.Errorf("sourceId = %v, want null — a manual adjustment has no document behind it",
			*result.Entry.SourceID)
	}
	if result.Entry.QtyDelta != -12.5 {
		t.Errorf("qtyDelta = %v, want -12.5", result.Entry.QtyDelta)
	}
	// The cost the entry was valued at defaults to the product's standard cost
	// rather than to zero, or the adjustment would understate inventory value.
	if result.Entry.UnitCost != 10 {
		t.Errorf("unitCost = %v, want the product's standard cost of 10", result.Entry.UnitCost)
	}

	// And the view agrees, read back through the grid.
	page := testsupport.Decode[list[stockCell]](t, h.Get(t,
		fmt.Sprintf("/api/inventory/stock?productId=%s", f.ProductID), token))
	if len(page.Data) != 1 || page.Data[0].QtyOnHand != 27.5 {
		t.Errorf("stock grid = %+v, want a single row of 27.5", page.Data)
	}
}

// An adjustment of zero is refused with a sentence rather than a constraint
// name, and nothing is written. ledger_qty_nonzero would refuse it anyway; the
// handler check exists so the user is told why.
func TestAdjustmentOfZeroIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Zero Ltd")
	token := inventoryAdmin(t, f)

	testsupport.AssertErrorCode(t,
		postAdjustment(t, h, token, f.ProductID, f.WarehouseID, "0.0000", "nothing happened"),
		http.StatusBadRequest, "malformed")

	assertLedgerCount(t, f, 0)
}

// An adjustment against a soft-deleted product is a 404. This is the picker
// case, not the historical-reference case: new movements need live master data.
func TestAdjustmentAgainstDeletedProductIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Deleted Picker Ltd")
	token := inventoryAdmin(t, f)

	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductID.String(), token),
		http.StatusOK)

	testsupport.AssertErrorCode(t,
		postAdjustment(t, h, token, f.ProductID, f.WarehouseID, "5", "should not land"),
		http.StatusNotFound, "not_found")

	assertLedgerCount(t, f, 0)
}

// --------------------------------------------------------------------------
// Group G — Deletion policy, the Phase 4 half.
// --------------------------------------------------------------------------

// G1 — a soft-deleted product is absent from default queries and present when
// asked for.
//
// The raw-SQL equivalent of §6.9.1's `.Unscoped()`: the list filters, the
// includeDeleted view does not, and resolving by ID never does — because the
// ledger row below still has to render the product's name.
func TestG1SoftDeletedProductIsHiddenButStillResolves(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Recycle Bin Ltd")
	token := inventoryAdmin(t, f)

	// A movement first, so there is history pointing at the product when it goes.
	f.PostLedger(t, f.ProductID, f.WarehouseID, "7", "receipt")

	sku := skuOf(t, h, token, f.ProductID)

	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductID.String(), token),
		http.StatusOK)

	if listContains(t, h, token, "/api/inventory/products?pageSize=100", f.ProductID) {
		t.Error("a deleted product is still in the default product list")
	}
	if !listContains(t, h, token, "/api/inventory/products?pageSize=100&includeDeleted=true", f.ProductID) {
		t.Error("a deleted product is missing from the includeDeleted list")
	}

	// Resolvable by ID, with deletedAt set so the screen can say so.
	resp := h.Get(t, "/api/inventory/products/"+f.ProductID.String(), token)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	if got := testsupport.Decode[product](t, resp); got.DeletedAt == nil {
		t.Error("the product resolves but does not report that it is deleted")
	}

	// And the ledger still names it. This is the whole point of soft delete: a
	// movement that happened must stay readable.
	entries := testsupport.Decode[list[ledgerEntry]](t,
		h.Get(t, "/api/inventory/ledger?productId="+f.ProductID.String(), token))
	if len(entries.Data) != 1 {
		t.Fatalf("ledger rows = %d, want 1 — deleting a product deleted its history",
			len(entries.Data))
	}
	if entries.Data[0].SKU != sku || entries.Data[0].ProductName == "" {
		t.Errorf("ledger row = %+v, want the deleted product's sku and name resolved",
			entries.Data[0])
	}
	if !entries.Data[0].ProductDeleted {
		t.Error("the ledger row does not mark its product as deleted")
	}
}

// Deleting a product does not hide its stock, and three places have to agree
// about that.
//
// The first version of this screen hid deleted products from the grid, and it
// produced a contradiction a user hit immediately: the warehouse list said "1
// product, 30 on hand" and refused the delete over stock the grid showed
// nowhere. A balance is not the product record — it is a quantity of goods in a
// place, and the goods do not leave the shelf when somebody tidies up the
// catalogue.
func TestDeletedProductsStockStaysVisibleEverywhere(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Stranded Stock Ltd")
	token := inventoryAdmin(t, f)

	f.PostLedger(t, f.ProductID, f.WarehouseID, "30", "receipt")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductID.String(), token),
		http.StatusOK)

	// 1. The grid still shows the balance, marked.
	grid := testsupport.Decode[list[stockCell]](t,
		h.Get(t, "/api/inventory/stock?productId="+f.ProductID.String(), token))
	if len(grid.Data) != 1 {
		t.Fatalf("stock grid rows = %d, want 1 — deleting the product hid its stock",
			len(grid.Data))
	}
	if grid.Data[0].QtyOnHand != 30 {
		t.Errorf("grid qtyOnHand = %v, want 30", grid.Data[0].QtyOnHand)
	}
	if !grid.Data[0].ProductDeleted {
		t.Error("the grid row does not mark its product as deleted, so the reader " +
			"cannot tell why it is absent from the product list")
	}

	// 2. The warehouse counts it, and says the same number.
	warehouses := testsupport.Decode[list[warehouse]](t,
		h.Get(t, "/api/inventory/warehouses", token))
	if len(warehouses.Data) != 1 {
		t.Fatalf("warehouses = %d, want 1", len(warehouses.Data))
	}
	if got := warehouses.Data[0]; got.QtyOnHand != 30 || got.ProductCount != 1 {
		t.Errorf("warehouse reports %v across %d products, want 30 across 1 — "+
			"this is the number the delete refusal is about, so it has to match the grid",
			got.QtyOnHand, got.ProductCount)
	}

	// 3. And the refusal still fires, which is the whole reason the two above
	//    must agree: being refused over stock you cannot see is the bug.
	testsupport.AssertErrorCode(t,
		h.Delete(t, "/api/inventory/warehouses/"+f.WarehouseID.String(), token),
		http.StatusConflict, "in_use")
}

// A viewer cannot see the recycle bin. The list is a `viewer` route, so this is
// enforced inside the handler rather than by the route's gate.
func TestIncludeDeletedRequiresModuleAdmin(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Viewer Ltd")
	viewer := f.NewUser(t, map[string]string{"inventory": "viewer"}).FirebaseUID

	testsupport.AssertStatus(t,
		h.Get(t, "/api/inventory/products", viewer), http.StatusOK)

	body := testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/products?includeDeleted=true", viewer),
		http.StatusForbidden, "insufficient_module_role")
	if body.Details["required"] != "admin" || body.Details["actual"] != "viewer" {
		t.Errorf("details = %v, want required admin and actual viewer", body.Details)
	}
}

// G2 — soft-deleting a product then creating a new one with the same SKU
// succeeds.
//
// THE test for products_sku_active. A plain UNIQUE (tenant_id, sku) passes every
// other test in this file and fails this one, because the deleted row still
// occupies the SKU. Without the partial index this phase looks finished and
// breaks the first time a customer deletes something.
func TestG2DeletedSKUCanBeReused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "SKU Reuse Ltd")
	token := inventoryAdmin(t, f)

	created := createProduct(t, h, token, "SKU-REUSE", "First attempt")

	// The same SKU while the first is live is refused — the index still binds
	// live rows, which is what makes the success below meaningful.
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/inventory/products", token,
			map[string]any{"sku": "SKU-REUSE", "name": "Too early"}),
		http.StatusConflict, "in_use")

	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+created.ID, token), http.StatusOK)

	replacement := createProduct(t, h, token, "SKU-REUSE", "Second attempt")
	if replacement.ID == created.ID {
		t.Fatal("the replacement reused the deleted row rather than being a new product")
	}
}

// G3 — restoring the first product while the replacement SKU exists is rejected
// with 409. The partial index that permitted G2 is the same one refusing here,
// and the refusal names the SKU so the user knows what to change.
func TestG3RestoreIsRefusedWhenTheSKUWasTaken(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Restore Conflict Ltd")
	token := inventoryAdmin(t, f)

	first := createProduct(t, h, token, "SKU-CONTESTED", "First")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+first.ID, token), http.StatusOK)
	createProduct(t, h, token, "SKU-CONTESTED", "Second")

	body := testsupport.AssertErrorCode(t,
		h.Post(t, "/api/inventory/products/"+first.ID+"/restore", token, nil),
		http.StatusConflict, "in_use")
	if body.Details["sku"] != "SKU-CONTESTED" {
		t.Errorf("details.sku = %v, want SKU-CONTESTED", body.Details["sku"])
	}

	// And the refusal left it deleted rather than half-restored.
	got := testsupport.Decode[product](t,
		h.Get(t, "/api/inventory/products/"+first.ID, token))
	if got.DeletedAt == nil {
		t.Error("the refused restore restored it anyway")
	}
}

// Restore works when nothing is in the way, at the same level that deleted it —
// §6.9.1: any user who can delete can restore.
func TestRestoreBringsAProductBack(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Restore Ltd")
	token := inventoryAdmin(t, f)

	created := createProduct(t, h, token, "SKU-BACK", "Deleted by mistake")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+created.ID, token), http.StatusOK)

	resp := h.Post(t, "/api/inventory/products/"+created.ID+"/restore", token, nil)
	testsupport.AssertStatus(t, resp, http.StatusOK)
	if got := testsupport.Decode[product](t, resp); got.DeletedAt != nil {
		t.Error("restore returned a product that is still deleted")
	}
	if !listContains(t, h, token, "/api/inventory/products?pageSize=100", uuid.MustParse(created.ID)) {
		t.Error("a restored product is missing from the default list")
	}
}

// The product half of G4: deleting master data is refused while an OPEN
// commitment references it, and permitted once that commitment is closed.
//
// G4 proper is about suppliers, whose endpoints arrive with procurement in
// Phase 5. The rule and the check are the same shape, and the data it acts on —
// purchase order lines — already exists, so it is tested here rather than
// deferred with the endpoint.
func TestProductWithOpenPOLineCannotBeDeleted(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Committed Ltd")
	token := inventoryAdmin(t, f)

	po := f.NewPurchaseOrder(t)
	f.NewPOLine(t, po, f.ProductID, 10)

	body := testsupport.AssertErrorCode(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductID.String(), token),
		http.StatusConflict, "in_use")
	if body.Details["openPurchaseOrderLines"] != float64(1) {
		t.Errorf("details = %v, want it to name the 1 open line that blocks the delete",
			body.Details)
	}

	// A historical reference does NOT block: a product on a received order is
	// exactly what soft delete is for (§6.9.1).
	f.SetPOStatus(t, po, "received")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductID.String(), token),
		http.StatusOK)
}

// G5 — deleting a warehouse with non-zero stock returns 409 in_use.
//
// The two products are the point. Their balances are +5 and -5, so any check
// that asks "does this warehouse sum to zero?" says yes and allows the delete,
// stranding both. Emptiness is a question per product.
func TestG5WarehouseWithStockCannotBeDeleted(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Full Shelves Ltd")
	token := inventoryAdmin(t, f)

	f.PostLedger(t, f.ProductID, f.WarehouseID, "5", "receipt")
	f.PostLedger(t, f.ProductAltID, f.WarehouseID, "-5", "adjustment")

	body := testsupport.AssertErrorCode(t,
		h.Delete(t, "/api/inventory/warehouses/"+f.WarehouseID.String(), token),
		http.StatusConflict, "in_use")
	if body.Details["productsWithStock"] != float64(2) {
		t.Errorf("details = %v, want both products named as blocking", body.Details)
	}

	// Bring both to zero and it goes.
	f.PostLedger(t, f.ProductID, f.WarehouseID, "-5", "adjustment")
	f.PostLedger(t, f.ProductAltID, f.WarehouseID, "5", "adjustment")

	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/warehouses/"+f.WarehouseID.String(), token),
		http.StatusOK)
}

// An empty warehouse deletes, and the code frees up for reuse — the warehouse
// half of G2, against warehouses_code_active.
func TestEmptyWarehouseDeletesAndFreesItsCode(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Empty Shelves Ltd")
	token := inventoryAdmin(t, f)

	first := createWarehouse(t, h, token, "WH-REUSE", "First")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/warehouses/"+first.ID, token), http.StatusOK)
	createWarehouse(t, h, token, "WH-REUSE", "Second")
}

// G6 — a purchase order line referencing a soft-deleted product still resolves
// the product name.
//
// Asserted at the query level, because purchase order screens arrive in Phase 5.
// The join that has to keep working is the one without a deleted filter, and
// writing it here is what stops Phase 5 from adding `AND p.deleted_at IS NULL`
// to it by reflex.
func TestG6POLineStillResolvesADeletedProductsName(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Historical Reference Ltd")
	token := inventoryAdmin(t, f)

	po := f.NewPurchaseOrder(t)
	line := f.NewPOLine(t, po, f.ProductID, 10)
	f.SetPOStatus(t, po, "received") // so the delete is allowed
	name := nameOf(t, h, token, f.ProductID)

	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+f.ProductID.String(), token),
		http.StatusOK)

	var resolved []struct {
		Name    string
		Deleted bool
	}
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`
			SELECT p.name, (p.deleted_at IS NOT NULL) AS deleted
			FROM purchase_order_lines pol
			JOIN products p ON p.id = pol.product_id
			WHERE pol.id = ?`, line).Scan(&resolved).Error
	})
	if len(resolved) != 1 {
		t.Fatalf("the PO line no longer resolves its product at all (%d rows)", len(resolved))
	}
	if resolved[0].Name != name {
		t.Errorf("resolved name = %q, want %q", resolved[0].Name, name)
	}
	if !resolved[0].Deleted {
		t.Error("the product is not actually deleted, so this test proved nothing")
	}
}

// G9 — UPDATE and DELETE against the immutable ledgers raise permission errors
// as erp_app.
//
// The grant, not a trigger, is the enforcement: a grant cannot be turned off by
// ALTER TABLE ... DISABLE TRIGGER, and there is nothing to remember on a new
// code path (§6.9.3). This asserts the two tables §12.3 names by ID.
func TestG9LedgersRejectUpdateAndDeleteAsErpApp(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Append Only Ltd")

	entry := f.PostLedger(t, f.ProductID, f.WarehouseID, "3", "adjustment")
	var journal uuid.UUID
	f.Must(t, func(tx *gorm.DB) error {
		journal = f.NewJournalEntry(t, tx)
		// Balanced, or the deferred trigger refuses at commit for the wrong
		// reason entirely.
		if err := tx.Exec(`
			INSERT INTO journal_entry_lines (id, tenant_id, journal_entry_id, account_id, debit, credit)
			VALUES (gen_random_uuid(), ?, ?, ?, 100.00, 0), (gen_random_uuid(), ?, ?, ?, 0, 100.00)`,
			f.ID, journal, f.InventoryAccountID,
			f.ID, journal, f.GRNIAccountID).Error; err != nil {
			return err
		}
		return nil
	})

	for _, attempt := range []struct {
		label string
		sql   string
		args  []any
	}{
		{"UPDATE stock_ledger", `UPDATE stock_ledger SET qty_delta = 999 WHERE id = ?`, []any{entry}},
		{"DELETE stock_ledger", `DELETE FROM stock_ledger WHERE id = ?`, []any{entry}},
		{"UPDATE journal_entries", `UPDATE journal_entries SET description = 'edited' WHERE id = ?`, []any{journal}},
		{"DELETE journal_entries", `DELETE FROM journal_entries WHERE id = ?`, []any{journal}},
	} {
		err := f.AsTenant(t, func(tx *gorm.DB) error {
			return tx.Exec(attempt.sql, attempt.args...).Error
		})
		if err == nil {
			t.Errorf("%s succeeded as erp_app — the ledger is editable", attempt.label)
			continue
		}
		if state := testsupport.PGCode(err); state != testsupport.CodeInsufficientPrivilege {
			t.Errorf("%s failed with SQLSTATE %s, want %s (insufficient privilege): %v",
				attempt.label, state, testsupport.CodeInsufficientPrivilege, err)
		}
	}

	// And the row is still there, unchanged.
	var qty float64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT qty_delta FROM stock_ledger WHERE id = ?`, entry).
			Scan(&qty).Error
	})
	if qty != 3 {
		t.Errorf("qty_delta = %v, want 3 — something got through", qty)
	}
}

// G10 — deactivating a user preserves their actor_id on historical documents.
//
// Users are deactivated and never deleted precisely because they are the
// created_by on every row like this one (§6.9.4). The ledger keeps naming them
// afterwards, which is what makes the trail readable a year later.
func TestG10DeactivatedUserStillNamesTheirLedgerEntries(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Departures Ltd")
	reader := inventoryAdmin(t, f)

	leaver := f.NewUser(t, map[string]string{"inventory": "approver"})
	resp := postAdjustment(t, h, leaver.FirebaseUID, f.ProductID, f.WarehouseID,
		"6", "counted before leaving")
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	written := testsupport.Decode[adjustmentResult](t, resp).Entry

	h.DB.Deactivate(t, leaver.ID)

	// They can no longer make one — identity resolution turns them away (I9).
	testsupport.AssertStatus(t,
		postAdjustment(t, h, leaver.FirebaseUID, f.ProductID, f.WarehouseID, "1", "after leaving"),
		http.StatusUnauthorized)

	// But the entry they already made still names them.
	entries := testsupport.Decode[list[ledgerEntry]](t,
		h.Get(t, "/api/inventory/ledger?productId="+f.ProductID.String(), reader))
	found := false
	for _, entry := range entries.Data {
		if entry.ID != written.ID {
			continue
		}
		found = true
		if entry.CreatedByID != leaver.ID.String() || entry.CreatedByName == "" {
			t.Errorf("entry = %+v, want it still attributed to the deactivated user", entry)
		}
	}
	if !found {
		t.Error("the deactivated user's ledger entry disappeared with them")
	}
}

// G11 — po_line_status.qty_received equals the sum of that line's receipt lines,
// and qty_outstanding is the remainder.
//
// A view test rather than an endpoint test: receipts are Phase 5. What is being
// asserted now is that received quantity is derived and not stored (I6) — there
// is no qty_received column on purchase_order_lines, and this is what proves the
// view is doing the work.
func TestG11POLineStatusDerivesReceivedAndOutstanding(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Partial Receipts Ltd")

	po := f.NewPurchaseOrder(t)
	line := f.NewPOLine(t, po, f.ProductID, 10)

	read := func() (received, outstanding float64) {
		t.Helper()
		var rows []struct {
			QtyReceived    float64
			QtyOutstanding float64
		}
		f.Must(t, func(tx *gorm.DB) error {
			return tx.Raw(`
				SELECT qty_received, qty_outstanding
				FROM po_line_status WHERE po_line_id = ?`, line).Scan(&rows).Error
		})
		if len(rows) != 1 {
			t.Fatalf("po_line_status rows = %d, want 1", len(rows))
		}
		return rows[0].QtyReceived, rows[0].QtyOutstanding
	}

	// Nothing received yet: a line with no receipt lines must read as zero
	// received, not as absent. The LEFT JOIN in the view is what makes that true.
	if received, outstanding := read(); received != 0 || outstanding != 10 {
		t.Errorf("before any receipt: received %v outstanding %v, want 0 and 10",
			received, outstanding)
	}

	gr := f.NewGoodsReceipt(t, po)
	f.NewGoodsReceiptLine(t, gr, line, f.ProductID, "4")
	if received, outstanding := read(); received != 4 || outstanding != 6 {
		t.Errorf("after 4 of 10: received %v outstanding %v, want 4 and 6",
			received, outstanding)
	}

	// A second receipt against the same line sums rather than replaces.
	gr2 := f.NewGoodsReceipt(t, po)
	f.NewGoodsReceiptLine(t, gr2, line, f.ProductID, "6")
	if received, outstanding := read(); received != 10 || outstanding != 0 {
		t.Errorf("after 4 then 6 of 10: received %v outstanding %v, want 10 and 0",
			received, outstanding)
	}
}

// --------------------------------------------------------------------------
// The module gate, on the routes that ship.
// --------------------------------------------------------------------------

// Every §9.5 route refuses a caller below its level, and the refusal names the
// level that would have worked.
//
// Group B already asserts RequireModule's behaviour against the probe routes in
// testsupport. This asserts the *levels in the route table* — that reads are
// `viewer`, adjustments `approver`, and master-data writes `admin`. A probe
// route cannot catch a real route registered at the wrong level.
func TestInventoryRoutesCarryTheLevelsFromTheSpec(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Levels Ltd")

	viewer := f.NewUser(t, map[string]string{"inventory": "viewer"}).FirebaseUID
	approver := f.NewUser(t, map[string]string{"inventory": "approver"}).FirebaseUID
	product := f.ProductID.String()

	// A user with a level in another module entirely, to check the gate is per
	// module and not per "has any level at all".
	elsewhere := f.NewUser(t, map[string]string{"procurement": "admin"}).FirebaseUID

	reads := []string{
		"/api/inventory/products",
		"/api/inventory/products/" + product,
		"/api/inventory/warehouses",
		"/api/inventory/stock",
		"/api/inventory/stock/low",
		"/api/inventory/ledger",
	}
	for _, path := range reads {
		testsupport.AssertStatus(t, h.Get(t, path, viewer), http.StatusOK)
		testsupport.AssertErrorCode(t, h.Get(t, path, elsewhere),
			http.StatusForbidden, "insufficient_module_role")
	}

	// Master data is `admin`: an approver is refused every write.
	writes := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/inventory/products"},
		{http.MethodPatch, "/api/inventory/products/" + product},
		{http.MethodDelete, "/api/inventory/products/" + product},
		{http.MethodPost, "/api/inventory/products/" + product + "/restore"},
		{http.MethodPost, "/api/inventory/warehouses"},
		{http.MethodPatch, "/api/inventory/warehouses/" + f.WarehouseID.String()},
		{http.MethodDelete, "/api/inventory/warehouses/" + f.WarehouseID.String()},
	}
	for _, write := range writes {
		body := testsupport.AssertErrorCode(t,
			h.Request(t, write.method, write.path, approver, map[string]any{}),
			http.StatusForbidden, "insufficient_module_role")
		if body.Details["required"] != "admin" {
			t.Errorf("%s %s: required = %v, want admin",
				write.method, write.path, body.Details["required"])
		}
	}

	// Adjustments sit between the two: a viewer cannot, an approver can.
	testsupport.AssertErrorCode(t,
		postAdjustment(t, h, viewer, f.ProductID, f.WarehouseID, "1", "not allowed"),
		http.StatusForbidden, "insufficient_module_role")
	testsupport.AssertStatus(t,
		postAdjustment(t, h, approver, f.ProductID, f.WarehouseID, "1", "allowed"),
		http.StatusCreated)
}

// A tenant with the inventory entitlement revoked gets module_not_enabled from
// the real routes — the superadmin's problem, not the tenant admin's (§5.7).
func TestInventoryRoutesRefuseAnUnentitledTenant(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "No Inventory Ltd")
	token := inventoryAdmin(t, f)
	f.DisableModule(t, "inventory")

	testsupport.AssertErrorCode(t, h.Get(t, "/api/inventory/products", token),
		http.StatusForbidden, "module_not_enabled")
}

// Isolation, asserted from two tenants. A single-tenant test cannot detect an
// isolation failure (§12.2), and every inventory table is RLS-forced — so this
// is really a test that the handlers stayed on the transaction TenantTx opened.
func TestInventoryIsNotVisibleAcrossTenants(t *testing.T) {
	h := testsupport.NewHarness(t)
	a := h.DB.NewTenant(t, "Tenant A")
	b := h.DB.NewTenant(t, "Tenant B")
	tokenA := inventoryAdmin(t, a)

	b.PostLedger(t, b.ProductID, b.WarehouseID, "99", "receipt")

	if listContains(t, h, tokenA, "/api/inventory/products?pageSize=100", b.ProductID) {
		t.Error("tenant A can see tenant B's product")
	}

	// By ID, which is the path RLS has to hold on rather than a WHERE clause.
	testsupport.AssertStatus(t,
		h.Get(t, "/api/inventory/products/"+b.ProductID.String(), tokenA),
		http.StatusNotFound)

	// And writes cannot reach across either: the 404 is the same one an
	// unknown ID gets, so nothing distinguishes "exists elsewhere" from
	// "never existed".
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+b.ProductID.String(), tokenA),
		http.StatusNotFound)

	entries := testsupport.Decode[list[ledgerEntry]](t,
		h.Get(t, "/api/inventory/ledger?pageSize=100", tokenA))
	if len(entries.Data) != 0 {
		t.Errorf("tenant A sees %d of tenant B's ledger entries", len(entries.Data))
	}

	// B's own data is intact — which is what makes the emptiness above a
	// filtering result rather than a broken fixture.
	tokenB := inventoryAdmin(t, b)
	theirs := testsupport.Decode[list[ledgerEntry]](t,
		h.Get(t, "/api/inventory/ledger?pageSize=100", tokenB))
	if len(theirs.Data) != 1 {
		t.Errorf("tenant B sees %d of their own ledger entries, want 1", len(theirs.Data))
	}
}

// The §9.0 list contract on the inventory lists: an unknown sort field is an
// error rather than a silent fallback, and sorting is over the whole result set.
func TestInventoryListsFollowTheListContract(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Paging Ltd")
	token := inventoryAdmin(t, f)

	for i := range 5 {
		createProduct(t, h, token, fmt.Sprintf("SKU-P%02d", i), fmt.Sprintf("Product %d", i))
	}

	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/products?sort=nonsense", token),
		http.StatusBadRequest, "malformed")

	// Page 2 of a descending sort holds items from the whole set, not a sorted
	// slice of one page. With 7 products (5 created + 2 from the fixture),
	// page 2 at pageSize 2 is the third and fourth SKUs descending.
	page := testsupport.Decode[list[product]](t,
		h.Get(t, "/api/inventory/products?sort=-sku&pageSize=2&page=2", token))
	if page.TotalItems != 7 {
		t.Fatalf("totalItems = %d, want 7", page.TotalItems)
	}
	if len(page.Data) != 2 {
		t.Fatalf("page size = %d, want 2", len(page.Data))
	}
	if page.Data[0].SKU < page.Data[1].SKU {
		t.Errorf("page 2 is not descending: %s then %s", page.Data[0].SKU, page.Data[1].SKU)
	}

	all := testsupport.Decode[list[product]](t,
		h.Get(t, "/api/inventory/products?sort=-sku&pageSize=100", token))
	if all.Data[2].SKU != page.Data[0].SKU {
		t.Errorf("page 2 starts at %s, but the whole set's third item is %s — "+
			"the sort is being applied per page", page.Data[0].SKU, all.Data[2].SKU)
	}
}

// --------------------------------------------------------------------------
// Helpers.
// --------------------------------------------------------------------------

func createProduct(t *testing.T, h *testsupport.Harness, token, sku, name string) product {
	t.Helper()
	resp := h.Post(t, "/api/inventory/products", token,
		map[string]any{"sku": sku, "name": name, "standardCost": "10.00"})
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	return testsupport.Decode[product](t, resp)
}

func createWarehouse(t *testing.T, h *testsupport.Harness, token, code, name string) warehouse {
	t.Helper()
	resp := h.Post(t, "/api/inventory/warehouses", token,
		map[string]any{"code": code, "name": name})
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	return testsupport.Decode[warehouse](t, resp)
}

func listContains(t *testing.T, h *testsupport.Harness, token, path string, id uuid.UUID) bool {
	t.Helper()
	page := testsupport.Decode[list[product]](t, h.Get(t, path, token))
	for _, row := range page.Data {
		if row.ID == id.String() {
			return true
		}
	}
	return false
}

func skuOf(t *testing.T, h *testsupport.Harness, token string, id uuid.UUID) string {
	t.Helper()
	return testsupport.Decode[product](t,
		h.Get(t, "/api/inventory/products/"+id.String(), token)).SKU
}

func nameOf(t *testing.T, h *testsupport.Harness, token string, id uuid.UUID) string {
	t.Helper()
	return testsupport.Decode[product](t,
		h.Get(t, "/api/inventory/products/"+id.String(), token)).Name
}

func assertLedgerCount(t *testing.T, f *testsupport.TenantFixture, want int64) {
	t.Helper()
	var got int64
	f.Must(t, func(tx *gorm.DB) error {
		return tx.Raw(`SELECT count(*) FROM stock_ledger`).Scan(&got).Error
	})
	if got != want {
		t.Errorf("stock_ledger rows = %d, want %d", got, want)
	}
}
