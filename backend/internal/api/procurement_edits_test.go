// Editing a supplier and editing a draft requisition — the validation branches
// Phase 5 built and Phase 5's tests did not reach.
//
// `patchSupplier` was the widest gap in the procurement group: fourteen of its
// branches are one-line field validations, and Group C only ever sent it a valid
// body. `patchRequisition` was the second, at 52.5%, and the branches it was
// missing are the ones a real editor hits — pointing a draft at a warehouse that
// does not exist, or at one product twice.
package api_test

import (
	"net/http"
	"testing"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

func newSupplier(t *testing.T, h *testsupport.Harness, token string, body map[string]any) supplier {
	t.Helper()
	resp := h.Post(t, "/api/procurement/suppliers", token, body)
	testsupport.AssertStatus(t, resp, http.StatusCreated)
	return testsupport.Decode[supplier](t, resp)
}

// --------------------------------------------------------------------------
// POST /api/procurement/suppliers
// --------------------------------------------------------------------------

func TestSupplierCreateValidatesItsFields(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Supplier Validation Ltd")
	token := procurementUser(t, f, "admin")

	for name, body := range map[string]map[string]any{
		"no code":            {"name": "Nameless Trading"},
		"blank code":         {"code": "   ", "name": "Nameless Trading"},
		"no name":            {"code": "SUP-X"},
		"blank name":         {"code": "SUP-X", "name": " "},
		"negative lead time": {"code": "SUP-X", "name": "Slow Trading", "leadTimeDays": -1},
	} {
		t.Run(name, func(t *testing.T) {
			testsupport.AssertErrorCode(t,
				h.Post(t, "/api/procurement/suppliers", token, body),
				http.StatusBadRequest, "malformed")
		})
	}
}

func TestSupplierCreateHonoursAnExplicitIsActive(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Dormant Supplier Ltd")
	token := procurementUser(t, f, "admin")

	// Inactive by request, which is a different fact from deleted (§6.9.1): the
	// seed uses it for the one supplier a demo needs to show as not orderable.
	created := newSupplier(t, h, token, map[string]any{
		"code": "SUP-DORMANT", "name": "Dormant Trading", "isActive": false,
	})
	if created.IsActive {
		t.Error("isActive = true, want the explicit false to be honoured")
	}
	if created.DeletedAt != nil {
		t.Error("an inactive supplier must not be a deleted one")
	}

	// The default is active, so the field is genuinely optional.
	if other := newSupplier(t, h, token, map[string]any{
		"code": "SUP-DEFAULT", "name": "Default Trading",
	}); !other.IsActive {
		t.Error("a supplier created without isActive should be active")
	}
}

// --------------------------------------------------------------------------
// PATCH /api/procurement/suppliers/:id
// --------------------------------------------------------------------------

func TestSupplierPatchValidatesEveryFieldItAccepts(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Supplier Edits Ltd")
	token := procurementUser(t, f, "admin")

	created := newSupplier(t, h, token, map[string]any{
		"code": "SUP-EDIT", "name": "Editable Trading",
	})
	path := "/api/procurement/suppliers/" + created.ID

	// Clearing a field is not omitting it, and none of these three may be empty.
	for name, body := range map[string]map[string]any{
		"blank code":         {"code": " "},
		"blank name":         {"name": ""},
		"blank terms":        {"paymentTerms": "  "},
		"negative lead time": {"leadTimeDays": -3},
		"nothing at all":     {},
	} {
		t.Run(name, func(t *testing.T) {
			testsupport.AssertErrorCode(t,
				h.Patch(t, path, token, body), http.StatusBadRequest, "malformed")
		})
	}
}

func TestSupplierPatchChangesEachFieldAndClearsTheOptionalOnes(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Contactable Supplier Ltd")
	token := procurementUser(t, f, "admin")

	created := newSupplier(t, h, token, map[string]any{
		"code":         "SUP-CONTACT",
		"name":         "Contactable Trading",
		"contactEmail": "sales@contactable.test",
		"contactPhone": "+62 21 555 0100",
		"leadTimeDays": 7,
		"paymentTerms": "NET30",
	})
	path := "/api/procurement/suppliers/" + created.ID

	renamed := testsupport.Decode[supplier](t,
		h.Patch(t, path, token, map[string]any{
			"name":         "Renamed Trading",
			"leadTimeDays": 14,
			"paymentTerms": "NET60",
		}))
	if renamed.Name != "Renamed Trading" || renamed.LeadTimeDays != 14 || renamed.PaymentTerms != "NET60" {
		t.Errorf("patch did not land: %+v", renamed)
	}
	if renamed.Code != "SUP-CONTACT" {
		t.Errorf("code = %q, want it untouched", renamed.Code)
	}

	// An empty string CLEARS the two optional contact fields, where omitting them
	// leaves them alone — the distinction `nullIfEmpty` exists for, and the only
	// way a screen can remove an email somebody typed by mistake.
	cleared := testsupport.Decode[supplier](t,
		h.Patch(t, path, token, map[string]any{"contactEmail": "", "contactPhone": ""}))
	if cleared.ContactEmail != nil {
		t.Errorf("contactEmail = %v, want it cleared to null", *cleared.ContactEmail)
	}

	// Zero is a legitimate lead time — same-day delivery — and must not be read as
	// "not supplied".
	zero := testsupport.Decode[supplier](t,
		h.Patch(t, path, token, map[string]any{"leadTimeDays": 0}))
	if zero.LeadTimeDays != 0 {
		t.Errorf("leadTimeDays = %d, want 0 to be storable", zero.LeadTimeDays)
	}
}

func TestSupplierPatchRefusesACodeAnotherSupplierHolds(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Colliding Suppliers Ltd")
	token := procurementUser(t, f, "admin")

	newSupplier(t, h, token, map[string]any{"code": "SUP-CLAIMED", "name": "Claimed"})
	other := newSupplier(t, h, token, map[string]any{"code": "SUP-FREE", "name": "Free"})

	body := testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/suppliers/"+other.ID, token,
			map[string]any{"code": "SUP-CLAIMED"}),
		http.StatusConflict, "in_use")
	if body.Details["code"] != "SUP-CLAIMED" {
		t.Errorf("details = %v, want the colliding code named", body.Details)
	}
}

