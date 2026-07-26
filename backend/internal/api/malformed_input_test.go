// Garbage in, 4xx out.
//
// WHY THIS IS ONE FILE RATHER THAN A CASE PER HANDLER. Phase 8's coverage pass found
// the same two branches unexercised in almost every handler in the application: the
// `pathUUID` miss and the query-parameter parse. Both are one line, both are
// trivially reachable, and both fail the same way when they are missing — a `uuid`
// parse error escaping into a **500**, which is the difference between "you asked
// for something that cannot exist" and "this server is broken".
//
// It matters more than a percentage: a 500 pages somebody, shows up in the error
// budget, and tells an attacker probing URLs that they found something. A 404 is the
// correct, boring answer. Table-driven, because the assertion is identical
// everywhere and the interesting content is the *list* of routes — a route missing
// from it is a route nobody checked.
package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// everyModuleAdmin is the widest tenant caller there is: staff with `admin` in all
// three modules. Deliberately not a tenant admin, so a refusal here is a real
// refusal and not the implicit-admin shortcut.
func everyModuleAdmin(t *testing.T, f *testsupport.TenantFixture) string {
	t.Helper()
	return f.NewUser(t, map[string]string{
		"procurement": "admin", "inventory": "admin", "finance": "admin",
	}).FirebaseUID
}

// A path parameter that is not a UUID is a request for a document that cannot
// exist. 404, never 500.
func TestAMalformedIDInAPathIsNotFoundRatherThan500(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Bad Paths Ltd")
	token := everyModuleAdmin(t, f)

	type call struct{ method, path string }
	calls := []call{
		{"GET", "/api/inventory/products/not-a-uuid"},
		{"PATCH", "/api/inventory/products/not-a-uuid"},
		{"DELETE", "/api/inventory/products/not-a-uuid"},
		{"POST", "/api/inventory/products/not-a-uuid/restore"},
		{"GET", "/api/inventory/warehouses/not-a-uuid"},
		{"PATCH", "/api/inventory/warehouses/not-a-uuid"},
		{"DELETE", "/api/inventory/warehouses/not-a-uuid"},
		{"POST", "/api/inventory/warehouses/not-a-uuid/restore"},
		{"GET", "/api/procurement/suppliers/not-a-uuid"},
		{"PATCH", "/api/procurement/suppliers/not-a-uuid"},
		{"DELETE", "/api/procurement/suppliers/not-a-uuid"},
		{"POST", "/api/procurement/suppliers/not-a-uuid/restore"},
		{"GET", "/api/procurement/requisitions/not-a-uuid"},
		{"PATCH", "/api/procurement/requisitions/not-a-uuid"},
		{"POST", "/api/procurement/requisitions/not-a-uuid/submit"},
		{"POST", "/api/procurement/requisitions/not-a-uuid/approve"},
		{"POST", "/api/procurement/requisitions/not-a-uuid/reject"},
		{"POST", "/api/procurement/requisitions/not-a-uuid/cancel"},
		{"GET", "/api/procurement/purchase-orders/not-a-uuid"},
		{"POST", "/api/procurement/purchase-orders/not-a-uuid/cancel"},
		{"GET", "/api/procurement/goods-receipts/not-a-uuid"},
	}

	for _, tc := range calls {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			// A body that would be valid if the id were, so the 404 is the id's
			// doing and not the body's.
			body := map[string]any{"name": "Anything", "reason": "Anything"}
			resp := h.Request(t, tc.method, tc.path, token, body)
			testsupport.AssertErrorCode(t, resp, http.StatusNotFound, "not_found")
		})
	}
}

// A receipt is posted with an Idempotency-Key, so it needs its own case: without
// the header the handler refuses before it ever looks at the path.
func TestAMalformedOrderIDOnAReceiptIsNotFound(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Bad Receipt Paths Ltd")
	token := f.NewUser(t, map[string]string{
		"procurement": "approver", "inventory": "user", "finance": "user",
	}).FirebaseUID

	resp := h.Request(t, "POST", "/api/procurement/purchase-orders/not-a-uuid/receipts",
		token, map[string]any{"lines": []map[string]any{}},
		[2]string{"Idempotency-Key", "key-for-a-bad-path"})
	testsupport.AssertErrorCode(t, resp, http.StatusNotFound, "not_found")
}

