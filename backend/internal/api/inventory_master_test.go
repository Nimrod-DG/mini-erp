// The rest of Group F's master-data surface, added in Phase 8.
//
// WHY THESE ARE NOT IN inventory_test.go. That file is Phase 4's Group F and G,
// written against the behaviours §12.3 names. This file is the coverage pass §12.6
// asks for, and it found something worth writing down: `GET`, `PATCH` and
// `POST /restore` on a warehouse had **no test at all** — `go tool cover` put all
// three at 0.0%, along with `duplicateWarehouseCode`, the refusal one of them
// raises. Three endpoints the frontend calls on every warehouse edit were shipping
// unexercised.
//
// That is the §9.6.1 failure mode arriving from the other direction: "a half-built
// entity — creatable but not editable — is the most common way a demo falls over."
// Here the entity was fully built and half *tested*, which looks identical until
// something regresses.
package api_test

import (
	"net/http"
	"testing"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// --------------------------------------------------------------------------
// GET /api/inventory/warehouses/:id
// --------------------------------------------------------------------------

func TestWarehouseResolvesByIDWithItsDerivedHoldings(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Warehouse Reads Ltd")
	token := inventoryAdmin(t, f)

	f.PostLedger(t, f.ProductID, f.WarehouseID, "7", "receipt")
	f.PostLedger(t, f.ProductAltID, f.WarehouseID, "3", "receipt")

	got := testsupport.Decode[warehouse](t,
		h.Get(t, "/api/inventory/warehouses/"+f.WarehouseID.String(), token))

	if got.ID != f.WarehouseID.String() {
		t.Fatalf("id = %q, want %q", got.ID, f.WarehouseID)
	}
	// Both numbers are derived on this request (I6). There is no stored counter,
	// which is why they can be checked against the ledger rows just written.
	if got.QtyOnHand != 10 {
		t.Errorf("qtyOnHand = %v, want 10", got.QtyOnHand)
	}
	if got.ProductCount != 2 {
		t.Errorf("productCount = %d, want 2", got.ProductCount)
	}
}

// The unscoped-read rule, for warehouses. A deleted warehouse still resolves by
// id, because the ledger rows and purchase orders that name it have to stay
// readable (§6.9.1) — the same reason G1 asserts it for products.
func TestDeletedWarehouseStillResolvesByID(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Closed Depot Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-CLOSING", "Closing depot")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/warehouses/"+created.ID, token), http.StatusOK)

	got := testsupport.Decode[warehouse](t,
		h.Get(t, "/api/inventory/warehouses/"+created.ID, token))
	if got.DeletedAt == nil {
		t.Error("deletedAt is null on a warehouse that was deleted")
	}
	if got.Code != "WH-CLOSING" {
		t.Errorf("code = %q, want the row to keep its identity", got.Code)
	}
}

func TestWarehouseByIDIsNotFoundForAnUnknownOrMalformedID(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Missing Depot Ltd")
	token := inventoryAdmin(t, f)

	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/warehouses/"+testsupport.NoSuchTenant.String(), token),
		http.StatusNotFound, "not_found")
	// A path that is not a UUID is a 404, not a 500: it is a request for a
	// warehouse that cannot exist.
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/warehouses/not-a-uuid", token),
		http.StatusNotFound, "not_found")
}

// --------------------------------------------------------------------------
// PATCH /api/inventory/warehouses/:id
// --------------------------------------------------------------------------

func TestWarehousePatchChangesOnlyWhatWasSent(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Renaming Depot Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-BEFORE", "Before")

	// An absent field is "leave it alone", which is the contract the frontend's
	// edit form relies on.
	renamed := testsupport.Decode[warehouse](t,
		h.Patch(t, "/api/inventory/warehouses/"+created.ID, token,
			map[string]any{"name": "After"}))
	if renamed.Name != "After" {
		t.Errorf("name = %q, want After", renamed.Name)
	}
	if renamed.Code != "WH-BEFORE" {
		t.Errorf("code = %q, want it untouched by a name-only patch", renamed.Code)
	}

	recoded := testsupport.Decode[warehouse](t,
		h.Patch(t, "/api/inventory/warehouses/"+created.ID, token,
			map[string]any{"code": "  WH-AFTER  "}))
	// Trimmed, like every other code and name in the application: " WH-1" and
	// "WH-1" are not two warehouses.
	if recoded.Code != "WH-AFTER" {
		t.Errorf("code = %q, want the value trimmed", recoded.Code)
	}
	if recoded.Name != "After" {
		t.Errorf("name = %q, want it untouched by a code-only patch", recoded.Name)
	}
}