func TestSupplierPatchAndRestoreAreNotFoundForAnUnknownID(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Absent Suppliers Ltd")
	token := procurementUser(t, f, "admin")

	missing := testsupport.NoSuchTenant.String()
	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/suppliers/"+missing, token,
			map[string]any{"name": "Nobody"}),
		http.StatusNotFound, "not_found")
	testsupport.AssertErrorCode(t,
		h.Post(t, "/api/procurement/suppliers/"+missing+"/restore", token, nil),
		http.StatusNotFound, "not_found")
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/procurement/suppliers/"+missing, token),
		http.StatusNotFound, "not_found")
}

// A deleted supplier cannot be edited — restore it first, for the same reason a
// deleted product cannot: two rows fighting over one code is not a state anybody
// chose.
func TestSupplierPatchIsRefusedWhileDeleted(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Recycled Suppliers Ltd")
	token := procurementUser(t, f, "admin")

	created := newSupplier(t, h, token, map[string]any{"code": "SUP-BIN", "name": "Binned"})
	testsupport.AssertStatus(t,
		h.Delete(t, "/api/procurement/suppliers/"+created.ID, token), http.StatusOK)

	testsupport.AssertErrorCode(t,
		h.Patch(t, "/api/procurement/suppliers/"+created.ID, token,
			map[string]any{"name": "Edited in the bin"}),
		http.StatusNotFound, "not_found")
}

// --------------------------------------------------------------------------
// PATCH /api/procurement/requisitions/:id
// --------------------------------------------------------------------------

func TestRequisitionPatchRepointsWarehouseSupplierAndNotes(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Editable Drafts Ltd")
	token := procurementUser(t, f, "user")

	// The draft has to be raised by the caller: only its author may change it.
	draft := createRequisition(t, h, token, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"supplierId":  f.SupplierID.String(),
		"lines":       []map[string]any{line(f.ProductID, "10", "100.00")},
	})
	path := "/api/procurement/requisitions/" + draft.ID

	otherWarehouse := f.NewWarehouse(t, "Second depot")
	otherSupplier := f.NewSupplier(t, "Second supplier")

	moved := testsupport.Decode[requisition](t,
		h.Patch(t, path, token, map[string]any{
			"warehouseId": otherWarehouse.String(),
			"supplierId":  otherSupplier.String(),
			"notes":       "  changed my mind  ",
		}))
	if moved.WarehouseID != otherWarehouse.String() {
		t.Errorf("warehouseId = %q, want %q", moved.WarehouseID, otherWarehouse)
	}
	if moved.SupplierID == nil || *moved.SupplierID != otherSupplier.String() {
		t.Errorf("supplierId = %v, want %q", moved.SupplierID, otherSupplier)
	}
	if moved.Notes == nil || *moved.Notes != "changed my mind" {
		t.Errorf("notes = %v, want the value trimmed", moved.Notes)
	}

	// An empty supplier clears it: a requisition may legitimately not name one
	// until approval (§8.3).
	cleared := testsupport.Decode[requisition](t,
		h.Patch(t, path, token, map[string]any{"supplierId": ""}))
	if cleared.SupplierID != nil {
		t.Errorf("supplierId = %v, want it cleared", *cleared.SupplierID)
	}
	// And so does an empty note.
	blanked := testsupport.Decode[requisition](t,
		h.Patch(t, path, token, map[string]any{"notes": "   "}))
	if blanked.Notes != nil {
		t.Errorf("notes = %v, want it cleared", *blanked.Notes)
	}
}