// A query parameter that will not parse is the caller's mistake, so it is a 400
// naming the parameter — not an empty list, which would be a wrong answer offered
// with confidence.
func TestAMalformedQueryParameterIsRefusedByName(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Bad Queries Ltd")
	token := everyModuleAdmin(t, f)

	cases := map[string]string{
		"/api/inventory/stock?productId=nope":              "productId",
		"/api/inventory/stock?warehouseId=nope":            "warehouseId",
		"/api/inventory/ledger?productId=nope":             "productId",
		"/api/inventory/ledger?warehouseId=nope":           "warehouseId",
		"/api/inventory/ledger?sourceId=nope":              "sourceId",
		"/api/inventory/ledger?entryType=teleported":       "entryType",
		"/api/inventory/ledger?sourceType=divination":      "sourceType",
		"/api/inventory/ledger?from=yesterday":             "from",
		"/api/inventory/ledger?to=soon":                    "to",
		"/api/procurement/purchase-orders?supplierId=nope": "supplierId",
		"/api/procurement/goods-receipts?poId=nope":        "poId",
	}

	for path, parameter := range cases {
		t.Run(path, func(t *testing.T) {
			body := testsupport.AssertErrorCode(t,
				h.Get(t, path, token), http.StatusBadRequest, "malformed")
			// The message names the parameter, because "malformed" on a list screen
			// with five filters is not something a person can act on.
			if !strings.Contains(body.Message, parameter) {
				t.Errorf("message = %q, want it to name %q", body.Message, parameter)
			}
		})
	}
}

// The enum parameters accept exactly the naming contract's values, and say what
// they would have accepted.
func TestLedgerEnumParametersListWhatTheyAccept(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Enum Filters Ltd")
	token := everyModuleAdmin(t, f)

	entry := testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/ledger?entryType=nope", token),
		http.StatusBadRequest, "malformed")
	for _, want := range []string{"receipt", "issue", "adjustment"} {
		if !strings.Contains(entry.Message, want) {
			t.Errorf("entryType message = %q, want it to offer %q", entry.Message, want)
		}
	}

	source := testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/ledger?sourceType=nope", token),
		http.StatusBadRequest, "malformed")
	for _, want := range []string{"goods_receipt", "manual_adjustment"} {
		if !strings.Contains(source.Message, want) {
			t.Errorf("sourceType message = %q, want it to offer %q", source.Message, want)
		}
	}

	// And every legal value is accepted, or the guard is stricter than the
	// contract.
	for _, value := range []string{"receipt", "issue", "adjustment"} {
		testsupport.AssertStatus(t,
			h.Get(t, "/api/inventory/ledger?entryType="+value, token), http.StatusOK)
	}
	for _, value := range []string{"goods_receipt", "manual_adjustment"} {
		testsupport.AssertStatus(t,
			h.Get(t, "/api/inventory/ledger?sourceType="+value, token), http.StatusOK)
	}
}

// An RFC 3339 window is accepted and normalised to UTC. Business dates use the
// tenant's zone (I7); a *filter* is an instant, and an instant is UTC.
func TestLedgerAcceptsAnRFC3339Window(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Windowed Ledger Ltd")
	token := everyModuleAdmin(t, f)

	f.PostLedger(t, f.ProductID, f.WarehouseID, "5", "receipt")

	// An offset that is not UTC, so the conversion has to happen rather than the
	// string being passed through.
	testsupport.AssertStatus(t,
		h.Get(t, "/api/inventory/ledger?from=2020-01-01T00:00:00%2B07:00&to=2999-01-01T00:00:00Z", token),
		http.StatusOK)

	// And a window that excludes everything returns an empty page, not an error.
	page := testsupport.Decode[list[ledgerEntry]](t,
		h.Get(t, "/api/inventory/ledger?from=2999-01-01T00:00:00Z", token))
	if page.TotalItems != 0 {
		t.Errorf("totalItems = %d, want 0 for a window in the future", page.TotalItems)
	}
}

// A body that is not JSON at all. Every write endpoint parses one, and the answer
// is a 400 rather than the handler carrying on with a zero-valued request.
func TestABodyThatIsNotJSONIsRefused(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Bad Bodies Ltd")
	token := everyModuleAdmin(t, f)

	for _, path := range []string{
		"/api/inventory/products",
		"/api/inventory/warehouses",
		"/api/procurement/suppliers",
		"/api/procurement/requisitions",
	} {
		resp := h.Request(t, "POST", path, token, "not json at all")
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST %s with a non-JSON body = %d, want 400", path, resp.StatusCode)
		}
	}
}

// `includeDeleted` is module `admin` only (§9.0), and the refusal is per module —
// which is the branch a single-module caller cannot reach.
func TestTheRecycleBinIsRefusedPerModule(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Partial Admin Ltd")

	// `admin` in procurement, `user` in inventory: the same caller may see one
	// recycle bin and not the other.
	token := f.NewUser(t, map[string]string{
		"procurement": "admin", "inventory": "user",
	}).FirebaseUID

	testsupport.AssertStatus(t,
		h.Get(t, "/api/procurement/suppliers?includeDeleted=true", token), http.StatusOK)
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/products?includeDeleted=true", token),
		http.StatusForbidden, "insufficient_module_role")
	testsupport.AssertErrorCode(t,
		h.Get(t, "/api/inventory/warehouses?includeDeleted=true", token),
		http.StatusForbidden, "insufficient_module_role")
}
