// The two platform-plane reads Phase 3 built and never asserted.
//
// `go tool cover` put `getTenant` and `listTenantModules` at 0.0% — the workspace
// detail screen and the entitlement matrix behind it, both reachable in the browser
// since Phase 3. `TestSetTenantModuleTogglesOneEntitlement` exercises the *write*
// and then re-reads the list endpoint, so the detail route stayed unexercised.
//
// The interesting assertion is the one about `modules`: the matrix is a LEFT JOIN
// from the catalogue, not a projection of `tenant_modules`, and only a tenant that
// has never held a module can tell those apart. A screen built from
// `tenant_modules` alone cannot offer a toggle for a module the tenant never had —
// which is to say, it cannot sell anything.
package api_test

import (
	"net/http"
	"testing"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

func TestTenantDetailReportsTheWholeEntitlementMatrix(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	// Created with procurement only, so the other two modules have no
	// `tenant_modules` row at all — the case the LEFT JOIN exists for.
	body := newTenantRequest("detail-read")
	body["modules"] = []string{"procurement"}
	created := testsupport.Decode[createTenantBody](t,
		h.Post(t, "/api/admin/tenants", super.FirebaseUID, body))

	got := testsupport.Decode[tenantBody](t,
		h.Get(t, "/api/admin/tenants/"+created.Tenant.ID, super.FirebaseUID))

	if got.ID != created.Tenant.ID {
		t.Fatalf("id = %q, want %q", got.ID, created.Tenant.ID)
	}
	if got.Slug != "detail-read" {
		t.Errorf("slug = %q, want detail-read", got.Slug)
	}
	if got.Status != "active" {
		t.Errorf("status = %q, want active", got.Status)
	}

	// Every module in the catalogue is present, enabled or not.
	if len(got.Modules) != 3 {
		t.Fatalf("modules = %d rows, want one per catalogue module", len(got.Modules))
	}
	enabled := map[string]bool{}
	for _, module := range got.Modules {
		enabled[module.Code] = module.Enabled
		if module.Name == "" {
			t.Errorf("module %q has no name — the catalogue was not joined", module.Code)
		}
	}
	if !enabled["procurement"] {
		t.Error("procurement should be enabled")
	}
	if enabled["inventory"] || enabled["finance"] {
		t.Errorf("modules the tenant was not given are enabled: %v", enabled)
	}

	// The counts on the detail row are the same derived numbers the list carries:
	// one seeded admin, who is also the only active user.
	if got.UserCount != 1 || got.AdminCount != 1 {
		t.Errorf("userCount/adminCount = %d/%d, want 1/1", got.UserCount, got.AdminCount)
	}
	if got.ModuleCount != 1 {
		t.Errorf("moduleCount = %d, want 1", got.ModuleCount)
	}
}

// The matrix on its own, which is what the module toggles re-read after a PUT.
func TestTenantModulesEndpointReturnsTheMatrixAlone(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	created := testsupport.Decode[createTenantBody](t,
		h.Post(t, "/api/admin/tenants", super.FirebaseUID, newTenantRequest("matrix-read")))
	path := "/api/admin/tenants/" + created.Tenant.ID + "/modules"

	before := testsupport.Decode[[]struct {
		Code    string `json:"code"`
		Enabled bool   `json:"enabled"`
	}](t, h.Get(t, path, super.FirebaseUID))
	if len(before) != 3 {
		t.Fatalf("matrix = %d rows, want 3", len(before))
	}

	// And it tracks the write, with no restart and no cache to invalidate —
	// entitlement is read from the database on every request (I9, B5).
	testsupport.AssertStatus(t,
		h.Put(t, "/api/admin/tenants/"+created.Tenant.ID+"/modules/finance",
			super.FirebaseUID, map[string]any{"enabled": false}),
		http.StatusOK)

	after := testsupport.Decode[[]struct {
		Code    string `json:"code"`
		Enabled bool   `json:"enabled"`
	}](t, h.Get(t, path, super.FirebaseUID))
	for _, module := range after {
		if module.Code == "finance" && module.Enabled {
			t.Error("finance is still enabled after being switched off")
		}
	}
}

func TestTenantReadsAreNotFoundForAnUnknownOrMalformedID(t *testing.T) {
	h := testsupport.NewHarness(t)
	super := h.DB.NewSuperadmin(t)

	for _, path := range []string{
		"/api/admin/tenants/" + testsupport.NoSuchTenant.String(),
		"/api/admin/tenants/" + testsupport.NoSuchTenant.String() + "/modules",
		"/api/admin/tenants/not-a-uuid",
		"/api/admin/tenants/not-a-uuid/modules",
	} {
		testsupport.AssertErrorCode(t,
			h.Get(t, path, super.FirebaseUID), http.StatusNotFound, "not_found")
	}
}

// The plane guard, on the two routes that had no test to carry it. A tenant admin
// is an administrator of their own workspace and has no business on the platform
// plane (§5.7) — including for their *own* tenant, which is the case a reader might
// expect to be allowed.
func TestTenantDetailIsRefusedToATenantAdmin(t *testing.T) {
	h := testsupport.NewHarness(t)
	f := h.DB.NewTenant(t, "Curious Workspace Ltd")
	admin := f.NewAdmin(t)

	for _, path := range []string{
		"/api/admin/tenants/" + f.ID.String(),
		"/api/admin/tenants/" + f.ID.String() + "/modules",
	} {
		testsupport.AssertErrorCode(t,
			h.Get(t, path, admin.FirebaseUID), http.StatusForbidden, "forbidden")
	}
}