func TestRequisitionPatchRefusesReferencesThatDoNotResolve(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Dangling References Ltd")
	token := procurementUser(t, f, "user")

	draft := createRequisition(t, h, token, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"lines":       []map[string]any{line(f.ProductID, "10", "100.00")},
	})
	path := "/api/procurement/requisitions/" + draft.ID
	missing := testsupport.NoSuchTenant.String()

	// A reference to something in another tenant is indistinguishable from a
	// reference to nothing, which is the point: RLS makes it invisible, so the
	// answer is 404 rather than 403.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{"warehouseId": missing}),
		http.StatusNotFound, "not_found")
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{"supplierId": missing}),
		http.StatusNotFound, "not_found")

	// A malformed id in the *body* is a 400, unlike one in the path: the caller
	// sent a field that is not an id, rather than asking for a document by a name
	// that cannot be one.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{"warehouseId": "not-a-uuid"}),
		http.StatusBadRequest, "malformed")

	// A line naming a product that does not resolve names the *line*, so the
	// author knows which part of the form is wrong.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{
			"lines": []map[string]any{line(testsupport.NoSuchTenant, "1", "1.00")},
		}),
		http.StatusNotFound, "not_found")
}

func TestRequisitionPatchRefusesAnEmptyChangeAndADuplicateProduct(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Repeated Lines Ltd")
	token := procurementUser(t, f, "user")

	draft := createRequisition(t, h, token, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"lines":       []map[string]any{line(f.ProductID, "10", "100.00")},
	})
	path := "/api/procurement/requisitions/" + draft.ID

	// A patch that asks for nothing is a mistake worth naming rather than a 200
	// that changed nothing.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{}), http.StatusBadRequest, "malformed")

	// One product twice on one requisition. Refused, because approving it would
	// produce a purchase order with two lines for the same thing — which
	// `pol_one_line_per_product` refuses at the database, and this is the same rule
	// said earlier and more usefully.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{
			"lines": []map[string]any{
				line(f.ProductID, "10", "100.00"),
				line(f.ProductID, "5", "100.00"),
			},
		}),
		http.StatusBadRequest, "malformed")

	// A quantity of zero is not a line.
	testsupport.AssertErrorCode(t,
		h.Patch(t, path, token, map[string]any{
			"lines": []map[string]any{line(f.ProductID, "0", "100.00")},
		}),
		http.StatusBadRequest, "malformed")
}

// Replacing the line set replaces it whole, which is the API's own contract — and
// the reason the frontend's editor starts from the lines the document has rather
// than from a blank row.
func TestRequisitionPatchReplacesTheWholeLineSet(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Replaced Lines Ltd")
	token := procurementUser(t, f, "user")

	draft := createRequisition(t, h, token, map[string]any{
		"warehouseId": f.WarehouseID.String(),
		"lines": []map[string]any{
			line(f.ProductID, "10", "100.00"),
			line(f.ProductAltID, "4", "25.00"),
		},
	})
	if draft.LineCount != 2 {
		t.Fatalf("lineCount = %d, want 2", draft.LineCount)
	}

	replaced := testsupport.Decode[requisition](t,
		h.Patch(t, "/api/procurement/requisitions/"+draft.ID, token, map[string]any{
			"lines": []map[string]any{line(f.ProductAltID, "6", "25.00")},
		}))
	if replaced.LineCount != 1 {
		t.Errorf("lineCount = %d, want the set replaced rather than appended to", replaced.LineCount)
	}
	if len(replaced.Lines) != 1 || replaced.Lines[0].ProductID != f.ProductAltID.String() {
		t.Errorf("lines = %+v, want only the second product", replaced.Lines)
	}
	// Renumbered from 1, because line numbers are the document's own and a gap
	// would read as a deleted line.
	if replaced.Lines[0].LineNo != 1 {
		t.Errorf("lineNo = %d, want 1", replaced.Lines[0].LineNo)
	}
}