func TestWarehousePatchRefusesACodeAnotherWarehouseHolds(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Colliding Depots Ltd")
	token := inventoryAdmin(t, f)

	createWarehouse(t, h, token, "WH-TAKEN", "Taken")
	other := createWarehouse(t, h, token, "WH-FREE", "Free")

	body := testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/inventory/warehouses/"+other.ID, token,
			map[string]any{"code": "WH-TAKEN"}),
		http.StatusConflict, "in_use")
	// The code is named in the details, so a screen can point at the field that
	// is wrong rather than at the form.
	if body.Details["code"] != "WH-TAKEN" {
		t.Errorf("details = %v, want the colliding code named", body.Details)
	}
}

func TestWarehousePatchRejectsEmptyFieldsAndAnEmptyBody(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Blank Fields Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-BLANK", "Blank")
	path := "/api/inventory/warehouses/" + created.ID

	// Clearing a field is not the same request as omitting it. An explicit empty
	// string is a warehouse with no code, which is not a thing.
	for _, sent := range []map[string]any{
		{"code": "   "},
		{"name": ""},
	} {
		testsupport.AssertErrorCode(t,
			h.Patch(t, path, token, sent), http.StatusBadRequest, "malformed")
	}

	// And a patch that asks for nothing is a mistake worth naming rather than a
	// silent 200 that changed nothing.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{}),
		http.StatusBadRequest, "malformed")
}

// A deleted warehouse cannot be edited — restore it first. Editing something in
// the recycle bin is how two rows end up fighting over one code without anyone
// having chosen that, which is the same reasoning patchProduct carries.
func TestWarehousePatchIsRefusedWhileDeleted(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Recycled Depot Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-GONE", "Gone")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/warehouses/"+created.ID, token), http.StatusOK)

	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/inventory/warehouses/"+created.ID, token,
			map[string]any{"name": "Back from the dead"}),
		http.StatusNotFound, "not_found")
}

func TestWarehousePatchCanDiscontinueAndReinstate(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Seasonal Depot Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-SEASON", "Seasonal")

	// `is_active` is a different fact from `deleted_at` (§6.9.1) — an inactive
	// warehouse keeps its stock and stays in every list.
	off := testsupport.Decode[warehouse](t,
		h.Patch(t, "/api/inventory/warehouses/"+created.ID, token,
			map[string]any{"isActive": false}))
	if off.DeletedAt != nil {
		t.Error("deactivating a warehouse must not delete it")
	}
	testsupport.AssertStatus(t,
		h.Patch(t, "/api/inventory/warehouses/"+created.ID, token,
			map[string]any{"isActive": true}), http.StatusOK)
}

// --------------------------------------------------------------------------
// POST /api/inventory/warehouses/:id/restore
// --------------------------------------------------------------------------

func TestWarehouseRestoreBringsItBack(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Reopening Depot Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-REOPEN", "Reopening")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/warehouses/"+created.ID, token), http.StatusOK)

	restored := testsupport.Decode[warehouse](t,
		h.Post(t, "/api/inventory/warehouses/"+created.ID+"/restore", token, nil))
	if restored.DeletedAt != nil {
		t.Error("deletedAt is still set after a restore")
	}

	// And it is back in the list, which is the observable half of a restore.
	page := testsupport.Decode[list[warehouse]](t,
		h.Get(t, "/api/inventory/warehouses", token))
	var found bool
	for _, row := range page.Data {
		if row.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Error("a restored warehouse is missing from the default list")
	}
}

// The warehouse half of G3. A code freed by a delete can be taken by somebody
// else, and then the restore has to be refused — there cannot be two live rows
// holding one code, and `warehouses_code_active` is the partial index that says so.
func TestWarehouseRestoreIsRefusedWhenTheCodeWasTaken(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Contested Code Ltd")
	token := inventoryAdmin(t, f)

	first := createWarehouse(t, h, token, "WH-CONTESTED", "First")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/warehouses/"+first.ID, token), http.StatusOK)
	createWarehouse(t, h, token, "WH-CONTESTED", "Second")

	body := testsupport.AssertErrorCode(t,
		h.Post(t, "/api/inventory/warehouses/"+first.ID+"/restore", token, nil),
		http.StatusConflict, "in_use")
	if body.Details["code"] != "WH-CONTESTED" {
		t.Errorf("details = %v, want the contested code named", body.Details)
	}
}

// Restoring something that was never deleted is not an error. It is the state the
// caller asked for, and a 409 here would make a double-tap on a slow connection
// look like a failure.
func TestWarehouseRestoreIsIdempotent(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Twice Restored Ltd")
	token := inventoryAdmin(t, f)

	created := createWarehouse(t, h, token, "WH-TWICE", "Twice")

	for range 2 {
		row := testsupport.Decode[warehouse](t,
			h.Post(t, "/api/inventory/warehouses/"+created.ID+"/restore", token, nil))
		if row.DeletedAt != nil {
			t.Fatal("a live warehouse came back deleted from restore")
		}
	}
}

func TestWarehouseRestoreIsNotFoundForAnUnknownID(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "No Such Depot Ltd")
	token := inventoryAdmin(t, f)

	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/inventory/warehouses/"+testsupport.NoSuchTenant.String()+"/restore", token, nil),
		http.StatusNotFound, "not_found")
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/inventory/warehouses/not-a-uuid/restore", token, nil),
		http.StatusNotFound, "not_found")
}

// --------------------------------------------------------------------------
// PATCH /api/inventory/products/:id — the branches Phase 4 left unexercised.
// --------------------------------------------------------------------------

func TestProductPatchChangesEachFieldIndependently(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Editable Catalogue Ltd")
	token := inventoryAdmin(t, f)

	created := createProduct(t, h, token, "SKU-EDIT", "Editable")
	path := "/api/inventory/products/" + created.ID

	// One field per request, and every other field unchanged after each — which
	// is the property the frontend's changed-fields-only PATCH depends on.
	sku := testsupport.Decode[product](t,
		h.Patch(t, path, token, map[string]any{"sku": "  SKU-EDITED  "}))
	if sku.SKU != "SKU-EDITED" || sku.Name != "Editable" {
		t.Errorf("sku patch changed more than the sku: %+v", sku)
	}

	uom := testsupport.Decode[product](t,
		h.Patch(t, path, token, map[string]any{"uom": "carton"}))
	if uom.UOM != "carton" || uom.SKU != "SKU-EDITED" {
		t.Errorf("uom patch changed more than the uom: %+v", uom)
	}

	// Quantities and money as decimal strings, never floats (I8).
	numbers := testsupport.Decode[product](t,
		h.Patch(t, path, token, map[string]any{
			"reorderPoint": "12.5000",
			"standardCost": "99.99",
		}))
	if numbers.ReorderPoint != 12.5 {
		t.Errorf("reorderPoint = %v, want 12.5", numbers.ReorderPoint)
	}
	if numbers.StandardCost != 99.99 {
		t.Errorf("standardCost = %v, want 99.99", numbers.StandardCost)
	}
}

func TestProductPatchRejectsBlankAndNegativeValues(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Validated Catalogue Ltd")
	token := inventoryAdmin(t, f)

	created := createProduct(t, h, token, "SKU-VALID", "Validated")
	path := "/api/inventory/products/" + created.ID

	for _, sent := range []map[string]any{
		{"sku": " "},
		{"name": ""},
		{"uom": "  "},
		// A negative reorder point or cost is not a correction, it is a typo.
		{"reorderPoint": "-1"},
		{"standardCost": "-0.01"},
		{"reorderPoint": "not a number"},
		{},
	} {
		testsupport.AssertErrorCode(t,
			h.Patch(t, path, token, sent), http.StatusBadRequest, "malformed")
	}
}

func TestProductPatchRefusesASKUAnotherProductHolds(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Colliding SKUs Ltd")
	token := inventoryAdmin(t, f)

	createProduct(t, h, token, "SKU-HELD", "Held")
	other := createProduct(t, h, token, "SKU-OTHER", "Other")

	body := testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/inventory/products/"+other.ID, token,
			map[string]any{"sku": "SKU-HELD"}),
		http.StatusConflict, "in_use")
	if body.Details["sku"] != "SKU-HELD" {
		t.Errorf("details = %v, want the colliding sku named", body.Details)
	}
}

// The rule the handler's own comment states: a deleted product cannot be edited.
func TestProductPatchIsRefusedWhileDeleted(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Recycled Catalogue Ltd")
	token := inventoryAdmin(t, f)

	created := createProduct(t, h, token, "SKU-BIN", "In the bin")
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/inventory/products/"+created.ID, token), http.StatusOK)

	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/inventory/products/"+created.ID, token,
			map[string]any{"name": "Edited while deleted"}),
		http.StatusNotFound, "not_found")
}

func TestProductPatchIsNotFoundForAnUnknownID(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Absent Catalogue Ltd")
	token := inventoryAdmin(t, f)

	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/inventory/products/"+testsupport.NoSuchTenant.String(), token,
			map[string]any{"name": "Nobody"}),
		http.StatusNotFound, "not_found")
	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/inventory/products/not-a-uuid", token,
			map[string]any{"name": "Nobody"}),
		http.StatusNotFound, "not_found")
}
